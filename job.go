package main

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"time"

	"code.google.com/p/go-uuid/uuid"
)

const (
	StatusQueued   = "queued"
	StatusRunning  = "running"
	StatusComplete = "complete"
	StatusFailed   = "failed"
)

const DefaultInfile = "input.xml"

type Job struct {
	Id         [16]byte
	Submitted  time.Time
	Cmds       [][]string
	Resources  map[string][]byte
	Results    []string
	ResultData []byte
	Status     string
	Output     string
	dir        string
	wd         string
}

func NewJob() *Job {
	uid := uuid.NewRandom()
	var id [16]byte
	copy(id[:], uid)
	return &Job{
		Id:         id,
		Cmds:       [][]string{},
		Results:    []string{},
		Resources:  map[string][]byte{},
		ResultData: []byte{},
	}
}

func NewJobDefault(data []byte) *Job {
	j := NewJob()
	j.Cmds = append(j.Cmds, []string{"cyclus", DefaultInfile})
	j.Results = []string{"cyclus.sqlite"}
	j.Resources[DefaultInfile] = data
	return j
}

func NewJobDefaultFile(fname string) (*Job, error) {
	data, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	return NewJobDefault(data), nil
}

func (j *Job) Size() int {
	n := 0
	for _, data := range j.Resources {
		n += len(data)
	}
	return n + len(j.ResultData)
}

func (j *Job) setup() error {
	var err error
	if j.wd == "" {
		j.wd, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	j.dir = uuid.NewRandom().String()
	err = os.MkdirAll(j.dir, 0755)
	if err != nil {
		return err
	}

	if err := os.Chdir(j.dir); err != nil {
		return err
	}

	for name, data := range j.Resources {
		err := ioutil.WriteFile(name, data, 0755)
		if err != nil {
			return err
		}
	}
	return nil
}

func (j *Job) Execute(dur time.Duration) {
	if err := j.setup(); err != nil {
		j.Status = StatusFailed
		return
	}
	defer j.teardown()

	timeout := time.After(dur)

	var out bytes.Buffer
	multiout := io.MultiWriter(os.Stdout, &out)
	multierr := io.MultiWriter(os.Stderr, &out)
	for _, args := range j.Cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stderr = multierr
		cmd.Stdout = multiout

		done := make(chan bool)
		go func() {
			if err := cmd.Run(); err != nil {
				j.Status = StatusFailed
				j.Output += "\n" + out.String()
			}
			done <- true
			close(done)
		}()

		select {
		case <-timeout:
			cmd.Process.Kill()
			j.Status = StatusFailed
			msg := fmt.Sprintf("Job timed out after %v", dur)
			j.Output += "\n" + msg
			log.Print(msg)
			return
		case <-done:
		}
	}
	j.Output = out.String()
	j.Status = StatusComplete

	var buf bytes.Buffer
	tarbuf := tar.NewWriter(&buf)
	for _, name := range j.Results {
		if err := writefile(name, tarbuf); err != nil {
			j.Status = StatusFailed
			return
		}
	}

	j.ResultData = buf.Bytes()
}

func (j *Job) teardown() error {
	defer func() {
		j.dir = ""
	}()

	if err := os.Chdir(j.wd); err != nil {
		return err
	}

	return os.RemoveAll(j.dir)
}

func writefile(fname string, buf *tar.Writer) error {
	f, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer f.Close()

	// make the tar header
	info, err := f.Stat()
	if err != nil {
		return err
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}

	// write the header and file data to the tar archive
	if err := buf.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := io.Copy(buf, f); err != nil {
		return err
	}
	return nil
}
