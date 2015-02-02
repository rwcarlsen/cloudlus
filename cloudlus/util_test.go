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

type test struct {
	Status string
	Purge  bool // true if GC should purge jobs
}

func TestGC(t *testing.T) {
	tests := []test{
		{StatusComplete, true},
		{StatusFailed, true},
		{StatusRunning, false},
		{StatusQueued, false},
	}

	dblimit := 10000
	j := NewJobCmd("echo", "1")
	jsize := int(j.Size())
	njobsmax := dblimit / jsize

	for _, test := range tests {
		db, _ := NewDB("", dblimit)
		db.PurgeAge = 0 * time.Second
		t.Logf("Testing '%v' jobs", test.Status)

		for k := 0; k < 2*njobsmax; k++ {
			j := NewJobCmd("echo", "1")
			j.Status = test.Status
			err := db.Put(j)
			if err != nil {
				t.Fatal(err)
			}
		}

		nadd, err := db.Count()
		if err != nil {
			t.Fatal(err)
		}
		if nadd != 2*njobsmax {
			t.Fatalf("    jobs not added correctly to db: expected %v, got %v", 2*njobsmax, nadd)
		}

		beforesize, err := db.Size()
		if err != nil {
			t.Fatal(err)
		}

		npurged, nremain, err := db.GC()
		if err != nil {
			t.Fatal(err)
		}

		aftersize, err := db.Size()
		if err != nil {
			t.Fatal(err)
		}

		upper := int64(dblimit) + 2*int64(jsize)
		lower := int64(dblimit) - 2*int64(jsize)

		t.Logf("    %v jobs purged, %v jobs remain", npurged, nremain)
		if test.Purge {
			if beforesize < upper {
				t.Errorf("    didn't overfill db prior to GC: expected >%v bytes, got %v", upper, beforesize)
			} else if aftersize > upper {
				t.Errorf("    GC failed to purge enough jobs: expected <%v bytes, got %v", upper, aftersize)
			} else if aftersize < lower {
				t.Errorf("    GC purged too many jobs: expected >%v bytes, got %v", lower, aftersize)
			} else {
				t.Logf("    db has %v bytes", aftersize)
			}
		} else {
			if npurged != 0 {
				t.Errorf("    no jobs should have been purged but %v were", npurged)
			}
		}
	}
}
