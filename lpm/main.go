package main

import (
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/itohio/golpm/pkg/config"
	"github.com/itohio/golpm/pkg/lpm"
	"github.com/itohio/golpm/pkg/meter"
	"github.com/itohio/golpm/pkg/sample"
	"github.com/itohio/golpm/pkg/scope"
)

func main() {
	var (
		portFlag       = flag.String("p", "", "Serial port override (e.g., COM3 or /dev/ttyACM0)")
		configFlag     = flag.String("config", "config.yaml", "Configuration file path")
		mockFlag       = flag.Bool("mock", false, "Use mocked device instead of serial port")
		statisticsFlag = flag.Bool("statistics", false, "Calculate and log statistics (variability, stddev, optimal EMA parameters)")
	)
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configFlag)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Override serial port if provided via command line
	if *portFlag != "" {
		cfg.Serial.Port = *portFlag
	}

	// Create Fyne application
	application := app.NewWithID("com.itohio.golpm")

	// Create main window
	window := application.NewWindow("Laser Power Meter")
	window.Resize(fyne.NewSize(1200, 800))
	window.CenterOnScreen()

	// Create power meter
	powerMeter := meter.New(cfg)

	// Create application state
	appState := &appState{
		cfg:           cfg,
		device:        nil,
		powerMeter:    powerMeter,
		window:        window,
		useMock:       *mockFlag,
		useStatistics: *statisticsFlag,
	}

	// Create toolbar
	toolbar := createToolbar(appState)

	// Create scope widget for graph display
	scopeWidget := scope.New(cfg)
	appState.scopeWidget = scopeWidget

	// Create border layout with toolbar at top and scope widget as content
	container := container.NewBorder(
		toolbar,
		nil,
		nil,
		nil,
		scopeWidget,
	)

	window.SetContent(container)
	window.ShowAndRun()
}

// measurementChain tracks the components of the measurement chain for graceful shutdown.
type measurementChain struct {
	device               lpm.Device
	rawSamples           <-chan lpm.RawSample
	rawSamplesForTee     <-chan lpm.RawSample
	heaterStateGoroutine chan struct{} // Closed when heater state goroutine exits
	samplesStream        <-chan sample.Sample
	meterGoroutine       chan struct{} // Closed when meter goroutine exits
}

// appState holds the application state.
type appState struct {
	cfg                *config.Config
	device             lpm.Device
	powerMeter         *meter.Meter
	scopeWidget        *scope.ScopeWidget
	window             fyne.Window
	connectBtn         *widget.Button
	heater1Btn         *widget.Button
	heater2Btn         *widget.Button
	heater3Btn         *widget.Button
	addCalPointBtn     *widget.Button
	heaterIncrementBtn *widget.Button
	heaterOffBtn       *widget.Button
	useMock            bool
	useStatistics      bool
	heaterState        [3]bool           // Current heater states [heater1, heater2, heater3]
	chain              *measurementChain // Current measurement chain (nil if not connected)

	// Throttling for scope updates
	lastUpdateTime time.Time
	updateMu       sync.Mutex
}

