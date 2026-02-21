package scope

import (
	"image/color"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"github.com/itohio/golpm/pkg/config"
	"github.com/itohio/golpm/pkg/meter"
	"github.com/itohio/golpm/pkg/sample"
)

// ScopeWidget is a custom Fyne widget that displays oscilloscope-style measurement graphs.
type ScopeWidget struct {
	widget.BaseWidget

	cfg *config.Config

	// Data (protected by mu)
	mu          sync.RWMutex
	samples     []sample.Sample
	derivatives []float64
	pulses      []meter.Pulse
	activePulse *meter.Pulse // Currently tracked pulse (Fitting or Updating), drawn in gray
	heaterPower float64

	// Display buffers (reused for downsampling)
	displaySamples     []sample.Sample
	displayDerivatives []float64

	// Auto-scaling - separate Y-axes for samples and derivatives
	sampleYMin, sampleYMax         float64 // Y-axis range for samples (left axis)
	derivativeYMin, derivativeYMax float64 // Y-axis range for derivatives (right axis)
	xMin, xMax                     time.Time

	// Display settings
	maxDisplayPoints int
}

// New creates a new ScopeWidget instance.
func New(cfg *config.Config) *ScopeWidget {
	s := &ScopeWidget{
		cfg:                cfg,
		samples:            make([]sample.Sample, 0),
		derivatives:        make([]float64, 0),
		pulses:             make([]meter.Pulse, 0),
		heaterPower:        0.0,
		displaySamples:     make([]sample.Sample, 0, 1000),
		displayDerivatives: make([]float64, 0, 1000),
		maxDisplayPoints:   1000, // Limit points for efficient rendering
	}
	s.ExtendBaseWidget(s)
	// Trigger initial refresh to display empty scope
	s.Refresh()
	return s
}

// UpdateData updates the widget with new measurement data.
// This should be called from the measurement callback using fyne.Do().
func (s *ScopeWidget) UpdateData(samples []sample.Sample, derivatives []float64, pulses []meter.Pulse, activePulse *meter.Pulse, heaterPower float64) {
	s.mu.Lock()

	// Downsample for display (reuse buffers)
	s.displaySamples = sample.DownsampleSamples(s.displaySamples, samples, s.maxDisplayPoints)
	s.displayDerivatives = sample.DownsampleDerivatives(s.displayDerivatives, derivatives, s.maxDisplayPoints)

	// Store full data
	s.samples = samples
	s.derivatives = derivatives
	s.pulses = pulses
	s.activePulse = activePulse // May be nil if no active tracking
	s.heaterPower = heaterPower

	// Calculate auto-scaling
	s.updateAutoScale()

	s.mu.Unlock()

	// Refresh the widget (must be outside lock to avoid potential deadlock)
	// This triggers the renderer's Refresh() method which rebuilds all canvas objects
	s.Refresh()

	// Explicitly refresh as a canvas object to ensure Fyne invalidates and repaints
	// This is critical - without this, Fyne may only repaint on user interaction
	canvas.Refresh(s)
}

// updateAutoScale calculates Y-axis ranges from current data.
// Samples and derivatives have separate Y-axes with independent scaling.
func (s *ScopeWidget) updateAutoScale() {
	if len(s.displaySamples) == 0 {
		s.sampleYMin = 0.0
		s.sampleYMax = 1.0
		s.derivativeYMin = -0.001 // Minimum range: -1 mV/s
		s.derivativeYMax = 0.001  // Minimum range: 1 mV/s
		s.xMin = time.Now()
		s.xMax = time.Now().Add(10 * time.Second)
		return
	}

	// Calculate ranges using the same unified method for both samples and derivatives
	// IMPORTANT: Convert to mV before scaling, then convert back to V
	// This ensures proper snapping (e.g., 457-501 mV → 450-510 mV, not 0.4-0.6 V)
	sampleValues := extractSampleValues(s.displaySamples)
	if len(sampleValues) > 0 {
		// Convert to mV for scaling
		sampleValuesMV := make([]float64, len(sampleValues))
		for i, v := range sampleValues {
			sampleValuesMV[i] = v * 1000.0 // V to mV
		}

		actualSampleMin := sampleValuesMV[0]
		actualSampleMax := sampleValuesMV[0]
		for _, v := range sampleValuesMV {
			if v < actualSampleMin {
				actualSampleMin = v
			}
			if v > actualSampleMax {
				actualSampleMax = v
			}
		}

		// Calculate range in mV, then convert back to V
		minMV, maxMV := calculateRangeFromValues(sampleValuesMV)
		s.sampleYMin = minMV / 1000.0 // mV to V
		s.sampleYMax = maxMV / 1000.0 // mV to V
	}

	if len(s.displayDerivatives) > 0 {
		// Convert to mV/s for scaling
		derivativeValuesMV := make([]float64, len(s.displayDerivatives))
		for i, v := range s.displayDerivatives {
			derivativeValuesMV[i] = v * 1000.0 // V/s to mV/s
		}

		actualDerivMin := derivativeValuesMV[0]
		actualDerivMax := derivativeValuesMV[0]
		for _, v := range derivativeValuesMV {
			if v < actualDerivMin {
				actualDerivMin = v
			}
			if v > actualDerivMax {
				actualDerivMax = v
			}
		}

		// Calculate range in mV/s, then convert back to V/s
		minMV, maxMV := calculateRangeFromValues(derivativeValuesMV)

		// Enforce minimum range of ±1 mV/s (i.e., ±0.001 V/s)
		// This prevents over-zooming on very small derivative values
		const minRangeMV = 1.0 // Minimum range in mV/s
		if maxMV < minRangeMV {
			maxMV = minRangeMV
		}
		if minMV > -minRangeMV {
			minMV = -minRangeMV
		}

		s.derivativeYMin = minMV / 1000.0 // mV/s to V/s
		s.derivativeYMax = maxMV / 1000.0 // mV/s to V/s
	} else {
		// No derivatives yet, use default range: ±1 mV/s (±0.001 V/s)
		s.derivativeYMin = -0.001
		s.derivativeYMax = 0.001
	}

	// Time range
	if len(s.displaySamples) > 0 {
		s.xMin = s.displaySamples[0].Timestamp
		s.xMax = s.displaySamples[len(s.displaySamples)-1].Timestamp
		// Ensure minimum window
		if s.xMax.Sub(s.xMin) < time.Duration(s.cfg.Measurement.WindowSeconds)*time.Second {
			s.xMax = s.xMin.Add(time.Duration(s.cfg.Measurement.WindowSeconds) * time.Second)
		}
	}
}

