package pattern

import (
	"crypto/sha1"
	"database/sql"
	"errors"
	"log"
	"math"
	"sort"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/optim"
)

var FoundBetterErr = errors.New("better position discovered")
var ZeroStepErr = errors.New("poll step size contracted to zero")

const (
	TblPolls = "patternpolls"
	TblInfo  = "patterninfo"
)

type Option func(*Method)

func Evaler(e optim.Evaler) Option { return func(m *Method) { m.ev = e } }

func NsuccessGrow(n int) Option {
	return func(m *Method) {
		m.NsuccessGrow = n
	}
}

const (
	Share   = true
	NoShare = false
)

func SearchMethod(m optim.Method, share bool) Option {
	return func(m2 *Method) {
		m2.Searcher = &WrapSearcher{Method: m, Share: share}
	}
}

func DiscreteSearch(m *Method) {
	m.DiscreteSearch = true
}

// Poll2N sets the method to poll in both forward and backward in every
// compass direction.
func Poll2N(m *Method) { m.Poller.Spanner = Compass2N{} }

// PollNp1 sets the method to poll in n compass directions with random
// polarity plus one direction with the opposite of all other directions in
// every dimension.
func PollNp1(m *Method) { m.Poller.Spanner = CompassNp1{} }

// PollRandN sets the method to poll in n random directions setting the
// direction for a randomly chosen number of dimensions to +/- step size.
func PollRandN(n int) Option {
	return func(m *Method) {
		if n > 0 {
			m.Poller.Spanner = &RandomN{N: n}
		}
	}
}

// PollRandNMask sets the method to poll in n random directions setting the
// direction for a randomly chosen number of dimensions to +/- step size.
// mask specifies which of the dimensions are allowed to be nonzero and
// len(mask) must be equal to the number of dimensions.
func PollRandNMask(n int, mask []bool) Option {
	return func(m *Method) {
		if n > 0 {
			m.Poller.Spanner = &RandomN{N: n, Mask: mask}
		}
	}
}

func DB(db *sql.DB) Option {
	return func(m *Method) {
		m.Db = db
	}
}

func SkipEps(eps float64) Option { return func(m *Method) { m.Poller.SkipEps = eps } }

func Nkeep(n int) Option { return func(m *Method) { m.Poller.Nkeep = n } }

func ResetStep(threshold float64) Option {
	return func(m *Method) { m.ResetStep = threshold }
}

type Method struct {
	Poller         *Poller
	Searcher       Searcher
	Curr           *optim.Point
	DiscreteSearch bool // true to project search points onto poll step size mesh
	NsuccessGrow   int  // number of successive successful polls before growing mesh
	nsuccess       int  // (internal) number of successive successful polls
	Db             *sql.DB
	// ResetStep is a step size threshold below which the mesh step is reset
	// to its original starting value.  This can be useful for problems where
	// the significance of a particular step size of one variable may be a
	// function of the value other variables.
	ResetStep float64
	origstep  float64
	count     int
	ev        optim.Evaler
}

func New(start *optim.Point, opts ...Option) *Method {
	m := &Method{
		Curr:         start,
		ev:           optim.SerialEvaler{},
		Poller:       &Poller{Nkeep: start.Len() / 4, SkipEps: 1e-10},
		Searcher:     NullSearcher{},
		NsuccessGrow: -1,
	}

	for _, opt := range opts {
		opt(m)
	}
	m.initdb()
	return m
}

func (m *Method) AddPoint(p *optim.Point) {
	if p.Val < m.Curr.Val {
		m.Curr = p
	}
}

// Iterate mutates m and so for each iteration, the same, mutated m should be
// passed in.
func (m *Method) Iterate(o optim.Objectiver, mesh optim.Mesh) (best *optim.Point, n int, err error) {
	if m.count == 0 {
		m.origstep = mesh.Step()
	} else if mesh.Step() < m.ResetStep {
		mesh.SetStep(m.origstep)
	}

	var nevalsearch, nevalpoll int
	var success bool
	defer m.updateDb(&nevalsearch, &nevalpoll, mesh.Step())
	m.count++

	prevstep := mesh.Step()
	if !m.DiscreteSearch {
		mesh.SetStep(0)
	}

	success, best, nevalsearch, err = m.Searcher.Search(o, mesh, m.Curr)
	mesh.SetStep(prevstep)

	n += nevalsearch
	if err != nil {
		return best, n, err
	} else if success {
		m.Curr = best
		return best, n, nil
	}

	// It is important to recenter mesh on new best point before polling.
	// This is necessary because the search may not be operating on the
	// current mesh grid.  This doesn't need to happen if search succeeds
	// because search either always operates on the same grid, or always
	// operates in continuous space.
	mesh.SetOrigin(m.Curr.Pos) // TODO: test that this doesn't get set to Zero pos [0 0 0...] on first iteration.

	success, best, nevalpoll, err = m.Poller.Poll(o, m.ev, mesh, m.Curr)
	m.Poller.Spanner.Update(mesh.Step(), success)

	n += nevalpoll
	if err != nil {
		return m.Curr, n, err
	} else if success {
		m.Curr = best
		m.nsuccess++
		if m.nsuccess == m.NsuccessGrow { // == allows -1 to mean never grow
			mesh.SetStep(mesh.Step() * 2.0)
			m.nsuccess = 0 // reset after resize
		}

		// Important to recenter mesh on new best point.  More particularly,
		// the mesh may have been resized and the new best may not lie on the
		// previous mesh grid.
		mesh.SetOrigin(best.Pos)

		return best, n, nil
	} else {
		m.nsuccess = 0
		var err error
		mesh.SetStep(mesh.Step() * 0.5)
		if mesh.Step() == 0 {
			err = ZeroStepErr
		}
		return m.Curr, n, err
	}
}

