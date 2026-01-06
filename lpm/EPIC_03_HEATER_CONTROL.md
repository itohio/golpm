# EPIC 03: Heater Control Buttons

## Overview

Add individual heater control buttons to the toolbar, allowing users to manually turn heaters on/off.

## Goals

- Add three heater control buttons to toolbar
- Update button states based on current heater status
- Send heater commands to MCU when buttons clicked
- Display heater state visually
- Update button states when receiving samples with heater status

## Requirements

### UI Components

**Toolbar Enhancement:**
- Add three heater control buttons (Heater 1, Heater 2, Heater 3)
- Buttons should be toggle buttons (pressed/unpressed state)
- Visual indication of active state (different color or style)
- Buttons initially disabled until connected

**Button Behavior:**
- Click toggles heater on/off
- Send command to MCU via serial connector
- Update button visual state immediately (optimistic update)
- Receive confirmation via incoming samples (actual state)
- Handle errors if command fails (revert button state)

**State Management:**
- Track current heater states
- Update states from incoming RawSample data
- Synchronize button states with actual heater states
- Handle state mismatches (user action vs MCU state)

### Integration Points

**Connection to Serial Package:**
- Use `SetHeaters()` method from serial connector
- Handle errors from SetHeaters() appropriately
- Update UI on successful command

**Sample Reading:**
- Monitor incoming RawSample channel
- Extract heater states from samples
- Update button states accordingly
- Handle state synchronization

**Application State:**
- Store current heater states in app state
- Update state when:
  - Button clicked (user action)
  - Sample received (MCU state)
  - Connection status changes

## Implementation Steps

1. **Add heater control buttons to toolbar**
   - Create three toggle buttons
   - Style buttons appropriately (active/inactive states)
   - Position in toolbar (after main buttons or separate group)

2. **Implement button click handlers**
   - Toggle heater state
   - Call SetHeaters() on connector
   - Update button visual state
   - Handle errors

3. **Implement state synchronization**
   - Monitor RawSample channel for heater states
   - Update button states from samples
   - Handle state conflicts

4. **Handle connection lifecycle**
   - Disable buttons when disconnected
   - Enable buttons when connected
   - Reset states on disconnect

5. **Add visual feedback**
   - Different colors/styles for active/inactive
   - Tooltips showing current state
   - Disabled state when not connected

## UI/UX Considerations

**Button Design:**
- Clear visual distinction between on/off states
- Tooltips: "Heater 1: On/Off" or "Heater 1 (~2kΩ): On/Off"
- Group heaters together visually
- Consider icons vs text labels

**State Feedback:**
- Immediate visual feedback on click
- Sync with actual MCU state from samples
- Handle delays gracefully

**Error Handling:**
- Show error dialog if command fails
- Revert button state on error
- Allow retry

## Testing Considerations

- Unit tests for state management
- Test button click handlers
- Test state synchronization
- Test error handling
- Test disabled states

## Success Criteria

- ✅ Three heater buttons visible in toolbar
- ✅ Buttons toggle on/off when clicked
- ✅ Commands sent to MCU correctly
- ✅ Button states update from incoming samples
- ✅ Buttons disabled when not connected
- ✅ Visual feedback for active/inactive states
- ✅ Error handling for failed commands
- ✅ Code follows SOLID principles

## Notes

- Consider using checkboxes instead of buttons for toggle behavior
- May want to group heaters visually (separator or box)
- Tooltips should include resistance values from config
- State synchronization may have slight delay (acceptable)
- Consider adding "All Off" button for convenience
- Button states should persist during connection (remember last state)

