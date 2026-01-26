package main

import (
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/itohio/golpm/pkg/config"
	"github.com/itohio/golpm/pkg/lpm"
	"github.com/itohio/golpm/pkg/meter"
)

// showSettingsDialog displays a settings dialog with tabs for all configuration options.
func showSettingsDialog(state *appState) {
	// Create tabs
	tabs := container.NewAppTabs(
		createSerialTab(state),
		createVoltageDividerTab(state),
		createHeatersTab(state),
		createMeasurementTab(state),
		createCalibrationTab(state),
		createMockTab(state),
	)

	// Create dialog with tabs as content
	content := container.NewBorder(nil, nil, nil, nil, tabs)
	content.Resize(fyne.NewSize(600, 500))

	d := dialog.NewCustom("Settings", "Close", content, state.window)
	d.Resize(fyne.NewSize(600, 500))
	d.Show()
}

// createSerialTab creates the Serial configuration tab.
func createSerialTab(state *appState) *container.TabItem {
	// Get available serial ports
	ports, err := lpm.Ports()
	portOptions := []string{}
	portMap := make(map[string]string) // Map display name to actual port name

	if err == nil {
		for _, port := range ports {
			displayName := port.Name
			if port.Description != "" && port.Description != port.Name {
				displayName = fmt.Sprintf("%s (%s)", port.Name, port.Description)
			}
			portOptions = append(portOptions, displayName)
			portMap[displayName] = port.Name
		}
	}

	// Add current port if not in list
	currentPort := state.cfg.Serial.Port
	currentDisplay := currentPort
	found := false
	for _, opt := range portOptions {
		if portMap[opt] == currentPort {
			currentDisplay = opt
			found = true
			break
		}
	}
	if !found && currentPort != "" {
		portOptions = append(portOptions, currentPort)
		portMap[currentPort] = currentPort
		currentDisplay = currentPort
	}

	portSelect := widget.NewSelect(portOptions, func(selected string) {
		// Selection handler - will be called on submit
	})
	if currentDisplay != "" {
		portSelect.SetSelected(currentDisplay)
	}

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Serial Port", Widget: portSelect},
		},
		OnSubmit: func() {
			if portSelect.Selected != "" {
				selectedPort := portMap[portSelect.Selected]
				if selectedPort == "" {
					selectedPort = portSelect.Selected // Fallback to selected text
				}

				// Check if port changed and device is connected
				portChanged := state.cfg.Serial.Port != selectedPort
				wasConnected := state.device != nil && state.device.IsConnected()

				state.cfg.Serial.Port = selectedPort
				if err := state.cfg.Save("config.yaml"); err != nil {
					dialog.ShowError(fmt.Errorf("failed to save config: %w", err), state.window)
					return
				}

				// If port changed and device was connected, restart the measurement chain
				if portChanged && wasConnected {
					// Gracefully close old chain
					closeMeasurementChain(state.chain)
					state.chain = nil

					// Close old device
					if state.device != nil {
						state.device.Close()
						state.device = nil
					}

					// Reconnect with new port
					handleConnect(state)
				}
			}
		},
	}

	return container.NewTabItem("Serial", form)
}

// createVoltageDividerTab creates the Voltage Divider configuration tab.
func createVoltageDividerTab(state *appState) *container.TabItem {
	r1Entry := widget.NewEntry()
	r1Entry.SetText(fmt.Sprintf("%.0f", state.cfg.VoltageDivider.R1))

	r2Entry := widget.NewEntry()
	r2Entry.SetText(fmt.Sprintf("%.0f", state.cfg.VoltageDivider.R2))

	vrefEntry := widget.NewEntry()
	vrefEntry.SetText(fmt.Sprintf("%.2f", state.cfg.VoltageDivider.VRef))

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "R1 (Ω)", Widget: r1Entry},
			{Text: "R2 (Ω)", Widget: r2Entry},
			{Text: "VRef (V)", Widget: vrefEntry},
		},
		OnSubmit: func() {
			if r1, err := strconv.ParseFloat(r1Entry.Text, 64); err == nil {
				state.cfg.VoltageDivider.R1 = r1
			}
			if r2, err := strconv.ParseFloat(r2Entry.Text, 64); err == nil {
				state.cfg.VoltageDivider.R2 = r2
			}
			if vref, err := strconv.ParseFloat(vrefEntry.Text, 64); err == nil {
				state.cfg.VoltageDivider.VRef = vref
			}
			if err := state.cfg.Save("config.yaml"); err != nil {
				dialog.ShowError(fmt.Errorf("failed to save config: %w", err), state.window)
			}
		},
	}

	return container.NewTabItem("Voltage Divider", form)
}

