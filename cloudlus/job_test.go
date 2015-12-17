package cloudlus

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestJobTimeout(t *testing.T) {
	j := NewJobCmd("sleep", "10000")
	j.Timeout = 1 * time.Second

	kill := make(chan bool)
	done := make(chan struct{})

	go func() {
		j.Execute(kill, ioutil.Discard)
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Errorf("job failed to time out")
	}
	fmt.Fprintf(os.Stderr, "\n")
}

// TestJobKill is also useful for finding data races.
func TestJobKill(t *testing.T) {
	j := NewJobCmd("sleep", "10000")
	j.Timeout = 1000 * time.Second

	kill := make(chan bool)
	done := make(chan struct{})

	go func() {
		j.Execute(kill, ioutil.Discard)
		done <- struct{}{}
	}()

	kill <- true
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Errorf("job failed to time out")
	}
	fmt.Fprintf(os.Stderr, "\n")
}
