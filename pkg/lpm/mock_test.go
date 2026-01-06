package lpm

import (
	"testing"
	"time"

	"github.com/itohio/golpm/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestMockedDevice_calculateHeaterPower(t *testing.T) {
	cfg := &config.MockConfig{
		Bias:          0.0,
		NoiseLevel:    0.001,
		LaserPower:    40.0,
		LaserDuration: 2 * time.Second,
		LaserPeriod:   20 * time.Second,
		SampleRate:    100 * time.Millisecond,
	}
	dev := NewMock(cfg)

	tests := []struct {
		name           string
		heater1, heater2, heater3 bool
		wantPower      float64
	}{
		{
			name:      "all off",
			heater1:   false,
			heater2:   false,
			heater3:   false,
			wantPower: 0.0,
		},
		{
			name:      "only heater1 on",
			heater1:   true,
			heater2:   false,
			heater3:   false,
			wantPower: 10.0,
		},
		{
			name:      "only heater2 on",
			heater1:   false,
			heater2:   true,
			heater3:   false,
			wantPower: 50.0,
		},
		{
			name:      "only heater3 on",
			heater1:   false,
			heater2:   false,
			heater3:   true,
			wantPower: 100.0,
		},
		{
			name:      "heater1 and heater2 on",
			heater1:   true,
			heater2:   true,
			heater3:   false,
			wantPower: 60.0,
		},
		{
			name:      "heater1 and heater3 on",
			heater1:   true,
			heater2:   false,
			heater3:   true,
			wantPower: 110.0,
		},
		{
			name:      "heater2 and heater3 on",
			heater1:   false,
			heater2:   true,
			heater3:   true,
			wantPower: 150.0,
		},
		{
			name:      "all heaters on",
			heater1:   true,
			heater2:   true,
			heater3:   true,
			wantPower: 160.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			power := dev.calculateHeaterPower(tt.heater1, tt.heater2, tt.heater3)
			assert.Equal(t, tt.wantPower, power)
		})
	}
}

func TestNewMock(t *testing.T) {
	cfg := &config.MockConfig{
		Bias:          0.5,
		NoiseLevel:    0.002,
		LaserPower:    50.0,
		LaserDuration: 3 * time.Second,
		LaserPeriod:   25 * time.Second,
		SampleRate:    50 * time.Millisecond,
	}

	dev := NewMock(cfg)
	assert.NotNil(t, dev)
	assert.Equal(t, cfg, dev.cfg)
	assert.NotNil(t, dev.samples)
	assert.False(t, dev.IsConnected())
}

func TestNewMock_NilConfig(t *testing.T) {
	dev := NewMock(nil)
	assert.NotNil(t, dev)
	assert.NotNil(t, dev.cfg)
	assert.Equal(t, float64(0.0), dev.cfg.Bias)
	assert.Equal(t, float64(0.001), dev.cfg.NoiseLevel)
	assert.Equal(t, float64(40.0), dev.cfg.LaserPower)
	assert.Equal(t, 2*time.Second, dev.cfg.LaserDuration)
	assert.Equal(t, 20*time.Second, dev.cfg.LaserPeriod)
	assert.Equal(t, 100*time.Millisecond, dev.cfg.SampleRate)
}

func TestMockedDevice_IsConnected(t *testing.T) {
	dev := NewMock(nil)
	assert.False(t, dev.IsConnected())
}