func (m *Method) initdb() {
	if m.Db == nil {
		return
	}

	s := "CREATE TABLE IF NOT EXISTS " + TblPolls + " (iter INTEGER,val REAL,posid BLOB);"
	_, err := m.Db.Exec(s)
	if checkdberr(err) {
		return
	}

	s = "CREATE TABLE IF NOT EXISTS " + TblInfo + " (iter INTEGER,step INTEGER,nsearch INTEGER,npoll INTEGER,val REAL,posid BLOB);"
	_, err = m.Db.Exec(s)
	if checkdberr(err) {
		return
	}
}

func (m Method) updateDb(nsearch, npoll *int, step float64) {
	if m.Db == nil {
		return
	}

	tx, err := m.Db.Begin()
	if err != nil {
		panic(err.Error())
	}
	defer tx.Commit()

	s1 := "INSERT INTO " + TblPolls + " (iter,val,posid) VALUES (?,?,?);"
	for _, p := range m.Poller.Points() {
		_, err := tx.Exec(s1, m.count, p.Val, p.HashSlice())
		if checkdberr(err) {
			return
		}
	}

	glob := m.Curr
	s2 := "INSERT INTO " + TblInfo + " (iter,step,nsearch, npoll,val,posid) VALUES (?,?,?,?,?,?);"
	_, err = tx.Exec(s2, m.count, step, *nsearch, *npoll, glob.Val, glob.HashSlice())
	if checkdberr(err) {
		return
	}

	pts := m.Poller.Points()
	pts = append(pts, glob)
	err = optim.RecordPointPos(tx, pts...)
	if checkdberr(err) {
		return
	}
}

type Poller struct {
	// Nkeep specifies the number of previous successful poll directions to
	// reuse on the next poll. The number of reused directions is min(Nkeep,
	// nsuccessful).
	Nkeep int
	// SkipEps is the distance from the center point within which a poll point
	// is excluded from evaluation.  This can occur if a mesh projection
	// results in a point being projected back near the poll origin point.
	SkipEps    float64
	Spanner    Spanner
	keepdirecs []direc
	points     []*optim.Point
	prevhash   [sha1.Size]byte
	prevstep   float64
}

func (cp *Poller) Points() []*optim.Point { return cp.points }

type direc struct {
	dir []int
	val float64
}

type byval []direc

func (b byval) At(i int) []int     { return b[i].dir }
func (b byval) Less(i, j int) bool { return b[i].val < b[j].val }
func (b byval) Len() int           { return len(b) }
func (b byval) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

