# Scope Widget Analysis Report

**Date:** 2026-01-26  
**Component:** `pkg/scope` - Oscilloscope-style measurement visualization widget  
**Version:** Current implementation  

---

## Executive Summary

The scope widget is a custom Fyne widget that displays oscilloscope-style graphs for laser power meter measurements. It renders dual Y-axis plots showing voltage samples (left axis, orange) and their derivatives (right axis, light blue), along with pulse detection markers, fitted lines, and power labels.

**Overall Assessment:** The widget is functionally complete with good separation of concerns, but has several rendering correctness issues, performance concerns, and usability limitations that should be addressed.

**Note:** During analysis, critical pulse tracking bugs were discovered in the `meter` package (not scope widget itself) that affect what data is sent to the scope for rendering. These have been fixed.

---

## Architecture Overview

### Component Structure

```
ScopeWidget (scope.go)
├── Data Management
│   ├── Thread-safe buffers (mutex-protected)
│   ├── Downsampling (1000 points max)
│   └── Auto-scaling (dual Y-axes)
├── Renderer (renderer.go)
│   ├── Grid drawing (dual Y-axes)
│   ├── Curve rendering (samples + derivatives)
│   ├── Pulse markers (vertical lines)
│   ├── Fitted lines (horizontal with bands)
│   └── Labels (power, heater, timestamps)
└── Tests (scope_test.go)
    └── Scaling algorithm tests
```

### Data Flow

```
Meter → UpdateData() → Downsample → Auto-scale → Refresh() → Renderer
                                                              ↓
                                                    Canvas Objects → Fyne
```

---

## Critical Issues

### 1. **CRITICAL: Incorrect Derivative Downsampling** 🔴

**Location:** `scope.go:67`, `downsample.go:184-221`

**Problem:** Derivatives are downsampled using simple decimation (picking every Nth point), while samples are downsampled using averaging. This creates a **fundamental mismatch** between the two curves.

```go
// samples.go:122-182 - Samples are AVERAGED in windows
func DownsampleSamples(dst []Sample, samples []Sample, maxPoints int) []Sample {
    // ... averages all samples in each window
    avg := Sample{
        Reading:     sumReading / n,
        Change:      sumChange / n,
        // ...
    }
}

// samples.go:184-221 - Derivatives are DECIMATED (pick every Nth)
func DownsampleDerivatives(dst []float64, derivatives []float64, maxPoints int) []float64 {
    // ... just picks one point per window
    dst = append(dst, derivatives[idx])
}
```

**Impact:**
- Derivative curve doesn't match sample curve behavior at high zoom levels
- Pulse detection visualization becomes inaccurate when downsampled
- Fitted lines may not align with displayed derivative points
- Misleading visual representation of actual data

**Fix Required:** Change `DownsampleDerivatives` to use averaging like `DownsampleSamples`:

```go
func DownsampleDerivatives(dst []float64, derivatives []float64, maxPoints int) []float64 {
    // ... calculate windows
    for i := range maxPoints {
        startIdx := int(float64(i) * step)
        endIdx := int(float64(i+1) * step)
        // Average all derivatives in window
        var sum float64
        for j := startIdx; j < endIdx; j++ {
            sum += derivatives[j]
        }
        dst = append(dst, sum/float64(endIdx-startIdx))
    }
}
```

---

### 2. **CRITICAL: Derivative Timestamp Misalignment** 🔴

**Location:** `renderer.go:133-145`

**Problem:** Derivatives are positioned at midpoints between samples, but this doesn't account for downsampling. When samples are downsampled (averaged), their timestamps represent the **last sample in the window**, not the average timestamp.

```go
// renderer.go:140
midTime := samples[i].Timestamp.Add(samples[i+1].Timestamp.Sub(samples[i].Timestamp) / 2)
```

**Impact:**
- Derivative curve appears shifted relative to sample curve
- Pulse detection markers don't align with derivative peaks
- Visual confusion when comparing samples vs derivatives

**Fix Required:** Either:
1. Store proper timestamps for derivatives during downsampling, OR
2. Use sample timestamps directly (derivatives represent change FROM sample[i] TO sample[i+1])

