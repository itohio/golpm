package main

import (
	"fyne.io/fyne/v2"
	"github.com/itohio/golpm/pkg/meter"
	"github.com/itohio/golpm/pkg/sample"
)

// UpdateWidgetOnMainThread schedules a widget update function to run on the main Fyne thread.
// This is required because Fyne widgets cannot be updated directly from goroutines.
// The callback should copy data quickly and return as fast as possible.
// Uses fyne.Do() to schedule the update on the main event loop.
func UpdateWidgetOnMainThread(callback func()) {
	if callback == nil {
		return
	}
	fyne.Do(callback)
}

// DownsampleSamples is a wrapper for sample.DownsampleSamples.
// Deprecated: Use sample.DownsampleSamples directly.
func DownsampleSamples(dst []sample.Sample, samples []sample.Sample, maxPoints int) []sample.Sample {
	return sample.DownsampleSamples(dst, samples, maxPoints)
}

// DownsampleDerivatives is a wrapper for sample.DownsampleDerivatives.
// Deprecated: Use sample.DownsampleDerivatives directly.
func DownsampleDerivatives(dst []float64, derivatives []float64, maxPoints int) []float64 {
	return sample.DownsampleDerivatives(dst, derivatives, maxPoints)
}

// MeasurementData holds a snapshot of measurement data for widget updates.
// This struct is used to pass data from the measurement goroutine to the widget
// via the main thread, minimizing allocations by reusing the same struct.
type MeasurementData struct {
	Samples     []sample.Sample
	Derivatives []float64
	Pulses      []meter.Pulse
}

// CopyMeasurementData creates a snapshot of current measurement data.
// This should be called quickly in the callback, then passed to the widget update.
// The widget update happens on the main thread via UpdateWidgetOnMainThread.
//
// NOTE: The scope widget (pkg/scope) handles downsampling internally, so this function
// should NOT be used when updating the scope widget. Pass full data directly to
// scopeWidget.UpdateData() instead. This function is kept for potential future use
// with other widgets that may need pre-downsampled data.
//
// Accepts destination slices for downsampling to enable array reuse.
// If dstSamples or dstDerivatives are nil or too small, new slices will be allocated.
func CopyMeasurementData(meter *meter.Meter, dstSamples []sample.Sample, dstDerivatives []float64, maxSamples int) MeasurementData {
	// Get data from meter (already thread-safe)
	samples := meter.Samples()
	derivatives := meter.Derivatives()
	pulses := meter.Pulses()

	// Downsample if needed (reuses dst slices if they have sufficient capacity)
	downsampledSamples := sample.DownsampleSamples(dstSamples, samples, maxSamples)
	downsampledDerivatives := sample.DownsampleDerivatives(dstDerivatives, derivatives, maxSamples)

	return MeasurementData{
		Samples:     downsampledSamples,
		Derivatives: downsampledDerivatives,
		Pulses:      pulses, // Pulses are typically few, no need to downsample
	}
}