// CreateRenderer creates the widget renderer.
func (s *ScopeWidget) CreateRenderer() fyne.WidgetRenderer {
	grid := canvas.NewRectangle(color.RGBA{R: 20, G: 20, B: 20, A: 255}) // Dark background
	return &scopeRenderer{
		scope:    s,
		grid:     grid,
		objects:  []fyne.CanvasObject{grid},
		lastSize: fyne.Size{Width: 0, Height: 0},
	}
}

// extractSampleValues extracts values from samples for range calculation.
func extractSampleValues(samples []sample.Sample) []float64 {
	values := make([]float64, len(samples))
	for i, s := range samples {
		values[i] = s.Reading
	}
	return values
}

// calculateRangeFromValues calculates min/max from a slice of values and rounds to nice range.
// Uses the SAME method for all value types (samples, derivatives, etc.).
// Scaling method:
// 1. Calculate curve min and max
// 2. Snap min to lowest multiple of 10 (or 1)
// 3. Snap max to highest multiple of 10 (or 1)
// 4. Axis labels = min/max and anything in between depending on number of grid lines
func calculateRangeFromValues(values []float64) (min, max float64) {
	if len(values) == 0 {
		return 0, 1
	}

	// Find actual min/max from data
	min = values[0]
	max = values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	// Snap to multiples of 10 (or 1) using the SAME method for all curves
	return snapToMultiples(min, max)
}

// snapToMultiples snaps min to lowest multiple and max to highest multiple.
// Determines appropriate step size (10, 1, 0.1, 0.01, etc.) based on value magnitude.
// The step is determined by the maximum absolute value (not the range).
func snapToMultiples(min, max float64) (snappedMin, snappedMax float64) {
	// Handle edge case: identical values
	if min == max {
		step := determineStepFromValue(min)
		snappedMin = snapDown(min, step)
		snappedMax = snapUp(max, step)
		// Ensure minimum range
		if snappedMax == snappedMin {
			snappedMax = snappedMin + step
		}
		return snappedMin, snappedMax
	}

	// Determine step size based on the maximum absolute value
	absMin := min
	if absMin < 0 {
		absMin = -absMin
	}
	absMax := max
	if absMax < 0 {
		absMax = -absMax
	}
	maxAbsValue := absMax
	if absMin > absMax {
		maxAbsValue = absMin
	}
	step := determineStepFromValue(maxAbsValue)

	// Snap min down to lowest multiple of step
	snappedMin = snapDown(min, step)

	// Snap max up to highest multiple of step
	snappedMax = snapUp(max, step)

	return snappedMin, snappedMax
}

// determineStepFromValue determines the appropriate step size (10, 1, 0.1, 0.01, etc.)
// based on the magnitude of the value.
// Rule: For a value in range [10^n, 10^(n+1)), use step 10^n (same order of magnitude).
// Examples: 458 (in [100, 1000)) -> 10, 45.8 (in [10, 100)) -> 10, 4.58 (in [1, 10)) -> 1, 0.05 (in [0.01, 0.1)) -> 0.1
func determineStepFromValue(value float64) float64 {
	if value < 0 {
		value = -value
	}
	if value == 0 {
		return 1.0
	}

	// Determine which decade the value falls into and return appropriate step
	// [100, ∞) -> step 10
	// [10, 100) -> step 10
	// [1, 10) -> step 1
	// [0.1, 1) -> step 0.1
	// [0.01, 0.1) -> step 0.1
	// [0.001, 0.01) -> step 0.01
	// etc.
	if value >= 100.0 {
		return 10.0
	} else if value >= 10.0 {
		return 10.0
	} else if value >= 1.0 {
		return 1.0
	} else if value >= 0.1 {
		return 0.1
	} else if value >= 0.01 {
		return 0.1
	} else if value >= 0.001 {
		return 0.01
	} else if value >= 0.0001 {
		return 0.001
	} else {
		return 0.0001
	}
}

// snapDown rounds a value down to the nearest multiple of step.
func snapDown(value, step float64) float64 {
	if step == 0 {
		return value
	}
	if value >= 0 {
		return float64(int64(value/step)) * step
	}
	// For negative values, floor means more negative
	return float64(int64(value/step)-1) * step
}

// snapUp rounds a value up to the nearest multiple of step.
func snapUp(value, step float64) float64 {
	if step == 0 {
		return value
	}
	if value >= 0 {
		return float64(int64(value/step)+1) * step
	}
	// For negative values, ceil means less negative
	return float64(int64(value/step)) * step
}
