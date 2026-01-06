package sample

// DownsampleSamples downsamples a slice of samples to a maximum number of points.
// Uses simple decimation to reduce the number of points for display.
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

	// Need to downsample
	if cap(dst) >= maxPoints {
		// Reuse dst
		dst = dst[:0] // Reset length but keep capacity
	} else {
		// Allocate new slice
		dst = make([]Sample, 0, maxPoints)
	}

	// Calculate step size for decimation
	step := float64(len(samples)) / float64(maxPoints)

	for i := range maxPoints {
		idx := int(float64(i) * step)
		if idx < len(samples) {
			dst = append(dst, samples[idx])
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
