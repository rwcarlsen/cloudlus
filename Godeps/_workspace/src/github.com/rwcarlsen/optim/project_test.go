package optim

import (
	"math"
	"testing"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/gonum/matrix/mat64"
)

type projtest struct {
	A    [][]float64
	b    []float64
	x0   []float64
	want []float64
}

func TestProject(t *testing.T) {
	eps := 1e-10
	var tests []projtest = []projtest{
		{
			A: [][]float64{
				{2, 1},
				{-4, 1},
			},
			b:    []float64{2, 0},
			x0:   []float64{1, 2},
			want: []float64{1.0 / 3, 4.0 / 3},
		},
		{
			A: [][]float64{
				{2, 1},
				{-4, 1},
			},
			b: []float64{2, 0},
			// this point violates both constraints
			x0:   []float64{0.5, 100},
			want: []float64{1.0 / 3, 4.0 / 3},
		},
		{
			A: [][]float64{
				{2, 1},
				{-4, 1},
			},
			b: []float64{2, 0},
			// this point is right on the orthogonal projection to the
			// intersection of both constraints.
			x0:   []float64{1, 10.0 / 6},
			want: []float64{1.0 / 3, 4.0 / 3},
		},
		{
			A: [][]float64{
				{2, 1},
				{-4, 1},
			},
			b: []float64{2, 0},
			// this point is right below the orthogonal projection to the
			// intersection of both constraints - and is the first point where
			// the wanted projection is not at the intersection of the two
			// constraints.
			x0:   []float64{1, 9.0 / 6},
			want: []float64{0.4, 1.2},
		},
	}

	for n, test := range tests {
		adata := []float64{}
		for _, vals := range test.A {
			adata = append(adata, vals...)
		}
		A := mat64.NewDense(len(test.A), len(test.A[0]), adata)
		b := mat64.NewDense(len(test.b), 1, test.b)
		got, _ := Project(test.x0, A, b)

		for i := range got {
			if diff := math.Abs(got[i] - test.want[i]); diff > eps {
				t.Errorf("FAIL test %v proj[%v]: want %v, got %v", n, i, test.want[i], got[i])
			} else {
				t.Logf("pass test %v proj[%v]: got %v", n, i, got[i])
			}
		}
	}
}

func TestOrthoProj(t *testing.T) {
	eps := 1e-10
	var tests []projtest = []projtest{
		{
			A: [][]float64{
				{2, 1},
			},
			b:    []float64{2},
			x0:   []float64{1, 2},
			want: []float64{0.20, 1.60},
		},
		{
			A: [][]float64{
				{2, 1},
				{-1, 1},
			},
			b:    []float64{2, 0.6},
			x0:   []float64{1, 2},
			want: []float64{3.2/3 - 0.6, 3.2 / 3},
		},
		{
			A: [][]float64{
				{2, 1},
				{-4, 1},
			},
			b:    []float64{2, 0},
			x0:   []float64{1, 2},
			want: []float64{1.0 / 3, 4.0 / 3},
		},
	}

	// make one big multi-dimensional test projection (single constraint)
	n := 42
	xmax := 10 * float64(n)
	A := [][]float64{make([]float64, n)}
	b := []float64{xmax}
	x0 := make([]float64, n)
	want := make([]float64, n)
	for i := range A[0] {
		A[0][i] = 1
		x0[i] = xmax
		want[i] = 10
	}
	bigtest := projtest{A: A, b: b, x0: x0, want: want}
	tests = append(tests, bigtest)

	for n, test := range tests {
		adata := []float64{}
		for _, vals := range test.A {
			adata = append(adata, vals...)
		}
		A := mat64.NewDense(len(test.A), len(test.A[0]), adata)
		b := mat64.NewDense(len(test.b), 1, test.b)
		got, err := OrthoProj(test.x0, A, b)
		if err != nil {
			t.Fatal(err)
		}

		for i := range got {
			if diff := math.Abs(got[i] - test.want[i]); diff > eps {
				t.Errorf("FAIL test %v proj[%v]: want %v, got %v", n, i, test.want[i], got[i])
			} else {
				t.Logf("pass test %v proj[%v]: got %v", n, i, got[i])
			}
		}
	}
}
