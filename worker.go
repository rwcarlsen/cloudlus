package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

type Worker struct {
	ServerAddr string
	Wait       time.Duration
}

func (w *Worker) Run() {
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
