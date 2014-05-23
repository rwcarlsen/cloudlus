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
		w := &Worker{ServerAddr: fulladdr(*addr)}
		w.Run()
	case "submit":
		data := stdinOrFile()
		resp, err := http.Post(fulladdr(*addr)+"/job/submit", "application/json", bytes.NewBuffer(data))
		fatalif(err)
		data, err = ioutil.ReadAll(resp.Body)
		fatalif(err)
		fmt.Printf("job submitted (id=%s)\n", data)
	case "submit-infile":
		data := stdinOrFile()
		resp, err := http.Post(fulladdr(*addr)+"/job/submit-infile", "application/json", bytes.NewBuffer(data))
		fatalif(err)
		data, err = ioutil.ReadAll(resp.Body)
		fatalif(err)
		fmt.Printf("job submitted (id=%s)\n", data)
	case "retrieve":
		resp, err := http.Get(fulladdr(*addr) + "/job/retrieve/" + flag.Arg(1))
		fatalif(err)
		data, err := ioutil.ReadAll(resp.Body)
		fatalif(err)
		fmt.Println(string(data))
	case "status":
		resp, err := http.Get(fulladdr(*addr) + "/job/status/" + flag.Arg(1))
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
				err := json.Unmarshal(data, &j.Results)
				fatalif(err)
			} else {
				j.Resources[info.Name()] = data
			}
		}
		data, err := json.Marshal(j)
		fatalif(err)
		fmt.Printf("%s\n", data)
	default:
		log.Printf("Invalid command '%v'", cmd)
	}
}

func fatalif(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func fulladdr(addr string) string {
	if !strings.HasPrefix(addr, "http://") {
		return "http://" + addr
	}
	return addr
}

func stdinOrFile() []byte {
	if len(flag.Args()) > 0 {
		data, err := ioutil.ReadFile(flag.Arg(1))
		fatalif(err)
		return data
	}
	data, err := ioutil.ReadAll(os.Stdin)
	fatalif(err)
	return data
}
