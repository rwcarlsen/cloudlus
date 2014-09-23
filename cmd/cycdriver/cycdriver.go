package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"strconv"

	_ "github.com/mxk/go-sqlite/sqlite3"
	"github.com/rwcarlsen/cloudlus/cloudlus"
	"github.com/rwcarlsen/cloudlus/scen"
	"github.com/rwcarlsen/cyan/nuc"
	"github.com/rwcarlsen/cyan/query"
)

var scenfile = flag.String("scen", "scenario.json", "file containing problem scenification")
var addr = flag.String("addr", "127.0.0.1:9875", "address to submit jobs to (otherwise, run locally)")
var out = flag.String("out", "out.txt", "name of output file")
var obj = flag.Bool("obj", false, "true to calculate objective instead of submit job")

const tmpDir = "cyctmp"

func main() {
	var err error
	flag.Parse()
	log.SetFlags(log.Lshortfile)

	params := make([]int, flag.NArg())
	for i, s := range flag.Args() {
		params[i], err = strconv.Atoi(s)
		fatalif(err)
	}

	// load problem scen file
	scen := &scen.Scenario{}
	err = scen.Load(*scenfile)
	fatalif(err)

	n := len(params)
	if n != 0 && n != scen.Nvars() {
		log.Fatalf("expected %v vars, got %v as args", scen.Nvars(), n)
	}

	if n > 0 {
		scen.InitParams(params)
	}

	if !*obj {
		submitjob()
	} else {
		runjob()
	}

}

func submitjob() {
	scendata, err := json.Marshal(scen)
	fatalif(err)

	tmpldata, err := ioutil.ReadFile(scen.CyclusTmpl)
	fatalif(err)

	j := cloudlus.NewJobCmd("cycdriver", "-obj", "-out", *out, "-scen", *scenfile)
	j.AddInfile(scen.CyclusTmpl, tmpldata)
	j.AddInfile(*scenfile, scendata)
	j.AddOutfile(*out)

	client, err := cloudlus.Dial(*addr)
	fatalif(err)
	defer client.Close()

	j, err = client.Run(j)
	fatalif(err)
	for _, f := range j.Outfiles {
		if f.Name == *out {
			fmt.Printf("%s\n", f.Data)
			break
		}
	}
}

func runjob() {
	dbfile, simid, err := scen.Run()
	val, err := CalcObjective(dbfile, simid, scen)
	fatalif(err)

	err = ioutil.WriteFile(*out, []byte(fmt.Sprint(val)), 0755)
	fatalif(err)
}

func CalcObjective(dbfile string, simid []byte, scen *scen.Scenario) (float64, error) {
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

func fatalif(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