// Poll polls on mesh m centered on point from.  It is responsible for
// selecting points and evaluating them with ev using obj.  If a better
// point was found, it returns success == true, the point, and number of
// evaluations.  If a better point was not found, it returns false, the
// from point, and the number of evaluations.  If err is non-nil, success
// must be false and best must be from - neval may be non-zero.
func (cp *Poller) Poll(obj optim.Objectiver, ev optim.Evaler, m optim.Mesh, from *optim.Point) (success bool, best *optim.Point, neval int, err error) {
	best = from
	if cp.Spanner == nil {
		cp.Spanner = Compass2N{}
	}

	pollpoints := []*optim.Point{}

	// Only poll compass directions if we haven't polled from this point
	// before.  DONT DELETE - this can fire sometimes if the mesh isn't
	// allowed to contract below a certain step (i.e. integer meshes).
	h := from.Hash()
	if h != cp.prevhash || cp.prevstep != m.Step() {
		// TODO: write test that checks we poll compass dirs again if only mesh
		// step changed (and not from point)
		pollpoints = genPollPoints(from, cp.Spanner, m)
		cp.prevhash = h
	} else {
		// Use random directions instead.
		n := len(genPollPoints(from, cp.Spanner, m))
		pollpoints = genPollPoints(from, &RandomN{N: n}, m)
	}
	cp.prevstep = m.Step()

	// Add successful directions from last poll.  We want to add these points
	// in front of the other points so we can potentially stop earlier if
	// polling opportunistically.
	perms := optim.Rand.Perm(len(pollpoints))

	// this is an extra safety check to make sure we don't index out of bounds
	// on the perms slice
	max := len(cp.keepdirecs)
	if max > len(perms) {
		max = len(perms)
	}

	for i, dir := range cp.keepdirecs[:max] {
		swapindex := perms[i]
		pollpoints[swapindex] = pointFromDirec(from, dir.dir, m)
	}

	// project points onto feasible region and mesh grid
	if m != nil {
		for _, p := range pollpoints {
			p.Pos = m.Nearest(p.Pos)
		}
	}

	cp.points = make([]*optim.Point, 0, len(pollpoints))
	if cp.SkipEps == 0 {
		cp.points = pollpoints
	} else {
		for _, p := range pollpoints {
			// It is possible that due to the mesh gridding, the poll point is
			// outside of constraints or bounds and will be rounded back to the
			// current point. Check for this and skip the poll point if this is
			// the case.
			dist := optim.L2Dist(from, p)
			if dist > cp.SkipEps {
				cp.points = append(cp.points, p)
			}
		}
	}

	objstop := &objStopper{Objectiver: obj, Best: from.Val}
	results, n, err := ev.Eval(objstop, cp.points...)
	if err != nil && err != FoundBetterErr {
		return false, best, n, err
	}

	// this is separate from best to allow all points better than from to be
	// added to keepdirecs before we update the best point.
	nextbest := from

	// Sort results and keep the best Nkeep as poll directions.
	for _, p := range results {
		if p.Val < best.Val {
			cp.keepdirecs = append(cp.keepdirecs, direc{direcbetween(from, p, m), p.Val})
		}
		if p.Val < nextbest.Val {
			nextbest = p
		}
	}
	best = nextbest

	nkeep := cp.Nkeep
	if max := len(pollpoints) / 4; max < nkeep {
		nkeep = max
	}

	sort.Sort(byval(cp.keepdirecs))
	if len(cp.keepdirecs) > nkeep {
		cp.keepdirecs = cp.keepdirecs[:nkeep]
	}

	if best.Val < from.Val {
		return true, best, n, nil
	} else {
		return false, from, n, nil
	}
}

type Searcher interface {
	Search(o optim.Objectiver, m optim.Mesh, curr *optim.Point) (success bool, best *optim.Point, n int, err error)
}

type NullSearcher struct{}

func (_ NullSearcher) Search(o optim.Objectiver, m optim.Mesh, curr *optim.Point) (success bool, best *optim.Point, n int, err error) {
	return false, curr, 0, nil // TODO: test that this returns curr instead of something else
}

type WrapSearcher struct {
	Method optim.Method
	// Share specifies whether to add the current best point to the
	// searcher's underlying method before performing the search.
	Share bool
}

func (s *WrapSearcher) Search(o optim.Objectiver, m optim.Mesh, curr *optim.Point) (success bool, best *optim.Point, n int, err error) {
	if s.Share {
		s.Method.AddPoint(curr)
	}
	best, n, err = s.Method.Iterate(o, m)
	if err != nil {
		return false, &optim.Point{Val: math.Inf(1)}, n, err
	}
	if best.Val < curr.Val {
		return true, best, n, nil
	}
	// TODO: write test that checks we return curr instead of best for search
	// fail.
	return false, curr, n, nil
}

// objStopper is wraps an Objectiver and returns the objective value along
// with FoundBetterErr as soon as calculates a value better than Best.  This
// is useful for things like terminating early with opportunistic polling.
type objStopper struct {
	Best float64
	optim.Objectiver
}

func (s *objStopper) Objective(v []float64) (float64, error) {
	obj, err := s.Objectiver.Objective(v)
	if err != nil {
		return obj, err
	} else if obj < s.Best {
		return obj, FoundBetterErr
	}
	return obj, nil
}

func genPollPoints(from *optim.Point, span Spanner, m optim.Mesh) []*optim.Point {
	ndim := from.Len()
	dirs := span.Span(ndim)
	polls := make([]*optim.Point, 0, len(dirs))
	for _, d := range dirs {
		polls = append(polls, pointFromDirec(from, d, m))
	}
	return polls
}

