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

	// create and initialize solver
	lb := scen.LowerBounds().Col(nil, 0)
	ub := scen.UpperBounds().Col(nil, 0)
	low, A, up := scen.IneqConstr()
	it := buildIter(low, A, up, lb, ub)

	pobj := &optim.ObjectivePenalty{
		Obj:    &objective{scen},
		A:      A,
		Low:    low,
		Up:     up,
		Weight: 1,
	}

	m := mesh.NewBounded(&mesh.Infinite{StepSize: 4}, lb, ub)

	// solve and print results
	best, niter, neval, err := optim.Solve(it, pobj, m, *maxiter, *maxeval)
	if err != nil {
		log.Print(err)
	}
	fmt.Printf("best: %v\n", best)
	fmt.Printf("%v optimizer iterations\n", niter)
	fmt.Printf("%v objective evaluations\n", neval)
}

func buildIter(low, A, up *mat64.Dense, lb, ub []float64) optim.Iterator {
	minv := make([]float64, len(lb))
	maxv := make([]float64, len(lb))
	for i := range lb {
		minv[i] = (ub[i] - lb[i]) / 20
		maxv[i] = minv[i] * 4
	}

	n := 10 + 7*len(lb)
	if n > *maxiter/1000 {
		n = *maxiter / 1000
	}

	points, _, _ := pop.NewConstr(n, n*1000, lb, ub, low, A, up)
	pop := pswarm.NewPopulation(points, minv, maxv)
	ev := optim.NewCacheEvaler(optim.ParallelEvaler{})
	swarm := pswarm.NewIterator(ev, nil, pop, pswarm.LinInertia(0.9, 0.4, *maxiter))
	return pattern.NewIterator(ev, pop[0].Point, pattern.SearchIter(swarm))
}

type objective struct {
	s *scen.Scenario
}

func (o *objective) Objective(v []float64) (float64, error) {
	if n := len(v); n != o.s.Nvars() {
		panic(fmt.Sprintf("expected %v vars, got %v as args", o.s.Nvars(), n))
	}

	params := make([]int, len(v))
	for i := range v {
		params[i] = int(math.Ceil(v[i] + .5))
	}

	o.s.InitParams(params)
	if *addr == "" {
		dbfile, simid, err := o.s.Run()
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
