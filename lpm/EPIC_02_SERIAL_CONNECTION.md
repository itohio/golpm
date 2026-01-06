# EPIC 02: Serial Port Connection and Communication

## Overview

Implement serial port communication package that connects to the MCU, reads measurement data asynchronously, and allows controlling heaters. Also includes a mocked device for testing and development without hardware.

## Goals

- Create serial communication package
- Connect to serial port specified in configuration
- Read data lines asynchronously
- Parse measurement data into RawSample struct
- Send heater control commands
- Handle connection lifecycle (connect/disconnect/reconnect)
- Create Device interface for abstraction
- Implement Mock for testing/development
- Support `-mock` command-line flag to use mocked device

## Requirements

### Serial Package (`pkg/lpm`)

**RawSample Structure:**
```go
type RawSample struct {
    Timestamp  time.Time
    Reading    uint16 // 12-bit ADC reading (0-4095)
    Voltage    uint16 // 12-bit ADC reading for voltage (0-4095)
    Heater1    bool   // Heater 1 state
    Heater2    bool   // Heater 2 state
    Heater3    bool   // Heater 3 state
}
```

**Serial Structure:**
```go
type Serial struct {
    port     string
    baudRate int
    
    conn     io.ReadWriteCloser  // Serial port connection
    samples  chan RawSample      // Buffered channel for samples
    mu       sync.RWMutex        // Protects connection state
    ctx      context.Context      // For cancellation
    cancel   context.CancelFunc  // Cancel function
    connected bool                // Connection state
}
```

**Device Interface:**
```go
type Device interface {
    Connect() error
    Close() error
    Samples() <-chan RawSample
    SetHeaters(heater1, heater2, heater3 bool) error
    IsConnected() bool
}
```

**Package Interface:**
- `New(port string, baudRate int, bufSize int) *Serial` - create the serial device with raw sample buffer
- `NewMock(cfg *config.MockConfig) *Mock` - create a mocked device for testing
- `Ports() []Port` - lists serial available ports
- Both `Serial` and `Mock` implement `Device` interface

**Design Principles:**
- Single Responsibility: Only handles serial communication
- Async data reading: Use buffered channel for samples
- Error handling: Return errors, don't panic
- Connection state management: Track connection status
- Accept interfaces: Could accept io.ReadWriter for testing

**Implementation Details:**
- Use `go.bug.st/serial` or `github.com/tarm/serial` for serial port access
- Parse lines with format: `unix_micros,reading,voltage,[heater1|heater2|heater3]` (`[heater1|heater2|heater3]` e.g. `010` or `000` for all off)
- Handle line parsing errors gracefully (log and skip malformed lines)
- Buffered channel size: 100 samples (configurable)
- Baud rate: 115200 (standard for XIAO)
- Read timeout: Handle timeouts appropriately
- Close channel when connection closes

**Error Handling:**
- Connection errors should be returned
- Parse errors should be logged but not stop reading
- Malformed lines should be skipped
- Connection state should be maintained

### Mock Device (`pkg/lpm`)

**Mock Structure:**
- Simulates LPM device behavior without hardware
- Generates realistic samples with configurable bias and noise
- Simulates thermal response when heaters are turned on/off
- Simulates laser pulses (configurable power, duration, period)
- Implements `Device` interface for seamless integration

**Mock Configuration:**
```yaml
mock:
  bias: 0.0              # Bias voltage (V)
  noise_level: 0.001      # Noise level (V)
  laser_power: 40.0       # Simulated laser power (mW)
  laser_duration: 2s      # Laser pulse duration
  laser_period: 20s       # Time between laser pulses
  sample_rate: 100ms      # Sample rate
```

**Mock Behavior:**
- Generates samples at configured sample rate
- Adds configurable bias and noise to measurements
- Simulates thermal response with exponential approach to steady state
- Automatically turns on laser every `laser_period` for `laser_duration`
- Simulates heater heating and cooling when heaters are controlled
- Voltage reading simulates voltage across heaters

### Integration with Main Application

**Connect Button Behavior:**
- When clicked, connects to serial port from config (or mocked device if `-mock` flag)
- Button text changes to "Disconnect" when connected
- Enables/disables other controls based on connection state
- Shows connection status (tooltip or status bar)

**Command-Line Flags:**
- `-mock` - Use mocked device instead of serial port

**Connection State:**
- Store device instance (Device interface) in application state
- Handle connection errors with error dialogs
- Allow reconnection attempts
- Clean up resources on disconnect
- Use Device interface to abstract real vs mocked device

## Implementation Steps

1. **Create `serial` package**
   - Define RawSample struct
   - Create Connector struct
   - Implement Connect() method
   - Implement async ReadSamples() with goroutine
   - Implement line parsing
   - Implement SetHeaters() method
   - Implement Close() method
   - Add proper error handling

2. **Integrate with main application**
   - Add connector instance to app state
   - Wire Connect button to connect/disconnect
   - Handle connection state changes
   - Display connection status

3. **Add error handling**
   - Connection errors
   - Parse errors (log, don't stop)
   - Disconnection handling
   - Resource cleanup

## Dependencies

- `go.bug.st/serial/v2` or `github.com/tarm/serial` - Serial port library
- `bufio` - Line reading
- `strconv` - String to number conversion
- `strings` - String parsing

## Data Format

**MCU Output Format:**
```
1234567890123,2048,1024,101\n
```

Where:
- `1234567890123` - Unix microseconds (timestamp)
- `2048` - 12-bit ADC reading (0-4095) for temperature differential
- `1024` - 12-bit ADC reading (0-4095) for voltage measurement
- `101` - Heater states (1=on, 0=off) for heater1, heater2, heater3

**MCU Input Format:**
```
111\n  # All heaters on
000\n  # All heaters off
101\n  # Heaters 1 and 3 on, heater 2 off
```

## Testing Considerations

- Unit tests for parsing (various valid/invalid inputs)
- Mock serial port for testing
- Test connection lifecycle
- Test heater control commands
- Test error handling

## Success Criteria

- ✅ Can connect to serial port
- ✅ Reads samples asynchronously via channel
- ✅ Correctly parses data lines
- ✅ Can control heaters via commands
- ✅ Handles connection errors gracefully
- ✅ Clean resource cleanup on disconnect
- ✅ Connect button updates UI state
- ✅ Device interface defined and implemented by both Serial and Mock
- ✅ Mock generates realistic simulated samples
- ✅ Mock simulates thermal response and laser pulses
- ✅ `-mock` flag enables mocked device
- ✅ Mock configuration in YAML
- ✅ Code follows SOLID principles

## Notes

- Consider using context for cancellation of async operations
- Channel should be buffered to avoid blocking
- Heater commands should include newline character
- Consider adding read timeout to detect connection issues
- May need to flush output buffer after sending commands
- XIAO SAMD21 typically uses 115200 baud rate
- Device interface allows components to work with either real or mocked device
- Mock is useful for development and testing without hardware
- Mock configuration allows fine-tuning of simulation parameters
- Both Serial and Mock must pass static interface conformance checks
- Following Go best practices: interface named `Device`, implementations named `Serial` and `Mock`

