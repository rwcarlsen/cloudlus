package optim

import (
	"math"
	"testing"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/gonum/matrix/mat64"
)

type Problem struct {
	Step       float64
	Point, Exp []float64
	Basis      *mat64.Dense
	Origin     []float64
}

var tests = []Problem{
	Problem{
		Step:   1.3,
		Basis:  nil,
		Origin: []float64{0, 0},
		Point:  []float64{0.1, 0.1},
		Exp:    []float64{0.0, 0.0},
	},
	Problem{
		Step:   1.3,
		Basis:  nil,
		Origin: []float64{0, 0},
		Point:  []float64{1.0, 1.0},
		Exp:    []float64{1.3, 1.3},
	},
	Problem{
		Step:   1.3,
		Basis:  nil,
		Origin: []float64{0, 0},
		Point:  []float64{1.9, 1.9},
		Exp:    []float64{1.3, 1.3},
	},
	Problem{ // 45 deg clockwise rotation of the identity basis
		Step:   1.0,
		Basis:  mat64.NewDense(2, 2, []float64{1 / math.Sqrt(2), 1 / math.Sqrt(2), -1 / math.Sqrt(2), 1 / math.Sqrt(2)}),
		Origin: []float64{0, 0},
		Point:  []float64{1.0, 1.0},
		Exp:    []float64{1 / math.Sqrt(2), 1 / math.Sqrt(2)},
	},
	Problem{ // non-zero origin
		Step:   1.0,
		Basis:  nil,
		Origin: []float64{0.2, 0.3},
		Point:  []float64{1.6, 2.1},
		Exp:    []float64{1.2, 2.3},
	},
}

func TestSimple(t *testing.T) {
	maxulps := uint64(1)

	for i, prob := range tests {
		m := &InfMesh{StepSize: prob.Step, Basis: prob.Basis, Center: prob.Origin}
		got := m.Nearest(prob.Point)
		t.Logf("prob %v:", i)
		for j := range got {
			if diff := DiffInUlps(got[j], prob.Exp[j]); diff > maxulps {
				t.Errorf("   (FAIL) v[%v]=%v: got %v, expected %v", j, prob.Point[j], got[j], prob.Exp[j])
			} else {
				t.Logf("   (pass) v[%v]=%v: got %v", j, prob.Point[j], got[j])
			}
		}
	}
}

func DiffInUlps(x, y float64) uint64 {
	switch {
	case math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0):
		return math.MaxInt64
	case x == y:
		return 0
	default:
		xi := math.Float64bits(x)
		yi := math.Float64bits(y)
		if xi > yi {
			return xi - yi
		}
		return yi - xi
	}
}
