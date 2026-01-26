package meter

import (
	"log"
	"sync"
	"time"

	"github.com/itohio/golpm/pkg/config"
	"github.com/itohio/golpm/pkg/sample"
)

var _ PowerMeter = (*Meter)(nil)

// PowerMeter processes samples, maintains buffers, and detects pulses.
type PowerMeter interface {
	ProcessSamples(input <-chan sample.Sample)
	Samples() []sample.Sample                                                      // Get current raw samples buffer (FIFO, ordered first to last)
	Derivatives() []float64                                                        // Get differentiated samples (corresponds to Samples, n-1 derivatives for n samples)
	Pulses() []Pulse                                                               // Get detected pulses within window
	OnUpdate(func(samples []sample.Sample, derivatives []float64, pulses []Pulse)) // Register callback for updates
}

// Meter implements PowerMeter interface.
// Internally uses FIFO buffers (can be implemented as ring buffers for efficiency).
// Externally exposes ordered slices (first sample/derivative first, latest last).
type Meter struct {
	cfg *config.Config

	// Buffers
	// Both samples and derivatives are FIFO buffers that maintain order:
	// - First sample/derivative is at index 0 (oldest)
	// - Latest sample/derivative is at the end (newest)
	// Internally can be implemented as ring buffers for efficiency, but externally
	// appear as ordered slices for oscillogram drawing (first to last).
	// Removal is based on timestamp (time window), not number of samples.
	//
	// Derivatives come directly from the Change field of samples (already filtered by the sample processing chain):
	// - derivative[i] = sample[i+1].Change
	// - If we have n samples, we have n-1 derivatives
	// - derivative[0] corresponds to the change from sample[0] to sample[1] (stored in sample[1].Change)
	// - derivative[1] corresponds to the change from sample[1] to sample[2] (stored in sample[2].Change)
	// - etc.
	samples     []sample.Sample // FIFO buffer of processed samples (ordered first to last, removed by timestamp)
	derivatives []float64       // FIFO buffer of derivatives from Change field (n-1 derivatives for n samples)
	pulses      []Pulse         // Detected and finalized pulses
	activePulse *Pulse          // Currently active pulse being built (nil if no active pulse)

	// Pulse detection state
	nextPulseID      int       // Auto-incrementing ID for next pulse
	lastPulseEndTime time.Time // Time when last pulse ended (for cooling phase tracking)

	// Thread safety
	mu sync.RWMutex

	// Update callbacks
	// Callbacks receive current samples, derivatives, and pulses directly
	callbacks []func(samples []sample.Sample, derivatives []float64, pulses []Pulse)
	cbMu      sync.RWMutex

	// Configuration
	windowDuration        time.Duration
	threshold             float64 // in V/s (converted from mV/s)
	minPulseDuration      time.Duration
	lineFitMinDuration    time.Duration
	lineFitRangeMVS       float64 // acceptable range in mV/s
	absorbanceCoefficient float64
	powerPolynomial       []float64

	// Shutdown control
	shutdown bool // Set to true when input channel closes, prevents further callbacks
}

// New creates a new PowerMeter instance.
// Returns concrete type (*Meter) following Go best practices.
func New(cfg *config.Config) *Meter {
	minPulseDuration := time.Duration(cfg.Measurement.MinPulseDuration * float64(time.Second))
	lineFitMinDuration := time.Duration(cfg.Measurement.PulseLineFitMinDuration * float64(time.Second))

	// Use MinPulseDuration for line fitting if PulseLineFitMinDuration is not set or is larger
	if lineFitMinDuration == 0 || lineFitMinDuration > minPulseDuration {
		lineFitMinDuration = minPulseDuration
	}

	m := &Meter{
		cfg:                   cfg,
		samples:               make([]sample.Sample, 0),
		derivatives:           make([]float64, 0),
		pulses:                make([]Pulse, 0),
		callbacks:             make([]func(samples []sample.Sample, derivatives []float64, pulses []Pulse), 0),
		windowDuration:        time.Duration(cfg.Measurement.WindowSeconds * float64(time.Second)),
		threshold:             cfg.Measurement.PulseThresholdMVS / 1000.0, // Convert mV/s to V/s
		minPulseDuration:      minPulseDuration,
		lineFitMinDuration:    lineFitMinDuration,
		lineFitRangeMVS:       cfg.Measurement.PulseLineFitRangeMVS,
		absorbanceCoefficient: cfg.Measurement.AbsorbanceCoefficient,
		powerPolynomial:       cfg.Measurement.PowerPolynomial,
		shutdown:              false,
	}

	return m
}

