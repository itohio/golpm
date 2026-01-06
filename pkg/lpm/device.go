package lpm

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

const (
	// DefaultBaudRate is the standard baud rate for XIAO SAMD21.
	DefaultBaudRate = 115200
	// DefaultBufferSize is the default size for the samples channel buffer.
	DefaultBufferSize = 100
)

// RawSample represents a raw measurement sample from the MCU.
type RawSample struct {
	Timestamp time.Time
	Reading   uint16 // 12-bit ADC reading (0-4095)
	Voltage   uint16 // 12-bit ADC reading for voltage (0-4095)
	Heater1   bool   // Heater 1 state
	Heater2   bool   // Heater 2 state
	Heater3   bool   // Heater 3 state
}

// Port represents a serial port.
type Port struct {
	Name        string
	Description string
}

// Serial represents a connection to the LPM MCU.
type Serial struct {
	port     string
	baudRate int
	bufSize  int

	conn      serial.Port
	samples   chan RawSample
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	connected bool
}

// New creates a new Device instance with the specified port, baud rate, and buffer size.
func New(port string, baudRate int, bufSize int) *Serial {
	if baudRate == 0 {
		baudRate = DefaultBaudRate
	}
	if bufSize == 0 {
		bufSize = DefaultBufferSize
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Serial{
		port:      port,
		baudRate:  baudRate,
		bufSize:   bufSize,
		samples:   make(chan RawSample, bufSize),
		ctx:       ctx,
		cancel:    cancel,
		connected: false,
	}
}

// Ports returns a list of available serial ports.
func Ports() ([]Port, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, fmt.Errorf("failed to list serial ports: %w", err)
	}

	result := make([]Port, 0, len(ports))
	for _, name := range ports {
		// Try to get port description if available
		port, err := serial.Open(name, &serial.Mode{
			BaudRate: DefaultBaudRate,
		})
		if err == nil {
			// Port opened successfully, get description
			desc := name // Use name as description if we can't get more info
			port.Close()
			result = append(result, Port{
				Name:        name,
				Description: desc,
			})
		} else {
			// Still add the port even if we can't open it
			result = append(result, Port{
				Name:        name,
				Description: name,
			})
		}
	}

	return result, nil
}

// Connect connects to the serial port and starts reading samples.
func (d *Serial) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.connected {
		return fmt.Errorf("already connected")
	}

	mode := &serial.Mode{
		BaudRate: d.baudRate,
	}

	port, err := serial.Open(d.port, mode)
	if err != nil {
		return fmt.Errorf("failed to open serial port %s: %w", d.port, err)
	}

	d.conn = port
	d.connected = true

	// Start reading samples in a goroutine
	go d.readSamples()

	return nil
}

// Close closes the connection and stops reading samples.
func (d *Serial) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.connected {
		return nil
	}

	// Cancel context to stop reading goroutine
	d.cancel()

	// Close serial port
	if d.conn != nil {
		if err := d.conn.Close(); err != nil {
			log.Printf("Error closing serial port: %v", err)
		}
		d.conn = nil
	}

	d.connected = false

	// Close samples channel
	close(d.samples)

	return nil
}

// Samples returns the channel for reading samples.
func (d *Serial) Samples() <-chan RawSample {
	return d.samples
}

// SetHeaters sets the heater states and sends the command to the MCU.
func (d *Serial) SetHeaters(heater1, heater2, heater3 bool) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.connected {
		return fmt.Errorf("not connected")
	}

	// Build command string: "111\n" for all on, "000\n" for all off, etc.
	var cmd strings.Builder
	if heater1 {
		cmd.WriteByte('1')
	} else {
		cmd.WriteByte('0')
	}
	if heater2 {
		cmd.WriteByte('1')
	} else {
		cmd.WriteByte('0')
	}
	if heater3 {
		cmd.WriteByte('1')
	} else {
		cmd.WriteByte('0')
	}
	cmd.WriteByte('\n')

	_, err := d.conn.Write([]byte(cmd.String()))
	if err != nil {
		return fmt.Errorf("failed to send heater command: %w", err)
	}

	return nil
}

// IsConnected returns whether the device is currently connected.
func (d *Serial) IsConnected() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.connected
}

// readSamples reads lines from the serial port and parses them into RawSample.
func (d *Serial) readSamples() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in readSamples: %v", r)
		}
	}()

	scanner := bufio.NewScanner(d.conn)
	for {
		select {
		case <-d.ctx.Done():
			return
		default:
			if !scanner.Scan() {
				// Scanner stopped (EOF or error)
				if err := scanner.Err(); err != nil {
					if err != io.EOF {
						log.Printf("Error reading from serial port: %v", err)
					}
				}
				return
			}

			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			sample, err := parseLine(line)
			if err != nil {
				log.Printf("Failed to parse line '%s': %v", line, err)
				continue
			}

			// Send sample to channel (non-blocking)
			select {
			case d.samples <- sample:
			case <-d.ctx.Done():
				return
			default:
				// Channel full, log and skip
				log.Printf("Samples channel full, dropping sample")
			}
		}
	}
}

// parseLine parses a line from the MCU into a RawSample.
// Format: unix_micros,reading,voltage,heater1heater2heater3
// Example: 1234567890123,2048,1024,101
func parseLine(line string) (RawSample, error) {
	parts := strings.Split(line, ",")
	if len(parts) != 4 {
		return RawSample{}, fmt.Errorf("invalid line format: expected 4 comma-separated values, got %d", len(parts))
	}

	// Parse timestamp (unix microseconds)
	timestampMicros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return RawSample{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	timestamp := time.Unix(0, timestampMicros*1000) // Convert microseconds to nanoseconds

	// Parse reading (12-bit ADC)
	reading, err := strconv.ParseUint(parts[1], 10, 16)
	if err != nil {
		return RawSample{}, fmt.Errorf("invalid reading: %w", err)
	}
	if reading > 4095 {
		return RawSample{}, fmt.Errorf("reading out of range: %d (max 4095)", reading)
	}

	// Parse voltage (12-bit ADC)
	voltage, err := strconv.ParseUint(parts[2], 10, 16)
	if err != nil {
		return RawSample{}, fmt.Errorf("invalid voltage: %w", err)
	}
	if voltage > 4095 {
		return RawSample{}, fmt.Errorf("voltage out of range: %d (max 4095)", voltage)
	}

	// Parse heater states (3 digits: heater1, heater2, heater3)
	heaterStr := parts[3]
	if len(heaterStr) != 3 {
		return RawSample{}, fmt.Errorf("invalid heater states: expected 3 digits, got %d", len(heaterStr))
	}

	heater1 := heaterStr[0] == '1'
	heater2 := heaterStr[1] == '1'
	heater3 := heaterStr[2] == '1'

	return RawSample{
		Timestamp: timestamp,
		Reading:   uint16(reading),
		Voltage:   uint16(voltage),
		Heater1:   heater1,
		Heater2:   heater2,
		Heater3:   heater3,
	}, nil
}
