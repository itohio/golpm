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
// Also controls visibility of "Add Cal Point" button - only visible when at least one heater is on.
func updateHeaterButtonStates(state *appState) {
	updateHeaterButton(state.heater1Btn, state.heaterState[0])
	updateHeaterButton(state.heater2Btn, state.heaterState[1])
	updateHeaterButton(state.heater3Btn, state.heaterState[2])

	// Show "Add Cal Point" button only when at least one heater is on
	anyHeaterOn := state.heaterState[0] || state.heaterState[1] || state.heaterState[2]
	if anyHeaterOn {
		state.addCalPointBtn.Show()
	} else {
		state.addCalPointBtn.Hide()
	}
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

// handleHeaterIncrement increments heaters as a binary counter.
// Binary progression: 000 -> 100 -> 010 -> 110 -> 001 -> 101 -> 011 -> 111 -> 000
// This is equivalent to treating heaters as bits: [H1=bit0, H2=bit1, H3=bit2]
func handleHeaterIncrement(state *appState) {
	if state.device == nil || !state.device.IsConnected() {
		return
	}

	// Convert current state to binary number (H1=bit0, H2=bit1, H3=bit2)
	currentValue := 0
	if state.heaterState[0] {
		currentValue |= 1 // bit 0
	}
	if state.heaterState[1] {
		currentValue |= 2 // bit 1
	}
	if state.heaterState[2] {
		currentValue |= 4 // bit 2
	}

	// Increment and wrap around (0-7)
	nextValue := (currentValue + 1) % 8

	// Convert back to boolean array
	newState := [3]bool{
		(nextValue & 1) != 0, // bit 0 -> H1
		(nextValue & 2) != 0, // bit 1 -> H2
		(nextValue & 4) != 0, // bit 2 -> H3
	}

	// Send command to device
	err := state.device.SetHeaters(newState[0], newState[1], newState[2])
	if err != nil {
		dialog.ShowError(fmt.Errorf("failed to increment heaters: %w", err), state.window)
		return
	}

	// Update state (optimistic update)
	state.heaterState = newState
	updateHeaterButtonStates(state)
}

// handleHeaterOff turns off all heaters immediately.
func handleHeaterOff(state *appState) {
	if state.device == nil || !state.device.IsConnected() {
		return
	}

	// Turn off all heaters
	err := state.device.SetHeaters(false, false, false)
	if err != nil {
		dialog.ShowError(fmt.Errorf("failed to turn off heaters: %w", err), state.window)
		return
	}

	// Update state (optimistic update)
	state.heaterState = [3]bool{false, false, false}
	updateHeaterButtonStates(state)
}
