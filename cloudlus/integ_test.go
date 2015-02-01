package cloudlus

import (
	"log"
	"testing"
	"time"

	"code.google.com/p/go-uuid/uuid"
)

const addr = "127.0.0.1:45687"

const workerpoll = 1 * time.Second

func TestCacheLimit(t *testing.T) {
	cachesize := 1000000
	db, _ := NewDB("", cachesize, dblimit)
	s := NewServer(addr, addr, db)
	go s.ListenAndServe()
	defer s.Close()

	j := NewJobCmd("date")
	jsize := int(j.Size())
	njobsmax := cachesize / jsize

	for i := 0; i < 2*njobsmax; i++ {
		j := NewJobCmd("date")
		s.Start(j, nil)
	}

	size := s.alljobs.cache.Size()
	if size > int64(cachesize) {
		t.Errorf("cache over full: expected %v bytes, got %v", cachesize, size)
	} else {
		t.Logf("cache has %v bytes", size)
	}

	for i := 0; i < njobsmax; i++ {
		j := NewJobCmd("date")
		s.Start(j, nil)
	}

	newsize := s.alljobs.cache.Size()
	diff := size - newsize
	if diff < 0 {
		diff = -diff
	}
	if diff > int64(jsize) {
		t.Errorf("cache should be full: expected ~%v bytes, got %v", cachesize, newsize)
	}
}

func TestWorkerFailure(t *testing.T) {
	kill1 := make(chan struct{})
	kill2 := make(chan struct{})
	w1 := &badWorker{ServerAddr: addr}
	go w1.Run(kill1)

	// empty path for in-memory db
	db, err := NewDB("", cachelimit, dblimit)
	s := NewServer(addr, addr, db)
	go s.ListenAndServe()
	defer s.Close()

	// submit job
	j := NewJobCmd("date")
	s.Start(j, nil)

	// wait for it to be running
	<-time.After(2 * workerpoll)

	js, err := s.Get(j.Id)
	if err != nil {
		t.Errorf("unexpected error retrieving job: %v", err)
	}
	if js.Status != StatusRunning {
		t.Errorf("wrong job status: got '%v', expected '%v'", js.Status, StatusRunning)
	}

	close(kill1)
	w2 := &goodWorker{ServerAddr: addr}
	go w2.Run(kill2)
	<-time.After((beatLimit + beatCheckFreq + workerpoll) * 2)

	js, err = s.Get(j.Id)
	if err != nil {
		t.Errorf("unexpected error retrieving job: %v", err)
	}
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
	j.Execute()
	j.WorkerId = w.Id
	j.Infiles = nil // don't need to send back input files

	return client.Push(tmp, j)
}

type badWorker struct {
	Id         WorkerId
	ServerAddr string
}

func (w *badWorker) Run(kill chan struct{}) error {
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

	return nil
}
