package sample

import (
	"log"
)

// NewDifferentiationConverter creates a converter that calculates the Change field
// by differentiating the Reading signal. The Change field represents the change
// from the previous reading, normalized by time (dV/dt).
//
// For the first sample, Change is set to 0.0.
// For subsequent samples, Change = (current.Reading - previous.Reading) / dt
func NewDifferentiationConverter(bufSize int) func(in <-chan Sample) <-chan Sample {
	if bufSize <= 0 {
		bufSize = 100
	}

	return func(in <-chan Sample) <-chan Sample {
		out := make(chan Sample, bufSize)

		go func() {
			defer close(out)

			var prev Sample
			initialized := false

			for sample := range in {
				if !initialized {
					// First sample: Change is 0
					sample.Change = 0.0
					prev = sample
					initialized = true
					select {
					case out <- sample:
					default:
						log.Printf("Differentiation converter output channel full")
					}
					continue
				}

				// Calculate change: (current - previous) / dt
				dt := sample.Timestamp.Sub(prev.Timestamp).Seconds()
				if dt > 0 {
					sample.Change = (sample.Reading - prev.Reading) / dt
				} else {
					// Zero or negative dt, set change to 0
					sample.Change = 0.0
				}

				prev = sample

				select {
				case out <- sample:
				default:
					log.Printf("Differentiation converter output channel full")
				}
			}
		}()

		return out
	}
}
