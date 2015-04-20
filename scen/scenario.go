package scen

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"text/template"

	"code.google.com/p/go-uuid/uuid"

	_ "github.com/gonum/blas/native"
	"github.com/gonum/matrix/mat64"
	_ "github.com/mxk/go-sqlite/sqlite3"
	"github.com/rwcarlsen/cyan/nuc"
	"github.com/rwcarlsen/cyan/post"
	"github.com/rwcarlsen/cyan/query"
)

// Facility represents a cyclus agent prototype that could be built by the
// optimizer.
type Facility struct {
	Proto string
	// Cap is the total Power output capacity of the facility.
	Cap float64
	// OpCost represents the per timstep operating cost for the facility
	OpCost float64
	// CapitalCost represents the overnight cost for building the facility
	CapitalCost float64
	// The lifetime of the facility (in timesteps). The lifetime must also
	// be specified manually (consistent with this value) in the prototype
	// definition in the cyclus input template file.
	Life int
	// BuildAfter is the time step after which this facility type can be built.
	// -1 for never available, and 0 for always available.
	BuildAfter int
	// WasteDiscount represents the fraction is discounted from the waste cost
	// for this facility.
	WasteDiscount float64
}

// Alive returns whether or not a facility built at the specified time is
// still operating/active at t.
func (f *Facility) Alive(built, t int) bool { return Alive(built, t, f.Life) }

// Available returns true if the facility type can be built at time t.
func (f *Facility) Available(t int) bool {
	return t >= f.BuildAfter && f.BuildAfter >= 0
}

type Param struct {
	Time  int
	Proto string
	N     int
	Life  int
}

// Alive returns whether or not a facility with the given lifetime and built
// at the specified time is still operating/active at t.
func Alive(built, t, life int) bool {
	return built <= t && (built+life >= t || life <= 0)
}

type Scenario struct {
	// SimDur is the simulation duration in timesteps (months)
	SimDur int
	// BuildOffset is the number of timesteps after simulation start at which
	// deployments actually begin.  This allows facilities and other initial
	// conditions to be set up and run before the deploying begins.
	BuildOffset int
	// TrailingDur is the number of timesteps of the simulation duration that
	// are reserved for wind-down - no new deployments will be made.
	TrailingDur int
	// CyclusTmpl is the path to the text templated cyclus input file.
	CyclusTmpl string
	// BuildPeriod is the number of timesteps between timesteps in which
	// facilities are deployed
	BuildPeriod int
	// NuclideCost represents the waste cost per kg material per time step for
	// each nuclide in the entire simulation (repository's exempt).
	NuclideCost map[string]float64
	// Discount represents the nominal annual discount rate (including
	// inflation) for the simulation.
	Discount float64
	// Facs is a list of facilities that could be built and associated
	// parameters relevant to the optimization objective.
	Facs []Facility
	// MinPower is a series of min deployed power capacity requirements that
	// must be maintained for each build period.
	MinPower []float64
	// MaxPower is a series of max deployed power capacity requirements that
	// must be maintained for each build period.
	MaxPower []float64
	// Params holds the set of build schedule values for all agents in the
	// scenario.  This can be used to specify initial condition deployments.
	Params []Param
	// Addr is the location of the cyclus simulation execution server.  An
	// empty string "" indicates that simulations will run locally.
	Addr string
	// File is the name of the scenario file. This is for internal use and
	// does not need to be filled out by the user.
	File string
	// Handle is used internally and does not need to be specified by the
	// user.
	Handle string
}

func (s *Scenario) ExpandParams() []Param {
	lifes := map[string]int{}
	for _, fac := range s.Facs {
		lifes[fac.Proto] = fac.Life
	}

	params := []Param{}
	for _, p := range s.Params {
		life := lifes[p.Proto]
		if p.Life > 0 {
			life = p.Life
		}
		params = append(params, Param{p.Time, p.Proto, p.N, life})
	}
	return params
}

