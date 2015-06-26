package optim

import (
	"crypto/sha1"
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/rand"
	"sync"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/gonum/matrix/mat64"
)

var Rand Rng = rand.New(rand.NewSource(1))

type Rng interface {
	Float64() float64
	Intn(n int) int
	Perm(n int) []int
}

func RandFloat() float64 { return Rand.Float64() }

type Solver struct {
	Method       Method
	Obj          Objectiver
	Mesh         Mesh
	MaxIter      int
	MaxEval      int
	MaxNoImprove int
	MinStep      float64

	neval, niter int
	noimprove    int
	best         *Point
	err          error
}

func (s *Solver) Best() *Point { return s.best }
func (s *Solver) Niter() int   { return s.niter }
func (s *Solver) Neval() int   { return s.neval }
func (s *Solver) Err() error   { return s.err }

func (s *Solver) Run() error {
	for s.Next() {
	}
	return s.Err()
}

func (s *Solver) Next() (more bool) {
	if s.Mesh == nil {
		s.Mesh = &InfMesh{}
	}
	if s.niter == 0 {
		s.best = &Point{Val: math.Inf(1)}
	}

	var n int
	var best *Point
	best, n, s.err = s.Method.Iterate(s.Obj, s.Mesh)
	s.neval += n
	s.niter++

	if best.Val < s.best.Val {
		s.best = best
		s.noimprove = 0
	} else {
		s.noimprove++
	}

	if s.err != nil {
		return false
	}

	more = true && (s.MaxNoImprove == 0 || s.noimprove < s.MaxNoImprove)
	more = more && (s.MaxIter == 0 || s.niter < s.MaxIter)
	more = more && (s.MaxEval == 0 || s.neval < s.MaxEval)
	more = more && (s.MinStep == 0 || s.Mesh.Step() > s.MinStep)
	return more
}

type Point struct {
	Pos []float64
	Val float64
}

func (p *Point) Len() int             { return len(p.Pos) }
func (p *Point) Matrix() *mat64.Dense { return mat64.NewDense(p.Len(), 1, p.Pos) }
func (p *Point) String() string       { return fmt.Sprintf("f%v = %v", p.Pos, p.Val) }

func (p *Point) Clone() *Point {
	pos := make([]float64, len(p.Pos))
	copy(pos, p.Pos)
	return &Point{Pos: pos, Val: p.Val}
}

func (p *Point) Hash() [sha1.Size]byte {
	data := make([]byte, p.Len()*8)
	for i := 0; i < p.Len(); i++ {
		binary.BigEndian.PutUint64(data[i*8:], math.Float64bits(p.Pos[i]))
	}
	return sha1.Sum(data)
}

func (p *Point) HashSlice() []byte {
	h := p.Hash()
	return h[:]
}

type Method interface {
	// Iterate runs a single iteration of a solver and reports the number of
	// function evaluations n and the best point.
	Iterate(obj Objectiver, m Mesh) (best *Point, n int, err error)
	// AddPoint enables limited hybriding of different optimization iterators
	// by allowing iterators/solvers to add share information by suggesting
	// points to each other.
	AddPoint(p *Point)
}

type Evaler interface {
	// Eval evaluates each point using obj and sets its value.  It also
	// returns the resulting points with corresponding objective values. It
	// also returns the number of times obj was called n and any error that
	// occurred.  Unevaluated points should not be returned in the results
	// slice.  The order of points in results may be different than the order
	// of the passed in points.  len(results) may be less than len(points).
	// Note that it is not strictly necessary to use the results as the passed
	// in pointers will be modified also.
	Eval(obj Objectiver, points ...*Point) (results []*Point, n int, err error)
}

type Objectiver interface {
	// Objective evaluates the variables in v and returns the objective
	// function value.  The objective function must be framed so that lower
	// values are better. If the evaluation fails, positive infinity should be
	// returned along with an error.  Note that it is possible for an error to
	// be returned if the evaulation succeeds.
	Objective(v []float64) (float64, error)
}

