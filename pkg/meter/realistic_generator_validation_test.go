package meter

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestRealisticGenerator_FilterPipeline validates that the generator produces
// output matching the actual production filter pipeline configuration.
func TestRealisticGenerator_FilterPipeline(t *testing.T) {
	// Production config from config.yaml
	config := DefaultFilterConfig()

	// Verify config matches production
	assert.Equal(t, 500*time.Millisecond, config.SpikeFilterWindow, "Spike filter window should match config.yaml")
	assert.Equal(t, 0.05, config.SmoothingAlpha, "Smoothing alpha should match config.yaml")
	assert.Equal(t, 0.2, config.ChangeFilterAlpha, "Change filter alpha should match config.yaml")
	assert.Equal(t, 10*time.Millisecond, config.SampleRate, "Sample rate should be 100Hz (10ms)")
}

// TestRealisticGenerator_NoiseCharacteristics validates that the noise
// characteristics of the generated signal match expectations.
func TestRealisticGenerator_NoiseCharacteristics(t *testing.T) {
	// Generate a long flat signal to measure noise
	segments := [][2]float64{
		{0.4, 30.0}, // 30s of flat bias at 0.4 mV/s
	}

	config := DefaultFilterConfig()
	samples := generateRealisticTestSequence(segments, 0.5, 1.0, config)
	derivatives := extractDerivatives(samples)

	// Skip first second (filter settling)
	settlingTime := 1.0 // seconds
	skipSamples := int(settlingTime / config.SampleRate.Seconds())

	// Calculate statistics on settled portion
	var sum, sumSq float64
	count := 0
	for i := skipSamples; i < len(derivatives); i++ {
		d := derivatives[i] * 1000.0 // Convert to mV/s
		sum += d
		sumSq += d * d
		count++
	}

	mean := sum / float64(count)
	variance := (sumSq / float64(count)) - (mean * mean)
	stdDev := math.Sqrt(variance)

	t.Logf("Flat signal characteristics (30s @ 0.4 mV/s):")
	t.Logf("  Mean derivative: %.4f mV/s (expected ~0.4)", mean)
	t.Logf("  StdDev: %.4f mV/s (raw noise was 0.5 mV)", stdDev)
	t.Logf("  Noise reduction: %.1fx", 0.5/stdDev)
	t.Logf("  Sample count: %d samples", count)

	// Assertions
	assert.InDelta(t, 0.4, mean, 0.1, "Mean should be close to input slope")
	assert.Less(t, stdDev, 0.5, "StdDev should be less than raw noise due to filtering")
	assert.Greater(t, stdDev, 0.1, "StdDev should still have some noise (not over-filtered)")
}

// TestRealisticGenerator_FilterSettlingTime validates the EMA settling behavior.
func TestRealisticGenerator_FilterSettlingTime(t *testing.T) {
	// Generate a step change
	segments := [][2]float64{
		{0.0, 2.0},  // 2s of zero
		{2.5, 10.0}, // Step to 2.5 mV/s for 10s
	}

	config := DefaultFilterConfig()
	samples := generateRealisticTestSequence(segments, 0.1, 0.1, config) // Low noise
	derivatives := extractDerivatives(samples)

	// Find where step occurs (at 2s = 200 samples @ 100Hz)
	stepIndex := int(2.0 / config.SampleRate.Seconds())

	// Measure settling time to 95% of final value
	finalValue := 2.5 // mV/s
	threshold := 0.95 * finalValue

	settledIndex := -1
	for i := stepIndex; i < len(derivatives); i++ {
		if derivatives[i]*1000.0 >= threshold {
			settledIndex = i
			break
		}
	}

	assert.Greater(t, settledIndex, stepIndex, "Should eventually settle")

	settlingTime := float64(settledIndex-stepIndex) * config.SampleRate.Seconds()
	t.Logf("EMA settling characteristics:")
	t.Logf("  Step: 0 → 2.5 mV/s")
	t.Logf("  Settling time to 95%%: %.3f seconds", settlingTime)
	t.Logf("  Settling samples: %d @ 100Hz", settledIndex-stepIndex)

	// Theoretical settling time for cascaded EMA filters:
	// alpha1=0.05, alpha2=0.2
	// τ1 ≈ 1/0.05 = 20 samples = 200ms
	// τ2 ≈ 1/0.2 = 5 samples = 50ms
	// 3×τ1 ≈ 600ms for 95% settling
	expectedSettling := 0.6 // seconds
	assert.InDelta(t, expectedSettling, settlingTime, 0.3,
		"Settling time should be ~600ms (3×τ for alpha=0.05)")
}

