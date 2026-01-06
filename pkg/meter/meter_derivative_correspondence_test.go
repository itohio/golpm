package meter

import (
	"testing"
	"time"

	"github.com/itohio/golpm/pkg/config"
	"github.com/itohio/golpm/pkg/sample"
	"github.com/stretchr/testify/assert"
)

// TestDerivativeCorrespondence verifies that derivatives correspond exactly to sample pairs.
// derivative[i] = (sample[i+1] - sample[i]) / dt
func TestDerivativeCorrespondence(t *testing.T) {
	cfg := config.Default()
	m := New(cfg)

	now := time.Now()
	dt := 100 * time.Millisecond

	// Create samples with known values
	samples := []sample.Sample{
		{Timestamp: now, Reading: 1.0, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(dt), Reading: 1.1, Voltage: 2.0, HeaterPower: 0.0}, // +0.1V in 0.1s = 1.0 V/s
		{Timestamp: now.Add(2 * dt), Reading: 1.2, Voltage: 2.0, HeaterPower: 0.0}, // +0.1V in 0.1s = 1.0 V/s
		{Timestamp: now.Add(3 * dt), Reading: 1.3, Voltage: 2.0, HeaterPower: 0.0}, // +0.1V in 0.1s = 1.0 V/s
	}

	for _, s := range samples {
		m.processSample(s)
	}

	// Verify we have n-1 derivatives for n samples
	resultSamples := m.Samples()
	resultDerivatives := m.Derivatives()
	assert.Equal(t, len(resultSamples)-1, len(resultDerivatives), "Should have n-1 derivatives for n samples")

	// Verify derivative values correspond to sample pairs
	// derivative[0] should be (sample[1] - sample[0]) / dt
	expectedDeriv0 := (resultSamples[1].Reading - resultSamples[0].Reading) / resultSamples[1].Timestamp.Sub(resultSamples[0].Timestamp).Seconds()
	assert.InDelta(t, expectedDeriv0, resultDerivatives[0], 0.01, "derivative[0] should correspond to (sample[1]-sample[0])/dt")

	// derivative[1] should be (sample[2] - sample[1]) / dt
	expectedDeriv1 := (resultSamples[2].Reading - resultSamples[1].Reading) / resultSamples[2].Timestamp.Sub(resultSamples[1].Timestamp).Seconds()
	assert.InDelta(t, expectedDeriv1, resultDerivatives[1], 0.01, "derivative[1] should correspond to (sample[2]-sample[1])/dt")
}

// TestTimestampBasedRemoval verifies that samples are removed based on timestamp, not count.
func TestTimestampBasedRemoval(t *testing.T) {
	cfg := config.Default()
	cfg.Measurement.WindowSeconds = 1.0 // 1 second window
	m := New(cfg)

	now := time.Now()

	// Add samples at different times
	// Sample at t=0s (will be removed when we add sample at t=1.5s)
	s1 := sample.Sample{Timestamp: now, Reading: 1.0, Voltage: 2.0, HeaterPower: 0.0}
	m.processSample(s1)

	// Sample at t=0.5s (will be kept when we add sample at t=1.5s)
	s2 := sample.Sample{Timestamp: now.Add(500 * time.Millisecond), Reading: 1.1, Voltage: 2.0, HeaterPower: 0.0}
	m.processSample(s2)

	// Sample at t=1.5s (outside window from s1's perspective, but within window from s2's)
	s3 := sample.Sample{Timestamp: now.Add(1500 * time.Millisecond), Reading: 1.2, Voltage: 2.0, HeaterPower: 0.0}
	m.processSample(s3)

	// Verify s1 was removed (outside 1s window from s3)
	resultSamples := m.Samples()
	assert.LessOrEqual(t, len(resultSamples), 2, "Should remove samples outside time window")
	
	// Verify s2 and s3 are still present
	if len(resultSamples) >= 2 {
		assert.True(t, resultSamples[0].Timestamp.Equal(s2.Timestamp) || resultSamples[0].Timestamp.After(s2.Timestamp), "First sample should be s2 or later")
	}

	// Verify derivatives correspond correctly after removal
	resultDerivatives := m.Derivatives()
	assert.Equal(t, len(resultSamples)-1, len(resultDerivatives), "Derivatives should still correspond exactly after timestamp-based removal")
}

// TestDerivativeCorrespondenceAfterRemoval verifies derivatives remain correct after sample removal.
func TestDerivativeCorrespondenceAfterRemoval(t *testing.T) {
	cfg := config.Default()
	cfg.Measurement.WindowSeconds = 2.0 // 2 second window
	m := New(cfg)

	now := time.Now()
	dt := 200 * time.Millisecond

	// Create 5 samples
	for i := 0; i < 5; i++ {
		s := sample.Sample{
			Timestamp:   now.Add(time.Duration(i) * dt),
			Reading:     1.0 + float64(i)*0.1,
			Voltage:     2.0,
			HeaterPower: 0.0,
		}
		m.processSample(s)
	}

	// Verify initial correspondence: 5 samples = 4 derivatives
	samples1 := m.Samples()
	derivatives1 := m.Derivatives()
	assert.Equal(t, 5, len(samples1))
	assert.Equal(t, 4, len(derivatives1), "Should have 4 derivatives for 5 samples")

	// Add a sample that will cause removal of first 2 samples (outside 2s window)
	// First sample is at t=0, new sample at t=2.5s, so samples before t=0.5s are removed
	s6 := sample.Sample{
		Timestamp:   now.Add(2500 * time.Millisecond),
		Reading:     1.5,
		Voltage:     2.0,
		HeaterPower: 0.0,
	}
	m.processSample(s6)

	// Verify samples were removed based on timestamp
	samples2 := m.Samples()
	derivatives2 := m.Derivatives()
	
	// Should have fewer samples now
	assert.Less(t, len(samples2), len(samples1), "Should have removed some samples")
	
	// Derivatives should still correspond exactly: n samples = n-1 derivatives
	assert.Equal(t, len(samples2)-1, len(derivatives2), "Derivatives should still correspond exactly after removal")
	
	// Verify derivative values still correspond to correct sample pairs
	if len(derivatives2) > 0 && len(samples2) > 1 {
		expectedDeriv := (samples2[1].Reading - samples2[0].Reading) / samples2[1].Timestamp.Sub(samples2[0].Timestamp).Seconds()
		assert.InDelta(t, expectedDeriv, derivatives2[0], 0.01, "First derivative should correspond to first sample pair after removal")
	}
}

