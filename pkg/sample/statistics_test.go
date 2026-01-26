package sample

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculateStatistics(t *testing.T) {
	// Test with known values
	values := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	stats := calculateStatistics(values)

	assert.Equal(t, 5, stats.Count)
	assert.InDelta(t, 3.0, stats.Mean, 0.001)
	assert.InDelta(t, math.Sqrt(2.0), stats.StdDev, 0.001) // Variance = 2.0
	assert.Equal(t, 1.0, stats.Min)
	assert.Equal(t, 5.0, stats.Max)
	assert.InDelta(t, math.Sqrt(2.0)/3.0, stats.Variability, 0.001)
}

func TestCalculateStatistics_Empty(t *testing.T) {
	values := []float64{}
	stats := calculateStatistics(values)

	assert.Equal(t, 0, stats.Count)
	assert.Equal(t, 0.0, stats.Mean)
	assert.Equal(t, 0.0, stats.StdDev)
}

func TestCalculateVariabilityAgainstEMA(t *testing.T) {
	// Test with known values
	input := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	ema := []float64{1.5, 2.5, 3.5, 4.5, 5.5}

	variability := calculateVariabilityAgainstEMA(input, ema)

	// RMSE = sqrt(sum((input[i] - ema[i])^2) / n)
	// All differences are 0.5, so RMSE = 0.5
	assert.InDelta(t, 0.5, variability, 0.001)
}

func TestCalculateVariabilityAgainstEMA_Empty(t *testing.T) {
	input := []float64{}
	ema := []float64{}

	variability := calculateVariabilityAgainstEMA(input, ema)
	assert.Equal(t, 0.0, variability)
}

func TestCalculateVariabilityAgainstEMA_MismatchedLength(t *testing.T) {
	input := []float64{1.0, 2.0, 3.0}
	ema := []float64{1.0, 2.0}

	variability := calculateVariabilityAgainstEMA(input, ema)
	assert.Equal(t, 0.0, variability)
}

func TestEstimateOptimalEMA(t *testing.T) {
	// Test with typical values
	noiseStdDev := 0.001
	sampleRate := 50.0 // 50 samples per second
	targetDelay := 1.0 // 1 second

	alpha := estimateOptimalEMA(noiseStdDev, sampleRate, targetDelay)

	// For 1s delay at 50 Hz: alpha = 1 / (1 + 1*50) = 1/51 ≈ 0.0196
	expectedAlpha := 1.0 / (1.0 + targetDelay*sampleRate)
	assert.InDelta(t, expectedAlpha, alpha, 0.001)
}

func TestEstimateOptimalEMA_InvalidInputs(t *testing.T) {
	// Test with invalid inputs
	alpha := estimateOptimalEMA(0.001, 0.0, 1.0) // Invalid sample rate
	assert.Equal(t, 0.1, alpha)                  // Should return default

	alpha = estimateOptimalEMA(0.001, 50.0, 0.0) // Invalid target delay
	assert.Equal(t, 0.1, alpha)                  // Should return default
}

func TestNewStatisticsConverter_PassThrough(t *testing.T) {
	// Test that statistics converter passes through samples unchanged
	converter := NewStatisticsConverter(0.25, 10)

	in := make(chan Sample, 10)
	out := converter(in)

	// Send test samples
	testSamples := []Sample{
		{Timestamp: time.Now(), Reading: 1.0, Voltage: 3.3},
		{Timestamp: time.Now(), Reading: 2.0, Voltage: 3.4},
		{Timestamp: time.Now(), Reading: 3.0, Voltage: 3.5},
	}

	go func() {
		for _, s := range testSamples {
			in <- s
		}
		close(in)
	}()

	// Collect output samples
	var outputSamples []Sample
	for s := range out {
		outputSamples = append(outputSamples, s)
	}

	// Verify samples are passed through unchanged
	assert.Equal(t, len(testSamples), len(outputSamples))
	for i := range testSamples {
		assert.InDelta(t, testSamples[i].Reading, outputSamples[i].Reading, 0.001)
		assert.InDelta(t, testSamples[i].Voltage, outputSamples[i].Voltage, 0.001)
	}
}

func TestNewStatisticsConverter_NoEMA(t *testing.T) {
	// Test statistics converter without EMA (alpha = 0)
	converter := NewStatisticsConverter(0.0, 10)

	in := make(chan Sample, 10)
	out := converter(in)

	// Send test samples
	go func() {
		for i := range 5 {
			in <- Sample{
				Timestamp: time.Now(),
				Reading:   float64(i + 1),
				Voltage:   3.3,
			}
		}
		close(in)
	}()

	// Drain output
	for range out {
	}

	// Statistics should be logged (check manually in logs)
	// This test just verifies no crash occurs
}

func TestNewStatisticsConverter_WithEMA(t *testing.T) {
	// Test statistics converter with EMA
	converter := NewStatisticsConverter(0.25, 10)

	in := make(chan Sample, 10)
	out := converter(in)

	// Send test samples
	go func() {
		for i := range 10 {
			in <- Sample{
				Timestamp: time.Now(),
				Reading:   float64(i + 1),
				Voltage:   3.3,
			}
			time.Sleep(10 * time.Millisecond) // Simulate sample rate
		}
		close(in)
	}()

	// Drain output
	for range out {
	}

	// Statistics should be logged with EMA comparison (check manually in logs)
	// This test just verifies no crash occurs
}
