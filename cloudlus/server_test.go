package cloudlus

import (
	"testing"
	"time"
)

func TestServerJobGC(t *testing.T) {
	const testaddr = "127.0.0.1:45687"
	dblimit := 10000
	j := NewJobCmd("echo", "1")
	jsize := int(j.Size())
	njobsmax := dblimit / jsize

	db, _ := NewDB("", dblimit)
	db.PurgeAge = 0 * time.Second

	for k := 0; k < 2*njobsmax; k++ {
		j := NewJobCmd("echo", "1")
		j.Status = StatusComplete
		err := db.Put(j)
		if err != nil {
			t.Fatal(err)
		}
	}

	nbefore, err := db.Count()
	if err != nil {
		t.Fatal(err)
	}

	s := NewServer(testaddr, testaddr, db)
	s.CollectFreq = 1 * time.Second
	go s.ListenAndServe()
	defer s.Close()

	<-time.After(2 * time.Second)

	nafter, err := db.Count()
	if err != nil {
		t.Fatal(err)
	}

	if nbefore == nafter {
		t.Errorf("server failed to run job GC")
	}
}