// Validate returns an error if the scenario is ill-configured.
func (s *Scenario) Validate() error {
	if min, max := len(s.MinPower), len(s.MaxPower); min != max {
		return fmt.Errorf("MaxPower length %v != MinPower length %v", max, min)
	}

	np := s.nPeriods()
	lmin := len(s.MinPower)
	if np != lmin {
		return fmt.Errorf("number power constraints %v != number build periods %v", lmin, np)
	}

	protos := map[string]bool{}
	for _, fac := range s.Facs {
		protos[fac.Proto] = true
	}

	for _, p := range s.Params {
		if !protos[p.Proto] {
			return fmt.Errorf("param prototype '%v' is not defined in Facs", p.Proto)
		}
	}
	return nil
}

func (s *Scenario) Load(fname string) error {
	if s == nil {
		s = &Scenario{}
	}
	data, err := ioutil.ReadFile(fname)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, s); err != nil {
		if serr, ok := err.(*json.SyntaxError); ok {
			line, col := findLine(data, serr.Offset)
			return fmt.Errorf("%s:%d:%d: %v", fname, line, col, err)
		}
		return err
	}

	s.File = fname
	if len(s.Params) == 0 {
		s.Params = make([]Param, s.Nvars())
	}
	return s.Validate()
}

func (s *Scenario) GenCyclusInfile() ([]byte, error) {
	if s.Handle == "" {
		s.Handle = "none"
	}

	var buf bytes.Buffer
	tmpl := s.CyclusTmpl
	t := template.Must(template.ParseFiles(tmpl))

	err := t.Execute(&buf, s)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (s *Scenario) Run(stdout, stderr io.Writer) (dbfile string, simid []byte, err error) {
	// generate cyclus input file and run cyclus
	ui := uuid.NewRandom()
	cycin := ui.String() + ".cyclus.xml"
	cycout := ui.String() + ".sqlite"

	data, err := s.GenCyclusInfile()
	if err != nil {
		return "", nil, err
	}
	err = ioutil.WriteFile(cycin, data, 0644)
	if err != nil {
		return "", nil, err
	}

	cmd := exec.Command("cyclus", cycin, "-o", cycout)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if stdout != nil {
		cmd.Stdout = stdout
	}
	if stderr != nil {
		cmd.Stderr = stderr
	}

	if err := cmd.Run(); err != nil {
		return "", nil, err
	}

	// post process cyclus output db
	db, err := sql.Open("sqlite3", cycout)
	if err != nil {
		return "", nil, err
	}
	defer db.Close()

	simids, err := post.Process(db)
	if err != nil {
		return "", nil, err
	}

	return cycout, simids[0], nil
}

func (s *Scenario) InitParams(vals []int) {
	for i, val := range vals {
		f := i / s.nPeriods()
		t := s.timeOf(i % s.nPeriods())
		s.Params = append(s.Params, Param{Proto: s.Facs[f].Proto, Time: t, N: val})
	}
}

func (s *Scenario) VarNames() []string {
	nperiods := s.nPeriods()
	names := make([]string, s.Nvars())
	for f := range s.Facs {
		for n, t := range s.periodTimes() {
			i := f*nperiods + n
			names[i] = fmt.Sprintf("f%v_t%v", f, t)
		}
	}
	return names
}

func (s *Scenario) LowerBounds() *mat64.Dense {
	return mat64.NewDense(s.Nvars(), 1, nil)
}

func (s *Scenario) UpperBounds() *mat64.Dense {
	nperiods := s.nPeriods()
	up := mat64.NewDense(s.Nvars(), 1, nil)
	for f, fac := range s.Facs {
		for n, t := range s.periodTimes() {
			if !fac.Available(t) {
				up.Set(f*nperiods+n, 0, 0)
			} else if fac.Cap != 0 {
				v := s.MaxPower[n]/fac.Cap*.2 + 1
				if v < 10 {
					v = 10
				}
				for _, ifac := range s.Params {
					if ifac.Proto == fac.Proto && Alive(ifac.Time, t, fac.Life) {
						v -= float64(ifac.N)
					}
				}
				if v < 0 {
					v = 0
				}
				up.Set(f*nperiods+n, 0, v)
			} else {
				up.Set(f*nperiods+n, 0, 10)
			}
		}
	}
	return up
}

// SupportConstr builds and returns matrices representing linear inequality
// constraints with a parameter multiplier matrix A and upper and lower
// bounds. The constraint expresses that the total number of support
// facilities (i.e. not reactors) at every timestep must never be more
// than twice the number of deployed reactors.
func (s *Scenario) SupportConstr() (low, A, up *mat64.Dense) {
	nperiods := s.nPeriods()

	A = mat64.NewDense(nperiods, s.Nvars(), nil)
	low = mat64.NewDense(nperiods, 1, nil)
	tmp := make([]float64, len(s.MaxPower))
	copy(tmp, s.MaxPower)
	up = mat64.NewDense(nperiods, 1, tmp)
	up.Apply(func(r, c int, v float64) float64 { return 1e200 }, up)

	for nt, t := range s.periodTimes() {
		for f, fac := range s.Facs {
			for ntbuilt, tbuilt := range s.periodTimes() {
				if !fac.Alive(tbuilt, t) {
					continue
				}

				i := f*nperiods + ntbuilt
				if fac.Cap == 0 {
					A.Set(nt, i, -1)
				} else {
					A.Set(nt, i, 2)
				}
			}
		}
	}

	return low, A, up
}

// PowerConstr builds and returns matrices representing linear inequality
// constraints with a parameter multiplier matrix A and upper and lower
// bounds. The constraint expresses that the total power capacity deployed at
// every timestep must always be between the given MinPower and MaxPower
// scenario bounds.
func (s *Scenario) PowerConstr() (low, A, up *mat64.Dense) {
	nperiods := s.nPeriods()

	tmpl := make([]float64, len(s.MinPower))
	tmpu := make([]float64, len(s.MaxPower))
	copy(tmpl, s.MinPower)
	copy(tmpu, s.MaxPower)
	// correct for initially built capacity
	i := 0
	for _, t := range s.periodTimes() {
		cap := 0.0
		for _, fac := range s.Facs {
			for _, ifac := range s.Params {
				if ifac.Proto == fac.Proto && Alive(ifac.Time, t, fac.Life) {
					cap += fac.Cap * float64(ifac.N)
				}
			}
		}
		tmpl[i] -= cap
		tmpu[i] -= cap
		i++
	}

	low = mat64.NewDense(nperiods, 1, tmpl)
	up = mat64.NewDense(nperiods, 1, tmpu)
	A = mat64.NewDense(nperiods, s.Nvars(), nil)

	for f, fac := range s.Facs {
		for nbuilt, tbuilt := range s.periodTimes() {
			for n, t := range s.periodTimes() {
				if fac.Alive(tbuilt, t) {
					i := f*nperiods + nbuilt
					A.Set(n, i, fac.Cap)
				}
			}
		}
	}

	return low, A, up
}

// AfterConstr builds and returns matrices representing equality
// constraints with a parameter multiplier matrix A and upper and lower
// bounds. The constraint expresses that each facility can only be built after
// a certain date.
func (s *Scenario) AfterConstr() (A, target *mat64.Dense) {
	nperiods := s.nPeriods()

	// count facilities that have build time constraints
	n := 0
	for _, fac := range s.Facs {
		if fac.BuildAfter != 0 {
			n++
		}
	}

	A = mat64.NewDense(n*nperiods, s.Nvars(), nil)
	target = mat64.NewDense(n*nperiods, 1, nil)

	r := 0
	for f, fac := range s.Facs {
		if fac.BuildAfter == 0 {
			continue
		}
		for n, t := range s.periodTimes() {
			if !fac.Available(t) {
				c := f*nperiods + n
				A.Set(r, c, 1)
			}
			r++
		}
	}

	return A, target
}

func (s *Scenario) IneqConstr() (low, A, up *mat64.Dense) {
	low, A, up = &mat64.Dense{}, &mat64.Dense{}, &mat64.Dense{}
	l1, a1, u1 := s.SupportConstr()
	l2, a2, u2 := s.PowerConstr()

	low.Stack(l1, l2)
	A.Stack(a1, a2)
	up.Stack(u1, u2)

	return low, A, up
}

func (s *Scenario) ConstrMat() (A *mat64.Dense) {
	_, A, _ = s.IneqConstr()
	return A
}

func (s *Scenario) ConstrLow() (low *mat64.Dense) {
	low, _, _ = s.IneqConstr()
	return low
}

func (s *Scenario) ConstrUp() (up *mat64.Dense) {
	_, _, up = s.IneqConstr()
	return up
}

func (s *Scenario) EqConstrMat() (A *mat64.Dense) {
	A, _ = s.AfterConstr()
	return A
}

func (s *Scenario) EqConstrTarget() (target *mat64.Dense) {
	_, target = s.AfterConstr()
	return target
}

func (s *Scenario) Nvars() int {
	return s.nPeriods() * len(s.Facs)
}

func (s *Scenario) timeOf(period int) int {
	return period*s.BuildPeriod + 1 + s.BuildOffset
}

func (s *Scenario) periodOf(time int) int {
	return (time - s.BuildOffset - 1) / s.BuildPeriod
}

func (s *Scenario) periodTimes() []int {
	periods := make([]int, s.nPeriods())
	for i := range periods {
		periods[i] = s.timeOf(i)
	}
	return periods
}

func (s *Scenario) nPeriods() int {
	return (s.SimDur-s.BuildOffset-s.TrailingDur-2)/s.BuildPeriod + 1
}

func findLine(data []byte, pos int64) (line, col int) {
	line = 1
	buf := bytes.NewBuffer(data)
	for n := int64(0); n < pos; n++ {
		b, err := buf.ReadByte()
		if err != nil {
			panic(err) //I don't really see how this could happen
		}
		if b == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return
}

func (scen *Scenario) CalcObjective(dbfile string, simid []byte) (float64, error) {
	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	// add up overnight and operating costs converted to PV(t=0)
	q1 := `
		SELECT tl.Time FROM TimeList AS tl
			INNER JOIN Agents As a ON a.EnterTime <= tl.Time AND (a.ExitTime >= tl.Time OR a.ExitTime IS NULL)
		WHERE
			a.SimId = tl.SimId AND a.SimId = ?
			AND a.Prototype = ?;
		`
	q2 := `SELECT EnterTime FROM Agents WHERE SimId = ? AND Prototype = ?`

	totcost := 0.0
	for _, fac := range scen.Facs {
		// calc total operating cost
		rows, err := db.Query(q1, simid, fac.Proto)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var t int
			if err := rows.Scan(&t); err != nil {
				return 0, err
			}
			totcost += PV(fac.OpCost, t, scen.Discount)
		}
		if err := rows.Err(); err != nil {
			return 0, err
		}

		// calc overnight capital cost
		rows, err = db.Query(q2, simid, fac.Proto)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var t int
			if err := rows.Scan(&t); err != nil {
				return 0, err
			}
			totcost += PV(fac.CapitalCost, t, scen.Discount)
		}
		if err := rows.Err(); err != nil {
			return 0, err
		}

		// add in waste penalty
		ags, err := query.AllAgents(db, simid, fac.Proto)
		if err != nil {
			return 0, err
		}

		// InvAt uses all agents if no ids are passed - so we need to skip from here
		if len(ags) == 0 {
			continue
		}

		ids := make([]int, len(ags))
		for i, a := range ags {
			ids[i] = a.Id
		}

		for t := 0; t < scen.SimDur; t++ {
			mat, err := query.InvAt(db, simid, t, ids...)
			if err != nil {
				return 0, err
			}
			for nuc, qty := range mat {
				nucstr := fmt.Sprint(nuc)
				totcost += PV(scen.NuclideCost[nucstr]*float64(qty)*(1-fac.WasteDiscount), t, scen.Discount)
			}
		}
	}

	// normalize to energy produced
	joules, err := query.EnergyProduced(db, simid, 0, scen.SimDur)
	if err != nil {
		return 0, err
	}
	mwh := joules / nuc.MWh
	mult := 1e6 // to get the objective around 0.1 same magnitude as constraint penalties
	return totcost / (mwh + 1e-30) * mult, nil
}

func PV(amt float64, nt int, rate float64) float64 {
	monrate := rate / 12
	return amt / math.Pow(1+monrate, float64(nt))
}
