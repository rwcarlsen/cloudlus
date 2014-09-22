package cloudlus

import (
	"log"
	"os"
	"time"

	"code.google.com/p/go-uuid/uuid"
)

type Worker struct {
	Id         WorkerId
	ServerAddr string
	FileCache  map[string][]byte
	Wait       time.Duration
}

func (w *Worker) Run() error {
	w.FileCache = map[string][]byte{}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	os.Setenv("PATH", os.Getenv("PATH")+":"+wd)

	uid := uuid.NewRandom()
	copy(w.Id[:], uid)

	if w.Wait == 0 {
		w.Wait = 10 * time.Second
	}

	for {
		<-time.After(w.Wait)
		err := w.dojob()
		if err != nil {
			log.Print(err)
		}
	}
}

func (w *Worker) dojob() error {
	client, err := Dial(w.ServerAddr)
	if err != nil {
		return err
	}
	defer client.Close()

	j, err := client.Fetch(w)
	if err != nil {
		return err
	}

	// add precached files
	for name, data := range w.FileCache {
		j.AddInfile(name, data)
	}

	// cache new files needing caching
	for _, f := range j.Infiles {
		if f.Cache {
			w.FileCache[f.Name] = f.Data
		}
	}

	done := make(chan struct{})
	defer close(done)

	tick := time.NewTicker(beatInterval)
	defer tick.Stop()

	go func() {
		for {
			select {
			case <-tick.C:
				err := client.Heartbeat(w.Id, j.Id)
				if err != nil {
					log.Print(err)
				}
			case <-done:
				return
			}
		}
	}()

	// run job
	j.Execute()
	j.WorkerId = w.Id
	j.Infiles = nil // don't need to send back input files

	return client.Push(w, j)
}
