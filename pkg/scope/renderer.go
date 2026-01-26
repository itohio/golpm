package scope

import (
	"image/color"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/itohio/golpm/pkg/meter"
	"github.com/itohio/golpm/pkg/sample"
)

// scopeRenderer renders the scope widget.
type scopeRenderer struct {
	scope *ScopeWidget

	// Background
	grid *canvas.Rectangle

	// Lines for samples and derivatives
	sampleLine     *canvas.Line
	derivativeLine *canvas.Line

	// Pulse markers (vertical lines)
	pulseLines []*canvas.Line

	// Power labels
	powerLabels []*canvas.Text

	// Heater power and voltage labels
	heaterLabel        *canvas.Text
	heaterVoltLabel    *canvas.Text
	timestampDiffLabel *canvas.Text

	// Grid lines
	gridLines []*canvas.Line
	gridTexts []*canvas.Text

	// Objects list for Fyne
	objects []fyne.CanvasObject

	// Track last size to detect changes
	lastSize fyne.Size
}

// MinSize returns the minimum size of the widget.
func (r *scopeRenderer) MinSize() fyne.Size {
	return fyne.NewSize(400, 300)
}

// Layout arranges the widget components.
func (r *scopeRenderer) Layout(size fyne.Size) {
	// Background fills entire widget
	r.grid.Resize(size)

	// Check if size changed
	if r.lastSize.Width != size.Width || r.lastSize.Height != size.Height {
		r.lastSize = size
		// Size changed, trigger widget refresh to redraw with new dimensions
		// Use BaseWidget.Refresh() to properly trigger Fyne's refresh cycle
		r.scope.BaseWidget.Refresh()
	}
}

// Refresh updates the widget display.
func (r *scopeRenderer) Refresh() {
	r.scope.mu.RLock()
	samples := r.scope.displaySamples
	derivatives := r.scope.displayDerivatives
	pulses := r.scope.pulses
	heaterPower := r.scope.heaterPower
	sampleYMin := r.scope.sampleYMin
	sampleYMax := r.scope.sampleYMax
	derivativeYMin := r.scope.derivativeYMin
	derivativeYMax := r.scope.derivativeYMax
	xMin := r.scope.xMin
	xMax := r.scope.xMax
	// Get heater voltage from latest sample (if available)
	var heaterVoltage float64
	if len(samples) > 0 {
		heaterVoltage = samples[len(samples)-1].Voltage
	}
	// Calculate timestamp difference between 10 samples
	var timestampDiff time.Duration
	if len(samples) >= 10 {
		timestampDiff = samples[len(samples)-1].Timestamp.Sub(samples[len(samples)-10].Timestamp)
	} else if len(samples) >= 2 {
		timestampDiff = samples[len(samples)-1].Timestamp.Sub(samples[0].Timestamp)
	}
	r.scope.mu.RUnlock()

	size := r.scope.Size()
	if size.Width == 0 || size.Height == 0 {
		return
	}

	// Clear old objects (but keep grid)
	r.objects = []fyne.CanvasObject{r.grid}
	r.gridLines = r.gridLines[:0]
	r.gridTexts = r.gridTexts[:0]
	r.pulseLines = r.pulseLines[:0]
	r.powerLabels = r.powerLabels[:0]
	r.heaterLabel = nil
	r.heaterVoltLabel = nil
	r.timestampDiffLabel = nil

	// Calculate margins - need more space on right for derivative axis labels
	marginLeft := float32(60.0)
	marginRight := float32(60.0) // Increased for right Y-axis labels
	marginTop := float32(20.0)
	marginBottom := float32(40.0)

	plotWidth := size.Width - marginLeft - marginRight
	plotHeight := size.Height - marginTop - marginBottom
	plotX := marginLeft
	plotY := marginTop

	// Draw grid with dual Y-axes
	r.drawGrid(plotX, plotY, plotWidth, plotHeight, sampleYMin, sampleYMax, derivativeYMin, derivativeYMax, xMin, xMax)

	// Draw samples (orange line) using left Y-axis - USE THE SAME METHOD
	if len(samples) > 1 {
		samplePoints := make([]dataPoint, len(samples))
		for i, s := range samples {
			samplePoints[i] = dataPoint{time: s.Timestamp, value: s.Reading}
		}
		r.drawCurve(plotX, plotY, plotWidth, plotHeight, samplePoints, sampleYMin, sampleYMax, xMin, xMax,
			color.RGBA{R: 255, G: 165, B: 0, A: 255}, 1.5) // Orange
	}

	// Draw derivatives (light blue, thinner line) using right Y-axis - USE THE SAME METHOD
	if len(derivatives) > 0 && len(samples) > 1 {
		derivativePoints := make([]dataPoint, 0, len(derivatives))
		for i, deriv := range derivatives {
			if i+1 >= len(samples) {
				break
			}
			// Use midpoint between samples for derivative position
			midTime := samples[i].Timestamp.Add(samples[i+1].Timestamp.Sub(samples[i].Timestamp) / 2)
			derivativePoints = append(derivativePoints, dataPoint{time: midTime, value: deriv})
		}
		r.drawCurve(plotX, plotY, plotWidth, plotHeight, derivativePoints, derivativeYMin, derivativeYMax, xMin, xMax,
			color.RGBA{R: 100, G: 200, B: 255, A: 255}, 1.0) // Light blue, thinner
	}

	// Draw pulses (dark blue vertical lines)
	r.drawPulses(plotX, plotY, plotWidth, plotHeight, pulses, samples, xMin, xMax)

	// Draw fitted lines for pulses (use derivative Y-axis)
	r.drawFittedLines(plotX, plotY, plotWidth, plotHeight, pulses, samples, derivatives, derivativeYMin, derivativeYMax, xMin, xMax)

	// Draw power labels (use derivative Y-axis for positioning - labels go on fitted line)
	r.drawPowerLabels(plotX, plotY, plotWidth, plotHeight, pulses, samples, derivativeYMin, derivativeYMax, xMin, xMax)

	// Draw heater power and voltage indicator (use sample Y-axis)
	if heaterPower > 0 {
		r.drawHeaterPower(plotX, plotY, plotWidth, plotHeight, heaterPower, heaterVoltage, sampleYMin, sampleYMax)
	}

	// Draw timestamp difference between 10 samples
	r.drawTimestampDiff(plotX, plotY, plotWidth, plotHeight, timestampDiff)
}

