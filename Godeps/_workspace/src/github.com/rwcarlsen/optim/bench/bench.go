// Package bench provides tools for testing solvers against benchmark
// optimization functions from
// http://en.wikipedia.org/wiki/Test_functions_for_optimization.
package bench

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/optim"
)

var (
	sin  = math.Sin
	cos  = math.Cos
	abs  = math.Abs
	exp  = math.Exp
	sqrt = math.Sqrt
)

var Basic = []Func{
	Ackley{},
	CrossTray{},
	//Eggholder{},
	HolderTable{},
	Schaffer2{},
	Rosenbrock{NDim: 2},
	Rosenbrock{NDim: 10},
	Griewank{NDim: 2},
	//Griewank{NDim: 10},
	Rastrigrin{NDim: 2},
	Rastrigrin{NDim: 10},
}

type Func interface {
	Eval(v []float64) float64
	Bounds() (low, up []float64)
	Optima() []*optim.Point
	// Tol returns a value below which the Func is considered
	// optimized/solved.
	Tol() float64
	Name() string
}

type Ackley struct{}

func (fn Ackley) Name() string { return "Ackley" }

func (fn Ackley) Eval(v []float64) float64 {
	if !InsideBounds(v, fn) {
		return math.Inf(1)
	}

	x := v[0]
	y := v[1]
	return -20*math.Exp(-0.2*math.Sqrt(0.5*(x*x+y*y))) -
		math.Exp(0.5*(math.Cos(2*math.Pi*x)+math.Cos(2*math.Pi*y))) +
		20 + math.E
}

func (fn Ackley) Tol() float64 { return .01 }

func (fn Ackley) Bounds() (low, up []float64) {
	return []float64{-5, -5}, []float64{5, 5}
}

func (fn Ackley) Optima() []*optim.Point {
	return []*optim.Point{
		&optim.Point{[]float64{0, 0}, 0},
	}
}

type CrossTray struct{}

func (fn CrossTray) Name() string { return "CrossTray" }

func (fn CrossTray) Tol() float64 { return fn.Optima()[0].Val + math.Abs(fn.Optima()[0].Val*.01) }

func (fn CrossTray) Eval(v []float64) float64 {
	if !InsideBounds(v, fn) {
		return math.Inf(1)
	}

	x := v[0]
	y := v[1]
	return -.0001 * math.Pow(abs(sin(x)*sin(y)*exp(abs(100-sqrt(x*x+y*y)/math.Pi)))+1, 0.1)
}

func (fn CrossTray) Bounds() (low, up []float64) {
	return []float64{-10, -10}, []float64{10, 10}
}

func (fn CrossTray) Optima() []*optim.Point {
	return []*optim.Point{
		&optim.Point{[]float64{1.34941, -1.34941}, -2.06261},
		&optim.Point{[]float64{1.34941, 1.34941}, -2.06261},
		&optim.Point{[]float64{-1.34941, 1.34941}, -2.06261},
		&optim.Point{[]float64{-1.34941, -1.34941}, -2.06261},
	}
}

type Eggholder struct{}

func (fn Eggholder) Name() string { return "Eggholder" }

func (fn Eggholder) Tol() float64 { return fn.Optima()[0].Val + math.Abs(fn.Optima()[0].Val*.01) }

func (fn Eggholder) Eval(v []float64) float64 {
	if !InsideBounds(v, fn) {
		return math.Inf(1)
	}

	x := v[0]
	y := v[1]
	return -(y+47)*sin(sqrt(abs(y+x/2+47))) - x*sin(sqrt(abs(x-(y+47))))
}

func (fn Eggholder) Bounds() (low, up []float64) {
	return []float64{-512, -512}, []float64{512, 512}
}

func (fn Eggholder) Optima() []*optim.Point {
	return []*optim.Point{
		&optim.Point{[]float64{512, 404.2319}, -959.6407},
	}
}

type HolderTable struct{}

func (fn HolderTable) Name() string { return "HolderTable" }

func (fn HolderTable) Tol() float64 { return fn.Optima()[0].Val + math.Abs(fn.Optima()[0].Val*.01) }

func (fn HolderTable) Eval(v []float64) float64 {
	if !InsideBounds(v, fn) {
		return math.Inf(1)
	}

	x := v[0]
	y := v[1]
	return -abs(sin(x) * cos(y) * exp(abs(1-sqrt(x*x+y*y)/math.Pi)))
}

