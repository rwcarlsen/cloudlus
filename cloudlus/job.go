package cloudlus

import (
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

var DefaultTimeout = 600 * time.Second

type Job struct {
	Id        JobId
	Cmd       []string
	Infiles   []File
	Outfiles  []File
	Status    string
	Stdout    string
	Stderr    string
	Timeout   time.Duration
	Submitted time.Time
	Fetched   time.Time
	Started   time.Time
	Finished  time.Time
	WorkerId  WorkerId
	Note      string
	dir       string
	wd        string
	whitelist []string
	log       io.Writer
}

type File struct {
	Name  string
	Data  []byte
	Cache bool
}

func NewJob() *Job {
	uid := uuid.NewRandom()
	var id [16]byte
	copy(id[:], uid)
	return &Job{
		Id:      id,
		Timeout: DefaultTimeout,
	}
}

func NewJobCmd(cmd string, args ...string) *Job {
	j := NewJob()
	j.Cmd = append([]string{cmd}, args...)
	return j
}

func NewJobDefault(data []byte) *Job {
	j := NewJobCmd("cyclus", DefaultInfile)
	j.AddOutfile("cyclus.sqlite")
	j.AddInfile(DefaultInfile, data)
	return j
}

func NewJobDefaultFile(fname string) (*Job, error) {
	data, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	return NewJobDefault(data), nil
}

func (j *Job) Whitelist(cmds ...string) {
	j.whitelist = append(j.whitelist, cmds...)
}

func (j *Job) Done() bool {
	return j.Status == StatusComplete || j.Status == StatusFailed
}

func (j *Job) AddOutfile(fname string) {
	j.Outfiles = append(j.Outfiles, File{fname, nil, false})
}

func (j *Job) AddInfile(fname string, data []byte) {
	j.Infiles = append(j.Infiles, File{fname, data, false})
}

func (j *Job) AddInfileCached(fname string, data []byte) {
	j.Infiles = append(j.Infiles, File{fname, data, true})
}

func (j *Job) Size() int64 {
	n := len(j.Stdout) + len(j.Stderr)
	for _, f := range j.Infiles {
		n += len(f.Data)
	}
	for _, f := range j.Outfiles {
		n += len(f.Data)
	}
	return int64(n) + 12*8
}

func (j *Job) Execute(kill chan bool) {
	if j.log == nil {
		j.log = os.Stdout
	}
	if j.Timeout == 0 {
		j.Timeout = DefaultTimeout
	}
	j.Started = time.Now()
	defer func() { j.Finished = time.Now() }()

	// set up stderr/stdout tee's and exec command
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	multiout := io.MultiWriter(j.log, &stdout)
	multierr := io.MultiWriter(j.log, &stderr)
	defer func() { j.Stdout += stdout.String() }()
	defer func() { j.Stderr += stderr.String() }()

	// make sure job is valid/acceptable
	if len(j.Cmd) == 0 {
		j.Status = StatusFailed
		fmt.Fprint(multierr, "job has no command to run")
		return
	} else if len(j.whitelist) > 0 {
		approved := false
		for _, cmd := range j.whitelist {
			if j.Cmd[0] == cmd {
				approved = true
				break
			}
		}
		if !approved {
			j.Status = StatusFailed
			fmt.Fprintf(multierr, "'%v' is not a white-listed command in %v", j.Cmd[0], j.whitelist)
			return
		}
	}

	if err := j.setup(); err != nil {
		j.Status = StatusFailed
		fmt.Fprint(multierr, err)
		return
	}
	defer j.teardown()

	var err error

	cmd := exec.Command(j.Cmd[0], j.Cmd[1:]...)
	fmt.Fprintf(j.log, "running job %v command: %v\n", j.Id, cmd.Args)

	cmd.Stderr = multierr
	cmd.Stdout = multiout

	// launch job process
	done := make(chan bool)
	go func() {
		if err := cmd.Run(); err != nil {
			j.Status = StatusFailed
			fmt.Fprint(multierr, err)
		} else {
			j.Status = StatusComplete
		}
		close(done)
	}()

	// wait for job to finish or timeout
	select {
	case <-time.After(j.Timeout):
		cmd.Process.Kill()
		j.Status = StatusFailed
		fmt.Fprintf(multierr, "\nJob timed out after %v\n", j.Timeout)
		<-done
		return
	case <-kill:
		cmd.Process.Kill()
		j.Status = StatusFailed
		fmt.Fprintf(multierr, "\nJob was terminated by server\n")
		<-done
		return
	case <-done:
	}

	// collect output data
	for i, f := range j.Outfiles {
		j.Outfiles[i].Data, err = ioutil.ReadFile(f.Name)
		if err != nil {
			j.Status = StatusFailed
			fmt.Fprintf(multierr, "%v", err)
		}
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

	if err := os.Chdir(j.dir); err != nil {
		return err
	}

	for _, f := range j.Infiles {
		err := ioutil.WriteFile(f.Name, f.Data, 0755)
		if err != nil {
			return err
		}
	}
	return nil
}

func (j *Job) teardown() error {
	defer func() {
		j.dir = ""
	}()

	if err := os.Chdir(j.wd); err != nil {
		log.Print(err)
		return err
	}

	if err := os.RemoveAll(j.dir); err != nil {
		log.Print(err)
		return err
	}
	return nil
}

// JobStat is holds a subset of job fields for marshalling and sending small
// messages with current job state/status info while avoiding sending large
// data like input and output files.
type JobStat struct {
	Id        JobId
	Cmd       []string
	Status    string
	Size      int64
	Stdout    string
	Stderr    string
	Submitted time.Time
	Started   time.Time
	Finished  time.Time
}

func NewJobStat(j *Job) *JobStat {
	return &JobStat{
		Id:        j.Id,
		Cmd:       j.Cmd,
		Status:    j.Status,
		Size:      j.Size(),
		Stdout:    j.Stdout,
		Stderr:    j.Stderr,
		Submitted: j.Submitted,
		Started:   j.Started,
		Finished:  j.Finished,
	}
}
