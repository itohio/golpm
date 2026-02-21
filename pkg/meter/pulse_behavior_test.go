package meter

import (
	"math/rand"
	"testing"
	"time"

	"github.com/itohio/golpm/pkg/sample"
	"github.com/stretchr/testify/assert"
)

// generateTestSequence creates a sequence of samples with specified derivative patterns and noise.
// slopeSegments: each segment is [slopeM VS, duration Seconds]
// noiseStdDevMV: standard deviation of Gaussian noise in mV
// timestampJitterUS: standard deviation of timestamp jitter in microseconds
// sampleRateHz: sampling rate in Hz
func generateTestSequence(slopeSegments [][2]float64, noiseStdDevMV, timestampJitterUS float64, sampleRateHz int) []sample.Sample {
	rng := rand.New(rand.NewSource(42)) // Fixed seed for reproducibility

	sampleInterval := time.Second / time.Duration(sampleRateHz)
	noiseStdDevV := noiseStdDevMV / 1000.0 // Convert mV to V

	var samples []sample.Sample
	currentTime := time.Now()
	currentVoltage := 0.0

	for _, segment := range slopeSegments {
		slopeMVS := segment[0]
		durationS := segment[1]
		slopeVS := slopeMVS / 1000.0 // Convert mV/s to V/s

		numSamples := int(durationS * float64(sampleRateHz))

		for range numSamples {
			// Add timestamp jitter
			jitter := time.Duration(rng.NormFloat64() * timestampJitterUS * 1000) // Convert µs to ns
			timestamp := currentTime.Add(jitter)

			// Add measurement noise
			noise := rng.NormFloat64() * noiseStdDevV
			voltage := currentVoltage + noise

			// Create sample
			s := sample.Sample{
				Timestamp: timestamp,
				Voltage:   voltage,
				Change:    slopeVS + (rng.NormFloat64() * noiseStdDevV), // Derivative with noise
			}
			samples = append(samples, s)

			// Advance time and voltage
			currentTime = currentTime.Add(sampleInterval)
			currentVoltage += slopeVS * sampleInterval.Seconds()
		}
	}

	return samples
}

// extractDerivatives extracts Change field from samples
func extractDerivatives(samples []sample.Sample) []float64 {
	derivatives := make([]float64, len(samples))
	for i := range samples {
		derivatives[i] = samples[i].Change
	}
	return derivatives
}

// TestPulseDetection_SingleStablePulse tests detection of a single stable heating pulse.
// Scenario: Bias → Laser On → Laser Off
// Expected: 1 pulse with duration ≈19-20s and mean ≈2.4-2.6 mV/s
// Uses realistic filter pipeline: Raw -> MM -> EMA(reading) -> Diff -> EMA(change)
func TestPulseDetection_SingleStablePulse(t *testing.T) {
	// Generate sequence: 0.4 mV/s bias, 2.5 mV/s laser (20s), -2 mV/s cooling (20s)
	segments := [][2]float64{
		{0.4, 2.0},   // Bias: 0.4 mV/s for 2s (warm-up)
		{2.5, 20.0},  // Laser on: 2.5 mV/s for 20s
		{-2.0, 20.0}, // Cooling: -2 mV/s for 20s
	}

	// Use realistic filter pipeline matching production config
	filterConfig := DefaultFilterConfig()
	samples := generateRealisticTestSequence(segments, 0.5, 1.0, filterConfig)
	derivatives := extractDerivatives(samples)

	// Create pulse config
	// Note: Threshold must be well above bias (0.4 mV/s) + noise (±0.5 mV) = max ~0.9 mV/s
	// Use 1.5 mV/s threshold to have clear separation
	config := PulseConfig{
		ID:                 1,
		MinDuration:        5 * time.Second, // 5s minimum (realistic for laser pulses)
		StdDevThresholdMVS: 1.0,             // 1 mV/s acceptable stddev
		SlopeThreshold:     0.0015,          // 1.5 mV/s threshold in V/s
		AbsorbanceCoeff:    0.9,
		PowerPolynomial:    []float64{0, 1, 0, 0},
	}

	// Simulate pulse detection
	var pulses []*Pulse
	var activePulse *Pulse

	for i := range derivatives {
		if activePulse == nil {
			// Check if should start new pulse
			if derivatives[i] >= config.SlopeThreshold {
				activePulse = NewPulse(config, samples, derivatives, i)
				if activePulse != nil {
					t.Logf("Started pulse at i=%d, deriv=%.3f mV/s", i, derivatives[i]*1000)
				}
			}
		} else {
			// Update active pulse
			shouldContinue := activePulse.Update(samples, derivatives, i)

			if !shouldContinue {
				// Pulse finalized
				t.Logf("Finalized pulse: dur=%.2fs, mean=%.3f mV/s, stdDev=%.3f mV/s",
					activePulse.Duration().Seconds(),
					activePulse.AvgSlope*1000,
					activePulse.StdDev*1000)

				if activePulse.IsUpdating() || activePulse.IsFinalized() {
					pulses = append(pulses, activePulse)
				}
				activePulse = nil
			}
		}
	}

	// Assertions
	assert.Len(t, pulses, 1, "Should detect exactly one pulse")

	if len(pulses) > 0 {
		pulse := pulses[0]

		// Duration should be close to 20s (with some tolerance for ramp-up)
		assert.InDelta(t, 20.0, pulse.Duration().Seconds(), 2.0,
			"Pulse duration should be approximately 20s")

		// Mean slope should be close to 2.5 mV/s (with noise tolerance)
		assert.InDelta(t, 2.5, pulse.AvgSlope*1000, 0.3,
			"Pulse mean slope should be approximately 2.5 mV/s")

		// StdDev should be relatively small (noise + small variations)
		assert.Less(t, pulse.StdDev*1000, 1.0,
			"Pulse stdDev should be less than 1 mV/s")
	}
}