// createHeatersTab creates the Heaters configuration tab.
func createHeatersTab(state *appState) *container.TabItem {
	heater1Entry := widget.NewEntry()
	heater1Entry.SetText(fmt.Sprintf("%.0f", state.cfg.Heaters[0].Resistance))

	heater2Entry := widget.NewEntry()
	heater2Entry.SetText(fmt.Sprintf("%.0f", state.cfg.Heaters[1].Resistance))

	heater3Entry := widget.NewEntry()
	heater3Entry.SetText(fmt.Sprintf("%.0f", state.cfg.Heaters[2].Resistance))

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Heater 1 Resistance (Ω)", Widget: heater1Entry},
			{Text: "Heater 2 Resistance (Ω)", Widget: heater2Entry},
			{Text: "Heater 3 Resistance (Ω)", Widget: heater3Entry},
		},
		OnSubmit: func() {
			if r1, err := strconv.ParseFloat(heater1Entry.Text, 64); err == nil {
				state.cfg.Heaters[0].Resistance = r1
			}
			if r2, err := strconv.ParseFloat(heater2Entry.Text, 64); err == nil {
				state.cfg.Heaters[1].Resistance = r2
			}
			if r3, err := strconv.ParseFloat(heater3Entry.Text, 64); err == nil {
				state.cfg.Heaters[2].Resistance = r3
			}
			if err := state.cfg.Save("config.yaml"); err != nil {
				dialog.ShowError(fmt.Errorf("failed to save config: %w", err), state.window)
			}
			// Update heater button labels
			state.heater1Btn.SetText(fmt.Sprintf("H1 (~%.0fΩ)", state.cfg.Heaters[0].Resistance))
			state.heater2Btn.SetText(fmt.Sprintf("H2 (~%.0fΩ)", state.cfg.Heaters[1].Resistance))
			state.heater3Btn.SetText(fmt.Sprintf("H3 (~%.0fΩ)", state.cfg.Heaters[2].Resistance))
		},
	}

	return container.NewTabItem("Heaters", form)
}

