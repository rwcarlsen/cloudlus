package cloudlus

import (
	"testing"
	"time"
)

func TestWorkerDie(t *testing.T) {
	maxidle := 1 * time.Second
	w := &Worker{MaxIdle: maxidle, Wait: 1 * time.Second, ServerAddr: "127.0.0.1:8762"}

	done := make(chan struct{})
	go func() {
		w.Run()
		close(done)
	}()

	select {
	case <-time.After(3 * time.Second):
		t.Errorf("worker failed to die after %v", maxidle)
	case <-done:
	}
}

func TestWorkerLive(t *testing.T) {
	w := &Worker{Wait: 1 * time.Second, ServerAddr: "127.0.0.1:8762"}

	done := make(chan struct{})
	go func() {
		w.Run()
		close(done)
	}()

	select {
	case <-done:
		t.Errorf("worker shouldn't have died, but did")
	case <-time.After(3 * time.Second):
	}
}