---

### 3. **MAJOR: Inconsistent Axis Scaling Algorithm** 🟠

**Location:** `scope.go:269-305`

**Problem:** The `determineStepFromValue()` function has inconsistent logic for different value ranges:

```go
// scope.go:288-304
if value >= 100.0 {
    return 10.0      // [100, ∞) → 10
} else if value >= 10.0 {
    return 10.0      // [10, 100) → 10
} else if value >= 1.0 {
    return 1.0       // [1, 10) → 1
} else if value >= 0.1 {
    return 0.1       // [0.1, 1) → 0.1
} else if value >= 0.01 {
    return 0.1       // [0.01, 0.1) → 0.1  ← WRONG! Should be 0.01
} else if value >= 0.001 {
    return 0.01      // [0.001, 0.01) → 0.01
}
```

**Impact:**
- Values in range [0.01, 0.1) get wrong step size (0.1 instead of 0.01)
- Example: 0.05 gets step 0.1, snaps to 0.0-0.1 instead of 0.04-0.06
- Inconsistent grid spacing for small values

**Fix Required:**

```go
} else if value >= 0.01 {
    return 0.01  // NOT 0.1
}
```

---

### 4. **MAJOR: Negative Value Snapping Bug** 🟠

**Location:** `scope.go:307-329`

**Problem:** `snapDown()` and `snapUp()` have incorrect logic for negative values:

```go
// scope.go:307-317
func snapDown(value, step float64) float64 {
    if value >= 0 {
        return float64(int64(value/step)) * step
    }
    // For negative values, floor means more negative
    return float64(int64(value/step)-1) * step  // ← BUG: double-floors
}
```

**Example:**
- `snapDown(-0.05, 0.1)` should return `-0.1`
- Actual: `int64(-0.05/0.1) = int64(-0.5) = 0`, then `0-1 = -1`, result `-0.1` ✓ (works by accident)
- But `snapDown(-1.5, 1.0)` should return `-2.0`
- Actual: `int64(-1.5) = -1`, then `-1-1 = -2`, result `-2.0` ✓ (works by accident)
- However, `snapDown(-1.0, 1.0)` should return `-1.0`
- Actual: `int64(-1.0) = -1`, then `-1-1 = -2`, result `-2.0` ✗ (WRONG!)

**Impact:**
- Incorrect axis ranges for negative values at exact multiples
- Asymmetric scaling around zero

**Fix Required:** Use proper floor/ceil functions:

```go
func snapDown(value, step float64) float64 {
    if step == 0 {
        return value
    }
    return math.Floor(value/step) * step
}

func snapUp(value, step float64) float64 {
    if step == 0 {
        return value
    }
    return math.Ceil(value/step) * step
}
```

---

### 5. **MAJOR: No Bounds Checking for Pulse Indices** 🟠

**Location:** `renderer.go:476-480`

**Problem:** Power labels use `pulse.StartIndex` and `pulse.EndIndex` directly without checking if they're valid for the current `samples` slice:

```go
// renderer.go:476-480
for i := pulse.StartIndex; i <= pulse.EndIndex && i < len(samples); i++ {
    if samples[i].Reading > maxReading {
        maxReading = samples[i].Reading
    }
}
```

**Impact:**
- If `pulse.StartIndex` is out of bounds (negative or >= len(samples)), this will panic or access wrong data
- Pulses detected in old data may have indices that don't match current downsampled buffer
- **Potential crash** when switching between different time windows

**Fix Required:** Add bounds checking:

```go
startIdx := pulse.StartIndex
endIdx := pulse.EndIndex
if startIdx < 0 || startIdx >= len(samples) || endIdx < 0 || endIdx >= len(samples) {
    continue // Skip this pulse
}
```

---

## Performance Issues

### 6. **Memory Allocations in Hot Path** 🟡

**Location:** `renderer.go:124-145`, `renderer.go:239-264`

**Problem:** Multiple allocations per frame in `Refresh()`:

