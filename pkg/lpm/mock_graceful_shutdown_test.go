package lpm

import (
	"testing"
	"time"

	"github.com/itohio/golpm/pkg/config"
	"github.com/stretchr/testify/assert"
)

// TestMock_GracefulShutdown tests that Mock device closes samples channel
// when Close() is called.
func TestMock_GracefulShutdown(t *testing.T) {
	cfg := &config.MockConfig{
		Bias:          0.0,
		NoiseLevel:    0.001,
		LaserPower:    40.0,
		LaserDuration: 2 * time.Second,
		LaserPeriod:   20 * time.Second,
		SampleRate:    100 * time.Millisecond,
	}

	mock := NewMock(cfg)
	err := mock.Connect()
	assert.NoError(t, err)

	samples := mock.Samples()

	// Read a few samples
	received := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range samples {
			received++
			if received >= 3 {
				// Got enough samples, now close device
				mock.Close()
			}
		}
	}()

	// Wait for samples and channel closure
	select {
	case <-done:
		// Channel closed successfully
	case <-time.After(5 * time.Second):
		t.Fatal("Samples channel did not close within timeout")
	}

	// Should have received at least a few samples
	assert.GreaterOrEqual(t, received, 3, "Should receive samples before channel closes")

	// Verify channel is closed
	_, ok := <-samples
	assert.False(t, ok, "Channel should be closed")
}

