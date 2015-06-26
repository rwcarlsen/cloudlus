package optim

import (
	"math"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/gonum/matrix/mat64"
)

// RandPop generates n randomly positioned points in the boxed bounds defined by
// low and up.  The number of dimensions is equal to len(low).  Returned
// points have their values initialized to +infinity.
func RandPop(n int, low, up []float64) []*Point {
	if len(low) != len(up) {
		panic("low and up vectors are not same length")
	}

	ndims := len(low)

	points := make([]*Point, n)
	for i := 0; i < n; i++ {
		pos := make([]float64, ndims)
		for j := range pos {
			pos[j] = low[j] + RandFloat()*(up[j]-low[j])
		}
		points[i] = &Point{pos, math.Inf(1)}
	}
	return points
}

// RandPopConstr generates a random population of n feasible points satisfying
// the linear constraints "low <= Ax <= up". lb and ub define lower and upper
// box bounds for the variables.  Points are generated in the range
// 2*(ub[i]-lb[i]) centered around the box bounds - twice the range of the box
// bounds in each dimension. Infeasible/out-of-box points are projected to the
// nearest positions on the feasible region.  This allows problems with a
// relatively small feasible region within the box bounds to have points
// distributed around the entire outer surface (all sides) of the feasible
// region.
func RandPopConstr(n int, lb, ub []float64, low, A, up *mat64.Dense) []*Point {
	stackA, b, _ := StackConstrBoxed(lb, ub, low, A, up)
	_, ndims := A.Dims()

	points := make([]*Point, 0, n)
	for i := 0; i < n; i++ {
		pos := make([]float64, ndims)
		for j := range pos {
			l, u := lb[j], ub[j]
			pos[j] = l + RandFloat()*2*(u-l) - (u-l)/2
		}

		// project onto feasible region
		proj, _ := Project(pos, stackA, b)
		points = append(points, &Point{proj, math.Inf(1)})
	}

	return points
}
