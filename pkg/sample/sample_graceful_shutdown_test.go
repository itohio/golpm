package sample

import (
	"testing"
	"time"

	"github.com/itohio/golpm/pkg/config"
	"github.com/itohio/golpm/pkg/lpm"
	"github.com/stretchr/testify/assert"
)

// TestConverter_GracefulShutdown tests that converter closes output channel
// when input channel is closed.
func TestConverter_GracefulShutdown(t *testing.T) {
	cfg := &config.Config{
		VoltageDivider: config.VoltageDividerConfig{
			R1:   20000,
			R2:   20000,
			VRef: 3.3,
		},
		Heaters: []config.HeaterConfig{
			{Resistance: 1000},
			{Resistance: 500},
			{Resistance: 200},
		},
	}

	converter := NewConverter(cfg, 10)
	input := make(chan lpm.RawSample, 10)
	output := converter(input)

	// Read samples in background
	received := make(chan int, 1)
	done := make(chan struct{})
	sampleReceived := make(chan struct{}, 10) // Signal each received sample
	go func() {
		defer close(done)
		count := 0
		for range output {
			count++
			select {
			case sampleReceived <- struct{}{}:
			default:
			}
		}
		received <- count
	}()

	// Send some samples
	now := time.Now()
	numSamples := 3
	for i := 0; i < numSamples; i++ {
		input <- lpm.RawSample{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Reading:   2048,
			Voltage:   1024,
		}
	}

	// Wait for samples to be received (with timeout to avoid hanging)
	// The reader goroutine will signal via sampleReceived channel
	timeout := time.After(500 * time.Millisecond)
	receivedCount := 0
	for receivedCount < numSamples {
		select {
		case <-sampleReceived:
			receivedCount++
		case <-timeout:
			// Timeout - proceed anyway, converter should still close properly
			// This ensures the test doesn't hang if there's a timing issue
			break
		}
	}

	// Close input channel - this should cause converter to close output
	close(input)

	// Wait for output channel to close (reader goroutine to finish)
	select {
	case <-done:
		// Output channel closed successfully
	case <-time.After(2 * time.Second):
		t.Fatal("Output channel did not close within timeout")
	}

	// Check how many samples were received
	select {
	case count := <-received:
		assert.Equal(t, numSamples, count, "Should receive all samples before channel closes")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Did not receive sample count")
	}
}

// TestAveragingConverter_GracefulShutdown tests that averaging converter
// closes output channel when input channel is closed.
func TestAveragingConverter_GracefulShutdown(t *testing.T) {
	converter := NewAveragingConverterForSamples(3, 10)
	input := make(chan Sample, 10)
	output := converter(input)

	// Read samples in background
	received := make(chan int, 1)
	done := make(chan struct{})
	sampleReceived := make(chan struct{}, 10) // Signal each received sample
	go func() {
		defer close(done)
		count := 0
		for range output {
			count++
			select {
			case sampleReceived <- struct{}{}:
			default:
			}
		}
		received <- count
	}()

	// Send some samples
	now := time.Now()
	numSamples := 5
	for i := 0; i < numSamples; i++ {
		input <- Sample{
			Timestamp: now.Add(time.Duration(i) * 100 * time.Millisecond),
			Reading:   float64(i) * 0.1,
		}
	}

	// Wait for at least one averaged sample to be produced (ticker fires every 100ms)
	// Wait a bit longer to ensure ticker has fired at least once
	timeout := time.After(250 * time.Millisecond)
	select {
	case <-sampleReceived:
		// At least one averaged sample received, good to proceed
	case <-timeout:
		// Timeout - proceed anyway, converter should still close properly
	}

	// Close input channel - converter should flush buffer and close output
	close(input)

	// Wait for output channel to close (reader goroutine to finish)
	select {
	case <-done:
		// Output channel closed successfully
	case <-time.After(2 * time.Second):
		t.Fatal("Output channel did not close within timeout")
	}

	// Check how many averaged samples were received
	// Ticker fires every 100ms, so in 250ms we might get 2-3 ticks
	// Plus one more when input closes (buffer flush)
	// So we could get 3-4 samples total, but the important thing is channel closes properly
	select {
	case count := <-received:
		assert.Greater(t, count, 0, "Should receive at least one averaged sample")
		// Allow for multiple ticker fires + flush on close
		assert.LessOrEqual(t, count, 5, "Should receive reasonable number of averaged samples")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Did not receive sample count")
	}
}
