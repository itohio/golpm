package meter

import (
	"log"
	"math"
	"time"

	"github.com/itohio/golpm/pkg/sample"
)

// PulseState represents the current state of a pulse.
type PulseState int

const (
	PulseStateActive   PulseState = iota // Pulse is actively being tracked and updated
	PulseStateFinalized                  // Pulse is complete and frozen (no more updates)
)

// Pulse represents a detected heating pulse with self-contained state management.
type Pulse struct {
	// Identification
	ID int // Auto-incrementing ID for tracking and debugging

	// State
	State PulseState // Current state of the pulse

	// Detection boundaries (where derivative crossed threshold)
	DetectStartIndex int       // Index where derivative first crossed threshold
	DetectEndIndex   int       // Index where derivative dropped below threshold
	DetectStartTime  time.Time // Timestamp when derivative first crossed threshold
	DetectEndTime    time.Time // Timestamp when derivative dropped below threshold

	// Actual pulse boundaries (best fit window, may be subset of detection range)
	StartIndex int       // Start of best fit window
	EndIndex   int       // End of best fit window
	StartTime  time.Time // Start timestamp of best fit window
	EndTime    time.Time // End timestamp of best fit window

	// Fitted values
	AvgSlope       float64 // Average slope (mean derivative) in V/s
	AvgPower       float64 // Average calculated power in W
	AvgHeaterPower float64 // Average heater power in W during pulse

	// Fit quality
	RSquared        float64 // R² coefficient of determination for fit quality (0-1)
	StdDev          float64 // Actual standard deviation of derivatives in V/s
	StdDevThreshold float64 // Configured threshold for stdDev in V/s

	// For display
	FittedLine []float64 // Fitted horizontal line values for each derivative point
	Outliers   []int     // Indices of outlier points (relative to StartIndex)

	// Configuration (passed at creation)
	minDuration          time.Duration // Minimum duration to be considered valid
	stdDevThresholdMVS   float64       // Acceptable stdDev in mV/s
	slopeThreshold       float64       // Minimum slope in V/s
	absorbanceCoeff      float64       // Absorbance coefficient for power calculation
	powerPolynomial      []float64     // Polynomial coefficients for power calculation
	heaterPowerProvider  func(int, int) float64
}

// PulseConfig contains configuration for pulse creation and fitting.
type PulseConfig struct {
	ID                  int
	MinDuration         time.Duration
	StdDevThresholdMVS  float64
	SlopeThreshold      float64
	AbsorbanceCoeff     float64
	PowerPolynomial     []float64
	HeaterPowerProvider func(int, int) float64
}

// NewPulse creates a new active pulse starting at the given index.
func NewPulse(config PulseConfig, samples []sample.Sample, derivatives []float64, startIdx int) *Pulse {
	if startIdx < 0 || startIdx >= len(derivatives) || startIdx+1 >= len(samples) {
		return nil
	}

	p := &Pulse{
		ID:                  config.ID,
		State:               PulseStateActive,
		DetectStartIndex:    startIdx,
		DetectEndIndex:      startIdx,
		DetectStartTime:     samples[startIdx].Timestamp,
		DetectEndTime:       samples[startIdx+1].Timestamp,
		StartIndex:          startIdx,
		EndIndex:            startIdx,
		StartTime:           samples[startIdx].Timestamp,
		EndTime:             samples[startIdx+1].Timestamp,
		StdDevThreshold:     config.StdDevThresholdMVS / 1000.0, // Convert mV/s to V/s
		minDuration:         config.MinDuration,
		stdDevThresholdMVS:  config.StdDevThresholdMVS,
		slopeThreshold:      config.SlopeThreshold,
		absorbanceCoeff:     config.AbsorbanceCoeff,
		powerPolynomial:     config.PowerPolynomial,
		heaterPowerProvider: config.HeaterPowerProvider,
	}

	return p
}

// IsActive returns true if the pulse is still being actively tracked.
func (p *Pulse) IsActive() bool {
	return p.State == PulseStateActive
}

// IsFinalized returns true if the pulse is complete and frozen.
func (p *Pulse) IsFinalized() bool {
	return p.State == PulseStateFinalized
}

// Duration returns the current duration of the pulse (detection window).
func (p *Pulse) Duration() time.Duration {
	return p.DetectEndTime.Sub(p.DetectStartTime)
}

