package main

import (
	"archive/tar"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	"code.google.com/p/go-uuid/uuid"
)

type Job struct {
	Id         int
	Cmds       [][]string
	Resources  map[string][]byte
	Results    []string
	ResultData []byte
	Status     string
	dir        string
	wd         string
}

func NewJob() *Job {
	return &Job{
		Cmds:       [][]string{},
		Results:    []string{},
		Resources:  map[string][]byte{},
		ResultData: []byte{},
	}
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

	for name, data := range j.Resources {
		err := ioutil.WriteFile(name, data, 0755)
		if err != nil {
			return err
		}
	}
	return nil
}

func (j *Job) Execute() error {
	if err := j.setup(); err != nil {
		j.Status = StatusFailed
		return err
	}
	defer j.teardown()

	for _, args := range j.Cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	var buf bytes.Buffer
	tarbuf := tar.NewWriter(&buf)
	for _, name := range j.Results {
		if err := writefile(name, tarbuf); err != nil {
			return err
		}
	}

	j.ResultData = buf.Bytes()
	return nil
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

func (j *Job) teardown() error {
	defer func() {
		j.dir = ""
	}()

	if err := os.Chdir(j.wd); err != nil {
		return err
	}

	return os.RemoveAll(j.dir)
}
