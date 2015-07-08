// Package swarm provides a particle swarm iterator based on work by Eberhart
// et al.  This solver has been verified to perform as well as some of their
// benchmark results in:
//
//     Eberhart, Russ C., and Yuhui Shi. "Comparing inertia weights and
//     constriction factors in particle swarm optimization." Evolutionary
//     Computation, 2000. Proceedings of the 2000 Congress on. Vol. 1. IEEE, 2000.
//
// The problem this solver is benchmarked most carefully against is:
//
//    * Rosenbrock 30 dimensions
//    * -30 <= xi <= 30
//    * 30 particles
//    * solved if f(x) <= 100
//    * average solution in 669 iterations
package swarm

import (
	"database/sql"
	"log"
	"math"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/optim"
)

// These parameters are calculated using a constriction factor originally
// described in:
//
//     Clerc and M.  “The swarm and the queen: towards a deterministic and
//     adaptive particle swarm optimization” Proc. 1999 Congress on
//     Evolutionary Computation, pp. 1951-1957
//
// The cognition and social parameters correspond to c1 and c2 values of 2.05
// that have been multiplied by their constriction coeffient - i.e.
// DefaultSocial = Constriction(2.05, 2.05)*2.05.  DefaultInertia is set equal
// to the constriction coefficient.
const (
	DefaultCognition = 1.496179765663133
	DefaultSocial    = 1.496179765663133
	DefaultInertia   = 0.7298437881283576
)

const (
	// TblParticles is the name of the sql database table that contains
	// positions and values for particles for each iteration.
	TblParticles = "swarmparticles"
	// TblParticlesMeshed is the name of the sql database table that contains
	// mesh-projected positions (where objective evaluations actually
	// occurred)  and values for particles for each iteration.
	TblParticlesMeshed = "swarmparticlesmesh"
	// TblParticlesBest is the name of the sql database table that contains
	// each particle's personal best position at each iteration.
	TblParticlesBest = "swarmparticlesbest"
	// TblBest is the name of the sql database table that contains
	// the best position for the entire swarm at each iteration.
	TblBest = "swarmbest"
)

// Constriction calculates the constriction coefficient for the given c1 and
// c2 for the particle velocity equation:
//
//    v_next = k(v_curr + c1*rand*(p_glob-x) + c2*rand*(p_personal-x))
//
//    or
//
//    v_next = w*v_curr + b1*rand*(p_glob-x) + b2*rand*(p_personal-x)
//
//    (with constriction coefficient multiplied through.
//
// c1+c2 should usually be greater than (but close to) 4.  'w = k' is often
// referred to as the inertia in the traditional swarm equation
func Constriction(c1, c2 float64) float64 {
	phi := c1 + c2
	return 2 / math.Abs(2-phi-math.Sqrt(phi*phi-4*phi))
}

type Particle struct {
	Id int
	*optim.Point
	Vel  []float64
	Best *optim.Point
}

func (p *Particle) L2Vel() float64 {
	tot := 0.0
	for _, v := range p.Vel {
		tot += v * v
	}
	return math.Sqrt(tot)
}

func (p *Particle) Move(gbest *optim.Point, vmax []float64, inertia, social, cognition float64) {
	// update velocity
	for i, currv := range p.Vel {
		// random numbers r1 and r2 MUST go inside this loop and be generated
		// uniquely for each dimension of p's velocity.
		r1 := optim.RandFloat()
		r2 := optim.RandFloat()
		p.Vel[i] = inertia*currv +
			cognition*r1*(p.Best.Pos[i]-p.Pos[i]) +
			social*r2*(gbest.Pos[i]-p.Pos[i])
		if math.Abs(p.Vel[i]) > vmax[i] {
			p.Vel[i] = math.Copysign(vmax[i], p.Vel[i])
		}
	}

	// update position
	for i := range p.Pos {
		p.Pos[i] += p.Vel[i]
	}
	p.Val = math.Inf(1)
}

func (p *Particle) Kill(gbest *optim.Point, xtol, vtol float64) bool {
	if xtol == 0 || vtol == 0 {
		return false
	}

	totv := 0.0
	diffx := 0.0
	for i, v := range p.Vel {
		totv += v * v
		diff := p.Pos[i] - gbest.Pos[i]
		diffx += diff * diff
	}
	return (totv < vtol*vtol) && (diffx < xtol*xtol)
}

func (p *Particle) Update(newp *optim.Point) {
	// DO NOT update p's position with newp's position - it may have been
	// projected onto a mesh and be different.
	p.Val = newp.Val
	if p.Val < p.Best.Val {
		p.Best = newp.Clone()
	}
}

type Population []*Particle

// NewPopulation initializes a population of particles using the given points
// and generates velocities for each dimension i initialized to uniform random
// values between minv[i] and maxv[i].  github.com/rwcarlsen/optim.Rand is
// used for random numbers.
func NewPopulation(points []*optim.Point, vmax []float64) Population {
	pop := make(Population, len(points))
	for i, p := range points {
		pop[i] = &Particle{
			Id:    i,
			Point: p,
			Best:  p.Clone(),
			Vel:   make([]float64, len(vmax)),
		}
		for j, v := range vmax {
			pop[i].Vel[j] = v * (1 - 2*optim.RandFloat())
		}
	}
	return pop
}