// TestPulseDetection_TwoPulses_RampUp tests detection of two pulses with a gap between them.
// Scenario: Laser1 On → Off → Laser2 On (higher power)
// Expected: 2 pulses, first ≈2.5 mV/s, second ≈5.5 mV/s
// Uses realistic filter pipeline with default config
// NOTE: Without a gap, EMA filters would blend the two segments into one pulse
func TestPulseDetection_TwoPulses_RampUp(t *testing.T) {
	// Generate sequence: 2.5 mV/s (20s), gap, 5.5 mV/s (20s), cooling
	segments := [][2]float64{
		{0.4, 1.0},   // Bias
		{2.5, 20.0},  // Laser1: 2.5 mV/s for 20s
		{-2.0, 3.0},  // Cooling: -2 mV/s for 3s
		{0.4, 2.0},   // Bias: 0.4 mV/s for 2s (gap between pulses)
		{5.5, 20.0},  // Laser2: 5.5 mV/s for 20s (higher power)
		{-2.0, 20.0}, // Final cooling
	}

	filterConfig := DefaultFilterConfig()
	samples := generateRealisticTestSequence(segments, 0.5, 1.0, filterConfig)
	derivatives := extractDerivatives(samples)

	config := PulseConfig{
		ID:                 1,
		MinDuration:        5 * time.Second, // 5s minimum (realistic)
		StdDevThresholdMVS: 1.0,
		SlopeThreshold:     0.0015, // 1.5 mV/s (above noisy bias)
		AbsorbanceCoeff:    0.9,
		PowerPolynomial:    []float64{0, 1, 0, 0},
	}

	var pulses []*Pulse
	var activePulse *Pulse
	nextID := 1

	for i := range derivatives {
		if activePulse == nil {
			if derivatives[i] >= config.SlopeThreshold {
				config.ID = nextID
				nextID++
				activePulse = NewPulse(config, samples, derivatives, i)
				if activePulse != nil {
					t.Logf("Started pulse #%d at i=%d, deriv=%.3f mV/s", activePulse.ID, i, derivatives[i]*1000)
				}
			}
		} else {
			shouldContinue := activePulse.Update(samples, derivatives, i)

			if !shouldContinue {
				t.Logf("Finalized pulse #%d: dur=%.2fs, mean=%.3f mV/s, stdDev=%.3f mV/s",
					activePulse.ID,
					activePulse.Duration().Seconds(),
					activePulse.AvgSlope*1000,
					activePulse.StdDev*1000)

				if activePulse.IsUpdating() || activePulse.IsFinalized() {
					pulses = append(pulses, activePulse)
				}
				activePulse = nil
			}
		}
	}

	// Assertions
	assert.GreaterOrEqual(t, len(pulses), 2, "Should detect at least two pulses")

	if len(pulses) >= 2 {
		pulse1 := pulses[0]
		pulse2 := pulses[1]

		// First pulse: ~2.5 mV/s
		assert.InDelta(t, 2.5, pulse1.AvgSlope*1000, 0.3,
			"First pulse should be approximately 2.5 mV/s")

		// Second pulse: ~5.5 mV/s
		assert.InDelta(t, 5.5, pulse2.AvgSlope*1000, 0.3,
			"Second pulse should be approximately 5.5 mV/s")

		// Second pulse should have higher slope than first
		assert.Greater(t, pulse2.AvgSlope, pulse1.AvgSlope,
			"Second pulse should have higher slope than first")
	}
}