// createToolbar creates the application toolbar with Connect, Settings, Measure, and Heater buttons.
func createToolbar(state *appState) fyne.CanvasObject {
	// Connect button with icon
	connectBtn := widget.NewButtonWithIcon("", theme.LoginIcon(), func() {
		handleConnect(state)
	})
	connectBtn.Importance = widget.HighImportance
	state.connectBtn = connectBtn

	// Settings button with icon
	settingsBtn := widget.NewButtonWithIcon("", theme.SettingsIcon(), func() {
		showSettingsDialog(state)
	})

	// Create heater buttons with better icons
	// Using radio button checked/unchecked icons to represent heater state
	heater1Btn := widget.NewButtonWithIcon("", theme.RadioButtonIcon(), func() {
		handleHeaterToggle(state, 0)
	})
	heater1Btn.Disable()
	state.heater1Btn = heater1Btn

	heater2Btn := widget.NewButtonWithIcon("", theme.RadioButtonIcon(), func() {
		handleHeaterToggle(state, 1)
	})
	heater2Btn.Disable()
	state.heater2Btn = heater2Btn

	heater3Btn := widget.NewButtonWithIcon("", theme.RadioButtonIcon(), func() {
		handleHeaterToggle(state, 2)
	})
	heater3Btn.Disable()
	state.heater3Btn = heater3Btn

	// Add calibration point button
	// Only visible when at least one heater is on
	addCalPointBtn := widget.NewButtonWithIcon("Add Cal Point", theme.ContentAddIcon(), func() {
		handleAddCalibrationPoint(state)
	})
	addCalPointBtn.Disable()
	addCalPointBtn.Hide() // Start hidden - will be shown when heater is turned on
	state.addCalPointBtn = addCalPointBtn

	// Heater increment button (binary counter)
	// Increments heaters as binary: 000 -> 100 -> 010 -> 110 -> 001 -> 101 -> 011 -> 111 -> 000
	heaterIncrementBtn := widget.NewButtonWithIcon("", theme.MediaFastForwardIcon(), func() {
		handleHeaterIncrement(state)
	})
	heaterIncrementBtn.Disable()
	state.heaterIncrementBtn = heaterIncrementBtn

	// Heater off button (turn off all heaters)
	heaterOffBtn := widget.NewButtonWithIcon("", theme.MediaStopIcon(), func() {
		handleHeaterOff(state)
	})
	heaterOffBtn.Disable()
	heaterOffBtn.Importance = widget.DangerImportance
	state.heaterOffBtn = heaterOffBtn

	// Create separators
	separator1 := widget.NewSeparator()
	separator2 := widget.NewSeparator()

	// Create toolbar with buttons on left and heater controls aligned to the right
	// Layout: [Connect] [Settings] ... [Add Cal] | [>>] [H1] [H2] [H3] | [Off]
	return container.NewBorder(
		nil, // top
		nil, // bottom
		container.NewHBox(connectBtn, settingsBtn), // left
		container.NewHBox( // right
			addCalPointBtn,
			separator1,
			heaterIncrementBtn,
			heater1Btn,
			heater2Btn,
			heater3Btn,
			separator2,
			heaterOffBtn,
		),
		nil, // center (spacer)
	)
}

// closeMeasurementChain gracefully closes the measurement chain.
// Waits for all goroutines to finish and channels to drain.
func closeMeasurementChain(chain *measurementChain) {
	if chain == nil {
		return
	}

	// Close device - this will close the rawSamples channel
	if chain.device != nil {
		chain.device.Close()
	}

	// Wait for heater state goroutine to finish
	if chain.heaterStateGoroutine != nil {
		<-chain.heaterStateGoroutine
	}

	// Wait for meter goroutine to finish
	// The meter goroutine will exit when samplesStream closes
	// The samplesStream will close when converters finish draining
	if chain.meterGoroutine != nil {
		<-chain.meterGoroutine
	}
}

