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

const (
	partial = "partial"
	full    = "all"
	none    = "none"
)

type test struct {
	Statuss   []string
	PurgeType string // partial, full, or none
}

func TestDB_Count(t *testing.T) {
	db, _ := NewDB("", dblimit)

	njobs := 200

	for i := 0; i < njobs/2; i++ {
		j := NewJobCmd("echo", "1")
		j.Status = StatusComplete
		if err := db.Put(j); err != nil {
			t.Fatal(err)
		}
		j = NewJobCmd("echo", "1")
		j.Status = StatusRunning
		if err := db.Put(j); err != nil {
			t.Fatal(err)
		}
	}

	n, err := db.Count()
	if err != nil {
		log.Fatal(err)
	}

	if n != njobs {
		t.Errorf("DB gives wrong job count: want %v, got %v.", njobs, n)
		t.Errorf("  - This is likely caused by not skipping index prefixes properly.")
	}
}

func TestGC(t *testing.T) {
	tests := []test{
		{[]string{StatusComplete}, full},
		{[]string{StatusFailed}, full},
		{[]string{StatusRunning}, none},
		{[]string{StatusQueued}, none},
		{[]string{StatusComplete, StatusQueued}, partial},
	}

	dblimit := 10000
	j := NewJobCmd("echo", "1")
	jsize := int(j.Size())
	njobsmax := dblimit / jsize

	for _, test := range tests {
		db, _ := NewDB("", dblimit)
		db.PurgeAge = 0 * time.Second
		t.Logf("Testing '%v' jobs", test.Statuss)

		for k := 0; k < 2*njobsmax; k++ {
			j := NewJobCmd("echo", "1")
			j.Status = test.Statuss[k%len(test.Statuss)]
			if err := db.Put(j); err != nil {
				t.Fatal(err)
			}
		}

		nadd, err := db.Count()
		if err != nil {
			t.Fatal(err)
		} else if nadd != 2*njobsmax {
			t.Errorf("    jobs not added correctly to db: expected %v, got %v", 2*njobsmax, nadd)
			continue
		}

		beforesize, err := db.Size()
		if err != nil {
			t.Fatal(err)
		}

		npurged, nremain, err := db.GC()
		if err != nil {
			t.Fatal(err)
		}

		upper := int64(dblimit) + 2*int64(jsize)

		t.Logf("    %v jobs purged, %v jobs remain, db limit = %v bytes", npurged, nremain, dblimit)
		if beforesize < upper {
			t.Errorf("    didn't overfill db prior to GC: expected > %v bytes, got %v", upper, beforesize)
		}

		switch test.PurgeType {
		case partial:
			if npurged >= nadd || npurged <= 0 {
				t.Errorf("    GC purged wrong # jobs: want 0 < n < %v, got %v", nadd, npurged)
			}
		case full:
			if npurged != nadd {
				t.Errorf("    GC purged wrong # jobs: want %v, got %v", nadd, npurged)
			}
		case none:
			if npurged != 0 {
				t.Errorf("    no jobs should have been purged but %v were", npurged)
			}
		}
	}
}
