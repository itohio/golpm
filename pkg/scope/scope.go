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
	heaterPower float64

	// Display buffers (reused for downsampling)
	displaySamples     []sample.Sample
	displayDerivatives []float64

	// Auto-scaling
	yMin, yMax float64
	xMin, xMax time.Time

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
func (s *ScopeWidget) UpdateData(samples []sample.Sample, derivatives []float64, pulses []meter.Pulse, heaterPower float64) {
	s.mu.Lock()

	// Downsample for display (reuse buffers)
	s.displaySamples = sample.DownsampleSamples(s.displaySamples, samples, s.maxDisplayPoints)
	s.displayDerivatives = sample.DownsampleDerivatives(s.displayDerivatives, derivatives, s.maxDisplayPoints)

	// Store full data
	s.samples = samples
	s.derivatives = derivatives
	s.pulses = pulses
	s.heaterPower = heaterPower

	// Calculate auto-scaling
	s.updateAutoScale()

	s.mu.Unlock()

	// Refresh the widget (must be outside lock to avoid potential deadlock)
	s.Refresh()
}

// updateAutoScale calculates Y-axis range from current data.
func (s *ScopeWidget) updateAutoScale() {
	if len(s.displaySamples) == 0 {
		s.yMin = 0.0
		s.yMax = 1.0
		s.xMin = time.Now()
		s.xMax = time.Now().Add(10 * time.Second)
		return
	}

	// Find min/max for samples
	s.yMin = s.displaySamples[0].Reading
	s.yMax = s.displaySamples[0].Reading
	for _, sample := range s.displaySamples {
		if sample.Reading < s.yMin {
			s.yMin = sample.Reading
		}
		if sample.Reading > s.yMax {
			s.yMax = sample.Reading
		}
	}

	// Find min/max for derivatives
	for _, deriv := range s.displayDerivatives {
		if deriv < s.yMin {
			s.yMin = deriv
		}
		if deriv > s.yMax {
			s.yMax = deriv
		}
	}

	// Add 10% margin
	range_ := s.yMax - s.yMin
	if range_ == 0 {
		range_ = 1.0
	}
	margin := range_ * 0.1
	s.yMin -= margin
	s.yMax += margin

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
