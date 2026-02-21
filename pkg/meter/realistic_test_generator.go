package meter

import (
	"math/rand"
	"time"

	"github.com/itohio/golpm/pkg/sample"
)

// FilterPipelineConfig contains configuration for the realistic filter pipeline
type FilterPipelineConfig struct {
	SpikeFilterWindow time.Duration // Moving median window for spike removal
	SmoothingAlpha    float64       // EMA alpha for Reading/Voltage smoothing
	ChangeFilterAlpha float64       // EMA alpha for Change (derivative) smoothing
	SampleRate        time.Duration // Sample interval (e.g. 10ms for 100Hz)
}

// DefaultFilterConfig returns the default config matching config.yaml
func DefaultFilterConfig() FilterPipelineConfig {
	return FilterPipelineConfig{
		SpikeFilterWindow: 500 * time.Millisecond,
		SmoothingAlpha:    0.05,
		ChangeFilterAlpha: 0.2,
		SampleRate:        10 * time.Millisecond, // 100Hz
	}
}

// generateRealisticTestSequence creates samples through the actual filter pipeline.
// This matches the production pipeline: Raw -> MM -> EMA(reading) -> Diff -> EMA(change)
func generateRealisticTestSequence(
	slopeSegments [][2]float64, // [slopeMVS, durationS]
	noiseStdDevMV float64,
	timestampJitterUS float64,
	config FilterPipelineConfig,
) []sample.Sample {
	rng := rand.New(rand.NewSource(42)) // Fixed seed for reproducibility

	// Step 1: Generate raw samples with noise
	rawSamples := generateRawSamples(slopeSegments, noiseStdDevMV, timestampJitterUS, config.SampleRate, rng)

	// Step 2: Apply Moving Median filter to remove spikes (on Reading field)
	mmFiltered := applyMovingMedian(rawSamples, config.SpikeFilterWindow, config.SampleRate)

	// Step 3: Apply EMA smoothing to Reading
	emaSmoothed := applyEMAToReading(mmFiltered, config.SmoothingAlpha)

	// Step 4: Calculate derivatives (Change field) from smoothed Reading
	withDerivatives := calculateDerivatives(emaSmoothed)

	// Step 5: Apply EMA smoothing to Change field
	finalSamples := applyEMAToChange(withDerivatives, config.ChangeFilterAlpha)

	return finalSamples
}

// generateRawSamples creates raw unfiltered samples with realistic noise
func generateRawSamples(
	slopeSegments [][2]float64,
	noiseStdDevMV float64,
	timestampJitterUS float64,
	sampleRate time.Duration,
	rng *rand.Rand,
) []sample.Sample {
	noiseStdDevV := noiseStdDevMV / 1000.0 // Convert mV to V

	var samples []sample.Sample
	currentTime := time.Now()
	currentVoltage := 0.0

	for _, segment := range slopeSegments {
		slopeMVS := segment[0]
		durationS := segment[1]
		slopeVS := slopeMVS / 1000.0 // Convert mV/s to V/s

		numSamples := int(durationS * float64(time.Second) / float64(sampleRate))

		for range numSamples {
			// Add timestamp jitter
			jitter := time.Duration(rng.NormFloat64() * timestampJitterUS * 1000) // Convert µs to ns
			timestamp := currentTime.Add(jitter)

			// Add measurement noise
			noise := rng.NormFloat64() * noiseStdDevV
			voltage := currentVoltage + noise

			// Create sample (Reading = Voltage for photodiode)
			s := sample.Sample{
				Timestamp: timestamp,
				Voltage:   voltage,
				Reading:   voltage, // Reading is same as Voltage for photodiode
				Change:    0,       // Will be calculated later
			}
			samples = append(samples, s)

			// Advance time and voltage
			currentTime = currentTime.Add(sampleRate)
			currentVoltage += slopeVS * sampleRate.Seconds()
		}
	}

	return samples
}

