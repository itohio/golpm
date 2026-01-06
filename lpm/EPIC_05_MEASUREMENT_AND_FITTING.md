# EPIC 05: Measurement Package with Thermal Model Fitting

## Overview

Implement measurement package that maintains a FIFO buffer of samples, performs differentiation-based pulse detection, and provides mechanism for widget updates. The package must handle multiple laser pulses within the measurement window and be robust to noise.

This epic is split into three phases:
- **Phase 1**: Basic measurement with differentiation and pulse detection (no power calculations)
- **Phase 2**: Real-time linear fitting and power calculations
- **Phase 3**: Automated calibration system

## Goals

- Create measurement package that consumes samples
- Maintain FIFO buffer of samples within measurement window
- Implement differentiation-based pulse detection
- Detect multiple laser pulses within the window
- Provide callback mechanism for widget updates
- Calculate fitted slope and power for each detected pulse (Phase 2)
- Automated calibration system (Phase 3)

---

## Phase 1: Basic Measurement and Pulse Detection

### Overview

Implement core measurement functionality: sample collection, differentiation, and pulse detection without power calculations. This phase focuses on getting the data pipeline working and allowing GUI visualization for experimentation.

### Goals

- Collect samples in FIFO buffer
- Differentiate samples and store in ring buffer
- Detect pulses based on threshold (from config)
- Expose raw samples, differentiated samples, and pulses for GUI display
- No power calculations yet (deferred to Phase 2)

### Requirements

**Pulse Structure (Phase 1):**
```go
type Pulse struct {
    StartIndex int       // Start sample index in buffer
    EndIndex   int       // End sample index in buffer
    StartTime  time.Time // Start timestamp
    EndTime    time.Time // End timestamp (updated as pulse continues)
    RawValue   float32   // Raw derivative value (for debugging/display)
    // Power and Slope fields added in Phase 2
}
```

**PowerMeter Interface (Phase 1):**
```go
type PowerMeter interface {
    ProcessSamples(input <-chan sample.Sample)
    Samples() []sample.Sample      // Get current raw samples buffer
    Derivatives() []float32        // Get differentiated samples (ring buffer)
    Pulses() []Pulse                // Get detected pulses within window
    OnUpdate(func())                // Register callback for updates
}
```

**Key Components:**

1. **FIFO Buffer for Raw Samples:**
   - Maintain buffer of samples within measurement window (from config)
   - Remove old samples when window exceeded
   - Efficient insertion/removal (circular buffer or slice)
   - Maintain time-ordered sequence
   - Thread-safe access with `sync.RWMutex`

2. **Ring Buffer for Differentiated Samples:**
   - Calculate derivative: `dT/dt = (T[i+1] - T[i]) / (t[i+1] - t[i])`
   - Store differentiated values in ring buffer (same size as sample buffer minus 1)
   - Handle edge cases (first sample, buffer too small)
   - Update on every new sample

3. **Pulse Detection:**
   - Detect sections where derivative is significantly above threshold (heating)
   - Threshold configurable via config file
   - Group consecutive positive derivatives into heating pulses
   - Maintain list of active and completed pulses
   - Remove pulses that fall outside the time window
   - Update pulse end time as new samples arrive (if still active)

4. **Update Mechanism:**
   - Callback function registration
   - Invoke callbacks when:
     - New sample arrives
     - New pulse detected
     - Existing pulse updated (end time extended)
     - Pulse removed (fell outside window)
   - Thread-safe callback invocation

**Configuration (Phase 1):**
```yaml
measurement:
  window_seconds: 10    # Measurement window size in seconds
  pulse_threshold: 0.001 # Threshold for pulse detection (V/s)
```

**Design Principles:**
- Single Responsibility: Only handles measurement and pulse detection
- No widget dependencies: Uses callbacks for updates
- Explicit dependencies: Configuration, callback mechanism
- Thread-safe: Handle concurrent access to buffers and callbacks
- Data exposure: Raw samples, derivatives, and pulses all accessible for GUI

**Implementation Details:**
- Use `sync.RWMutex` for thread-safe buffer access
- Buffer size: Based on measurement window and sample rate
- Differentiation: Calculate on every new sample
- Pulse detection: Update on every new sample

### Implementation Steps

1. **Create `pkg/meter` package**
   - Define PowerMeter interface and Pulse struct (Phase 1 version)
   - Implement FIFO buffer for raw samples with time window
   - Implement ring buffer for differentiated samples
   - Implement sample processing (goroutine)
   - Add thread-safe buffer access

