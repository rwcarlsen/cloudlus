package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strconv"

	_ "github.com/mxk/go-sqlite/sqlite3"
	"github.com/rwcarlsen/cloudlus/objective"
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
		for _, b := range scen.Builds {
			fmt.Printf("%v t%v %v\n", b.Proto, b.Time, b.N)
		}
	} else if *db != "" {
		dbh, err := sql.Open("sqlite3", *db)
		check(err)
		defer dbh.Close()
		simids, err := post.Process(dbh)
		val, err := objective.Calc(scen, *db, simids[0])
		check(err)
		fmt.Println(val)
	} else {
		dbfile, simid, err := scen.Run(nil, nil)
		val, err := objective.Calc(scen, dbfile, simid)
		check(err)
		fmt.Println(val)
	}
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
