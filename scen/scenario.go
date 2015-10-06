package scen

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"path/filepath"
	"text/template"
)

// Facility represents a cyclus agent prototype that could be built by the
// optimizer.
type Facility struct {
	Proto string
	// Cap is the total Power output capacity of the facility.
	Cap float64
	// The lifetime of the facility (in timesteps). The lifetime must also
	// be specified manually (consistent with this value) in the prototype
	// definition in the cyclus input template file.
	Life int
	// BuildAfter is the time step after which this facility type can be built.
	// -1 for never available, and 0 for always available.
	BuildAfter int
	// FracOfProto names a prototype that build fractions of this prototype
	// are a portion of.
	FracOfProtos []string
}

// Alive returns whether or not a facility built at the specified time is
// still operating/active at t.
func (f *Facility) Alive(built, t int) bool { return Alive(built, t, f.Life) }

// Available returns true if the facility type can be built at time t.
func (f *Facility) Available(t int) bool {
	return t >= f.BuildAfter && f.BuildAfter >= 0
}

type Build struct {
	Time  int
	Proto string
	N     int
	Life  int
	fac   Facility
}

// Alive returns whether or not the facility is still operabing/active at t.
func (b Build) Alive(t int) bool { return Alive(b.Time, t, b.Lifetime()) }

func (b Build) Lifetime() int {
	if b.Life > 0 {
		return b.Life
	} else if b.fac.Life > 0 {
		return b.fac.Life
	} else {
		return -1
	}
}

// Alive returns whether or not a facility with the given lifetime and built
// at the specified time is still operating/active at t.
func Alive(built, t, life int) bool {
	return built <= t && (built+life > t || life <= 0)
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
	// CyclusTmpl is the relative path to the text templated cyclus input file
	// rooted from the directory of the scenario file.
	CyclusTmpl string
	// BuildPeriod is the number of timesteps between timesteps in which
	// facilities are deployed
	BuildPeriod int
	// NuclideCost represents the waste cost per kg material per time step for
	// each nuclide in the entire simulation (repository's exempt).
	NuclideCost map[string]float64
	// ObjFunc is the name of the objective function in the
	// github.com/rwcarlsen/cloudlus/scen.ObjFuncs map to be used for
	// objective value calculations.
	ObjFunc string
	// ObjMode identifies the way the overall objective value is computed for
	// this scenario.  It must be one of the names in the Modes map.  The
	// default (empty string) is to just run a single simulation and use the
	// returned value of the chosen ObjFunc as the objective value.  Other
	// modes allow things like a scenario involving many sub-simulations whose
	// objectives are combined to a single value.
	ObjMode string
	// SpliceVars holds an optional complete set of variable values that can
	// be spliced with the actual scenario variable values.  Times before the
	// splice time use the SpliceVars values, and times after the splice time
	// use the actual variable values passed to TransformVars.
	SpliceVars []float64
	// SpliceTime is the time before which SpliceVars (if defined) are used
	// instead of the actual passed variables for TransformVars.
	SpliceTime int
	// SingleCalc is for internal usage (not users) and is marked true for
	// multi-sim scenarios where the current simulation being run is a
	// sub-[scenario/simulation] and CalcObjective should be called instead of
	// CalcTotalObjective.  Without this, running a simulation remotely would fire off one
	// job for each sub-simulation, and then each remote worker would also
	// fire off one simulation for each sub-simulation - causing problems.
	SingleCalc bool
	// Discount represents the nominal annual discount rate (including
	// inflation) for the simulation.
	Discount float64
	// CustomConfig is for internal use for sub-scenario-specific
	// configuration used in things like disruption scenarios where each run
	// or objective evaluation consists of multiple simulations with various
	// perturbations.
	CustomConfig map[string]interface{}
	// Facs is a list of facilities that could be built and associated
	// parameters relevant to the optimization objective.
	Facs []Facility
	// MinPower is a series of min deployed power capacity requirements that
	// must be maintained for each build period.
	MinPower []float64
	// MaxPower is a series of max deployed power capacity requirements that
	// must be maintained for each build period.
	MaxPower []float64
	// StartBuilds holds the set of build schedule values for all agents
	// initially in the scenario (not added/deployed by optimizer).
	StartBuilds []Build
	// Builds holds all scenario deployments (including startbuilds).  This is
	// only non-nil after TransformVars has been called.
	Builds []Build
	// File is the name of the scenario file. This is for internal use and
	// does not need to be filled out by the user.
	File string
	// Handle is used internally and does not need to be specified by the
	// user.
	Handle string
	// tmpl is a cach for the templated cyclus input file
	tmpl *template.Template
}

