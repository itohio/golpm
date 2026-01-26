package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFitPolynomial_Linear(t *testing.T) {
	// Test linear fit: y = 2x + 1
	x := []float64{0, 1, 2, 3, 4}
	y := []float64{1, 3, 5, 7, 9}

	coeffs, rSquared := fitPolynomial(x, y, 1)

	require.Len(t, coeffs, 2)
	assert.InDelta(t, 1.0, coeffs[0], 0.0001) // c0 = 1
	assert.InDelta(t, 2.0, coeffs[1], 0.0001) // c1 = 2
	assert.InDelta(t, 1.0, rSquared, 0.0001)  // Perfect fit
}

func TestFitPolynomial_Quadratic(t *testing.T) {
	// Test quadratic fit: y = x² + 2x + 1
	x := []float64{0, 1, 2, 3, 4}
	y := []float64{1, 4, 9, 16, 25}

	coeffs, rSquared := fitPolynomial(x, y, 2)

	require.Len(t, coeffs, 3)
	assert.InDelta(t, 1.0, coeffs[0], 0.0001) // c0 = 1
	assert.InDelta(t, 2.0, coeffs[1], 0.0001) // c1 = 2
	assert.InDelta(t, 1.0, coeffs[2], 0.0001) // c2 = 1
	assert.Greater(t, rSquared, 0.99)         // Very good fit
}

func TestFitPolynomial_Cubic(t *testing.T) {
	// Test cubic fit: y = x³ + 1
	x := []float64{0, 1, 2, 3, 4}
	y := []float64{1, 2, 9, 28, 65}

	coeffs, rSquared := fitPolynomial(x, y, 3)

	require.Len(t, coeffs, 4)
	assert.InDelta(t, 1.0, coeffs[0], 0.0001) // c0 = 1
	assert.InDelta(t, 0.0, coeffs[1], 0.0001) // c1 = 0
	assert.InDelta(t, 0.0, coeffs[2], 0.0001) // c2 = 0
	assert.InDelta(t, 1.0, coeffs[3], 0.0001) // c3 = 1
	assert.Greater(t, rSquared, 0.99)         // Very good fit
}

func TestFitPolynomial_RealCalibrationData(t *testing.T) {
	// Simulate real calibration data: heater power vs slope
	// Assume some nonlinear relationship
	x := []float64{0.001, 0.002, 0.003, 0.004, 0.005} // slopes in V/s
	y := []float64{10.0, 22.0, 35.0, 49.0, 64.0}      // powers in mW

	coeffs, rSquared := fitPolynomial(x, y, 2)

	require.Len(t, coeffs, 3)
	assert.Greater(t, rSquared, 0.95) // Good fit

	// Verify we can predict values
	testSlope := 0.0025
	predictedPower := coeffs[0] + coeffs[1]*testSlope + coeffs[2]*testSlope*testSlope
	assert.Greater(t, predictedPower, 20.0)
	assert.Less(t, predictedPower, 30.0)
}

func TestSolveLinearSystem_Simple(t *testing.T) {
	// Solve: 2x + 3y = 8
	//        x - y = 1
	// Solution: x = 2.2, y = 1.2
	A := [][]float64{
		{2, 3},
		{1, -1},
	}
	b := []float64{8, 1}

	x := solveLinearSystem(A, b)

	require.Len(t, x, 2)
	assert.InDelta(t, 2.2, x[0], 0.0001)
	assert.InDelta(t, 1.2, x[1], 0.0001)
}

func TestSolveLinearSystem_3x3(t *testing.T) {
	// Solve: 2x + y - z = 8
	//        -3x - y + 2z = -11
	//        -2x + y + 2z = -3
	// Solution: x = 2, y = 3, z = -1
	A := [][]float64{
		{2, 1, -1},
		{-3, -1, 2},
		{-2, 1, 2},
	}
	b := []float64{8, -11, -3}

	x := solveLinearSystem(A, b)

	require.Len(t, x, 3)
	assert.InDelta(t, 2.0, x[0], 0.0001)
	assert.InDelta(t, 3.0, x[1], 0.0001)
	assert.InDelta(t, -1.0, x[2], 0.0001)
}