// createMeasurementTab creates the Measurement configuration tab.
func createMeasurementTab(state *appState) *container.TabItem {
	windowSecondsEntry := widget.NewEntry()
	windowSecondsEntry.SetText(fmt.Sprintf("%.1f", state.cfg.Measurement.WindowSeconds))

	pulseThresholdEntry := widget.NewEntry()
	pulseThresholdEntry.SetText(fmt.Sprintf("%.3f", state.cfg.Measurement.PulseThresholdMVS))

	pulseLineFitRangeEntry := widget.NewEntry()
	pulseLineFitRangeEntry.SetText(fmt.Sprintf("%.3f", state.cfg.Measurement.PulseLineFitRangeMVS))

	minPulseDurationEntry := widget.NewEntry()
	minPulseDurationEntry.SetText(fmt.Sprintf("%.1f", state.cfg.Measurement.MinPulseDuration))

	smoothingAlphaEntry := widget.NewEntry()
	smoothingAlphaEntry.SetText(fmt.Sprintf("%.2f", state.cfg.Measurement.SmoothingAlpha))

	spikeFilterWindowSizeEntry := widget.NewEntry()
	spikeFilterWindowSizeEntry.SetText(state.cfg.Measurement.SpikeFilterWindowSize.String())

	downsampleRateEntry := widget.NewEntry()
	if state.cfg.Measurement.DownsampleRate != nil {
		downsampleRateEntry.SetText(state.cfg.Measurement.DownsampleRate.String())
	} else {
		// Show default value (1s) but user can change it
		downsampleRateEntry.SetText("1s")
	}

	changeFilterTypeSelect := widget.NewSelect([]string{"ema", "ma", "mm"}, func(selected string) {})
	changeFilterTypeSelect.SetSelected(state.cfg.Measurement.ChangeFilterType)
	if changeFilterTypeSelect.Selected == "" {
		changeFilterTypeSelect.SetSelected("ema") // Default
	}

	changeFilterAlphaEntry := widget.NewEntry()
	changeFilterAlphaEntry.SetText(fmt.Sprintf("%.2f", state.cfg.Measurement.ChangeFilterAlpha))

	changeFilterWindowSizeEntry := widget.NewEntry()
	changeFilterWindowSizeEntry.SetText(state.cfg.Measurement.ChangeFilterWindowSize.String())

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Window (seconds)", Widget: windowSecondsEntry},
			{Text: "Pulse Threshold (mV/s)", Widget: pulseThresholdEntry},
			{Text: "Pulse Fit Range (mV/s)", Widget: pulseLineFitRangeEntry},
			{Text: "Min Pulse Duration (s)", Widget: minPulseDurationEntry},
			{Text: "Smoothing Alpha (0-1, 0=disabled)", Widget: smoothingAlphaEntry},
			{Text: "Spike Filter Window Size (0=disabled)", Widget: spikeFilterWindowSizeEntry},
			{Text: "Downsample Rate (e.g., 1s, 0s=disabled)", Widget: downsampleRateEntry},
			{Text: "Change Filter Type (ema/ma/mm)", Widget: changeFilterTypeSelect},
			{Text: "Change Filter Alpha (0-1, for EMA)", Widget: changeFilterAlphaEntry},
			{Text: "Change Filter Window Size (for MA/MM)", Widget: changeFilterWindowSizeEntry},
		},
		OnSubmit: func() {
			if ws, err := strconv.ParseFloat(windowSecondsEntry.Text, 64); err == nil {
				state.cfg.Measurement.WindowSeconds = ws
			}
			if pt, err := strconv.ParseFloat(pulseThresholdEntry.Text, 64); err == nil {
				state.cfg.Measurement.PulseThresholdMVS = pt
			}
			if plr, err := strconv.ParseFloat(pulseLineFitRangeEntry.Text, 64); err == nil {
				state.cfg.Measurement.PulseLineFitRangeMVS = plr
			}
			if mpd, err := strconv.ParseFloat(minPulseDurationEntry.Text, 64); err == nil {
				state.cfg.Measurement.MinPulseDuration = mpd
			}
			if sa, err := strconv.ParseFloat(smoothingAlphaEntry.Text, 64); err == nil {
				state.cfg.Measurement.SmoothingAlpha = sa
			}
			if sfws, err := time.ParseDuration(spikeFilterWindowSizeEntry.Text); err == nil {
				state.cfg.Measurement.SpikeFilterWindowSize = sfws
			}
			if dsr, err := time.ParseDuration(downsampleRateEntry.Text); err == nil {
				state.cfg.Measurement.DownsampleRate = &dsr
			}
			if changeFilterTypeSelect.Selected != "" {
				state.cfg.Measurement.ChangeFilterType = changeFilterTypeSelect.Selected
			}
			if cfa, err := strconv.ParseFloat(changeFilterAlphaEntry.Text, 64); err == nil {
				state.cfg.Measurement.ChangeFilterAlpha = cfa
			}
			if cfws, err := time.ParseDuration(changeFilterWindowSizeEntry.Text); err == nil {
				state.cfg.Measurement.ChangeFilterWindowSize = cfws
			}
			if err := state.cfg.Save("config.yaml"); err != nil {
				dialog.ShowError(fmt.Errorf("failed to save config: %w", err), state.window)
			}
			// Recreate power meter with new config
			state.powerMeter = meter.New(state.cfg)
			// Restart measurement chain with new settings
			if state.chain != nil {
				closeMeasurementChain(state.chain)
				state.chain = nil
				if state.device != nil && state.device.IsConnected() {
					handleConnect(state)
				}
			}
		},
	}

	return container.NewTabItem("Measurement", form)
}

