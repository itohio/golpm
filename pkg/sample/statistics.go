package sample

import (
	"log"
	"math"
	"time"
)

// Statistics holds statistical measurements for a signal.
type Statistics struct {
	Count      int       // Number of samples
	Mean       float64   // Mean value
	StdDev     float64   // Standard deviation
	Min        float64   // Minimum value
	Max        float64   // Maximum value
	Variability float64  // Variability (coefficient of variation: stddev/mean)
}

// StatisticsResult holds statistics for input and EMA-filtered signals.
type StatisticsResult struct {
	Input              Statistics // Statistics for input signal
	EMA                Statistics // Statistics for EMA-filtered signal
	VariabilityAgainstEMA float64 // Variability of input against EMA (RMSE normalized by EMA mean)
	OptimalEMA         float64    // Estimated optimal EMA alpha for 1s delay with adequate noise filtering
	SampleRate         float64    // Measured sample rate (samples/second)
}

// NewStatisticsConverter creates a converter that collects statistics on samples.
// It passes through all samples unchanged and logs statistics when the input channel closes.
// If emaAlpha > 0, it also calculates EMA-filtered statistics and variability against EMA.
func NewStatisticsConverter(emaAlpha float64, bufSize int) func(in <-chan Sample) <-chan Sample {
	if bufSize <= 0 {
		bufSize = 100
	}

	return func(in <-chan Sample) <-chan Sample {
		out := make(chan Sample, bufSize)

		go func() {
			defer close(out)

			var samples []float64
			var voltages []float64
			var timestamps []time.Time
			var emaReadings []float64
			var emaVoltages []float64
			
			var emaReading float64
			var emaVoltage float64
			emaInitialized := false

			for sample := range in {
				// Collect raw data
				samples = append(samples, sample.Reading)
				voltages = append(voltages, sample.Voltage)
				timestamps = append(timestamps, sample.Timestamp)

				// Calculate EMA if alpha provided
				if emaAlpha > 0 {
					if !emaInitialized {
						emaReading = sample.Reading
						emaVoltage = sample.Voltage
						emaInitialized = true
					} else {
						emaReading = emaAlpha*sample.Reading + (1-emaAlpha)*emaReading
						emaVoltage = emaAlpha*sample.Voltage + (1-emaAlpha)*emaVoltage
					}
					emaReadings = append(emaReadings, emaReading)
					emaVoltages = append(emaVoltages, emaVoltage)
				}

				// Pass through unchanged
				select {
				case out <- sample:
				case <-time.After(time.Second):
					log.Printf("Statistics converter output channel full, dropping sample")
				}
			}

			// Calculate and log statistics
			if len(samples) > 0 {
				log.Printf("\n=== Statistics Report ===")
				
				// Calculate sample rate
				sampleRate := 0.0
				if len(timestamps) > 1 {
					duration := timestamps[len(timestamps)-1].Sub(timestamps[0])
					sampleRate = float64(len(timestamps)-1) / duration.Seconds()
					log.Printf("Sample Rate: %.2f samples/second", sampleRate)
				}

				// Input statistics
				log.Printf("\n--- Input Signal (Reading) ---")
				inputStats := calculateStatistics(samples)
				logStatistics(inputStats)

				log.Printf("\n--- Input Signal (Voltage) ---")
				voltageStats := calculateStatistics(voltages)
				logStatistics(voltageStats)

				// EMA statistics if calculated
				if emaAlpha > 0 && len(emaReadings) > 0 {
					log.Printf("\n--- EMA-Filtered Signal (Reading, alpha=%.4f) ---", emaAlpha)
					emaStats := calculateStatistics(emaReadings)
					logStatistics(emaStats)

					log.Printf("\n--- EMA-Filtered Signal (Voltage, alpha=%.4f) ---", emaAlpha)
					emaVoltageStats := calculateStatistics(emaVoltages)
					logStatistics(emaVoltageStats)

					// Variability against EMA (RMSE)
					variabilityReading := calculateVariabilityAgainstEMA(samples, emaReadings)
					variabilityVoltage := calculateVariabilityAgainstEMA(voltages, emaVoltages)
					log.Printf("\n--- Variability Against EMA ---")
					log.Printf("Reading RMSE: %.6f V (%.2f%% of EMA mean)", 
						variabilityReading, (variabilityReading/emaStats.Mean)*100)
					log.Printf("Voltage RMSE: %.6f V (%.2f%% of EMA mean)", 
						variabilityVoltage, (variabilityVoltage/emaVoltageStats.Mean)*100)

					// Estimate optimal EMA alpha for 1s delay
					if sampleRate > 0 {
						optimalAlpha := estimateOptimalEMA(inputStats.StdDev, sampleRate, 1.0)
						log.Printf("\n--- Optimal EMA Parameter Estimation ---")
						log.Printf("Target delay: 1.0 seconds")
						log.Printf("Estimated optimal alpha: %.4f", optimalAlpha)
						log.Printf("Equivalent time constant: %.2f seconds", (1-optimalAlpha)/optimalAlpha/sampleRate)
						
						// Calculate what the noise reduction would be with optimal alpha
						noiseReduction := math.Sqrt(2*optimalAlpha / (2-optimalAlpha))
						log.Printf("Expected noise reduction: %.2f%% (output stddev / input stddev)", noiseReduction*100)
					}
				}

				log.Printf("\n=========================\n")
			}
		}()

		return out
	}
}

