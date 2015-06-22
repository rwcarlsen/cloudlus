package scen

import (
	"database/sql"
	"math"
)

type ObjFunc func(scen *Scenario, dbfile string, simid []byte) (float64, error)

var ObjFuncs = map[string]ObjFunc{
	"":          ObjSlowVsFastPower,
	"slowvfast": ObjSlowVsFastPower,
	"ans2014":   ObjANS2014,
}

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
           WHERE a.Prototype=?
		`

	slowpower := 0.0
	err = db.QueryRow(q1, "slow_reactor").Scan(&slowpower)
	if err != nil {
		return math.Inf(1), err
	}

	fastpower := 0.0
	err = db.QueryRow(q1, "fast_reactor").Scan(&fastpower)
	if err != nil {
		return math.Inf(1), err
	}

	return slowpower / (slowpower + fastpower), nil
}
