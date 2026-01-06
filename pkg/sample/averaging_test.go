package sample

import (
	"testing"
	"time"

	"github.com/itohio/golpm/pkg/config"
	"github.com/itohio/golpm/pkg/lpm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAveragingConverter_BasicAveraging(t *testing.T) {
	cfg := config.Default()
	converter := NewAveragingConverter(cfg, 3, 10)

	in := make(chan lpm.RawSample, 10)
	out := converter(in)

	now := time.Now()

	// Send 5 samples with increasing values
	for i := 0; i < 5; i++ {
		in <- lpm.RawSample{
			Timestamp: now.Add(time.Duration(i) * time.Millisecond),
			Reading:   uint16(1000 + i*100),
			Voltage:   uint16(2000 + i*100),
			Heater1:   i%2 == 0,
			Heater2:   false,
			Heater3:   false,
		}
	}

	// Wait a bit for ticker to fire
	time.Sleep(150 * time.Millisecond)

	close(in)

	// Read samples
	var samples []Sample
	for sample := range out {
		samples = append(samples, sample)
	}

	// Should have at least one averaged sample
	assert.Greater(t, len(samples), 0, "Should receive at least one averaged sample")

	// Check that values are reasonable (averaged)
	for _, s := range samples {
		assert.Greater(t, s.Reading, float64(0))
		assert.Greater(t, s.Voltage, float64(0))
	}
}

func TestNewAveragingConverter_WindowSize(t *testing.T) {
	cfg := config.Default()
	converter := NewAveragingConverter(cfg, 5, 10)

	in := make(chan lpm.RawSample, 10)
	out := converter(in)

	now := time.Now()

	// Send 10 samples with constant value
	constValue := uint16(2047)
	for i := 0; i < 10; i++ {
		in <- lpm.RawSample{
			Timestamp: now.Add(time.Duration(i) * time.Millisecond),
			Reading:   constValue,
			Voltage:   constValue,
			Heater1:   false,
			Heater2:   false,
			Heater3:   false,
		}
	}

	time.Sleep(150 * time.Millisecond)
	close(in)

	var samples []Sample
	for sample := range out {
		samples = append(samples, sample)
	}

	// Should have averaged samples
	assert.Greater(t, len(samples), 0)
}

func TestNewAveragingConverter_EmptyChannel(t *testing.T) {
	cfg := config.Default()
	converter := NewAveragingConverter(cfg, 3, 10)

	in := make(chan lpm.RawSample)
	out := converter(in)

	close(in)

	// Should close immediately (no samples to average)
	_, ok := <-out
	assert.False(t, ok, "Output channel should be closed")
}

func TestNewAveragingConverter_InvalidWindowSize(t *testing.T) {
	cfg := config.Default()
	converter := NewAveragingConverter(cfg, 0, 10) // Invalid window size

	in := make(chan lpm.RawSample, 5)
	out := converter(in)

	now := time.Now()
	in <- lpm.RawSample{
		Timestamp: now,
		Reading:   2047,
		Voltage:   2047,
		Heater1:   false,
		Heater2:   false,
		Heater3:   false,
	}

	time.Sleep(150 * time.Millisecond)
	close(in)

	// Should still process (window size defaults to 1)
	var samples []Sample
	for sample := range out {
		samples = append(samples, sample)
	}

	assert.Greater(t, len(samples), 0)
}

