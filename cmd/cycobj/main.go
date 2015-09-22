package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/cyan/post"
	_ "github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/go-sqlite/sqlite3"
	"github.com/rwcarlsen/cloudlus/runscen"
	"github.com/rwcarlsen/cloudlus/scen"
)

var (
	transform = flag.Bool("transform", false, "print the deployment schedule form of the passed variables")
	sched     = flag.Bool("sched", false, "parse build schedule from stdin instead of var vals")
	scenfile  = flag.String("scen", "scenario.json", "file containing problem scenification")
	addr      = flag.String("addr", "", "address to submit jobs to (otherwise, run locally)")
	db        = flag.String("db", "", "database file to calculate objective for")
	stats     = flag.Bool("stats", false, "print basic stats about deploy sched")
	gen       = flag.Bool("gen", false, "true to just print out job file without submitting")
	quiet     = flag.Bool("q", false, "don't print job stdout+stderr")
	obj       = flag.String("obj", "", "(internal) if non-empty, run scenario and store objective in `FILE`")
)

var objfile = "cloudlus-cycobj.dat"

// with no flags specified, compute and run simulation
func main() {
	flag.Parse()

	scn := &scen.Scenario{}
	err := scn.Load(*scenfile)
	check(err)

	if len(scn.Builds) == 0 {
		parseSchedVars(scn)
	} else {
		log.Print("because of pre-existing builds, ignoring any deploy variables/schedule")
	}

	if *stats {
		scn.PrintStats()
	} else if *transform && !*sched {
		tw := tabwriter.NewWriter(os.Stdout, 4, 4, 1, ' ', 0)
		fmt.Fprint(tw, "Prototype\tBuildTime\tLifetime\tNumber\n")
		for _, b := range scn.Builds {
			fmt.Fprintf(tw, "%v\t%v\t%v\t%v\n", b.Proto, b.Time, b.Lifetime(), b.N)
		}
		tw.Flush()
	} else if *transform && *sched {
		vars, err := scn.TransformSched()
		check(err)

		for _, val := range vars {
			fmt.Printf("%v\n", val)
		}
	} else if *gen {
		j, err := runscen.BuildRemoteJob(scn, objfile)
		check(err)
		data, err := json.Marshal(j)
		check(err)
		fmt.Printf("%s\n", data)
	} else if *db != "" {
		dbh, err := sql.Open("sqlite3", *db)
		check(err)
		defer dbh.Close()
		simids, err := post.Process(dbh)
		val, err := scn.CalcObjective(*db, simids[0])
		check(err)
		fmt.Println(val)
	} else {
		val := runjob(scn, *addr)
		if *obj != "" {
			err := ioutil.WriteFile(*obj, []byte(fmt.Sprint(val)), 0644)
			check(err)
		} else {
			fmt.Println(val)
		}
	}
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func parseSched(r io.Reader) []scen.Build {
	data, err := ioutil.ReadAll(r)
	check(err)
	s := string(data)
	lines := strings.Split(s, "\n")
	builds := []scen.Build{}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		fields := strings.Fields(l)
		if len(l) == 0 {
			continue
		} else if fields[1] == "BuildTime" {
			continue
		}
		proto := fields[0]
		t, err := strconv.Atoi(fields[1])
		check(err)
		life, err := strconv.Atoi(fields[2])
		check(err)
		n, err := strconv.Atoi(fields[3])
		check(err)
		builds = append(builds, scen.Build{Proto: proto, Time: t, N: n, Life: life})
	}
	return builds
}

func parseVars(r io.Reader) []float64 {
	data, err := ioutil.ReadAll(r)
	check(err)
	s := string(data)
	lines := strings.Split(s, "\n")
	vars := []float64{}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if len(l) == 0 {
			continue
		}
		val, err := strconv.ParseFloat(l, 64)
		check(err)
		vars = append(vars, val)
	}
	return vars
}

func parseSchedVars(scn *scen.Scenario) {
	var err error
	if *sched {
		scn.Builds = parseSched(os.Stdin)
	} else {
		params := []float64{}
		if flag.NArg() > 0 {
			params = make([]float64, flag.NArg())
			for i, s := range flag.Args() {
				params[i], err = strconv.ParseFloat(s, 64)
				check(err)
			}
		} else {
			params = parseVars(os.Stdin)
		}

		_, err = scn.TransformVars(params)
		check(err)
	}
	err = scn.Validate()
	check(err)
}

func runjob(scen *scen.Scenario, addr string) float64 {
	var stdout, stderr io.Writer
	if !*quiet {
		stdout, stderr = os.Stdout, os.Stderr
	}

	if addr == "" {
		val, _, _, err := runscen.Local(scen, stdout, stderr)
		check(err)
		return val
	} else {
		val, err := runscen.Remote(scen, stdout, stderr, addr)
		check(err)
		return val
	}
}
