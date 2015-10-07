package cloudlus

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"os"
	"time"
)

const MB = 1 << 20
const cachelimit = 400 * MB
const dblimit = 7000 * MB

var nojoberr = errors.New("no jobs available to run")

const defaultdbpath = "./jobdb"

// defaultCollectFreq if the duration between old job purging from db.
var defaultCollectFreq = 2 * time.Minute

const beatInterval = 15 * time.Second
const beatLimit = 2 * beatInterval
const beatCheckFreq = beatInterval / 3

type Server struct {
	log          *log.Logger
	serv         *http.Server
	Host         string
	CollectFreq  time.Duration
	submitjobs   chan jobSubmit
	submitchans  map[[16]byte]chan *Job
	retrievejobs chan jobRequest
	pushjobs     chan *Job
	fetchjobs    chan workRequest
	reset        chan struct{}
	queue        []JobId
	alljobs      *DB
	rpc          *RPC
	jobinfo      map[JobId]Beat // map[Worker]Job
	beat         chan Beat
	rpcaddr      string
	kill         chan struct{}
	Stats        *Stats
}

type Stats struct {
	Started     time.Time
	NSubmitted  int
	NCompleted  int
	NFailed     int
	NPurged     int
	NRequeued   int
	CurrQueued  int
	CurrRunning int
	TotJobTime  time.Duration
	AvgJobTime  time.Duration
	MinJobTime  time.Duration
	MaxJobTime  time.Duration
	TotCmdTime  time.Duration
	AvgCmdTime  time.Duration
	MinCmdTime  time.Duration
	MaxCmdTime  time.Duration
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
		reset:        make(chan struct{}),
		rpcaddr:      rpcaddr,
		log:          log.New(os.Stdout, "", log.LstdFlags),
		kill:         make(chan struct{}),
		CollectFreq:  defaultCollectFreq,
		Stats:        &Stats{},
	}

	var err error
	if db == nil {
		db, err = NewDB(defaultdbpath, dblimit)
		if err != nil {
			panic(err)
		}
	}
	s.alljobs = db
	q, err := db.Current()
	if err != nil {
		panic(err)
	}
	for _, j := range q {
		s.queue = append(s.queue, j.Id)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.dashmain)
	mux.HandleFunc("/reset", s.dashreset)
	mux.HandleFunc("/reset/", s.dashreset)
	mux.HandleFunc("/api/v1/reset-queue", s.handleReset)
	mux.HandleFunc("/api/v1/job", s.handleJob)
	mux.HandleFunc("/api/v1/job/", s.handleJob)
	mux.HandleFunc("/api/v1/job-stat/", s.handleJobStat)
	mux.HandleFunc("/api/v1/job-infile", s.handleSubmitInfile)
	mux.HandleFunc("/api/v1/job-outfiles/", s.handleOutfiles)
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
	s.Stats.Started = time.Now()
	go s.dispatcher()
	go func() {
		for {
			select {
			case <-s.kill:
				return
			default:
				npurged, nremain, err := s.alljobs.GC()
				s.Stats.NPurged += npurged
				if err != nil {
					s.log.Print(err)
				}
				s.log.Printf("[INFO] purged %v old jobs from db, %v remain", npurged, nremain)
			}
			<-time.After(s.CollectFreq)
		}
	}()

	if s.rpcaddr != s.serv.Addr {
		go func() {
			if err := http.ListenAndServe(s.rpcaddr, nil); err != nil {
				log.Fatal(err)
			}
		}()
	}
	return s.serv.ListenAndServe()
}