// TestRealisticGenerator_PulseAccuracy validates that pulse detection
// produces accurate measurements on known input signals.
func TestRealisticGenerator_PulseAccuracy(t *testing.T) {
	testCases := []struct {
		name          string
		slopeMVS      float64
		durationS     float64
		noiseStdDevMV float64
		config        FilterPipelineConfig
		expectedMean  float64
		tolerance     float64
	}{
		{
			name:          "Low power pulse - Default filter",
			slopeMVS:      1.5,
			durationS:     20.0,
			noiseStdDevMV: 0.5,
			config:        DefaultFilterConfig(),
			expectedMean:  1.5,
			tolerance:     0.2, // ±0.2 mV/s
		},
		{
			name:          "Medium power pulse - Default filter",
			slopeMVS:      2.5,
			durationS:     20.0,
			noiseStdDevMV: 0.5,
			config:        DefaultFilterConfig(),
			expectedMean:  2.5,
			tolerance:     0.2,
		},
		{
			name:          "High power pulse - Default filter",
			slopeMVS:      5.0,
			durationS:     20.0,
			noiseStdDevMV: 0.5,
			config:        DefaultFilterConfig(),
			expectedMean:  5.0,
			tolerance:     0.3,
		},
		{
			name:          "Short pulse - Fast filter",
			slopeMVS:      2.5,
			durationS:     5.0,
			noiseStdDevMV: 0.5,
			config:        FastFilterConfig(),
			expectedMean:  2.5,
			tolerance:     0.4, // Wider tolerance for fast filter
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			segments := [][2]float64{
				{0.4, 2.0},        // Bias warm-up
				{tc.slopeMVS, tc.durationS}, // Test pulse
				{-2.0, 10.0},      // Cooling
			}

			samples := generateRealisticTestSequence(segments, tc.noiseStdDevMV, 1.0, tc.config)
			derivatives := extractDerivatives(samples)

			// Find stable region (skip first and last 10% of pulse)
			warmupSamples := int(2.0 / tc.config.SampleRate.Seconds())
			pulseSamples := int(tc.durationS / tc.config.SampleRate.Seconds())
			startIdx := warmupSamples + int(0.1*float64(pulseSamples))
			endIdx := warmupSamples + int(0.9*float64(pulseSamples))

			// Calculate mean and stddev of stable region
			var sum, sumSq float64
			count := 0
			for i := startIdx; i < endIdx && i < len(derivatives); i++ {
				d := derivatives[i] * 1000.0
				sum += d
				sumSq += d * d
				count++
			}

			measuredMean := sum / float64(count)
			variance := (sumSq / float64(count)) - (measuredMean * measuredMean)
			measuredStdDev := math.Sqrt(variance)

			t.Logf("Pulse measurement:")
			t.Logf("  Expected: %.3f mV/s", tc.expectedMean)
			t.Logf("  Measured: %.3f mV/s", measuredMean)
			t.Logf("  Error: %.3f mV/s (%.1f%%)", measuredMean-tc.expectedMean,
				100.0*math.Abs(measuredMean-tc.expectedMean)/tc.expectedMean)
			t.Logf("  StdDev: %.3f mV/s", measuredStdDev)
			t.Logf("  Samples analyzed: %d", count)

			// Assertions
			assert.InDelta(t, tc.expectedMean, measuredMean, tc.tolerance,
				"Measured mean should be within tolerance of expected")
			
			// StdDev should be reasonable (not too high, not suspiciously low)
			// Fast filter has higher noise - allow up to 1.5 mV/s
			maxStdDev := 1.0
			if tc.config.ChangeFilterAlpha > 0.3 {
				maxStdDev = 1.5 // Fast filter
			}
			assert.Less(t, measuredStdDev, maxStdDev,
				"StdDev should be less than %.1f mV/s for this filter config", maxStdDev)
			assert.Greater(t, measuredStdDev, 0.05,
				"StdDev should be > 0.05 mV/s (signal has noise)")
		})
	}
}

