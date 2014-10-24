package cloudlus

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"time"
)

const MB = 1 << 20
const cachelimit = 400 * MB
const dblimit = 7000 * MB

var nojoberr = errors.New("no jobs available to run")

const defaultdbpath = "./jobdb"

const beatInterval = 60 * time.Second

type Server struct {
	serv         *http.Server
	Host         string
	submitjobs   chan jobSubmit
	submitchans  map[[16]byte]chan *Job
	retrievejobs chan jobRequest
	pushjobs     chan *Job
	fetchjobs    chan workRequest
	queue        []JobId
	alljobs      *DB
	rpc          *RPC
	jobinfo      map[JobId]Beat // map[Worker]Job
	beat         chan Beat
	rpcaddr      string
}

// TODO: Make worker RPC serving separate from submitter RPC interface serving
// to allow for local listening only for job submission for more security.

func NewServer(httpaddr, rpcaddr string, db *DB) *Server {
	s := &Server{
		submitjobs:   make(chan jobSubmit),
		submitchans:  map[[16]byte]chan *Job{},
		retrievejobs: make(chan jobRequest),
		pushjobs:     make(chan *Job),
		fetchjobs:    make(chan workRequest),
		jobinfo:      map[JobId]Beat{},
		beat:         make(chan Beat),
		rpcaddr:      rpcaddr,
	}

	var err error
	if db == nil {
		db, err = NewDB(defaultdbpath, cachelimit, dblimit)
		if err != nil {
			panic(err)
		}
	}
	s.alljobs = db
	queue, err := db.LoadQueue()
	if err != nil {
		panic(err)
	}
	s.queue = queue

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.dashmain)
	mux.HandleFunc("/api/v1/job", s.handleJob)
	mux.HandleFunc("/api/v1/job/", s.handleJob)
	mux.HandleFunc("/api/v1/job-stat/", s.handleJobStat)
	mux.HandleFunc("/api/v1/job-infile", s.handleSubmitInfile)
	mux.HandleFunc("/api/v1/job-outfiles/", s.handleRetrieveZip)
	mux.HandleFunc("/dashboard", s.dashboard)
	mux.HandleFunc("/dashboard/", s.dashboard)
	mux.HandleFunc("/dashboard/infile/", s.dashboardInfile)
	mux.HandleFunc("/dashboard/output/", s.dashboardOutput)
	mux.HandleFunc("/dashboard/default-infile", s.dashboardDefaultInfile)

	s.rpc = &RPC{s}
	rpc.Register(s.rpc)

	if httpaddr == rpcaddr {
		mux.Handle(rpc.DefaultRPCPath, rpc.DefaultServer)
	} else {
		rpc.HandleHTTP()
	}

	s.serv = &http.Server{Addr: httpaddr, Handler: mux}
	return s
}

func (s *Server) ListenAndServe() error {
	go s.dispatcher()

	if s.rpcaddr != s.serv.Addr {
		go func() {
			if err := http.ListenAndServe(s.rpcaddr, nil); err != nil {
				log.Fatal(err)
			}
		}()
	}
	return s.serv.ListenAndServe()
}

func (s *Server) Run(j *Job) *Job {
	ch := s.Start(j, nil)
	return <-ch
}

func (s *Server) Start(j *Job, ch chan *Job) chan *Job {
	if ch == nil {
		ch = make(chan *Job, 1)
	}
	s.submitjobs <- jobSubmit{j, ch}
	return ch
}

func (s *Server) Get(jid JobId) (*Job, error) {
	ch := make(chan *Job)
	s.retrievejobs <- jobRequest{Id: jid, Resp: ch}
	j := <-ch
	if j == nil {
		return nil, fmt.Errorf("unknown job id %v", jid)
	}
	return j, nil
}

func (s *Server) dispatcher() {
	beatcheck := time.NewTicker(beatInterval)
	defer beatcheck.Stop()

	for {
		// check for workers that have stopped responding and requeue their
		// jobs to try again.
		select {
		case <-beatcheck.C:
			now := time.Now()
			for jid, b := range s.jobinfo {
				if now.Sub(b.Time) > 2*beatInterval {
					j, err := s.alljobs.Get(jid)
					if err != nil {
						log.Printf("cannot find job %v for reassignment", jid)
					} else {
						fmt.Printf("requeuing job %v\n", jid)
						j.Status = StatusQueued
						s.queue = append([]JobId{j.Id}, s.queue...)
						delete(s.jobinfo, jid)
						s.alljobs.Put(j)
					}
				}
			}
		default: // don't block
		}

		select {
		case js := <-s.submitjobs:
			fmt.Printf("job %v submitted\n", js.J.Id)
			j := js.J
			if js.Result != nil {
				s.submitchans[j.Id] = js.Result
			}
			j.Status = StatusQueued
			j.Submitted = time.Now()
			s.queue = append(s.queue, j.Id)

			s.alljobs.Put(j)
		case req := <-s.retrievejobs:
			if j, err := s.alljobs.Get(req.Id); err == nil {
				req.Resp <- j
			} else {
				req.Resp <- nil
			}
		case j := <-s.pushjobs:
			fmt.Printf("job %v pushed by worker\n", j.Id)
			if jj, err := s.alljobs.Get(j.Id); err == nil {
				// workers nilify the Infiles to reduce network traffic
				// we want to re-add the locally stored infiles back to keep
				// job data complete.
				j.Infiles = jj.Infiles
			}

			if ch, ok := s.submitchans[j.Id]; ok {
				ch <- j
				close(ch)
				delete(s.submitchans, j.Id)
			}
			delete(s.jobinfo, j.Id)
			s.alljobs.Put(j)
		case req := <-s.fetchjobs:
			var j *Job
			var err error

			// skip jobs that were finished by a worker reassigned *from*
			for i, id := range s.queue {
				j, err = s.alljobs.Get(id)
				if err == nil && j.Status == StatusQueued {
					s.queue = s.queue[i+1:]
					break
				}
			}

			if j == nil {
				s.queue = nil
			} else {
				fmt.Printf("job %v fetched by worker\n", j.Id)
				s.jobinfo[j.Id] = NewBeat(req.WorkerId, j.Id)
				j.Status = StatusRunning
				s.alljobs.Put(j)
			}

			req.Ch <- j
		case b := <-s.beat:
			// make sure that this job hasn't been reassigned to another worker
			oldb := s.jobinfo[b.JobId]
			if oldb.WorkerId == b.WorkerId {
				s.jobinfo[b.JobId] = b
			}
		}
	}
}

type jobRequest struct {
	Id   JobId
	Resp chan *Job
}

type jobSubmit struct {
	J      *Job
	Result chan *Job
}

type workRequest struct {
	WorkerId WorkerId
	Ch       chan *Job
}
