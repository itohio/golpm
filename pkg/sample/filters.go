package sample

import (
	"log"
	"sort"
	"time"
)

// FilterType specifies the type of filter to apply.
type FilterType int

const (
	FilterEMA FilterType = iota // Exponential Moving Average
	FilterMA                    // Moving Average
	FilterMM                    // Moving Median
)

// NewEMAFilter creates a converter that applies exponential moving average (EMA) filtering
// to specified fields. EMA is more responsive than simple moving average and provides
// better real-time smoothing.
//
// alpha is the smoothing factor (0.0 to 1.0):
//   - Lower alpha (e.g., 0.1) = more smoothing, less responsive
//   - Higher alpha (e.g., 0.5) = less smoothing, more responsive
//   - Recommended: 0.2-0.3 for good balance
//
// fields specifies which fields to apply filtering to (FieldReading, FieldChange, FieldVoltage, FieldHeaterPower).
// Multiple fields can be combined with bitwise OR (e.g., FieldReading|FieldChange).
// If fields is 0, defaults to FieldReading|FieldVoltage|FieldHeaterPower (for backward compatibility).
//
// If alpha <= 0 or alpha > 1, it defaults to 0.25 (good default for most cases).
func NewEMAFilter(alpha float64, fields FieldFlags, bufSize int) func(in <-chan Sample) <-chan Sample {
	// Validate and set default alpha
	if alpha <= 0 || alpha > 1 {
		alpha = 0.25 // Good default: smooth but still responsive
	}
	// Default fields for backward compatibility
	if fields == 0 {
		fields = FieldReading | FieldVoltage | FieldHeaterPower
	}
	if bufSize <= 0 {
		bufSize = 100
	}

	return func(in <-chan Sample) <-chan Sample {
		out := make(chan Sample, bufSize)

		go func() {
			defer close(out)

			var smoothed Sample
			initialized := false

			for sample := range in {
				if !initialized {
					// Initialize with first sample - output it as-is to prevent jumps
					// Copy all fields to ensure no zero values
					smoothed = sample
					initialized = true
					select {
					case out <- smoothed:
					default:
						log.Printf("EMA filter output channel full")
					}
					continue
				}

				// Apply EMA: smoothed = alpha * new + (1 - alpha) * smoothed
				// Only apply to specified fields, others pass through unchanged
				// This ensures smooth transitions from initialized values
				result := sample // Start with current sample for non-filtered fields
				if HasField(fields, FieldReading) {
					result.Reading = alpha*sample.Reading + (1-alpha)*smoothed.Reading
					smoothed.Reading = result.Reading // Update state for filtered field
				}
				if HasField(fields, FieldChange) {
					result.Change = alpha*sample.Change + (1-alpha)*smoothed.Change
					smoothed.Change = result.Change // Update state for filtered field
				}
				if HasField(fields, FieldVoltage) {
					result.Voltage = alpha*sample.Voltage + (1-alpha)*smoothed.Voltage
					smoothed.Voltage = result.Voltage // Update state for filtered field
				}
				if HasField(fields, FieldHeaterPower) {
					result.HeaterPower = alpha*sample.HeaterPower + (1-alpha)*smoothed.HeaterPower
					smoothed.HeaterPower = result.HeaterPower // Update state for filtered field
				}
				// For non-filtered fields, update smoothed to current sample value
				// This ensures smooth state tracking
				if !HasField(fields, FieldReading) {
					smoothed.Reading = sample.Reading
				}
				if !HasField(fields, FieldChange) {
					smoothed.Change = sample.Change
				}
				if !HasField(fields, FieldVoltage) {
					smoothed.Voltage = sample.Voltage
				}
				if !HasField(fields, FieldHeaterPower) {
					smoothed.HeaterPower = sample.HeaterPower
				}
				result.Timestamp = sample.Timestamp // Always use latest timestamp
				smoothed.Timestamp = sample.Timestamp

				select {
				case out <- result:
				default:
					log.Printf("EMA filter output channel full")
				}
			}
		}()

		return out
	}
}