// drawGrid draws the oscilloscope-style grid with dual Y-axes.
// Left Y-axis: samples (voltage in mV)
// Right Y-axis: derivatives (rate of change in mV/s)
// Uses the SAME method for calculating labels for both axes.
func (r *scopeRenderer) drawGrid(plotX, plotY, plotWidth, plotHeight float32, sampleYMin, sampleYMax, derivativeYMin, derivativeYMax float64, xMin, xMax time.Time) {
	// Horizontal grid lines (shared by both axes)
	numHLines := 8
	for i := range numHLines + 1 {
		y := plotY + float32(i)*plotHeight/float32(numHLines)
		line := canvas.NewLine(color.RGBA{R: 40, G: 40, B: 40, A: 255})
		line.Position1 = fyne.NewPos(plotX, y)
		line.Position2 = fyne.NewPos(plotX+plotWidth, y)
		line.StrokeWidth = 1
		r.gridLines = append(r.gridLines, line)
		r.objects = append(r.objects, line)

		// Left Y-axis label (samples - voltage in mV)
		// Calculate evenly-spaced tick between min and max
		sampleValue := calculateAxisLabel(sampleYMin, sampleYMax, numHLines, i)
		sampleText := canvas.NewText(formatVoltageMV(sampleValue), color.RGBA{R: 255, G: 165, B: 0, A: 255}) // Orange for samples
		sampleText.TextSize = 10
		sampleText.Alignment = fyne.TextAlignTrailing
		sampleText.Move(fyne.NewPos(plotX-5, y-6))
		r.gridTexts = append(r.gridTexts, sampleText)
		r.objects = append(r.objects, sampleText)

		// Right Y-axis label (derivatives - rate in mV/s)
		// Calculate evenly-spaced tick between min and max using the SAME method
		derivativeValue := calculateAxisLabel(derivativeYMin, derivativeYMax, numHLines, i)
		derivativeText := canvas.NewText(formatDerivative(derivativeValue), color.RGBA{R: 100, G: 200, B: 255, A: 255}) // Light blue for derivatives
		derivativeText.TextSize = 10
		derivativeText.Alignment = fyne.TextAlignLeading
		derivativeText.Move(fyne.NewPos(plotX+plotWidth+5, y-6))
		r.gridTexts = append(r.gridTexts, derivativeText)
		r.objects = append(r.objects, derivativeText)
	}

	// Vertical grid lines (time)
	numVLines := 10
	for i := range numVLines + 1 {
		x := plotX + float32(i)*plotWidth/float32(numVLines)
		line := canvas.NewLine(color.RGBA{R: 40, G: 40, B: 40, A: 255})
		line.Position1 = fyne.NewPos(x, plotY)
		line.Position2 = fyne.NewPos(x, plotY+plotHeight)
		line.StrokeWidth = 1
		r.gridLines = append(r.gridLines, line)
		r.objects = append(r.objects, line)

		// X-axis label
		timeRange := xMax.Sub(xMin)
		timeOffset := time.Duration(float64(i) * float64(timeRange) / float64(numVLines))
		timeVal := xMin.Add(timeOffset)
		text := canvas.NewText(formatTime(timeVal.Sub(xMin)), color.RGBA{R: 150, G: 150, B: 150, A: 255})
		text.TextSize = 10
		text.Alignment = fyne.TextAlignCenter
		text.Move(fyne.NewPos(x-20, plotY+plotHeight+5))
		r.gridTexts = append(r.gridTexts, text)
		r.objects = append(r.objects, text)
	}
}