// ProcessSamples processes samples from the input channel in a goroutine.
// When the input channel closes, it sets shutdown flag to prevent further callbacks.
func (m *Meter) ProcessSamples(input <-chan sample.Sample) {
	for s := range input {
		m.processSample(s)
	}
	// Channel closed - mark as shutdown to prevent further callbacks
	m.mu.Lock()
	m.shutdown = true
	m.mu.Unlock()
}

// processSample adds a sample to the buffer, updates derivatives, and detects pulses.
func (m *Meter) processSample(s sample.Sample) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Add sample to FIFO buffer
	m.samples = append(m.samples, s)

	// Remove samples outside time window (based on timestamp, not count)
	// Calculate cutoff time: samples before this time are outside the window
	cutoffTime := s.Timestamp.Add(-m.windowDuration)
	cutoffIndex := 0
	for i, sample := range m.samples {
		if sample.Timestamp.After(cutoffTime) {
			cutoffIndex = i
			break
		}
	}
	if cutoffIndex > 0 {
		// Remove samples before cutoffIndex (they're outside the time window)
		m.samples = m.samples[cutoffIndex:]

		// Remove corresponding derivatives to keep exact correspondence
		// derivative[i] = sample[i+1].Change
		// If we remove samples[0..cutoffIndex-1], we need to remove derivatives[0..cutoffIndex-1]
		// because those derivatives correspond to pairs involving removed samples
		if cutoffIndex <= len(m.derivatives) {
			m.derivatives = m.derivatives[cutoffIndex:]
		} else {
			// Edge case: if we removed more samples than we have derivatives, clear all
			// This can happen if we had very few samples and removed most/all of them
			m.derivatives = m.derivatives[:0]
		}
		// Adjust pulse indices
		for i := range m.pulses {
			m.pulses[i].StartIndex -= cutoffIndex
			m.pulses[i].EndIndex -= cutoffIndex
		}
		// Remove pulses with invalid indices
		validPulses := make([]Pulse, 0)
		for _, pulse := range m.pulses {
			if pulse.StartIndex >= 0 && pulse.EndIndex >= 0 {
				validPulses = append(validPulses, pulse)
			}
		}
		m.pulses = validPulses

		// Adjust active pulse indices
		if m.activePulse != nil {
			m.activePulse.StartIndex -= cutoffIndex
			m.activePulse.EndIndex -= cutoffIndex

			// If active pulse is now invalid, clear it
			if m.activePulse.StartIndex < 0 {
				m.activePulse = nil
			}
		}
	}

	// Update derivatives using Change field from samples
	// derivative[i] corresponds exactly to the change from sample[i] to sample[i+1]
	// This is stored in sample[i+1].Change (already filtered by the sample processing chain)
	if len(m.samples) >= 2 {
		lastIdx := len(m.samples) - 1
		curr := m.samples[lastIdx] // sample[i+1]

		// Use Change field from current sample (already filtered by the sample processing chain)
		derivative := curr.Change

		// Store derivative
		m.derivatives = append(m.derivatives, derivative)

		// Ensure exact correspondence: n samples = n-1 derivatives
		if len(m.derivatives) > len(m.samples)-1 {
			m.derivatives = m.derivatives[1:]
		}
	}

	// Detect and update pulses
	m.updatePulses()

	// Check shutdown flag and prepare for callback (must do this while holding lock)
	shouldNotify := !m.shutdown

	// Release lock before calling notifyCallbacks (which needs RLock)
	// This prevents deadlock: we can't acquire RLock while holding Lock
	m.mu.Unlock()

	if shouldNotify {
		m.notifyCallbacks()
	}

	// Re-acquire lock for defer (though we're about to return anyway)
	m.mu.Lock()
}