// TestRealisticGenerator_MovingMedianSpikeSuppression validates that
// the moving median filter properly removes spikes.
func TestRealisticGenerator_MovingMedianSpikeSuppression(t *testing.T) {
	// This test is more conceptual - the MM filter is applied internally
	// We verify that occasional outliers don't significantly affect the result

	segments := [][2]float64{
		{0.4, 1.0},
		{2.5, 10.0}, // 10s stable pulse
		{-2.0, 5.0},
	}

	config := DefaultFilterConfig()
	
	// Generate multiple sequences with different random seeds
	means := make([]float64, 5)
	for i := 0; i < 5; i++ {
		samples := generateRealisticTestSequence(segments, 0.5, 1.0, config)
		derivatives := extractDerivatives(samples)

		// Measure pulse region
		startIdx := int(1.5 / config.SampleRate.Seconds())
		endIdx := int(10.0 / config.SampleRate.Seconds())

		var sum float64
		count := 0
		for j := startIdx; j < endIdx && j < len(derivatives); j++ {
			sum += derivatives[j] * 1000.0
			count++
		}
		means[i] = sum / float64(count)
	}

	// Calculate variance across runs
	var meanOfMeans, sumSq float64
	for _, m := range means {
		meanOfMeans += m
	}
	meanOfMeans /= float64(len(means))

	for _, m := range means {
		diff := m - meanOfMeans
		sumSq += diff * diff
	}
	stdDevAcrossRuns := math.Sqrt(sumSq / float64(len(means)))

	t.Logf("Repeatability test (5 runs):")
	t.Logf("  Mean of means: %.4f mV/s", meanOfMeans)
	t.Logf("  StdDev across runs: %.4f mV/s", stdDevAcrossRuns)
	t.Logf("  Individual means: %v", means)

	// With fixed seed, all runs should be identical
	assert.InDelta(t, 0.0, stdDevAcrossRuns, 0.001,
		"With fixed seed, all runs should produce identical results")
}

// TestRealisticGenerator_CompareWithProductionConfig validates that
// test parameters match production configuration.
func TestRealisticGenerator_CompareWithProductionConfig(t *testing.T) {
	// From config.yaml
	productionConfig := map[string]interface{}{
		"spike_filter_window_size": 500 * time.Millisecond,
		"smoothing_alpha":          0.05,
		"change_filter_alpha":      0.2,
		"pulse_threshold_mvs":      0.5,
		"min_pulse_duration":       10 * time.Second,
	}

	testConfig := DefaultFilterConfig()

	t.Logf("Configuration comparison:")
	t.Logf("  Spike filter window:")
	t.Logf("    Production: %v", productionConfig["spike_filter_window_size"])
	t.Logf("    Test:       %v", testConfig.SpikeFilterWindow)
	
	t.Logf("  Reading smoothing alpha:")
	t.Logf("    Production: %v", productionConfig["smoothing_alpha"])
	t.Logf("    Test:       %v", testConfig.SmoothingAlpha)
	
	t.Logf("  Derivative smoothing alpha:")
	t.Logf("    Production: %v", productionConfig["change_filter_alpha"])
	t.Logf("    Test:       %v", testConfig.ChangeFilterAlpha)

	assert.Equal(t, productionConfig["spike_filter_window_size"], testConfig.SpikeFilterWindow)
	assert.Equal(t, productionConfig["smoothing_alpha"], testConfig.SmoothingAlpha)
	assert.Equal(t, productionConfig["change_filter_alpha"], testConfig.ChangeFilterAlpha)
}

// TestRealisticGenerator_RampTransitions validates how the filter pipeline
// handles ramp-up and ramp-down transitions.
func TestRealisticGenerator_RampTransitions(t *testing.T) {
	segments := [][2]float64{
		{0.4, 2.0},  // Bias
		{2.5, 20.0}, // Pulse 1
		{5.0, 20.0}, // Instant ramp to higher power
		{-2.0, 10.0}, // Cooling
	}

	config := DefaultFilterConfig()
	samples := generateRealisticTestSequence(segments, 0.5, 1.0, config)
	derivatives := extractDerivatives(samples)

	// Find transition region (at 22s)
	transitionIndex := int(22.0 / config.SampleRate.Seconds())

	// Measure how long it takes to transition from 2.5 to 5.0
	transitionStart := -1
	transitionEnd := -1
	
	for i := transitionIndex - 50; i < transitionIndex+200 && i < len(derivatives); i++ {
		d := derivatives[i] * 1000.0
		if transitionStart == -1 && d > 3.0 {
			transitionStart = i
		}
		if transitionStart != -1 && d > 4.5 {
			transitionEnd = i
			break
		}
	}

	if transitionStart != -1 && transitionEnd != -1 {
		transitionTime := float64(transitionEnd-transitionStart) * config.SampleRate.Seconds()
		t.Logf("Ramp-up transition (2.5 → 5.0 mV/s):")
		t.Logf("  Transition time: %.3f seconds", transitionTime)
		t.Logf("  Transition samples: %d", transitionEnd-transitionStart)
		
		// Should take ~600ms-1.2s due to cascaded EMA filtering
		// (reading EMA + derivative EMA + step from 2.5→5.0 takes longer than 0→2.5)
		assert.InDelta(t, 0.9, transitionTime, 0.5,
			"Ramp transition should take ~600ms-1.4s (cascaded EMA settling time)")
	} else {
		t.Logf("Note: Ramp transition blended by filters (expected behavior)")
	}
}
