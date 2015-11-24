package scen

import (
	"math"
	"testing"
)

func TestIntegrateMid(t *testing.T) {
	tests := []struct {
		fn     smoothFn
		x1, x2 float64
		Tot    float64
	}{
		// linear
		{func(x float64) float64 { return 0.5 * x }, 0.0, 1.0, 0.25},
		// normal distribution
		{func(x float64) float64 { return 1 / math.Sqrt(2*math.Pi) * math.Exp(-(x*x)/2) }, -100, 100, 1.0},
		// normal distribution half
		{func(x float64) float64 { return 1 / math.Sqrt(2*math.Pi) * math.Exp(-(x*x)/2) }, -100, 0, 0.5},
		// normal distribution segment
		{func(x float64) float64 { return 1 / math.Sqrt(2*math.Pi) * math.Exp(-(x*x)/2) }, -2, -1, .1359051219835},
	}

	for i, test := range tests {
		got := integrateMid(test.fn, test.x1, test.x2, 10000)
		if diff := math.Abs(got - test.Tot); diff > 1e-10 {
			t.Errorf("case %v (integral from %v to %v): got %v, want %v", i, test.x1, test.x2, got, test.Tot)
		}
	}
}

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
		{0.0, 0}, // check extrapolation beyond sample x-vals
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
		{7.0, 10}, // check extrapolation beyond sample x-vals
	}

	for i, test := range tests {
		gotY := fn(test.X)
		if diff := math.Abs(gotY - test.WantY); diff > 1e-10 {
			t.Errorf("case %v: fn[%v] = %v, want %v", i, test.X, gotY, test.WantY)
		}
	}
}
