package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/itohio/golpm/pkg/lpm"
)

// handleHeaterToggle handles heater button click to toggle heater state.
func handleHeaterToggle(state *appState, heaterIndex int) {
	if state.device == nil || !state.device.IsConnected() {
		return
	}

	// Toggle heater state
	state.heaterState[heaterIndex] = !state.heaterState[heaterIndex]

	// Send command to device
	err := state.device.SetHeaters(
		state.heaterState[0],
		state.heaterState[1],
		state.heaterState[2],
	)
	if err != nil {
		// Revert state on error
		state.heaterState[heaterIndex] = !state.heaterState[heaterIndex]
		dialog.ShowError(fmt.Errorf("failed to set heaters: %w", err), state.window)
		return
	}

	// Update button visual state (optimistic update)
	updateHeaterButtonStates(state)
}

// updateHeaterStatesFromSample updates heater button states from incoming sample.
// Only updates UI when heater state actually changes.
// Uses fyne.Do() to ensure thread-safe UI updates from goroutine.
func updateHeaterStatesFromSample(state *appState, sample lpm.RawSample) {
	// Check if state changed - arrays are directly comparable in Go
	newState := [3]bool{sample.Heater1, sample.Heater2, sample.Heater3}
	if state.heaterState == newState {
		// No change, skip update
		return
	}

	// Update state from sample
	state.heaterState[0] = sample.Heater1
	state.heaterState[1] = sample.Heater2
	state.heaterState[2] = sample.Heater3

	// Update UI on main thread using fyne.Do()
	fyne.Do(func() {
		updateHeaterButtonStates(state)
	})
}

// updateHeaterButtonStates updates the visual state of heater buttons.
func updateHeaterButtonStates(state *appState) {
	updateHeaterButton(state.heater1Btn, state.heaterState[0])
	updateHeaterButton(state.heater2Btn, state.heaterState[1])
	updateHeaterButton(state.heater3Btn, state.heaterState[2])
}

// updateHeaterButton updates a single heater button's visual state.
func updateHeaterButton(btn *widget.Button, isOn bool) {
	if isOn {
		btn.Importance = widget.HighImportance
	} else {
		btn.Importance = widget.MediumImportance
	}
	btn.Refresh()
}