// dataPoint represents a single point in a time series with a value.
type dataPoint struct {
	time  time.Time
	value float64
}

// drawCurve draws a curve from a slice of data points using the same method for all curves.
func (r *scopeRenderer) drawCurve(plotX, plotY, plotWidth, plotHeight float32, points []dataPoint, yMin, yMax float64, xMin, xMax time.Time, color color.RGBA, strokeWidth float32) {
	if len(points) < 2 {
		return
	}

	// Calculate positions for all points
	positions := make([]fyne.Position, 0, len(points))
	timeRange := xMax.Sub(xMin).Seconds()
	yRange := yMax - yMin

	for _, p := range points {
		// X coordinate: map time to plot width
		x := plotX + float32((p.time.Sub(xMin).Seconds()/timeRange))*plotWidth

		// Y coordinate: map value to plot height (with safety check)
		var y float32
		if yRange == 0 {
			y = plotY + plotHeight/2
		} else {
			y = plotY + plotHeight - float32((p.value-yMin)/yRange)*plotHeight
		}
		positions = append(positions, fyne.NewPos(x, y))
	}

	// Draw connected line segments
	for i := range len(positions) - 1 {
		line := canvas.NewLine(color)
		line.Position1 = positions[i]
		line.Position2 = positions[i+1]
		line.StrokeWidth = strokeWidth
		r.objects = append(r.objects, line)
	}
}

// drawPulses draws vertical lines for detected pulses (dark blue).
func (r *scopeRenderer) drawPulses(plotX, plotY, plotWidth, plotHeight float32, pulses []meter.Pulse, samples []sample.Sample, xMin, xMax time.Time) {
	if len(samples) == 0 {
		return
	}

	timeRange := xMax.Sub(xMin).Seconds()

	for _, pulse := range pulses {
		// Check if pulse is within visible time range
		if pulse.EndTime.Before(xMin) || pulse.StartTime.After(xMax) {
			continue
		}

		// Use timestamps directly (no index lookup needed)
		startTime := pulse.StartTime
		endTime := pulse.EndTime

		// Draw start line (only if within visible range)
		if !startTime.Before(xMin) && !startTime.After(xMax) {
			xStart := plotX + float32(startTime.Sub(xMin).Seconds()/timeRange)*plotWidth
			lineStart := canvas.NewLine(color.RGBA{R: 0, G: 100, B: 200, A: 255}) // Dark blue
			lineStart.Position1 = fyne.NewPos(xStart, plotY)
			lineStart.Position2 = fyne.NewPos(xStart, plotY+plotHeight)
			lineStart.StrokeWidth = 1
			r.pulseLines = append(r.pulseLines, lineStart)
			r.objects = append(r.objects, lineStart)
		}

		// Draw end line (only if within visible range)
		if !endTime.Before(xMin) && !endTime.After(xMax) {
			xEnd := plotX + float32(endTime.Sub(xMin).Seconds()/timeRange)*plotWidth
			lineEnd := canvas.NewLine(color.RGBA{R: 0, G: 100, B: 200, A: 255}) // Dark blue
			lineEnd.Position1 = fyne.NewPos(xEnd, plotY)
			lineEnd.Position2 = fyne.NewPos(xEnd, plotY+plotHeight)
			lineEnd.StrokeWidth = 1
			r.pulseLines = append(r.pulseLines, lineEnd)
			r.objects = append(r.objects, lineEnd)
		}
	}
}

