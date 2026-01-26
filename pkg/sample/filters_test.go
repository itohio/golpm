package sample

import (
	"testing"
	"time"
)

func TestEMAFilter_NoJump(t *testing.T) {
	// Test that EMA filter doesn't cause jumps with non-zero values
	filter := NewEMAFilter(0.5, FieldReading, 10)
	
	in := make(chan Sample, 10)
	out := filter(in)
	
	// Send constant non-zero values
	baseTime := time.Now()
	for i := 0; i < 5; i++ {
		s := Sample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			Reading:   10.0, // Constant value
			Change:    0.0,
			Voltage:   5.0,
			HeaterPower: 0.0,
		}
		in <- s
	}
	close(in)
	
	// Collect outputs
	var outputs []Sample
	for s := range out {
		outputs = append(outputs, s)
	}
	
	if len(outputs) != 5 {
		t.Fatalf("Expected 5 outputs, got %d", len(outputs))
	}
	
	// First sample should be output as-is (10.0)
	if outputs[0].Reading != 10.0 {
		t.Errorf("First sample: expected 10.0, got %f", outputs[0].Reading)
	}
	
	// Subsequent samples should be smooth (with constant input, output should approach input)
	// With alpha=0.5 and constant input=10.0:
	// s1: 10.0 (initialized)
	// s2: 0.5*10.0 + 0.5*10.0 = 10.0
	// s3: 0.5*10.0 + 0.5*10.0 = 10.0
	// All should be 10.0
	for i, out := range outputs {
		if out.Reading != 10.0 {
			t.Errorf("Sample %d: expected 10.0, got %f", i, out.Reading)
		}
	}
}

func TestMAFilter_NoJump(t *testing.T) {
	// Test that MA filter doesn't cause jumps with non-zero values
	filter := NewMAFilter(3, FieldReading, 10)
	
	in := make(chan Sample, 10)
	out := filter(in)
	
	// Send constant non-zero values
	baseTime := time.Now()
	for i := 0; i < 5; i++ {
		s := Sample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			Reading:   10.0, // Constant value
			Change:    0.0,
			Voltage:   5.0,
			HeaterPower: 0.0,
		}
		in <- s
	}
	close(in)
	
	// Collect outputs
	var outputs []Sample
	for s := range out {
		outputs = append(outputs, s)
	}
	
	if len(outputs) != 5 {
		t.Fatalf("Expected 5 outputs, got %d", len(outputs))
	}
	
	// First sample: buffer=[10.0], average=10.0
	if outputs[0].Reading != 10.0 {
		t.Errorf("First sample: expected 10.0, got %f", outputs[0].Reading)
	}
	
	// Second sample: buffer=[10.0, 10.0], average=10.0
	if outputs[1].Reading != 10.0 {
		t.Errorf("Second sample: expected 10.0, got %f", outputs[1].Reading)
	}
	
	// Third sample: buffer=[10.0, 10.0, 10.0], average=10.0
	if outputs[2].Reading != 10.0 {
		t.Errorf("Third sample: expected 10.0, got %f", outputs[2].Reading)
	}
	
	// All should be 10.0 with constant input
	for i, out := range outputs {
		if out.Reading != 10.0 {
			t.Errorf("Sample %d: expected 10.0, got %f", i, out.Reading)
		}
	}
}

func TestMMFilter_NoJump(t *testing.T) {
	// Test that MM filter doesn't cause jumps with non-zero values
	filter := NewMMFilter(3, FieldReading, 10)
	
	in := make(chan Sample, 10)
	out := filter(in)
	
	// Send constant non-zero values
	baseTime := time.Now()
	for i := 0; i < 5; i++ {
		s := Sample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			Reading:   10.0, // Constant value
			Change:    0.0,
			Voltage:   5.0,
			HeaterPower: 0.0,
		}
		in <- s
	}
	close(in)
	
	// Collect outputs
	var outputs []Sample
	for s := range out {
		outputs = append(outputs, s)
	}
	
	if len(outputs) != 5 {
		t.Fatalf("Expected 5 outputs, got %d", len(outputs))
	}
	
	// First sample: buffer=[10.0], median=10.0
	if outputs[0].Reading != 10.0 {
		t.Errorf("First sample: expected 10.0, got %f", outputs[0].Reading)
	}
	
	// Second sample: buffer=[10.0, 10.0], median=(10.0+10.0)/2=10.0
	if outputs[1].Reading != 10.0 {
		t.Errorf("Second sample: expected 10.0, got %f", outputs[1].Reading)
	}
	
	// All should be 10.0 with constant input
	for i, out := range outputs {
		if out.Reading != 10.0 {
			t.Errorf("Sample %d: expected 10.0, got %f", i, out.Reading)
		}
	}
}