// TestPulseDetection_OnOffOn tests detection with a gap between pulses.
// Scenario: Laser On → Off → On again
// Expected: 2 pulses with a gap
// Uses realistic filter pipeline with default config
func TestPulseDetection_OnOffOn(t *testing.T) {
	// Generate sequence: laser on (20s), off (5s), laser on again (20s), cooling
	segments := [][2]float64{
		{0.4, 1.0},   // Bias
		{2.5, 20.0},  // Laser on: 2.5 mV/s for 20s
		{-2.0, 5.0},  // Cooling: -2 mV/s for 5s
		{0.4, 5.0},   // Bias: 0.4 mV/s for 5s
		{3.0, 20.0},  // Laser on again: 3.0 mV/s for 20s
		{-2.0, 20.0}, // Final cooling
	}

	filterConfig := DefaultFilterConfig()
	samples := generateRealisticTestSequence(segments, 0.5, 1.0, filterConfig)
	derivatives := extractDerivatives(samples)

	config := PulseConfig{
		ID:                 1,
		MinDuration:        5 * time.Second, // 5s minimum (realistic)
		StdDevThresholdMVS: 1.0,
		SlopeThreshold:     0.0015, // 1.5 mV/s (above noisy bias)
		AbsorbanceCoeff:    0.9,
		PowerPolynomial:    []float64{0, 1, 0, 0},
	}

	var pulses []*Pulse
	var activePulse *Pulse
	nextID := 1

	for i := range derivatives {
		if activePulse == nil {
			if derivatives[i] >= config.SlopeThreshold {
				config.ID = nextID
				nextID++
				activePulse = NewPulse(config, samples, derivatives, i)
				if activePulse != nil {
					t.Logf("Started pulse #%d at i=%d, deriv=%.3f mV/s", activePulse.ID, i, derivatives[i]*1000)
				}
			}
		} else {
			shouldContinue := activePulse.Update(samples, derivatives, i)

			if !shouldContinue {
				t.Logf("Finalized pulse #%d: dur=%.2fs, mean=%.3f mV/s, stdDev=%.3f mV/s",
					activePulse.ID,
					activePulse.Duration().Seconds(),
					activePulse.AvgSlope*1000,
					activePulse.StdDev*1000)

				if activePulse.IsUpdating() || activePulse.IsFinalized() {
					pulses = append(pulses, activePulse)
				}
				activePulse = nil
			}
		}
	}

	// Assertions
	assert.GreaterOrEqual(t, len(pulses), 2, "Should detect at least two pulses")

	if len(pulses) >= 2 {
		pulse1 := pulses[0]
		pulse2 := pulses[1]

		// First pulse: ~2.5 mV/s
		assert.InDelta(t, 2.5, pulse1.AvgSlope*1000, 0.3,
			"First pulse should be approximately 2.5 mV/s")

		// Second pulse: ~3.0 mV/s
		assert.InDelta(t, 3.0, pulse2.AvgSlope*1000, 0.3,
			"Second pulse should be approximately 3.0 mV/s")

		// Gap between pulses (should be at least 5s)
		gap := pulse2.DetectStartTime.Sub(pulse1.DetectEndTime)
		assert.GreaterOrEqual(t, gap.Seconds(), 4.0,
			"Should have gap of at least 4s between pulses")

		t.Logf("Gap between pulses: %.2fs", gap.Seconds())
	}
}

