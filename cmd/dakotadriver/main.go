package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/rwcarlsen/cloudlus/scen"
)

var (
	genInfile = flag.String("gen-infile", "", "generate the dakota input file using the named template")
	scenfile  = flag.String("scen", "scenario.json", "name of optimization scenario file")
	addr      = flag.String("addr", "", "address to submit jobs to (otherwise, run locally)")
	npop      = flag.Int("npop", 0, "population size  (0 => choose automatically)")
	maxeval   = flag.Int("maxeval", 50000, "max number of objective evaluations")
	maxiter   = flag.Int("maxiter", 500, "max number of optimizer iterations")
	parallel  = flag.Int("parallel", 8, "max number of concurrent evaluations")
)

func main() {
	log.SetFlags(0)
	flag.Parse()

	if *genInfile != "" {
		genDakotaFile(*genInfile, *addr)
		return
	}

	paramsfile := flag.Arg(0)
	objfile := flag.Arg(1)

	f, err := os.Create(objfile)
	check(err)
	defer f.Close()

	params, err := ParseParams(paramsfile)
	check(err)

	args := []string{"-scen", *scenfile, "-addr", *addr, "-obj", "-out", objfile}
	args = append(args, params...)
	cmd := exec.Command("cycdriver", args...)

	cmd.Stderr = os.Stderr
	cmd.Stdout = f

	err = cmd.Run()
	check(err)
}

func ParseParams(fname string) ([]string, error) {
	data, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}

	vals := []string{}
	lines := strings.Split(string(data), "\n")
	for i, l := range lines {
		l = strings.TrimSpace(l)
		lines[i] = l
		fields := strings.Split(l, " ")
		for j, field := range fields {
			field = strings.TrimSpace(field)
			fields[j] = field
		}

		if len(fields) < 2 {
			continue
		}

		if strings.HasPrefix(fields[1], "t") && strings.Contains(fields[1], "_f") {
			vals = append(vals, fields[0])
		}
	}
	return vals, nil
}

type Config struct {
	*scen.Scenario
	MaxIter    int
	MaxEval    int
	PopSize    int
	MaxConcurr int
}

func genDakotaFile(tmplName string, addr string) {
	scen := &scen.Scenario{}
	err := scen.Load(*scenfile)
	check(err)
	scen.Addr = addr

	tmpl, err := template.ParseFiles(tmplName)
	check(err)

	n := 100 + 1*len(scen.LowerBounds())
	if *npop != 0 {
		n = *npop
	} else if n < 100 {
		n = 100
	}

	config := &Config{
		Scenario:   scen,
		MaxIter:    *maxiter,
		MaxEval:    *maxeval,
		PopSize:    n,
		MaxConcurr: *parallel,
	}

	err = tmpl.Execute(os.Stdout, config)
	check(err)
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
