package sample

import (
	"log"
	"time"

	"github.com/itohio/golpm/pkg/config"
	"github.com/itohio/golpm/pkg/lpm"
)

// NewAveragingConverter creates a converter that averages N consecutive RawSamples
// and converts them to Samples. This reduces noise in the measurements.
func NewAveragingConverter(cfg *config.Config, windowSize int, bufSize int) Converter {
	if windowSize <= 0 {
		windowSize = 1 // No averaging if invalid
	}
	if bufSize <= 0 {
		bufSize = 100
	}

	return func(in <-chan lpm.RawSample) <-chan Sample {
		out := make(chan Sample, bufSize)

		go func() {
			defer close(out)

			var buffer []lpm.RawSample
			ticker := time.NewTicker(100 * time.Millisecond) // Output rate
			defer ticker.Stop()

			for {
				select {
				case raw, ok := <-in:
					if !ok {
						// Input closed, output any remaining samples
						if len(buffer) > 0 {
							avg, err := averageAndConvertSamples(buffer, cfg)
							if err == nil {
								select {
								case out <- avg:
								default:
								}
							}
						}
						return
					}

					buffer = append(buffer, raw)
					if len(buffer) > windowSize {
						buffer = buffer[1:] // Remove oldest
					}

				case <-ticker.C:
					// Output averaged sample periodically
					if len(buffer) > 0 {
						avg, err := averageAndConvertSamples(buffer, cfg)
						if err == nil {
							select {
							case out <- avg:
							default:
								log.Printf("Averaging converter output channel full")
							}
						}
					}
				}
			}
		}()

		return out
	}
}

// averageAndConvertSamples averages a slice of RawSamples and converts to Sample.
// Uses the most recent sample's timestamp and heater states.
func averageAndConvertSamples(samples []lpm.RawSample, cfg *config.Config) (Sample, error) {
	if len(samples) == 0 {
		return Sample{}, nil
	}

	var sumReading, sumVoltage uint32
	lastSample := samples[len(samples)-1]

	for _, s := range samples {
		sumReading += uint32(s.Reading)
		sumVoltage += uint32(s.Voltage)
	}

	n := float64(len(samples))
	avgReadingADC := uint16((float64(sumReading) / n) + 0.5) // Round to nearest
	avgVoltageADC := uint16((float64(sumVoltage) / n) + 0.5)

	// Create averaged RawSample and convert
	avgRaw := lpm.RawSample{
		Timestamp: lastSample.Timestamp,
		Reading:   avgReadingADC,
		Voltage:   avgVoltageADC,
		Heater1:   lastSample.Heater1, // Use most recent heater states
		Heater2:   lastSample.Heater2,
		Heater3:   lastSample.Heater3,
	}

	return convertSample(avgRaw, cfg)
}

// NewAveragingConverterForSamples creates an averaging converter that works on already-converted Samples.
// This is useful when you want to average after conversion.
func NewAveragingConverterForSamples(windowSize int, bufSize int) func(in <-chan Sample) <-chan Sample {
	if windowSize <= 0 {
		windowSize = 1
	}
	if bufSize <= 0 {
		bufSize = 100
	}

	return func(in <-chan Sample) <-chan Sample {
		out := make(chan Sample, bufSize)

		go func() {
			defer close(out)

			var buffer []Sample
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case sample, ok := <-in:
					if !ok {
						if len(buffer) > 0 {
							avg := averageConvertedSamples(buffer)
							select {
							case out <- avg:
							default:
							}
						}
						return
					}

					buffer = append(buffer, sample)
					if len(buffer) > windowSize {
						buffer = buffer[1:]
					}

				case <-ticker.C:
					if len(buffer) > 0 {
						avg := averageConvertedSamples(buffer)
						select {
						case out <- avg:
						default:
							log.Printf("Averaging converter output channel full")
						}
					}
				}
			}
		}()

		return out
	}
}

// averageConvertedSamples averages a slice of converted Samples.
func averageConvertedSamples(samples []Sample) Sample {
	if len(samples) == 0 {
		return Sample{}
	}

	var sumReading, sumVoltage, sumPower float64
	lastSample := samples[len(samples)-1]

	for _, s := range samples {
		sumReading += s.Reading
		sumVoltage += s.Voltage
		sumPower += s.HeaterPower
	}

	n := float64(len(samples))
	return Sample{
		Timestamp:   lastSample.Timestamp,
		Reading:     sumReading / n,
		Voltage:     sumVoltage / n,
		HeaterPower: sumPower / n,
	}
}
