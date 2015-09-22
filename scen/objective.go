package scen

import (
	"database/sql"
	"math"
)

type ObjFunc func(scen *Scenario, db *sql.DB, simid []byte) (float64, error)

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