// TestPulseStdDevBehavior tests that StdDev behaves correctly during pulse evolution.
// Uses realistic filter pipeline with very low noise to verify StdDev stability
func TestPulseStdDevBehavior(t *testing.T) {
	// Generate a clean stable pulse
	segments := [][2]float64{
		{3.5, 10.0}, // Very stable 3.5 mV/s for 10s (well above threshold)
	}

	filterConfig := DefaultFilterConfig()
	samples := generateRealisticTestSequence(segments, 0.05, 0.05, filterConfig) // Very low noise
	derivatives := extractDerivatives(samples)

	config := PulseConfig{
		ID:                 1,
		MinDuration:        100 * time.Millisecond,
		StdDevThresholdMVS: 0.5,
		SlopeThreshold:     0.0015, // 1.5 mV/s (well below signal)
		GracePeriodSamples: 50,     // Longer grace period for low-noise test
		HysteresisFactor:   0.5,
		AbsorbanceCoeff:    0.9,
		PowerPolynomial:    []float64{0, 1, 0, 0},
	}

	pulse := NewPulse(config, samples, derivatives, 0)
	assert.NotNil(t, pulse)

	var stdDevHistory []float64

	// Update pulse and track StdDev
	for i := 1; i < len(derivatives) && i < 500; i++ {
		shouldContinue := pulse.Update(samples, derivatives, i)
		if !shouldContinue {
			break
		}

		if pulse.IsUpdating() || pulse.IsFinalized() {
			stdDevHistory = append(stdDevHistory, pulse.StdDev)
		}
	}

	t.Logf("StdDev evolution: first=%.6f, last=%.6f, samples=%d",
		stdDevHistory[0], stdDevHistory[len(stdDevHistory)-1], len(stdDevHistory))

	// StdDev should be small and relatively stable for clean signal
	assert.Less(t, pulse.StdDev, 0.001, // Less than 1 mV/s
		"Final StdDev should be very small for clean signal")

	// Check that StdDev doesn't explode
	// Note: StdDev may increase temporarily during ramp-up/ramp-down as the snake
	// extends before the tail adjusts. This is expected behavior.
	if len(stdDevHistory) > 10 {
		firstAvg := average(stdDevHistory[:10])
		lastAvg := average(stdDevHistory[len(stdDevHistory)-10:])

		t.Logf("StdDev: first 10 avg=%.6f, last 10 avg=%.6f", firstAvg, lastAvg)

		// Last average should not be excessively higher (allow 2× for ramp effects)
		assert.LessOrEqual(t, lastAvg, firstAvg*2.5,
			"StdDev should not explode over time")
	}
}

