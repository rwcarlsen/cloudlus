package scen

import (
	"database/sql"
	"fmt"
	"math"
)

type ObjFunc func(scen *Scenario, dbfile string, simid []byte) (float64, error)

var ObjFuncs = map[string]ObjFunc{
	"":                 ObjSlowVsFastPower,
	"slowvfast":        ObjSlowVsFastPower,
	"slowvfast-fueled": ObjSlowVsFastPowerFueled,
	"ans2014":          ObjANS2014,
}

// ObjSlowVsFastPowerFueled returns:
//
//    (thermal reactor energy) / (total energy)
//
// where 'slow_reactor' and 'fast_reactor' must be the names of the thermal
// and fast reactor prototypes respectively.
func ObjSlowVsFastPower(scen *Scenario, dbfile string, simid []byte) (float64, error) {
	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	// add up overnight and operating costs converted to PV(t=0)
	q1 := `
    	SELECT SUM(Value) FROM timeseriespower AS p
           JOIN agents AS a ON a.agentid=p.agentid AND a.simid=p.simid
           WHERE a.Prototype=? AND p.simid=?
		`

	slowpower := 0.0
	err = db.QueryRow(q1, "slow_reactor", simid).Scan(&slowpower)
	if err != nil {
		return math.Inf(1), err
	}

	fastpower := 0.0
	err = db.QueryRow(q1, "fast_reactor", simid).Scan(&fastpower)
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
// and fast reactor prototypes respectively.
func ObjSlowVsFastPowerFueled(scen *Scenario, dbfile string, simid []byte) (float64, error) {
	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	// add up overnight and operating costs converted to PV(t=0)
	q1 := `
    	SELECT TOTAL(Value) FROM timeseriespower AS p
           JOIN agents AS a ON a.agentid=p.agentid AND a.simid=p.simid
           WHERE a.Prototype=? AND p.simid=?
		`

	slowpower := 0.0
	err = db.QueryRow(q1, "slow_reactor", simid).Scan(&slowpower)
	if err != nil {
		return math.Inf(1), err
	}

	fastpower := 0.0
	err = db.QueryRow(q1, "fast_reactor", simid).Scan(&fastpower)
	if err != nil {
		return math.Inf(1), err
	}

	q2 := `
    	SELECT TOTAL(%v) FROM timeseriespower AS p
           JOIN agents AS a ON a.agentid=p.agentid AND a.simid=p.simid
           WHERE a.Prototype=? AND p.simid=?
		`
	fac, err := scen.Prototype("slow_reactor")
	if err != nil {
		return math.Inf(1), err
	}
	slowcap := 0.0
	err = db.QueryRow(fmt.Sprintf(q2, fac.Cap), "slow_reactor", simid).Scan(&slowcap)
	if err != nil {
		return math.Inf(1), err
	}

	fac, err = scen.Prototype("fast_reactor")
	if err != nil {
		return math.Inf(1), err
	}
	fastcap := 0.0
	err = db.QueryRow(fmt.Sprintf(q2, fac.Cap), "fast_reactor", simid).Scan(&fastcap)
	if err != nil {
		return math.Inf(1), err
	}

	return (slowpower + slowcap + fastcap) / (slowpower + fastpower), nil
}
