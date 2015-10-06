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

func SingleMode(s *Scenario, obj ObjExecFunc) (float64, error) { return obj(s) }

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

var Modes = map[string]ModeFunc{
	"":              SingleMode,
	"single":        SingleMode,
	"disrup-multi":  disrupMode,
	"disrup-single": disrupSingleMode,
	"double":        doubleMode, // for testing
}

// ObjFunc computes objective function values for scen using already-generated
// simulation data for the given simulation id available in db.
type ObjFunc func(scen *Scenario, db *sql.DB, simid []byte) (float64, error)

// ObjFuncs is a master list of string keyed functions that are
// referenced/used for computing objective values for scen.Scenarios.  New
// alternative objective functions should be added to this list.
var ObjFuncs = map[string]ObjFunc{
	"":                 ObjSlowVsFastPower,
	"slowvfast":        ObjSlowVsFastPower,
	"slowvfast-fueled": ObjSlowVsFastPowerFueled,
	"ans2014":          ObjANS2014,
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
    	SELECT SUM(Value) FROM timeseriespower AS p
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

// ObjSlowVsFastPowerFueled returns:
//
//     [(thermal reactor energy) + (total reactor capacity)] / (total energy)
//
// where 'slow_reactor' and 'fast_reactor' must be the names of the thermal
// and fast reactor prototypes respectively.  It is assumed that there are no
// other reactor prototypes deployed in the simulation.
func ObjSlowVsFastPowerFueled(scen *Scenario, db *sql.DB, simid []byte) (float64, error) {
	// add up overnight and operating costs converted to PV(t=0)
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
