package cloudlus

import (
	"io/ioutil"
	"log"
	"testing"
	"time"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/code.google.com/p/go-uuid/uuid"
)

const workerpoll = 1 * time.Second

// TestRemoteKill checks that the server will force-terminate jobs that exceed
// their job timeout - guarding against workers that aren't killing the jobs
// themselves past the job timeout.
func TestRemoteKill(t *testing.T) {
	testaddr := "127.0.0.1:45687"
	beatInterval = 2 * time.Second
	beatLimit = 2 * beatInterval
	beatCheckFreq = beatInterval / 2

	// empty path for in-memory db
	db, _ := NewDB("", dblimit)
	s := NewServer(testaddr, testaddr, db)
	go s.ListenAndServe()
	defer s.Close()

	// submit job
	j := NewJobCmd("sleep", "100")
	j.Timeout = 3 * time.Second
	s.Start(j, nil)

	kill1 := make(chan struct{})
	w1 := &foreverWorker{ServerAddr: testaddr}
	go w1.Run(kill1)
	defer close(kill1)

	// wait for it to be running
	<-time.After(2 * workerpoll)

	if !w1.running {
		t.Errorf("foreverWorker is not running, but should be")
	}

	<-time.After(beatLimit + 2*time.Second)

	if w1.running {
		t.Errorf("worker is still running a job that should have been killed by the server")
	}
}

// TestRequeue checks that jobs are successfully requeued and completed
// after the job's original worker stops beating.
func TestRequeue(t *testing.T) {
	testaddr := "127.0.0.1:45689"
	beatInterval = 2 * time.Second
	beatLimit = 2 * beatInterval
	beatCheckFreq = beatInterval / 2

	// empty path for in-memory db
	db, err := NewDB("", dblimit)
	s := NewServer(testaddr, testaddr, db)
	go func() {
		t.Fatal(s.ListenAndServe())
	}()
	defer s.Close()

	// submit job
	j := NewJobCmd("date")
	s.Start(j, nil)

	time.Sleep(1 * time.Second)

	kill1 := make(chan struct{})
	w1 := &badWorker{ServerAddr: testaddr, MaxFetch: 1}
	go w1.Run(kill1)

	// wait for worker to be running job
	time.Sleep(3 * workerpoll)

	js, err := s.Get(j.Id)
	if err != nil {
		t.Errorf("unexpected error retrieving job: %v", err)
	}
	if js.Status != StatusRunning {
		t.Errorf("wrong job status: got '%v', expected '%v'", js.Status, StatusRunning)
	}

	// kill bad worker and wait for job to be requeued
	close(kill1)
	<-time.After(beatLimit + beatCheckFreq)

	js, _ = s.Get(j.Id)
	if js.Status != StatusQueued {
		t.Errorf("wrong job status: got '%v', expected '%v'", js.Status, StatusQueued)
	}

	// start good worker and wait for job to complete
	w2 := &goodWorker{ServerAddr: testaddr}
	kill2 := make(chan struct{})
	go w2.Run(kill2)
	<-time.After(workerpoll + 2*time.Second)

	js, _ = s.Get(j.Id)
	if js.Status != StatusComplete {
		t.Errorf("wrong job status: got '%v', expected '%v'", js.Status, StatusComplete)
	}
	close(kill2)
}

type goodWorker struct {
	Id         WorkerId
	ServerAddr string
}

func (w *goodWorker) Run(kill chan struct{}) error {
	uid := uuid.NewRandom()
	copy(w.Id[:], uid)

	for {
		select {
		case <-kill:
			return nil
		default:
			err := w.dojob()
			if err != nil {
				log.Print(err)
			}
			<-time.After(workerpoll)
		}
	}
}

func (w *goodWorker) dojob() error {
	client, err := Dial(w.ServerAddr)
	if err != nil {
		return err
	}
	defer client.Close()

	tmp := &Worker{Id: w.Id}

	j, err := client.Fetch(tmp)
	if err == nojoberr {
		return nil
	} else if err != nil {
		return err
	}

	done := make(chan struct{})
	defer close(done)
	client.Heartbeat(w.Id, j.Id, done)

	// run job
	j.Whitelist("date")
	j.Execute(nil, ioutil.Discard)
	j.WorkerId = w.Id
	j.Infiles = nil // don't need to send back input files

	return client.Push(tmp, j)
}

// badWorker never sends back the results of the jobs it runs.
type badWorker struct {
	Id         WorkerId
	ServerAddr string
	MaxFetch   int
	nfetched   int
}

func (w *badWorker) Run(kill chan struct{}) error {
	uid := uuid.NewRandom()
	copy(w.Id[:], uid)

	for w.nfetched < w.MaxFetch || w.MaxFetch == 0 {
		select {
		case <-kill:
			return nil
		default:
			err := w.dojob()
			if err != nil {
				log.Print(err)
			}
			<-time.After(workerpoll)
		}
	}
	return nil
}

func (w *badWorker) dojob() error {
	client, err := Dial(w.ServerAddr)
	if err != nil {
		return err
	}
	defer client.Close()

	tmp := &Worker{Id: w.Id}

	_, err = client.Fetch(tmp)
	if err == nojoberr {
		return nil
	} else if err != nil {
		return err
	}
	w.nfetched++

	return nil
}

// foreverWorker fails to terminate jobs when they exceed their specified job
// timeout and just keeps running them.
type foreverWorker struct {
	Id         WorkerId
	ServerAddr string
	running    bool
}

func (w *foreverWorker) Run(kill chan struct{}) error {
	uid := uuid.NewRandom()
	copy(w.Id[:], uid)

	for {
		select {
		case <-kill:
			return nil
		default:
			err := w.dojob()
			if err != nil {
				log.Print(err)
			}
			<-time.After(workerpoll)
		}
	}
}

func (w *foreverWorker) dojob() error {
	w.running = true
	defer func() { w.running = false }()

	client, err := Dial(w.ServerAddr)
	if err != nil {
		return err
	}
	defer client.Close()

	tmp := &Worker{Id: w.Id}

	j, err := client.Fetch(tmp)
	if err == nojoberr {
		return nil
	} else if err != nil {
		return err
	}

	// sneakily increase timeout
	j.Timeout = 1000 * time.Hour
	done := make(chan struct{})
	defer close(done)
	kill := client.Heartbeat(w.Id, j.Id, done)

	// run job
	j.Whitelist("sleep")
	j.Execute(kill, ioutil.Discard)
	j.WorkerId = w.Id
	j.Infiles = nil // don't need to send back input files

	return client.Push(tmp, j)
}
