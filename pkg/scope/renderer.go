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

	// Heater power label
	heaterLabel *canvas.Text

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
	yMin := r.scope.yMin
	yMax := r.scope.yMax
	xMin := r.scope.xMin
	xMax := r.scope.xMax
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

	// Calculate margins
	marginLeft := float32(60.0)
	marginRight := float32(20.0)
	marginTop := float32(20.0)
	marginBottom := float32(40.0)

	plotWidth := size.Width - marginLeft - marginRight
	plotHeight := size.Height - marginTop - marginBottom
	plotX := marginLeft
	plotY := marginTop

	// Draw grid
	r.drawGrid(plotX, plotY, plotWidth, plotHeight, yMin, yMax, xMin, xMax)

	// Draw samples (orange line)
	if len(samples) > 1 {
		r.drawSampleLine(plotX, plotY, plotWidth, plotHeight, samples, yMin, yMax, xMin, xMax)
	}

	// Draw derivatives (light blue, thicker line)
	if len(derivatives) > 0 && len(samples) > 1 {
		r.drawDerivativeLine(plotX, plotY, plotWidth, plotHeight, derivatives, samples, yMin, yMax, xMin, xMax)
	}

	// Draw pulses (dark blue vertical lines)
	r.drawPulses(plotX, plotY, plotWidth, plotHeight, pulses, samples, xMin, xMax)

	// Draw power labels
	r.drawPowerLabels(plotX, plotY, plotWidth, plotHeight, pulses, samples, yMin, yMax, xMin, xMax)

	// Draw heater power indicator
	if heaterPower > 0 {
		r.drawHeaterPower(plotX, plotY, plotWidth, plotHeight, heaterPower, yMin, yMax)
	}
}

// drawGrid draws the oscilloscope-style grid.
func (r *scopeRenderer) drawGrid(plotX, plotY, plotWidth, plotHeight float32, yMin, yMax float64, xMin, xMax time.Time) {
	// Horizontal grid lines (voltage)
	numHLines := 8
	for i := range numHLines + 1 {
		y := plotY + float32(i)*plotHeight/float32(numHLines)
		line := canvas.NewLine(color.RGBA{R: 40, G: 40, B: 40, A: 255})
		line.Position1 = fyne.NewPos(plotX, y)
		line.Position2 = fyne.NewPos(plotX+plotWidth, y)
		line.StrokeWidth = 1
		r.gridLines = append(r.gridLines, line)
		r.objects = append(r.objects, line)

		// Y-axis label
		value := yMax - float64(i)*(yMax-yMin)/float64(numHLines)
		text := canvas.NewText(formatVoltage(value), color.RGBA{R: 150, G: 150, B: 150, A: 255})
		text.TextSize = 10
		text.Alignment = fyne.TextAlignTrailing
		text.Move(fyne.NewPos(plotX-5, y-6))
		r.gridTexts = append(r.gridTexts, text)
		r.objects = append(r.objects, text)
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
		timeOffset := float64(i) * xMax.Sub(xMin).Seconds() / float64(numVLines)
		timeVal := xMin.Add(time.Duration(timeOffset * float64(time.Second)))
		text := canvas.NewText(formatTime(timeVal.Sub(xMin)), color.RGBA{R: 150, G: 150, B: 150, A: 255})
		text.TextSize = 10
		text.Alignment = fyne.TextAlignCenter
		text.Move(fyne.NewPos(x-20, plotY+plotHeight+5))
		r.gridTexts = append(r.gridTexts, text)
		r.objects = append(r.objects, text)
	}
}

// drawSampleLine draws the sample measurement curve (orange).
func (r *scopeRenderer) drawSampleLine(plotX, plotY, plotWidth, plotHeight float32, samples []sample.Sample, yMin, yMax float64, xMin, xMax time.Time) {
	if len(samples) < 2 {
		return
	}

	points := make([]fyne.Position, 0, len(samples))
	for _, s := range samples {
		x := plotX + float32(s.Timestamp.Sub(xMin).Seconds()/xMax.Sub(xMin).Seconds())*plotWidth
		y := plotY + plotHeight - float32((s.Reading-yMin)/(yMax-yMin))*plotHeight
		points = append(points, fyne.NewPos(x, y))
	}

	// Draw connected line segments
	for i := range len(points) - 1 {
		line := canvas.NewLine(color.RGBA{R: 255, G: 165, B: 0, A: 255}) // Orange
		line.Position1 = points[i]
		line.Position2 = points[i+1]
		line.StrokeWidth = 1.5
		r.objects = append(r.objects, line)
	}
}

// drawDerivativeLine draws the derivative curve (light blue, thicker).
func (r *scopeRenderer) drawDerivativeLine(plotX, plotY, plotWidth, plotHeight float32, derivatives []float64, samples []sample.Sample, yMin, yMax float64, xMin, xMax time.Time) {
	if len(derivatives) == 0 || len(samples) < 2 {
		return
	}

	// Derivatives correspond to sample pairs, so we use sample timestamps
	points := make([]fyne.Position, 0, len(derivatives))
	for i, deriv := range derivatives {
		if i+1 >= len(samples) {
			break
		}
		// Use midpoint between samples for derivative position
		midTime := samples[i].Timestamp.Add(samples[i+1].Timestamp.Sub(samples[i].Timestamp) / 2)
		x := plotX + float32(midTime.Sub(xMin).Seconds()/xMax.Sub(xMin).Seconds())*plotWidth
		y := plotY + plotHeight - float32((deriv-yMin)/(yMax-yMin))*plotHeight
		points = append(points, fyne.NewPos(x, y))
	}

	// Draw connected line segments
	for i := range len(points) - 1 {
		line := canvas.NewLine(color.RGBA{R: 100, G: 200, B: 255, A: 255}) // Light blue
		line.Position1 = points[i]
		line.Position2 = points[i+1]
		line.StrokeWidth = 2.5
		r.objects = append(r.objects, line)
	}
}

