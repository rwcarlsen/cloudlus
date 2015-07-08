package bench_test

import (
	"database/sql"
	"math"
	"math/rand"
	"testing"

	_ "github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/go-sqlite/sqlite3"
	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/optim"
	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/optim/bench"
	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/optim/pattern"
	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/optim/swarm"
)

const (
	maxeval      = 50000
	maxiter      = 5000
	maxnoimprove = 500
	minstep      = 1e-8
)

const seed = 7

func init() { bench.BenchSeed = seed }

func TestBenchSwarmRosen(t *testing.T) {
	ndim := 30
	npar := 30
	maxiter := 10000
	successfrac := 1.00
	avgiter := 500.0

	fn := bench.Rosenbrock{ndim}
	sfn := func() *optim.Solver {
		return &optim.Solver{
			Method:  swarmsolver(fn, nil, npar),
			Obj:     optim.Func(fn.Eval),
			MaxEval: maxiter * npar,
			MaxIter: maxiter,
		}
	}
	bench.Benchmark(t, fn, sfn, successfrac, avgiter)
}

func TestBenchPSwarmRosen(t *testing.T) {
	ndim := 30
	npar := 30
	maxiter := 10000
	successfrac := 1.0
	avgiter := 300.0

	fn := bench.Rosenbrock{ndim}
	sfn := func() *optim.Solver {
		m, mesh := pswarmsolver(fn, nil, npar)
		low, _ := fn.Bounds()
		ndim := len(low)
		m.Poller = &pattern.Poller{Spanner: &pattern.RandomN{N: ndim}}
		return &optim.Solver{
			Method:  m,
			Obj:     optim.Func(fn.Eval),
			Mesh:    mesh,
			MaxEval: maxiter * npar,
			MaxIter: maxiter,
		}
	}
	bench.Benchmark(t, fn, sfn, successfrac, avgiter)
}

func TestBenchPSwarmGriewank(t *testing.T) {
	ndim := 30
	npar := 30
	maxiter := 10000
	successfrac := 1.00
	avgiter := 220.0

	fn := bench.Griewank{ndim}
	sfn := func() *optim.Solver {
		m, mesh := pswarmsolver(fn, nil, npar)
		return &optim.Solver{
			Method:  m,
			Obj:     optim.Func(fn.Eval),
			Mesh:    mesh,
			MaxEval: maxiter * npar,
			MaxIter: maxiter,
		}
	}
	bench.Benchmark(t, fn, sfn, successfrac, avgiter)
}

func TestBenchPSwarmRastrigrin(t *testing.T) {
	ndim := 30
	npar := 30
	maxiter := 10000
	successfrac := 1.00
	avgiter := 130.0

	fn := bench.Rastrigrin{ndim}
	sfn := func() *optim.Solver {
		m, mesh := pswarmsolver(fn, nil, npar)
		return &optim.Solver{
			Method:  m,
			Obj:     optim.Func(fn.Eval),
			Mesh:    mesh,
			MaxEval: maxiter * npar,
			MaxIter: maxiter,
		}
	}
	bench.Benchmark(t, fn, sfn, successfrac, avgiter)
}

func TestOverviewPattern(t *testing.T) {
	maxeval := 50000
	maxiter := 5000
	successfrac := 0.50
	avgiter := 2500.0

	// ONLY test plain pattern search on convex functions
	for _, fn := range []bench.Func{bench.Rosenbrock{NDim: 2}} {
		sfn := func() *optim.Solver {
			m, mesh := patternsolver(fn, nil)
			m.Poller = &pattern.Poller{Spanner: pattern.CompassNp1{}}
			return &optim.Solver{
				Method:  m,
				Obj:     optim.Func(fn.Eval),
				Mesh:    mesh,
				MaxIter: maxiter,
				MaxEval: maxeval,
			}
		}
		bench.Benchmark(t, fn, sfn, successfrac, avgiter)
	}
}

func TestOverviewSwarm(t *testing.T) {
	maxeval := 50000
	maxiter := 5000
	successfrac := 1.00
	avgiter := 250.0

	for _, fn := range bench.Basic {
		sfn := func() *optim.Solver {
			return &optim.Solver{
				Method:  swarmsolver(fn, nil, -1),
				Obj:     optim.Func(fn.Eval),
				MaxEval: maxeval,
				MaxIter: maxiter,
			}
		}
		bench.Benchmark(t, fn, sfn, successfrac, avgiter)
	}
}

func TestOverviewPSwarm(t *testing.T) {
	maxeval := 50000
	maxiter := 5000
	successfrac := .90
	avgiter := 250.00

	for _, fn := range bench.Basic {
		sfn := func() *optim.Solver {
			it, m := pswarmsolver(fn, nil, -1)
			return &optim.Solver{
				Method:  it,
				Obj:     optim.Func(fn.Eval),
				Mesh:    m,
				MaxEval: maxeval,
				MaxIter: maxiter,
			}
		}
		bench.Benchmark(t, fn, sfn, successfrac, avgiter)
	}
}

func patternsolver(fn bench.Func, db *sql.DB) (*pattern.Method, optim.Mesh) {
	low, up := fn.Bounds()
	max, min := up[0], low[0]
	mesh := &optim.InfMesh{StepSize: (max - min) / 10}
	p := initialpoint(fn)
	mesh.SetOrigin(p.Pos)
	return pattern.New(p, pattern.DB(db)), mesh
}

func swarmsolver(fn bench.Func, db *sql.DB, n int) optim.Method {
	low, up := fn.Bounds()

	if n < 0 {
		n = 30 + 1*len(low)
		if n > maxeval/500 {
			n = maxeval / 500
		}
	}

	return swarm.New(
		swarm.NewPopulationRand(n, low, up),
		swarm.VmaxBounds(fn.Bounds()),
		swarm.DB(db),
	)
}

func pswarmsolver(fn bench.Func, db *sql.DB, n int) (*pattern.Method, optim.Mesh) {
	low, up := fn.Bounds()
	max, min := up[0], low[0]
	mesh := &optim.InfMesh{StepSize: (max - min) / 10}
	p := initialpoint(fn)
	mesh.SetOrigin(p.Pos)

	return pattern.New(p,
		pattern.SearchMethod(swarmsolver(fn, db, n), pattern.Share),
		pattern.DB(db),
	), mesh
}

func initialpoint(fn bench.Func) *optim.Point {
	low, up := fn.Bounds()
	max, min := up[0], low[0]
	pos := make([]float64, len(low))
	for i := range low {
		pos[i] = rand.Float64()*(max-min) + min
	}
	return &optim.Point{pos, math.Inf(1)}
}