// NewPopulationRand creates a population of randomly positioned particles
// uniformly distributed in the box-bounds described by low and up.
func NewPopulationRand(n int, low, up []float64) Population {
	points := optim.RandPop(n, low, up)
	return NewPopulation(points, vmaxfrombounds(low, up))
}

func (pop Population) Best() *Particle {
	if len(pop) == 0 {
		return nil
	}

	best := pop[0]
	for _, p := range pop[1:] {
		// TODO: write test to make sure this checks p.Best.Val < best.Best.Val
		// and NOT p.Val or best.Val.
		if p.Best.Val < best.Best.Val {
			best = p
		}
	}
	return best
}

type Option func(*Method)

func Vmax(vmaxes []float64) Option {
	return func(m *Method) {
		m.Vmax = vmaxes
	}
}

func VmaxAll(vmax float64) Option {
	return func(m *Method) {
		for i := range m.Vmax {
			m.Vmax[i] = vmax
		}
	}
}

// VmaxBounds sets the maximum particle speed for each dimension equal to
// the bounded range for the problem - i.e. up[i]-low[i]/2 for each dimension.
// This is a good rule of thumb given in:
//
//     Eberhart, R.C.; Yuhui Shi, "Particle swarm optimization: developments,
//     applications and resources," Evolutionary Computation, 2001. Proceedings of
//     the 2001 Congress on , vol.1, no., pp.81,86 vol. 1, 2001 doi:
//     10.1109/CEC.2001.934374
func VmaxBounds(low, up []float64) Option {
	return func(m *Method) {
		m.Vmax = vmaxfrombounds(low, up)
	}
}

func DB(db *sql.DB) Option {
	return func(m *Method) {
		m.Db = db
	}
}

func KillTol(xtol, vtol float64) Option {
	return func(m *Method) {
		m.Xtol = xtol
		m.Vtol = vtol
	}
}

func LearnFactors(cognition, social float64) Option {
	return func(m *Method) {
		m.Cognition = cognition
		m.Social = social
	}
}

func Evaler(e optim.Evaler) Option { return func(m *Method) { m.Evaler = e } }

// LinInertia sets particle inertia for velocity updates to varry linearly
// from the start (high) to end (low) values from 0 to maxiter.  Common values
// are start = 0.9 and end = 0.4 - for details see:
//
//     Eberhart, R.C.; Yuhui Shi, "Particle swarm optimization: developments,
//     applications and resources," Evolutionary Computation, 2001. Proceedings of
//     the 2001 Congress on , vol.1, no., pp.81,86 vol. 1, 2001 doi:
//     10.1109/CEC.2001.934374
func LinInertia(start, end float64, maxiter int) Option {
	return func(m *Method) {
		m.InertiaFn = func(iter int) float64 {
			return start - (start-end)*float64(iter)/float64(maxiter)
		}
	}
}

func FixedInertia(v float64) Option {
	return func(m *Method) {
		m.InertiaFn = func(iter int) float64 { return v }
	}
}

func InitIter(iter int) Option {
	return func(m *Method) { m.iter = iter }
}

type Method struct {
	// Xtol is the distance from the global best under which particles are
	// considered to removal.  This must occur simultaneously with the Vtol
	// condition.
	Xtol float64
	// Vtol is the velocity under which particles are considered to removal.
	// This must occur simultaneously with the Xtol condition.
	Vtol float64
	Pop  Population
	optim.Evaler
	Cognition float64
	Social    float64
	InertiaFn func(iter int) float64
	// Vmax is the speed limit per dimension for particles.  If nil,
	// infinity is used.
	Vmax []float64
	Db   *sql.DB
	iter int
	best *optim.Point
}

func New(pop Population, opts ...Option) *Method {
	vmax := make([]float64, pop[0].Len())
	for i := range vmax {
		vmax[i] = math.Inf(1)
	}

	m := &Method{
		Pop:       pop,
		Evaler:    optim.SerialEvaler{},
		Cognition: DefaultCognition,
		Social:    DefaultSocial,
		InertiaFn: func(iter int) float64 { return DefaultInertia },
		Vmax:      vmax,
		best:      pop.Best().Point.Clone(), // TODO: write test that checks best is a Clone
	}

	for _, opt := range opts {
		opt(m)
	}

	m.initdb()
	return m
}