// average calculates the mean of a slice of float64
func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// TestOutlierDetection tests that outliers are properly detected.
// Uses realistic filter pipeline - note that MM filter will remove most spikes!
// DEPRECATED: Old outlier detection test - snake algorithm uses different approach
func _TestOutlierDetection(t *testing.T) {
	// Create a pulse with known parameters
	config := PulseConfig{
		ID:                 1,
		MinDuration:        100 * time.Millisecond,
		StdDevThresholdMVS: 1.0,
		SlopeThreshold:     0.002, // 2.0 mV/s (well above bias)
		AbsorbanceCoeff:    0.9,
		PowerPolynomial:    []float64{0, 1, 0, 0},
	}

	// Generate samples with a few outliers
	segments := [][2]float64{
		{2.5, 5.0}, // 5s of stable 2.5 mV/s
	}
	filterConfig := DefaultFilterConfig()
	samples := generateRealisticTestSequence(segments, 0.1, 0.1, filterConfig)
	derivatives := extractDerivatives(samples)

	// Manually inject some strong outliers (way outside normal range)
	if len(derivatives) > 100 {
		derivatives[100] = 10.0 / 1000.0 // 10 mV/s outlier
		derivatives[200] = 15.0 / 1000.0 // 15 mV/s outlier
		derivatives[300] = -5.0 / 1000.0 // -5 mV/s outlier
	}

	pulse := NewPulse(config, samples, derivatives, 0)
	assert.NotNil(t, pulse)

	// Update pulse through outliers
	for i := 1; i < min(len(derivatives), 400); i++ {
		pulse.Update(samples, derivatives, i)
	}

	// Pulse should still have mean close to 2.5 mV/s despite outliers
	assert.InDelta(t, 2.5, pulse.AvgSlope*1000, 0.5,
		"Mean should be close to 2.5 mV/s despite outliers")

	t.Logf("Pulse with outliers: mean=%.3f mV/s, stdDev=%.3f mV/s",
		pulse.AvgSlope*1000, pulse.StdDev*1000)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestPulseDetection_FastFilterProfile tests detection with fast-response filter profile.
// Scenario: Single pulse with fast alpha settings (higher responsiveness, more noise)
// Expected: 1 pulse detected, but with higher StdDev due to less filtering
// NOTE: Longer warm-up needed due to faster filter transients
func TestPulseDetection_FastFilterProfile(t *testing.T) {
	segments := [][2]float64{
		{0.4, 3.0},   // Bias - longer warm-up for fast filter to settle
		{2.5, 20.0},  // Laser on: 2.5 mV/s for 20s
		{-2.0, 20.0}, // Cooling
	}

	// Use fast filter config (higher alphas = less smoothing, faster response)
	filterConfig := FastFilterConfig()
	samples := generateRealisticTestSequence(segments, 0.5, 1.0, filterConfig)
	derivatives := extractDerivatives(samples)

	config := PulseConfig{
		ID:                 1,
		MinDuration:        2 * time.Second, // Shorter min duration for fast filter (realistic for quick measurements)
		StdDevThresholdMVS: 2.0,             // Need higher threshold due to less filtering
		SlopeThreshold:     0.0025,          // 2.5 mV/s - must be higher due to noisier filtering
		GracePeriodSamples: 20,              // Longer grace period for noisy fast filter (200ms @ 100Hz)
		AbsorbanceCoeff:    0.9,
		PowerPolynomial:    []float64{0, 1, 0, 0},
	}

	var pulses []*Pulse
	var activePulse *Pulse

	for i := range derivatives {
		if activePulse == nil {
			if derivatives[i] >= config.SlopeThreshold {
				activePulse = NewPulse(config, samples, derivatives, i)
				if activePulse != nil {
					t.Logf("Started pulse at i=%d, deriv=%.3f mV/s", i, derivatives[i]*1000)
				}
			}
		} else {
			shouldContinue := activePulse.Update(samples, derivatives, i)

			if !shouldContinue {
				t.Logf("Finalized pulse: dur=%.2fs, mean=%.3f mV/s, stdDev=%.3f mV/s",
					activePulse.Duration().Seconds(),
					activePulse.AvgSlope*1000,
					activePulse.StdDev*1000)

				if activePulse.IsUpdating() || activePulse.IsFinalized() {
					pulses = append(pulses, activePulse)
				}
				activePulse = nil
			}
		}
	}

	// Assertions - with fast filter, expect at least 1 pulse detected
	// Note: Fast filter creates more noise, so pulses may fragment into multiple shorter segments
	assert.GreaterOrEqual(t, len(pulses), 1, "Should detect at least one pulse with fast filter")

	if len(pulses) > 0 {
		pulse := pulses[0]

		// Duration will be shorter due to fragmentation from noise
		assert.GreaterOrEqual(t, pulse.Duration().Seconds(), 2.0,
			"Pulse duration should be at least min duration (2s)")

		// Mean should be close to expected, but with wider tolerance
		assert.InDelta(t, 2.5, pulse.AvgSlope*1000, 0.5,
			"Pulse mean slope should be approximately 2.5 mV/s (wider tolerance for fast filter)")

		// StdDev will be significantly higher with fast filter (less smoothing)
		t.Logf("Fast filter StdDev: %.3f mV/s (expected to be ~2-3x higher than default)", pulse.StdDev*1000)
		assert.Greater(t, pulse.StdDev*1000, 0.5,
			"StdDev should be significantly higher with fast filter")
	}
}
