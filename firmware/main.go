//go:generate tinygo flash -target=xiao

package main

import (
	"machine"
	"time"
)

var (
	adcAbsorber machine.ADC
	adcVoltage  machine.ADC
	uart        = machine.UART0

	// Heater states
	heaterStates    [3]bool
	previousStates  [3]bool
	ignoreCountdown int

	// ADC averaging - running sums and counts
	absorberSum   uint32
	voltageSum    uint32
	absorberCount int // Current count of samples (resets after N samples)
	voltageCount  int // Current count of samples (resets after N samples)

	// Timing
	lastADCRead time.Time

	// Serial buffer for reading lines
	serialBuffer [16]byte
	serialPos    int
)

func main() {
	// Configure heater pins as outputs
	PIN_HEATER1.Configure(machine.PinConfig{Mode: machine.PinOutput})
	PIN_HEATER2.Configure(machine.PinConfig{Mode: machine.PinOutput})
	PIN_HEATER3.Configure(machine.PinConfig{Mode: machine.PinOutput})

	// Configure ADC pins and set up ADCs with highest resolution
	PIN_ADC.Configure(machine.PinConfig{Mode: machine.PinInput})
	PIN_VOLTAGE_ADC.Configure(machine.PinConfig{Mode: machine.PinInput})

	adcAbsorber = machine.ADC{Pin: PIN_ADC}
	adcVoltage = machine.ADC{Pin: PIN_VOLTAGE_ADC}

	adcConfig := machine.ADCConfig{
		Reference:  ADC_REFERENCE_MV,
		Resolution: ADC_RESOLUTION,
	}

	adcAbsorber.Configure(adcConfig)
	adcVoltage.Configure(adcConfig)

	// Configure UART for heater control
	uart.Configure(machine.UARTConfig{
		BaudRate: UART_BAUD_RATE,
	})

	// Initialize timing
	lastADCRead = time.Now()

	// Main loop
	for {
		now := time.Now()

		// Check for serial input (non-blocking)
		processSerial()

		// Read both ADCs at the same time and rate (every 1ms)
		if now.Sub(lastADCRead) >= time.Duration(SAMPLE_INTERVAL_MS)*time.Millisecond {
			readAbsorberADC()
			readVoltageADC()
			lastADCRead = now
		}

		// Check if we've collected N samples for either ADC and output
		if absorberCount >= NUM_SAMPLES || voltageCount >= NUM_SAMPLES {
			outputAveragedValues()
			// Reset and start accumulating again
			absorberSum = 0
			absorberCount = 0
			voltageSum = 0
			voltageCount = 0
		}

		// Small delay to prevent tight loop (but still allow precise timing)
		time.Sleep(100 * time.Microsecond)
	}
}

func readAbsorberADC() {
	if ignoreCountdown > 0 {
		// Ignore this sample
		ignoreCountdown--
		return
	}

	value := adcAbsorber.Get()
	absorberSum += uint32(value)
	absorberCount++
}

func readVoltageADC() {
	if ignoreCountdown > 0 {
		// Ignore this sample
		ignoreCountdown--
		return
	}

	value := adcVoltage.Get()
	voltageSum += uint32(value)
	voltageCount++
}

func outputAveragedValues() {
	// Calculate average for absorber (use actual count, up to NUM_SAMPLES)
	absorberN := absorberCount
	if absorberN > NUM_SAMPLES {
		absorberN = NUM_SAMPLES
	}
	if absorberN == 0 {
		absorberN = 1 // Avoid division by zero
	}
	absorberAvg := uint16(absorberSum / uint32(absorberN))

	// Calculate average for voltage (use actual count, up to NUM_SAMPLES)
	voltageN := voltageCount
	if voltageN > NUM_SAMPLES {
		voltageN = NUM_SAMPLES
	}
	if voltageN == 0 {
		voltageN = 1 // Avoid division by zero
	}
	voltageAvg := uint16(voltageSum / uint32(voltageN))

	// Get timestamp in unix microseconds
	now := time.Now()
	timestampMicros := now.UnixNano() / 1000 // Convert nanoseconds to microseconds

	// Output format: "unix_micros,reading,voltage,heater1heater2heater3\n"
	// Example: "1234567890123,2048,1024,101\n"
	print(timestampMicros)
	print(",")
	print(absorberAvg)
	print(",")
	print(voltageAvg)
	print(",")
	// Output heater states as 3 digits
	if heaterStates[0] {
		print("1")
	} else {
		print("0")
	}
	if heaterStates[1] {
		print("1")
	} else {
		print("0")
	}
	if heaterStates[2] {
		print("1")
	} else {
		print("0")
	}
	print("\n")
}

func processSerial() {
	// Read available bytes from serial
	for uart.Buffered() > 0 {
		data, err := uart.ReadByte()
		if err != nil {
			break
		}

		// Check for newline (end of line)
		if data == '\n' || data == '\r' {
			if serialPos == 3 {
				// We have exactly 3 characters, process heater states
				updateHeaterStates()
			}
			// Reset buffer regardless of length
			serialPos = 0
			continue
		}

		// Ignore whitespace
		if data == ' ' || data == '\t' {
			continue
		}

		// Only accept '0' or '1', and only up to 3 characters
		if data == '0' || data == '1' {
			if serialPos < 3 {
				serialBuffer[serialPos] = data
				serialPos++
			}
			// If we already have 3 characters, ignore additional ones until newline
		} else {
			// Invalid character - reset buffer
			serialPos = 0
		}
	}
}

func updateHeaterStates() {
	// Parse three characters from buffer
	var stateChanged bool

	for i := range 3 {
		newState := serialBuffer[i] == '1'
		if heaterStates[i] != newState {
			stateChanged = true
		}
		previousStates[i] = heaterStates[i]
		heaterStates[i] = newState
	}

	// Update heater pins
	if heaterStates[0] {
		PIN_HEATER1.High()
	} else {
		PIN_HEATER1.Low()
	}

	if heaterStates[1] {
		PIN_HEATER2.High()
	} else {
		PIN_HEATER2.Low()
	}

	if heaterStates[2] {
		PIN_HEATER3.High()
	} else {
		PIN_HEATER3.Low()
	}

	// If any heater state changed, reset ADC averaging and start ignoring samples
	if stateChanged {
		ignoreCountdown = IGNORE_SAMPLES_AFTER_CHANGE
		absorberSum = 0
		voltageSum = 0
		absorberCount = 0
		voltageCount = 0
	}
}
