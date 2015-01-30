package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/gonum/matrix/mat64"
	"github.com/rwcarlsen/cloudlus/cloudlus"
	"github.com/rwcarlsen/cloudlus/scen"
	"github.com/rwcarlsen/optim"
	"github.com/rwcarlsen/optim/mesh"
	"github.com/rwcarlsen/optim/pattern"
	"github.com/rwcarlsen/optim/pop"
	"github.com/rwcarlsen/optim/pswarm"
)

var (
	scenfile  = flag.String("scen", "scenario.json", "file containing problem scenification")
	objlog    = flag.String("objlog", "obj.log", "file to log function evaluations")
	penobjlog = flag.String("penobjlog", "penalized-obj.log", "file to log penalized function evaluations")
	bestlog   = flag.String("bestlog", "best.log", "file to log function evaluations to")
	runlog    = flag.String("runlog", "run.log", "file to log local cyclus run output")
	addr      = flag.String("addr", "", "address to submit jobs to (otherwise, run locally)")
	maxeval   = flag.Int("maxeval", 10000, "max number of objective evaluations")
	maxiter   = flag.Int("maxiter", 300, "max number of optimizer iterations")
	penalty   = flag.Float64("penalty", 0.5, "fractional penalty for constraint violations")
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

	f1, err := os.Create(*objlog)
	check(err)
	defer f1.Close()
	f2, err := os.Create(*penobjlog)
	check(err)
	defer f2.Close()
	f3, err := os.Create(*bestlog)
	check(err)
	defer f3.Close()
	f4, err := os.Create(*runlog)
	check(err)
	defer f4.Close()

	// create and initialize solver
	lb := scen.LowerBounds().Col(nil, 0)
	ub := scen.UpperBounds().Col(nil, 0)
	low, A, up := scen.IneqConstr()
	it, ev := buildIter(low, A, up, lb, ub)

	loggedobj := &optim.ObjectiveLogger{Obj: &objective{scen, f4}, W: f1}
	pobj := &optim.ObjectivePenalty{
		Obj:    loggedobj,
		A:      A,
		Low:    low,
		Up:     up,
		Weight: 1,
	}
	loggedpobj := &optim.ObjectiveLogger{Obj: pobj, W: f2}

	m := mesh.Integer{mesh.NewBounded(&mesh.Infinite{StepSize: 2}, lb, ub)}

	// these are defined here so that signals goroutine can close over them
	best := optim.Point{}
	neval, niter := 0, 0

	// handle signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		f1.Close()
		f2.Close()
		f3.Close()
		f4.Close()
		fmt.Println("\n*** optimizer killed early ***")
		fmt.Printf("best: %v\n", best)
		fmt.Printf("%v optimizer iterations\n", niter)
		fmt.Printf("%v objective evaluations\n", neval)
		fmt.Printf("%v cached objective uses\n", ev.UseCount)
		os.Exit(1)
	}()

	// solve and print results
	fmt.Fprintf(f3, "Iteration #: f[bestpos] = bestobj\n")
	for neval < *maxeval && niter < *maxiter {
		var n int
		best, n, err = it.Iterate(loggedpobj, m)
		neval += n
		niter++
		if err != nil {
			log.Print(err)
		}
		fmt.Fprintf(f3, "%v:  %v\n", niter, best)
		fmt.Printf("iteration %v (%v evals) best point:  %v\n", niter, n, best)
	}
	fmt.Printf("best: %v\n", best)
	fmt.Printf("%v optimizer iterations\n", niter)
	fmt.Printf("%v objective evaluations\n", neval)
	fmt.Printf("%v cached objective uses\n", ev.UseCount)
}

func buildIter(low, A, up *mat64.Dense, lb, ub []float64) (optim.Iterator, *optim.CacheEvaler) {
	minv := make([]float64, len(lb))
	maxv := make([]float64, len(lb))
	maxmaxv := 0.0
	for i := range lb {
		minv[i] = (ub[i] - lb[i]) / 20
		maxv[i] = minv[i] * 4
		maxmaxv += maxv[i] * maxv[i]
	}
	maxmaxv = math.Sqrt(maxmaxv)

	n := 10 + 5*len(lb)
	if n < 20 {
		n = 20
	}

	points, nbad, _ := pop.NewConstr(n, 1000000, lb, ub, low, A, up)

	fmt.Printf("swarming with %v particles\n", n)
	fmt.Printf("initial population includes %v infeasible solutions/particles", nbad)

	pop := pswarm.NewPopulation(points, minv, maxv)
	ev := optim.NewCacheEvaler(optim.ParallelEvaler{})
	swarm := pswarm.NewIterator(ev, nil, pop,
		pswarm.LinInertia(0.9, 0.4, *maxiter),
		pswarm.Vmax(maxmaxv),
	)
	return pattern.NewIterator(ev, pop[0].Point,
		pattern.SearchIter(swarm),
		pattern.NfailGrow(-1), // never grow mesh
	), ev
}

type objective struct {
	s      *scen.Scenario
	runlog io.Writer
}

func (o *objective) Objective(v []float64) (float64, error) {
	if n := len(v); n != o.s.Nvars() {
		panic(fmt.Sprintf("expected %v vars, got %v as args", o.s.Nvars(), n))
	}

	params := make([]int, len(v))
	for i := range v {
		params[i] = int(v[i])
	}

	o.s.InitParams(params)
	if *addr == "" {
		dbfile, simid, err := o.s.Run(o.runlog, o.runlog)
		if err != nil {
			return math.Inf(1), err
		}
		return o.s.CalcObjective(dbfile, simid)
	} else {
		j := buildjob(o.s)
		return submitjob(o.s, j)
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
		if f.Name == outfile {
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
