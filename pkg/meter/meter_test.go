package meter

import (
	"testing"
	"time"

	"github.com/itohio/golpm/pkg/config"
	"github.com/itohio/golpm/pkg/sample"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	cfg := config.Default()
	m := New(cfg)

	assert.NotNil(t, m)
	assert.Equal(t, 0, len(m.Samples()))
	assert.Equal(t, 0, len(m.Derivatives()))
	assert.Equal(t, 0, len(m.Pulses()))
}

func TestProcessSample_Basic(t *testing.T) {
	cfg := config.Default()
	m := New(cfg)

	now := time.Now()
	s := sample.Sample{
		Timestamp:   now,
		Reading:     1.0,
		Voltage:     2.0,
		HeaterPower: 0.0,
	}

	m.processSample(s)

	samples := m.Samples()
	assert.Len(t, samples, 1)
	assert.Equal(t, s, samples[0])
	assert.Len(t, m.Derivatives(), 0) // Need at least 2 samples for derivatives
}

func TestProcessSample_Differentiation(t *testing.T) {
	cfg := config.Default()
	m := New(cfg)

	now := time.Now()
	s1 := sample.Sample{
		Timestamp:   now,
		Reading:     1.0,
		Voltage:     2.0,
		HeaterPower: 0.0,
	}
	s2 := sample.Sample{
		Timestamp:   now.Add(100 * time.Millisecond),
		Reading:     1.1, // 0.1V increase in 0.1s = 1.0 V/s
		Voltage:     2.0,
		HeaterPower: 0.0,
	}

	m.processSample(s1)
	m.processSample(s2)

	derivatives := m.Derivatives()
	assert.Len(t, derivatives, 1)
	assert.InDelta(t, 1.0, derivatives[0], 0.01) // 0.1V / 0.1s = 1.0 V/s
}

func TestProcessSample_WindowRemoval(t *testing.T) {
	cfg := config.Default()
	cfg.Measurement.WindowSeconds = 1.0 // 1 second window
	m := New(cfg)

	now := time.Now()
	s1 := sample.Sample{
		Timestamp:   now,
		Reading:     1.0,
		Voltage:     2.0,
		HeaterPower: 0.0,
	}
	s2 := sample.Sample{
		Timestamp:   now.Add(500 * time.Millisecond),
		Reading:     1.1,
		Voltage:     2.0,
		HeaterPower: 0.0,
	}
	s3 := sample.Sample{
		Timestamp:   now.Add(1500 * time.Millisecond), // Outside window
		Reading:     1.2,
		Voltage:     2.0,
		HeaterPower: 0.0,
	}

	m.processSample(s1)
	m.processSample(s2)
	m.processSample(s3)

	samples := m.Samples()
	// s1 should be removed (outside window from s3's perspective)
	assert.LessOrEqual(t, len(samples), 2)
}

func TestProcessSample_PulseDetection(t *testing.T) {
	cfg := config.Default()
	cfg.Measurement.PulseThreshold = 0.5 // 0.5 V/s threshold
	cfg.Measurement.MinPulseDuration = 0.1 // Lower threshold for test (0.1s)
	m := New(cfg)

	now := time.Now()
	dt := 100 * time.Millisecond

	// Create samples with increasing reading (heating) - 12 samples = 1.2s pulse
	samples := []sample.Sample{
		{Timestamp: now, Reading: 1.0, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(dt), Reading: 1.05, Voltage: 2.0, HeaterPower: 0.0}, // 0.5 V/s
		{Timestamp: now.Add(2 * dt), Reading: 1.1, Voltage: 2.0, HeaterPower: 0.0},  // 0.5 V/s
		{Timestamp: now.Add(3 * dt), Reading: 1.15, Voltage: 2.0, HeaterPower: 0.0}, // 0.5 V/s
		{Timestamp: now.Add(4 * dt), Reading: 1.2, Voltage: 2.0, HeaterPower: 0.0},  // 0.5 V/s
		{Timestamp: now.Add(5 * dt), Reading: 1.25, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(6 * dt), Reading: 1.3, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(7 * dt), Reading: 1.35, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(8 * dt), Reading: 1.4, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(9 * dt), Reading: 1.45, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(10 * dt), Reading: 1.5, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(11 * dt), Reading: 1.55, Voltage: 2.0, HeaterPower: 0.0},
	}

	for _, s := range samples {
		m.processSample(s)
	}

	pulses := m.Pulses()
	assert.Greater(t, len(pulses), 0, "Should detect at least one pulse")

	if len(pulses) > 0 {
		pulse := pulses[0]
		assert.GreaterOrEqual(t, pulse.StartIndex, 0)
		assert.Less(t, pulse.StartIndex, len(m.Samples()))
		assert.GreaterOrEqual(t, pulse.EndIndex, pulse.StartIndex)
		assert.Greater(t, pulse.RawValue, cfg.Measurement.PulseThreshold)
	}
}

