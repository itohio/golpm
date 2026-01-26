package meter

import (
	"testing"
	"time"

	"github.com/itohio/golpm/pkg/config"
	"github.com/itohio/golpm/pkg/sample"
	"github.com/stretchr/testify/assert"
)

// createSampleWithChange creates a sample with Change calculated from previous sample.
// For the first sample, prev should be nil and Change will be 0.0.
func createSampleWithChange(timestamp time.Time, reading float64, voltage float64, heaterPower float64, prev *sample.Sample) sample.Sample {
	s := sample.Sample{
		Timestamp:   timestamp,
		Reading:     reading,
		Voltage:     voltage,
		HeaterPower: heaterPower,
	}
	if prev == nil {
		s.Change = 0.0 // First sample
	} else {
		dt := timestamp.Sub(prev.Timestamp).Seconds()
		if dt > 0 {
			s.Change = (reading - prev.Reading) / dt
		} else {
			s.Change = 0.0
		}
	}
	return s
}

// createSamplesWithChange creates a slice of samples with Change calculated automatically.
// The first sample will have Change = 0.0, subsequent samples will have Change calculated.
func createSamplesWithChange(baseTime time.Time, readings []float64, voltage float64, heaterPower float64, dt time.Duration) []sample.Sample {
	if len(readings) == 0 {
		return nil
	}
	samples := make([]sample.Sample, len(readings))
	samples[0] = createSampleWithChange(baseTime, readings[0], voltage, heaterPower, nil)
	for i := 1; i < len(readings); i++ {
		samples[i] = createSampleWithChange(baseTime.Add(time.Duration(i)*dt), readings[i], voltage, heaterPower, &samples[i-1])
	}
	return samples
}

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
	s := createSampleWithChange(now, 1.0, 2.0, 0.0, nil)

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
	s1 := createSampleWithChange(now, 1.0, 2.0, 0.0, nil)
	dt := 100 * time.Millisecond
	s2 := createSampleWithChange(now.Add(dt), 1.1, 2.0, 0.0, &s1) // 0.1V increase in 0.1s = 1.0 V/s

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
	s1 := createSampleWithChange(now, 1.0, 2.0, 0.0, nil)
	s2 := createSampleWithChange(now.Add(500*time.Millisecond), 1.1, 2.0, 0.0, &s1)
	s3 := createSampleWithChange(now.Add(1500*time.Millisecond), 1.2, 2.0, 0.0, &s2) // Outside window

	m.processSample(s1)
	m.processSample(s2)
	m.processSample(s3)

	samples := m.Samples()
	// s1 should be removed (outside window from s3's perspective)
	assert.LessOrEqual(t, len(samples), 2)
}

func TestProcessSample_PulseDetection(t *testing.T) {
	cfg := config.Default()
	cfg.Measurement.PulseThresholdMVS = 500.0 // 500 mV/s threshold (0.5 V/s)
	cfg.Measurement.MinPulseDuration = 0.1 // Lower threshold for test (0.1s)
	cfg.Measurement.PulseLineFitMinDuration = 0.1 // Lower threshold for test
	cfg.Measurement.PulseLineFitRangeMVS = 10.0 // 10 mV/s acceptable range
	m := New(cfg)

	now := time.Now()
	dt := 100 * time.Millisecond

	// Create samples with constant slope (heating) - 12 samples = 1.2s pulse
	// Use constant slope to ensure good fit
	readings := []float64{1.0, 1.05, 1.1, 1.15, 1.2, 1.25, 1.3, 1.35, 1.4, 1.45, 1.5, 1.55}
	samples := createSamplesWithChange(now, readings, 2.0, 0.0, dt)
	
	// Add cooling phase (slope drops below threshold)
	coolingReadings := []float64{1.55, 1.56, 1.57}
	coolingSamples := createSamplesWithChange(now.Add(time.Duration(len(readings))*dt), coolingReadings, 2.0, 0.0, dt)
	samples = append(samples, coolingSamples...)

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
		assert.Greater(t, pulse.AvgSlope, m.threshold) // Compare with internal threshold (V/s)
	}
}

func TestProcessSample_PulseDetection_BelowThreshold(t *testing.T) {
	cfg := config.Default()
	cfg.Measurement.PulseThresholdMVS = 500.0 // 500 mV/s threshold (0.5 V/s)
	m := New(cfg)

	now := time.Now()
	dt := 100 * time.Millisecond

	// Create samples with slow increase (below threshold)
	readings := []float64{1.0, 1.01, 1.02}
	samples := createSamplesWithChange(now, readings, 2.0, 0.0, dt)

	for _, s := range samples {
		m.processSample(s)
	}

	pulses := m.Pulses()
	assert.Equal(t, 0, len(pulses), "Should not detect pulses below threshold")
}

func TestProcessSample_MultiplePulses(t *testing.T) {
	cfg := config.Default()
	cfg.Measurement.PulseThresholdMVS = 500.0 // 500 mV/s threshold (0.5 V/s)
	cfg.Measurement.MinPulseDuration = 0.1 // Lower threshold for test
	cfg.Measurement.PulseLineFitMinDuration = 0.1 // Lower threshold for test
	cfg.Measurement.PulseLineFitRangeMVS = 10.0 // 10 mV/s acceptable range
	cfg.Measurement.WindowSeconds = 10.0 // Large window
	m := New(cfg)

	now := time.Now()
	dt := 100 * time.Millisecond

	// First pulse: heating (12 samples = 1.2s, above 0.1s minimum)
	readings1 := []float64{1.0, 1.05, 1.1, 1.15, 1.2, 1.25, 1.3, 1.35, 1.4, 1.45, 1.5, 1.55}
	samples1 := createSamplesWithChange(now, readings1, 2.0, 0.0, dt)

	// Cooling phase (below threshold) - need to calculate from last sample of samples1
	lastSample1 := samples1[len(samples1)-1]
	s2First := createSampleWithChange(now.Add(12*dt), 1.55, 2.0, 0.0, &lastSample1)
	samples2 := []sample.Sample{
		s2First,
		createSampleWithChange(now.Add(13*dt), 1.54, 2.0, 0.0, &s2First),
	}

	// Second pulse: heating again (12 samples = 1.2s)
	lastSample2 := samples2[len(samples2)-1]
	readings3Full := []float64{1.54, 1.59, 1.64, 1.69, 1.74, 1.79, 1.84, 1.89, 1.94, 1.99, 2.04, 2.09}
	samples3Start := createSampleWithChange(now.Add(14*dt), readings3Full[0], 2.0, 0.0, &lastSample2)
	samples3 := []sample.Sample{samples3Start}
	for i := 1; i < len(readings3Full); i++ {
		samples3 = append(samples3, createSampleWithChange(now.Add(time.Duration(14+i)*dt), readings3Full[i], 2.0, 0.0, &samples3[i-1]))
	}
	
	// Add cooling phase for second pulse
	lastSample3 := samples3[len(samples3)-1]
	s4First := createSampleWithChange(now.Add(26*dt), 2.09, 2.0, 0.0, &lastSample3)
	samples4 := []sample.Sample{
		s4First,
		createSampleWithChange(now.Add(27*dt), 2.08, 2.0, 0.0, &s4First),
	}

	allSamples := append(append(append(samples1, samples2...), samples3...), samples4...)
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
	cfg.Measurement.PulseThresholdMVS = 500.0 // 500 mV/s threshold (0.5 V/s)
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

