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
	PulseStateFitting   PulseState = iota // Fitting: initial phase, not in list yet, no power calc, can be discarded
	PulseStateUpdating                    // Updating: in list, met min duration, calculating power, actively tracked
	PulseStateFinalized                   // Finalized: complete, locked in, no longer tracking, only rendered
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

	// Best fit checkpoint (for backtracking when StdDev increases)
	bestFitStartIndex int       // Start index of best fit found so far
	bestFitEndIndex   int       // End index of best fit found so far
	bestFitStdDev     float64   // StdDev of best fit
	bestFitMean       float64   // Mean of best fit
	bestFitStartTime  time.Time // Start time of best fit
	bestFitEndTime    time.Time // End time of best fit

	// For display
	FittedLine []float64 // Fitted horizontal line values for each derivative point
	Outliers   []int     // Indices of outlier points (relative to StartIndex)

	// Configuration (passed at creation)
	minDuration         time.Duration // Minimum duration to be considered valid
	stdDevThresholdMVS  float64       // Acceptable stdDev in mV/s
	slopeThreshold      float64       // Minimum slope in V/s (enter threshold)
	hysteresisFactor    float64       // Exit threshold multiplier (e.g., 0.5 = exit at 50% of enter)
	absorbanceCoeff     float64       // Absorbance coefficient for power calculation
	powerPolynomial     []float64     // Polynomial coefficients for power calculation
	heaterPowerProvider func(int, int) float64

	// Grace period for noise tolerance (state-dependent)
	gracePeriodFitting        int // Grace period when in Fitting state
	gracePeriodUpdating       int // Grace period when in Updating state
	consecutiveBelowThreshold int // Count of consecutive samples below threshold
}

// PulseConfig contains configuration for pulse creation and fitting.
type PulseConfig struct {
	ID                  int
	MinDuration         time.Duration
	StdDevThresholdMVS  float64
	SlopeThreshold      float64
	HysteresisFactor    float64 // Exit threshold = SlopeThreshold × HysteresisFactor (default: 1.0, typical: 0.5)
	GracePeriodFitting  int     // Grace period for Fitting state (default: 30 samples = 300ms @ 100Hz)
	GracePeriodUpdating int     // Grace period for Updating state (default: 5 samples = 50ms @ 100Hz)
	GracePeriodSamples  int     // Deprecated: use GracePeriodFitting/Updating instead
	AbsorbanceCoeff     float64
	PowerPolynomial     []float64
	HeaterPowerProvider func(int, int) float64
}

// NewPulse creates a new active pulse starting at the given index.
func NewPulse(config PulseConfig, samples []sample.Sample, derivatives []float64, startIdx int) *Pulse {
	if startIdx < 0 || startIdx >= len(derivatives) || startIdx+1 >= len(samples) {
		return nil
	}

	// Default grace periods if not specified
	graceFitting := config.GracePeriodFitting
	if graceFitting <= 0 {
		// Backward compatibility: use old GracePeriodSamples if set
		if config.GracePeriodSamples > 0 {
			graceFitting = config.GracePeriodSamples * 6 // 6× longer for Fitting
		} else {
			graceFitting = 30 // Default: 300ms @ 100Hz
		}
	}

	graceUpdating := config.GracePeriodUpdating
	if graceUpdating <= 0 {
		if config.GracePeriodSamples > 0 {
			graceUpdating = config.GracePeriodSamples
		} else {
			graceUpdating = 5 // Default: 50ms @ 100Hz
		}
	}

	// Default hysteresis factor (1.0 = no hysteresis, 0.5 = exit at 50% of enter)
	hysteresis := config.HysteresisFactor
	if hysteresis <= 0 {
		hysteresis = 0.5 // Default: exit at 50% of enter threshold
	}

	p := &Pulse{
		ID:                  config.ID,
		State:               PulseStateFitting, // Start as Fitting, not in list yet
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
		hysteresisFactor:    hysteresis,
		absorbanceCoeff:     config.AbsorbanceCoeff,
		powerPolynomial:     config.PowerPolynomial,
		heaterPowerProvider: config.HeaterPowerProvider,
		gracePeriodFitting:  graceFitting,
		gracePeriodUpdating: graceUpdating,
	}

	return p
}

// IsFitting returns true if the pulse is in the initial fitting phase (not in list yet).
func (p *Pulse) IsFitting() bool {
	return p.State == PulseStateFitting
}

// IsUpdating returns true if the pulse is in the updating phase (in list, being tracked).
// Updating pulses calculate power and are considered stable.
func (p *Pulse) IsUpdating() bool {
	return p.State == PulseStateUpdating
}

