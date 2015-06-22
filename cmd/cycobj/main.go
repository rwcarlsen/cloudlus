package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"text/tabwriter"

	_ "github.com/mxk/go-sqlite/sqlite3"
	"github.com/rwcarlsen/cloudlus/scen"
	"github.com/rwcarlsen/cyan/post"
)

var (
	transform = flag.Bool("transform", false, "print the deployment schedule form of the passed variables")
	scenfile  = flag.String("scen", "scenario.json", "file containing problem scenification")
	db        = flag.String("db", "", "database file to calculate objective for")
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

	scen := &scen.Scenario{}
	err = scen.Load(*scenfile)
	check(err)

	if flag.NArg() > 0 {
		_, err = scen.TransformVars(params)
		check(err)
	}

	if *transform {
		tw := tabwriter.NewWriter(os.Stdout, 4, 4, 1, ' ', 0)
		fmt.Fprint(tw, "Prototype\tBuildTime\tNumber\n")
		for _, b := range scen.Builds {
			fmt.Fprintf(tw, "%v\t%v\t%v\n", b.Proto, b.Time, b.N)
		}
		tw.Flush()
	} else if *db != "" {
		dbh, err := sql.Open("sqlite3", *db)
		check(err)
		defer dbh.Close()
		simids, err := post.Process(dbh)
		val, err := scen.CalcObjective(*db, simids[0])
		check(err)
		fmt.Println(val)
	} else {
		dbfile, simid, err := scen.Run(nil, nil)
		val, err := scen.CalcObjective(dbfile, simid)
		check(err)
		fmt.Println(val)
	}
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
