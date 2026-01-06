package lpm

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/itohio/golpm/pkg/config"
)

// Mock simulates an LPM device for testing and development.
type Mock struct {
	cfg *config.MockConfig

	samples   chan RawSample
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	connected bool

	// Heater states
	heater1 bool
	heater2 bool
	heater3 bool

	// Simulation state
	startTime   time.Time
	lastLaserOn time.Time
	laserActive bool
	temperature float64 // Simulated temperature (V)
	voltage     float64 // Simulated voltage (V)
}

// Ensure MockedDevice implements DeviceInterface.
var _ Device = (*Mock)(nil)

// NewMock creates a new mocked device instance.
func NewMock(cfg *config.MockConfig) *Mock {
	if cfg == nil {
		cfg = &config.MockConfig{
			Bias:          0.0,
			NoiseLevel:    0.001,
			LaserPower:    40.0,
			LaserDuration: 2 * time.Second,
			LaserPeriod:   20 * time.Second,
			SampleRate:    100 * time.Millisecond,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Mock{
		cfg:       cfg,
		samples:   make(chan RawSample, DefaultBufferSize),
		ctx:       ctx,
		cancel:    cancel,
		connected: false,
	}
}

// Connect simulates connecting to the device.
func (m *Mock) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connected {
		return fmt.Errorf("already connected")
	}

	m.connected = true
	m.startTime = time.Now()
	m.lastLaserOn = m.startTime
	m.temperature = m.cfg.Bias
	m.voltage = 0.0

	// Start generating samples
	go m.generateSamples()

	return nil
}

// Close stops the mocked device.
func (m *Mock) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil
	}

	m.cancel()
	m.connected = false
	close(m.samples)

	return nil
}

// Samples returns the channel for reading samples.
func (m *Mock) Samples() <-chan RawSample {
	return m.samples
}

// SetHeaters sets the heater states (simulated).
func (m *Mock) SetHeaters(heater1, heater2, heater3 bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return fmt.Errorf("not connected")
	}

	m.heater1 = heater1
	m.heater2 = heater2
	m.heater3 = heater3

	return nil
}

// IsConnected returns whether the device is currently connected.
func (m *Mock) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// generateSamples generates simulated samples.
func (m *Mock) generateSamples() {
	ticker := time.NewTicker(m.cfg.SampleRate)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			sample := m.generateSample()
			select {
			case m.samples <- sample:
			case <-m.ctx.Done():
				return
			default:
				// Channel full, skip
			}
		}
	}
}

// generateSample generates a single simulated sample.
func (m *Mock) generateSample() RawSample {
	m.mu.RLock()
	now := time.Now()
	elapsed := now.Sub(m.startTime)
	laserElapsed := now.Sub(m.lastLaserOn)
	heater1 := m.heater1
	heater2 := m.heater2
	heater3 := m.heater3
	m.mu.RUnlock()

	// Check if laser should be on
	laserActive := false
	if laserElapsed >= m.cfg.LaserPeriod {
		// Time for next laser pulse
		laserActive = true
		m.mu.Lock()
		m.lastLaserOn = now
		m.laserActive = true
		m.mu.Unlock()
	} else if laserElapsed < m.cfg.LaserDuration {
		// Laser is still on
		laserActive = true
		m.mu.Lock()
		m.laserActive = true
		m.mu.Unlock()
	} else {
		m.mu.Lock()
		m.laserActive = false
		m.mu.Unlock()
	}

	// Simulate temperature response
	// Heating from laser or heaters
	heaterPower := m.calculateHeaterPower(heater1, heater2, heater3)
	laserPower := 0.0
	if laserActive {
		laserPower = m.cfg.LaserPower
	}

	// Thermal response: exponential approach to steady state
	// Simplified model: T = T0 + (P/k) * (1 - exp(-t/tau))
	// For simulation, use simpler linear ramp with thermal lag
	targetTemp := m.cfg.Bias + (heaterPower+laserPower)*0.001 // 0.001 V per mW
	thermalTimeConstant := 2.0                                // seconds

	// Update temperature with thermal lag
	dt := m.cfg.SampleRate.Seconds()
	alpha := dt / thermalTimeConstant
	m.temperature = m.temperature + alpha*(targetTemp-m.temperature)

	// Add noise
	noise := (math.Sin(float64(elapsed.Nanoseconds())*0.001) +
		math.Cos(float64(elapsed.Nanoseconds())*0.0013)) *
		m.cfg.NoiseLevel * 0.5
	m.temperature += noise

	// Simulate voltage (for heater power measurement)
	// Voltage increases when heaters are on
	if heater1 || heater2 || heater3 {
		m.voltage = math.Min(m.voltage+0.01, 2.5) // Ramp up to ~2.5V
	} else {
		m.voltage = math.Max(m.voltage-0.01, 0.0) // Ramp down
	}

	// Convert to ADC values (12-bit, 0-4095, 3.3V reference)
	readingVal := (m.temperature / 3.3) * 4095
	if readingVal < 0 {
		readingVal = 0
	} else if readingVal > 4095 {
		readingVal = 4095
	}
	readingADC := uint16(readingVal)

	voltageVal := (m.voltage / 3.3) * 4095
	if voltageVal < 0 {
		voltageVal = 0
	} else if voltageVal > 4095 {
		voltageVal = 4095
	}
	voltageADC := uint16(voltageVal)

	return RawSample{
		Timestamp: now,
		Reading:   readingADC,
		Voltage:   voltageADC,
		Heater1:   heater1,
		Heater2:   heater2,
		Heater3:   heater3,
	}
}

// calculateHeaterPower calculates simulated heater power based on heater states.
// This is a simplified model - in reality, power depends on voltage and resistance.
func (m *Mock) calculateHeaterPower(heater1, heater2, heater3 bool) float64 {
	power := 0.0
	// Simplified: assume each heater contributes fixed power when on
	if heater1 {
		power += 10.0 // ~10 mW
	}
	if heater2 {
		power += 50.0 // ~50 mW
	}
	if heater3 {
		power += 100.0 // ~100 mW
	}
	return power
}
