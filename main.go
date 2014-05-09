package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

var addr = flag.String("addr", "127.0.0.1:4242", "network address of dispatch server")

func main() {
	flag.Parse()

	cmd := flag.Arg(0)
	switch cmd {
	case "server":
		s := NewServer()
		err := s.ListenAndServe(*addr)
		fatalif(err)
	case "worker":
		if !strings.HasPrefix(*addr, "http://") {
			*addr = "http://" + *addr
		}
		w := &Worker{ServerAddr: *addr}
		w.Run()
	case "submit":
		var err error
		var data []byte
		if len(flag.Args()) > 0 {
			data, err = ioutil.ReadFile(flag.Arg(1))
			fatalif(err)
		} else {
			data, err = ioutil.ReadAll(os.Stdin)
			fatalif(err)
		}

		if !strings.HasPrefix(*addr, "http://") {
			*addr = "http://" + *addr
		}
		resp, err := http.Post(*addr+"/job/submit", "application/json", bytes.NewBuffer(data))
		fatalif(err)
		data, err = ioutil.ReadAll(resp.Body)
		fatalif(err)
		fmt.Printf("job submitted (id=%s)\n", data)
	case "retrieve":
		if !strings.HasPrefix(*addr, "http://") {
			*addr = "http://" + *addr
		}
		resp, err := http.Get(*addr + "/job/retrieve/" + flag.Arg(1))
		fatalif(err)
		data, err := ioutil.ReadAll(resp.Body)
		fatalif(err)
		fmt.Println(string(data))
	case "status":
		if !strings.HasPrefix(*addr, "http://") {
			*addr = "http://" + *addr
		}
		resp, err := http.Get(*addr + "/job/status/" + flag.Arg(1))
		fatalif(err)
		data, err := ioutil.ReadAll(resp.Body)
		fatalif(err)
		fmt.Println(string(data))
	case "pack":
		d, err := os.Open(".")
		fatalif(err)
		defer d.Close()

		files, err := d.Readdir(-1)
		fatalif(err)
		j := NewJob()
		for _, info := range files {
			if info.IsDir() {
				continue
			}
			data, err := ioutil.ReadFile(info.Name())
			fatalif(err)
			if info.Name() == "cmds.txt" {
				err := json.Unmarshal(data, &j.Cmds)
				fatalif(err)
			} else if info.Name() == "want.txt" {
				wanted := []string{}
				err := json.Unmarshal(data, &wanted)
				fatalif(err)
				for _, w := range wanted {
					j.Results[w] = []byte{}
				}
			} else {
				j.Resources[info.Name()] = data
			}
		}
		data, err := json.Marshal(j)
		fatalif(err)
		fmt.Printf("%s\n", data)

	case "unpack":
		fname := flag.Arg(1)
		data, err := ioutil.ReadFile(fname)
		j := NewJob()
		err = json.Unmarshal(data, &j)
		fatalif(err)

		for fname, data := range j.Results {
			err := ioutil.WriteFile(fname, data, 0644)
			fatalif(err)
		}
	default:
		log.Printf("Invalid command '%v'", cmd)
	}
}

func fatalif(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