func TestProcessSample_PulseDetection_BelowThreshold(t *testing.T) {
	cfg := config.Default()
	cfg.Measurement.PulseThreshold = 0.5 // 0.5 V/s threshold
	m := New(cfg)

	now := time.Now()
	dt := 100 * time.Millisecond

	// Create samples with slow increase (below threshold)
	samples := []sample.Sample{
		{Timestamp: now, Reading: 1.0, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(dt), Reading: 1.01, Voltage: 2.0, HeaterPower: 0.0}, // 0.1 V/s
		{Timestamp: now.Add(2 * dt), Reading: 1.02, Voltage: 2.0, HeaterPower: 0.0}, // 0.1 V/s
	}

	for _, s := range samples {
		m.processSample(s)
	}

	pulses := m.Pulses()
	assert.Equal(t, 0, len(pulses), "Should not detect pulses below threshold")
}

func TestProcessSample_MultiplePulses(t *testing.T) {
	cfg := config.Default()
	cfg.Measurement.PulseThreshold = 0.5
	cfg.Measurement.MinPulseDuration = 0.1 // Lower threshold for test
	cfg.Measurement.WindowSeconds = 10.0 // Large window
	m := New(cfg)

	now := time.Now()
	dt := 100 * time.Millisecond

	// First pulse: heating (12 samples = 1.2s, above 0.1s minimum)
	samples1 := []sample.Sample{
		{Timestamp: now, Reading: 1.0, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(dt), Reading: 1.05, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(2 * dt), Reading: 1.1, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(3 * dt), Reading: 1.15, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(4 * dt), Reading: 1.2, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(5 * dt), Reading: 1.25, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(6 * dt), Reading: 1.3, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(7 * dt), Reading: 1.35, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(8 * dt), Reading: 1.4, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(9 * dt), Reading: 1.45, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(10 * dt), Reading: 1.5, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(11 * dt), Reading: 1.55, Voltage: 2.0, HeaterPower: 0.0},
	}

	// Cooling phase (below threshold)
	samples2 := []sample.Sample{
		{Timestamp: now.Add(12 * dt), Reading: 1.55, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(13 * dt), Reading: 1.54, Voltage: 2.0, HeaterPower: 0.0},
	}

	// Second pulse: heating again (12 samples = 1.2s)
	samples3 := []sample.Sample{
		{Timestamp: now.Add(14 * dt), Reading: 1.54, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(15 * dt), Reading: 1.59, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(16 * dt), Reading: 1.64, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(17 * dt), Reading: 1.69, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(18 * dt), Reading: 1.74, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(19 * dt), Reading: 1.79, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(20 * dt), Reading: 1.84, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(21 * dt), Reading: 1.89, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(22 * dt), Reading: 1.94, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(23 * dt), Reading: 1.99, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(24 * dt), Reading: 2.04, Voltage: 2.0, HeaterPower: 0.0},
		{Timestamp: now.Add(25 * dt), Reading: 2.09, Voltage: 2.0, HeaterPower: 0.0},
	}

	allSamples := append(append(samples1, samples2...), samples3...)
	for _, s := range allSamples {
		m.processSample(s)
	}

	pulses := m.Pulses()
	assert.GreaterOrEqual(t, len(pulses), 1, "Should detect at least one pulse")
	// May detect 1 or 2 pulses depending on detection logic
}

