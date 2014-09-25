package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"code.google.com/p/go.crypto/ssh"
	"code.google.com/p/go.crypto/ssh/agent"
)

var (
	n       = flag.Int("n", 10, "number of bots to deploy")
	keyfile = flag.String("keyfile", filepath.Join(os.Getenv("HOME"), ".ssh/id_rsa"), "path to ssh private key file")
	user    = flag.String("user", "rcarlsen", "condor (and via node) ssh username")
	via     = flag.String("via", "best-tux.cae.wisc.edu:22", "intermediate server URI (if needed)")
	dst     = flag.String("dst", "submit-3.chtc.wisc.edu:22", "condor submit node URI")
	run     = flag.String("run", "", "name of script for condor to run")
	addr    = flag.String("addr", "", "ip:port of cloudlus server")
)

type CondorConfig struct {
	Executable string
	Infiles    string
}

const condorname = "condor.submit"

const condorfile = `
universe = vanilla
executable = {{.Executable}}
transfer_input_files = {{.Infiles}}
should_transfer_files = yes
when_to_transfer_output = on_exit
output = worker.$(PROCESS).output
error = worker.$(PROCESS).error
log = workers.log
requirements = OpSys == "LINUX" && Arch == "x86_64" && (OpSysAndVer =?= "SL6")

queue
`

const runfilename = "CLOUDLUS_runfile.sh"

const runfile = `
#!/bin/bash

{{with .Runfile}}bash ./{{.}}{{end}}
./cloudlus work -addr {{.Addr}}
`

var condortmpl = template.Must(template.New("submitfile").Parse(condorfile))
var runtmpl = template.Must(template.New("runfile").Parse(runfile))

func main() {
	log.SetFlags(0)
	flag.Usage = func() {
		fmt.Println("Usage: condor [FILE...]")
		fmt.Println("Copy listed files to condor submit node and possibly submit a job.\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *addr == "" {
		log.Fatal("must specify server address")
	}

	// assemple file names to copy over
	srcfiles := flag.Args()

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
	cc := CondorConfig{runfilename, strings.Join(dstfiles, " ")}
	var condorbuf, runbuf bytes.Buffer
	err = condortmpl.Execute(&condorbuf, cc)
	if err != nil {
		log.Fatal(err)
	}
	err = runtmpl.Execute(&runbuf, struct{ Runfile, Addr string }{*run, *addr})
	if err != nil {
		log.Fatal(err)
	}

	//fmt.Println("condor.submit contents:\n", condorbuf.String())
	//fmt.Printf("%v contents:\n%v\n", runfilename, runbuf.String())

	if *dst == "" {
		log.Fatal("no destination specified")
	}

	submitssh(srcfiles, dstfiles, &condorbuf, &runbuf)
}

func submitssh(srcs, dsts []string, submitdata, runbuf io.Reader) {
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

	for i := 0; i < *n; i++ {
		//fmt.Printf("running command 'condor_submit %v'\n", condorname)
		//continue
		out, err := combined(client, "condor_submit "+condorname)
		if err != nil {
			fmt.Printf("%s\n", out)
			log.Fatal(err)
		}
	}
}

func copyFile(c *ssh.Client, r io.Reader, path string) error {
	//fmt.Printf("copying file %v\n", path)
	//return nil

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