func pointFromDirec(from *optim.Point, direc []int, m optim.Mesh) *optim.Point {
	pos := make([]float64, from.Len())
	step := m.Step()
	for i, x0 := range from.Pos {
		pos[i] = x0 + float64(direc[i])*step

	}
	return &optim.Point{m.Nearest(pos), math.Inf(1)}
}

// Spanner is returns a set of poll directions (maybe positive spanning set?)
type Spanner interface {
	Update(step float64, prevsuccess bool)
	// Span returns a set of ndim dimensional polling directions (either a 1,
	// 0, -1).
	Span(ndim int) [][]int
}

// Compass2N returns a compass positive basis set of polling directions in a
// randomized order.
type Compass2N struct{}

func (c Compass2N) Update(step float64, prevsuccess bool) {}

func (c Compass2N) Span(ndim int) [][]int {
	dirs := make([][]int, 2*ndim)
	perms := optim.Rand.Perm(ndim)
	for i := 0; i < ndim; i++ {
		d := make([]int, ndim)
		d[i] = 1
		dirs[perms[i]] = d

		d = make([]int, ndim)
		d[i] = -1
		dirs[ndim+perms[i]] = d
	}
	return dirs
}

type CompassNp1 struct{}

func (c CompassNp1) Update(step float64, prevsuccess bool) {}

func (c CompassNp1) Span(ndim int) [][]int {
	dirs := make([][]int, 0, ndim+1)
	final := make([]int, ndim)
	for i := 0; i < ndim; i++ {
		d := make([]int, ndim)

		r := optim.Rand.Intn(2)
		d[i] = 1
		final[i] = -1
		if r == 0 {
			d[i] = -1
			final[i] = 1
		}

		dirs = append(dirs, d)
	}
	dirs = append(dirs, final)
	end := len(dirs) - 1
	// poll the diagonal direction first
	dirs[0], dirs[end] = dirs[end], dirs[0]
	return dirs
}

// RandomN returns n random polling directions by randomly choosing a number
// of dimensions to receive a non-zero step and randomly assigning each
// non-zero step either a forward or backward polarity.  mask specifies which
// dimensions are allowed to have a nonzero step.
type RandomN struct {
	// N is the number of random directions to generate.
	N int
	// Mask has either true or false for each dimension indicating whether or
	// not it is allowed to be nonzero in the generated drections.
	Mask        []bool
	nonzeroFrac float64
	origstep    float64
}

func (r *RandomN) Update(step float64, prevsuccess bool) {
	if r.origstep == 0 {
		r.origstep = step
	}
	r.nonzeroFrac = math.Min(1, math.Sqrt(step/r.origstep))
}

func (r *RandomN) Span(ndim int) [][]int {
	if r.nonzeroFrac == 0 {
		r.nonzeroFrac = 1
	}
	if r.Mask == nil {
		r.Mask = make([]bool, ndim)
		for i := range r.Mask {
			r.Mask[i] = true
		}
	}

	// index map tells us at which index into a full dimensional direction
	// vector to place a non-zero value into.
	indexmap := []int{}
	nactive := 0
	for i, active := range r.Mask {
		if active {
			nactive++
			indexmap = append(indexmap, i)
		}
	}
	if nactive == 0 {
		panic("pattern: mask cannot be zero length")
	} else if ndim != len(r.Mask) {
		panic("pattern: ndim != len(mask)")
	}

	dirs := make([][]int, 0, r.N)
	for len(dirs) < r.N {
		d1 := make([]int, ndim)
		d2 := make([]int, ndim)

		nNonzero := 1
		maxnonzero := int(float64(nactive) * r.nonzeroFrac)
		if maxnonzero > 1 {
			// the +1 is to exclude vector of all zeros. And since Intn
			// returns numbers < nactive we don't have to worry about
			// nNonzero being greater than nactive.
			nNonzero = optim.Rand.Intn(maxnonzero) + 1
		}
		perms := optim.Rand.Perm(nactive)
		for i := 0; i < nNonzero; i++ {
			r := optim.Rand.Intn(2)
			if r == 0 {
				d1[indexmap[perms[i]]] = 1
				d2[indexmap[perms[i]]] = -1
			} else {
				d1[indexmap[perms[i]]] = -1
				d2[indexmap[perms[i]]] = 1
			}
		}
		dirs = append(dirs, d1)
		dirs = append(dirs, d2)
	}
	return dirs
}

func direcbetween(from, to *optim.Point, m optim.Mesh) []int {
	d := make([]int, from.Len())
	step := m.Step()
	for i, x0 := range from.Pos {
		d[i] = int((to.Pos[i] - x0) / step)
	}
	return d
}

func checkdberr(err error) bool {
	if err != nil {
		log.Print("pattern: db write failed -", err)
		return true
	}
	return false
}