// IsFinalized returns true if the pulse is complete and frozen.
func (p *Pulse) IsFinalized() bool {
	return p.State == PulseStateFinalized
}

// IsActive returns true if the pulse is still being tracked (Fitting or Updating).
func (p *Pulse) IsActive() bool {
	return p.State == PulseStateFitting || p.State == PulseStateUpdating
}

// Duration returns the current duration of the pulse (detection window).
func (p *Pulse) Duration() time.Duration {
	return p.DetectEndTime.Sub(p.DetectStartTime)
}

// FitDuration returns the duration of the fitted window.
func (p *Pulse) FitDuration() time.Duration {
	return p.EndTime.Sub(p.StartTime)
}

// HasMinDuration returns true if the pulse has met the minimum duration requirement.
func (p *Pulse) HasMinDuration() bool {
	return p.Duration() >= p.minDuration
}

// StdDevThresholdMVS returns the configured StdDev threshold in mV/s for rendering.
func (p *Pulse) StdDevThresholdMVS() float64 {
	return p.stdDevThresholdMVS
}

// Power calculates optical power from the average slope using polynomial and absorbance coefficient.
// Uses the formula: power = (c0 + c1*slope + c2*slope² + c3*slope³) / absorbanceCoeff
// If polynomial is not configured (< 4 coefficients), uses linear relationship: power = slope / absorbanceCoeff
// Returns 0 for Fitting pulses (power only calculated for Updating and Finalized states).
func (p *Pulse) Power() float64 {
	// Fitting pulses don't calculate power yet
	if p.State == PulseStateFitting {
		return 0
	}

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
// State machine:
// - Fitting: Initial phase, not in list yet, trying to establish stable fit. Can be discarded if unstable.
// - Updating: In list, met min duration, calculating power, actively tracked.
// - Finalized: Complete, locked in, no longer tracking, only rendered.
//
// Critical constraint: Pulses must only exist during POSITIVE slopes.
// Any negative slope immediately discards (Fitting) or finalizes (Updating) the pulse.
//
// Returns true if pulse should continue tracking, false if discarded/finalized.
func (p *Pulse) Update(samples []sample.Sample, derivatives []float64, currentIdx int) bool {
	// Finalized pulses are never updated
	if p.State == PulseStateFinalized {
		return false
	}

	if currentIdx < 0 || currentIdx >= len(derivatives) || currentIdx+1 >= len(samples) {
		return false
	}

	currentDeriv := derivatives[currentIdx]

	// CRITICAL: No negative slopes allowed in any pulse!
	if currentDeriv < 0 {
		if p.State == PulseStateFitting {
			// Fitting pulse with negative slope - discard immediately
			log.Printf("[PULSE #%d] Pulse DISCARDED: negative slope detected (%.3f mV/s) in Fitting state",
				p.ID, currentDeriv*1000.0)
			return false
		} else if p.State == PulseStateUpdating {
			// Updating pulse with negative slope - finalize it
			p.Finalize()
			log.Printf("[PULSE #%d] Pulse FINALIZED: negative slope detected (%.3f mV/s) - cooling phase",
				p.ID, currentDeriv*1000.0)
			return false
		}
	}

	// Check if derivative dropped below threshold (with hysteresis and state-specific grace period)
	// Fitting state: Use hysteresis (lower exit threshold) and longer grace period
	// Updating state: Use normal threshold and shorter grace period
	exitThreshold := p.slopeThreshold
	gracePeriod := p.gracePeriodUpdating

	if p.State == PulseStateFitting {
		// Fitting pulses use hysteresis - exit threshold is lower (more tolerant)
		exitThreshold = p.slopeThreshold * p.hysteresisFactor
		gracePeriod = p.gracePeriodFitting
	}

	if currentDeriv < exitThreshold {
		p.consecutiveBelowThreshold++

		// Grace period: allow N consecutive samples below exit threshold
		if p.consecutiveBelowThreshold >= gracePeriod {
			if p.State == PulseStateFitting {
				// Fitting pulse that never reached min duration - discard
				log.Printf("[PULSE #%d] Pulse DISCARDED: derivative %.3f mV/s below exit threshold %.3f mV/s (×%.2f hysteresis) for %d samples (%.2fs < %.2fs min)",
					p.ID, currentDeriv*1000.0, exitThreshold*1000.0, p.hysteresisFactor, p.consecutiveBelowThreshold,
					p.Duration().Seconds(), p.minDuration.Seconds())
				return false
			} else if p.State == PulseStateUpdating {
				// Updating pulse cooling off - finalize
				p.Finalize()
				log.Printf("[PULSE #%d] Pulse FINALIZED: derivative %.3f mV/s below threshold %.3f mV/s for %d samples",
					p.ID, currentDeriv*1000.0, exitThreshold*1000.0, p.consecutiveBelowThreshold)
				return false
			}
		}
		// Continue tracking but don't extend detection window
		return true
	}

	// Reset grace period counter when back above threshold
	p.consecutiveBelowThreshold = 0

	// Extend detection window (snake head moves forward)
	p.DetectEndIndex = currentIdx
	p.DetectEndTime = samples[currentIdx+1].Timestamp

	stdDevThresholdVS := p.stdDevThresholdMVS / 1000.0 // Convert mV/s to V/s

	// SNAKE ALGORITHM: The snake grows and optimizes incrementally
	// 1. Calculate fit for full window (tail to head)
	// 2. If stressed (StdDev too high), try moving tail forward
	// 3. Remember the best position (longest duration with acceptable StdDev)

	currentFit := p.fitHorizontalLine(derivatives, samples, p.DetectStartIndex, currentIdx)

	// Snake optimization: If stressed, try moving tail forward
	if currentFit.stdDev > stdDevThresholdVS {
		// Calculate min points needed
		minPoints := 2
		if len(samples) >= 2 {
			sampleInterval := samples[1].Timestamp.Sub(samples[0].Timestamp)
			if sampleInterval > 0 {
				minPoints = int(p.minDuration / sampleInterval)
				if minPoints < 2 {
					minPoints = 2
				}
			}
		}

		// Try moving tail forward (up to 20% of window or 200 samples)
		windowSize := currentIdx - p.DetectStartIndex + 1
		maxTailMove := windowSize / 5
		if maxTailMove > 200 {
			maxTailMove = 200
		}

		bestStart := p.DetectStartIndex
		bestFit := currentFit

		for tryStart := p.DetectStartIndex + 1; tryStart <= p.DetectStartIndex+maxTailMove; tryStart++ {
			// Must maintain min duration
			if tryStart > currentIdx-minPoints+1 {
				break
			}

			fit := p.fitHorizontalLine(derivatives, samples, tryStart, currentIdx)
			if fit.duration < p.minDuration {
				break
			}

			// If moving tail helps, keep it
			if fit.stdDev < bestFit.stdDev {
				bestFit = fit
				bestStart = tryStart

				// If snake feels good now, stop
				if bestFit.stdDev <= stdDevThresholdVS {
					break
				}
			} else {
				// Moving tail doesn't help - stop trying
				break
			}
		}

		// Update tail position if we found a better spot
		if bestStart != p.DetectStartIndex {
			p.DetectStartIndex = bestStart
			p.DetectStartTime = samples[bestStart].Timestamp
			currentFit = bestFit
			log.Printf("[PULSE #%d] Snake moved tail forward: StdDev %.3f → %.3f mV/s",
				p.ID, currentFit.stdDev*1000.0, bestFit.stdDev*1000.0)
		}

		// CRITICAL: If snake is still stressed after optimization, must take action!
		if currentFit.stdDev > stdDevThresholdVS {
			if p.State == PulseStateFitting {
				// Fitting pulse that can't find good fit - keep trying until 1/3 duration
				// (handled below in decision logic)
			} else if p.State == PulseStateUpdating {
				// Updating pulse that can't maintain threshold even after tail optimization
				// This is a phase change - finalize immediately!
				if p.bestFitStdDev > 0 {
					// Backtrack to best fit checkpoint
					bestFitResult := lineFitResult{
						mean:     p.bestFitMean,
						stdDev:   p.bestFitStdDev,
						startIdx: p.bestFitStartIndex,
						endIdx:   p.bestFitEndIndex,
						duration: p.bestFitEndTime.Sub(p.bestFitStartTime),
					}
					p.applyFit(&bestFitResult, samples)
				}
				p.Finalize()
				log.Printf("[PULSE #%d] Pulse FINALIZED: StdDev %.3f > %.3f mV/s even after tail optimization - phase change",
					p.ID, currentFit.stdDev*1000.0, stdDevThresholdVS*1000.0)
				return false
			}
		}
	}

	// Check if this is a better fit than our best checkpoint
	isBetterFit := false
	if p.bestFitStdDev == 0 {
		// First fit
		isBetterFit = true
	} else {
		// Better if: (lower StdDev) OR (same StdDev but longer duration)
		if currentFit.stdDev < p.bestFitStdDev {
			isBetterFit = true
		} else if currentFit.stdDev == p.bestFitStdDev && currentFit.duration > (p.bestFitEndTime.Sub(p.bestFitStartTime)) {
			isBetterFit = true
		}
	}

	// Update best fit checkpoint if this is better
	if isBetterFit && currentFit.stdDev <= stdDevThresholdVS {
		p.bestFitStartIndex = currentFit.startIdx
		p.bestFitEndIndex = currentFit.endIdx
		p.bestFitStdDev = currentFit.stdDev
		p.bestFitMean = currentFit.mean
		p.bestFitStartTime = samples[currentFit.startIdx].Timestamp
		p.bestFitEndTime = samples[currentFit.endIdx+1].Timestamp

		log.Printf("[PULSE #%d] New BEST fit: indices %d-%d, duration %.2fs, stdDev %.3f mV/s",
			p.ID, p.bestFitStartIndex, p.bestFitEndIndex,
			p.bestFitEndTime.Sub(p.bestFitStartTime).Seconds(),
			p.bestFitStdDev*1000.0)
	}

	// Decide what to do based on current fit quality
	if currentFit.stdDev > stdDevThresholdVS {
		// Current fit exceeds threshold - can't maintain quality

		if p.State == PulseStateFitting {
			// Still fitting
			if p.Duration() >= p.minDuration/3 && p.bestFitStdDev == 0 {
				// Been trying for 1/3 duration, no good fit found - reject
				log.Printf("[PULSE #%d] Pulse REJECTED: No good fit after 1/3 duration, StdDev %.3f > %.3f mV/s",
					p.ID, currentFit.stdDev*1000.0, p.stdDevThresholdMVS)
				return false
			}
			// Keep trying (use current fit for display even if not great)
			p.applyFit(&currentFit, samples)
		} else if p.State == PulseStateUpdating {
			// Was updating, now can't maintain threshold → phase change
			// BACKTRACK to best fit checkpoint
			if p.bestFitStdDev > 0 {
				// Apply best fit we found
				bestFitResult := lineFitResult{
					mean:     p.bestFitMean,
					stdDev:   p.bestFitStdDev,
					startIdx: p.bestFitStartIndex,
					endIdx:   p.bestFitEndIndex,
					duration: p.bestFitEndTime.Sub(p.bestFitStartTime),
				}
				p.applyFit(&bestFitResult, samples)
				log.Printf("[PULSE #%d] BACKTRACKED to best fit: duration %.2fs, stdDev %.3f mV/s",
					p.ID, p.Duration().Seconds(), p.StdDev*1000.0)
			}

			p.Finalize()
			log.Printf("[PULSE #%d] Pulse FINALIZED: StdDev %.3f > threshold %.3f mV/s - phase change detected",
				p.ID, currentFit.stdDev*1000.0, p.stdDevThresholdMVS)
			return false
		}
	} else {
		// Current fit is good (within threshold)
		p.applyFit(&currentFit, samples)

		// State transition: Fitting → Updating when min duration reached
		if p.State == PulseStateFitting && p.HasMinDuration() {
			p.State = PulseStateUpdating
			log.Printf("[PULSE #%d] Pulse transitioned to UPDATING: duration %.2fs >= %.2fs, stdDev %.3f mV/s",
				p.ID, p.Duration().Seconds(), p.minDuration.Seconds(), currentFit.stdDev*1000.0)
		}
	}

	return true // Continue tracking
}

// Finalize marks the pulse as complete and frozen (Finalized state).
// Finalized pulses optimize for minimal StdDev and are no longer tracked.
func (p *Pulse) Finalize() {
	p.State = PulseStateFinalized
	// Final optimization: find absolute best fit with minimal StdDev
	// This is done once when transitioning to Finalized state
}

// findBestFitWindowMinimalStdDev is now a simple fallback that returns the current window fit.
//
// THE SNAKE ALGORITHM: We no longer need expensive trim searches!
// - During Update(), the snake already finds the optimal position incrementally
// - The snake (pulse window) grows and feels stressed when on ramps
// - bestFitCheckpoint remembers the longest position where snake felt best
// - This function just returns current state (used only for Fitting pulses < minDuration)
//
// The expensive O(N²) trim loops are GONE - the snake does it all in O(N)!
func (p *Pulse) findBestFitWindowMinimalStdDev(samples []sample.Sample, derivatives []float64, endIdx int) *lineFitResult {
	if p.DetectStartIndex < 0 || endIdx >= len(derivatives) || p.DetectStartIndex >= endIdx {
		return nil
	}

	// Simply return the full window fit
	// The real optimization happens incrementally in Update() via bestFitCheckpoint
	fit := p.fitHorizontalLine(derivatives, samples, p.DetectStartIndex, endIdx)
	return &fit
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