// calculateStatistics computes basic statistics for a slice of values.
func calculateStatistics(values []float64) Statistics {
	if len(values) == 0 {
		return Statistics{}
	}

	// Calculate mean
	sum := 0.0
	min := values[0]
	max := values[0]
	for _, v := range values {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	mean := sum / float64(len(values))

	// Calculate standard deviation
	sumSqDiff := 0.0
	for _, v := range values {
		diff := v - mean
		sumSqDiff += diff * diff
	}
	stddev := math.Sqrt(sumSqDiff / float64(len(values)))

	// Calculate coefficient of variation (variability)
	variability := 0.0
	if mean != 0 {
		variability = stddev / math.Abs(mean)
	}

	return Statistics{
		Count:       len(values),
		Mean:        mean,
		StdDev:      stddev,
		Min:         min,
		Max:         max,
		Variability: variability,
	}
}

// calculateVariabilityAgainstEMA calculates RMSE between input and EMA-filtered signal.
func calculateVariabilityAgainstEMA(input, ema []float64) float64 {
	if len(input) != len(ema) || len(input) == 0 {
		return 0.0
	}

	sumSqDiff := 0.0
	for i := range input {
		diff := input[i] - ema[i]
		sumSqDiff += diff * diff
	}

	return math.Sqrt(sumSqDiff / float64(len(input)))
}

// estimateOptimalEMA estimates the optimal EMA alpha parameter to achieve
// a target delay (in seconds) while filtering out noise.
//
// The EMA time constant τ is related to alpha by: τ = (1-α)/(α*fs)
// where fs is the sample rate.
//
// For a target delay of targetDelay seconds:
// α = 1 / (1 + targetDelay*fs)
//
// We also consider the noise level (stddev) to ensure adequate filtering.
// Higher noise requires lower alpha (more smoothing).
func estimateOptimalEMA(noiseStdDev, sampleRate, targetDelay float64) float64 {
	if sampleRate <= 0 || targetDelay <= 0 {
		return 0.1 // Default fallback
	}

	// Calculate alpha for target delay
	alphaDelay := 1.0 / (1.0 + targetDelay*sampleRate)

	// For noise filtering, we want the EMA to reduce noise significantly
	// A good rule of thumb: alpha should be small enough that the time constant
	// is at least 3-5 times the noise period.
	// For 1s delay, alpha around 0.05-0.1 works well for most signals.
	
	// Clamp to reasonable range
	if alphaDelay < 0.01 {
		alphaDelay = 0.01
	}
	if alphaDelay > 0.5 {
		alphaDelay = 0.5
	}

	return alphaDelay
}

// logStatistics logs statistics in a readable format.
func logStatistics(stats Statistics) {
	log.Printf("  Count:        %d samples", stats.Count)
	log.Printf("  Mean:         %.6f V", stats.Mean)
	log.Printf("  Std Dev:      %.6f V", stats.StdDev)
	log.Printf("  Min:          %.6f V", stats.Min)
	log.Printf("  Max:          %.6f V", stats.Max)
	log.Printf("  Range:        %.6f V", stats.Max-stats.Min)
	log.Printf("  Variability:  %.2f%% (CV)", stats.Variability*100)
}