// updatePulses detects and updates pulses using modular Pulse struct.
// Simplified logic:
// 1. If no active pulse and derivative > threshold → start new pulse
// 2. If active pulse → let it update itself with new data
// 3. If pulse finalizes → add to pulses array and clear activePulse
// 4. Remove pulses outside time window (FIFO)
func (m *Meter) updatePulses() {
	if len(m.derivatives) == 0 || len(m.samples) < 2 {
		return
	}

	lastDerivIdx := len(m.derivatives) - 1
	currentTime := m.samples[lastDerivIdx+1].Timestamp

	// Decision point 1: Should we start a new pulse?
	if m.activePulse == nil {
		if ShouldStartNewPulse(m.derivatives, lastDerivIdx, m.threshold, m.lastPulseEndTime, currentTime) {
			// Start new pulse
			m.nextPulseID++
			config := PulseConfig{
				ID:                  m.nextPulseID,
				MinDuration:         m.minPulseDuration,
				StdDevThresholdMVS:  m.lineFitRangeMVS,
				SlopeThreshold:      m.threshold,
				AbsorbanceCoeff:     m.absorbanceCoefficient,
				PowerPolynomial:     m.powerPolynomial,
				HeaterPowerProvider: m.calculateAvgHeaterPower,
			}
			m.activePulse = NewPulse(config, m.samples, m.derivatives, lastDerivIdx)

			if m.activePulse != nil {
				log.Printf("[PULSE #%d] Started pulse detection at index %d (deriv=%.3f mV/s, threshold=%.3f mV/s)",
					m.activePulse.ID, lastDerivIdx, m.derivatives[lastDerivIdx]*1000.0, m.threshold*1000.0)
			}
		}
		return
	}

	// Decision point 2: Update active pulse with new data
	if m.activePulse.IsActive() {
		shouldContinue := m.activePulse.Update(m.samples, m.derivatives, lastDerivIdx)

		// Check if pulse became official (met minimum duration)
		if m.activePulse.IsOfficial() && !m.pulseInArray(m.activePulse.ID) {
			// Add to pulses array
			m.pulses = append(m.pulses, *m.activePulse)
			log.Printf("[PULSE #%d] Pulse became OFFICIAL: duration=%.3fs >= min=%.3fs, slope=%.3f mV/s, stdDev=%.3f mV/s, power=%.6f W, heater=%.6f W",
				m.activePulse.ID, m.activePulse.Duration().Seconds(), m.minPulseDuration.Seconds(),
				m.activePulse.AvgSlope*1000.0, m.activePulse.StdDev*1000.0,
				m.activePulse.AvgPower, m.activePulse.AvgHeaterPower)
		} else if m.pulseInArray(m.activePulse.ID) {
			// Update existing pulse in array
			for i := range m.pulses {
				if m.pulses[i].ID == m.activePulse.ID {
					m.pulses[i] = *m.activePulse
					break
				}
			}
		}

		// If pulse should not continue, finalize it
		if !shouldContinue {
			if m.activePulse.IsOfficial() {
				m.lastPulseEndTime = currentTime
			} else {
				// Pulse was rejected (too short)
				log.Printf("[PULSE #%d] Pulse REJECTED: duration %.3fs < min %.3fs",
					m.activePulse.ID, m.activePulse.Duration().Seconds(), m.minPulseDuration.Seconds())
				// Remove from array if it was added
				m.removePulseFromArray(m.activePulse.ID)
			}
			m.activePulse = nil
		}
	}

	// Remove pulses outside the time window (FIFO)
	m.removeOldPulses()
}

// pulseInArray checks if a pulse with the given ID is in the pulses array.
func (m *Meter) pulseInArray(id int) bool {
	for _, p := range m.pulses {
		if p.ID == id {
			return true
		}
	}
	return false
}

// removePulseFromArray removes a pulse with the given ID from the pulses array.
func (m *Meter) removePulseFromArray(id int) {
	validPulses := make([]Pulse, 0, len(m.pulses))
	for _, p := range m.pulses {
		if p.ID != id {
			validPulses = append(validPulses, p)
		}
	}
	m.pulses = validPulses
}

// removeOldPulses removes pulses that are completely outside the time window.
func (m *Meter) removeOldPulses() {
	if len(m.samples) == 0 {
		return
	}

	oldestSampleTime := m.samples[0].Timestamp
	validPulses := make([]Pulse, 0, len(m.pulses))
	for _, pulse := range m.pulses {
		// Keep pulse if its end time is after the oldest sample
		if pulse.EndTime.After(oldestSampleTime) || pulse.EndTime.Equal(oldestSampleTime) {
			validPulses = append(validPulses, pulse)
		} else {
			log.Printf("[PULSE #%d] Removed from buffer: ended at %v, oldest sample at %v",
				pulse.ID, pulse.EndTime.Format("15:04:05.000"), oldestSampleTime.Format("15:04:05.000"))
		}
	}
	m.pulses = validPulses
}

// Samples returns a copy of the current samples buffer.
func (m *Meter) Samples() []sample.Sample {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]sample.Sample, len(m.samples))
	copy(result, m.samples)
	return result
}

// Derivatives returns a copy of the current derivatives buffer.
func (m *Meter) Derivatives() []float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]float64, len(m.derivatives))
	copy(result, m.derivatives)
	return result
}

// Pulses returns a copy of the current pulses list.
func (m *Meter) Pulses() []Pulse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Pulse, len(m.pulses))
	copy(result, m.pulses)
	return result
}

