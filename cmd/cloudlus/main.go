package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/rpc"
	"os"
	"strings"
	"time"

	"github.com/rwcarlsen/cloudlus"
)

var addr = flag.String("addr", "127.0.0.1:4242", "network address of dispatch server")

type CmdFunc func(cmd string, args []string)

var cmds = map[string]CmdFunc{
	"serve":         serve,
	"work":          work,
	"submit":        submit,
	"submitrpc":     submitrpc,
	"submit-infile": submitInfile,
	"retrieve":      retrieve,
	"status":        stat,
	"pack":          pack,
}

func newFlagSet(cmd, args, desc string) *flag.FlagSet {
	fs := flag.NewFlagSet("put", flag.ExitOnError)
	fs.Usage = func() {
		log.Printf("Usage: cloudlus %s [OPTION] %s\n%s\n", cmd, args, desc)
		fs.PrintDefaults()
	}
	return fs
}

func main() {
	log.SetFlags(0)
	flag.Usage = func() {
		log.Printf("Usage: cloudlus [OPTION] <subcommand> [OPTION] [args]\n")
		flag.PrintDefaults()
		log.Printf("Subcommands:\n")
		for cmd, _ := range cmds {
			log.Printf("  %v", cmd)
		}
	}
	flag.Parse()

	if len(flag.Args()) == 0 {
		flag.Usage()
		return
	}

	cmd, ok := cmds[flag.Arg(0)]
	if !ok {
		flag.Usage()
		return
	}
	cmd(flag.Arg(0), flag.Args()[1:])
}

func serve(cmd string, args []string) {
	fs := newFlagSet(cmd, "", "run a work dispatch server listening for jobs and workers")
	host := fs.String("host", "", "server host base url")
	fs.Parse(args)
	s := cloudlus.NewServer(*addr)
	s.Host = fulladdr(*host)
	err := s.Run()
	fatalif(err)
}

func work(cmd string, args []string) {
	fs := newFlagSet(cmd, "", "run a worker polling for jobs and workers")
	wait := fs.Duration("interval", 10*time.Second, "time interval between work polls when idle")
	fs.Parse(args)
	w := &cloudlus.Worker{ServerAddr: fulladdr(*addr), Wait: *wait}
	w.Run()
}

func submit(cmd string, args []string) {
	fs := newFlagSet(cmd, "[FILE]", "submit a job file (may be piped to stdin)")
	fs.Parse(args)

	data := stdinOrFile(fs)
	resp, err := http.Post(fulladdr(*addr)+"/job/submit", "application/json", bytes.NewBuffer(data))
	fatalif(err)
	data, err = ioutil.ReadAll(resp.Body)
	fatalif(err)
	fmt.Printf("job submitted successfully:\n%s\n", data)
}

func submitrpc(cmd string, args []string) {
	fs := newFlagSet(cmd, "[FILE]", "submit a job file (may be piped to stdin)")
	fs.Parse(args)

	data := stdinOrFile(fs)
	j := cloudlus.NewJob()
	err := json.Unmarshal(data, &j)
	fatalif(err)

	client, err := rpc.DialHTTP("tcp", *addr)
	fatalif(err)
	result := cloudlus.NewJob()
	err = client.Call("RPC.Submit", j, &result)
	fatalif(err)

	data, err = json.Marshal(result)
	fatalif(err)
	fmt.Println(string(data))
}

func submitInfile(cmd string, args []string) {
	fs := newFlagSet(cmd, "[FILE]", "submit a cyclus input file with default run params (may be piped to stdin)")
	fs.Parse(args)

	data := stdinOrFile(fs)
	resp, err := http.Post(fulladdr(*addr)+"/job/submit-infile", "application/json", bytes.NewBuffer(data))
	fatalif(err)
	data, err = ioutil.ReadAll(resp.Body)
	fatalif(err)
	fmt.Printf("job submitted successfully:\n%s\n", data)
}

func retrieve(cmd string, args []string) {
	fs := newFlagSet(cmd, "[JOB-ID]", "retrieve the result tar file for the given job id")
	fname := fs.String("o", "", "send result tar to file instead of stdout")
	fs.Parse(args)

	if len(fs.Args()) == 0 {
		log.Fatal("no job id specified")
	}

	resp, err := http.Get(fulladdr(*addr) + "/job/retrieve/" + fs.Arg(0))
	fatalif(err)
	data, err := ioutil.ReadAll(resp.Body)
	fatalif(err)

	if *fname == "" {
		fmt.Println(string(data))
	} else {
		err := ioutil.WriteFile(*fname, data, 0644)
		fatalif(err)
	}
}

func stat(cmd string, args []string) {
	fs := newFlagSet(cmd, "[JOB-ID]", "get the status of the given job id")
	fs.Parse(args)

	if len(fs.Args()) == 0 {
		log.Fatal("no job id specified")
	}

	resp, err := http.Get(fulladdr(*addr) + "/job/status/" + fs.Arg(0))
	fatalif(err)
	data, err := ioutil.ReadAll(resp.Body)
	fatalif(err)

	j := cloudlus.NewJob()
	if err := json.Unmarshal(data, &j); err != nil {
		log.Fatalf("server has no job of id %v", fs.Arg(0))
	}

	fmt.Printf("Job %x status: %v\n", j.Id, j.Status)
}

func pack(cmd string, args []string) {
	fs := newFlagSet(cmd, "", "pack all files in the working directory into a job submit file")
	fname := fs.String("o", "", "send pack data to file instead of stdout")
	fs.Parse(args)

	d, err := os.Open(".")
	fatalif(err)
	defer d.Close()

	files, err := d.Readdir(-1)
	fatalif(err)
	j := cloudlus.NewJob()
	for _, info := range files {
		if info.IsDir() {
			continue
		}
		data, err := ioutil.ReadFile(info.Name())
		fatalif(err)
		if info.Name() == "cmd.txt" {
			err := json.Unmarshal(data, &j.Cmd)
			fatalif(err)
		} else if info.Name() == "want.txt" {
			list := []string{}
			err := json.Unmarshal(data, &list)
			fatalif(err)
			for _, name := range list {
				j.AddOutfile(name)
			}
		} else {
			j.AddInfile(info.Name(), data)
		}
	}
	data, err := json.Marshal(j)
	fatalif(err)

	if *fname == "" {
		fmt.Printf("%s\n", data)
	} else {
		err := ioutil.WriteFile(*fname, data, 0644)
		fatalif(err)
	}
}

func fatalif(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func fulladdr(addr string) string {
	if !strings.HasPrefix(addr, "http://") && addr != "" {
		return "http://" + addr
	}
	return addr
}

func stdinOrFile(fs *flag.FlagSet) []byte {
	if len(fs.Args()) > 0 {
		data, err := ioutil.ReadFile(fs.Arg(0))
		fatalif(err)
		return data
	}
	data, err := ioutil.ReadAll(os.Stdin)
	fatalif(err)
	return data
}