// drawFittedLines draws the fitted horizontal lines for detected pulses.
// Shows the fitted line (green) and accepted range (semi-transparent green band).
func (r *scopeRenderer) drawFittedLines(plotX, plotY, plotWidth, plotHeight float32, pulses []meter.Pulse, samples []sample.Sample, derivatives []float64, yMin, yMax float64, xMin, xMax time.Time) {
	if len(samples) == 0 || len(derivatives) == 0 {
		return
	}

	timeRange := xMax.Sub(xMin).Seconds()
	yRange := yMax - yMin

	for _, pulse := range pulses {
		// Check if pulse is within visible time range
		if pulse.EndTime.Before(xMin) || pulse.StartTime.After(xMax) {
			continue
		}
		if len(pulse.FittedLine) == 0 {
			continue
		}

		// Find pulse start/end indices in current sample buffer using timestamps
		startIdx := -1
		endIdx := -1
		for i := range len(samples) - 1 {
			sampleTime := samples[i].Timestamp
			if startIdx == -1 && (sampleTime.Equal(pulse.StartTime) || sampleTime.After(pulse.StartTime)) {
				startIdx = i
			}
			if sampleTime.Before(pulse.EndTime) || sampleTime.Equal(pulse.EndTime) {
				endIdx = i
			}
		}

		// Skip if pulse not found in current buffer
		if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
			continue
		}

		// Use pulse.StdDev directly (already calculated correctly in meter)
		stdDev := pulse.StdDev
		mean := pulse.AvgSlope

		// Accepted range: use StdDevThreshold from pulse (configured threshold)
		// Display both threshold and actual stdDev
		thresholdRange := pulse.StdDevThreshold * 2.0 // ±2σ threshold
		actualRange := stdDev * 2.0                   // ±2σ actual

		upperBoundThreshold := mean + thresholdRange
		lowerBoundThreshold := mean - thresholdRange
		upperBoundActual := mean + actualRange
		lowerBoundActual := mean - actualRange

		// Draw threshold range band (light green - what we expect)
		for i := startIdx; i < endIdx && i+1 < len(samples); i++ {
			t1 := samples[i].Timestamp
			t2 := samples[i+1].Timestamp

			// Skip if outside visible range
			if t2.Before(xMin) || t1.After(xMax) {
				continue
			}

			x1 := plotX + float32(t1.Sub(xMin).Seconds()/timeRange)*plotWidth
			x2 := plotX + float32(t2.Sub(xMin).Seconds()/timeRange)*plotWidth

			// Y positions for threshold bounds
			yUpperThreshold := plotY + plotHeight - float32((upperBoundThreshold-yMin)/yRange)*plotHeight
			yLowerThreshold := plotY + plotHeight - float32((lowerBoundThreshold-yMin)/yRange)*plotHeight

			// Draw light green rectangle for threshold range
			rect := canvas.NewRectangle(color.RGBA{R: 0, G: 255, B: 0, A: 30}) // Very transparent green
			rect.Move(fyne.NewPos(x1, yUpperThreshold))
			rect.Resize(fyne.NewSize(x2-x1, yLowerThreshold-yUpperThreshold))
			r.objects = append(r.objects, rect)
		}

		// Draw actual stdDev range band (darker green - what we got)
		for i := startIdx; i < endIdx && i+1 < len(samples); i++ {
			t1 := samples[i].Timestamp
			t2 := samples[i+1].Timestamp

			// Skip if outside visible range
			if t2.Before(xMin) || t1.After(xMax) {
				continue
			}

			x1 := plotX + float32(t1.Sub(xMin).Seconds()/timeRange)*plotWidth
			x2 := plotX + float32(t2.Sub(xMin).Seconds()/timeRange)*plotWidth

			// Y positions for actual bounds
			yUpperActual := plotY + plotHeight - float32((upperBoundActual-yMin)/yRange)*plotHeight
			yLowerActual := plotY + plotHeight - float32((lowerBoundActual-yMin)/yRange)*plotHeight

			// Draw darker green rectangle for actual range
			rect := canvas.NewRectangle(color.RGBA{R: 0, G: 150, B: 0, A: 50}) // Semi-transparent darker green
			rect.Move(fyne.NewPos(x1, yUpperActual))
			rect.Resize(fyne.NewSize(x2-x1, yLowerActual-yUpperActual))
			r.objects = append(r.objects, rect)
		}

		// Draw fitted line (bright green, thicker) on top of the bands
		for i := startIdx; i < endIdx && i+1 < len(samples); i++ {
			t1 := samples[i].Timestamp
			t2 := samples[i+1].Timestamp

			// Skip if outside visible range
			if t2.Before(xMin) || t1.After(xMax) {
				continue
			}

			x1 := plotX + float32(t1.Sub(xMin).Seconds()/timeRange)*plotWidth
			x2 := plotX + float32(t2.Sub(xMin).Seconds()/timeRange)*plotWidth

			// Y position for fitted line (constant at mean)
			y := plotY + plotHeight - float32((mean-yMin)/yRange)*plotHeight

			line := canvas.NewLine(color.RGBA{R: 0, G: 255, B: 0, A: 255}) // Bright green, opaque
			line.Position1 = fyne.NewPos(x1, y)
			line.Position2 = fyne.NewPos(x2, y)
			line.StrokeWidth = 2.0
			r.objects = append(r.objects, line)
		}

		// Mark outliers with red dots (points outside threshold range)
		for i := startIdx; i < endIdx && i < len(derivatives); i++ {
			t := samples[i].Timestamp

			// Skip if outside visible range
			if t.Before(xMin) || t.After(xMax) {
				continue
			}

			// Check if this point is an outlier (outside threshold range)
			actualValue := derivatives[i]
			if actualValue > upperBoundThreshold || actualValue < lowerBoundThreshold {
				// This is an outlier - mark with red dot
				x := plotX + float32(t.Sub(xMin).Seconds()/timeRange)*plotWidth
				y := plotY + plotHeight - float32((actualValue-yMin)/yRange)*plotHeight

				// Draw red circle
				dot := canvas.NewCircle(color.RGBA{R: 255, G: 0, B: 0, A: 200}) // Red, semi-transparent
				dot.Move(fyne.NewPos(x-2, y-2))
				dot.Resize(fyne.NewSize(4, 4))
				r.objects = append(r.objects, dot)
			}
		}
	}
}

