package sample

import (
	"testing"
	"time"

	"github.com/itohio/golpm/pkg/config"
	"github.com/itohio/golpm/pkg/lpm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestADCToVoltage(t *testing.T) {
	tests := []struct {
		name string
		adc  uint16
		vref float64
		want float64
	}{
		{
			name: "zero ADC",
			adc:  0,
			vref: 3.3,
			want: 0.0,
		},
		{
			name: "max ADC",
			adc:  4095,
			vref: 3.3,
			want: 3.3,
		},
		{
			name: "half ADC",
			adc:  2047,
			vref: 3.3,
			want: 1.65, // Approximately
		},
		{
			name: "quarter ADC",
			adc:  1024,
			vref: 3.3,
			want: 0.825, // Approximately
		},
		{
			name: "different VRef",
			adc:  2047,
			vref: 5.0,
			want: 2.5, // Approximately
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adcToVoltage(tt.adc, tt.vref)
			assert.InDelta(t, tt.want, got, 0.01, "adcToVoltage(%d, %f) = %f, want %f", tt.adc, tt.vref, got, tt.want)
		})
	}
}

func TestVoltageDivider(t *testing.T) {
	tests := []struct {
		name string
		vout float64
		r1   float64
		r2   float64
		want float64
	}{
		{
			name: "equal resistors",
			vout: 1.65,
			r1:   20000,
			r2:   20000,
			want: 3.3, // V_in = V_out * (R1+R2)/R2 = 1.65 * 2 = 3.3
		},
		{
			name: "unequal resistors",
			vout: 1.0,
			r1:   30000,
			r2:   10000,
			want: 4.0, // V_in = V_out * (R1+R2)/R2 = 1.0 * 4 = 4.0
		},
		{
			name: "zero output",
			vout: 0.0,
			r1:   20000,
			r2:   20000,
			want: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := voltageDivider(tt.vout, tt.r1, tt.r2)
			assert.InDelta(t, tt.want, got, 0.01, "voltageDivider(%f, %f, %f) = %f, want %f", tt.vout, tt.r1, tt.r2, got, tt.want)
		})
	}
}

func TestCalculateHeaterPower(t *testing.T) {
	cfg := &config.Config{
		Heaters: []config.HeaterConfig{
			{Resistance: 2300}, // H1
			{Resistance: 500},  // H2
			{Resistance: 200},  // H3
		},
	}

	tests := []struct {
		name    string
		voltage float64
		heater1 bool
		heater2 bool
		heater3 bool
		want    float64
		heaters []config.HeaterConfig
	}{
		{
			name:    "no heaters",
			voltage: 5.0,
			heater1: false,
			heater2: false,
			heater3: false,
			want:    0.0,
			heaters: cfg.Heaters,
		},
		{
			name:    "heater 1 only",
			voltage: 5.0,
			heater1: true,
			heater2: false,
			heater3: false,
			want:    25.0 / 2300.0, // V²/R = 25/2300 ≈ 0.01087
			heaters: cfg.Heaters,
		},
		{
			name:    "heater 2 only",
			voltage: 5.0,
			heater1: false,
			heater2: true,
			heater3: false,
			want:    25.0 / 500.0, // V²/R = 25/500 = 0.05
			heaters: cfg.Heaters,
		},
		{
			name:    "heater 3 only",
			voltage: 5.0,
			heater1: false,
			heater2: false,
			heater3: true,
			want:    25.0 / 200.0, // V²/R = 25/200 = 0.125
			heaters: cfg.Heaters,
		},
		{
			name:    "all heaters",
			voltage: 5.0,
			heater1: true,
			heater2: true,
			heater3: true,
			want:    (25.0 / 2300.0) + (25.0 / 500.0) + (25.0 / 200.0),
			heaters: cfg.Heaters,
		},
		{
			name:    "heaters 1 and 2",
			voltage: 5.0,
			heater1: true,
			heater2: true,
			heater3: false,
			want:    (25.0 / 2300.0) + (25.0 / 500.0),
			heaters: cfg.Heaters,
		},
		{
			name:    "insufficient heaters config",
			voltage: 5.0,
			heater1: true,
			heater2: false,
			heater3: false,
			want:    0.0,
			heaters: []config.HeaterConfig{{Resistance: 2300}}, // Only 1 heater
		},
		{
			name:    "zero resistance",
			voltage: 5.0,
			heater1: true,
			heater2: false,
			heater3: false,
			want:    0.0,
			heaters: []config.HeaterConfig{
				{Resistance: 0}, // Zero resistance
				{Resistance: 500},
				{Resistance: 200},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateHeaterPower(tt.voltage, tt.heater1, tt.heater2, tt.heater3, tt.heaters)
			assert.InDelta(t, tt.want, got, 0.0001, "calculateHeaterPower(%f, %v, %v, %v, ...) = %f, want %f",
				tt.voltage, tt.heater1, tt.heater2, tt.heater3, got, tt.want)
		})
	}
}

