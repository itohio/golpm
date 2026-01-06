# EPIC 04: Sample Conversion (RawSample to Sample)

## Overview

Implement conversion from raw ADC readings to physical values (voltage, power, temperature differential) using configuration data.

## Goals

- Create converter package that transforms RawSample to Sample
- Calculate actual voltages from ADC readings
- Calculate heater power from voltage measurements
- Apply voltage divider calculations
- Process samples asynchronously via channels

## Requirements

### Sample Structure

**Sample Structure:**
```go
type Sample struct {
    Timestamp    time.Time  // Timestamp converted from unix microseconds
    Reading      float64    // Temperature differential voltage (V)
    Voltage      float64    // Voltage measurement (V)
    HeaterPower  float64    // Total heater power (W)
}
```

### Converter Package (`pkg/sample`)

**Package Interface:**
- `Converter` function that returns a closure which returns a channel:
  - Takes configuration as dependency
  - `NewConverter(cfg config.Config, bufSize int) Converter` - Convert samples
  - `type Converter func(in<-chan RawSample) <-chan Sample`
  - Runs in goroutine, processes until input channel closes
  - Closes output channel when input closes

**Conversion Logic:**
2. **Reading Conversion (Temperature Differential):**
   - Convert 12-bit ADC to voltage: `V = (ADC / 4095) * Vref`
   - Vref from configuration (typically 3.3V)
   - This represents the differential amplifier output

3. **Voltage Conversion (Heater Voltage):**
   - Convert 12-bit ADC to measured voltage (after divider): `V_measured = (ADC / 4095) * Vref`
   - Apply voltage divider formula: `V_actual = V_measured * (R1 + R2) / R2`
   - This gives the actual voltage across the heaters

4. **Heater Power Calculation:**
   - For each active heater: `P = V² / R`
   - Sum power from all active heaters: `P_total = P1 + P2 + P3`
   - Use heater resistances from configuration
   - If no heaters active, power is 0

**Design Principles:**
- Single Responsibility: Only converts raw samples to processed samples
- Explicit dependencies: Configuration passed as parameter
- No side effects: Pure conversion logic
- Channel-based: Consumes from input channel, sends to output channel
- Error handling: Log errors but continue processing (don't stop on bad sample)

**Implementation Details:**
- Use buffered channels (size: 100)
- Process samples in order (maintain FIFO)
- Handle channel closure gracefully
- Convert only valid samples (skip invalid ones with log)

### Integration

**Integration Points:**
- Consumes from `serial.ReadSamples()` channel
- Sends to measurement package channel (next epic)
- Uses configuration from config package

**Channel Flow:**
```
Serial Package → RawSample Channel → Converter → Sample Channel → Measurement Package
```

## Implementation Steps

1. **Create `converter` package**
   - Define Sample struct
   - Create Converter struct with config dependency
   - Implement Convert() method (goroutine)
   - Implement ADC to voltage conversion
   - Implement voltage divider calculation
   - Implement heater power calculation
   - Handle channel lifecycle

2. **Add conversion logic**
   - Timestamp conversion
   - Reading conversion (temperature differential)
   - Voltage conversion (with divider)
   - Power calculation

3. **Integration with application**
   - Wire converter between serial and measurement
   - Set up channels
   - Handle channel closures

4. **Add error handling**
   - Log invalid samples
   - Continue processing on errors
   - Handle channel closure

## Dependencies

- Configuration package
- Serial `pkg/lpm` package (for RawSample type)

## Formulas

**ADC to Voltage:**
```
V = (ADC_value / 4095) * Vref
```

**Voltage Divider:**
```
V_out = V_in * (R2 / (R1 + R2))
V_in = V_out * ((R1 + R2) / R2)
```

**Heater Power:**
```
P = V² / R
P_total = Σ(P_i) for all active heaters i
```

## Testing Considerations

- Unit tests for conversion functions:
  - ADC to voltage conversion
  - Voltage divider calculation
  - Power calculation with various heater combinations
  - Timestamp conversion
- Test channel processing
- Test with sample data from actual MCU
- Test edge cases (max ADC value, zero values)

## Success Criteria

- ✅ RawSample converted to Sample correctly
- ✅ ADC values converted to voltages correctly
- ✅ Voltage divider calculation correct
- ✅ Heater power calculated correctly for all combinations
- ✅ Processes samples asynchronously via channels
- ✅ Handles channel closure gracefully
- ✅ Uses math32 for float32 operations
- ✅ Code follows SOLID principles

## Notes

- Power calculations should handle all 8 possible heater combinations (2³)
- Consider adding validation for ADC values (0-4095 range)
- Timestamp precision: microseconds should be preserved
- Channel buffering prevents blocking between stages

