package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/mxk/go-sqlite/sqlite3"
	"github.com/rwcarlsen/cloudlus/cloudlus"
	"github.com/rwcarlsen/cloudlus/scen"
	"github.com/rwcarlsen/optim"
	"github.com/rwcarlsen/optim/pattern"
	"github.com/rwcarlsen/optim/swarm"
)

var (
	scenfile     = flag.String("scen", "scenario.json", "file containing problem scenification")
	npar         = flag.Int("npar", 0, "number of particles (0 => choose automatically)")
	addr         = flag.String("addr", "", "address to submit jobs to (otherwise, run locally)")
	seed         = flag.Int("seed", 1, "seed for random number generator")
	objlog       = flag.String("objlog", "obj.log", "file to log unpenalized objective values")
	runlog       = flag.String("runlog", "run.log", "file to log local cyclus run output")
	maxeval      = flag.Int("maxeval", 50000, "max number of objective evaluations")
	maxiter      = flag.Int("maxiter", 500, "max number of optimizer iterations")
	maxnoimprove = flag.Int("maxnoimprove", 100, "max iterations with no objective improvement(zero -> infinite)")
	dbname       = flag.String("db", "pswarm.sqlite", "name for database containing optimizer work")
)

const outfile = "objective.out"

func init() {
	log.SetFlags(0)
	flag.Usage = func() {
		log.Printf("Usage: pswarmdriver [opts]\n")
		log.Println("Uses a PSwarm-like solver to find optimum solutions for the scenario.")
		flag.PrintDefaults()
	}
}

var db *sql.DB
var client *cloudlus.Client

func main() {
	var err error
	flag.Parse()
	optim.Rand = rand.New(rand.NewSource(int64(*seed)))

	if *addr != "" {
		client, err = cloudlus.Dial(*addr)
		check(err)
		defer client.Close()
	}

	os.Remove(*dbname)
	db, err = sql.Open("sqlite3", *dbname)
	check(err)
	defer db.Close()

	params := make([]int, flag.NArg())
	for i, s := range flag.Args() {
		params[i], err = strconv.Atoi(s)
		check(err)
	}

	// load problem scen file
	scen := &scen.Scenario{}
	err = scen.Load(*scenfile)
	check(err)

	f1, err := os.Create(*objlog)
	check(err)
	defer f1.Close()
	f4, err := os.Create(*runlog)
	check(err)
	defer f4.Close()

	// create and initialize solver
	lb := scen.LowerBounds()
	ub := scen.UpperBounds()
	it, ev := buildIter(lb, ub)

	obj := &optim.ObjectiveLogger{Obj: &objective{scen, f4}, W: f1}

	m := &optim.BoxMesh{Mesh: &optim.InfMesh{StepSize: 0.2}, Lower: lb, Upper: ub}

	// this is here so that signals goroutine can close over it
	solv := &optim.Solver{
		Method:       it,
		Obj:          obj,
		Mesh:         m,
		MaxIter:      *maxiter,
		MaxEval:      *maxeval,
		MaxNoImprove: *maxnoimprove,
	}

	// handle signals
	start := time.Now()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		f1.Close()
		f4.Close()
		fmt.Println("\n*** optimizer killed early ***")
		final(solv, ev.UseCount, start)
		os.Exit(1)
	}()

	// solve and print results
	for solv.Next() {
		fmt.Printf("Iter %v (%v evals):  %v\n", solv.Niter(), solv.Neval(), solv.Best())
	}

	final(solv, ev.UseCount, start)
}

func final(s *optim.Solver, cache int, start time.Time) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS optiminfo (start INTEGER,end INTEGER,niter INTEGER,neval INTEGER,ncacheuses INTEGER);")
	check(err)
	_, err = db.Exec("INSERT INTO optiminfo VALUES (?,?,?,?,?);", start, time.Now(), s.Niter(), s.Neval(), cache)
	check(err)

	if err := s.Err(); err != nil {
		log.Print(err)
	}

	fmt.Printf("best: %v\n", s.Best())
	fmt.Printf("%v optimizer iterations\n", s.Niter())
	fmt.Printf("%v objective evaluations\n", s.Neval())
	fmt.Printf("%v cached objective uses\n", cache)
}

func buildIter(lb, ub []float64) (optim.Method, *optim.CacheEvaler) {
	vmax := make([]float64, len(lb))
	for i := range lb {
		vmax[i] = (ub[i] - lb[i])
	}

	n := 30 + 1*len(lb)
	if *npar != 0 {
		n = *npar
	} else if n < 30 {
		n = 30
	}

	points := optim.RandPop(n, lb, ub)
	fmt.Printf("swarming with %v particles\n", n)

	ev := optim.NewCacheEvaler(optim.ParallelEvaler{})
	swarm := swarm.New(
		swarm.NewPopulation(points, vmax),
		swarm.Evaler(ev),
		swarm.VmaxBounds(lb, ub),
		swarm.DB(db),
	)
	return pattern.New(points[0],
		pattern.Evaler(ev),
		pattern.SearchMethod(swarm, pattern.Share),
		pattern.DB(db),
	), ev
}

type objective struct {
	s      *scen.Scenario
	runlog io.Writer
}

func (o *objective) Objective(v []float64) (float64, error) {

	scencopyval := *o.s
	scencopy := &scencopyval
	scencopy.TransformVars(v)
	if *addr == "" {
		dbfile, simid, err := scencopy.Run(o.runlog, o.runlog)
		if err != nil {
			return math.Inf(1), err
		}
		return scencopy.CalcObjective(dbfile, simid)
	} else {
		j := buildjob(scencopy)
		return submitjob(scencopy, j)
	}
}

func buildjob(scen *scen.Scenario) *cloudlus.Job {
	scendata, err := json.Marshal(scen)
	check(err)

	tmpldata, err := ioutil.ReadFile(scen.CyclusTmpl)
	check(err)

	j := cloudlus.NewJobCmd("cycdriver", "-obj", "-out", outfile, "-scen", *scenfile)
	j.AddInfile(scen.CyclusTmpl, tmpldata)
	j.AddInfile(*scenfile, scendata)
	j.AddOutfile(outfile)

	if flag.NArg() > 0 {
		j.Note = strings.Join(flag.Args(), " ")
	}

	return j
}

func submitjob(scen *scen.Scenario, j *cloudlus.Job) (float64, error) {
	var err error
	j, err = client.Run(j)
	if err != nil {
		return math.Inf(1), err
	}

	for _, f := range j.Outfiles {
		if f.Name == outfile {
			s := fmt.Sprintf("%s", f.Data)
			val, err := strconv.ParseFloat(s, 64)
			if err != nil {
				log.Printf("job returned invalid objective string '%v'", s)
				return math.Inf(1), nil
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
