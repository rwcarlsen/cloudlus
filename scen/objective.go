package scen

import (
	"database/sql"
	"math"
)

// ObjExecFunc is a function that, when called, runs a the single simulation
// described and computes the single objective specified in s, returning the
// objective value and any error.  Implementations of this function will
// likely make calls to github.com/rwcarlsen/cloudlus/runsim.[Local/Remote].
// in order to accomplish this.
type ObjExecFunc func(s *Scenario) (float64, error)

// ModeFunc is a function that performs the full, overall objective function
// computation for a scenario, including running the actual simulations.  The
// full objective function may involve multiple; obj will be called once for
// each simulation to compute the sub-objectives.  The final, aggregate
// objective is computed and returned along with any error.
type ModeFunc func(s *Scenario, obj ObjExecFunc) (float64, error)

func singleMode(s *Scenario, obj ObjExecFunc) (float64, error) { return obj(s) }

// doubleMode is just for testing to see that the mode handling for scenarios
// works.
func doubleMode(s *Scenario, obj ObjExecFunc) (float64, error) {
	ch := make(chan float64)
	go func() { val, _ := obj(s); ch <- val }()
	go func() { val, _ := obj(s); ch <- val }()
	val1 := <-ch
	val2 := <-ch
	return val1 + val2, nil
}

// Modes holds all possible Scenario.ObjMode values for a scenario:
//
//   * single: Used for calculating a single-simulation, simple objective
//   function for a scenario.
//
//   * disrup-multi: Used to compute a multi-simulation weighted average
//   objective function for the scenario (i.e. runs several single mode
//   sub-scenario objective calcs using the
//   Scenario.CustomConfig["disrup-multi"]=[]Disruption{...} with
//   corresponding disruption points, probabilities, etc.  The probabilities
//   must sum up to 1.0.
//
//   * disrup-multi-lin: Is the same as disrup-multi except sub objectives are
//   computed by using a linear combination of the normal calculated sub
//   objective with the disruption-time-specific optimized objective value.
//   Weights are proportional to the fraction the simulation that was pre/post
//   disruption.  This uses the same CustomConfig key and value as
//   disrup-multi, except KnownBest values must be set for each disruption.
//
//   * disrup-single-lin: Is the same as disrup-single except objective is
//   computed by using a linear combination of the normal calculated
//   objective with the disruption-time-specific optimized objective value.
//   Weights are proportional to the fraction the simulation that was pre/post
//   disruption.  This uses the same CustomConfig key and value as
//   disrup-multi, except KnownBest value must be set for the disruption.
//
//   * disrup-single: Used to compute a single-simulation objective function
//   for the scenario but also inserting a disruption at the specified point
//   using the Scenario.CustomConfig["disrup-multi"]=[]Disruption{...} with
//   corresponding disruption times, prototype to disrupt, etc.
var Modes = map[string]ModeFunc{
	"":                  singleMode,
	"single":            singleMode,
	"disrup-multi":      disrupMode,
	"disrup-multi-lin":  disrupModeLin,
	"disrup-single":     disrupSingleMode,
	"disrup-single-lin": disrupSingleModeLin,
	"double":            doubleMode, // for testing
}

// ObjFunc computes objective function values for scen using already-generated
// simulation data for the given simulation id available in db.
type ObjFunc func(scen *Scenario, db *sql.DB, simid []byte) (float64, error)

// ObjFuncs is a master list of string keyed functions that are
// referenced/used for computing objective values for scen.Scenarios.  New
// alternative objective functions should be added to this list.
var ObjFuncs = map[string]ObjFunc{
	"":                  ObjSlowVsFastPower,
	"slowvfast":         ObjSlowVsFastPower,
	"slowvfast-penalty": ObjSlowVsFastPowerPenalty,
	"slowvfast-fueled":  ObjSlowVsFastPowerFueled,
	"ans2014":           ObjANS2014,
}