func (s *Scenario) Clone() *Scenario {
	data, _ := json.Marshal(s)
	clone := &Scenario{}
	json.Unmarshal(data, &clone)
	clone.Validate()
	return clone
}

func (s *Scenario) reactors() []Facility {
	rs := []Facility{}
	for _, fac := range s.Facs {
		if fac.Cap > 0 && fac.BuildAfter >= 0 {
			rs = append(rs, fac)
		}
	}
	return rs
}

func (s *Scenario) notreactors() []Facility {
	fs := []Facility{}
	for _, fac := range s.Facs {
		if fac.Cap == 0 && fac.BuildAfter >= 0 {
			fs = append(fs, fac)
		}
	}
	return fs
}

func (s *Scenario) Prototype(proto string) (Facility, error) {
	for _, fac := range s.Facs {
		if fac.Proto == proto {
			return fac, nil
		}
	}
	return Facility{}, fmt.Errorf("no prototype named '%v'", proto)
}

func (s *Scenario) NVars() int { return s.NVarsPerPeriod() * s.nperiods() }

func (s *Scenario) NVarsPerPeriod() int {
	numFacVars := len(s.reactors()) + len(s.notreactors()) - 1
	numPowerVars := 1
	return numFacVars + numPowerVars
}

func (s *Scenario) periodFacOrder() (varfacs []Facility, implicitreactor Facility) {
	err := s.Validate()
	if err != nil {
		panic(err.Error())
	}

	facs := []Facility{}
	facs = append(facs, Facility{}) // add blank to account for power var offset
	for _, fac := range s.reactors()[1:] {
		facs = append(facs, fac)
	}
	for _, fac := range s.notreactors() {
		facs = append(facs, fac)
	}
	return facs, s.reactors()[0]
}

func (s *Scenario) PrintStats() {
	err := s.Validate()
	if err != nil {
		log.Fatal(err)
	}

	builds := map[string][]Build{}
	for _, b := range s.Builds {
		builds[b.Proto] = append(builds[b.Proto], b)
	}

	for i, t := range s.periodTimes() {
		currpow := s.PowerCap(builds, t)
		capbuilt := s.CapBuilt(s.Builds, t)
		maxpow := s.MaxPower[i]
		minpow := s.MinPower[i]
		fmt.Printf("t%v: capbuilt=%v, currpow=%v, minpow=%v, maxpow=%v\n", t, capbuilt, currpow, minpow, maxpow)
	}
}

