package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/rwcarlsen/cloudlus/scen"
)

var (
	genInfile = flag.String("gen-infile", "", "generate the dakota input file using the named template")
	scenfile  = flag.String("scen", "scenario.json", "name of optimization scenario file")
	addr      = flag.String("addr", "", "address to submit jobs to (otherwise, run locally)")
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

	args := append([]string{"-scen", *scenfile}, params...)
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
		fmt.Println(l)
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

		if strings.HasPrefix(fields[1], "b_f") {
			vals = append(vals, fields[0])
		}
	}
	return vals, nil
}

func genDakotaFile(tmplName string, addr string) {
	genname := filepath.Base(tmplName) + ".gen"

	scen := &scen.Scenario{}
	err := scen.Load(*scenfile)
	check(err)
	scen.Addr = addr

	f, err := os.Create(genname)
	check(err)
	defer f.Close()

	tmpl, err := template.ParseFiles(tmplName)
	check(err)

	err = tmpl.Execute(f, scen)
	check(err)
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
