package main

import "machine"

const (
	// Sampling configuration
	SAMPLE_INTERVAL_MS          = 1  // ADC read interval in milliseconds (same for both ADCs)
	NUM_SAMPLES                 = 20 // Number of samples to average
	IGNORE_SAMPLES_AFTER_CHANGE = 10 // Ignore this many samples after heater state change

	// ADC configuration
	ADC_REFERENCE_MV = 3300 // Reference voltage in millivolts (3.3V)
	ADC_RESOLUTION   = 12   // ADC resolution in bits (12-bit = 0-4095)

	// Heater pins
	PIN_HEATER1 = machine.D7
	PIN_HEATER2 = machine.D8
	PIN_HEATER3 = machine.D9

	// ADC pins
	PIN_ADC         = machine.A1
	PIN_VOLTAGE_ADC = machine.A10

	// Serial configuration
	// Baud rate calculation: Format "unix_micros,reading,voltage,heater1heater2heater3\n"
	// Example: "1234567890123456,4095,4095,111\n" = ~30 bytes max per line
	// 50 outputs/sec * 30 bytes/line = 1,500 bytes/sec
	// UART 8N1: 10 bits/byte = 15,000 baud minimum. With 3x headroom: 45,000 baud minimum
	// 115200 provides ~7.7x headroom (11,520 bytes/sec max / 1,500 bytes/sec required)
	UART_BAUD_RATE = 115200
)