// ObjSlowVsFastPower returns:
//
//    (thermal reactor energy) / (total energy)
//
// where 'slow_reactor' and 'fast_reactor' must be the names of the thermal
// and fast reactor prototypes respectively.  It is assumed that there are no
// other reactor prototypes deployed in the simulation.
func ObjSlowVsFastPower(scen *Scenario, db *sql.DB, simid []byte) (float64, error) {
	// add up overnight and operating costs converted to PV(t=0)
	q1 := `
        SELECT TOTAL(Value) FROM timeseriespower AS p
           JOIN agents AS a ON a.agentid=p.agentid AND a.simid=p.simid
           WHERE a.Prototype IN (?,?) AND p.simid=?
		`

	slowpower := 0.0
	err := db.QueryRow(q1, "slow_reactor", "init_slow_reactor", simid).Scan(&slowpower)
	if err != nil {
		return math.Inf(1), err
	}

	fastpower := 0.0
	err = db.QueryRow(q1, "fast_reactor", "fast_reactor", simid).Scan(&fastpower)
	if err != nil {
		return math.Inf(1), err
	}

	return slowpower / (slowpower + fastpower), nil
}

// ObjSlowVsFastPowerPenalty is the same as ObjSlowVsFastPower except there is
// an extra factor [(total installed MWe years) / (tot energy produced)]
// multiplied onto the objective that penalizes offline capacity due to e.g.
// fuel shortages.
func ObjSlowVsFastPowerPenalty(scen *Scenario, db *sql.DB, simid []byte) (float64, error) {
	// calculate actual generated power
	q1 := `
        SELECT TOTAL(Value) FROM timeseriespower AS p
           JOIN agents AS a ON a.agentid=p.agentid AND a.simid=p.simid
           WHERE a.Prototype IN (?,?) AND p.simid=?
		`

	slowE := 0.0
	err := db.QueryRow(q1, "slow_reactor", "init_slow_reactor", simid).Scan(&slowE)
	if err != nil {
		return math.Inf(1), err
	}

	fastE := 0.0
	err = db.QueryRow(q1, "fast_reactor", "fast_reactor", simid).Scan(&fastE)
	if err != nil {
		return math.Inf(1), err
	}

	totE := slowE + fastE

	// calculate integrated capacity
	builds := map[string][]Build{}
	for _, b := range scen.Builds {
		builds[b.Proto] = append(builds[b.Proto], b)
	}
	totcap := 0.0
	for t := 0; t < scen.SimDur; t++ {
		totcap += scen.PowerCap(builds, t)
	}

	return slowE / totE * totcap / totE, nil
}

// ObjSlowVsFastPowerFueled returns:
//
//     [(thermal reactor energy) + (total reactor capacity)] / (total energy)
//
// where 'slow_reactor' and 'fast_reactor' must be the names of the thermal
// and fast reactor prototypes respectively.  It is assumed that there are no
// other reactor prototypes deployed in the simulation.
func ObjSlowVsFastPowerFueled(scen *Scenario, db *sql.DB, simid []byte) (float64, error) {
	q1 := `
    	SELECT TOTAL(Value) FROM timeseriespower AS p
           JOIN agents AS a ON a.agentid=p.agentid AND a.simid=p.simid
           WHERE a.Prototype=? AND p.simid=?
		`

	slowpower := 0.0
	err := db.QueryRow(q1, "slow_reactor", simid).Scan(&slowpower)
	if err != nil {
		return math.Inf(1), err
	}

	fastpower := 0.0
	err = db.QueryRow(q1, "fast_reactor", simid).Scan(&fastpower)
	if err != nil {
		return math.Inf(1), err
	}

	// total capacity
	builds := map[string][]Build{}
	for _, b := range scen.Builds {
		builds[b.Proto] = append(builds[b.Proto], b)
	}
	totcap := 0.0
	for t := 0; t < scen.SimDur; t++ {
		totcap += scen.PowerCap(builds, t)
	}

	return (slowpower + totcap) / (slowpower + fastpower), nil
}
