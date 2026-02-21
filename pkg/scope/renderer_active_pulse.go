package scope

import (
	"fmt"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/itohio/golpm/pkg/meter"
	"github.com/itohio/golpm/pkg/sample"
)

// drawActivePulse draws the currently tracked pulse (Fitting or Updating state) in gray/dashed style.
// This allows the user to see what pulse is being detected in real-time before it's finalized.
// Fitting pulses: light gray, dashed (still being evaluated)
// Updating pulses: darker gray (official, but still tracking)
func (r *scopeRenderer) drawActivePulse(
	plotX, plotY, plotWidth, plotHeight float32,
	activePulse *meter.Pulse,
	samples []sample.Sample,
	derivatives []float64,
	yMin, yMax float64,
	xMin, xMax time.Time,
) {
	if activePulse == nil {
		return
	}

	// Choose color based on state
	var lineColor color.Color
	var labelColor color.Color
	var stateLabel string
	
	if activePulse.IsFitting() {
		// Fitting: very light gray, still being evaluated
		lineColor = color.RGBA{R: 150, G: 150, B: 150, A: 180}
		labelColor = color.RGBA{R: 150, G: 150, B: 150, A: 255}
		stateLabel = "FITTING"
	} else if activePulse.IsUpdating() {
		// Updating: darker gray, official pulse
		lineColor = color.RGBA{R: 120, G: 120, B: 120, A: 220}
		labelColor = color.RGBA{R: 120, G: 120, B: 120, A: 255}
		stateLabel = "UPDATING"
	} else {
		// Finalized or other - shouldn't happen, but handle gracefully
		return
	}

	// Check if pulse is within visible time range
	if activePulse.DetectEndTime.Before(xMin) || activePulse.DetectStartTime.After(xMax) {
		return
	}

	// Use detection timestamps (not fitted timestamps, as fit may not be stable yet)
	startTime := activePulse.DetectStartTime
	endTime := activePulse.DetectEndTime
	timeRange := xMax.Sub(xMin).Seconds()

	// Draw start line (dashed vertical line)
	if !startTime.Before(xMin) && !startTime.After(xMax) {
		xStart := plotX + float32(startTime.Sub(xMin).Seconds()/timeRange)*plotWidth
		lineStart := canvas.NewLine(lineColor)
		lineStart.Position1 = fyne.NewPos(xStart, plotY)
		lineStart.Position2 = fyne.NewPos(xStart, plotY+plotHeight)
		lineStart.StrokeWidth = 2
		r.objects = append(r.objects, lineStart)
	}

	// Draw end line (dashed vertical line)
	if !endTime.Before(xMin) && !endTime.After(xMax) {
		xEnd := plotX + float32(endTime.Sub(xMin).Seconds()/timeRange)*plotWidth
		lineEnd := canvas.NewLine(lineColor)
		lineEnd.Position1 = fyne.NewPos(xEnd, plotY)
		lineEnd.Position2 = fyne.NewPos(xEnd, plotY+plotHeight)
		lineEnd.StrokeWidth = 2
		r.objects = append(r.objects, lineEnd)
	}

	// If pulse has a fit, draw the horizontal line
	// For Fitting pulses, just draw a horizontal line at AvgSlope if available
	if activePulse.AvgSlope != 0 {
		// Draw horizontal line across the detection window
		yRange := yMax - yMin
		if yRange > 0 {
			// Find visible portion of pulse
			pulseXStart := plotX + float32(startTime.Sub(xMin).Seconds()/timeRange)*plotWidth
			pulseXEnd := plotX + float32(endTime.Sub(xMin).Seconds()/timeRange)*plotWidth
			
			// Clamp to visible area
			if pulseXStart < plotX {
				pulseXStart = plotX
			}
			if pulseXEnd > plotX+plotWidth {
				pulseXEnd = plotX + plotWidth
			}
			
			// Calculate Y position from AvgSlope
			yVal := activePulse.AvgSlope
			yPos := plotY + plotHeight - float32((yVal-yMin)/yRange)*plotHeight
			
			// Draw horizontal line
			line := canvas.NewLine(lineColor)
			line.Position1 = fyne.NewPos(pulseXStart, yPos)
			line.Position2 = fyne.NewPos(pulseXEnd, yPos)
			line.StrokeWidth = 2
			r.objects = append(r.objects, line)
			
			// Draw TWO sets of bands:
			// 1. Actual StdDev bands (±1σ) - shows current fit quality (DYNAMIC)
			// 2. Threshold bands - shows configured acceptable StdDev (STATIC)
			
			if activePulse.StdDev > 0 {
				// Actual StdDev bands (current fit quality)
				yUpperActual := activePulse.AvgSlope + activePulse.StdDev
				yLowerActual := activePulse.AvgSlope - activePulse.StdDev
				
				yPosUpperActual := plotY + plotHeight - float32((yUpperActual-yMin)/yRange)*plotHeight
				yPosLowerActual := plotY + plotHeight - float32((yLowerActual-yMin)/yRange)*plotHeight
				
				// Actual StdDev band color (brighter, shows current state)
				var actualBandColor color.Color
				if activePulse.IsFitting() {
					actualBandColor = color.RGBA{R: 150, G: 150, B: 150, A: 120}
				} else {
					actualBandColor = color.RGBA{R: 100, G: 200, B: 100, A: 120} // Greenish for updating
				}
				
				// Upper actual StdDev band
				upperActualLine := canvas.NewLine(actualBandColor)
				upperActualLine.Position1 = fyne.NewPos(pulseXStart, yPosUpperActual)
				upperActualLine.Position2 = fyne.NewPos(pulseXEnd, yPosUpperActual)
				upperActualLine.StrokeWidth = 1
				r.objects = append(r.objects, upperActualLine)
				
				// Lower actual StdDev band
				lowerActualLine := canvas.NewLine(actualBandColor)
				lowerActualLine.Position1 = fyne.NewPos(pulseXStart, yPosLowerActual)
				lowerActualLine.Position2 = fyne.NewPos(pulseXEnd, yPosLowerActual)
				lowerActualLine.StrokeWidth = 1
				r.objects = append(r.objects, lowerActualLine)
			}
			
			// Threshold bands (configured acceptable StdDev) - STATIC, doesn't change
			// StdDevThresholdMVS() returns mV/s, convert to V/s
			thresholdStdDevVS := activePulse.StdDevThresholdMVS() / 1000.0
			if thresholdStdDevVS > 0 {
				yUpperThreshold := activePulse.AvgSlope + thresholdStdDevVS
				yLowerThreshold := activePulse.AvgSlope - thresholdStdDevVS
				
				yPosUpperThreshold := plotY + plotHeight - float32((yUpperThreshold-yMin)/yRange)*plotHeight
				yPosLowerThreshold := plotY + plotHeight - float32((yLowerThreshold-yMin)/yRange)*plotHeight
				
				// Threshold band color (dim, reference lines)
				thresholdBandColor := color.RGBA{R: 200, G: 200, B: 0, A: 60} // Yellowish, very dim
				
				// Upper threshold band (dashed appearance - draw segments)
				upperThresholdLine := canvas.NewLine(thresholdBandColor)
				upperThresholdLine.Position1 = fyne.NewPos(pulseXStart, yPosUpperThreshold)
				upperThresholdLine.Position2 = fyne.NewPos(pulseXEnd, yPosUpperThreshold)
				upperThresholdLine.StrokeWidth = 1
				r.objects = append(r.objects, upperThresholdLine)
				
				// Lower threshold band
				lowerThresholdLine := canvas.NewLine(thresholdBandColor)
				lowerThresholdLine.Position1 = fyne.NewPos(pulseXStart, yPosLowerThreshold)
				lowerThresholdLine.Position2 = fyne.NewPos(pulseXEnd, yPosLowerThreshold)
				lowerThresholdLine.StrokeWidth = 1
				r.objects = append(r.objects, lowerThresholdLine)
			}
		}
	}

	// Draw label with state and current stats
	centerTime := activePulse.DetectStartTime.Add(activePulse.DetectEndTime.Sub(activePulse.DetectStartTime) / 2)
	x := plotX + float32(centerTime.Sub(xMin).Seconds()/xMax.Sub(xMin).Seconds())*plotWidth

	// Position label on the fitted line (use AvgSlope)
	yRange := yMax - yMin
	var y float32
	if yRange == 0 {
		y = plotY + plotHeight/2
	} else {
		y = plotY + plotHeight - float32((activePulse.AvgSlope-yMin)/yRange)*plotHeight
	}

	// Show state and duration
	duration := activePulse.Duration().Seconds()
	labelText := fmt.Sprintf("%s (%.1fs)", stateLabel, duration)
	
	// Always show mean and stdDev (even if 0 or negative)
	labelText += fmt.Sprintf("\n%.3f mV/s", activePulse.AvgSlope*1000)
	labelText += fmt.Sprintf("\nσ=%.3f mV/s", activePulse.StdDev*1000)

	label := canvas.NewText(labelText, labelColor)
	label.TextSize = 12
	label.Alignment = fyne.TextAlignCenter
	label.TextStyle.Bold = true
	label.Move(fyne.NewPos(x-40, y-50))
	r.objects = append(r.objects, label)
}
