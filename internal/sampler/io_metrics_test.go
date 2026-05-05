package sampler

import "testing"

func TestNormalizeBytesPerSec(t *testing.T) {
	const max = 1000.0
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{name: "zero", in: 0, want: 0},
		{name: "half", in: 500, want: 50},
		{name: "max", in: 1000, want: 100},
		{name: "over", in: 2000, want: 100},
		{name: "invalid max", in: 500, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			maxValue := max
			if tc.name == "invalid max" {
				maxValue = 0
			}
			if got := NormalizeBytesPerSec(tc.in, maxValue); got != tc.want {
				t.Fatalf("NormalizeBytesPerSec(%v, %v) = %v, want %v", tc.in, maxValue, got, tc.want)
			}
		})
	}
}

func TestNormalizeDiskActiveTimePct(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{name: "zero", in: 0, want: 0},
		{name: "mid", in: 42.5, want: 42.5},
		{name: "max", in: 100, want: 100},
		{name: "over", in: 150, want: 100},
		{name: "negative", in: -10, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeDiskActiveTimePct(tc.in); got != tc.want {
				t.Fatalf("NormalizeDiskActiveTimePct(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