// drawPowerLabels draws power labels over each detected pulse.
// Labels are positioned on the fitted line (derivative Y-axis) to show:
// - Optical power (orange, 16px) - most prominent
// - Average slope (light blue, 12px) - medium
// - Heater power (orange, 10px) - smallest
func (r *scopeRenderer) drawPowerLabels(plotX, plotY, plotWidth, plotHeight float32, pulses []meter.Pulse, samples []sample.Sample, yMin, yMax float64, xMin, xMax time.Time) {
	if len(samples) == 0 {
		return
	}

	for _, pulse := range pulses {
		// Check if pulse is within visible time range
		if pulse.EndTime.Before(xMin) || pulse.StartTime.After(xMax) {
			continue
		}

		// Calculate center of pulse using timestamps
		centerTime := pulse.StartTime.Add(pulse.EndTime.Sub(pulse.StartTime) / 2)

		x := plotX + float32(centerTime.Sub(xMin).Seconds()/xMax.Sub(xMin).Seconds())*plotWidth

		// Position labels on the fitted line (AvgSlope) using derivative Y-axis
		// This places labels directly on the green fitted line
		yRange := yMax - yMin
		var y float32
		if yRange == 0 {
			y = plotY + plotHeight/2
		} else {
			y = plotY + plotHeight - float32((pulse.AvgSlope-yMin)/yRange)*plotHeight
		}

		// Optical power label (large, orange, most prominent)
		powerText := formatPower(pulse.AvgPower)
		powerLabel := canvas.NewText(powerText, color.RGBA{R: 255, G: 165, B: 0, A: 255}) // Orange
		powerLabel.TextSize = 16
		powerLabel.Alignment = fyne.TextAlignCenter
		powerLabel.Move(fyne.NewPos(x-40, y-30)) // Above fitted line
		r.powerLabels = append(r.powerLabels, powerLabel)
		r.objects = append(r.objects, powerLabel)

		// Slope label (medium, light blue)
		slopeText := formatDerivative(pulse.AvgSlope)
		slopeLabel := canvas.NewText(slopeText, color.RGBA{R: 100, G: 200, B: 255, A: 255}) // Light blue (matches derivative color)
		slopeLabel.TextSize = 12
		slopeLabel.Alignment = fyne.TextAlignCenter
		slopeLabel.Move(fyne.NewPos(x-40, y-12)) // Middle
		r.powerLabels = append(r.powerLabels, slopeLabel)
		r.objects = append(r.objects, slopeLabel)

		// Heater power label (smallest, orange)
		heaterText := formatPower(pulse.AvgHeaterPower)
		heaterLabel := canvas.NewText(heaterText, color.RGBA{R: 255, G: 165, B: 0, A: 255}) // Orange
		heaterLabel.TextSize = 10
		heaterLabel.Alignment = fyne.TextAlignCenter
		heaterLabel.Move(fyne.NewPos(x-40, y+2)) // Below, slightly overlapping fitted line
		r.powerLabels = append(r.powerLabels, heaterLabel)
		r.objects = append(r.objects, heaterLabel)
	}
}