2. **Implement differentiation**
   - Calculate derivatives for consecutive sample pairs
   - Store in ring buffer
   - Handle edge cases (first sample, buffer too small)
   - Update ring buffer on each new sample

3. **Implement pulse detection**
   - Read threshold from config
   - Detect consecutive positive derivatives above threshold (heating sections)
   - Group sections into pulses
   - Maintain list of active pulses
   - Update pulse end time as samples arrive
   - Remove pulses outside time window

4. **Implement update mechanism**
   - Callback registration
   - Thread-safe callback invocation
   - Invoke on new sample, new pulse, pulse update, pulse removal

5. **Expose data for GUI**
   - Implement `Samples()` method
   - Implement `Derivatives()` method
   - Implement `Pulses()` method
   - All methods thread-safe

6. **Integration**
   - Wire into application
   - Connect to converter output channel
   - Register callbacks from widget
   - Test with GUI visualization

### Success Criteria (Phase 1)

- ✅ Maintains FIFO buffer of raw samples within time window
- ✅ Differentiates sample series correctly
- ✅ Stores differentiated values in ring buffer
- ✅ Detects multiple pulses within window based on threshold
- ✅ Maintains list of detected pulses
- ✅ Exposes raw samples, derivatives, and pulses for GUI
- ✅ Provides callback mechanism for updates
- ✅ Thread-safe buffer access
- ✅ Handles edge cases gracefully
- ✅ Uses math32 for calculations
- ✅ Code follows SOLID principles

---

## Phase 2: Real-time Linear Fitting and Power Calculation

### Overview

Add real-time linear fitting to detected pulses and calculate power. Fitting happens as new samples arrive until the pulse returns below threshold.

### Goals

- Perform linear fitting on detected pulses in real-time
- Calculate slope as pulse develops
- Calculate integrated power from differentiated samples
- Add duration and power fields to Pulse struct
- Update pulses as new samples arrive until they return below threshold

### Requirements

**Pulse Structure (Phase 2 - Extended):**
```go
type Pulse struct {
    StartIndex   int       // Start sample index in buffer
    EndIndex     int       // End sample index in buffer (updated in real-time)
    StartTime    time.Time // Start timestamp
    EndTime      time.Time // End timestamp (updated as pulse continues)
    RawValue     float32   // Raw derivative value
    Slope        float32   // Fitted slope (V/s) - calculated in real-time
    Power        float32   // Calculated absorbed power (mW) - from calibration
    IntegratedPower float32 // Integrated power from derivatives (mW·s)
    Duration     float32   // Pulse duration in seconds
    IsActive     bool      // True if pulse is still above threshold
}
```

**PowerMeter Interface (Phase 2 - Extended):**
```go
type PowerMeter interface {
    // ... Phase 1 methods ...
    
    // New in Phase 2:
    SetCalibration(calibration []CalibrationPoint) // Set calibration data
}
```

**Key Components:**

1. **Real-time Linear Fitting:**
   - For each active pulse, fit linear model to samples from start to current end
   - Update fitting as new samples arrive (if pulse still active)
   - Stop fitting when pulse returns below threshold
   - Use robust fitting (least squares with outlier filtering)
   - Calculate slope in V/s
   - Handle edge cases (too few samples, constant values)

2. **Integrated Power Calculation:**
   - Integrate differentiated samples over pulse range
   - Sum derivatives: `IntegratedPower = Σ(derivative[i] * dt[i])`
   - Convert to power units (requires calibration or conversion factor)
   - Provides alternative power measurement method

3. **Power Calculation from Slope:**
   - Use calibration data to convert slope to power
   - Interpolate between calibration points
   - Calculate power for each detected pulse
   - Store power with pulse information

4. **Pulse Lifecycle:**
   - Pulse starts when derivative exceeds threshold
   - Pulse continues as long as derivative stays above threshold
   - Fitting updates in real-time as pulse develops
   - Pulse ends when derivative returns below threshold
   - Final fitting performed on complete pulse
   - Pulse marked as inactive but kept in list until outside window

**Calibration Data Structure:**
```yaml
calibration:
  points:
    - slope: 0.0      # V/s
      power: 0.0      # mW
    - slope: 0.001
      power: 10.0
    # ... more points
```

**Implementation Details:**
- Fitting happens on every new sample for active pulses
- Use robust regression with outlier filtering
- Power calculation uses calibration interpolation
- Integrated power provides alternative measurement
- Duration calculated from start to end time

### Implementation Steps