func (s *Scenario) TransformSched() ([]float64, error) {
	err := s.Validate()
	if err != nil {
		return nil, err
	}

	builds := map[string][]Build{}
	for _, b := range s.Builds {
		builds[b.Proto] = append(builds[b.Proto], b)
	}

	startbuilds := map[string][]Build{}
	for _, b := range s.StartBuilds {
		startbuilds[b.Proto] = append(startbuilds[b.Proto], b)
	}

	varfacs, _ := s.periodFacOrder()
	vars := make([]float64, s.NVars())
	for i, t := range s.periodTimes() {
		currpow := s.PowerCap(builds, t)
		capbuilt := s.CapBuilt(s.Builds, t) - s.CapBuilt(s.StartBuilds, t)
		prevpow := currpow - capbuilt

		maxpow := s.MaxPower[i]
		lower := math.Max(s.MinPower[i], prevpow)
		powerrange := math.Max(1e-10, maxpow-lower)
		minbuild := math.Max(0, lower-prevpow)

		powervar := math.Min(1, (capbuilt-minbuild)/powerrange)
		powervar = math.Max(0, powervar)
		vars[i*s.NVarsPerPeriod()] = powervar

		// handle reactor builds
		capleft := math.Max(1e-10, capbuilt)
		// skip j = 0 which is the power cap variable
		j := 1
		for j = 1; j < s.NVarsPerPeriod(); j++ {
			fac := varfacs[j]
			if fac.Cap > 0 && fac.Available(t) {
				protocap := s.CapBuilt(builds[fac.Proto], t)
				index := i*s.NVarsPerPeriod() + j
				vars[index] = math.Min(1, protocap/capleft)
				vars[index] = math.Max(0, vars[index])
				capleft -= protocap
			} else {
				// done processing reactors (except last one)
				break
			}
		}

		// handle other facilities
		for ; j < s.NVarsPerPeriod(); j++ {
			fac := varfacs[j]
			if !fac.Available(t) { // skip
				continue
			}

			nref := s.naliveproto(builds, t, fac.FracOfProtos...)
			nhave := s.naliveproto(builds, t, fac.Proto)

			index := i*s.NVarsPerPeriod() + j
			vars[index] = math.Min(1, float64(nhave)/float64(nref))
			vars[index] = math.Max(0, vars[index])
		}
	}
	return vars, nil
}

func (s *Scenario) NBuilt(builds []Build, t int) int {
	n := 0
	for _, b := range builds {
		if b.Time == t {
			n += b.N
		}
	}
	return n
}

func (s *Scenario) CapBuilt(builds []Build, t int) float64 {
	tot := 0.0
	for _, b := range builds {
		if b.Time == t {
			fac, err := s.Prototype(b.Proto)
			if err != nil {
				panic(err.Error())
			}
			tot += float64(b.N) * fac.Cap
		}
	}
	return tot
}

func (s *Scenario) splice(origvars []float64) []float64 {
	if len(s.SpliceVars) == 0 {
		return origvars
	}

	vars := make([]float64, len(origvars))
	copy(vars, origvars)
outer:
	for i, t := range s.periodTimes() {
		if t >= s.SpliceTime {
			break
		}
		for j := 0; j < s.NVarsPerPeriod(); j++ {
			index := i*s.NVarsPerPeriod() + j
			if index >= len(s.SpliceVars) {
				break outer
			}
			vars[index] = s.SpliceVars[index]
		}
	}
	return vars
}

