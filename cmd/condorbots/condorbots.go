package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
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
	run     = flag.String("run", "init.sh", "name of script for condor to run")
)

type CondorConfig struct {
	Executable string
	Infiles    string
}

const condorname = "condor.submit"

var condorfile = `
	universe = vanilla
	executable = {{.Executable}}
	arguments = $arguments $host $port
	transfer_input_files = {{.Infiles}}
	should_transfer_files = yes
	when_to_transfer_output = on_exit
	output = worker.\$(PROCESS).output
	error = worker.\$(PROCESS).error
	log = workers.log
	requirements = OpSys == "LINUX" && Arch == "x86_64" && (OpSysAndVer =?= "SL6")
`
var tmpl = template.Must(template.New("submitfile").Parse(condorfile))

// TODO: add cloudlus binary to list of files to condor submit file
// TODO: create init.sh wrapper script that is sent over that starts cloudlus
//       worker after running init.sh.  Use this wrapper as the condor
//       executable.

func main() {
	log.SetFlags(0)
	flag.Usage = func() {
		fmt.Println("Usage: condor [FILE...]")
		fmt.Println("Copy listed files to condor submit node and possibly submit a job.\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	cc := CondorConfig{*run, strings.Join(append(flag.Args(), *run), " ")}
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, cc)
	if err != nil {
		log.Fatal(err)
	}

	if *via == "" && *dst == "" {
		submitlocal(&buf)
	} else {
		submitssh(&buf)
	}
}

func submitlocal(submitdata io.Reader) {
	panic("not implemented")
}

func submitssh(submitdata io.Reader) {
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
	fnames := flag.Args()

	err = copyFile(client, submitdata, condorname)
	if err != nil {
		log.Fatal(err)
	}

	for _, fname := range fnames {
		fmt.Printf("copying %v\n", fname)
		f, err := os.Open(fname)
		if err != nil {
			log.Fatal(err)
		}
		err = copyFile(client, f, fname)
		if err != nil {
			log.Fatal(err)
		}
		f.Close()
	}

	for i := 0; i < *n; i++ {
		out, err := combined(client, "condor_submit "+condorname)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", out)
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