func (s *Server) Close() error {
	close(s.kill)
	return s.alljobs.Close()
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

// ResetQueue removes all jobs from the queue permanently.
func (s *Server) ResetQueue() {
	s.reset <- struct{}{}
}

// checkbeat checks for workers that have stopped responding and requeues their
// jobs to try again.
func (s *Server) checkbeat() {
	now := time.Now()
	for jid, b := range s.jobinfo {
		if now.Sub(b.Time) > beatLimit {
			j, err := s.alljobs.Get(jid)
			delete(s.jobinfo, jid)
			if err != nil {
				log.Printf("cannot find job %v for reassignment", jid)
				continue
			}

			s.log.Printf("[REQUEUE] job %v\n", jid)
			s.Stats.NRequeued++
			j.Status = StatusQueued
			s.queue = append([]JobId{j.Id}, s.queue...)
			s.alljobs.Put(j)
		}
	}
}

func (s *Server) dispatcher() {
	beatcheck := time.NewTicker(beatCheckFreq)
	defer beatcheck.Stop()

	for {
		s.Stats.CurrQueued = len(s.queue)
		s.Stats.CurrRunning = len(s.jobinfo)

		select {
		case <-beatcheck.C:
			s.checkbeat()
		case <-s.reset:
			s.log.Printf("[RESET] removed %v queued jobs\n", len(s.queue))
			for _, jid := range s.queue {
				j, err := s.alljobs.Get(jid)
				if err == nil {
					j.Status = StatusFailed
					j.Stderr += "\nkilled by server reset\n"
				}
				s.finnishJob(j)
			}
			s.queue = s.queue[:0]
		case <-s.kill:
			return
		case js := <-s.submitjobs:
			s.Stats.NSubmitted++
			s.log.Printf("[SUBMIT] job %v\n", js.J.Id)
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
				s.log.Printf("[RETRIEVE] job %v\n", j.Id)
				req.Resp <- j
			} else {
				req.Resp <- nil
			}
		case j := <-s.pushjobs:
			s.log.Printf("[PUSH] job %v\n", j.Id)
			if jj, err := s.alljobs.Get(j.Id); err == nil {
				// workers nilify the Infiles to reduce network traffic
				// we want to re-add the locally stored infiles back to keep
				// job data complete.
				j.Infiles = jj.Infiles
			}
			s.finnishJob(j)
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
				s.log.Printf("[FETCH] no work in queue (worker %v)\n", req.WorkerId)
				s.queue = nil
			} else {
				s.log.Printf("[FETCH] job %v (worker %v)\n", j.Id, req.WorkerId)
				s.jobinfo[j.Id] = NewBeat(req.WorkerId, j.Id)
				j.Fetched = time.Now()
				j.Status = StatusRunning
				s.alljobs.Put(j)
			}

			req.Ch <- j
		case b := <-s.beat:
			var err error
			oldb, ok := s.jobinfo[b.JobId]
			kill := false
			var j *Job
			if ok {
				j, err = s.alljobs.Get(b.JobId)
				if err != nil {
					s.log.Printf("[BEAT] error - job %v not found in db\n", b.JobId)
					continue
				}
				kill = time.Now().Sub(j.Fetched) > j.Timeout
			} else {
				// job was completed by another worker already
				kill = true
			}

			// make sure that this job hasn't been reassigned to another worker
			if oldb.WorkerId == b.WorkerId {
				s.log.Printf("[BEAT] job %v (worker %v)\n", b.JobId, b.WorkerId)
				s.jobinfo[b.JobId] = b
			} else {
				// job has been reassigned
				kill = true
			}

			if kill && j != nil {
				j.Status = StatusFailed
				s.finnishJob(j)
			}
			b.kill <- kill
		}
	}
}

func (s *Server) finnishJob(j *Job) {
	if j.Status == StatusFailed {
		s.Stats.NFailed++
	} else if j.Status == StatusComplete {
		s.Stats.NCompleted++

		jobtime := j.Finished.Sub(j.Started)
		s.Stats.TotJobTime += jobtime
		s.Stats.AvgJobTime = s.Stats.TotJobTime / time.Duration(s.Stats.NCompleted)
		if s.Stats.MinJobTime == 0 || jobtime < s.Stats.MinJobTime {
			s.Stats.MinJobTime = jobtime
		}
		if s.Stats.MaxJobTime == 0 || jobtime > s.Stats.MaxJobTime {
			s.Stats.MaxJobTime = jobtime
		}

		s.Stats.TotCmdTime += j.CmdDur
		s.Stats.AvgCmdTime = s.Stats.TotCmdTime / time.Duration(s.Stats.NCompleted)
		if s.Stats.MinCmdTime == 0 || j.CmdDur < s.Stats.MinCmdTime {
			s.Stats.MinCmdTime = j.CmdDur
		}
		if s.Stats.MaxCmdTime == 0 || j.CmdDur > s.Stats.MaxCmdTime {
			s.Stats.MaxCmdTime = j.CmdDur
		}
	}

	if ch, ok := s.submitchans[j.Id]; ok {
		ch <- j
		close(ch)
		delete(s.submitchans, j.Id)
	}

	delete(s.jobinfo, j.Id)
	s.alljobs.Put(j)
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