// drawPulses draws vertical lines for detected pulses (dark blue).
func (r *scopeRenderer) drawPulses(plotX, plotY, plotWidth, plotHeight float32, pulses []meter.Pulse, samples []sample.Sample, xMin, xMax time.Time) {
	if len(samples) == 0 {
		return
	}

	for _, pulse := range pulses {
		// Get pulse start and end positions from indices
		if pulse.StartIndex < 0 || pulse.StartIndex >= len(samples) {
			continue
		}
		if pulse.EndIndex < 0 || pulse.EndIndex >= len(samples) {
			continue
		}

		startTime := samples[pulse.StartIndex].Timestamp
		endTime := samples[pulse.EndIndex].Timestamp

		// Draw start line
		xStart := plotX + float32(startTime.Sub(xMin).Seconds()/xMax.Sub(xMin).Seconds())*plotWidth
		lineStart := canvas.NewLine(color.RGBA{R: 0, G: 100, B: 200, A: 255}) // Dark blue
		lineStart.Position1 = fyne.NewPos(xStart, plotY)
		lineStart.Position2 = fyne.NewPos(xStart, plotY+plotHeight)
		lineStart.StrokeWidth = 1
		r.pulseLines = append(r.pulseLines, lineStart)
		r.objects = append(r.objects, lineStart)

		// Draw end line
		xEnd := plotX + float32(endTime.Sub(xMin).Seconds()/xMax.Sub(xMin).Seconds())*plotWidth
		lineEnd := canvas.NewLine(color.RGBA{R: 0, G: 100, B: 200, A: 255}) // Dark blue
		lineEnd.Position1 = fyne.NewPos(xEnd, plotY)
		lineEnd.Position2 = fyne.NewPos(xEnd, plotY+plotHeight)
		lineEnd.StrokeWidth = 1
		r.pulseLines = append(r.pulseLines, lineEnd)
		r.objects = append(r.objects, lineEnd)
	}
}

// drawPowerLabels draws power labels over each detected pulse.
func (r *scopeRenderer) drawPowerLabels(plotX, plotY, plotWidth, plotHeight float32, pulses []meter.Pulse, samples []sample.Sample, yMin, yMax float64, xMin, xMax time.Time) {
	if len(samples) == 0 {
		return
	}

	for _, pulse := range pulses {
		if pulse.StartIndex < 0 || pulse.StartIndex >= len(samples) {
			continue
		}
		if pulse.EndIndex < 0 || pulse.EndIndex >= len(samples) {
			continue
		}

		// Calculate center of pulse
		startTime := samples[pulse.StartIndex].Timestamp
		endTime := samples[pulse.EndIndex].Timestamp
		centerTime := startTime.Add(endTime.Sub(startTime) / 2)

		x := plotX + float32(centerTime.Sub(xMin).Seconds()/xMax.Sub(xMin).Seconds())*plotWidth

		// Find max reading in pulse range for Y position
		maxReading := yMin
		for i := pulse.StartIndex; i <= pulse.EndIndex && i < len(samples); i++ {
			if samples[i].Reading > maxReading {
				maxReading = samples[i].Reading
			}
		}
		y := plotY + plotHeight - float32((maxReading-yMin)/(yMax-yMin))*plotHeight - 15

		// Create power label (power is in mW, stored in Pulse struct)
		// In Phase 1, Power may be 0, so we display raw value or skip
		var powerText string
		if pulse.Power > 0 {
			powerText = formatPower(pulse.Power)
		} else {
			// Phase 1: Display raw derivative value instead
			powerText = formatVoltage(pulse.RawValue) + "/s"
		}
		text := canvas.NewText(powerText, color.RGBA{R: 255, G: 165, B: 0, A: 255}) // Orange
		text.TextSize = 12
		text.Alignment = fyne.TextAlignCenter
		text.Move(fyne.NewPos(x-30, y))
		r.powerLabels = append(r.powerLabels, text)
		r.objects = append(r.objects, text)
	}
}

// drawHeaterPower draws the heater power indicator.
func (r *scopeRenderer) drawHeaterPower(plotX, plotY, plotWidth, plotHeight float32, heaterPower float64, yMin, yMax float64) {
	text := canvas.NewText(formatPower(heaterPower*1000), color.RGBA{R: 200, G: 200, B: 200, A: 255}) // Light gray
	text.TextSize = 11
	text.Alignment = fyne.TextAlignLeading
	text.Move(fyne.NewPos(plotX+10, plotY+10))
	r.heaterLabel = text
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

func formatTime(d time.Duration) string {
	if d < time.Second {
		return formatFloat(d.Seconds(), 2) + "s"
	}
	return formatFloat(d.Seconds(), 1) + "s"
}

func formatPower(powerMW float64) string {
	return formatFloat(powerMW, 2) + " mW"
}

func formatFloat(v float64, decimals int) string {
	mult := math.Pow(10, float64(decimals))
	return formatFloatRaw(v*mult, decimals)
}

func formatFloatRaw(v float64, decimals int) string {
	rounded := math.Round(v)
	if rounded == 0 {
		return "0"
	}
	// Simple formatting - can be improved
	str := ""
	if v < 0 {
		str = "-"
		v = -v
	}
	intPart := int64(v)
	str += formatInt(intPart)
	if decimals > 0 {
		frac := v - float64(intPart)
		fracStr := formatInt(int64(frac * math.Pow(10, float64(decimals))))
		// Pad with zeros
		for len(fracStr) < decimals {
			fracStr = "0" + fracStr
		}
		str += "." + fracStr
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
