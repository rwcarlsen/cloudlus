package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
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
	obj      = flag.Bool("obj", false, "true to run job and calculate objective (i.e. workers use this flag)")
	gen      = flag.Bool("gen", false, "true to just print out job file without submitting")
)

const tmpDir = "cyctmp"

func init() {
	log.SetFlags(0)
	flag.Usage = func() {
		log.Printf("Usage: cycdriver [opts] [param1 param2 ... paramN]\n")
		log.Println("generates and submits a cyclus job with the given")
		log.Println("parameters applied to the specified scenario file")
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

	if len(params) == scen.Nvars() {
		scen.InitParams(params)
	} else if len(params) != 0 {
		log.Fatalf("expected %v vars, got %v as args", scen.Nvars(), len(params))
	}

	// perform action
	if *gen {
		j := buildjob(scen)
		data, err := json.Marshal(j)
		check(err)
		fmt.Printf("%s\n", data)
	} else if !*obj {
		j := buildjob(scen)
		submitjob(scen, j)
	} else {
		runjob(scen)
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

func submitjob(scen *scen.Scenario, j *cloudlus.Job) {
	if *addr == "" {
		runjob(scen)
		return
	}

	client, err := cloudlus.Dial(*addr)
	check(err)
	defer client.Close()

	j, err = client.Run(j)
	check(err)
	for _, f := range j.Outfiles {
		if f.Name == *out {
			fmt.Printf("%s\n", f.Data)
			break
		}
	}
}

func runjob(scen *scen.Scenario) {
	dbfile, simid, err := scen.Run(nil, nil)
	val, err := scen.CalcObjective(dbfile, simid)
	check(err)

	err = ioutil.WriteFile(*out, []byte(fmt.Sprint(val)), 0644)
	check(err)
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