```go
// renderer.go:124
samplePoints := make([]dataPoint, len(samples))  // Allocation 1

// renderer.go:134
derivativePoints := make([]dataPoint, 0, len(derivatives))  // Allocation 2

// renderer.go:239
positions := make([]fyne.Position, 0, len(points))  // Allocation 3
```

**Impact:**
- Increased GC pressure (refresh happens frequently)
- Unnecessary allocations for data that could be reused
- Performance degradation on resource-constrained systems

**Fix:** Pre-allocate buffers in `scopeRenderer` and reuse:

```go
type scopeRenderer struct {
    // ... existing fields
    samplePointsBuf     []dataPoint
    derivativePointsBuf []dataPoint
    positionsBuf        []fyne.Position
}
```

---

### 7. **Redundant Canvas Object Creation** 🟡

**Location:** `renderer.go:98-106`, `renderer.go:258-264`

**Problem:** Every `Refresh()` call creates entirely new canvas objects (lines, rectangles, text) instead of reusing existing ones:

```go
// renderer.go:98-106
r.objects = []fyne.CanvasObject{r.grid}
r.gridLines = r.gridLines[:0]
r.gridTexts = r.gridTexts[:0]
// ... creates all new objects

// renderer.go:258-264
for i := range len(positions) - 1 {
    line := canvas.NewLine(color)  // New object every frame!
    // ...
}
```

**Impact:**
- Excessive object creation/destruction
- Higher memory churn
- Slower rendering on complex graphs

**Fix:** Reuse canvas objects and update their properties:

```go
// Instead of creating new lines, reuse existing ones
if i < len(r.cachedLines) {
    line := r.cachedLines[i]
    line.Position1 = positions[i]
    line.Position2 = positions[i+1]
    canvas.Refresh(line)
} else {
    line := canvas.NewLine(color)
    r.cachedLines = append(r.cachedLines, line)
}
```

---

### 8. **No Throttling for High-Frequency Updates** 🟡

**Location:** `scope.go:62-87`

**Problem:** `UpdateData()` calls `Refresh()` on every update without throttling:

```go
// scope.go:80-86
s.mu.Unlock()
s.Refresh()
canvas.Refresh(s)
```

**Impact:**
- If meter sends updates at 50Hz (20ms intervals), widget refreshes 50 times/second
- Unnecessary CPU usage for rendering
- Fyne may not even be able to render that fast (typically 60fps max)

**Note:** The `appState` in `main.go` has throttling logic (lines 112-113), but it's not clear if it's being used effectively.

**Fix:** Add internal throttling to `UpdateData()`:

```go
const minRefreshInterval = 33 * time.Millisecond // ~30fps

func (s *ScopeWidget) UpdateData(...) {
    s.mu.Lock()
    // ... update data
    now := time.Now()
    shouldRefresh := now.Sub(s.lastRefreshTime) >= minRefreshInterval
    s.mu.Unlock()
    
    if shouldRefresh {
        s.Refresh()
        canvas.Refresh(s)
        s.lastRefreshTime = now
    }
}
```

---

## Usability Issues

### 9. **No User Interaction** 🟡

**Problem:** Widget has no zoom, pan, or cursor functionality. Users cannot:
- Zoom into specific time ranges
- Pan through historical data
- Measure values at specific points
- Select/highlight pulses

**Impact:**
- Limited analysis capability
- Cannot inspect fine details
- No way to compare specific pulses

**Recommendation:** Add interaction modes:
- Mouse wheel zoom (X and Y axes independently)
- Click-drag to pan
- Crosshair cursor with value readout
- Click pulse to highlight and show details

---

### 10. **Fixed Color Scheme** 🟡

**Problem:** Colors are hardcoded and not configurable:

```go
// renderer.go:182
grid := canvas.NewRectangle(color.RGBA{R: 20, G: 20, B: 20, A: 255})

// renderer.go:129
color.RGBA{R: 255, G: 165, B: 0, A: 255}  // Orange for samples
```

**Impact:**
- No dark/light theme support
- Cannot adapt to user preferences
- May be hard to read in different lighting conditions