func TestEMAFilter_ProgressiveValues(t *testing.T) {
	// Test EMA with progressive values to check for smooth transitions
	filter := NewEMAFilter(0.5, FieldReading, 10)
	
	in := make(chan Sample, 10)
	out := filter(in)
	
	// Send progressive values: 10.0, 11.0, 12.0, 13.0, 14.0
	baseTime := time.Now()
	values := []float64{10.0, 11.0, 12.0, 13.0, 14.0}
	for i, val := range values {
		s := Sample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			Reading:   val,
			Change:    0.0,
			Voltage:   5.0,
			HeaterPower: 0.0,
		}
		in <- s
	}
	close(in)
	
	// Collect outputs
	var outputs []Sample
	for s := range out {
		outputs = append(outputs, s)
	}
	
	if len(outputs) != 5 {
		t.Fatalf("Expected 5 outputs, got %d", len(outputs))
	}
	
	// First sample: 10.0 (initialized)
	if outputs[0].Reading != 10.0 {
		t.Errorf("First sample: expected 10.0, got %f", outputs[0].Reading)
	}
	
	// Second sample: 0.5*11.0 + 0.5*10.0 = 10.5
	expected := 10.5
	if outputs[1].Reading != expected {
		t.Errorf("Second sample: expected %f, got %f", expected, outputs[1].Reading)
	}
	
	// Third sample: 0.5*12.0 + 0.5*10.5 = 11.25
	expected = 11.25
	if outputs[2].Reading != expected {
		t.Errorf("Third sample: expected %f, got %f", expected, outputs[2].Reading)
	}
	
	t.Logf("EMA outputs: %v", outputs)
}

func TestEMAFilter_SuddenChange(t *testing.T) {
	// Test EMA with a sudden change to check for jumps
	// Input: 10.0, 10.0, 10.0, 20.0, 20.0, 20.0
	// With alpha=0.5, output should transition smoothly from 10.0 towards 20.0
	filter := NewEMAFilter(0.5, FieldReading, 10)
	
	in := make(chan Sample, 10)
	out := filter(in)
	
	baseTime := time.Now()
	// Send 3 samples at 10.0, then 3 samples at 20.0
	values := []float64{10.0, 10.0, 10.0, 20.0, 20.0, 20.0}
	for i, val := range values {
		s := Sample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			Reading:   val,
			Change:    0.0,
			Voltage:   5.0,
			HeaterPower: 0.0,
		}
		in <- s
	}
	close(in)
	
	var outputs []Sample
	for s := range out {
		outputs = append(outputs, s)
	}
	
	if len(outputs) != 6 {
		t.Fatalf("Expected 6 outputs, got %d", len(outputs))
	}
	
	// First sample: 10.0 (initialized)
	if outputs[0].Reading != 10.0 {
		t.Errorf("First sample: expected 10.0, got %f", outputs[0].Reading)
	}
	
	// Second sample: 0.5*10.0 + 0.5*10.0 = 10.0
	if outputs[1].Reading != 10.0 {
		t.Errorf("Second sample: expected 10.0, got %f", outputs[1].Reading)
	}
	
	// Third sample: 0.5*10.0 + 0.5*10.0 = 10.0
	if outputs[2].Reading != 10.0 {
		t.Errorf("Third sample: expected 10.0, got %f", outputs[2].Reading)
	}
	
	// Fourth sample (sudden change to 20.0): 0.5*20.0 + 0.5*10.0 = 15.0
	expected := 15.0
	if outputs[3].Reading != expected {
		t.Errorf("Fourth sample: expected %f, got %f", expected, outputs[3].Reading)
	}
	
	// Fifth sample: 0.5*20.0 + 0.5*15.0 = 17.5
	expected = 17.5
	if outputs[4].Reading != expected {
		t.Errorf("Fifth sample: expected %f, got %f", expected, outputs[4].Reading)
	}
	
	// Sixth sample: 0.5*20.0 + 0.5*17.5 = 18.75
	expected = 18.75
	if outputs[5].Reading != expected {
		t.Errorf("Sixth sample: expected %f, got %f", expected, outputs[5].Reading)
	}
	
	t.Logf("EMA outputs with sudden change: %v", outputs)
}

