# EPIC 06: Graph Widget Implementation

## Overview

Implement efficient graph widget that displays real-time measurements, fitted slope, calibration heater power, and calculated laser power. The widget should resemble an oscilloscope display with a very slow horizontal sweep (5-10 seconds per full window width).

## Goals

- Create custom Fyne widget for graph display
- Display real-time temperature readings
- Display fitted slope line
- Display calibration heater power
- Display calculated laser power
- Efficient rendering and updates
- Auto-scaling and zoom capabilities

## Requirements

### Graph Widget Package (`pkg/scope`)

**Widget Interface:**
```go
type ScopeWidget struct {
    // Fyne widget implementation
}

// Methods:
- New(cfg *config.Config) *ScopeWidget
- UpdateData(samples []sample.Sample, pulses []meter.Pulse, heaterPower float32)
```

**Display Elements:**

1. **Measurement Curve:**
   - Line plot of reading (voltage) vs time
   - Smooth line connecting sample points
   - Real-time updates as new samples arrive

2. **Detected Pulse Ranges:**
   - Highlight/shade detected laser pulse ranges on the graph
   - Visual indication of pulse boundaries (start/end)
   - Different color or shading for each pulse
   - Pulse ranges update as new pulses are detected or old ones removed

3. **Fitted Slope Lines:**
   - Overlay line showing fitted linear model for each detected pulse
   - Different color/style from measurement curve
   - Extends over each pulse range
   - One fitted line per detected pulse

4. **Power Display Over Pulses:**
   - Floating text label over each detected pulse showing calculated power
   - Display format: "XX.XX mW"
   - Positioned at center or peak of each pulse
   - Updates when pulse is recalculated
   - Multiple power labels can be displayed simultaneously (one per pulse)

5. **Calibration Heater Power Indicator:**
   - Text label showing current heater power (when heaters are active)
   - Optional: Horizontal line at power level
   - Display format: "Heater: XX.XX mW"

5. **Axes and Labels:**
   - X-axis: Time (relative to oldest sample in window)
   - Y-axis: Voltage (differential reading)
   - Grid lines for readability (oscilloscope-style grid)
   - Axis labels and units
   - Horizontal timebase: Very slow sweep (5-10 seconds per full window width)
   - Time window scrolls/updates as new samples arrive (oscilloscope-style sweep)

**Design Principles:**
- Efficient rendering: Only redraw when data changes
- Canvas-based: Use Fyne Canvas for drawing
- Accept interfaces: Accept data via interface methods
- Single Responsibility: Only handles graph display
- Performance: Optimize for real-time updates

**Implementation Details:**
- Use Fyne's `canvas` package for drawing
- Cache rendering where possible
- Update on callback from measurement package
- Auto-scale Y-axis based on data range (with margins)
- Fixed X-axis range (time window from config, typically 5-10 seconds per full width)
- Oscilloscope-style behavior: New samples scroll in from right, old samples scroll out to left
- samples are drawn with thin orange line
- derivatives are drawn with light blue, thicker line
- detected pulses are drawn as vertical darker blue thin lines (pulse struct must allow for this either with indices or timestamps -whatever is more performant)
- pulse struct will have measurement info in them, thus must be displayed within each detected pulse range as orange text

### Power Display

**Power Calculation:**
- Power is already calculated in the measurement package (pkg/meter)
- Each Pulse struct contains the calculated Power field and range when it was detected
- Widget simply displays the power value from each pulse
- No calculation needed in the widget (separation of concerns)

**Display Strategy:**
- Each detected pulse has its power displayed as floating text
- Text positioned at the center of the pulse range (or at peak)
- Multiple power labels displayed simultaneously (one per pulse)
- Labels update when pulses are recalculated or removed

### Integration

**Update Flow:**
```
Measurement Package → Callback → Graph Widget → Canvas Refresh
```

**Data Flow:**
1. Measurement package processes samples
2. Differentiates samples and detects pulses
3. Calculates slope and power for each pulse
4. Invokes callback when:
   - New sample arrives
   - New pulse detected
   - Existing pulse updated
   - Pulse removed
5. Widget fetches latest data via PowerMeter interface
6. Widget updates display (samples, pulses, heater power)
7. Canvas refreshes

