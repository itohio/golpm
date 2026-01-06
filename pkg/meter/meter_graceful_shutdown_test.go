package meter

import (
	"sync"
	"testing"
	"time"

	"github.com/itohio/golpm/pkg/config"
	"github.com/itohio/golpm/pkg/sample"
	"github.com/stretchr/testify/assert"
)

// TestMeter_GracefulShutdown_NoCallbacksAfterClose tests that meter stops sending
// callbacks after the input channel is closed.
func TestMeter_GracefulShutdown_NoCallbacksAfterClose(t *testing.T) {
	cfg := &config.Config{
		Measurement: config.MeasurementConfig{
			WindowSeconds:    10.0,
			PulseThreshold:   0.001,
			MinPulseDuration: 1.0,
		},
	}

	m := New(cfg)

	callbackCount := 0
	callbackReceived := make(chan struct{}, 10)

	m.OnUpdate(func(samples []sample.Sample, derivatives []float64, pulses []Pulse) {
		callbackCount++
		select {
		case callbackReceived <- struct{}{}:
		default:
		}
	})

	// Create input channel and send some samples
	input := make(chan sample.Sample, 10)
	go m.ProcessSamples(input)

	// Send a few samples
	now := time.Now()
	for i := 0; i < 3; i++ {
		input <- sample.Sample{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Reading:   float64(i) * 0.1,
		}
	}

	// Wait for callbacks
	time.Sleep(100 * time.Millisecond)
	initialCount := callbackCount

	// Close the channel
	close(input)

	// Wait for ProcessSamples to finish
	time.Sleep(200 * time.Millisecond)

	// Send one more sample through a new channel (should not trigger callback)
	newInput := make(chan sample.Sample, 1)
	newInput <- sample.Sample{
		Timestamp: time.Now(),
		Reading:   1.0,
	}
	close(newInput)

	// Process should not trigger callbacks since shutdown flag is set
	time.Sleep(100 * time.Millisecond)

	// Callback count should not have increased after channel close
	assert.Equal(t, initialCount, callbackCount, "No callbacks should be sent after channel closes")
}

// TestMeter_ResetShutdown tests that ResetShutdown allows callbacks again.
func TestMeter_ResetShutdown(t *testing.T) {
	cfg := &config.Config{
		Measurement: config.MeasurementConfig{
			WindowSeconds:    10.0,
			PulseThreshold:   0.001,
			MinPulseDuration: 1.0,
		},
	}

	m := New(cfg)

	callbackCount := 0
	callbackMu := &sync.Mutex{}
	m.OnUpdate(func(samples []sample.Sample, derivatives []float64, pulses []Pulse) {
		callbackMu.Lock()
		callbackCount++
		callbackMu.Unlock()
	})

	// First chain - send and close
	input1 := make(chan sample.Sample, 10)
	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		m.ProcessSamples(input1)
	}()
	
	// Send sample with enough time difference to create a derivative
	now := time.Now()
	input1 <- sample.Sample{Timestamp: now, Reading: 0.1}
	time.Sleep(100 * time.Millisecond)
	input1 <- sample.Sample{Timestamp: now.Add(100 * time.Millisecond), Reading: 0.2}
	time.Sleep(50 * time.Millisecond)
	
	// Close input and wait for ProcessSamples to finish
	// This ensures the goroutine has exited and shutdown flag is set
	close(input1)
	select {
	case <-done1:
		// ProcessSamples finished - shutdown flag should now be set
	case <-time.After(2 * time.Second):
		t.Fatal("First ProcessSamples did not finish within timeout")
	}

	callbackMu.Lock()
	count1 := callbackCount
	callbackMu.Unlock()

	// Reset shutdown flag (now safe since first goroutine is done and shutdown is set)
	m.ResetShutdown()

	// Second chain - should work again
	input2 := make(chan sample.Sample, 10)
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		m.ProcessSamples(input2)
	}()
	
	// Send sample with enough time difference to create a derivative
	now2 := time.Now()
	input2 <- sample.Sample{Timestamp: now2, Reading: 0.3}
	time.Sleep(100 * time.Millisecond)
	input2 <- sample.Sample{Timestamp: now2.Add(100 * time.Millisecond), Reading: 0.4}
	time.Sleep(50 * time.Millisecond)
	
	// Close input and wait for ProcessSamples to finish
	close(input2)
	select {
	case <-done2:
		// ProcessSamples finished
	case <-time.After(2 * time.Second):
		t.Fatal("Second ProcessSamples did not finish within timeout")
	}

	callbackMu.Lock()
	count2 := callbackCount
	callbackMu.Unlock()

	// Should have received more callbacks after reset
	assert.Greater(t, count2, count1, "Callbacks should resume after ResetShutdown")
}

