package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"

	"github.com/rwcarlsen/gocache"
)

const (
	StatusQueued   = "queued"
	StatusRunning  = "running"
	StatusComplete = "complete"
	StatusFailed   = "failed"
)

type JobRequest struct {
	Id   int
	Resp chan *Job
}

type JobSubmit struct {
	J    *Job
	Resp chan int
}

type WorkRequest chan *Job

type Server struct {
	nextid       int
	submitjobs   chan JobSubmit
	retrievejobs chan JobRequest
	pushjobs     chan *Job
	fetchjobs    chan WorkRequest
	statjobs     chan JobRequest
	queue        []*Job

	alljobs *cache.LRUCache
}

const MB = 1 << 20

func NewServer() *Server {
	return &Server{
		submitjobs:   make(chan JobSubmit),
		retrievejobs: make(chan JobRequest),
		statjobs:     make(chan JobRequest),
		pushjobs:     make(chan *Job),
		fetchjobs:    make(chan WorkRequest),
		alljobs:      cache.NewLRUCache(250 * MB),
	}
}

func (s *Server) ListenAndServe(addr string) error {
	http.HandleFunc("/job/submit", s.submit)
	http.HandleFunc("/job/submit-infile", s.submitInfile)
	http.HandleFunc("/job/retrieve/", s.retrieve)
	http.HandleFunc("/job/status/", s.status)
	http.HandleFunc("/work/fetch", s.fetch)
	http.HandleFunc("/work/push", s.push)

	go s.dispatcher()

	return http.ListenAndServe(addr, nil)
}

func (s *Server) dispatcher() {
	for {
		select {
		case sub := <-s.submitjobs:
			s.nextid++
			j := sub.J
			j.Id = s.nextid
			j.Status = StatusQueued
			s.queue = append(s.queue, j)
			s.alljobs.Set(j.Id, j)
			sub.Resp <- j.Id
		case req := <-s.retrievejobs:
			v, _ := s.alljobs.Get(req.Id)
			j := v.(*Job)

			req.Resp <- j
		case req := <-s.statjobs:
			v, _ := s.alljobs.Get(req.Id)
			j := v.(*Job)
			req.Resp <- j
		case j := <-s.pushjobs:
			if j.Status != StatusFailed {
				j.Status = StatusComplete
			}
			s.alljobs.Set(j.Id, j)
		case req := <-s.fetchjobs:
			var j *Job = nil
			if len(s.queue) > 0 {
				j = s.queue[0]
				j.Status = StatusRunning
				s.queue = s.queue[1:]
			}
			req <- j
		}
	}
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}

	j := NewJob()
	if err := json.Unmarshal(data, &j); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}

	ch := make(chan int)
	s.submitjobs <- JobSubmit{J: j, Resp: ch}
	id := <-ch
	fmt.Fprintf(w, "%v", id)
}

func (s *Server) submitInfile(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}

	j := NewJob()
	if err := json.Unmarshal(data, &j); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}

	ch := make(chan int)
	s.submitjobs <- JobSubmit{J: j, Resp: ch}
	id := <-ch
	fmt.Fprintf(w, "%v", id)
}

func (s *Server) retrieve(w http.ResponseWriter, r *http.Request) {
	idstr := r.URL.Path[len("/job/retrieve/"):]
	id, err := strconv.Atoi(idstr)
	if err != nil {
		http.Error(w, "malformed job id "+idstr, http.StatusBadRequest)
		log.Print("malformed job id status request: ", idstr)
		return
	}

	ch := make(chan *Job)
	s.retrievejobs <- JobRequest{Id: id, Resp: ch}
	j := <-ch
	if j == nil {
		http.Error(w, "unknown job id "+idstr, http.StatusBadRequest)
		log.Print("unknown job id status request: ", idstr)
		return
	}

	w.Header().Add("Content-Disposition", fmt.Sprintf("filename=\"result-id-%v.tar\"", id))
	_, err = w.Write(j.ResultData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	idstr := r.URL.Path[len("/job/status/"):]
	id, err := strconv.Atoi(idstr)
	if err != nil {
		http.Error(w, "malformed job id "+idstr, http.StatusBadRequest)
		log.Print("malformed job id status request: ", idstr)
		return
	}

	ch := make(chan *Job)
	s.statjobs <- JobRequest{Id: id, Resp: ch}
	j := <-ch
	if j == nil {
		http.Error(w, "unknown job id "+idstr, http.StatusBadRequest)
		log.Print("unknown job id status request: ", idstr)
		return
	}

	jj := NewJob()
	jj.Id = j.Id
	jj.Status = j.Status
	data, err := json.Marshal(jj)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}

	_, err = w.Write(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}
}

func (s *Server) fetch(w http.ResponseWriter, r *http.Request) {
	ch := make(WorkRequest)
	s.fetchjobs <- ch
	j := <-ch

	data, err := json.Marshal(j)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}

	_, err = w.Write(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}
}

func (s *Server) push(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}

	j := NewJob()
	if err := json.Unmarshal(data, &j); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}
	s.pushjobs <- j
}
