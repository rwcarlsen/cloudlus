package scen

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/cyan/nuc"
	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/cyan/query"
)

type ANSFacility struct {
	Proto string
	// Cap is the total Power output capacity of the facility.
	Cap float64
	// OpCost represents the per timstep operating cost for the facility
	OpCost float64
	// CapitalCost represents the overnight cost for building the facility
	CapitalCost float64
	// The lifetime of the facility (in timesteps). The lifetime must also be
	// specified manually (consistent with this value) in the prototype
	// definition in the cyclus input template file.
	Life int
	// BuildAfter is the time step after which this facility type can be
	// built.  -1 for never available, and 0 for always available.
	BuildAfter int
	// WasteDiscount represents the fraction is discounted from the waste cost
	// for this facility.
	WasteDiscount float64
}

type ANSScenario struct {
	// SimDur is the simulation duration in timesteps (months)
	SimDur int
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
	Facs []ANSFacility
}

func (s *ANSScenario) Load(fname string) error {
	if s == nil {
		s = &ANSScenario{}
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
	return nil
}

func ObjANS2014(scen *Scenario, db *sql.DB, simid []byte) (float64, error) {
	s := &ANSScenario{}
	err := s.Load(scen.File)
	if err != nil {
		return math.Inf(1), err
	}

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
	for _, fac := range s.Facs {
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
			totcost += PV(fac.OpCost, t, s.Discount)
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
			totcost += PV(fac.CapitalCost, t, s.Discount)
		}
		if err := rows.Err(); err != nil {
			return 0, err
		}

		// add in waste penalty
		ags, err := query.AllAgents(db, simid, fac.Proto)
		if err != nil {
			return 0, err
		}

		// InvAt uses all agents if no ids are passed - so we need to skip
		// from here
		if len(ags) == 0 {
			continue
		}

		ids := make([]int, len(ags))
		for i, a := range ags {
			ids[i] = a.Id
		}

		for t := 0; t < s.SimDur; t++ {
			mat, err := query.InvAt(db, simid, t, ids...)
			if err != nil {
				return 0, err
			}
			for nuc, qty := range mat {
				nucstr := fmt.Sprint(nuc)
				totcost += PV(s.NuclideCost[nucstr]*float64(qty)*(1-fac.WasteDiscount), t, s.Discount)
			}
		}
	}

	// normalize to energy produced
	joules, err := query.EnergyProduced(db, simid, 0, s.SimDur)
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