func (m *Method) Iterate(obj optim.Objectiver, mesh optim.Mesh) (best *optim.Point, neval int, err error) {
	defer func() { m.iter++ }()

	// project positions onto mesh
	pmap := make(map[*optim.Point]*Particle, len(m.Pop))
	points := make([]*optim.Point, len(m.Pop))
	for i, particle := range m.Pop {
		p := particle.Point.Clone()
		p.Val = math.Inf(1)
		points[i] = p
		pmap[p] = particle
	}
	if mesh != nil {
		for _, p := range points {
			p.Pos = mesh.Nearest(p.Pos)
		}
	}

	// evaluate current positions
	results, n, err := m.Evaler.Eval(obj, points...)
	if err != nil {
		return &optim.Point{Val: math.Inf(1)}, n, err
	}
	for _, p := range results {
		pmap[p].Update(p)
	}
	m.updateDb(mesh)

	// move particles and update current best
	for _, p := range m.Pop {
		p.Move(m.best, m.Vmax, m.InertiaFn(m.iter), m.Social, m.Cognition)
	}

	// TODO: write test to make sure this checks pbest.Best.Val instead of p.Val.
	pbest := m.Pop.Best()
	if pbest != nil && pbest.Best.Val < m.best.Val {
		m.best = pbest.Best
	}

	// Kill slow particles near global optimum.
	// This MUST go after the updating of the iterator's best position.
	for i, p := range m.Pop {
		if p.Kill(m.best, m.Xtol, m.Vtol) {
			m.Pop = append(m.Pop[:i], m.Pop[i+1:]...)
		}
	}

	return m.best, n, nil
}

func (m *Method) AddPoint(p *optim.Point) {
	if p.Val < m.best.Val {
		m.best = p
	}
}

func (m *Method) initdb() {
	if m.Db == nil {
		return
	}

	s := "CREATE TABLE IF NOT EXISTS " + TblParticles + " (particle INTEGER, iter INTEGER, val REAL, posid BLOB, velid BLOB, vel INTEGER);"
	_, err := m.Db.Exec(s)
	if checkdberr(err) {
		return
	}

	s = "CREATE TABLE IF NOT EXISTS " + TblParticlesMeshed + " (particle INTEGER, iter INTEGER, val REAL, posid BLOB);"
	_, err = m.Db.Exec(s)
	if checkdberr(err) {
		return
	}

	s = "CREATE TABLE IF NOT EXISTS " + TblParticlesBest + " (particle INTEGER, iter INTEGER, best REAL, posid BLOB);"
	_, err = m.Db.Exec(s)
	if checkdberr(err) {
		return
	}

	s = "CREATE TABLE IF NOT EXISTS " + TblBest + " (iter INTEGER, val REAL, posid BLOB);"
	_, err = m.Db.Exec(s)
	if checkdberr(err) {
		return
	}
}

func (m *Method) updateDb(mesh optim.Mesh) {
	if m.Db == nil {
		return
	}

	tx, err := m.Db.Begin()
	if err != nil {
		panic(err.Error())
	}
	defer tx.Commit()

	s0, err := tx.Prepare("INSERT INTO " + TblParticles + " (particle,iter,val,posid,velid,vel) VALUES (?,?,?,?,?,?);")
	if checkdberr(err) {
		return
	}
	s0b, err := tx.Prepare("INSERT INTO " + TblParticlesMeshed + " (particle,iter,val,posid) VALUES (?,?,?,?);")
	if checkdberr(err) {
		return
	}
	s1, err := tx.Prepare("INSERT INTO " + TblParticlesBest + " (particle,iter,best,posid) VALUES (?,?,?,?);")
	if checkdberr(err) {
		return
	}

	pts := []*optim.Point{}

	for _, p := range m.Pop {
		vel := &optim.Point{Pos: p.Vel}
		pts = append(pts, p.Point)
		pts = append(pts, p.Best) // best might be a projected location and not present in normal eval points
		pts = append(pts, vel)

		_, err := s0.Exec(p.Id, m.iter, p.Val, p.HashSlice(), vel.HashSlice(), p.L2Vel())
		if checkdberr(err) {
			return
		}

		_, err = s1.Exec(p.Id, m.iter, p.Best.Val, p.Best.HashSlice())
		if checkdberr(err) {
			return
		}

		pp := &optim.Point{mesh.Nearest(p.Pos), p.Val}
		_, err = s0b.Exec(p.Id, m.iter, p.Val, pp.HashSlice())
		if checkdberr(err) {
			return
		}
	}

	s2, err := tx.Prepare("INSERT INTO " + TblBest + " (iter,val,posid) VALUES (?,?,?);")
	glob := m.best
	_, err = s2.Exec(m.iter, glob.Val, glob.HashSlice())
	if checkdberr(err) {
		return
	}

	pts = append(pts, glob)
	err = optim.RecordPointPos(tx, pts...)
	if checkdberr(err) {
		return
	}
}

// TODO: remove all uses of this
func checkdberr(err error) bool {
	if err != nil {
		log.Print("swarm: db write failed -", err)
		return true
	}
	return false
}

func vmaxfrombounds(low, up []float64) []float64 {
	vmax := make([]float64, len(low))
	for i := range vmax {
		// Eberhart et al. suggest this: (up-low)/2 - removing divide by two
		// seems to help swarm avoid premature convergence in difficult
		// problems.
		vmax[i] = (up[i] - low[i])
	}
	return vmax
}
