package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"code.google.com/p/go-uuid/uuid"
)

type Worker struct {
	Id         [16]byte
	ServerAddr string
	Wait       time.Duration
}

func (w *Worker) Run() {
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

		if err := j.Execute(); err != nil {
			j.Status = StatusFailed
			log.Print(err)
		}

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