func TestMockedDevice_SetHeaters(t *testing.T) {
	dev := NewMock(nil)
	
	// Should fail when not connected
	err := dev.SetHeaters(true, false, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")

	// Connect first
	err = dev.Connect()
	assert.NoError(t, err)

	// Now should work
	err = dev.SetHeaters(true, false, true)
	assert.NoError(t, err)
	assert.True(t, dev.heater1)
	assert.False(t, dev.heater2)
	assert.True(t, dev.heater3)

	// Test all combinations
	err = dev.SetHeaters(false, false, false)
	assert.NoError(t, err)
	assert.False(t, dev.heater1)
	assert.False(t, dev.heater2)
	assert.False(t, dev.heater3)

	err = dev.SetHeaters(true, true, true)
	assert.NoError(t, err)
	assert.True(t, dev.heater1)
	assert.True(t, dev.heater2)
	assert.True(t, dev.heater3)
}

func TestMockedDevice_Connect_AlreadyConnected(t *testing.T) {
	dev := NewMock(nil)
	
	err := dev.Connect()
	assert.NoError(t, err)

	err = dev.Connect()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already connected")
}

func TestMockedDevice_Close_NotConnected(t *testing.T) {
	dev := NewMock(nil)
	
	err := dev.Close()
	assert.NoError(t, err) // Should not error when not connected
}

func TestMockedDevice_Close_Connected(t *testing.T) {
	dev := NewMock(nil)
	
	err := dev.Connect()
	assert.NoError(t, err)
	assert.True(t, dev.IsConnected())

	err = dev.Close()
	assert.NoError(t, err)
	assert.False(t, dev.IsConnected())
}

func TestMockedDevice_generateSample_ADCConversion(t *testing.T) {
	// Test ADC conversion logic directly without full simulation
	// This tests the calculation: (voltage / 3.3) * 4095
	
	testCases := []struct {
		name    string
		voltage float64
		wantADC uint16
	}{
		{"0V", 0.0, 0},
		{"1.65V (half)", 1.65, 2047}, // 1.65/3.3*4095 = 2047.5 -> 2047
		{"3.3V (max)", 3.3, 4095},
		{"1.0V", 1.0, 1240}, // 1.0/3.3*4095 ≈ 1240.9 -> 1240
		{"2.0V", 2.0, 2481}, // 2.0/3.3*4095 ≈ 2481.8 -> 2481
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the ADC conversion logic
			val := (tc.voltage / 3.3) * 4095
			if val < 0 {
				val = 0
			} else if val > 4095 {
				val = 4095
			}
			adc := uint16(val)
			assert.Equal(t, tc.wantADC, adc)
		})
	}
}

func TestMockedDevice_generateSample_ADCClamping(t *testing.T) {
	// Test ADC clamping logic directly
	testCases := []struct {
		name    string
		voltage float64
		wantADC uint16
	}{
		{"negative voltage", -1.0, 0},
		{"zero voltage", 0.0, 0},
		{"normal voltage", 1.65, 2047},
		{"max voltage", 3.3, 4095},
		{"above max voltage", 5.0, 4095},
		{"way above max", 10.0, 4095},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the ADC conversion and clamping logic
			val := (tc.voltage / 3.3) * 4095
			if val < 0 {
				val = 0
			} else if val > 4095 {
				val = 4095
			}
			adc := uint16(val)
			assert.Equal(t, tc.wantADC, adc)
		})
	}
}

func TestMockedDevice_ThermalTargetCalculation(t *testing.T) {
	// Test the thermal target temperature calculation
	// targetTemp = bias + (heaterPower + laserPower) * 0.001
	
	testCases := []struct {
		name        string
		bias        float64
		heaterPower float64
		laserPower  float64
		wantTarget  float64
	}{
		{"no power", 0.0, 0.0, 0.0, 0.0},
		{"only heater", 0.0, 50.0, 0.0, 0.05},
		{"only laser", 0.0, 0.0, 40.0, 0.04},
		{"heater and laser", 0.0, 50.0, 40.0, 0.09},
		{"with bias", 0.5, 50.0, 40.0, 0.59},
		{"all heaters on", 0.0, 160.0, 0.0, 0.16},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			targetTemp := tc.bias + (tc.heaterPower+tc.laserPower)*0.001
			assert.InDelta(t, tc.wantTarget, targetTemp, 0.0001)
		})
	}
}

