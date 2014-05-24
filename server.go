package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/rwcarlsen/gocache"
)

type JobRequest struct {
	Id   [16]byte
	Resp chan *Job
}

type WorkRequest chan *Job

type Server struct {
	Host         string
	submitjobs   chan *Job
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
		submitjobs:   make(chan *Job),
		retrievejobs: make(chan JobRequest),
		statjobs:     make(chan JobRequest),
		pushjobs:     make(chan *Job),
		fetchjobs:    make(chan WorkRequest),
		alljobs:      cache.NewLRUCache(250 * MB),
	}
}

func (s *Server) ListenAndServe(addr string) error {
	http.HandleFunc("/", s.dashmain)
	http.HandleFunc("/job/submit", s.submit)
	http.HandleFunc("/job/submit-infile", s.submitInfile)
	http.HandleFunc("/job/retrieve/", s.retrieve)
	http.HandleFunc("/job/status/", s.status)
	http.HandleFunc("/work/fetch", s.fetch)
	http.HandleFunc("/work/push", s.push)
	http.HandleFunc("/dashboard", s.dashboard)
	http.HandleFunc("/dashboard/infile/", s.dashboardInfile)
	http.HandleFunc("/dashboard/output/", s.dashboardOutput)
	http.HandleFunc("/dashboard/default-infile", s.dashboardDefaultInfile)

	go s.dispatcher()

	return http.ListenAndServe(addr, nil)
}

func (s *Server) dispatcher() {
	for {
		select {
		case j := <-s.submitjobs:
			j.Status = StatusQueued
			j.Submitted = time.Now()
			s.queue = append(s.queue, j)
			s.alljobs.Set(j.Id, j)
		case req := <-s.retrievejobs:
			if v, ok := s.alljobs.Get(req.Id); ok {
				req.Resp <- v.(*Job)
			} else {
				req.Resp <- nil
			}
		case req := <-s.statjobs:
			if v, ok := s.alljobs.Get(req.Id); ok {
				req.Resp <- v.(*Job)
			} else {
				req.Resp <- nil
			}
		case j := <-s.pushjobs:
			s.alljobs.Set(j.Id, j)
		case req := <-s.fetchjobs:
			var j *Job
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

	s.submitjobs <- j

	// allow cross-domain ajax requests for job submission
	w.Header().Add("Access-Control-Allow-Origin", "*")
	fmt.Fprintf(w, "%x", j.Id)
}

func (s *Server) submitInfile(w http.ResponseWriter, r *http.Request) {
	// TODO add shortcut code to check for cached db files if this infile has
	// already been run
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}

	j := NewJobDefault(data)
	s.submitjobs <- j

	// allow cross-domain ajax requests for job submission
	w.Header().Add("Access-Control-Allow-Origin", "*")
	fmt.Fprintf(w, "%x", j.Id)
}

func (s *Server) retrieve(w http.ResponseWriter, r *http.Request) {
	idstr := r.URL.Path[len("/job/retrieve/"):]
	j, err := s.getjob(idstr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	} else if j.Status != StatusComplete {
		msg := fmt.Sprintf("job %v status: %v", idstr, j.Status)
		http.Error(w, msg, http.StatusBadRequest)
		log.Print(msg)
		return
	}

	w.Header().Add("Content-Disposition", fmt.Sprintf("filename=\"results-id-%x.tar\"", j.Id))
	_, err = w.Write(j.ResultData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}
}

func (s *Server) getjob(idstr string) (*Job, error) {
	id, err := convid(idstr)
	if err != nil {
		return nil, fmt.Errorf("malformed job id %v", idstr)
	}

	ch := make(chan *Job)
	s.statjobs <- JobRequest{Id: id, Resp: ch}
	j := <-ch
	if j == nil {
		return nil, fmt.Errorf("unknown job id %v", idstr)
	}
	return j, nil
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	idstr := r.URL.Path[len("/job/status/"):]
	j, err := s.getjob(idstr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
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

func convid(s string) ([16]byte, error) {
	uid, err := hex.DecodeString(s)
	if err != nil {
		return [16]byte{}, err
	}
	var id [16]byte
	copy(id[:], uid)
	return id, nil
}