**Recommendation:** Use Fyne's theme system or add color configuration.

---

### 11. **No Legend** 🟡

**Problem:** No legend explaining what each color/line represents.

**Impact:**
- New users don't know what they're looking at
- Confusion between samples (orange) vs derivatives (blue)
- No explanation of pulse markers, fitted lines, or bands

**Recommendation:** Add a legend in corner showing:
- Orange line = Voltage samples (mV)
- Light blue line = Derivatives (mV/s)
- Dark blue lines = Pulse boundaries
- Green line = Fitted slope
- Green bands = Acceptance ranges

---

### 12. **Overlapping Labels** 🟡

**Problem:** Power labels are positioned at fixed offsets without collision detection:

```go
// renderer.go:489
powerLabel.Move(fyne.NewPos(x-40, y-10))

// renderer.go:498
slopeLabel.Move(fyne.NewPos(x-40, y+8))
```

**Impact:**
- Labels overlap when pulses are close together
- Unreadable text in dense pulse regions
- No automatic repositioning

**Recommendation:** Implement label collision detection and automatic repositioning.

---

### 13. **No Error Handling for Edge Cases** 🟡

**Problem:** Several edge cases are not handled gracefully:

1. **Empty data:** Widget shows empty grid but no "No data" message
2. **Single sample:** May cause division by zero in time range calculations
3. **Zero range:** `yRange == 0` is checked in `drawCurve()` but not everywhere
4. **Invalid timestamps:** No validation that timestamps are monotonically increasing

**Impact:**
- Confusing display when no data is available
- Potential panics or incorrect rendering

**Recommendation:** Add validation and user-friendly error messages.

---

## Code Quality Issues

### 14. **Inconsistent Naming** 🟢

**Problem:** Some inconsistencies in naming:
- `sampleYMin` vs `derivativeYMin` (good)
- `xMin` vs `sampleYMin` (inconsistent - should be `timeMin`?)
- `drawCurve()` vs `drawPulses()` vs `drawFittedLines()` (inconsistent verb usage)

**Impact:** Minor - slightly harder to read and maintain.

---

### 15. **Magic Numbers** 🟢

**Problem:** Many hardcoded values without explanation:

```go
// renderer.go:109-112
marginLeft := float32(60.0)
marginRight := float32(60.0)
marginTop := float32(20.0)
marginBottom := float32(40.0)

// renderer.go:172
numHLines := 8

// renderer.go:204
numVLines := 10

// scope.go:52
maxDisplayPoints:   1000
```

**Impact:** Hard to adjust layout without understanding context.

**Recommendation:** Extract to constants with descriptive names.

---

### 16. **Custom Float Formatting** 🟢

**Problem:** Custom float formatting functions (`formatFloat`, `formatFloatRaw`, `formatInt`) instead of using `fmt.Sprintf`:

```go
// renderer.go:613-672 - 60 lines of custom formatting code
```

**Impact:**
- More code to maintain
- Potential bugs (not well-tested)
- Reinventing the wheel

**Recommendation:** Use standard library unless there's a specific performance reason:

```go
func formatVoltage(v float64) string {
    return fmt.Sprintf("%.3fV", v)
}
```

**Note:** If performance is critical, benchmark first before custom implementation.

---

### 17. **Insufficient Test Coverage** 🟢

**Problem:** Only one test file with 7 test cases for `snapToMultiples()`. No tests for:
- Downsampling algorithms
- Rendering logic
- Edge cases (empty data, single sample, etc.)
- Auto-scaling with different data patterns
- Pulse rendering

**Impact:** Harder to refactor safely, bugs may go unnoticed.

**Recommendation:** Add comprehensive tests for all major functions.

---

## Rendering Correctness Issues

### 18. **Pulse Index vs Timestamp Confusion** 🟠

**Location:** `renderer.go:329-343`, `renderer.go:476`

**Problem:** Code mixes index-based and timestamp-based pulse tracking:

