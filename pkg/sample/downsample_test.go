package sample

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownsampleSamples_NoDownsampling(t *testing.T) {
	now := time.Now()
	samples := []Sample{
		{Timestamp: now, Reading: 1.0, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(100 * time.Millisecond), Reading: 1.1, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(200 * time.Millisecond), Reading: 1.2, Voltage: 2.0, HeaterPower: 0.0},
	}

	// Test with nil dst
	result := DownsampleSamples(nil, samples, 10)
	require.Equal(t, 3, len(result))
	assert.Equal(t, samples[0], result[0])
	assert.Equal(t, samples[1], result[1])
	assert.Equal(t, samples[2], result[2])

	// Test with sufficient capacity dst
	dst := make([]Sample, 0, 10)
	result = DownsampleSamples(dst, samples, 10)
	require.Equal(t, 3, len(result))
	assert.Equal(t, samples[0], result[0])
	assert.Equal(t, samples[1], result[1])
	assert.Equal(t, samples[2], result[2])
	// Should reuse dst
	assert.Equal(t, cap(dst), cap(result))
}

func TestDownsampleSamples_WithDownsampling(t *testing.T) {
	now := time.Now()
	samples := make([]Sample, 100)
	for i := 0; i < 100; i++ {
		samples[i] = Sample{
			Timestamp:   now.Add(time.Duration(i) * 10 * time.Millisecond),
			Reading:     float64(i) * 0.01,
			Voltage:     2.0,
			HeaterPower: 0.0,
		}
	}

	// Downsample to 10 points
	dst := make([]Sample, 0, 20)
	result := DownsampleSamples(dst, samples, 10)
	require.Equal(t, 10, len(result))
	
	// Should always include first sample
	assert.Equal(t, samples[0], result[0])
	
	// Last sample should be close to the end (may not be exactly samples[99] due to decimation)
	// Check that we got samples from across the range
	assert.GreaterOrEqual(t, result[len(result)-1].Reading, 0.8) // Should be in last 20% of range
	
	// Should reuse dst if capacity sufficient
	assert.GreaterOrEqual(t, cap(result), 10)
}

func TestDownsampleSamples_DestinationReuse(t *testing.T) {
	now := time.Now()
	samples1 := []Sample{
		{Timestamp: now, Reading: 1.0, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(100 * time.Millisecond), Reading: 1.1, Voltage: 2.0, HeaterPower: 0.0},
	}
	
	samples2 := []Sample{
		{Timestamp: now, Reading: 2.0, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(100 * time.Millisecond), Reading: 2.1, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(200 * time.Millisecond), Reading: 2.2, Voltage: 2.0, HeaterPower: 0.0},
	}

	// First call
	dst := make([]Sample, 0, 10)
	result1 := DownsampleSamples(dst, samples1, 10)
	require.Equal(t, 2, len(result1))
	
	// Second call - should reuse dst
	result2 := DownsampleSamples(result1, samples2, 10)
	require.Equal(t, 3, len(result2))
	
	// Should reuse same underlying array
	assert.Equal(t, cap(result1), cap(result2))
}

func TestDownsampleSamples_EmptyInput(t *testing.T) {
	result := DownsampleSamples(nil, []Sample{}, 10)
	require.Equal(t, 0, len(result))
}

func TestDownsampleDerivatives_NoDownsampling(t *testing.T) {
	derivatives := []float64{0.1, 0.2, 0.3, 0.4, 0.5}

	// Test with nil dst
	result := DownsampleDerivatives(nil, derivatives, 10)
	require.Equal(t, 5, len(result))
	assert.Equal(t, derivatives[0], result[0])
	assert.Equal(t, derivatives[4], result[4])

	// Test with sufficient capacity dst
	dst := make([]float64, 0, 10)
	result = DownsampleDerivatives(dst, derivatives, 10)
	require.Equal(t, 5, len(result))
	assert.Equal(t, derivatives[0], result[0])
	assert.Equal(t, derivatives[4], result[4])
	// Should reuse dst
	assert.Equal(t, cap(dst), cap(result))
}

func TestDownsampleDerivatives_WithDownsampling(t *testing.T) {
	derivatives := make([]float64, 100)
	for i := 0; i < 100; i++ {
		derivatives[i] = float64(i) * 0.01
	}

	// Downsample to 10 points
	dst := make([]float64, 0, 20)
	result := DownsampleDerivatives(dst, derivatives, 10)
	require.Equal(t, 10, len(result))
	
	// Should always include first value
	assert.Equal(t, derivatives[0], result[0])
	
	// Last value should be close to the end (may not be exactly derivatives[99] due to decimation)
	// Check that we got values from across the range
	assert.GreaterOrEqual(t, result[len(result)-1], 0.8) // Should be in last 20% of range
	
	// Should reuse dst if capacity sufficient
	assert.GreaterOrEqual(t, cap(result), 10)
}

func TestDownsampleDerivatives_DestinationReuse(t *testing.T) {
	derivatives1 := []float64{0.1, 0.2}
	derivatives2 := []float64{0.3, 0.4, 0.5}

	// First call
	dst := make([]float64, 0, 10)
	result1 := DownsampleDerivatives(dst, derivatives1, 10)
	require.Equal(t, 2, len(result1))
	
	// Second call - should reuse dst
	result2 := DownsampleDerivatives(result1, derivatives2, 10)
	require.Equal(t, 3, len(result2))
	
	// Should reuse same underlying array
	assert.Equal(t, cap(result1), cap(result2))
}

func TestDownsampleDerivatives_EmptyInput(t *testing.T) {
	result := DownsampleDerivatives(nil, []float64{}, 10)
	require.Equal(t, 0, len(result))
}

func TestDownsampleSamples_ExactMaxPoints(t *testing.T) {
	now := time.Now()
	samples := make([]Sample, 10)
	for i := 0; i < 10; i++ {
		samples[i] = Sample{
			Timestamp:   now.Add(time.Duration(i) * 10 * time.Millisecond),
			Reading:     float64(i) * 0.01,
			Voltage:     2.0,
			HeaterPower: 0.0,
		}
	}

	// Downsample to exactly 10 points (same as input)
	result := DownsampleSamples(nil, samples, 10)
	require.Equal(t, 10, len(result))
	
	// Should be identical
	for i := 0; i < 10; i++ {
		assert.Equal(t, samples[i], result[i])
	}
}

func TestDownsampleDerivatives_ExactMaxPoints(t *testing.T) {
	derivatives := make([]float64, 10)
	for i := 0; i < 10; i++ {
		derivatives[i] = float64(i) * 0.01
	}

	// Downsample to exactly 10 points (same as input)
	result := DownsampleDerivatives(nil, derivatives, 10)
	require.Equal(t, 10, len(result))
	
	// Should be identical
	for i := 0; i < 10; i++ {
		assert.Equal(t, derivatives[i], result[i])
	}
}