func TestOnUpdate(t *testing.T) {
	cfg := config.Default()
	m := New(cfg)

	callbackCalled := false
	var receivedSamples []sample.Sample
	var receivedDerivatives []float64
	var receivedPulses []Pulse

	m.OnUpdate(func(samples []sample.Sample, derivatives []float64, pulses []Pulse) {
		callbackCalled = true
		receivedSamples = samples
		receivedDerivatives = derivatives
		receivedPulses = pulses
	})

	now := time.Now()
	s := sample.Sample{
		Timestamp:   now,
		Reading:     1.0,
		Voltage:    2.0,
		HeaterPower: 0.0,
	}

	m.processSample(s)

	assert.True(t, callbackCalled, "Callback should be called when sample is processed")
	assert.NotNil(t, receivedSamples, "Callback should receive samples")
	assert.NotNil(t, receivedDerivatives, "Callback should receive derivatives")
	assert.NotNil(t, receivedPulses, "Callback should receive pulses")
}

func TestSamples_ThreadSafe(t *testing.T) {
	cfg := config.Default()
	m := New(cfg)

	// Add samples in goroutine
	done := make(chan bool)
	go func() {
		now := time.Now()
		for i := 0; i < 100; i++ {
			s := sample.Sample{
				Timestamp:   now.Add(time.Duration(i) * time.Millisecond),
				Reading:     float64(1.0 + float64(i)*0.01),
				Voltage:     2.0,
				HeaterPower: 0.0,
			}
			m.processSample(s)
		}
		done <- true
	}()

	// Read samples concurrently
	for {
		select {
		case <-done:
			return
		default:
			samples := m.Samples()
			_ = samples // Just reading, should not panic
		}
	}
}

func TestDerivatives_Count(t *testing.T) {
	cfg := config.Default()
	m := New(cfg)

	now := time.Now()
	for i := 0; i < 5; i++ {
		s := sample.Sample{
			Timestamp:   now.Add(time.Duration(i) * 100 * time.Millisecond),
			Reading:     float64(1.0 + float64(i)*0.1),
			Voltage:    2.0,
			HeaterPower: 0.0,
		}
		m.processSample(s)
	}

	// Should have n-1 derivatives for n samples
	samples := m.Samples()
	derivatives := m.Derivatives()
	assert.Equal(t, len(samples)-1, len(derivatives), "Should have n-1 derivatives for n samples")
}

func TestPulses_IndicesValid(t *testing.T) {
	cfg := config.Default()
	cfg.Measurement.PulseThreshold = 0.5
	cfg.Measurement.WindowSeconds = 5.0
	m := New(cfg)

	now := time.Now()
	dt := 100 * time.Millisecond

	// Create a pulse
	for i := 0; i < 10; i++ {
		s := sample.Sample{
			Timestamp:   now.Add(time.Duration(i) * dt),
			Reading:     float64(1.0 + float64(i)*0.06), // 0.6 V/s
			Voltage:    2.0,
			HeaterPower: 0.0,
		}
		m.processSample(s)
	}

	pulses := m.Pulses()
	samples := m.Samples()

	for _, pulse := range pulses {
		assert.GreaterOrEqual(t, pulse.StartIndex, 0, "StartIndex should be valid")
		assert.Less(t, pulse.StartIndex, len(samples), "StartIndex should be within bounds")
		assert.GreaterOrEqual(t, pulse.EndIndex, pulse.StartIndex, "EndIndex should be >= StartIndex")
		assert.Less(t, pulse.EndIndex, len(samples), "EndIndex should be within bounds")
	}
}

func TestProcessSamples_Channel(t *testing.T) {
	cfg := config.Default()
	m := New(cfg)

	input := make(chan sample.Sample, 10)
	go m.ProcessSamples(input)

	now := time.Now()
	for i := 0; i < 5; i++ {
		s := sample.Sample{
			Timestamp:   now.Add(time.Duration(i) * 100 * time.Millisecond),
			Reading:     float64(1.0 + float64(i)*0.1),
			Voltage:    2.0,
			HeaterPower: 0.0,
		}
		input <- s
	}

	close(input)

	// Wait a bit for processing
	time.Sleep(50 * time.Millisecond)

	samples := m.Samples()
	assert.Equal(t, 5, len(samples), "Should process all samples from channel")
}