1. **Extend Pulse struct**
   - Add Slope, Power, IntegratedPower, Duration, IsActive fields
   - Update pulse creation and management

2. **Implement real-time linear fitting**
   - Extract samples for each active pulse range
   - Filter outliers before fitting
   - Implement least squares regression
   - Update fitting as new samples arrive
   - Stop fitting when pulse ends

3. **Implement integrated power calculation**
   - Integrate derivatives over pulse range
   - Calculate integrated power value
   - Store in pulse struct

4. **Implement power calculation from slope**
   - Load calibration data from config
   - Implement interpolation function
   - Convert slope to power for each pulse
   - Store power with pulse information

5. **Update pulse lifecycle**
   - Mark pulses as active/inactive
   - Continue fitting while active
   - Finalize fitting when pulse ends
   - Keep completed pulses until outside window

6. **Add calibration interface**
   - Method to set calibration data
   - Use calibration for power conversion

### Success Criteria (Phase 2)

- ✅ Performs linear fitting on detected pulses in real-time
- ✅ Updates fitting as new samples arrive (for active pulses)
- ✅ Calculates slope correctly for each pulse
- ✅ Calculates integrated power from derivatives
- ✅ Calculates power from slope using calibration
- ✅ Handles pulse lifecycle (active → inactive)
- ✅ Stops fitting when pulse returns below threshold
- ✅ Robust to noise and outliers
- ✅ Uses math32 for calculations
- ✅ Code follows SOLID principles

---

## Phase 3: Automated Calibration System

### Overview

Implement automated calibration system that uses the PowerMeter, serial device, and configuration to perform calibration measurements and calculate calibration coefficients.

### Goals

- Create calibration package that orchestrates calibration process
- Measure baseline for configurable duration
- Measure with each heater in sequence
- Calculate calibration coefficients
- Save calibration data to config

### Requirements

**Calibration Package (`pkg/calibration`):**

**Calibration Struct:**
```go
type Calibrator struct {
    meter   PowerMeter      // PowerMeter instance
    device  *lpm.Device      // Serial device for heater control
    config  *config.Config  // Configuration
}

type CalibrationPoint struct {
    Slope float32  // Measured slope (V/s)
    Power float32  // Known power (mW) - from heater
}
```

**Calibration Interface:**
```go
type Calibrator interface {
    Run() ([]CalibrationPoint, error)  // Run full calibration sequence
    MeasureBaseline(duration time.Duration) (float32, error)  // Measure baseline
    MeasureHeater(heaterIndex int, duration time.Duration) (CalibrationPoint, error)  // Measure with heater
    CalculateCoefficients(points []CalibrationPoint) ([]float32, error)  // Calculate calibration coefficients
    SaveToConfig(points []CalibrationPoint) error  // Save to config file
}
```

**Calibration Process:**

1. **Baseline Measurement:**
   - Measure for 10 seconds (configurable)
   - Calculate average baseline value
   - Store baseline for offset correction

2. **Heater Sequence:**
   - For each heater (in order of increasing power):
     - Turn on heater
     - Wait for stabilization (optional)
     - Measure for 2 seconds (configurable)
     - Turn off heater
     - Wait 20 seconds (configurable) for cooloff
   - Repeat for all heaters

3. **Cooloff Measurement:**
   - After each heater measurement, measure cooloff slope
   - Can be used for validation or additional calibration points

4. **Coefficient Calculation:**
   - Fit calibration curve (linear or polynomial)
   - Calculate coefficients for slope → power conversion
   - Store coefficients in config

**Configuration (Phase 3):**
```yaml
calibration:
  baseline_duration: 10s      # Baseline measurement duration
  heater_duration: 2s         # Heater measurement duration
  cooloff_duration: 20s       # Cooloff wait duration between heaters
  heater_sequence: [1, 2, 3]  # Heater order (by index)
  
  # Calibration points (populated after calibration):
  points:
    - slope: 0.0
      power: 0.0
    - slope: 0.001
      power: 10.0
    # ... more points
```

**Design Principles:**
- Single Responsibility: Only handles calibration orchestration
- Uses PowerMeter for measurements
- Uses serial device for heater control
- Uses config for parameters and storage
- Can be run manually or automatically

**Implementation Details:**
- Calibration runs as separate process/goroutine
- Can be triggered from GUI (calibration button)
- Shows progress/status during calibration
- Handles errors gracefully
- Validates calibration results

### Implementation Steps

1. **Create `pkg/calibration` package**
   - Define Calibrator struct
   - Define CalibrationPoint struct
   - Implement calibration interface

