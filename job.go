package main

import (
	"io/ioutil"
	"os"
	"os/exec"

	"code.google.com/p/go-uuid/uuid"
)

type Job struct {
	Id        int
	Cmds      [][]string
	Resources map[string][]byte
	Results   map[string][]byte
	Status    string
	dir       string
	wd        string
}

func NewJob() *Job {
	return &Job{
		Cmds:      [][]string{},
		Results:   map[string][]byte{},
		Resources: map[string][]byte{},
	}
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

	for name := range j.Results {
		data, err := ioutil.ReadFile(name)
		if err != nil {
			return err
		}
		j.Results[name] = data
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
