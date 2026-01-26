package main

import (
	"fmt"
	"math"

	"fyne.io/fyne/v2/dialog"
	"github.com/itohio/golpm/pkg/config"
)

// handleAddCalibrationPoint adds a calibration point from current measurements.
// It takes the average heater power and average slope from the most recent pulse.
func handleAddCalibrationPoint(state *appState) {
	if state.powerMeter == nil {
		dialog.ShowError(fmt.Errorf("no power meter available"), state.window)
		return
	}

	// Get current pulses
	pulses := state.powerMeter.Pulses()
	if len(pulses) == 0 {
		dialog.ShowInformation("No Pulse Detected", "Please wait for a pulse to be detected before adding a calibration point.", state.window)
		return
	}

	// Get the most recent pulse
	lastPulse := pulses[len(pulses)-1]

	// Create calibration point
	// Note: We use AvgHeaterPower (actual measured heater power) and AvgSlope (measured slope)
	// The user will calibrate to find the polynomial that maps slope -> optical power
	point := config.CalibrationPoint{
		Slope: lastPulse.AvgSlope,     // in V/s
		Power: lastPulse.AvgHeaterPower, // in mW
	}

	// Add to config
	state.cfg.Calibration.Points = append(state.cfg.Calibration.Points, point)

	// Save config
	if err := state.cfg.Save("config.yaml"); err != nil {
		dialog.ShowError(fmt.Errorf("failed to save calibration point: %w", err), state.window)
		return
	}

	// Show confirmation
	dialog.ShowInformation("Calibration Point Added",
		fmt.Sprintf("Added calibration point:\nHeater Power: %.3f mW\nSlope: %.6f V/s (%.3f mV/s)",
			point.Power, point.Slope, point.Slope*1000), state.window)
}

// handleCalibrate performs polynomial fitting on calibration points.
// Fits a cubic polynomial: Power = c0 + c1*slope + c2*slope² + c3*slope³
func handleCalibrate(state *appState) {
	points := state.cfg.Calibration.Points
	if len(points) < 2 {
		dialog.ShowError(fmt.Errorf("need at least 2 calibration points to fit a polynomial"), state.window)
		return
	}

	// Extract x (slope) and y (power) values
	n := len(points)
	x := make([]float64, n)
	y := make([]float64, n)
	for i, point := range points {
		x[i] = point.Slope
		y[i] = point.Power
	}

	// Fit polynomial (cubic: degree 3)
	degree := 3
	if n < degree+1 {
		// If we have fewer points than degree+1, reduce degree
		degree = n - 1
	}

	coeffs, rSquared := fitPolynomial(x, y, degree)

	// Update config
	// Ensure we have exactly 4 coefficients (pad with zeros if needed)
	for len(coeffs) < 4 {
		coeffs = append(coeffs, 0.0)
	}
	state.cfg.Measurement.PowerPolynomial = coeffs[:4]

	// Save config
	if err := state.cfg.Save("config.yaml"); err != nil {
		dialog.ShowError(fmt.Errorf("failed to save calibration: %w", err), state.window)
		return
	}

	// Update power meter if it exists
	if state.powerMeter != nil {
		state.powerMeter.UpdateCalibration(coeffs[:4], state.cfg.Measurement.AbsorbanceCoefficient)
	}

	// Show results
	resultText := fmt.Sprintf("Calibration successful!\n\n"+
		"Polynomial coefficients:\n"+
		"c0 = %.6f\n"+
		"c1 = %.6f\n"+
		"c2 = %.6f\n"+
		"c3 = %.6f\n\n"+
		"R² = %.6f\n\n"+
		"Power = c0 + c1*slope + c2*slope² + c3*slope³",
		coeffs[0], coeffs[1], coeffs[2], coeffs[3], rSquared)

	dialog.ShowInformation("Calibration Complete", resultText, state.window)
}

// fitPolynomial fits a polynomial of given degree to (x, y) data points.
// Returns coefficients [c0, c1, c2, ...] and R² value.
// Uses least squares method with normal equations.
func fitPolynomial(x, y []float64, degree int) ([]float64, float64) {
	n := len(x)
	if n != len(y) {
		panic("x and y must have the same length")
	}
	if n < degree+1 {
		panic("need at least degree+1 points")
	}

	// Build Vandermonde matrix and solve normal equations
	// X = [1, x, x², x³, ...]
	// We need to solve: X^T * X * c = X^T * y
	// This gives us the least squares solution

	// Build X^T * X (symmetric matrix)
	size := degree + 1
	XTX := make([][]float64, size)
	for i := range XTX {
		XTX[i] = make([]float64, size)
	}

	for i := range size {
		for j := range size {
			sum := 0.0
			for k := range n {
				sum += math.Pow(x[k], float64(i+j))
			}
			XTX[i][j] = sum
		}
	}

	// Build X^T * y
	XTy := make([]float64, size)
	for i := range size {
		sum := 0.0
		for k := range n {
			sum += math.Pow(x[k], float64(i)) * y[k]
		}
		XTy[i] = sum
	}

	// Solve linear system using Gaussian elimination
	coeffs := solveLinearSystem(XTX, XTy)

	// Calculate R²
	// R² = 1 - (SS_res / SS_tot)
	// SS_res = sum of squared residuals
	// SS_tot = total sum of squares

	// Calculate mean of y
	meanY := 0.0
	for _, yi := range y {
		meanY += yi
	}
	meanY /= float64(n)

	// Calculate SS_tot and SS_res
	ssTot := 0.0
	ssRes := 0.0
	for i := range n {
		// Predicted value
		yPred := 0.0
		for j, c := range coeffs {
			yPred += c * math.Pow(x[i], float64(j))
		}

		ssTot += math.Pow(y[i]-meanY, 2)
		ssRes += math.Pow(y[i]-yPred, 2)
	}

	rSquared := 1.0
	if ssTot > 0 {
		rSquared = 1.0 - (ssRes / ssTot)
	}

	return coeffs, rSquared
}

// solveLinearSystem solves Ax = b using Gaussian elimination with partial pivoting.
func solveLinearSystem(A [][]float64, b []float64) []float64 {
	n := len(b)

	// Create augmented matrix [A|b]
	aug := make([][]float64, n)
	for i := range n {
		aug[i] = make([]float64, n+1)
		copy(aug[i], A[i])
		aug[i][n] = b[i]
	}

	// Forward elimination with partial pivoting
	for i := range n {
		// Find pivot
		maxRow := i
		for k := i + 1; k < n; k++ {
			if math.Abs(aug[k][i]) > math.Abs(aug[maxRow][i]) {
				maxRow = k
			}
		}

		// Swap rows
		aug[i], aug[maxRow] = aug[maxRow], aug[i]

		// Make all rows below this one 0 in current column
		for k := i + 1; k < n; k++ {
			if aug[i][i] == 0 {
				continue
			}
			factor := aug[k][i] / aug[i][i]
			for j := i; j <= n; j++ {
				aug[k][j] -= factor * aug[i][j]
			}
		}
	}

	// Back substitution
	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		x[i] = aug[i][n]
		for j := i + 1; j < n; j++ {
			x[i] -= aug[i][j] * x[j]
		}
		if aug[i][i] != 0 {
			x[i] /= aug[i][i]
		}
	}

	return x
}
