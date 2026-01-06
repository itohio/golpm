package sample

import (
	"log"
	"time"

	"github.com/itohio/golpm/pkg/config"
	"github.com/itohio/golpm/pkg/lpm"
)

// Sample represents a processed measurement sample with physical values.
type Sample struct {
	Timestamp   time.Time
	Reading     float64 // Temperature differential voltage (V)
	Voltage     float64 // Voltage measurement (V)
	HeaterPower float64 // Total heater power (W)
}

// Converter is a function type that converts RawSample channel to Sample channel.
type Converter func(in <-chan lpm.RawSample) <-chan Sample

// NewConverter creates a converter function that transforms RawSample to Sample.
func NewConverter(cfg *config.Config, bufSize int) Converter {
	if bufSize <= 0 {
		bufSize = 100
	}

	return func(in <-chan lpm.RawSample) <-chan Sample {
		out := make(chan Sample, bufSize)

		go func() {
			defer close(out)

			for raw := range in {
				sample, err := convertSample(raw, cfg)
				if err != nil {
					log.Printf("Failed to convert sample: %v", err)
					continue
				}

				select {
				case out <- sample:
				case <-time.After(time.Second):
					log.Printf("Converter output channel full, dropping sample")
				}
			}
		}()

		return out
	}
}

// convertSample converts a RawSample to Sample using configuration.
func convertSample(raw lpm.RawSample, cfg *config.Config) (Sample, error) {
	// Convert reading (temperature differential) from ADC to voltage
	readingVoltage := adcToVoltage(raw.Reading, cfg.VoltageDivider.VRef)

	// Convert voltage measurement from ADC to voltage (after divider)
	voltageMeasured := adcToVoltage(raw.Voltage, cfg.VoltageDivider.VRef)
	voltageActual := voltageDivider(voltageMeasured, cfg.VoltageDivider.R1, cfg.VoltageDivider.R2)

	// Calculate heater power
	heaterPower := calculateHeaterPower(voltageActual, raw.Heater1, raw.Heater2, raw.Heater3, cfg.Heaters)

	return Sample{
		Timestamp:   raw.Timestamp,
		Reading:     readingVoltage,
		Voltage:     voltageActual,
		HeaterPower: heaterPower,
	}, nil
}

// adcToVoltage converts a 12-bit ADC reading to voltage.
func adcToVoltage(adc uint16, vref float64) float64 {
	return (float64(adc) / 4095.0) * vref
}

// voltageDivider calculates the input voltage from the measured output voltage.
// Formula: V_in = V_out * ((R1 + R2) / R2)
func voltageDivider(vout float64, r1, r2 float64) float64 {
	return vout * ((r1 + r2) / r2)
}

// calculateHeaterPower calculates the total power from all active heaters.
func calculateHeaterPower(voltage float64, heater1, heater2, heater3 bool, heaters []config.HeaterConfig) float64 {
	if len(heaters) < 3 {
		return 0.0
	}

	var totalPower float64

	// P = V² / R for each active heater
	if heater1 {
		if heaters[0].Resistance > 0 {
			power := (voltage * voltage) / heaters[0].Resistance
			totalPower += power
		}
	}
	if heater2 {
		if heaters[1].Resistance > 0 {
			power := (voltage * voltage) / heaters[1].Resistance
			totalPower += power
		}
	}
	if heater3 {
		if heaters[2].Resistance > 0 {
			power := (voltage * voltage) / heaters[2].Resistance
			totalPower += power
		}
	}

	return totalPower
}
