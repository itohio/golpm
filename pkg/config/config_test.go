package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	assert.NotNil(t, cfg)
	assert.Equal(t, "COM3", cfg.Serial.Port)
	assert.Equal(t, float64(20000), cfg.VoltageDivider.R1)
	assert.Equal(t, float64(20000), cfg.VoltageDivider.R2)
	assert.Equal(t, float64(3.3), cfg.VoltageDivider.VRef)
	assert.Len(t, cfg.Heaters, 3)
	assert.Equal(t, float64(10), cfg.Measurement.WindowSeconds)
	assert.Equal(t, float64(0.001), cfg.Measurement.PulseThreshold)
	assert.Equal(t, 10*time.Second, cfg.Calibration.BaselineDuration)
	assert.Equal(t, 2*time.Second, cfg.Calibration.HeaterDuration)
	assert.Equal(t, 20*time.Second, cfg.Calibration.CooloffDuration)
	assert.Equal(t, []int{1, 2, 3}, cfg.Calibration.HeaterSequence)
	assert.Len(t, cfg.Calibration.Points, 1)
}

func TestLoad_FileNotExists(t *testing.T) {
	cfg, err := Load("nonexistent.yaml")
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, "COM3", cfg.Serial.Port)
}

func TestLoad_ValidYAML(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test_config_*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	yamlContent := `
serial:
  port: "/dev/ttyACM0"

voltage_divider:
  r1: 10000
  r2: 10000
  vref: 3.3

heaters:
  - resistance: 2000
  - resistance: 400
  - resistance: 150

measurement:
  window_seconds: 5
  pulse_threshold: 0.002

calibration:
  baseline_duration: 5s
  heater_duration: 1s
  cooloff_duration: 15s
  heater_sequence: [3, 2, 1]
  points:
    - slope: 0.0
      power: 0.0
    - slope: 0.001
      power: 10.0
`

	_, err = tmpfile.WriteString(yamlContent)
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())

	cfg, err := Load(tmpfile.Name())
	require.NoError(t, err)
	assert.NotNil(t, cfg)

	assert.Equal(t, "/dev/ttyACM0", cfg.Serial.Port)
	assert.Equal(t, float64(10000), cfg.VoltageDivider.R1)
	assert.Equal(t, float64(10000), cfg.VoltageDivider.R2)
	assert.Equal(t, float64(5), cfg.Measurement.WindowSeconds)
	assert.Equal(t, float64(0.002), cfg.Measurement.PulseThreshold)
	assert.Equal(t, 5*time.Second, cfg.Calibration.BaselineDuration)
	assert.Equal(t, 1*time.Second, cfg.Calibration.HeaterDuration)
	assert.Equal(t, 15*time.Second, cfg.Calibration.CooloffDuration)
	assert.Equal(t, []int{3, 2, 1}, cfg.Calibration.HeaterSequence)
	assert.Len(t, cfg.Calibration.Points, 2)
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test_config_*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.WriteString("invalid: yaml: content: [")
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())

	cfg, err := Load(tmpfile.Name())
	assert.Error(t, err)
	assert.Nil(t, cfg)
}

func TestLoad_PartialYAML(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test_config_*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	yamlContent := `
serial:
  port: "/dev/ttyACM0"
`

	_, err = tmpfile.WriteString(yamlContent)
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())

	cfg, err := Load(tmpfile.Name())
	require.NoError(t, err)
	assert.NotNil(t, cfg)

	// Should use defaults for missing fields
	assert.Equal(t, "/dev/ttyACM0", cfg.Serial.Port)
	assert.Equal(t, float64(20000), cfg.VoltageDivider.R1)      // default
	assert.Equal(t, float64(10), cfg.Measurement.WindowSeconds) // default
}

func TestSave(t *testing.T) {
	cfg := Default()
	cfg.Serial.Port = "/dev/ttyUSB0"
	cfg.Measurement.WindowSeconds = 15

	tmpfile, err := os.CreateTemp("", "test_save_*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	err = cfg.Save(tmpfile.Name())
	require.NoError(t, err)

	// Load it back and verify
	loaded, err := Load(tmpfile.Name())
	require.NoError(t, err)
	assert.Equal(t, "/dev/ttyUSB0", loaded.Serial.Port)
	assert.Equal(t, float64(15), loaded.Measurement.WindowSeconds)
}

func TestConfig_FieldAccess(t *testing.T) {
	cfg := Default()

	// Test field access
	assert.Equal(t, "COM3", cfg.Serial.Port)
	assert.Equal(t, float64(2300), cfg.Heaters[0].Resistance)
	assert.Equal(t, float64(500), cfg.Heaters[1].Resistance)
	assert.Equal(t, float64(200), cfg.Heaters[2].Resistance)
}