2. **Implement baseline measurement**
   - Measure for configured duration
   - Calculate average baseline
   - Return baseline value

3. **Implement heater sequence**
   - Iterate through heaters in configured order
   - Turn on heater via serial device
   - Measure for configured duration
   - Turn off heater
   - Wait for cooloff
   - Collect calibration point (slope, known power)

4. **Implement cooloff measurement**
   - Measure cooloff slope after each heater
   - Optional: use for validation

5. **Implement coefficient calculation**
   - Fit calibration curve to points
   - Calculate coefficients (linear or polynomial)
   - Return coefficients

6. **Implement config saving**
   - Save calibration points to config
   - Save coefficients to config
   - Update config file

7. **Add GUI integration**
   - Add calibration button to toolbar
   - Show calibration progress
   - Display calibration results
   - Allow saving calibration data

### Success Criteria (Phase 3)

- ✅ Measures baseline for configured duration
- ✅ Measures with each heater in sequence
- ✅ Waits for cooloff between heaters
- ✅ Calculates calibration coefficients
- ✅ Saves calibration data to config
- ✅ Handles errors gracefully
- ✅ Provides progress feedback
- ✅ Validates calibration results
- ✅ Code follows SOLID principles

---

## Dependencies

- `github.com/chewxy/math32` - Float32 math operations
- `sync` - Thread synchronization
- `time` - Time handling
- `pkg/sample` package (for Sample type)
- `pkg/lpm` package (for serial device - Phase 3)
- Configuration package (for measurement window, thresholds, calibration data)

## Formulas

**Differentiation:**
```
dT/dt[i] = (T[i+1] - T[i]) / (t[i+1] - t[i])
```

Where:
- T = temperature reading (voltage)
- t = timestamp
- i = sample index

**Least Squares Linear Regression:**
```
slope = (n*Σ(xy) - Σ(x)*Σ(y)) / (n*Σ(x²) - (Σ(x))²)
intercept = (Σ(y) - slope*Σ(x)) / n
```

Where:
- x = time (seconds since pulse start)
- y = reading (voltage)
- n = number of samples in pulse range

**Integrated Power:**
```
IntegratedPower = Σ(derivative[i] * dt[i])
```

Where:
- derivative[i] = differentiated value at index i
- dt[i] = time difference between samples

## Testing Considerations

**Phase 1:**
- Unit tests for differentiation
- Unit tests for pulse detection
- Test FIFO buffer and ring buffer
- Test with synthetic data
- Visual testing with GUI

**Phase 2:**
- Unit tests for linear fitting
- Unit tests for power calculation
- Test with known calibration data
- Test real-time fitting updates

**Phase 3:**
- Integration tests for calibration process
- Test heater control sequence
- Test calibration coefficient calculation
- Test config saving

## Notes

**Phase 1 Focus:**
- Get basic measurement pipeline working
- Allow GUI visualization for experimentation
- Test differentiation approach with real data
- Adjust thresholds and parameters based on observations

**Phase 2 Focus:**
- Add power calculations after Phase 1 is validated
- Real-time fitting provides immediate feedback
- Integrated power provides alternative measurement method

**Phase 3 Focus:**
- Automated calibration reduces manual work
- Configurable durations allow fine-tuning
- Can be run multiple times to refine calibration

**Differentiation Approach Benefits:**
- Simpler detection of heating/cooling phases
- Natural handling of multiple pulses
- Easier to identify pulse boundaries
- More robust to baseline drift

**Noise Handling:**
- Measurements are inherently noisy
- Consider averaging converter in `pkg/sample` package
- May need smoothing before differentiation
- Outlier filtering essential for robust fitting

**Multiple Pulse Handling:**
- Pulses can occur anywhere in the buffer
- Example: 10s window, 2s pulse, 1s delay, 2s pulse → 2 heating sections, 1 cooling section
- Each pulse gets its own slope and power calculation
- Pulses are removed when they fall outside the time window

**UX Integration (see EPIC_06):**
- Each detected pulse will be displayed on the scope with its range highlighted
- Power value will float over each pulse on the graph
- Pulse ranges will be visually distinct (e.g., shaded region or colored line)
- Multiple pulses can be displayed simultaneously
- Differentiated samples can be displayed as separate trace (optional)

**Performance Considerations:**
- Differentiation is O(n) per new sample
- Pulse detection is O(n) per new sample
- Fitting is O(m) per pulse where m is pulse length
- Consider batching updates if sample rate is very high
