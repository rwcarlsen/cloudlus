package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/rwcarlsen/cloudlus/cloudlus"
)

var obj = flag.Bool("obj", false, "true to print objective if it exists")
var pf = flag.Bool("fname", false, "true to print filename")

func main() {
	flag.Parse()

	fnames := flag.Args()
	if len(fnames) == 0 {
		r := bufio.NewReader(os.Stdin)

		for {
			line, err := r.ReadString('\n')
			if line != "" {
				fnames = append(fnames, strings.TrimSpace(line))
			}
			if err != nil {
				break
			}
		}
	}

	for _, fname := range fnames {
		data, err := ioutil.ReadFile(fname)
		if err != nil {
			log.Fatal(err)
		}

		var j *cloudlus.Job
		err = json.Unmarshal(data, &j)
		if err != nil {
			log.Fatal(err)
		}

		if *obj {
			for _, f := range j.Outfiles {
				if f.Name == "out.txt" {
					if *pf {
						fmt.Printf("%v %v %s %v\n", j.Id, j.Note, f.Data, fname)
					} else {
						fmt.Printf("%v %v %s\n", j.Id, j.Note, f.Data)
					}
					break
				}
			}
		} else {
			if *pf {
				fmt.Printf("%v %v %v\n", j.Id, j.Note, fname)
			} else {
				fmt.Printf("%v %v\n", j.Id, j.Note)
			}
		}
	}
}
