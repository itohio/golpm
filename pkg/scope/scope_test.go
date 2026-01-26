package scope

import (
	"testing"
)

func TestSnapToMultiples(t *testing.T) {
	tests := []struct {
		name     string
		min      float64
		max      float64
		wantMin  float64
		wantMax  float64
	}{
		{
			name:    "19.9-30.1 should snap to 10-40",
			min:      19.9,
			max:      30.1,
			wantMin:  10,
			wantMax:  40,
		},
		{
			name:    "11-60.9 should snap to 10-70",
			min:      11,
			max:      60.9,
			wantMin:  10,
			wantMax:  70,
		},
		{
			name:    "458-462 should snap to 450-470",
			min:      458,
			max:      462,
			wantMin:  450,
			wantMax:  470,
		},
		{
			name:    "45.8-46.2 should snap to 40-50",
			min:      45.8,
			max:      46.2,
			wantMin:  40,
			wantMax:  50,
		},
		{
			name:    "4.58-4.62 should snap to 4-5",
			min:      4.58,
			max:      4.62,
			wantMin:  4,
			wantMax:  5,
		},
		{
			name:    "0.458-0.462 should snap to 0.4-0.5",
			min:      0.458,
			max:      0.462,
			wantMin:  0.4,
			wantMax:  0.5,
		},
		{
			name:    "symmetric range -0.05 to 0.05",
			min:      -0.05,
			max:      0.05,
			wantMin:  -0.1,
			wantMax:  0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMin, gotMax := snapToMultiples(tt.min, tt.max)
			if gotMin != tt.wantMin {
				t.Errorf("snapToMultiples(%v, %v) min = %v, want %v", tt.min, tt.max, gotMin, tt.wantMin)
			}
			if gotMax != tt.wantMax {
				t.Errorf("snapToMultiples(%v, %v) max = %v, want %v", tt.min, tt.max, gotMax, tt.wantMax)
			}
		})
	}
}
