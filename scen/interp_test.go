package scen

import (
	"math"
	"testing"
)

func TestInterpolate(t *testing.T) {
	samples := []sample{
		{1, 1},
		{2, 2},
		{3, 3},
		{4, 3},
		{5, 4},
		{6, 7},
	}

	fn := interpolate(samples)

	tests := []struct {
		X     float64
		WantY float64
	}{
		{0.0, 0},
		{0.5, 0.5},
		{1.0, 1},
		{1.2, 1.2},
		{2.0, 2},
		{2.7, 2.7},
		{3.0, 3},
		{3.5, 3},
		{4.0, 3},
		{4.5, 3.5},
		{5.0, 4},
		{5.5, 5.5},
		{6.0, 7},
		{6.2, 7.6},
		{7.0, 10},
	}

	for i, test := range tests {
		gotY := fn(test.X)
		if diff := math.Abs(gotY - test.WantY); diff > 1e-10 {
			t.Errorf("case %v: fn[%v] = %v, want %v", i, test.X, gotY, test.WantY)
		}
	}
}
