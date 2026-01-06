package main

import (
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
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
	pulseThresholdEntry.SetText(fmt.Sprintf("%.6f", state.cfg.Measurement.PulseThreshold))

	minPulseDurationEntry := widget.NewEntry()
	minPulseDurationEntry.SetText(fmt.Sprintf("%.1f", state.cfg.Measurement.MinPulseDuration))

	averageSamplesEntry := widget.NewEntry()
	averageSamplesEntry.SetText(fmt.Sprintf("%d", state.cfg.Measurement.AverageSamples))

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Window (seconds)", Widget: windowSecondsEntry},
			{Text: "Pulse Threshold (V/s)", Widget: pulseThresholdEntry},
			{Text: "Min Pulse Duration (s)", Widget: minPulseDurationEntry},
			{Text: "Average Samples (0=disabled)", Widget: averageSamplesEntry},
		},
		OnSubmit: func() {
			if ws, err := strconv.ParseFloat(windowSecondsEntry.Text, 64); err == nil {
				state.cfg.Measurement.WindowSeconds = ws
			}
			if pt, err := strconv.ParseFloat(pulseThresholdEntry.Text, 64); err == nil {
				state.cfg.Measurement.PulseThreshold = pt
			}
			if mpd, err := strconv.ParseFloat(minPulseDurationEntry.Text, 64); err == nil {
				state.cfg.Measurement.MinPulseDuration = mpd
			}
			if avg, err := strconv.Atoi(averageSamplesEntry.Text); err == nil {
				state.cfg.Measurement.AverageSamples = avg
			}
			if err := state.cfg.Save("config.yaml"); err != nil {
				dialog.ShowError(fmt.Errorf("failed to save config: %w", err), state.window)
			}
			// Recreate power meter with new config
			state.powerMeter = meter.New(state.cfg)
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

	return container.NewTabItem("Calibration", form)
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
