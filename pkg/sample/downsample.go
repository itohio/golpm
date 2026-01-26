package sample

import (
	"log"
	"time"
)

// NewDownsamplingConverter creates a converter that downsamples samples to a target sample rate.
// Samples within each time window are averaged together.
// All fields are averaged within each window.
//
// targetRate is the target sample rate (e.g., 1*time.Second = 1 sample per second).
// If targetRate <= 0, no downsampling is performed (passes through all samples).
//
// This is useful for reducing the data rate for processing or display while maintaining
// signal integrity through averaging.
func NewDownsamplingConverter(targetRate time.Duration, bufSize int) func(in <-chan Sample) <-chan Sample {
	if bufSize <= 0 {
		bufSize = 100
	}

	return func(in <-chan Sample) <-chan Sample {
		out := make(chan Sample, bufSize)

		go func() {
			defer close(out)

			if targetRate <= 0 {
				// No downsampling, pass through all samples
				for sample := range in {
					select {
					case out <- sample:
					default:
						log.Printf("Downsampling converter output channel full")
					}
				}
				return
			}

			var windowStart time.Time
			var windowSamples []Sample
			initialized := false

			for sample := range in {
				if !initialized {
					// Initialize first window
					windowStart = sample.Timestamp
					windowSamples = []Sample{sample}
					initialized = true
					continue
				}

				// Check if sample is within current window
				windowEnd := windowStart.Add(targetRate)
				if sample.Timestamp.Before(windowEnd) {
					// Add to current window
					windowSamples = append(windowSamples, sample)
				} else {
					// Current window is complete, output averaged sample
					if len(windowSamples) > 0 {
						avg := averageWindow(windowSamples)
						select {
						case out <- avg:
						default:
							log.Printf("Downsampling converter output channel full")
						}
					}

					// Start new window
					windowStart = sample.Timestamp
					windowSamples = []Sample{sample}
				}
			}

			// Output any remaining samples in the last window
			if len(windowSamples) > 0 {
				avg := averageWindow(windowSamples)
				select {
				case out <- avg:
				default:
					log.Printf("Downsampling converter output channel full")
				}
			}
		}()

		return out
	}
}

// averageWindow averages all samples in a window.
func averageWindow(samples []Sample) Sample {
	if len(samples) == 0 {
		return Sample{}
	}

	var sumReading, sumChange, sumVoltage float64
	lastSample := samples[len(samples)-1]

	for _, s := range samples {
		sumReading += s.Reading
		sumChange += s.Change
		sumVoltage += s.Voltage
		// HeaterPower is never filtered/averaged - use latest value
	}

	n := float64(len(samples))
	return Sample{
		Timestamp:   lastSample.Timestamp, // Use last sample's timestamp in window
		Reading:     sumReading / n,
		Change:      sumChange / n,
		Voltage:     sumVoltage / n,
		HeaterPower: lastSample.HeaterPower, // Use latest value (never filtered)
	}
}

// DownsampleSamples downsamples a slice of samples to a maximum number of points.
// Uses averaging within each window to reduce the number of points for display.
// All fields are averaged within each window.
// Destination-based: reuses dst if it has sufficient capacity, otherwise allocates new.
// Returns the destination slice (may be dst if reused, or a new slice if dst was too small).
// If len(samples) <= maxPoints, copies all samples to dst (or allocates if dst is nil/too small).
func DownsampleSamples(dst []Sample, samples []Sample, maxPoints int) []Sample {
	if len(samples) <= maxPoints {
		// Need to copy all samples
		if cap(dst) >= len(samples) {
			dst = dst[:len(samples)]
			copy(dst, samples)
			return dst
		}
		// dst too small, allocate new
		result := make([]Sample, len(samples))
		copy(result, samples)
		return result
	}

	// Need to downsample using averaging
	if cap(dst) >= maxPoints {
		// Reuse dst
		dst = dst[:0] // Reset length but keep capacity
	} else {
		// Allocate new slice
		dst = make([]Sample, 0, maxPoints)
	}

	// Calculate step size for windowing
	step := float64(len(samples)) / float64(maxPoints)

	for i := range maxPoints {
		startIdx := int(float64(i) * step)
		endIdx := int(float64(i+1) * step)
		if endIdx > len(samples) {
			endIdx = len(samples)
		}
		if startIdx >= len(samples) {
			break
		}

		// Average all samples in this window (except HeaterPower - use latest value)
		if startIdx < endIdx {
			var sumReading, sumChange, sumVoltage float64
			windowSize := endIdx - startIdx
			lastSampleInWindow := samples[endIdx-1]
			for j := startIdx; j < endIdx; j++ {
				sumReading += samples[j].Reading
				sumChange += samples[j].Change
				sumVoltage += samples[j].Voltage
				// HeaterPower is never filtered/averaged - use latest value
			}
			n := float64(windowSize)
			avg := Sample{
				Timestamp:   lastSampleInWindow.Timestamp, // Use last sample's timestamp in window
				Reading:     sumReading / n,
				Change:      sumChange / n,
				Voltage:     sumVoltage / n,
				HeaterPower: lastSampleInWindow.HeaterPower, // Use latest value (never filtered)
			}
			dst = append(dst, avg)
		}
	}

	return dst
}

// DownsampleDerivatives downsamples a slice of derivatives to a maximum number of points.
// Destination-based: reuses dst if it has sufficient capacity, otherwise allocates new.
// Returns the destination slice (may be dst if reused, or a new slice if dst was too small).
func DownsampleDerivatives(dst []float64, derivatives []float64, maxPoints int) []float64 {
	if len(derivatives) <= maxPoints {
		// Need to copy all derivatives
		if cap(dst) >= len(derivatives) {
			dst = dst[:len(derivatives)]
			copy(dst, derivatives)
			return dst
		}
		// dst too small, allocate new
		result := make([]float64, len(derivatives))
		copy(result, derivatives)
		return result
	}

	// Need to downsample
	if cap(dst) >= maxPoints {
		// Reuse dst
		dst = dst[:0] // Reset length but keep capacity
	} else {
		// Allocate new slice
		dst = make([]float64, 0, maxPoints)
	}

	// Calculate step size for decimation
	step := float64(len(derivatives)) / float64(maxPoints)

	for i := range maxPoints {
		idx := int(float64(i) * step)
		if idx < len(derivatives) {
			dst = append(dst, derivatives[idx])
		}
	}

	return dst
}