func TestAverageAndConvertSamples(t *testing.T) {
	cfg := config.Default()
	now := time.Now()

	tests := []struct {
		name    string
		samples []lpm.RawSample
		wantErr bool
	}{
		{
			name:    "empty samples",
			samples: []lpm.RawSample{},
			wantErr: false, // Returns zero sample, no error
		},
		{
			name: "single sample",
			samples: []lpm.RawSample{
				{
					Timestamp: now,
					Reading:   2047,
					Voltage:   2047,
					Heater1:   false,
					Heater2:   false,
					Heater3:   false,
				},
			},
			wantErr: false,
		},
		{
			name: "multiple samples",
			samples: []lpm.RawSample{
				{
					Timestamp: now,
					Reading:   1000,
					Voltage:   2000,
					Heater1:   false,
					Heater2:   false,
					Heater3:   false,
				},
				{
					Timestamp: now.Add(time.Millisecond),
					Reading:   1100,
					Voltage:   2100,
					Heater1:   true,
					Heater2:   false,
					Heater3:   false,
				},
				{
					Timestamp: now.Add(2 * time.Millisecond),
					Reading:   1200,
					Voltage:   2200,
					Heater1:   true,
					Heater2:   true,
					Heater3:   false,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sample, err := averageAndConvertSamples(tt.samples, cfg)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if len(tt.samples) > 0 {
					// Check that timestamp is from last sample
					assert.Equal(t, tt.samples[len(tt.samples)-1].Timestamp, sample.Timestamp)
					// Check that heater states are from last sample
					last := tt.samples[len(tt.samples)-1]
					expectedPower := calculateHeaterPower(
						voltageDivider(
							adcToVoltage(uint16((float64(tt.samples[0].Voltage)+float64(tt.samples[len(tt.samples)-1].Voltage))/2+0.5), cfg.VoltageDivider.VRef),
							cfg.VoltageDivider.R1,
							cfg.VoltageDivider.R2,
						),
						last.Heater1,
						last.Heater2,
						last.Heater3,
						cfg.Heaters,
					)
					// Power should match (approximately, due to averaging)
					assert.InDelta(t, expectedPower, sample.HeaterPower, 0.01)
				}
			}
		})
	}
}

func TestNewAveragingConverterForSamples(t *testing.T) {
	converter := NewAveragingConverterForSamples(3, 10)

	in := make(chan Sample, 10)
	out := converter(in)

	now := time.Now()

	// Send 5 samples
	for i := 0; i < 5; i++ {
		in <- Sample{
			Timestamp:   now.Add(time.Duration(i) * time.Millisecond),
			Reading:     float64(1.0 + float64(i)*0.1),
			Voltage:     float64(2.0 + float64(i)*0.1),
			HeaterPower: float64(0.01 + float64(i)*0.001),
		}
	}

	time.Sleep(150 * time.Millisecond)
	close(in)

	var samples []Sample
	for sample := range out {
		samples = append(samples, sample)
	}

	assert.Greater(t, len(samples), 0)

	// Check that values are averaged
	for _, s := range samples {
		assert.Greater(t, s.Reading, float64(0))
		assert.Greater(t, s.Voltage, float64(0))
		assert.GreaterOrEqual(t, s.HeaterPower, float64(0))
	}
}

func TestAverageConvertedSamples(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		samples []Sample
		want    Sample
	}{
		{
			name:    "empty samples",
			samples: []Sample{},
			want:    Sample{},
		},
		{
			name: "single sample",
			samples: []Sample{
				{
					Timestamp:   now,
					Reading:     1.0,
					Voltage:     2.0,
					HeaterPower: 0.01,
				},
			},
			want: Sample{
				Timestamp:   now,
				Reading:     1.0,
				Voltage:     2.0,
				HeaterPower: 0.01,
			},
		},
		{
			name: "multiple samples",
			samples: []Sample{
				{
					Timestamp:   now,
					Reading:     1.0,
					Voltage:     2.0,
					HeaterPower: 0.01,
				},
				{
					Timestamp:   now.Add(time.Millisecond),
					Reading:     1.1,
					Voltage:     2.1,
					HeaterPower: 0.011,
				},
				{
					Timestamp:   now.Add(2 * time.Millisecond),
					Reading:     1.2,
					Voltage:     2.2,
					HeaterPower: 0.012,
				},
			},
			want: Sample{
				Timestamp:   now.Add(2 * time.Millisecond),
				Reading:     1.1,      // (1.0 + 1.1 + 1.2) / 3
				Voltage:     2.1,      // (2.0 + 2.1 + 2.2) / 3
				HeaterPower: 0.011,    // (0.01 + 0.011 + 0.012) / 3
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := averageConvertedSamples(tt.samples)
			if len(tt.samples) == 0 {
				assert.Equal(t, tt.want, got)
			} else {
				assert.Equal(t, tt.want.Timestamp, got.Timestamp)
				assert.InDelta(t, tt.want.Reading, got.Reading, 0.001)
				assert.InDelta(t, tt.want.Voltage, got.Voltage, 0.001)
				assert.InDelta(t, tt.want.HeaterPower, got.HeaterPower, 0.0001)
			}
		})
	}
}

