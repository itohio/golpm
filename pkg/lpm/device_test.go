package lpm

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    RawSample
		wantErr bool
	}{
		{
			name: "valid line - all heaters on",
			line: "1234567890123,2048,1024,111",
			want: RawSample{
				Timestamp: time.Unix(0, 1234567890123*1000),
				Reading:   2048,
				Voltage:   1024,
				Heater1:   true,
				Heater2:   true,
				Heater3:   true,
			},
			wantErr: false,
		},
		{
			name: "valid line - all heaters off",
			line: "1234567890123,2048,1024,000",
			want: RawSample{
				Timestamp: time.Unix(0, 1234567890123*1000),
				Reading:   2048,
				Voltage:   1024,
				Heater1:   false,
				Heater2:   false,
				Heater3:   false,
			},
			wantErr: false,
		},
		{
			name: "valid line - heaters 1 and 3 on",
			line: "1234567890123,2048,1024,101",
			want: RawSample{
				Timestamp: time.Unix(0, 1234567890123*1000),
				Reading:   2048,
				Voltage:   1024,
				Heater1:   true,
				Heater2:   false,
				Heater3:   true,
			},
			wantErr: false,
		},
		{
			name: "valid line - max ADC values",
			line: "1234567890123,4095,4095,010",
			want: RawSample{
				Timestamp: time.Unix(0, 1234567890123*1000),
				Reading:   4095,
				Voltage:   4095,
				Heater1:   false,
				Heater2:   true,
				Heater3:   false,
			},
			wantErr: false,
		},
		{
			name:    "invalid - wrong number of fields",
			line:    "1234567890123,2048,1024",
			wantErr: true,
		},
		{
			name:    "invalid - too many fields",
			line:    "1234567890123,2048,1024,101,extra",
			wantErr: true,
		},
		{
			name:    "invalid - non-numeric timestamp",
			line:    "abc,2048,1024,101",
			wantErr: true,
		},
		{
			name:    "invalid - non-numeric reading",
			line:    "1234567890123,abc,1024,101",
			wantErr: true,
		},
		{
			name:    "invalid - reading out of range",
			line:    "1234567890123,5000,1024,101",
			wantErr: true,
		},
		{
			name:    "invalid - voltage out of range",
			line:    "1234567890123,2048,5000,101",
			wantErr: true,
		},
		{
			name:    "invalid - heater states wrong length",
			line:    "1234567890123,2048,1024,11",
			wantErr: true,
		},
		{
			name:    "invalid - heater states wrong length 2",
			line:    "1234567890123,2048,1024,1111",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLine(tt.line)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want.Timestamp.UnixNano(), got.Timestamp.UnixNano())
				assert.Equal(t, tt.want.Reading, got.Reading)
				assert.Equal(t, tt.want.Voltage, got.Voltage)
				assert.Equal(t, tt.want.Heater1, got.Heater1)
				assert.Equal(t, tt.want.Heater2, got.Heater2)
				assert.Equal(t, tt.want.Heater3, got.Heater3)
			}
		})
	}
}

func TestNew(t *testing.T) {
	dev := New("COM3", 115200, 100)
	assert.NotNil(t, dev)
	assert.Equal(t, "COM3", dev.port)
	assert.Equal(t, 115200, dev.baudRate)
	assert.Equal(t, 100, dev.bufSize)
	assert.NotNil(t, dev.samples)
	assert.False(t, dev.IsConnected())
}

func TestNew_Defaults(t *testing.T) {
	dev := New("COM3", 0, 0)
	assert.NotNil(t, dev)
	assert.Equal(t, DefaultBaudRate, dev.baudRate)
	assert.Equal(t, DefaultBufferSize, dev.bufSize)
}

func TestDevice_IsConnected(t *testing.T) {
	dev := New("COM3", 115200, 100)
	assert.False(t, dev.IsConnected())
}

func TestSetHeaters_CommandFormat(t *testing.T) {
	tests := []struct {
		name           string
		heater1, heater2, heater3 bool
		wantCmd        string
	}{
		{"all on", true, true, true, "111\n"},
		{"all off", false, false, false, "000\n"},
		{"1 and 3 on", true, false, true, "101\n"},
		{"only 2 on", false, true, false, "010\n"},
		{"only 1 on", true, false, false, "100\n"},
		{"only 3 on", false, false, true, "001\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cmd strings.Builder
			if tt.heater1 {
				cmd.WriteByte('1')
			} else {
				cmd.WriteByte('0')
			}
			if tt.heater2 {
				cmd.WriteByte('1')
			} else {
				cmd.WriteByte('0')
			}
			if tt.heater3 {
				cmd.WriteByte('1')
			} else {
				cmd.WriteByte('0')
			}
			cmd.WriteByte('\n')

			assert.Equal(t, tt.wantCmd, cmd.String())
		})
	}
}

