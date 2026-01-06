package main

import "machine"

const (
	// actually measured values
	VALUE_HEATER1 = 2300
	VALUE_HEATER2 = 500
	VALUE_HEATER3 = 200

	//
	// Voltage divider: R1 (to V+) and R2 (to GND), ADC 12-bit (0..4095), 3.3V ref
	VALUE_VOLTAGE_R1 = 20000 // ohms, connected to plus
	VALUE_VOLTAGE_R2 = 20000 // ohms, connected to ground

	// Multiply adcRead() by VALUE_VOLTAGE_CONSTANT to get voltage at the top of the divider (before R1)
	// Formula: Vin = (Vref * (R1+R2) / R2) * ADC / (2^12)
	//          Vout = ADC * VALUE_VOLTAGE_CONSTANT
	VALUE_VOLTAGE_CONSTANT = 3.3 * (VALUE_VOLTAGE_R1 + VALUE_VOLTAGE_R2) / (VALUE_VOLTAGE_R2 * float64(1<<12))

	PIN_HEATER1     = machine.D7
	PIN_HEATER2     = machine.D8
	PIN_HEATER3     = machine.D9
	PIN_ADC         = machine.A1
	PIN_VOLTAGE_ADC = machine.A10
)
