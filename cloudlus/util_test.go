package cloudlus

import (
	"log"
	"testing"
	"time"
)

func nolog(s *Server) {
	log.SetOutput(devnull)
	s.log = log.New(devnull, "", 0)
}

func TestDbLimit(t *testing.T) {
	CollectFreq = 1 * time.Second

	dblimit := 10000
	db, _ := NewDB("", dblimit)
	db.purgeAge = 0 * time.Second
	s := NewServer(testaddr, testaddr, db)
	nolog(s)
	go s.ListenAndServe()
	defer s.Close()

	w := &Worker{Wait: 1 * time.Second, ServerAddr: testaddr, nolog: true}
	go w.Run()

	j := NewJobCmd("date")
	jsize := int(j.Size())
	njobsmax := dblimit / jsize

	for i := 0; i < 2*njobsmax; i++ {
		j := NewJobCmd("echo", "1")
		j.log = devnull
		s.Run(j)
	}

	<-time.After(2 * time.Second) // wait for db to purge old jobs

	size, _ := s.alljobs.Size()
	if size > int64(dblimit)+2*int64(jsize) {
		t.Errorf("db over full: expected ~%v bytes, got %v", dblimit, size)
	} else if size < int64(dblimit)-2*int64(jsize) {
		t.Errorf("db over purged: expected ~%v bytes, got %v", dblimit, size)
	} else {
		t.Logf("db has %v bytes", size)
	}

}