// NewMAFilter creates a converter that applies moving average (MA) filtering
// to specified fields using a sliding time window. This provides more aggressive smoothing
// than EMA and is useful for very noisy signals.
//
// windowDuration is the time window to average over. Must be > 0.
// If windowDuration <= 0, it defaults to 1 second.
//
// fields specifies which fields to apply filtering to (FieldReading, FieldChange, FieldVoltage, FieldHeaterPower).
// Multiple fields can be combined with bitwise OR (e.g., FieldReading|FieldChange).
// If fields is 0, defaults to FieldReading|FieldVoltage|FieldHeaterPower (for backward compatibility).
func NewMAFilter(windowDuration time.Duration, fields FieldFlags, bufSize int) func(in <-chan Sample) <-chan Sample {
	if windowDuration <= 0 {
		windowDuration = time.Second
	}
	// Default fields for backward compatibility
	if fields == 0 {
		fields = FieldReading | FieldVoltage | FieldHeaterPower
	}
	if bufSize <= 0 {
		bufSize = 100
	}

	return func(in <-chan Sample) <-chan Sample {
		out := make(chan Sample, bufSize)

		go func() {
			defer close(out)

			var buffer []Sample

			for sample := range in {
				// Add new sample to buffer
				buffer = append(buffer, sample)

				// Remove samples outside the time window
				cutoffTime := sample.Timestamp.Add(-windowDuration)
				validStart := 0
				for i := range buffer {
					if buffer[i].Timestamp.After(cutoffTime) || buffer[i].Timestamp.Equal(cutoffTime) {
						validStart = i
						break
					}
				}
				if validStart > 0 {
					buffer = buffer[validStart:]
				}

				// Calculate average for specified fields using samples within time window
				result := sample // Start with current sample (for non-filtered fields)
				n := float64(len(buffer))

				// Average all samples in current time window
				if HasField(fields, FieldReading) {
					var sum float64
					for i := range buffer {
						sum += buffer[i].Reading
					}
					result.Reading = sum / n
				}
				if HasField(fields, FieldChange) {
					var sum float64
					for i := range buffer {
						sum += buffer[i].Change
					}
					result.Change = sum / n
				}
				if HasField(fields, FieldVoltage) {
					var sum float64
					for i := range buffer {
						sum += buffer[i].Voltage
					}
					result.Voltage = sum / n
				}
				if HasField(fields, FieldHeaterPower) {
					var sum float64
					for i := range buffer {
						sum += buffer[i].HeaterPower
					}
					result.HeaterPower = sum / n
				}
				result.Timestamp = sample.Timestamp // Use latest timestamp

				select {
				case out <- result:
				default:
					log.Printf("MA filter output channel full")
				}
			}
		}()

		return out
	}
}

// NewMMFilter creates a converter that applies moving median filtering
// to specified fields. Median filtering is excellent for removing outliers and impulse
// noise while preserving edges, making it ideal for pulse detection.
//
// windowDuration is the time window for the median window. Must be > 0.
// If windowDuration <= 0, it defaults to 1 second.
//
// fields specifies which fields to apply filtering to (FieldReading, FieldChange, FieldVoltage, FieldHeaterPower).
// Multiple fields can be combined with bitwise OR (e.g., FieldReading|FieldChange).
// If fields is 0, defaults to FieldReading|FieldVoltage|FieldHeaterPower (for backward compatibility).
func NewMMFilter(windowDuration time.Duration, fields FieldFlags, bufSize int) func(in <-chan Sample) <-chan Sample {
	if windowDuration <= 0 {
		windowDuration = time.Second
	}
	// Default fields for backward compatibility
	if fields == 0 {
		fields = FieldReading | FieldVoltage | FieldHeaterPower
	}
	if bufSize <= 0 {
		bufSize = 100
	}

	return func(in <-chan Sample) <-chan Sample {
		out := make(chan Sample, bufSize)

		go func() {
			defer close(out)

			var buffer []Sample

			for sample := range in {
				// Add new sample to buffer
				buffer = append(buffer, sample)

				// Remove samples outside the time window
				cutoffTime := sample.Timestamp.Add(-windowDuration)
				validStart := 0
				for i := range buffer {
					if buffer[i].Timestamp.After(cutoffTime) || buffer[i].Timestamp.Equal(cutoffTime) {
						validStart = i
						break
					}
				}
				if validStart > 0 {
					buffer = buffer[validStart:]
				}

				// Calculate median for specified fields using samples within time window
				result := sample // Start with current sample (for non-filtered fields)

				// Median of all samples in current time window
				if HasField(fields, FieldReading) {
					values := make([]float64, len(buffer))
					for i := range buffer {
						values[i] = buffer[i].Reading
					}
					sort.Float64s(values)
					result.Reading = median(values)
				}
				if HasField(fields, FieldChange) {
					values := make([]float64, len(buffer))
					for i := range buffer {
						values[i] = buffer[i].Change
					}
					sort.Float64s(values)
					result.Change = median(values)
				}
				if HasField(fields, FieldVoltage) {
					values := make([]float64, len(buffer))
					for i := range buffer {
						values[i] = buffer[i].Voltage
					}
					sort.Float64s(values)
					result.Voltage = median(values)
				}
				if HasField(fields, FieldHeaterPower) {
					values := make([]float64, len(buffer))
					for i := range buffer {
						values[i] = buffer[i].HeaterPower
					}
					sort.Float64s(values)
					result.HeaterPower = median(values)
				}
				result.Timestamp = sample.Timestamp // Use latest timestamp

				select {
				case out <- result:
				default:
					log.Printf("MM filter output channel full")
				}
			}
		}()

		return out
	}
}

// median calculates the median of a sorted slice.
func median(sorted []float64) float64 {
	if len(sorted) == 0 {
		return 0.0
	}
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		// Even number of elements: average of two middle values
		return (sorted[mid-1] + sorted[mid]) / 2.0
	}
	// Odd number of elements: middle value
	return sorted[mid]
}