func (fn HolderTable) Bounds() (low, up []float64) {
	return []float64{-10, -10}, []float64{10, 10}
}

func (fn HolderTable) Optima() []*optim.Point {
	return []*optim.Point{
		&optim.Point{[]float64{8.05502, 9.66459}, -19.2085},
		&optim.Point{[]float64{-8.05502, 9.66459}, -19.2085},
		&optim.Point{[]float64{8.05502, -9.66459}, -19.2085},
		&optim.Point{[]float64{-8.05502, -9.66459}, -19.2085},
	}
}

type Schaffer2 struct{}

func (fn Schaffer2) Tol() float64 { return .01 }

func (fn Schaffer2) Name() string { return "Schaffer2" }

func (fn Schaffer2) Eval(v []float64) float64 {
	if !InsideBounds(v, fn) {
		return math.Inf(1)
	}

	x := v[0]
	y := v[1]
	return 0.5 + (math.Pow(sin(x*x-y*y), 2)-0.5)/math.Pow(1+.0001*(x*x+y*y), 2)
}

func (fn Schaffer2) Bounds() (low, up []float64) {
	return []float64{-100, -100}, []float64{100, 100}
}

func (fn Schaffer2) Optima() []*optim.Point {
	return []*optim.Point{
		&optim.Point{[]float64{0, 0}, 0},
	}
}

type Styblinski struct {
	NDim int
}

func (fn Styblinski) Name() string { return fmt.Sprintf("Styblinski_%vD", fn.NDim) }

func (fn Styblinski) Tol() float64 { return fn.Optima()[0].Val + math.Abs(fn.Optima()[0].Val*.01) }

func (fn Styblinski) Eval(x []float64) float64 {
	if !InsideBounds(x, fn) {
		return math.Inf(1)
	}

	tot := 0.0
	for _, v := range x {
		tot += math.Pow(v, 4) - 16*math.Pow(v, 2) + 5*v
	}
	return tot / 2
}

func (fn Styblinski) Bounds() (low, up []float64) {
	low = make([]float64, fn.NDim)
	up = make([]float64, fn.NDim)
	for i := range low {
		low[i] = -5
		up[i] = 5
	}
	return low, up
}

func (fn Styblinski) Optima() []*optim.Point {
	pos := make([]float64, fn.NDim)
	for i := range pos {
		pos[i] = -2.903534
	}
	return []*optim.Point{
		&optim.Point{pos, -39.16599 * float64(fn.NDim)},
	}
}

type Rastrigrin struct {
	NDim int
}

func (fn Rastrigrin) Name() string { return fmt.Sprintf("Rastrigrin_%vD", fn.NDim) }

func (fn Rastrigrin) Tol() float64 { return 5.0 / 3.0 * float64(fn.NDim) }

func (fn Rastrigrin) Eval(x []float64) float64 {
	if !InsideBounds(x, fn) {
		return math.Inf(1)
	}

	tot := 10.0 * float64(fn.NDim)
	for i := 0; i < fn.NDim; i++ {
		tot += x[i]*x[i] - 10*math.Cos(2*math.Pi*x[i])
	}
	return tot
}

func (fn Rastrigrin) Bounds() (low, up []float64) {
	low = make([]float64, fn.NDim)
	up = make([]float64, fn.NDim)
	for i := range low {
		low[i] = -5.12
		up[i] = 5.12
	}
	return low, up
}

func (fn Rastrigrin) Optima() []*optim.Point {
	return []*optim.Point{
		&optim.Point{make([]float64, fn.NDim), 0},
	}
}

type Griewank struct {
	NDim int
}

func (fn Griewank) Name() string { return fmt.Sprintf("Griewank_%vD", fn.NDim) }

func (fn Griewank) Tol() float64 { return .1 } //return .1/ 30.0 * float64(fn.NDim) }

func (fn Griewank) Eval(x []float64) float64 {
	if !InsideBounds(x, fn) {
		return math.Inf(1)
	}

	sum := 0.0
	prod := 1.0
	for i := 0; i < fn.NDim; i++ {
		sum += x[i] * x[i]
		prod *= math.Cos(x[i] / math.Sqrt(float64(i+1)))
	}
	return 1 + sum/4000 - prod
}