```go
// renderer.go:329-339 - Uses timestamps to find indices
for i := range len(samples) - 1 {
    sampleTime := samples[i].Timestamp
    if startIdx == -1 && (sampleTime.Equal(pulse.StartTime) || sampleTime.After(pulse.StartTime)) {
        startIdx = i
    }
}

// renderer.go:476 - Uses stored indices directly
for i := pulse.StartIndex; i <= pulse.EndIndex && i < len(samples); i++ {
```

**Impact:**
- `pulse.StartIndex` and `pulse.EndIndex` are indices into the **original full sample buffer** in the meter
- After downsampling, these indices are **invalid** for the `displaySamples` buffer
- Fitted lines and power labels may be drawn at wrong positions or cause panics

**Root Cause:** Pulses are detected on full-resolution data but rendered on downsampled data. Indices don't translate between the two.

**Fix Required:** Always use timestamps for pulse rendering, never indices:

```go
// Find samples within pulse time range
for i, s := range samples {
    if s.Timestamp.After(pulse.StartTime) && s.Timestamp.Before(pulse.EndTime) {
        // Process this sample
    }
}
```

---

### 19. **Derivative Length Mismatch** 🟠

**Location:** `scope.go:70-71`, `renderer.go:133-145`

**Problem:** Comments state "n-1 derivatives for n samples" but code doesn't enforce this:

```go
// scope.go:70-71
s.samples = samples
s.derivatives = derivatives
```

No validation that `len(derivatives) == len(samples) - 1`.

**Impact:**
- If lengths don't match, derivative rendering may access out-of-bounds indices
- Midpoint calculation assumes `derivatives[i]` corresponds to `samples[i]` and `samples[i+1]`

**Fix Required:** Add validation:

```go
if len(derivatives) != len(samples)-1 {
    log.Printf("WARNING: derivative length mismatch: %d derivatives for %d samples",
        len(derivatives), len(samples))
    // Truncate or pad as needed
}
```

---

### 20. **Grid Label Precision Issues** 🟢

**Location:** `renderer.go:674-692`

**Problem:** `calculateAxisLabel()` uses linear interpolation which can produce ugly decimal values:

```go
// Example: min=10, max=20, 8 grid lines
// Labels: 20, 18.75, 17.5, 16.25, 15, 13.75, 12.5, 11.25, 10
```

**Impact:**
- Non-round numbers on axis labels (18.75, 13.75, etc.)
- Harder to read at a glance
- Doesn't match typical oscilloscope behavior (round numbers)

**Recommendation:** Snap labels to "nice" values (multiples of 1, 2, 5, 10, etc.) like professional oscilloscopes do.

---

## Documentation Issues

### 21. **Missing Package Documentation** 🟢

**Problem:** No package-level documentation explaining:
- What the scope widget does
- How to use it
- What data it expects
- Performance characteristics

**Recommendation:** Add package doc:

```go
// Package scope provides an oscilloscope-style widget for visualizing
// laser power meter measurements with dual Y-axes, pulse detection,
// and fitted line overlays.
//
// Usage:
//   scopeWidget := scope.New(cfg)
//   scopeWidget.UpdateData(samples, derivatives, pulses, heaterPower)
//
// The widget automatically downsamples to 1000 points for efficient rendering
// and uses dual Y-axes with independent auto-scaling.
package scope
```

---

### 22. **Incomplete Function Documentation** 🟢

**Problem:** Some functions lack documentation:
- `snapDown()`, `snapUp()` - no explanation of rounding behavior
- `determineStepFromValue()` - examples would help
- `drawCurve()` - doesn't explain coordinate system

**Recommendation:** Add comprehensive documentation with examples.

---

## Pulse Tracking Issues (Fixed in meter.go)

### 24. **Pulse Never Becomes Official** 🔴 **[FIXED]**

**Location:** `meter.go:336-346`

**Problem:** The pulse detection used `fitDuration` (from best fit window) to determine if a pulse should become official, but `findBestFitWindow()` continuously shrinks the fit window to reduce stdDev. This means:
- Pulse starts detecting at time T
- After 10 seconds (minPulseDuration), it should become official
- But `findBestFitWindow()` keeps removing samples from the start
- `fitDuration` keeps shrinking and never reaches `minPulseDuration`
- Pulse never becomes official and eventually gets rejected