type CacheEvaler struct {
	ev    Evaler
	cache map[[sha1.Size]byte]float64
	// UseCount reports the number of times a cached objective evaluation was
	// successfully used to avoid recalculation.
	UseCount int
}

func NewCacheEvaler(ev Evaler) *CacheEvaler {
	return &CacheEvaler{
		ev:    ev,
		cache: map[[sha1.Size]byte]float64{},
	}
}

func (ev *CacheEvaler) Eval(obj Objectiver, points ...*Point) (results []*Point, n int, err error) {
	results = make([]*Point, 0, len(points))
	newp := make([]*Point, 0, len(points))
	uniq := uniqof(points)
	for _, p := range uniq {
		h := p.Hash()
		if val, ok := ev.cache[h]; ok {
			p.Val = val
			results = append(results, p)
			ev.UseCount++
		} else {
			p.Val = math.Inf(1)
			newp = append(newp, p)
		}
	}

	newresults, n, err := ev.ev.Eval(obj, newp...)
	for _, p := range newresults {
		if p.Val != math.Inf(1) {
			ev.cache[p.Hash()] = p.Val
		}
	}
	return append(newresults, results...), n, err
}

type SerialEvaler struct {
	ContinueOnErr bool
}

func (ev SerialEvaler) Eval(obj Objectiver, points ...*Point) (results []*Point, n int, err error) {
	uniq := uniqof(points)
	for i, p := range uniq {

		p.Val, err = obj.Objective(p.Pos)
		n++
		if err != nil && !ev.ContinueOnErr {
			return uniq[:i+1], n, err
		}
	}
	return uniq, n, nil
}

type errpoint struct {
	*Point
	Err error
}

// uniqof returns only unique points in ps.
func uniqof(ps []*Point) []*Point {
	alreadyhave := map[[sha1.Size]byte]struct{}{}
	uniq := []*Point{}
	for _, p := range ps {
		h := p.Hash()
		if _, ok := alreadyhave[h]; !ok {
			uniq = append(uniq, p)
			alreadyhave[h] = struct{}{}
		}
	}
	return uniq
}

type ParallelEvaler struct {
	ContinueOnErr bool
	NConcurrent   int
}