// drawHeaterPower draws the heater power and voltage indicator.
func (r *scopeRenderer) drawHeaterPower(plotX, plotY, plotWidth, plotHeight float32, heaterPower, heaterVoltage float64, yMin, yMax float64) {
	// Heater power label (twice the size of voltage)
	powerText := canvas.NewText(formatPower(heaterPower), color.RGBA{R: 200, G: 200, B: 200, A: 255}) // Light gray
	powerText.TextSize = 32
	powerText.Alignment = fyne.TextAlignLeading
	powerText.Move(fyne.NewPos(plotX+10, plotY+10))
	r.heaterLabel = powerText
	r.objects = append(r.objects, powerText)

	// Heater voltage label (below power, half the size of power)
	voltageText := canvas.NewText(formatVoltage(heaterVoltage), color.RGBA{R: 180, G: 180, B: 200, A: 255}) // Slightly different color
	voltageText.TextSize = 16
	voltageText.Alignment = fyne.TextAlignLeading
	voltageText.Move(fyne.NewPos(plotX+10, plotY+50))
	r.heaterVoltLabel = voltageText
	r.objects = append(r.objects, voltageText)
}

// drawTimestampDiff draws the timestamp difference between 10 samples.
func (r *scopeRenderer) drawTimestampDiff(plotX, plotY, plotWidth, plotHeight float32, timestampDiff time.Duration) {
	if timestampDiff == 0 {
		return
	}

	// Format duration (e.g., "1.234s" or "123.4ms")
	var diffText string
	if timestampDiff >= time.Second {
		diffText = formatDuration(timestampDiff)
	} else {
		diffText = formatDurationMs(timestampDiff)
	}

	text := canvas.NewText("Δt(10): "+diffText, color.RGBA{R: 150, G: 150, B: 150, A: 255}) // Gray
	text.TextSize = 10
	text.Alignment = fyne.TextAlignLeading
	// Position in top-right corner
	text.Move(fyne.NewPos(plotX+plotWidth-120, plotY+10))
	r.timestampDiffLabel = text
	r.objects = append(r.objects, text)
}

