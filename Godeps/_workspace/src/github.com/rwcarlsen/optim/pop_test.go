package optim

import (
	"testing"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/gonum/matrix/mat64"
)

func TestRandPopConstr(t *testing.T) {
	n := 100
	lb := []float64{0, 0, 0, 0, 0}
	ub := []float64{100, 100, 100, 100, 100}

	// single linear constraint is: x1+x2 <= 10
	// this results in a
	// (10 / 100) * (10 / 100) * 1/2 chance == 0.005
	// that a random point will be feasible
	low := mat64.NewDense(1, 1, []float64{0})
	up := mat64.NewDense(1, 1, []float64{100})
	A := mat64.NewDense(1, 5, []float64{1, 1, 0, 0, 0})

	points := RandPopConstr(n, lb, ub, low, A, up)

	// TODO: build out this test
	for i, p := range points {
		t.Logf("    point %v: %v", i, p)
	}
}