**Example from logs:**
```
05:12:44 - Started pulse detection
05:13:12 - Pulse REJECTED (28 seconds later!)
```

**Impact:**
- Valid pulses are rejected even though they lasted > minPulseDuration
- User sees no pulses even when heating is clearly happening
- System appears broken

**Fix Applied:** Changed to use `detectDuration` (time since pulse started) instead of `fitDuration`:

```go
// OLD: if fitDuration >= m.minPulseDuration {
// NEW: if detectDuration >= m.minPulseDuration {
```

---

### 25. **Pulse Continues Tracking After Finalization** 🔴 **[FIXED]**

**Location:** `meter.go:369-409`

**Problem:** When a pulse ends (cooling phase detected), the code:
1. Adds pulse to `m.pulses` array (for rendering) ✓
2. BUT keeps it as `m.activePulse` and continues updating it ✗

This causes:
- Pulse continues being tracked even during cooling
- Logs show "Official pulse cooling" messages repeatedly
- New pulse detection is blocked because `activePulse != nil`
- Pulse is updated in the array while it should be frozen

**Example from logs:**
```
05:12:36 - Official pulse cooling: consecutive_outliers=101/104
05:12:36 - Official pulse cooling: consecutive_outliers=102/104
... (continues for many samples)
05:12:36 - Pulse ENDED: too many consecutive outliers
```

**Impact:**
- Only one pulse can be tracked at a time
- Second pulse cannot start until first pulse accumulates enough outliers
- Pulse data keeps changing even after it should be finalized
- Confusing logs

**Fix Applied:** Changed terminology and behavior:
- "ENDED" → "FINALIZED" (pulse is complete, stays in array)
- Clear `m.activePulse = nil` immediately when cooling is detected
- Pulse remains in `m.pulses` for rendering
- New pulse can start immediately after finalization

```go
// OLD: log.Printf("[PULSE] Pulse ENDED: ...")
// NEW: log.Printf("[PULSE] Pulse FINALIZED: ...")
// Added comment: "Pulse remains in m.pulses array for rendering"
```

---

## Security/Safety Issues

### 23. **No Mutex in Renderer** 🟠

**Location:** `renderer.go:67-91`

**Problem:** Renderer reads data with `RLock()` but then uses it after unlocking:

```go
// renderer.go:68-91
r.scope.mu.RLock()
samples := r.scope.displaySamples
derivatives := r.scope.displayDerivatives
// ... copy more data
r.scope.mu.RUnlock()

// Later use of samples/derivatives - data could be stale or inconsistent
```

**Impact:**
- If `UpdateData()` is called during rendering, data could be inconsistent
- Slices could be reallocated, causing the renderer to use old backing arrays
- Potential race conditions

**Fix:** Either:
1. Deep copy data while holding lock, OR
2. Hold lock for entire render (bad for performance), OR
3. Use immutable data structures

**Recommendation:** Deep copy:

```go
r.scope.mu.RLock()
samples := make([]sample.Sample, len(r.scope.displaySamples))
copy(samples, r.scope.displaySamples)
// ... copy other data
r.scope.mu.RUnlock()
```

---

## Positive Aspects ✅

Despite the issues, the widget has several good qualities:

1. **Good Separation of Concerns:** Widget logic separated from rendering logic
2. **Thread-Safe Updates:** Proper mutex usage in `UpdateData()`
3. **Dual Y-Axes:** Clever solution for displaying samples and derivatives together
4. **Downsampling:** Smart optimization to limit rendering to 1000 points
5. **Rich Visualization:** Shows samples, derivatives, pulses, fitted lines, and bands
6. **Configurable:** Uses config for thresholds and parameters
7. **Fyne Integration:** Proper implementation of Fyne widget interface
8. **Visual Feedback:** Heater power, voltage, and timing information displayed

---

## Recommendations Priority

### Immediate (Fix Before Production)

