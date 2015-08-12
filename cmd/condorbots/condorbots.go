package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/golang.org/x/crypto/ssh"
	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/golang.org/x/crypto/ssh/agent"
)

var (
	addr    = flag.String("addr", "", "ip:port of cloudlus server")
	run     = flag.String("run", "", "name of script for condor to run")
	n       = flag.Int("n", 0, "number of bots to deploy")
	ncpu    = flag.Int("ncpu", 1, "minimum number of cpus required per worker job")
	mem     = flag.Int("mem", 512, "minimum `MiB` of memory required per worker job")
	classad = flag.String("classad", "", "literal classad constraints (e.g. 'Mips >= 20000' for faster cpus)")
	keyfile = flag.String("keyfile", filepath.Join(os.Getenv("HOME"), ".ssh/id_rsa"), "path to ssh private key file")
	user    = flag.String("user", "rcarlsen", "condor (and via node) ssh username")
	dst     = flag.String("dst", "submit-3.chtc.wisc.edu:22", "condor submit node URI")
	via     = flag.String("via", "best-tux.cae.wisc.edu:22", "intermediate server URI (if needed)")
	cpy     = flag.Bool("copy", false, "true to automatically copy all needed files to submit node")
	local   = flag.Bool("local", false, "save local copies of generated files")
	wkflags = flag.String("workflags", "", "flags to be passed straight to cloudlus worker invocation")
)

type CondorConfig struct {
	Executable string
	Infiles    string
	N          int
	NCPU       int
	Memory     int
	ClassAds   string
}

const condorname = "condor.submit"

// rank = Kflops is to prefer faster FLOPS machines.
const condorfile = `
universe = vanilla
executable = {{.Executable}}
transfer_input_files = {{.Infiles}}
should_transfer_files = yes
when_to_transfer_output = on_exit
output = worker.$(PROCESS).output
error = worker.$(PROCESS).error
log = workers.log
request_cpus = {{.NCPU}}
request_memory = {{.Memory}}
Rank = Kflops
requirements = OpSys == "LINUX" && Arch == "x86_64" && (OpSysAndVer =?= "SL6") {{.ClassAds}}

queue {{.N}}
`

const runfilename = "CLOUDLUS_runfile.sh"

const runfile = `#!/bin/bash
{{with .Runfile}}bash ./{{.}}{{end}}
chmod a+x ./cloudlus
./cloudlus -addr {{.Addr}} work {{.Flags}}
`

var condortmpl = template.Must(template.New("submitfile").Parse(condorfile))
var runtmpl = template.Must(template.New("runfile").Parse(runfile))

func main() {
	log.SetFlags(0)
	flag.Usage = func() {
		fmt.Println("Usage: condor [FILE...]")
		fmt.Print("Copy listed files to condor submit node and possibly submit a job.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *addr == "" {
		log.Fatal("must specify server address")
	}

	// assemple file names to copy over
	srcfiles := append([]string{}, flag.Args()...)

	path, err := exec.LookPath("cloudlus")
	if err != nil {
		log.Fatal(err)
	}
	srcfiles = append(srcfiles, path)

	if *run != "" {
		srcfiles = append(srcfiles, *run)
	}

	dstfiles := make([]string, len(srcfiles))
	for i := range srcfiles {
		dstfiles[i] = filepath.Base(srcfiles[i])
	}

	// build condor submit file and condor submit executable script
	cc := CondorConfig{
		Executable: runfilename,
		Infiles:    strings.Join(dstfiles, ","),
		N:          *n,
		NCPU:       *ncpu,
		Memory:     *mem,
	}
	if *classad != "" {
		cc.ClassAds = " && " + *classad
	}

	var condorbuf, runbuf bytes.Buffer
	err = condortmpl.Execute(&condorbuf, cc)
	if err != nil {
		log.Fatal(err)
	}
	err = runtmpl.Execute(&runbuf, struct{ Runfile, Addr, Flags string }{*run, *addr, *wkflags})
	if err != nil {
		log.Fatal(err)
	}

	if *local {
		err := ioutil.WriteFile(runfilename, runbuf.Bytes(), 0755)
		if err != nil {
			log.Fatal(err)
		}
		err = ioutil.WriteFile(condorname, condorbuf.Bytes(), 0644)
		if err != nil {
			log.Fatal(err)
		}
	}

	if *dst == "" {
		log.Fatal("no destination specified")
	}

	submitssh(srcfiles, dstfiles, &condorbuf, &runbuf)
}

func submitssh(srcs, dsts []string, submitdata, runbuf io.Reader) {
	if !*cpy && *n < 1 {
		return
	}

	// use ssh agent
	agentconn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		log.Fatal(err)
	}
	defer agentconn.Close()
	ag := agent.NewClient(agentconn)
	config := &ssh.ClientConfig{
		User: *user,
		Auth: []ssh.AuthMethod{ssh.PublicKeysCallback(ag.Signers)},
	}

	// connect to server (with optional hops)
	var client *ssh.Client
	if *via != "" {
		client, err = ssh.Dial("tcp", *via, config)
		if err != nil {
			log.Fatal(err)
		}
	}

	if client != nil && *dst != "" {
		client, err = Hop(client, *dst, config)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		client, err = ssh.Dial("tcp", *dst, config)
		if err != nil {
			log.Fatal(err)
		}
	}

	// copy files
	err = copyFile(client, submitdata, condorname)
	if err != nil {
		log.Fatal(err)
	}

	err = copyFile(client, runbuf, runfilename)
	if err != nil {
		log.Fatal(err)
	}

	if *cpy {
		for i, name := range srcs {

			f, err := os.Open(name)
			if err != nil {
				log.Fatal(err)
			}
			err = copyFile(client, f, dsts[i])
			if err != nil {
				log.Fatal(err)
			}
			f.Close()
		}
	}

	if *n > 0 {
		out, err := combined(client, "condor_submit "+condorname)
		if err != nil {
			fmt.Printf("%s\n", out)
			log.Fatal(err)
		}
	}
}

func copyFile(c *ssh.Client, r io.Reader, path string) error {
	s, err := c.NewSession()
	if err != nil {
		return err
	}
	defer s.Close()

	w, err := s.StdinPipe()
	if err != nil {
		return err
	}

	s.Start("tee " + path)

	_, err = io.Copy(w, r)
	if err != nil {
		return err
	}
	w.Close()

	return s.Wait()
}

func combined(c *ssh.Client, cmd string) ([]byte, error) {
	s, err := c.NewSession()
	if err != nil {
		return nil, err
	}
	defer s.Close()

	return s.CombinedOutput(cmd)
}

func Hop(through *ssh.Client, toaddr string, c *ssh.ClientConfig) (*ssh.Client, error) {
	hopconn, err := through.Dial("tcp", toaddr)
	if err != nil {
		return nil, err
	}

	conn, chans, reqs, err := ssh.NewClientConn(hopconn, toaddr, c)
	if err != nil {
		return nil, err
	}

	return ssh.NewClient(conn, chans, reqs), nil
}
