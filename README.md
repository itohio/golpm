# golpm

Go and TinyGo Laser Power Meter firmware and application

## Overview

golpm is a complete solution for measuring laser/light power using a calorimetric approach. The system consists of firmware running on a Seeed XIAO SAMD21 microcontroller and a desktop application built with Fyne GUI framework.

## Hardware Description

The laser power meter uses a calorimetric design with two thermally isolated 2g copper plates:

- **Absorber Plate**: Covered with black soot and folded into a half-cube to form a black cavity that absorbs as much radiation as possible. This plate includes:
  - NTC (Negative Temperature Coefficient) thermistor for temperature measurement
  - Three calibration resistors (~2kΩ, ~500Ω, and ~200Ω) that can be individually controlled

- **Reference Plate**: Also contains an NTC thermistor for differential temperature measurement

### Measurement Principle

Two NTCs are connected in a bridge configuration with a differential amplifier comparing the two readings. The Seeed XIAO SAMD21's 12-bit ADC measures:
- The differential voltage from the NTC bridge (temperature difference)
- The voltage across the calibration resistors (for precise power calculations via voltage divider)

### Calibration Heaters

Three resistors attached to the absorber plate allow calibration at different power levels:
- Heater 1: ~2kΩ
- Heater 2: ~500Ω  
- Heater 3: ~200Ω

These can be turned on/off individually to provide known power inputs for calibration at approximately:
- ~10mW
- ~50mW
- ~100mW
- ~150mW
- ~160mW

## Project Structure

```
golpm/
├── firmware/          # TinyGo firmware for Seeed XIAO SAMD21
│   ├── main.go       # Main firmware code
│   └── pins.go       # Pin definitions and constants
├── lpm/              # Fyne desktop application
│   ├── EPIC_01_BASIC_GUI.md
│   ├── EPIC_02_SERIAL_CONNECTION.md
│   ├── EPIC_03_HEATER_CONTROL.md
│   ├── EPIC_04_SAMPLE_CONVERSION.md
│   ├── EPIC_05_MEASUREMENT_AND_FITTING.md
│   └── EPIC_06_GRAPH_WIDGET.md
└── README.md         # This file
```

## Firmware

The firmware runs on Seeed XIAO SAMD21 and:
- Reads ADC values from the NTC bridge differential amplifier
- Reads voltage across calibration resistors via voltage divider
- Controls three heater resistors via GPIO pins
- Outputs serial data in format: `unix_micros,reading,voltage,heater1,heater2,heater3`
- Accepts heater control commands: three-digit string (0 or 1 for each heater) followed by newline, e.g., `"000\n"` (all off) or `"111\n"` (all on)

## Desktop Application

The Fyne-based desktop application provides:

- **Real-time Data Acquisition**: Reads measurements from the MCU via serial port
- **Graphical Display**: Shows real-time temperature readings and calculated slope
- **Thermal Model Fitting**: Models the heating as a stack of copper→glue→NTC with proper lag accounting
- **Power Calculation**: Calculates absorbed laser power from temperature slope using calibration data
- **Calibration System**: Interactive calibration process with 6 points (zero, ~10mW, ~50mW, ~100mW, ~150mW, ~160mW)
- **Heater Control**: Manual control of individual heaters
- **Configuration Management**: YAML-based configuration file with serial port, resistor values, measurement window, and calibration data

## Features

- Real-time temperature measurement and display
- Automatic detection of heating and cooling phases
- Thermal model fitting for accurate slope calculation
- Multi-point calibration system
- Configurable heater power calculations
- Modular, SOLID-principle-based architecture

## Development Status

See the epic files in the `lpm/` directory for detailed implementation plans.
