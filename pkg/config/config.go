package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration.
type Config struct {
	Serial         SerialConfig         `yaml:"serial"`
	VoltageDivider VoltageDividerConfig `yaml:"voltage_divider"`
	Heaters        []HeaterConfig       `yaml:"heaters"`
	Measurement    MeasurementConfig    `yaml:"measurement"`
	Calibration    CalibrationConfig    `yaml:"calibration"`
	Mock           MockConfig           `yaml:"mock"`
}

// SerialConfig contains serial port configuration.
type SerialConfig struct {
	Port string `yaml:"port"`
}

// VoltageDividerConfig contains voltage divider configuration.
type VoltageDividerConfig struct {
	R1   float64 `yaml:"r1"`
	R2   float64 `yaml:"r2"`
	VRef float64 `yaml:"vref"`
}

// HeaterConfig contains heater resistance configuration.
type HeaterConfig struct {
	Resistance float64 `yaml:"resistance"`
}

// MeasurementConfig contains measurement parameters.
type MeasurementConfig struct {
	WindowSeconds    float64 `yaml:"window_seconds"`
	PulseThreshold   float64 `yaml:"pulse_threshold"`
	MinPulseDuration float64 `yaml:"min_pulse_duration"` // Minimum pulse duration in seconds (filters noise)
	AverageSamples   int     `yaml:"average_samples"`    // Number of samples to average (0 = disabled, default)
}

// CalibrationConfig contains calibration parameters and points.
type CalibrationConfig struct {
	BaselineDuration time.Duration      `yaml:"baseline_duration"`
	HeaterDuration   time.Duration      `yaml:"heater_duration"`
	CooloffDuration  time.Duration      `yaml:"cooloff_duration"`
	HeaterSequence   []int              `yaml:"heater_sequence"`
	Points           []CalibrationPoint `yaml:"points"`
}

// CalibrationPoint represents a single calibration point.
type CalibrationPoint struct {
	Slope float64 `yaml:"slope"`
	Power float64 `yaml:"power"`
}

// MockConfig contains mock device configuration.
type MockConfig struct {
	Bias          float64       `yaml:"bias"`           // Bias voltage (V)
	NoiseLevel    float64       `yaml:"noise_level"`    // Noise level (V)
	LaserPower    float64       `yaml:"laser_power"`    // Simulated laser power (mW)
	LaserDuration time.Duration `yaml:"laser_duration"` // Laser pulse duration
	LaserPeriod   time.Duration `yaml:"laser_period"`   // Time between laser pulses
	SampleRate    time.Duration `yaml:"sample_rate"`    // Sample rate
}

// Default returns a default configuration with sensible values.
func Default() *Config {
	return &Config{
		Serial: SerialConfig{
			Port: "COM3", // Default for Windows, should be "/dev/ttyACM0" on Linux/Mac
		},
		VoltageDivider: VoltageDividerConfig{
			R1:   20000,
			R2:   20000,
			VRef: 3.3,
		},
		Heaters: []HeaterConfig{
			{Resistance: 2300},
			{Resistance: 500},
			{Resistance: 200},
		},
		Measurement: MeasurementConfig{
			WindowSeconds:    10,
			PulseThreshold:   0.001,
			MinPulseDuration: 1.0, // Filter pulses shorter than 1 second
			AverageSamples:   0,   // No averaging by default
		},
		Calibration: CalibrationConfig{
			BaselineDuration: 10 * time.Second,
			HeaterDuration:   2 * time.Second,
			CooloffDuration:  20 * time.Second,
			HeaterSequence:   []int{1, 2, 3},
			Points: []CalibrationPoint{
				{Slope: 0.0, Power: 0.0},
			},
		},
		Mock: MockConfig{
			Bias:          0.0,
			NoiseLevel:    0.001,
			LaserPower:    40.0,
			LaserDuration: 2 * time.Second,
			LaserPeriod:   20 * time.Second,
			SampleRate:    20 * time.Millisecond, // 50 samples per second // 10 Hz
		},
	}
}

// Load loads configuration from a YAML file. If the file doesn't exist or
// fields are missing, it uses default values.
func Load(filename string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return defaults
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Ensure minimum required fields are set (use defaults if missing)
	cfg.ensureDefaults()

	return cfg, nil
}

// Save saves the configuration to a YAML file.
func (c *Config) Save(filename string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ensureDefaults ensures that all required fields have default values if missing.
func (c *Config) ensureDefaults() {
	def := Default()

	if c.Serial.Port == "" {
		c.Serial.Port = def.Serial.Port
	}

	if c.VoltageDivider.R1 == 0 {
		c.VoltageDivider.R1 = def.VoltageDivider.R1
	}
	if c.VoltageDivider.R2 == 0 {
		c.VoltageDivider.R2 = def.VoltageDivider.R2
	}
	if c.VoltageDivider.VRef == 0 {
		c.VoltageDivider.VRef = def.VoltageDivider.VRef
	}

	if len(c.Heaters) == 0 {
		c.Heaters = def.Heaters
	}

	if c.Measurement.WindowSeconds == 0 {
		c.Measurement.WindowSeconds = def.Measurement.WindowSeconds
	}
	if c.Measurement.PulseThreshold == 0 {
		c.Measurement.PulseThreshold = def.Measurement.PulseThreshold
	}

	if c.Calibration.BaselineDuration == 0 {
		c.Calibration.BaselineDuration = def.Calibration.BaselineDuration
	}
	if c.Calibration.HeaterDuration == 0 {
		c.Calibration.HeaterDuration = def.Calibration.HeaterDuration
	}
	if c.Calibration.CooloffDuration == 0 {
		c.Calibration.CooloffDuration = def.Calibration.CooloffDuration
	}
	if len(c.Calibration.HeaterSequence) == 0 {
		c.Calibration.HeaterSequence = def.Calibration.HeaterSequence
	}
	if len(c.Calibration.Points) == 0 {
		c.Calibration.Points = def.Calibration.Points
	}

	if c.Mock.SampleRate == 0 {
		c.Mock.SampleRate = def.Mock.SampleRate
	}
	if c.Mock.LaserPeriod == 0 {
		c.Mock.LaserPeriod = def.Mock.LaserPeriod
	}
	if c.Mock.LaserDuration == 0 {
		c.Mock.LaserDuration = def.Mock.LaserDuration
	}
}
