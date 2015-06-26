package optim

import (
	"math"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/gonum/matrix/mat64"
)

// OrthoProj computes the orthogonal projection of x0 onto the affine subspace
// defined by Ax=b which is the intersection of affine halfspaces
// (hyperplanes) that constitute the rows of A with associated shifts in b.
// The equation is:
//
//    proj = [I - A^T * (A * A^T)^-1 * A]*x0 + A^T * (A * A^T)^-1 * b
//
// where x0 is the point being projected and I is the identity matrix.  A is
// an m by n matrix where m <= n. if m == n, the returned result is the
// solution to the system A*x0=b.  The rows of A should always be linearly
// independent, otherwise OrthoProj may return mat64.ErrSingular.
func OrthoProj(x0 []float64, A, b *mat64.Dense) ([]float64, error) {
	x := mat64.NewDense(len(x0), 1, x0)

	m, n := A.Dims()
	if m >= n {
		proj, err := mat64.Solve(A, b)
		if err != nil {
			return nil, err
		}
		return proj.Col(nil, 0), nil
	}

	Atrans := &mat64.Dense{}
	Atrans.TCopy(A)

	AAtrans := &mat64.Dense{}
	AAtrans.Mul(A, Atrans)

	// B = A^T * (A*A^T)^-1
	B := &mat64.Dense{}
	inv, err := mat64.Inverse(AAtrans)
	if err != nil {
		return nil, err
	}
	B.Mul(Atrans, inv)

	n, _ = B.Dims()

	tmp := &mat64.Dense{}
	tmp.Mul(B, A)
	tmp.Sub(eye(n), tmp)
	tmp.Mul(tmp, x)

	tmp2 := &mat64.Dense{}
	tmp2.Mul(B, b)
	tmp.Add(tmp, tmp2)

	return tmp.Col(nil, 0), nil
}

func eye(n int) *mat64.Dense {
	m := mat64.NewDense(n, n, nil)

	for i := 0; i < n; i++ {
		m.Set(i, i, 1)
	}
	return m
}

// Project returns the nearest point to x0 that doesn't violate constraints in
// the equation Ax <= b.
func Project(x0 []float64, A, b *mat64.Dense) (proj []float64, success bool) {
	from := x0
	proj = x0
	var badA *mat64.Dense
	var badb *mat64.Dense
	failcount := 0
	for {
		Aviol, bviol := mostviolated(proj, A, b)

		if Aviol == nil { // projection is complete
			return proj, true
		} else {
			if badA == nil {
				badA, badb = Aviol, bviol
			} else {
				tmpA, tmpb := badA, badb
				badA, badb = &mat64.Dense{}, &mat64.Dense{}
				badA.Stack(tmpA, Aviol)
				badb.Stack(tmpb, bviol)
			}
		}

		newproj, err := OrthoProj(from, badA, badb)
		if err != nil {
			failcount++
			from = proj
			badA, badb = nil, nil
			if failcount == 2 {
				return proj, false
			}
		} else {
			proj = newproj
		}
	}
}

// mostviolated returns the most violated constraint in the system Ax <= b.
// Aviol and b each have one row and len(x0) columns. It returns nil, nil if
// x0 violates no constraints.  The most violated constraint is the one where
// the (orthogonal) distance from x0 to the constraint/hyperplane is largest.
func mostviolated(x0 []float64, A, b *mat64.Dense) (Aviol, bviol *mat64.Dense) {
	eps := 1e-7

	ax := &mat64.Dense{}
	xm := mat64.NewDense(len(x0), 1, x0)
	ax.Mul(A, xm)
	m, _ := ax.Dims()

	farthest := 0.0
	worstRow := -1
	for i := 0; i < m; i++ {
		if diff := ax.At(i, 0) - b.At(i, 0); diff > eps {
			// compute distance from x0 to plane of this violated constraint
			d := diff / l2norm(A.Row(nil, i))
			if d > farthest {
				farthest = d
				worstRow = i
			}
		}
	}
	if worstRow == -1 {
		return nil, nil
	}

	return mat64.NewDense(1, len(x0), A.Row(nil, worstRow)), mat64.NewDense(1, 1, b.Row(nil, worstRow))
}

func l2norm(x []float64) float64 {
	tot := 0.0
	for _, xi := range x {
		tot += xi * xi
	}
	return math.Sqrt(tot)
}
