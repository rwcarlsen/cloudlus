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
var sweep = flag.Bool("gen", false, "true to just generate sweep parameters")

func main() {
	flag.Parse()

	if *sweep {
		for _, p := range buildSweep() {
			fmt.Printf("%v\n", p)
		}
		return
	}

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

func buildSweep() [][]int {
	iperms := Permute(skipsums, 11, 11, 11, 11, 1, 2, 2, 2, 2, 1, 1, 2, 2, 2, 1, 1, 2, 2, 2, 1)
	perms := [][]int{}
	for _, p := range iperms {
		perms = append(perms, p)
		p[4] = 10 - Sum(p, 0, 4)
	}
	return perms
}

func skipsums(p []int) bool {
	if Sum(p, 0, 5) > 10 {
		return true
	} else if Sum(p, 5, 10) > 1 {
		return true
	} else if Sum(p, 10, 15) > 1 {
		return true
	} else if Sum(p, 15, 20) > 1 {
		return true
	}
	return false
}

func Sum(s []int, lower, upper int) int {
	if upper > len(s) {
		upper = len(s)
	}
	if lower > len(s) {
		lower = len(s)
	}

	tot := 0
	for _, v := range s[lower:upper] {
		tot += v
	}
	return tot
}

func Permute(skip func([]int) bool, dimensions ...int) [][]int {
	return permute(skip, dimensions, []int{})
}

func permute(skip func([]int) bool, dimensions []int, prefix []int) [][]int {
	set := make([][]int, 0)

	if len(dimensions) == 1 {
		for i := 0; i < dimensions[0]; i++ {
			val := append(append([]int{}, prefix...), i)
			set = append(set, val)
		}
		return set
	}

	max := dimensions[0]
	for i := 0; i < max; i++ {
		newprefix := append(prefix, i)
		if skip(newprefix) {
			continue
		}
		set = append(set, permute(skip, dimensions[1:], newprefix)...)
	}
	return set
}