**Widget Registration:**
- Widget registers callback with measurement package
- Callback fetches current state (samples, pulses, heater power)
- Widget updates display with:
  - All samples in buffer
  - All detected pulses (with ranges and power labels)
  - Heater power indicator (if active)

### UI Layout

**Main Window Layout:**
- Toolbar at top (from Epic 01)
- Graph widget as main content (centered, expandable)
- Optional: Status bar at bottom (connection status, sample rate)

**Graph Widget Sizing:**
- Fill available space
- Maintain aspect ratio (optional)
- Minimum size constraints

## Implementation Steps

1. **Create graph widget package**
   - Define GraphWidget struct (embed widget.BaseWidget)
   - Implement Fyne widget interface (CreateRenderer, MinSize, etc.)
   - Create custom renderer

2. **Implement rendering**
   - Draw axes and grid
   - Draw measurement curve
   - Draw pulse range highlights/shading
   - Draw fitted slope lines for each pulse
   - Draw power labels over each pulse
   - Draw heater power indicator
   - Handle coordinate transformations

3. **Implement update mechanism**
   - Register callback with measurement package
   - Fetch data on callback (samples, pulses, heater power)
   - Update widget state
   - Trigger canvas refresh

4. **Implement pulse visualization**
   - Draw shaded/highlighted regions for each pulse
   - Position power labels at center or peak of each pulse
   - Handle multiple overlapping or adjacent pulses
   - Update pulse displays when pulses are added/removed/updated

5. **Add auto-scaling**
   - Calculate Y-axis range from data
   - Add margins for readability
   - Update range as data changes

6. **Integration**
   - Add widget to main window
   - Wire up callbacks
   - Test with real data

## Dependencies

- `fyne.io/fyne/v2` - GUI framework
- `fyne.io/fyne/v2/widget` - Base widget
- `fyne.io/fyne/v2/canvas` - Canvas drawing
- `github.com/chewxy/math32` - Float32 math
- `pkg/meter` package (for PowerMeter interface and Pulse struct)
- `pkg/sample` package (for Sample type)
- Configuration package (for calibration data, if needed for display)

## Rendering Considerations

**Performance:**
- Only redraw changed regions (if possible)
- Limit number of points drawn (downsample for display)
- Cache expensive calculations
- Use efficient line drawing algorithms

**Visual Design:**
- Color scheme: Measurement (blue), Slope (red), Grid (gray)
- Line widths: Measurement (2px), Slope (1px dashed)
- Font sizes: Readable but not intrusive
- Legend: Optional, or use labels

**Coordinate Transformations:**
- Screen coordinates ↔ Data coordinates
- Handle different scales (time vs voltage)
- Account for margins and padding

## Testing Considerations

- Unit tests for coordinate transformations
- Test pulse range rendering
- Test multiple pulse display
- Test power label positioning
- Test with various data ranges
- Test update performance
- Test rendering correctness
- Visual testing with real data (including multiple pulses)

## Success Criteria

- ✅ Graph displays measurement curve
- ✅ Detected pulse ranges are highlighted/shaded
- ✅ Fitted slope lines overlay correctly for each pulse
- ✅ Power labels displayed over each detected pulse
- ✅ Multiple pulses can be displayed simultaneously
- ✅ Heater power displayed (when active)
- ✅ Efficient real-time updates
- ✅ Auto-scaling works correctly
- ✅ Axes and labels clear
- ✅ Uses math32 for calculations
- ✅ Code follows SOLID principles

## Notes

- Fyne widgets require implementing specific interfaces
- Consider using Fyne's `container` package for layout
- Canvas refresh should be throttled if updates are too frequent
- Downsampling: Show every Nth point for very large datasets
- **Oscilloscope-style display**: Very slow horizontal sweep (5-10 seconds per full window)
- Graph should scroll horizontally as new data arrives (oscilloscope sweep behavior)
- Color scheme should be oscilloscope-like (dark background, bright traces)
- **Pulse visualization**: Use semi-transparent shading or colored regions to highlight pulse ranges
- **Power labels**: Position at center of pulse, avoid overlapping with other labels
- Multiple pulses may overlap or be adjacent - handle visual clarity
- Consider adding zoom/pan capabilities (future enhancement)
- Color choices should be accessible (colorblind-friendly)
- Text labels should not overlap with graph data or other labels
- Consider adding cursor crosshair for precise reading
- Pulse ranges update dynamically as new pulses are detected or old ones removed

