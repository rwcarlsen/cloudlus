package optim

import "math"

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