// createCalibrationTab creates the Calibration configuration tab.
func createCalibrationTab(state *appState) *container.TabItem {
	baselineDurationEntry := widget.NewEntry()
	baselineDurationEntry.SetText(state.cfg.Calibration.BaselineDuration.String())

	heaterDurationEntry := widget.NewEntry()
	heaterDurationEntry.SetText(state.cfg.Calibration.HeaterDuration.String())

	cooloffDurationEntry := widget.NewEntry()
	cooloffDurationEntry.SetText(state.cfg.Calibration.CooloffDuration.String())

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Baseline Duration", Widget: baselineDurationEntry},
			{Text: "Heater Duration", Widget: heaterDurationEntry},
			{Text: "Cool-off Duration", Widget: cooloffDurationEntry},
		},
		OnSubmit: func() {
			if bd, err := time.ParseDuration(baselineDurationEntry.Text); err == nil {
				state.cfg.Calibration.BaselineDuration = bd
			}
			if hd, err := time.ParseDuration(heaterDurationEntry.Text); err == nil {
				state.cfg.Calibration.HeaterDuration = hd
			}
			if cd, err := time.ParseDuration(cooloffDurationEntry.Text); err == nil {
				state.cfg.Calibration.CooloffDuration = cd
			}
			if err := state.cfg.Save("config.yaml"); err != nil {
				dialog.ShowError(fmt.Errorf("failed to save config: %w", err), state.window)
			}
		},
	}

	// Create calibration points list
	pointsText := ""
	for i, point := range state.cfg.Calibration.Points {
		pointsText += fmt.Sprintf("%d. Heater: %.3f mW, Slope: %.6f V/s (%.3f mV/s)\n",
			i+1, point.Power, point.Slope, point.Slope*1000)
	}
	if pointsText == "" {
		pointsText = "No calibration points yet. Use 'Add Cal Point' button to add points."
	}

	pointsLabel := widget.NewLabel(pointsText)
	pointsLabel.Wrapping = fyne.TextWrapWord

	// Create calibrate button
	calibrateBtn := widget.NewButton("Calibrate (Fit Polynomial)", func() {
		handleCalibrate(state)
	})

	// Create clear points button
	clearPointsBtn := widget.NewButton("Clear All Points", func() {
		dialog.ShowConfirm("Clear Calibration Points",
			"Are you sure you want to clear all calibration points?",
			func(confirmed bool) {
				if confirmed {
					state.cfg.Calibration.Points = []config.CalibrationPoint{}
					if err := state.cfg.Save("config.yaml"); err != nil {
						dialog.ShowError(fmt.Errorf("failed to save config: %w", err), state.window)
					} else {
						dialog.ShowInformation("Success", "All calibration points cleared.", state.window)
					}
				}
			}, state.window)
	})

	// Layout
	content := container.NewVBox(
		form,
		widget.NewSeparator(),
		widget.NewLabel("Calibration Points:"),
		pointsLabel,
		container.NewHBox(calibrateBtn, clearPointsBtn),
	)

	return container.NewTabItem("Calibration", content)
}

// createMockTab creates the Mock device configuration tab.
func createMockTab(state *appState) *container.TabItem {
	biasEntry := widget.NewEntry()
	biasEntry.SetText(fmt.Sprintf("%.3f", state.cfg.Mock.Bias))

	noiseLevelEntry := widget.NewEntry()
	noiseLevelEntry.SetText(fmt.Sprintf("%.6f", state.cfg.Mock.NoiseLevel))

	laserPowerEntry := widget.NewEntry()
	laserPowerEntry.SetText(fmt.Sprintf("%.1f", state.cfg.Mock.LaserPower))

	laserDurationEntry := widget.NewEntry()
	laserDurationEntry.SetText(state.cfg.Mock.LaserDuration.String())

	laserPeriodEntry := widget.NewEntry()
	laserPeriodEntry.SetText(state.cfg.Mock.LaserPeriod.String())

	sampleRateEntry := widget.NewEntry()
	sampleRateEntry.SetText(state.cfg.Mock.SampleRate.String())

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Bias (V)", Widget: biasEntry},
			{Text: "Noise Level (V)", Widget: noiseLevelEntry},
			{Text: "Laser Power (mW)", Widget: laserPowerEntry},
			{Text: "Laser Duration", Widget: laserDurationEntry},
			{Text: "Laser Period", Widget: laserPeriodEntry},
			{Text: "Sample Rate", Widget: sampleRateEntry},
		},
		OnSubmit: func() {
			if bias, err := strconv.ParseFloat(biasEntry.Text, 64); err == nil {
				state.cfg.Mock.Bias = bias
			}
			if nl, err := strconv.ParseFloat(noiseLevelEntry.Text, 64); err == nil {
				state.cfg.Mock.NoiseLevel = nl
			}
			if lp, err := strconv.ParseFloat(laserPowerEntry.Text, 64); err == nil {
				state.cfg.Mock.LaserPower = lp
			}
			if ld, err := time.ParseDuration(laserDurationEntry.Text); err == nil {
				state.cfg.Mock.LaserDuration = ld
			}
			if lper, err := time.ParseDuration(laserPeriodEntry.Text); err == nil {
				state.cfg.Mock.LaserPeriod = lper
			}
			if sr, err := time.ParseDuration(sampleRateEntry.Text); err == nil {
				state.cfg.Mock.SampleRate = sr
			}
			if err := state.cfg.Save("config.yaml"); err != nil {
				dialog.ShowError(fmt.Errorf("failed to save config: %w", err), state.window)
			}
		},
	}

	return container.NewTabItem("Mock", form)
}