func (fn Griewank) Bounds() (low, up []float64) {
	low = make([]float64, fn.NDim)
	up = make([]float64, fn.NDim)
	for i := range low {
		low[i] = -600
		up[i] = 600
	}
	return low, up
}

func (fn Griewank) Optima() []*optim.Point {
	return []*optim.Point{
		&optim.Point{make([]float64, fn.NDim), 0},
	}
}

type Rosenbrock struct {
	NDim int
}

func (fn Rosenbrock) Name() string { return fmt.Sprintf("Rosenbrock_%vD", fn.NDim) }

func (fn Rosenbrock) Tol() float64 { return 10.0 / 3 * float64(fn.NDim) }

func (fn Rosenbrock) Eval(x []float64) float64 {
	if !InsideBounds(x, fn) {
		return math.Inf(1)
	}

	tot := 0.0
	for i := 0; i < fn.NDim-1; i++ {
		diff1 := x[i+1] - x[i]*x[i]
		tot += 100 * diff1 * diff1
		diff2 := x[i] - 1
		tot += diff2 * diff2
	}
	return tot
}

func (fn Rosenbrock) Bounds() (low, up []float64) {
	low = make([]float64, fn.NDim)
	up = make([]float64, fn.NDim)
	for i := range low {
		low[i] = -30
		up[i] = 30
	}
	return low, up
}

func (fn Rosenbrock) Optima() []*optim.Point {
	pos := make([]float64, fn.NDim)
	for i := range pos {
		pos[i] = 1
	}
	return []*optim.Point{
		&optim.Point{pos, 0},
	}
}

// BenchSeed is the seed value used to initialize optim.Rand for each batch of
// optimization runs performed by the Benchmark function.
var BenchSeed int64 = 7

// Benchmark performs several optimization runs using sfn to generate
// set up problems for each run.  It uses fn as the objective and performs
// tests confirming that at least some successfrac of runs achieved better
// than fn's tolerance for optimum in less than avgiter iterations. Results
// are logged to t.
func Benchmark(t *testing.T, fn Func, sfn func() *optim.Solver, successfrac, avgiter float64) {
	optim.Rand = rand.New(rand.NewSource(BenchSeed))
	nrun := 44
	ndrop := 2
	nkeep := nrun - 2*ndrop
	neval := 0
	niter := 0
	nsuccess := 0
	sum := 0.0

	solvs := []*optim.Solver{}
	for i := 0; i < nrun; i++ {
		s := sfn()

		for s.Next() {
			if s.Best().Val < fn.Tol() {
				break
			}
		}
		if err := s.Err(); err != nil {
			t.Errorf("[%v:ERROR] %v", fn.Name(), err)
		}

		solvs = append(solvs, s)
	}

	sort.Sort(byevals(solvs))

	for _, s := range solvs[ndrop : len(solvs)-ndrop] {
		neval += s.Neval()
		niter += s.Niter()
		sum += s.Best().Val
		if s.Best().Val < fn.Tol() {
			nsuccess++
		}
	}

	frac := float64(nsuccess) / float64(nkeep)
	gotavg := float64(niter) / float64(nkeep)

	t.Logf("[%v] %v/%v runs, %v iters, %v evals, want < %.3f, averaged %.3f", fn.Name(), nsuccess, nkeep, gotavg, neval/nkeep, fn.Tol(), sum/float64(nkeep))

	if frac < successfrac {
		t.Errorf("    FAIL: only %v/%v runs succeeded, want %v/%v", nsuccess, nkeep, math.Ceil(successfrac*float64(nkeep)), nkeep)
	}

	if gotavg > avgiter {
		t.Errorf("    FAIL: too many iterations: want %v, averaged %.2f", avgiter, gotavg)
	}
}

type byevals []*optim.Solver

func (b byevals) Less(i, j int) bool { return b[i].Neval() < b[j].Neval() }
func (b byevals) Len() int           { return len(b) }
func (b byevals) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

var ErrMax = errors.New("hit max eval or iter limit")

func InsideBounds(p []float64, fn Func) bool {
	low, up := fn.Bounds()
	for i := range p {
		if p[i] < low[i] || p[i] > up[i] {
			return false
		}
	}
	return true
}