// applyMovingMedian applies moving median filter to Reading field
func applyMovingMedian(samples []sample.Sample, windowDuration time.Duration, sampleRate time.Duration) []sample.Sample {
	if windowDuration <= 0 {
		return samples
	}

	windowSize := int(windowDuration / sampleRate)
	if windowSize < 2 {
		return samples
	}

	filtered := make([]sample.Sample, len(samples))
	window := make([]float64, 0, windowSize)

	for i, s := range samples {
		// Add current reading to window
		window = append(window, s.Reading)

		// Keep window size limited
		if len(window) > windowSize {
			window = window[1:]
		}

		// Calculate median
		median := calculateMedian(window)

		// Create filtered sample
		filtered[i] = s
		filtered[i].Reading = median
		filtered[i].Voltage = median // Keep Voltage in sync with Reading
	}

	return filtered
}

// calculateMedian calculates the median of a slice (creates a copy to avoid modifying original)
func calculateMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Create a copy and sort it
	sorted := make([]float64, len(values))
	copy(sorted, values)

	// Simple bubble sort (good enough for small windows)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Return median
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2.0
	}
	return sorted[mid]
}

// applyEMAToReading applies EMA smoothing to Reading field
func applyEMAToReading(samples []sample.Sample, alpha float64) []sample.Sample {
	if alpha <= 0 || alpha >= 1 {
		return samples // No smoothing
	}

	filtered := make([]sample.Sample, len(samples))
	ema := 0.0
	initialized := false

	for i, s := range samples {
		if !initialized {
			ema = s.Reading
			initialized = true
		} else {
			ema = alpha*s.Reading + (1-alpha)*ema
		}

		filtered[i] = s
		filtered[i].Reading = ema
		filtered[i].Voltage = ema // Keep Voltage in sync
	}

	return filtered
}

// calculateDerivatives calculates Change field from Reading
func calculateDerivatives(samples []sample.Sample) []sample.Sample {
	if len(samples) < 2 {
		return samples
	}

	withDerivatives := make([]sample.Sample, len(samples))
	copy(withDerivatives, samples)

	// First sample has no derivative
	withDerivatives[0].Change = 0

	// Calculate derivatives for rest
	for i := 1; i < len(samples); i++ {
		dt := samples[i].Timestamp.Sub(samples[i-1].Timestamp).Seconds()
		if dt > 0 {
			dv := samples[i].Reading - samples[i-1].Reading
			withDerivatives[i].Change = dv / dt
		} else {
			withDerivatives[i].Change = 0
		}
	}

	return withDerivatives
}

// applyEMAToChange applies EMA smoothing to Change field
func applyEMAToChange(samples []sample.Sample, alpha float64) []sample.Sample {
	if alpha <= 0 || alpha >= 1 {
		return samples // No smoothing
	}

	filtered := make([]sample.Sample, len(samples))
	ema := 0.0
	initialized := false

	for i, s := range samples {
		if !initialized {
			ema = s.Change
			initialized = true
		} else {
			ema = alpha*s.Change + (1-alpha)*ema
		}

		filtered[i] = s
		filtered[i].Change = ema
	}

	return filtered
}

// FastFilterConfig returns a config with higher alphas for faster response
// This simulates the alternate profile for measuring high/low power pulses quickly
func FastFilterConfig() FilterPipelineConfig {
	return FilterPipelineConfig{
		SpikeFilterWindow: 200 * time.Millisecond, // Shorter spike window
		SmoothingAlpha:    0.15,                    // Higher alpha = less smoothing, faster response
		ChangeFilterAlpha: 0.4,                     // Higher alpha for derivative
		SampleRate:        10 * time.Millisecond,
	}
}

// SlowFilterConfig returns a config with lower alphas for maximum stability
// This simulates the alternate profile for very stable measurements
func SlowFilterConfig() FilterPipelineConfig {
	return FilterPipelineConfig{
		SpikeFilterWindow: 1000 * time.Millisecond, // Longer spike window
		SmoothingAlpha:    0.02,                     // Lower alpha = more smoothing, slower response
		ChangeFilterAlpha: 0.1,                      // Lower alpha for derivative
		SampleRate:        10 * time.Millisecond,
	}
}