// FitDuration returns the duration of the fitted window.
func (p *Pulse) FitDuration() time.Duration {
	return p.EndTime.Sub(p.StartTime)
}

// IsOfficial returns true if the pulse has met the minimum duration requirement.
func (p *Pulse) IsOfficial() bool {
	return p.Duration() >= p.minDuration
}

// Power calculates optical power from the average slope using polynomial and absorbance coefficient.
// Uses the formula: power = (c0 + c1*slope + c2*slope² + c3*slope³) / absorbanceCoeff
// If polynomial is not configured (< 4 coefficients), uses linear relationship: power = slope / absorbanceCoeff
func (p *Pulse) Power() float64 {
	slope := p.AvgSlope

	if len(p.powerPolynomial) < 4 {
		// Fallback: linear relationship if polynomial not configured
		if p.absorbanceCoeff > 0 {
			return slope / p.absorbanceCoeff
		}
		return 0
	}

	// Calculate polynomial: c0 + c1*x + c2*x² + c3*x³
	c0 := p.powerPolynomial[0]
	c1 := p.powerPolynomial[1]
	c2 := p.powerPolynomial[2]
	c3 := p.powerPolynomial[3]

	power := c0 + c1*slope + c2*slope*slope + c3*slope*slope*slope

	// Apply absorbance correction
	if p.absorbanceCoeff > 0 {
		power /= p.absorbanceCoeff
	}

	return power
}

// Update extends the pulse with new data and recalculates the fit.
// Returns true if the pulse should continue tracking, false if it should be finalized.
func (p *Pulse) Update(samples []sample.Sample, derivatives []float64, currentIdx int) bool {
	if p.State != PulseStateActive {
		return false // Don't update finalized pulses
	}

	if currentIdx < 0 || currentIdx >= len(derivatives) || currentIdx+1 >= len(samples) {
		return false
	}

	// Check if derivative is still above threshold
	currentDeriv := derivatives[currentIdx]
	if currentDeriv < p.slopeThreshold {
		// Derivative dropped below threshold - finalize pulse
		p.Finalize()
		log.Printf("[PULSE #%d] Pulse FINALIZED: derivative %.3f mV/s dropped below threshold %.3f mV/s (cooling detected)",
			p.ID, currentDeriv*1000.0, p.slopeThreshold*1000.0)
		return false
	}

	// Extend detection window
	p.DetectEndIndex = currentIdx
	p.DetectEndTime = samples[currentIdx+1].Timestamp

	// Try to find best fit window
	bestFit := p.findBestFitWindow(samples, derivatives, currentIdx)

	if bestFit != nil {
		// Update pulse with best fit
		p.applyFit(bestFit, samples)
	} else {
		// No valid fit - use fallback fit over entire detection window
		fallbackFit := p.fitHorizontalLine(derivatives, samples, p.DetectStartIndex, currentIdx)
		p.applyFit(&fallbackFit, samples)
	}

	return true // Continue tracking
}

// Finalize marks the pulse as complete and frozen.
func (p *Pulse) Finalize() {
	p.State = PulseStateFinalized
}

// findBestFitWindow searches for the best fit window within the detection range.
// Returns the window with the lowest standard deviation that meets minimum duration.
func (p *Pulse) findBestFitWindow(samples []sample.Sample, derivatives []float64, endIdx int) *lineFitResult {
	if p.DetectStartIndex < 0 || endIdx >= len(derivatives) || p.DetectStartIndex >= endIdx {
		return nil
	}

	// Calculate minimum number of points needed for minimum duration
	if len(samples) < 2 {
		return nil
	}
	sampleInterval := samples[1].Timestamp.Sub(samples[0].Timestamp)
	if sampleInterval <= 0 {
		sampleInterval = 100 * time.Millisecond // Fallback
	}
	minPoints := int(p.minDuration / sampleInterval)
	if minPoints < 2 {
		minPoints = 2
	}

	var bestFit *lineFitResult
	bestStdDev := math.MaxFloat64

	// Try removing samples from the start to reduce stdDev
	for segStartIdx := p.DetectStartIndex; segStartIdx <= endIdx-minPoints+1; segStartIdx++ {
		fit := p.fitHorizontalLine(derivatives, samples, segStartIdx, endIdx)

		// Check if this segment meets minimum duration
		if fit.duration < p.minDuration {
			continue
		}

		// Check if stdDev is within acceptable range
		stdDevMVS := fit.stdDev * 1000.0
		if stdDevMVS > p.stdDevThresholdMVS {
			continue // Fit quality not good enough
		}

		// Check if this is the best fit so far (lowest stdDev)
		if fit.stdDev < bestStdDev {
			bestStdDev = fit.stdDev
			fitCopy := fit
			bestFit = &fitCopy
		}
	}

	return bestFit
}