func TestConvertSample(t *testing.T) {
	cfg := config.Default()
	now := time.Now()

	tests := []struct {
		name string
		raw  lpm.RawSample
		want Sample
	}{
		{
			name: "zero ADC values",
			raw: lpm.RawSample{
				Timestamp: now,
				Reading:   0,
				Voltage:   0,
				Heater1:   false,
				Heater2:   false,
				Heater3:   false,
			},
			want: Sample{
				Timestamp:   now,
				Reading:     0.0,
				Voltage:     0.0,
				HeaterPower: 0.0,
			},
		},
		{
			name: "max ADC values, no heaters",
			raw: lpm.RawSample{
				Timestamp: now,
				Reading:   4095,
				Voltage:   4095,
				Heater1:   false,
				Heater2:   false,
				Heater3:   false,
			},
			want: Sample{
				Timestamp:   now,
				Reading:     3.3, // VRef
				Voltage:     6.6, // After divider: 3.3V * (R1+R2)/R2 = 3.3 * 2 = 6.6V
				HeaterPower: 0.0,
			},
		},
		{
			name: "half ADC, heater 1 on",
			raw: lpm.RawSample{
				Timestamp: now,
				Reading:   2047,
				Voltage:   2047,
				Heater1:   true,
				Heater2:   false,
				Heater3:   false,
			},
			want: Sample{
				Timestamp:   now,
				Reading:     1.65,                 // Approximately
				Voltage:     3.3,                  // After divider: 1.65V * 2 = 3.3V
				HeaterPower: (3.3 * 3.3) / 2300.0, // V²/R
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertSample(tt.raw, cfg)
			require.NoError(t, err)
			assert.Equal(t, tt.want.Timestamp, got.Timestamp)
			assert.InDelta(t, tt.want.Reading, got.Reading, 0.01)
			assert.InDelta(t, tt.want.Voltage, got.Voltage, 0.01)
			assert.InDelta(t, tt.want.HeaterPower, got.HeaterPower, 0.001)
		})
	}
}

func TestNewConverter_ChannelProcessing(t *testing.T) {
	cfg := config.Default()
	converter := NewConverter(cfg, 10)

	in := make(chan lpm.RawSample, 5)
	out := converter(in)

	// Send some samples
	now := time.Now()
	for i := 0; i < 3; i++ {
		in <- lpm.RawSample{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Reading:   uint16(2047 + i*100),
			Voltage:   uint16(2047 + i*100),
			Heater1:   i%2 == 0,
			Heater2:   false,
			Heater3:   false,
		}
	}

	close(in)

	// Read all samples
	var samples []Sample
	for sample := range out {
		samples = append(samples, sample)
	}

	assert.Len(t, samples, 3, "Should receive 3 samples")
	for i, s := range samples {
		assert.Equal(t, now.Add(time.Duration(i)*time.Second), s.Timestamp)
		assert.Greater(t, s.Reading, float64(0))
		assert.Greater(t, s.Voltage, float64(0))
	}
}

func TestNewConverter_EmptyChannel(t *testing.T) {
	cfg := config.Default()
	converter := NewConverter(cfg, 10)

	in := make(chan lpm.RawSample)
	out := converter(in)

	close(in)

	// Should close immediately
	_, ok := <-out
	assert.False(t, ok, "Output channel should be closed")
}