// TransformVars takes a sequence of input variables for the scenario and
// transforms them into a set of prototype/facility deployments. The sequence
// of the vars follows this pattern: fac1_t1, fac1_t2, ..., fac1_tn, fac2_t1,
// ..., facm_t1, facm_t2, ..., facm_tn.
//
// The first reactor type variable represents the total fraction of new built
// power capacity satisfied by that reactor on the given time step.  For each
// subsequent reactor type (except the last), the variables represent the
// fraction of the remaining power capacity satisfied by that reactor type
// (e.g. the third reactor type's variable can be used to calculate its
// fraction like this (1-(react1frac + (1-react1frac) * react2frac)) *
// react3frac).  The last reactor type fraction is simply the remainining
// unsatisfied power capacity.
func (s *Scenario) TransformVars(vars []float64) (map[string][]Build, error) {
	err := s.Validate()
	if err != nil {
		return nil, err
	} else if len(vars) != s.NVars() {
		return nil, fmt.Errorf("wrong number of vars: want %v, got %v", s.NVars(), len(vars))
	}

	vars = s.splice(vars)

	up := s.UpperBounds()
	low := s.LowerBounds()
	for i, v := range vars {
		if v < low[i] {
			vars[i] = low[i]
		}
		if v > up[i] {
			vars[i] = up[i]
		}
	}

	builds := map[string][]Build{}
	for _, b := range s.StartBuilds {
		builds[b.Proto] = append(builds[b.Proto], b)
	}

	varfacs, implicitreactor := s.periodFacOrder()
	for i, t := range s.periodTimes() {
		minpow := s.MinPower[i]
		maxpow := s.MaxPower[i]
		currpower := s.PowerCap(builds, t)
		powervar := vars[i*s.NVarsPerPeriod()]

		lowerbound := math.Max(currpower, minpow)
		powerrange := math.Max(0, maxpow-lowerbound)
		newpower := powervar*powerrange + lowerbound
		captobuild := math.Max(newpower-currpower, 0)

		// handle reactor builds
		capleft := captobuild
		j := 1 // skip j = 0 which is the power cap variable
		for j = 1; j < s.NVarsPerPeriod(); j++ {
			val := vars[i*s.NVarsPerPeriod()+j]
			fac := varfacs[j]
			if fac.Cap > 0 && fac.Available(t) {
				wantcap := val * capleft
				nbuild := int(math.Max(0, math.Floor(wantcap/fac.Cap+0.5)))
				capleft -= float64(nbuild) * fac.Cap

				if nbuild > 0 {
					builds[fac.Proto] = append(builds[fac.Proto], Build{
						Time:  t,
						Proto: fac.Proto,
						N:     nbuild,
						fac:   fac,
					})
				}
			} else {
				// done processing reactors (except last one)
				break
			}
		}

		// handle last (implicit) reactor
		fac := implicitreactor
		if fac.Available(t) {
			wantcap := capleft
			nbuild := int(math.Max(0, math.Floor(wantcap/fac.Cap+0.5)))

			if nbuild > 0 {
				builds[fac.Proto] = append(builds[fac.Proto], Build{
					Time:  t,
					Proto: fac.Proto,
					N:     nbuild,
					fac:   fac,
				})
			}
		}

		// handle other facilities
		for ; j < s.NVarsPerPeriod(); j++ {
			facfrac := vars[i*s.NVarsPerPeriod()+j]
			fac := varfacs[j]
			if !fac.Available(t) { // skip
				continue
			}

			haven := float64(s.naliveproto(builds, t, fac.Proto))
			needn := facfrac * float64(s.naliveproto(builds, t, fac.FracOfProtos...))
			wantn := math.Max(0, needn-haven)
			nbuild := int(math.Floor(wantn + 0.5))
			if nbuild > 0 {
				builds[fac.Proto] = append(builds[fac.Proto], Build{
					Time:  t,
					Proto: fac.Proto,
					N:     nbuild,
					fac:   fac,
				})
			}
		}
	}

	s.Builds = nil
	for _, fac := range s.Facs {
		blds := builds[fac.Proto]
		for _, b := range blds {
			s.Builds = append(s.Builds, b)
		}
	}

	return builds, nil
}

func (s *Scenario) naliveproto(facs map[string][]Build, t int, protos ...string) int {
	count := 0
	for _, proto := range protos {
		builds := facs[proto]
		for _, b := range builds {
			if b.Alive(t) {
				count += b.N
			}
		}
	}
	return count
}

func (s *Scenario) PowerCap(builds map[string][]Build, t int) float64 {
	pow := 0.0
	for _, buildsproto := range builds {
		for _, b := range buildsproto {
			if b.Alive(t) {
				pow += b.fac.Cap * float64(b.N)
			}
		}
	}
	return pow
}

func (s *Scenario) CyclusTmplPath() string {
	return filepath.Join(filepath.Dir(s.File), s.CyclusTmpl)
}

