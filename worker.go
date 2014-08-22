package cloudlus

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"code.google.com/p/go-uuid/uuid"
)

type Worker struct {
	Id         [16]byte
	ServerAddr string
	Wait       time.Duration
}

func (w *Worker) Run() error {
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

		resp, err := http.Get(w.ServerAddr + "/work/fetch")
		if err != nil {
			log.Print(err)
			continue
		}

		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Print(err)
			continue
		}

		var j *Job
		if err := json.Unmarshal(data, &j); err != nil {
			log.Print(err)
			continue
		} else if j == nil {
			log.Print("got nil job")
			continue
		}

		j.Execute(600 * time.Second)

		data, err = json.Marshal(j)
		if err != nil {
			log.Print(err)
		}

		resp, err = http.Post(w.ServerAddr+"/work/push", "application/json", bytes.NewBuffer(data))
		if err != nil {
			log.Print(err)
		}
		resp.Body.Close()
	}
}