// fitHorizontalLine fits a horizontal line to a segment of derivatives.
func (p *Pulse) fitHorizontalLine(derivatives []float64, samples []sample.Sample, startIdx, endIdx int) lineFitResult {
	if startIdx < 0 || endIdx >= len(derivatives) || startIdx > endIdx {
		return lineFitResult{rSquared: 0.0}
	}

	n := endIdx - startIdx + 1
	if n < 2 {
		return lineFitResult{rSquared: 0.0}
	}

	// Calculate mean (horizontal line level)
	sum := 0.0
	for i := startIdx; i <= endIdx; i++ {
		sum += derivatives[i]
	}
	mean := sum / float64(n)

	// Calculate variance and standard deviation
	variance := 0.0
	for i := startIdx; i <= endIdx; i++ {
		diff := derivatives[i] - mean
		variance += diff * diff
	}
	variance /= float64(n)
	stdDev := math.Sqrt(variance)

	// Calculate R² based on coefficient of variation
	rSquared := 0.0
	if mean != 0 {
		cv := variance / (mean * mean)
		rSquared = 1.0 / (1.0 + cv)
	} else if variance == 0 {
		rSquared = 1.0
	}

	// Calculate duration
	duration := time.Duration(0)
	if startIdx+1 < len(samples) && endIdx+1 < len(samples) {
		duration = samples[endIdx+1].Timestamp.Sub(samples[startIdx].Timestamp)
	}

	return lineFitResult{
		mean:     mean,
		rSquared: rSquared,
		stdDev:   stdDev,
		startIdx: startIdx,
		endIdx:   endIdx,
		duration: duration,
	}
}

// applyFit applies a fit result to the pulse.
func (p *Pulse) applyFit(fit *lineFitResult, samples []sample.Sample) {
	p.StartIndex = fit.startIdx
	p.EndIndex = fit.endIdx + 1
	p.StartTime = samples[fit.startIdx].Timestamp
	p.EndTime = samples[fit.endIdx+1].Timestamp
	p.AvgSlope = fit.mean
	p.RSquared = fit.rSquared
	p.StdDev = fit.stdDev

	// Calculate average heater power
	if p.heaterPowerProvider != nil {
		p.AvgHeaterPower = p.heaterPowerProvider(fit.startIdx, fit.endIdx)
	}

	// Calculate optical power using Pulse's own Power() method
	p.AvgPower = p.Power()

	// Create fitted line
	fitLength := fit.endIdx - fit.startIdx + 1
	p.FittedLine = make([]float64, fitLength)
	for i := range p.FittedLine {
		p.FittedLine[i] = fit.mean
	}
}

// ShouldStartNewPulse checks if we should start tracking a new pulse.
// This is a static helper function that doesn't require an existing pulse.
func ShouldStartNewPulse(derivatives []float64, idx int, threshold float64, lastPulseEndTime time.Time, currentTime time.Time) bool {
	if idx < 0 || idx >= len(derivatives) {
		return false
	}

	// Check if derivative is above threshold
	if derivatives[idx] < threshold {
		return false
	}

	// Ensure sufficient cooling period since last pulse (1 second)
	if !lastPulseEndTime.IsZero() {
		coolingDuration := currentTime.Sub(lastPulseEndTime)
		if coolingDuration < 1*time.Second {
			return false
		}
	}

	return true
}

// lineFitResult contains the result of a horizontal line fit.
type lineFitResult struct {
	mean     float64       // Mean value (horizontal line level)
	rSquared float64       // R² coefficient of determination
	stdDev   float64       // Standard deviation of the values
	startIdx int           // Start index in derivatives array
	endIdx   int           // End index in derivatives array
	duration time.Duration // Duration of the segment
}
