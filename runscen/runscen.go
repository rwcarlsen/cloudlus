package runscen

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"code.google.com/p/go-uuid/uuid"
	"github.com/rwcarlsen/cloudlus/cloudlus"
	"github.com/rwcarlsen/cloudlus/scen"
	"github.com/rwcarlsen/cyan/post"
)

var objfile = "runsim-obj.dat"

const DefaultTimeout = 2 * time.Hour

// RemoteTimeout is the same as Remote, but with a custom timeout rather than
// the default.
func RemoteTimeout(s *scen.Scenario, stdout, stderr io.Writer, addr string, timeout time.Duration) (float64, error) {
	client, err := cloudlus.Dial(addr)
	if err != nil {
		return math.Inf(1), err
	}
	defer client.Close()

	execfn := func(scn *scen.Scenario) (float64, error) {
		j, err := BuildRemoteJob(scn, objfile)
		if err != nil {
			return math.Inf(1), fmt.Errorf("failed to build remote job: %v", err)
		}
		j.Timeout = timeout

		// closing this channel might cause a send on a closed channel if the
		// timeout in the select below fires before the goroutine completes.
		done := make(chan bool, 1)
		go func() {
			j, err = client.Run(j)
			done <- true
		}()

		select {
		case <-done:
			if err != nil {
				return math.Inf(1), fmt.Errorf("job execution failed: %v", err)
			}
		case <-time.After(j.Timeout + 1*time.Hour):
			return math.Inf(1), fmt.Errorf("job rpc timeout limit reached")
		}

		if err := writeLogs(j, stdout, stderr); err != nil {
			return math.Inf(1), fmt.Errorf("job logging failed: %v", err)
		}

		data, err := client.RetrieveOutfileData(j, objfile)
		if err != nil {
			return math.Inf(1), fmt.Errorf("couldn't find objective result file: %v", err)
		}

		val, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if err != nil {
			return math.Inf(1), fmt.Errorf("invalid objective string '%s': %v", data, err)
		}
		return val, nil
	}

	return s.CalcTotalObjective(execfn)
}

// Remote runs scenario s on a remote cloudlus server at addr writing the remote job's
// standard out and error to stdout and stderr respectively.
func Remote(s *scen.Scenario, stdout, stderr io.Writer, addr string) (float64, error) {
	return RemoteTimeout(s, stdout, stderr, addr, DefaultTimeout)
}

// Local runs scenario scn on the local machine connecting the simulation's
// standard out and error to stdout and stderr respectively.  The file names
// of the generated cyclus input file and database are returned along with the
// objective value.
func Local(scn *scen.Scenario, stdout, stderr io.Writer) (obj float64, err error) {
	execfn := func(s *scen.Scenario) (float64, error) {
		// generate cyclus input file and run cyclus
		ui := uuid.NewRandom()
		infile := ui.String() + ".cyclus.xml"
		dbfile := ui.String() + ".sqlite"

		data, err := s.GenCyclusInfile()
		if err != nil {
			return math.Inf(1), err
		}
		err = ioutil.WriteFile(infile, data, 0644)
		if err != nil {
			return math.Inf(1), err
		}

		cmd := exec.Command("cyclus", infile, "-o", dbfile)
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		if err := cmd.Run(); err != nil {
			return math.Inf(1), err
		}
		defer os.Remove(dbfile)

		// post process cyclus output db
		db, err := sql.Open("sqlite3", dbfile)
		if err != nil {
			return math.Inf(1), err
		}
		defer db.Close()

		simids, err := post.Process(db)
		if err != nil {
			return math.Inf(1), err
		}

		return s.CalcObjective(dbfile, simids[0])
	}
	return scn.CalcTotalObjective(execfn)
}

func BuildRemoteJob(s *scen.Scenario, objfile string) (*cloudlus.Job, error) {
	scendata, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	tmpldata, err := ioutil.ReadFile(s.CyclusTmpl)
	if err != nil {
		return nil, err
	}

	j := cloudlus.NewJobCmd("cycobj", "-obj", objfile, "-scen", s.File)
	j.Timeout = 2 * time.Hour
	j.AddInfile(s.CyclusTmpl, tmpldata)
	j.AddInfile(s.File, scendata)
	j.AddOutfile(objfile)

	if flag.NArg() > 0 {
		j.Note = strings.Join(flag.Args(), " ")
	}

	return j, nil
}

func writeLogs(j *cloudlus.Job, stdout, stderr io.Writer) error {
	if j == nil {
		return errors.New("cannot log nil job")
	}
	if stdout != nil {
		_, err := stdout.Write([]byte(j.Stdout))
		if err != nil {
			return err
		}
	}

	if stderr != nil {
		_, err := stderr.Write([]byte(j.Stderr))
		return err
	}

	return nil
}