func (ev ParallelEvaler) Eval(obj Objectiver, points ...*Point) (results []*Point, n int, err error) {
	nbuf := ev.NConcurrent
	if nbuf == 0 {
		nbuf = 100000
	}
	limiter := make(chan bool, nbuf)
	for i := 0; i < nbuf; i++ {
		limiter <- true
	}

	ch := make(chan errpoint, len(points))
	wg := sync.WaitGroup{}
	uniq := uniqof(points)
	for i, p := range uniq {
		wg.Add(1)
		go func(i int, p *Point) {
			defer wg.Done()
			<-limiter
			defer func() { limiter <- true }()
			perr := errpoint{Point: p}
			perr.Val, perr.Err = obj.Objective(p.Pos)
			ch <- perr
		}(i, p)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	results = make([]*Point, 0, len(points))
	for p := range ch {
		n++
		results = append(results, p.Point)
		if p.Err != nil {
			err = p.Err
		}
	}

	if ev.ContinueOnErr && len(results) > 0 {
		return results, n, nil
	} else {
		return results, n, err
	}
}

type Func func([]float64) float64

func (so Func) Objective(v []float64) (float64, error) { return so(v), nil }

type ObjectiveLogger struct {
	Obj Objectiver
	W   io.Writer
}

func (l *ObjectiveLogger) Objective(v []float64) (float64, error) {
	val, err := l.Obj.Objective(v)

	fmt.Fprintf(l.W, "f%v = %v\n", v, val)
	return val, err
}

// ObjectivePenalty wraps an objective function and adds a penalty factor for
// any violated linear constraints. If Weight is zero the underlying
// objective value will be returned unaltered.
type ObjectivePenalty struct {
	Obj     Objectiver
	A       *mat64.Dense
	Low, Up *mat64.Dense
	Weight  float64
	a       *mat64.Dense // stacked version of A
	b       *mat64.Dense // Low and Up stacked
	ranges  []float64    // ranges[i] = u[i] - l[i]
}

func (o *ObjectivePenalty) init() {
	if o.a != nil {
		// already initialized
		return
	}
	o.a, o.b, o.ranges = StackConstr(o.Low, o.A, o.Up)
}

func (o *ObjectivePenalty) Objective(v []float64) (float64, error) {
	o.init()
	val, err := o.Obj.Objective(v)

	if o.Weight == 0 {
		return val, err
	}

	ax := &mat64.Dense{}
	x := mat64.NewDense(len(v), 1, v)
	ax.Mul(o.a, x)

	m, _ := ax.Dims()

	penalty := 0.0
	for i := 0; i < m; i++ {
		if diff := ax.At(i, 0) - o.b.At(i, 0); diff > 0 {
			// maybe use "*=" for compounding penalty buildup
			penalty += diff / o.ranges[i] * o.Weight
		}
	}

	return val * (1 + penalty), err
}

func L2Dist(p1, p2 *Point) float64 {
	tot := 0.0
	for i := 0; i < p1.Len(); i++ {
		diff := p1.Pos[i] - p2.Pos[i]
		tot += diff * diff
	}
	return math.Sqrt(tot)
}

// StackConstrBoxed converts the equations:
//
//     lb <= Ix <= ub
//     and
//     low <= Ax <= up
//
// into a single equation of the form:
//
//     Ax <= b
func StackConstrBoxed(lb, ub []float64, low, A, up *mat64.Dense) (stackA, b *mat64.Dense, ranges []float64) {
	lbm := mat64.NewDense(len(lb), 1, lb)
	ubm := mat64.NewDense(len(ub), 1, ub)

	stacklow := &mat64.Dense{}
	stacklow.Stack(low, lbm)

	stackup := &mat64.Dense{}
	stackup.Stack(up, ubm)

	boxA := mat64.NewDense(len(lb), len(lb), nil)
	for i := 0; i < len(lb); i++ {
		boxA.Set(i, i, 1)
	}

	stacked := &mat64.Dense{}
	stacked.Stack(A, boxA)
	return StackConstr(stacklow, stacked, stackup)
}

func RecordPointPos(tx *sql.Tx, pts ...*Point) error {
	s := "CREATE TABLE IF NOT EXISTS points (posid BLOB,dim INTEGER,val REAL);"
	_, err := tx.Exec(s)
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO points VALUES (?,?,?);")
	if err != nil {
		return err
	}

	for _, p := range pts {
		id := p.HashSlice()
		for dim, pos := range p.Pos {
			_, err = stmt.Exec(id, dim, pos)
			if err != nil {
				return fmt.Errorf("db write failed: %v", err)
			}
		}
	}
	return nil
}

func StackConstr(low, A, up *mat64.Dense) (stackA, b *mat64.Dense, ranges []float64) {
	neglow := &mat64.Dense{}
	neglow.Scale(-1, low)
	b = &mat64.Dense{}
	b.Stack(up, neglow)

	negA := &mat64.Dense{}
	negA.Scale(-1, A)
	stackA = &mat64.Dense{}
	stackA.Stack(A, negA)

	// capture the range of each constraint from A because this information is
	// lost when converting from "low <= Ax <= up" via stacking to "Ax <= up".
	m, _ := A.Dims()
	ranges = make([]float64, m, 2*m)
	for i := 0; i < m; i++ {
		ranges[i] = up.At(i, 0) - low.At(i, 0)
		if ranges[i] == 0 {
			if up.At(i, 0) == 0 {
				ranges[i] = 1
			} else {
				ranges[i] = up.At(i, 0)
			}
		}
	}
	ranges = append(ranges, ranges...)

	return stackA, b, ranges
}
