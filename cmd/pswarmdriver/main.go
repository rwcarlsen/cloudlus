package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"strconv"
	"strings"

	_ "github.com/mxk/go-sqlite/sqlite3"
	"github.com/rwcarlsen/cloudlus/cloudlus"
	"github.com/rwcarlsen/cloudlus/scen"
)

var (
	scenfile = flag.String("scen", "scenario.json", "file containing problem scenification")
	addr     = flag.String("addr", "", "address to submit jobs to (otherwise, run locally)")
	out      = flag.String("out", "out.txt", "name of output file for the remote job")
	maxeval  = flag.Int("maxeval", 50000, "max number of objective evaluations")
	maxiter  = flag.Int("maxiter", 5000, "max number of optimizer iterations")
)

func init() {
	log.SetFlags(0)
	flag.Usage = func() {
		log.Printf("Usage: pswarmdriver [opts]\n")
		log.Println("Uses a PSwarm-like solver to find optimum solutions for the scenario.")
		flag.PrintDefaults()
	}
}

func main() {
	var err error
	flag.Parse()

	params := make([]int, flag.NArg())
	for i, s := range flag.Args() {
		params[i], err = strconv.Atoi(s)
		check(err)
	}

	// load problem scen file
	scen := &scen.Scenario{}
	err = scen.Load(*scenfile)
	check(err)

	if n := len(params); n != scen.Nvars() && n != 0 {
		log.Fatalf("expected %v vars, got %v as args", scen.Nvars(), n)
	} else {
		scen.InitParams(params)
	}

	// add pswarm initialization and iteration code here
}

func runjob(scen *scen.Scenario) (obj float64, err error) {
	if *addr == "" {
		dbfile, simid, err := scen.Run()
		return scen.CalcObjective(dbfile, simid)
	} else {
		j := buildjob(scen)
		return submitjob(scen, j)
	}
}

func buildjob(scen *scen.Scenario) *cloudlus.Job {
	scendata, err := json.Marshal(scen)
	check(err)

	tmpldata, err := ioutil.ReadFile(scen.CyclusTmpl)
	check(err)

	j := cloudlus.NewJobCmd("cycdriver", "-obj", "-out", *out, "-scen", *scenfile)
	j.AddInfile(scen.CyclusTmpl, tmpldata)
	j.AddInfile(*scenfile, scendata)
	j.AddOutfile(*out)

	if flag.NArg() > 0 {
		j.Note = strings.Join(flag.Args(), " ")
	}

	return j
}

func submitjob(scen *scen.Scenario, j *cloudlus.Job) (float64, error) {
	client, err := cloudlus.Dial(*addr)
	if err != nil {
		return math.Inf(1), err
	}
	defer client.Close()

	j, err = client.Run(j)
	if err != nil {
		return math.Inf(1), err
	}

	for _, f := range j.Outfiles {
		if f.Name == *out {
			s := fmt.Sprintf("%s", f.Data)
			val, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return math.Inf(1), fmt.Errorf("job returned invalid objective string '%v'", s)
			} else {
				return val, nil
			}
		}
	}

	return math.Inf(1), errors.New("job didn't return proper output file")
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
