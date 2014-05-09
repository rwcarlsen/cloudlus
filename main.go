package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
)

var addr = flag.String("addr", "127.0.0.1:4242", "network address to listen on")

var (
	submitjobs = make(chan *Job)
	pushjobs   = make(chan *Job)
	fetchjobs  = make(chan WorkRequest)
	alljobs    = map[int]*Job{}
	queue      = []*Job{}
)

const (
	StatusQueued   = "queued"
	StatusRunning  = "running"
	StatusComplete = "complete"
)

type Job struct {
	Id        int
	Infile    []byte
	Resources map[string][]byte
	PostCmds  []string
	Results   map[string][]byte
	Status    string
}

type JobRequest struct {
	Id   int
	Resp chan *Job
}

type WorkRequest chan *Job

func main() {
	flag.Parse()

	http.HandleFunc("/job/submit", submit)
	http.HandleFunc("/job/retrieve", retrieve)
	http.HandleFunc("/work/fetch", fetch)
	http.HandleFunc("/work/push", push)

	go dispatcher()

	if err := http.ListenAndServe(*addr); err != nil {
		log.Fatal(err)
	}
}

func submit(w http.ResponseWriter, r http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}

	j := &Job{}
	if err := json.Unmarshal(data, &j); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}
	submitjobs <- j
}

func submit(w http.ResponseWriter, r http.Request) {
	w
}

func dispatcher() {
	for {
		select {
		case j := <-submitjobs:
			j.Status = StatusQueued
			queue = append(queue, j)
			alljobs[j.Id] = j
		case req := <-retrievejobs:
			j := alljobs[req.Id]
			req.Resp <- j
			delete(alljobs, req.Id)
		case j := <-pushjobs:
			j.Status = StatusComplete
			alljobs[j.Id] = j
		case req := <-fetchjobs:
			j := queue[0]
			j.Status = StatusRunning
			queue = queue[1:]
			req <- j
		}
	}
}

func fetch(w http.ResponseWriter, r http.Request) {
}

func push(w http.ResponseWriter, r http.Request) {
}

type Queue struct {
	ch chan *Job
}