// Validate returns an error if the scenario is ill-configured.
func (s *Scenario) Validate() error {
	if min, max := len(s.MinPower), len(s.MaxPower); min != max {
		return fmt.Errorf("MaxPower length %v != MinPower length %v", max, min)
	}

	var err error
	if s.tmpl == nil && s.CyclusTmpl != "" {
		s.tmpl, err = template.ParseFiles(s.CyclusTmplPath())
		if err != nil {
			return err
		}
	}

	np := s.nperiods()
	lmin := len(s.MinPower)
	if np != lmin {
		return fmt.Errorf("number power constraints %v != number build periods %v", lmin, np)
	}

	protos := map[string]Facility{}
	havereactor := false
	for _, fac := range s.Facs {
		if fac.Cap > 0 {
			havereactor = true
		}
		if fac.Cap == 0 && len(fac.FracOfProtos) == 0 && fac.BuildAfter >= 0 {
			return fmt.Errorf("prototype %v needs at least one prototype defined in FracOfProtos", fac.Proto)
		}
		protos[fac.Proto] = fac
	}
	if !havereactor {
		return fmt.Errorf("scenario has no nonzero capacity (i.e. reactor) prototypes")
	}

	for i, p := range s.StartBuilds {
		fac, ok := protos[p.Proto]
		if !ok {
			return fmt.Errorf("StartBuild prototype '%v' is not defined in Facs", p.Proto)
		}
		s.StartBuilds[i].fac = fac
	}

	for i, p := range s.Builds {
		fac, ok := protos[p.Proto]
		if !ok {
			return fmt.Errorf("Build prototype '%v' is not defined in Facs", p.Proto)
		}
		s.Builds[i].fac = fac
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
	return s.Validate()
}

func (s *Scenario) CalcTotalObjective(execfn ObjExecFunc) (float64, error) {
	if s.SingleCalc {
		return execfn(s)
	}

	s.SingleCalc = true
	defer func() { s.SingleCalc = false }()

	modefn, ok := Modes[s.ObjMode]
	if !ok {
		return math.Inf(1), fmt.Errorf("invalid mode name '%v'", s.ObjMode)
	}
	return modefn(s, execfn)
}

// CalcObjective computes the single-simulation objective value for data
// stored in dbfile under the given simulation id.
func (s *Scenario) CalcObjective(dbfile string, simid []byte) (float64, error) {
	if fn, ok := ObjFuncs[s.ObjFunc]; ok {
		db, err := sql.Open("sqlite3", dbfile)
		if err != nil {
			return math.Inf(1), err
		}
		defer db.Close()

		return fn(s, db, simid)
	} else {
		return math.Inf(1), fmt.Errorf("invalid objective name '%v'", s.ObjFunc)
	}
}

func (s *Scenario) GenCyclusInfile() ([]byte, error) {
	if s.Handle == "" {
		s.Handle = "none"
	}

	if s.tmpl == nil {
		s.tmpl = template.Must(template.ParseFiles(s.CyclusTmplPath()))
	}

	var buf bytes.Buffer
	err := s.tmpl.Execute(&buf, s)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *Scenario) VarNames() []string {
	names := make([]string, 0, s.NVars())
	varfacs, _ := s.periodFacOrder()
	for i := range s.periodTimes() {
		for j := range varfacs {
			names = append(names, fmt.Sprintf("t%v_f%v", i, j))
		}
	}
	return names
}

func (s *Scenario) LowerBounds() []float64 {
	return make([]float64, s.NVars())
}

func (s *Scenario) UpperBounds() []float64 {
	facs, _ := s.periodFacOrder()
	up := make([]float64, 0, s.NVars())
	for _, t := range s.periodTimes() {
		for j, fac := range facs {
			if j == 0 { // power var
				up = append(up, 1)
			} else if fac.BuildAfter == -1 { // never can build
				up = append(up, 0)
			} else if fac.BuildAfter > 0 && fac.BuildAfter > t {
				up = append(up, 0)
			} else {
				up = append(up, 1)
			}
		}
	}
	return up
}

func (s *Scenario) timeOf(period int) int {
	return period*s.BuildPeriod + 1 + s.BuildOffset
}

func (s *Scenario) periodOf(time int) int {
	return (time - s.BuildOffset - 1) / s.BuildPeriod
}

func (s *Scenario) periodTimes() []int {
	periods := make([]int, s.nperiods())
	for i := range periods {
		periods[i] = s.timeOf(i)
	}
	return periods
}

func (s *Scenario) nperiods() int {
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