1. ✅ **Fix derivative downsampling** (Issue #1) - Use averaging, not decimation
2. ✅ **Fix derivative timestamp alignment** (Issue #2) - Proper time positioning
3. ✅ **Fix pulse index bounds checking** (Issue #5) - Prevent crashes
4. ✅ **Fix pulse index/timestamp confusion** (Issue #18) - Use timestamps only
5. ✅ **Fix pulse never becomes official** (Issue #24) - **FIXED** in meter.go
6. ✅ **Fix pulse continues tracking after finalization** (Issue #25) - **FIXED** in meter.go

### High Priority (Fix Soon)

5. ✅ **Fix axis scaling bug** (Issue #3) - Correct step size for [0.01, 0.1)
6. ✅ **Fix negative value snapping** (Issue #4) - Use proper floor/ceil
7. ✅ **Add derivative length validation** (Issue #19) - Prevent out-of-bounds
8. ✅ **Add data copying in renderer** (Issue #23) - Fix race conditions

### Medium Priority (Improve UX)

9. ✅ **Add update throttling** (Issue #8) - Limit refresh rate to ~30fps
10. ✅ **Add legend** (Issue #11) - Explain what colors mean
11. ✅ **Add "No data" message** (Issue #13) - Better empty state
12. ✅ **Fix overlapping labels** (Issue #12) - Collision detection

### Low Priority (Nice to Have)

13. ✅ **Add user interaction** (Issue #9) - Zoom, pan, cursor
14. ✅ **Add theme support** (Issue #10) - Dark/light modes
15. ✅ **Reduce memory allocations** (Issue #6) - Reuse buffers
16. ✅ **Reuse canvas objects** (Issue #7) - Less object churn
17. ✅ **Extract magic numbers** (Issue #15) - Named constants
18. ✅ **Add comprehensive tests** (Issue #17) - Better coverage
19. ✅ **Improve documentation** (Issues #21, #22) - Package and function docs
20. ✅ **Use standard formatting** (Issue #16) - Replace custom float formatting

---

## Conclusion

The scope widget is a well-structured component with good architectural decisions (dual Y-axes, downsampling, thread safety). However, it has several **critical correctness issues** that need immediate attention:

1. **Derivative downsampling mismatch** causes incorrect visualization
2. **Pulse index/timestamp confusion** causes rendering errors and potential crashes
3. **Axis scaling bugs** produce incorrect ranges for certain value ranges

Once these are fixed, the widget will be production-ready. The performance and usability improvements can be addressed incrementally based on user feedback and actual usage patterns.

**Estimated Effort:**
- Critical fixes: 4-8 hours
- High priority fixes: 4-6 hours
- Medium priority improvements: 8-16 hours
- Low priority enhancements: 16-32 hours

**Total:** ~32-62 hours for complete overhaul

---

## Appendix: Testing Recommendations

### Unit Tests Needed

```go
// scope_test.go additions
TestDownsampleSamples()           // Test averaging behavior
TestDownsampleDerivatives()       // Test averaging (after fix)
TestUpdateAutoScale()             // Test scaling with various data
TestSnapDown()                    // Test negative values
TestSnapUp()                      // Test negative values
TestDetermineStepFromValue()      // Test all ranges
TestCalculateAxisLabel()          // Test label generation

// renderer_test.go (new file)
TestDrawCurve()                   // Test coordinate mapping
TestDrawPulses()                  // Test pulse rendering
TestDrawFittedLines()             // Test fitted line rendering
TestFormatting()                  // Test all format functions
```

### Integration Tests Needed

```go
TestScopeWidgetWithRealData()     // Test with actual meter data
TestScopeWidgetEmptyData()        // Test with no data
TestScopeWidgetSingleSample()     // Test edge case
TestScopeWidgetHighFrequency()    // Test rapid updates
TestScopeWidgetConcurrency()      // Test thread safety
```

### Visual Tests Needed

1. Manual inspection with known waveforms (sine, square, triangle)
2. Comparison with reference oscilloscope screenshots
3. Verification of pulse detection visualization
4. Verification of fitted line accuracy
5. Testing with various screen sizes and DPI settings

---

**Report End**