// OnUpdate registers a callback function that will be called when samples are updated.
// The callback receives current samples, derivatives, and pulses directly.
// The callback should copy data quickly and return as fast as possible.
func (m *Meter) OnUpdate(callback func(samples []sample.Sample, derivatives []float64, pulses []Pulse)) {
	m.cbMu.Lock()
	defer m.cbMu.Unlock()
	m.callbacks = append(m.callbacks, callback)
}

// ResetShutdown resets the shutdown flag, allowing callbacks to be sent again.
// This should be called before starting a new measurement chain.
func (m *Meter) ResetShutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdown = false
}

// notifyCallbacks invokes all registered callbacks with current data.
// Makes copies of data while holding read lock, then calls callbacks without lock.
func (m *Meter) notifyCallbacks() {
	// Copy data while holding read lock
	m.mu.RLock()
	samplesCopy := make([]sample.Sample, len(m.samples))
	copy(samplesCopy, m.samples)
	derivativesCopy := make([]float64, len(m.derivatives))
	copy(derivativesCopy, m.derivatives)
	pulsesCopy := make([]Pulse, len(m.pulses))
	copy(pulsesCopy, m.pulses)
	m.mu.RUnlock()

	// Get callbacks (need read lock for callbacks slice)
	m.cbMu.RLock()
	callbacks := make([]func(samples []sample.Sample, derivatives []float64, pulses []Pulse), len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.cbMu.RUnlock()

	// Invoke callbacks without holding any locks
	for _, cb := range callbacks {
		if cb != nil {
			cb(samplesCopy, derivativesCopy, pulsesCopy)
		}
	}
}

// lineFitResult contains the result of a horizontal line fit.
// calculateAvgHeaterPower calculates the average heater power during a pulse segment.
func (m *Meter) calculateAvgHeaterPower(startIdx, endIdx int) float64 {
	if startIdx < 0 || endIdx >= len(m.samples) || startIdx > endIdx {
		return 0.0
	}

	sum := 0.0
	count := 0
	for i := startIdx; i <= endIdx && i < len(m.samples); i++ {
		sum += m.samples[i].HeaterPower
		count++
	}

	if count == 0 {
		return 0.0
	}
	return sum / float64(count)
}

// calculatePower calculates power from slope using polynomial and absorbance coefficient.
// Power = (c0 + c1*slope + c2*slope² + c3*slope³) / absorbanceCoefficient
// The absorbance coefficient corrects for reflection losses (<1 means some light is reflected).
func (m *Meter) calculatePower(slope float64) float64 {
	if len(m.powerPolynomial) < 4 {
		// Fallback: linear relationship if polynomial not configured
		return slope / m.absorbanceCoefficient
	}

	// Calculate polynomial: c0 + c1*x + c2*x² + c3*x³
	c0 := m.powerPolynomial[0]
	c1 := m.powerPolynomial[1]
	c2 := m.powerPolynomial[2]
	c3 := m.powerPolynomial[3]

	power := c0 + c1*slope + c2*slope*slope + c3*slope*slope*slope

	// Apply absorbance correction
	if m.absorbanceCoefficient > 0 {
		power /= m.absorbanceCoefficient
	}

	return power
}

// UpdateCalibration updates the power calculation polynomial and absorbance coefficient.
// This allows runtime calibration updates without restarting the meter.
func (m *Meter) UpdateCalibration(polynomial []float64, absorbanceCoefficient float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(polynomial) >= 4 {
		m.powerPolynomial = polynomial[:4]
	}
	m.absorbanceCoefficient = absorbanceCoefficient

	// Update power configuration for all existing pulses and recalculate
	for i := range m.pulses {
		m.pulses[i].absorbanceCoeff = absorbanceCoefficient
		if len(polynomial) >= 4 {
			m.pulses[i].powerPolynomial = polynomial[:4]
		}
		m.pulses[i].AvgPower = m.pulses[i].Power()
	}

	// Update power configuration for active pulse if it exists
	if m.activePulse != nil {
		m.activePulse.absorbanceCoeff = absorbanceCoefficient
		if len(polynomial) >= 4 {
			m.activePulse.powerPolynomial = polynomial[:4]
		}
		m.activePulse.AvgPower = m.activePulse.Power()

		// Update in pulses array if it's already official
		if m.activePulse.IsOfficial() {
			for i := range m.pulses {
				if m.pulses[i].ID == m.activePulse.ID {
					m.pulses[i] = *m.activePulse
					break
				}
			}
		}
	}
}