// handleConnect handles the connect/disconnect button click.
func handleConnect(state *appState) {
	if state.device != nil && state.device.IsConnected() {
		// Disconnect - gracefully close measurement chain
		closeMeasurementChain(state.chain)
		state.chain = nil
		state.device = nil
		// Connect button icon doesn't change
		state.heater1Btn.Disable()
		state.heater2Btn.Disable()
		state.heater3Btn.Disable()
		state.addCalPointBtn.Disable()
		state.heaterIncrementBtn.Disable()
		state.heaterOffBtn.Disable()
		// Reset heater states
		state.heaterState = [3]bool{false, false, false}
		updateHeaterButtonStates(state)
		if state.useMock {
			fmt.Println("Disconnected from mocked device")
		} else {
			fmt.Println("Disconnected from serial port")
		}
	} else {
		// Connect
		var device lpm.Device
		if state.useMock {
			device = lpm.NewMock(&state.cfg.Mock)
			fmt.Println("Using mocked device")
		} else {
			device = lpm.New(state.cfg.Serial.Port, lpm.DefaultBaudRate, lpm.DefaultBufferSize)
		}

		if err := device.Connect(); err != nil {
			if state.useMock {
				dialog.ShowError(fmt.Errorf("failed to connect to mocked device: %w", err), state.window)
			} else {
				dialog.ShowError(fmt.Errorf("failed to connect to %s: %w", state.cfg.Serial.Port, err), state.window)
			}
			return
		}
		state.device = device
		if state.useMock {
			fmt.Printf("Connected to mocked device\n")
		} else {
			fmt.Printf("Connected to serial port: %s\n", state.cfg.Serial.Port)
		}

		// Enable heater buttons
		state.heater1Btn.Enable()
		state.heater2Btn.Enable()
		state.heater3Btn.Enable()
		state.addCalPointBtn.Enable()
		state.heaterIncrementBtn.Enable()
		state.heaterOffBtn.Enable()

		// Reset meter shutdown flag for new chain
		state.powerMeter.ResetShutdown()

		// Register callback with power meter to update scope widget
		// This must be done before starting the measurement chain
		// Throttle updates to ~60 FPS (16.67ms between updates) to ensure smooth UI
		const updateInterval = 16 * time.Millisecond // ~60 FPS
		state.powerMeter.OnUpdate(func(samples []sample.Sample, derivatives []float64, pulses []meter.Pulse) {
			// Throttle updates to prevent UI from being overwhelmed
			state.updateMu.Lock()
			now := time.Now()
			timeSinceLastUpdate := now.Sub(state.lastUpdateTime)
			state.updateMu.Unlock()

			// Skip update if too soon since last update
			if timeSinceLastUpdate < updateInterval {
				return
			}

			// Calculate current heater power from latest sample
			var heaterPower float64
			if len(samples) > 0 {
				heaterPower = samples[len(samples)-1].HeaterPower
			}

			// Update timestamp
			state.updateMu.Lock()
			state.lastUpdateTime = now
			state.updateMu.Unlock()

			// Update scope widget on main thread
			// Scope widget handles downsampling internally, so pass full data
			fyne.Do(func() {
				state.scopeWidget.UpdateData(samples, derivatives, pulses, heaterPower)
			})
		})

		// Create converter pipeline with chaining support
		rawSamples := device.Samples()

		// Tee raw samples: one branch for heater state updates, one for converter chain
		// We need to tee because we need to read from the channel twice:
		// 1. For heater state synchronization
		// 2. For the converter chain
		rawSamplesForConverter := teeChannel(rawSamples)

		// Track goroutines for graceful shutdown
		heaterStateDone := make(chan struct{})
		meterDone := make(chan struct{})

		// Update heater states from raw samples (only when state changes)
		go func() {
			defer close(heaterStateDone)
			for rawSample := range rawSamples {
				updateHeaterStatesFromSample(state, rawSample)
			}
		}()

		// Chain converters:
		// 1. Base conversion from raw samples
		// 2. Statistics collection (if enabled) - collects stats on raw converted samples
		// 3. Median filter on main fields to remove spikes (hardware-induced spikes when heaters turn on)
		// 4. EMA smoothing on all fields except Change (if smoothing enabled)
		// 5. Downsampling to target sample rate (if enabled)
		// 6. Differentiation to calculate Change field from Reading
		// 7. Filter on Change field (EMA/MA/MM - configurable)
		// Increase buffer size to prevent channel full errors
		baseStream := sample.NewConverter(state.cfg, 500)(rawSamplesForConverter)

		// Apply statistics collection (if enabled)
		// This must be done BEFORE any filtering to capture raw signal characteristics
		var statsStream <-chan sample.Sample
		if state.useStatistics {
			// Use the configured smoothing alpha for EMA comparison
			log.Printf("Statistics Smoothing alpha: %f", state.cfg.Measurement.SmoothingAlpha)
			statsStream = sample.NewStatisticsConverter(state.cfg.Measurement.SmoothingAlpha, 500)(baseStream)
		} else {
			statsStream = baseStream
		}

		// Apply median filter to remove spikes from main signal (Reading, Voltage)
		// This filters out hardware-induced spikes when heaters turn on
		// Note: HeaterPower is never filtered - it's only calculated
		mainFields := sample.FieldReading | sample.FieldVoltage
		var spikeFilteredStream <-chan sample.Sample
		if state.cfg.Measurement.SpikeFilterWindowSize > 0 {
			spikeFilteredStream = sample.NewMMFilter(state.cfg.Measurement.SpikeFilterWindowSize, mainFields, 500)(statsStream)
		} else {
			// Spike filtering disabled, use stats stream directly
			spikeFilteredStream = statsStream
		}

		// Apply EMA smoothing on all fields except Change and HeaterPower (if smoothing enabled)
		// Note: HeaterPower is never filtered - it's only calculated
		var smoothedStream <-chan sample.Sample
		if state.cfg.Measurement.SmoothingAlpha > 0 {
			// Apply EMA to Reading and Voltage (Change will be calculated later, HeaterPower is never filtered)
			smoothedStream = sample.NewEMAFilter(state.cfg.Measurement.SmoothingAlpha, mainFields, 500)(spikeFilteredStream)
		} else {
			// No smoothing, use spike-filtered stream directly
			smoothedStream = spikeFilteredStream
		}

		// Apply downsampling to target sample rate (if enabled)
		var downsampledStream <-chan sample.Sample
		if state.cfg.Measurement.DownsampleRate != nil && *state.cfg.Measurement.DownsampleRate > 0 {
			downsampledStream = sample.NewDownsamplingConverter(*state.cfg.Measurement.DownsampleRate, 500)(smoothedStream)
		} else {
			// No downsampling, use smoothed stream directly
			downsampledStream = smoothedStream
		}

		// Always apply differentiation to calculate Change field from Reading
		diffStream := sample.NewDifferentiationConverter(500)(downsampledStream)

		// Apply filter on Change field (configurable: EMA, MA, or MM)
		var samplesStream <-chan sample.Sample
		changeFields := sample.FieldChange
		filterType := state.cfg.Measurement.ChangeFilterType
		if filterType == "" {
			filterType = "ema" // Default
		}

		switch filterType {
		case "ema", "EMA":
			if state.cfg.Measurement.ChangeFilterAlpha > 0 {
				samplesStream = sample.NewEMAFilter(state.cfg.Measurement.ChangeFilterAlpha, changeFields, 500)(diffStream)
			} else {
				samplesStream = diffStream // No filtering if alpha is 0
			}
		case "ma", "MA":
			windowDuration := state.cfg.Measurement.ChangeFilterWindowSize
			if windowDuration <= 0 {
				windowDuration = 200 * time.Millisecond // Default: 200ms
			}
			samplesStream = sample.NewMAFilter(windowDuration, changeFields, 500)(diffStream)
		case "mm", "MM":
			windowDuration := state.cfg.Measurement.ChangeFilterWindowSize
			if windowDuration <= 0 {
				windowDuration = 200 * time.Millisecond // Default: 200ms
			}
			samplesStream = sample.NewMMFilter(windowDuration, changeFields, 500)(diffStream)
		default:
			// Unknown filter type, use EMA as fallback
			if state.cfg.Measurement.ChangeFilterAlpha > 0 {
				samplesStream = sample.NewEMAFilter(state.cfg.Measurement.ChangeFilterAlpha, changeFields, 500)(diffStream)
			} else {
				samplesStream = diffStream
			}
		}

		// Process samples through power meter (starts measurement automatically)
		go func() {
			defer close(meterDone)
			state.powerMeter.ProcessSamples(samplesStream)
		}()

		// Store chain for graceful shutdown
		state.chain = &measurementChain{
			device:               device,
			rawSamples:           rawSamples,
			rawSamplesForTee:     rawSamplesForConverter,
			heaterStateGoroutine: heaterStateDone,
			samplesStream:        samplesStream,
			meterGoroutine:       meterDone,
		}
	}
}

// teeChannel creates a tee of the input channel, returning a new channel that receives
// all values from the input. This allows multiple consumers of the same channel.
func teeChannel(in <-chan lpm.RawSample) <-chan lpm.RawSample {
	out := make(chan lpm.RawSample, 100)

	go func() {
		defer close(out)
		for sample := range in {
			out <- sample
		}
	}()

	return out
}