func TestMAFilter_ProgressiveWindow(t *testing.T) {
	// Test that MA filter uses progressive window expansion (1, 2, 3, ..., windowSize)
	filter := NewMAFilter(5, FieldReading, 10)
	
	in := make(chan Sample, 10)
	out := filter(in)
	
	// Send samples with known values: 10.0, 20.0, 30.0, 40.0, 50.0, 60.0
	baseTime := time.Now()
	values := []float64{10.0, 20.0, 30.0, 40.0, 50.0, 60.0}
	for i, val := range values {
		s := Sample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			Reading:   val,
			Change:    0.0,
			Voltage:   5.0,
			HeaterPower: 0.0,
		}
		in <- s
	}
	close(in)
	
	var outputs []Sample
	for s := range out {
		outputs = append(outputs, s)
	}
	
	if len(outputs) != 6 {
		t.Fatalf("Expected 6 outputs, got %d", len(outputs))
	}
	
	// First sample: window = 1, average = 10.0
	if outputs[0].Reading != 10.0 {
		t.Errorf("Sample 0: expected 10.0, got %f", outputs[0].Reading)
	}
	
	// Second sample: window = 2, average = (10.0 + 20.0) / 2 = 15.0
	expected := 15.0
	if outputs[1].Reading != expected {
		t.Errorf("Sample 1: expected %f, got %f", expected, outputs[1].Reading)
	}
	
	// Third sample: window = 3, average = (10.0 + 20.0 + 30.0) / 3 = 20.0
	expected = 20.0
	if outputs[2].Reading != expected {
		t.Errorf("Sample 2: expected %f, got %f", expected, outputs[2].Reading)
	}
	
	// Fourth sample: window = 4, average = (10.0 + 20.0 + 30.0 + 40.0) / 4 = 25.0
	expected = 25.0
	if outputs[3].Reading != expected {
		t.Errorf("Sample 3: expected %f, got %f", expected, outputs[3].Reading)
	}
	
	// Fifth sample: window = 5 (full), average = (10.0 + 20.0 + 30.0 + 40.0 + 50.0) / 5 = 30.0
	expected = 30.0
	if outputs[4].Reading != expected {
		t.Errorf("Sample 4: expected %f, got %f", expected, outputs[4].Reading)
	}
	
	// Sixth sample: window = 5 (fixed), average = (20.0 + 30.0 + 40.0 + 50.0 + 60.0) / 5 = 40.0
	// (oldest sample 10.0 was removed)
	expected = 40.0
	if outputs[5].Reading != expected {
		t.Errorf("Sample 5: expected %f, got %f", expected, outputs[5].Reading)
	}
	
	t.Logf("MA progressive window outputs: %v", outputs)
}

func TestMMFilter_ProgressiveWindow(t *testing.T) {
	// Test that MM filter uses progressive window expansion (1, 2, 3, ..., windowSize)
	filter := NewMMFilter(5, FieldReading, 10)
	
	in := make(chan Sample, 10)
	out := filter(in)
	
	// Send samples with known values: 10.0, 20.0, 30.0, 40.0, 50.0, 60.0
	baseTime := time.Now()
	values := []float64{10.0, 20.0, 30.0, 40.0, 50.0, 60.0}
	for i, val := range values {
		s := Sample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			Reading:   val,
			Change:    0.0,
			Voltage:   5.0,
			HeaterPower: 0.0,
		}
		in <- s
	}
	close(in)
	
	var outputs []Sample
	for s := range out {
		outputs = append(outputs, s)
	}
	
	if len(outputs) != 6 {
		t.Fatalf("Expected 6 outputs, got %d", len(outputs))
	}
	
	// First sample: window = 1, median = 10.0
	if outputs[0].Reading != 10.0 {
		t.Errorf("Sample 0: expected 10.0, got %f", outputs[0].Reading)
	}
	
	// Second sample: window = 2, median = (10.0 + 20.0) / 2 = 15.0
	expected := 15.0
	if outputs[1].Reading != expected {
		t.Errorf("Sample 1: expected %f, got %f", expected, outputs[1].Reading)
	}
	
	// Third sample: window = 3, median = 20.0 (middle value)
	expected = 20.0
	if outputs[2].Reading != expected {
		t.Errorf("Sample 2: expected %f, got %f", expected, outputs[2].Reading)
	}
	
	// Fourth sample: window = 4, median = (20.0 + 30.0) / 2 = 25.0
	expected = 25.0
	if outputs[3].Reading != expected {
		t.Errorf("Sample 3: expected %f, got %f", expected, outputs[3].Reading)
	}
	
	// Fifth sample: window = 5 (full), median = 30.0 (middle value)
	expected = 30.0
	if outputs[4].Reading != expected {
		t.Errorf("Sample 4: expected %f, got %f", expected, outputs[4].Reading)
	}
	
	// Sixth sample: window = 5 (fixed), median = 40.0 (middle value of [20, 30, 40, 50, 60])
	expected = 40.0
	if outputs[5].Reading != expected {
		t.Errorf("Sample 5: expected %f, got %f", expected, outputs[5].Reading)
	}
	
	t.Logf("MM progressive window outputs: %v", outputs)
}
