package meter

import (
	"sync"
	"time"

	"github.com/itohio/golpm/pkg/config"
	"github.com/itohio/golpm/pkg/sample"
)

var _ PowerMeter = (*Meter)(nil)

// Pulse represents a detected heating pulse (Phase 1).
type Pulse struct {
	StartIndex int       // Start sample index in buffer
	EndIndex   int       // End sample index in buffer (updated as pulse continues)
	StartTime  time.Time // Start timestamp
	EndTime    time.Time // End timestamp (updated as pulse continues)
	RawValue   float64   // Raw derivative value (for debugging/display)
	Power      float64   // Calculated power in mW (0 in Phase 1, calculated in Phase 2)
	// Slope field added in Phase 2
}

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
	// Derivatives correspond exactly to sample pairs:
	// - derivative[i] = (sample[i+1] - sample[i]) / dt
	// - If we have n samples, we have n-1 derivatives
	// - derivative[0] corresponds to the change from sample[0] to sample[1]
	// - derivative[1] corresponds to the change from sample[1] to sample[2]
	// - etc.
	samples     []sample.Sample // FIFO buffer of raw samples (ordered first to last, removed by timestamp)
	derivatives []float64       // FIFO buffer of differentiated samples (n-1 derivatives for n samples, exactly corresponds to sample pairs)
	pulses      []Pulse         // Detected pulses

	// Thread safety
	mu sync.RWMutex

	// Update callbacks
	// Callbacks receive current samples, derivatives, and pulses directly
	callbacks []func(samples []sample.Sample, derivatives []float64, pulses []Pulse)
	cbMu      sync.RWMutex

	// Configuration
	windowDuration   time.Duration
	threshold        float64
	minPulseDuration time.Duration

	// Shutdown control
	shutdown bool // Set to true when input channel closes, prevents further callbacks
}

// New creates a new PowerMeter instance.
// Returns concrete type (*Meter) following Go best practices.
func New(cfg *config.Config) *Meter {
	m := &Meter{
		cfg:              cfg,
		samples:          make([]sample.Sample, 0),
		derivatives:      make([]float64, 0),
		pulses:           make([]Pulse, 0),
		callbacks:        make([]func(samples []sample.Sample, derivatives []float64, pulses []Pulse), 0),
		windowDuration:   time.Duration(cfg.Measurement.WindowSeconds * float64(time.Second)),
		threshold:        cfg.Measurement.PulseThreshold,
		minPulseDuration: time.Duration(cfg.Measurement.MinPulseDuration * float64(time.Second)),
		shutdown:         false,
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
		// derivative[i] = (sample[i+1] - sample[i]) / dt
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
	}

	// Update derivatives (need at least 2 samples)
	// Calculate derivative for the new sample pair: (sample[n-1], sample[n])
	// derivative[i] corresponds exactly to the change from sample[i] to sample[i+1]
	if len(m.samples) >= 2 {
		lastIdx := len(m.samples) - 1
		prev := m.samples[lastIdx-1] // sample[i]
		curr := m.samples[lastIdx]   // sample[i+1]

		dt := curr.Timestamp.Sub(prev.Timestamp).Seconds()
		if dt > 0 {
			// Calculate derivative: (sample[i+1] - sample[i]) / dt
			derivative := (curr.Reading - prev.Reading) / dt
			m.derivatives = append(m.derivatives, derivative)

			// Ensure exact correspondence: n samples = n-1 derivatives
			// If somehow we have more derivatives than expected, remove oldest
			if len(m.derivatives) > len(m.samples)-1 {
				m.derivatives = m.derivatives[1:]
			}
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

// updatePulses detects and updates pulses based on derivatives.
func (m *Meter) updatePulses() {
	if len(m.derivatives) == 0 {
		return
	}

	lastDerivIdx := len(m.derivatives) - 1
	lastDeriv := m.derivatives[lastDerivIdx]
	lastSampleIdx := len(m.samples) - 1

	// Check if we're in a heating phase (derivative above threshold)
	isHeating := lastDeriv > m.threshold

	// Update existing active pulses or create new ones
	if isHeating {
		// Find active pulse (last pulse that might still be active)
		activePulseIdx := -1
		for i := len(m.pulses) - 1; i >= 0; i-- {
			if m.pulses[i].EndIndex == lastSampleIdx-1 {
				// This pulse was just extended, check if it's still active
				activePulseIdx = i
				break
			}
		}

		if activePulseIdx >= 0 {
			// Extend existing pulse
			m.pulses[activePulseIdx].EndIndex = lastSampleIdx
			m.pulses[activePulseIdx].EndTime = m.samples[lastSampleIdx].Timestamp
			m.pulses[activePulseIdx].RawValue = lastDeriv
		} else {
			// Check if we should start a new pulse
			// Only start if previous derivative was below threshold (or this is first)
			shouldStart := true
			if lastDerivIdx > 0 {
				prevDeriv := m.derivatives[lastDerivIdx-1]
				if prevDeriv > m.threshold {
					// Previous was also above threshold, might be continuation
					// Check if there's a gap (cooling phase) between last pulse and now
					shouldStart = false
					if len(m.pulses) > 0 {
						lastPulse := m.pulses[len(m.pulses)-1]
						// If there's a gap, start new pulse
						if lastSampleIdx-1 > lastPulse.EndIndex+1 {
							shouldStart = true
						}
					}
				}
			}

			if shouldStart {
				// Start new pulse
				startIdx := lastSampleIdx - 1
				if startIdx < 0 {
					startIdx = 0
				}
				newPulse := Pulse{
					StartIndex: startIdx,
					EndIndex:   lastSampleIdx,
					StartTime:  m.samples[startIdx].Timestamp,
					EndTime:    m.samples[lastSampleIdx].Timestamp,
					RawValue:   lastDeriv,
					Power:      0.0, // Will be calculated in Phase 2
				}
				m.pulses = append(m.pulses, newPulse)
			} else if len(m.pulses) > 0 {
				// Extend the last pulse if it was close
				lastPulseIdx := len(m.pulses) - 1
				lastPulse := &m.pulses[lastPulseIdx]
				if lastSampleIdx <= lastPulse.EndIndex+2 {
					// Close enough, extend it
					lastPulse.EndIndex = lastSampleIdx
					lastPulse.EndTime = m.samples[lastSampleIdx].Timestamp
					lastPulse.RawValue = lastDeriv
				}
			}
		}
	}

	// Remove pulses that are completely outside the window or too short (noise filtering)
	validPulses := make([]Pulse, 0, len(m.pulses))
	for _, pulse := range m.pulses {
		if pulse.StartIndex >= 0 && pulse.StartIndex < len(m.samples) {
			// Filter out pulses shorter than minimum duration
			duration := pulse.EndTime.Sub(pulse.StartTime)
			if duration >= m.minPulseDuration {
				validPulses = append(validPulses, pulse)
			}
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