// Objects returns all canvas objects for rendering.
func (r *scopeRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

// Destroy cleans up resources.
func (r *scopeRenderer) Destroy() {
	// Cleanup handled by Fyne
}

// Helper functions for formatting

func formatVoltage(v float64) string {
	if math.Abs(v) < 0.001 {
		return "0.000V"
	}
	return formatFloat(v, 3) + "V"
}

func formatVoltageMV(v float64) string {
	// Format voltage as mV (millivolts)
	// Convert from V to mV by multiplying by 1000
	vMV := v * 1000
	if math.Abs(vMV) < 0.001 {
		return "0.000mV"
	}
	return formatFloat(vMV, 3) + "mV"
}

func formatDerivative(d float64) string {
	// Format derivative as mV/s (millivolts per second)
	// Convert from V/s to mV/s by multiplying by 1000
	dMV := d * 1000
	if math.Abs(dMV) < 0.000001 {
		return "0.000mV/s"
	}
	// Use appropriate precision based on magnitude
	if math.Abs(dMV) < 0.001 {
		return formatFloat(dMV, 6) + "mV/s"
	} else if math.Abs(dMV) < 1.0 {
		return formatFloat(dMV, 4) + "mV/s"
	} else {
		return formatFloat(dMV, 3) + "mV/s"
	}
}

func formatTime(d time.Duration) string {
	if d < time.Second {
		return formatFloat(d.Seconds(), 2) + "s"
	}
	return formatFloat(d.Seconds(), 1) + "s"
}

func formatPower(powerW float64) string {
	// Convert W to mW for display (power is stored internally in W)
	powerMW := powerW * 1000.0
	return formatFloat(powerMW, 2) + " mW"
}

func formatDuration(d time.Duration) string {
	// Format as seconds with 3 decimal places (e.g., "1.234s")
	return formatFloat(d.Seconds(), 3) + "s"
}

func formatDurationMs(d time.Duration) string {
	// Format as milliseconds with 1 decimal place (e.g., "123.4ms")
	return formatFloat(d.Seconds()*1000, 1) + "ms"
}

func formatFloat(v float64, decimals int) string {
	mult := math.Pow(10, float64(decimals))
	return formatFloatRaw(v*mult, decimals)
}

func formatFloatRaw(v float64, decimals int) string {
	// v is already multiplied by 10^decimals
	rounded := math.Round(v)
	if rounded == 0 {
		return "0"
	}
	// Simple formatting - can be improved
	str := ""
	neg := rounded < 0
	if neg {
		rounded = -rounded
	}
	// Extract integer part (divide by 10^decimals) and fractional part (modulo 10^decimals)
	mult := int64(math.Pow(10, float64(decimals)))
	val := int64(rounded)
	if decimals > 0 {
		intPart := val / mult
		fracPart := val % mult
		if neg {
			str = "-"
		}
		str += formatInt(intPart)
		fracStr := formatInt(fracPart)
		// Pad with zeros
		for len(fracStr) < decimals {
			fracStr = "0" + fracStr
		}
		str += "." + fracStr
	} else {
		if neg {
			str = "-"
		}
		str += formatInt(val)
	}
	return str
}

func formatInt(v int64) string {
	if v == 0 {
		return "0"
	}
	str := ""
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		str = string(rune('0'+v%10)) + str
		v /= 10
	}
	if neg {
		str = "-" + str
	}
	return str
}

// calculateAxisLabel calculates axis label values evenly spaced between min and max.
// Uses the SAME method for all curves (samples, derivatives, etc.).
// Labels are: min, max, and evenly-spaced values in between based on numGridLines.
// Example: if min=10, max=20, numGridLines=8, then step=10/8=1.25
// Labels from top to bottom: 20, 18.75, 17.5, 16.25, 15, 13.75, 12.5, 11.25, 10
func calculateAxisLabel(min, max float64, numGridLines, gridIndex int) float64 {
	if numGridLines == 0 {
		return min
	}
	// Calculate evenly-spaced value for this grid line
	// gridIndex 0 = top (max), gridIndex numGridLines = bottom (min)
	rangeVal := max - min
	if rangeVal == 0 {
		return min
	}
	// Linear interpolation from max (top) to min (bottom)
	ratio := float64(gridIndex) / float64(numGridLines)
	return max - ratio*rangeVal
}
