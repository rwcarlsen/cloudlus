package cloudlus

import (
	"log"
	"os"
	"time"

	"code.google.com/p/go-uuid/uuid"
)

type Worker struct {
	Id         [16]byte
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

		client, err := Dial(w.ServerAddr)
		if err != nil {
			log.Print(err)
			continue
		}

		j, err := client.Fetch(w)
		if err != nil {
			log.Print(err)
			continue
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

		// run job
		j.Execute()
		j.Infiles = nil // don't need to send back input files

		err = client.Push(w, j)
		if err != nil {
			log.Print(err)
		}

	}
}
