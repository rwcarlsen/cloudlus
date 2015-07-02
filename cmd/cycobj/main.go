package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/cyan/post"
	_ "github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/go-sqlite/sqlite3"
	"github.com/rwcarlsen/cloudlus/scen"
)

var (
	transform   = flag.Bool("transform", false, "print the deployment schedule form of the passed variables")
	untransform = flag.Bool("untransform", false, "print the variables form of the passed build schedule")
	scenfile    = flag.String("scen", "scenario.json", "file containing problem scenification")
	db          = flag.String("db", "", "database file to calculate objective for")
)

// with no flags specified, compute and run simulation
func main() {
	flag.Parse()
	var err error
	params := make([]float64, flag.NArg())
	for i, s := range flag.Args() {
		params[i], err = strconv.ParseFloat(s, 64)
		check(err)
	}

	scn := &scen.Scenario{}
	err = scn.Load(*scenfile)
	check(err)

	if flag.NArg() > 0 {
		_, err = scn.TransformVars(params)
		check(err)
	}

	if *transform {
		tw := tabwriter.NewWriter(os.Stdout, 4, 4, 1, ' ', 0)
		fmt.Fprint(tw, "Prototype\tBuildTime\tNumber\n")
		for _, b := range scn.Builds {
			fmt.Fprintf(tw, "%v\t%v\t%v\n", b.Proto, b.Time, b.N)
		}
		tw.Flush()
	} else if *untransform {
		data, err := ioutil.ReadAll(os.Stdin)
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
			n, err := strconv.Atoi(fields[2])
			check(err)
			builds = append(builds, scen.Build{Proto: proto, Time: t, N: n})
		}
		scn.Builds = builds
		vars, err := scn.TransformSched()
		check(err)

		for _, val := range vars {
			fmt.Printf("%v\n", val)
		}
	} else if *db != "" {
		dbh, err := sql.Open("sqlite3", *db)
		check(err)
		defer dbh.Close()
		simids, err := post.Process(dbh)
		val, err := scn.CalcObjective(*db, simids[0])
		check(err)
		fmt.Println(val)
	} else {
		dbfile, simid, err := scn.Run(nil, nil)
		val, err := scn.CalcObjective(dbfile, simid)
		check(err)
		fmt.Println(val)
	}
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
