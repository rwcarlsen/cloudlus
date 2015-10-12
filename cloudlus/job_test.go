package cloudlus

import (
	"io/ioutil"
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
}
