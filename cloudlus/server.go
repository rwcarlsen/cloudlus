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

var beatInterval = 30 * time.Second
var beatLimit = 3 * beatInterval
var beatCheckFreq = beatInterval / 3

// nfailban is the number of consecutive jobs after which a worker is
// permanently banned from receiving more jobs
var nfailban = 4

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
	queue        []*Job
	alljobs      *DB
	rpc          *RPC
	jobinfo      map[JobId]Beat
	running      map[JobId]*Job
	beat         chan Beat
	rpcaddr      string
	kill         chan struct{}
	Stats        *Stats
	rpcserv      *rpc.Server
	// workerFailures tracks consecutive failed jobs from workers
	workerFailures map[WorkerId]int
}

type Stats struct {
	Started time.Time
	// NBanned reports the number of workers that have been permanently banned
	// from running more jobs due to a poor track record.
	NBanned     int
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
		submitjobs:     make(chan jobSubmit),
		submitchans:    map[[16]byte]chan *Job{},
		retrievejobs:   make(chan jobRequest),
		pushjobs:       make(chan *Job),
		fetchjobs:      make(chan workRequest),
		jobinfo:        map[JobId]Beat{},
		running:        map[JobId]*Job{},
		beat:           make(chan Beat),
		reset:          make(chan struct{}),
		rpcaddr:        rpcaddr,
		log:            log.New(os.Stdout, "", log.LstdFlags),
		kill:           make(chan struct{}),
		CollectFreq:    defaultCollectFreq,
		Stats:          &Stats{},
		workerFailures: map[WorkerId]int{},
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
		s.queue = append(s.queue, j)
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
	mux.HandleFunc("/api/v1/server-stats/", s.handleServerStats)
	mux.HandleFunc("/dashboard", s.dashboard)
	mux.HandleFunc("/dashboard/", s.dashboard)
	mux.HandleFunc("/dashboard/infile/", s.dashboardInfile)
	mux.HandleFunc("/dashboard/output/", s.dashboardOutput)
	mux.HandleFunc("/dashboard/default-infile", s.dashboardDefaultInfile)

	s.rpc = &RPC{s}
	s.rpcserv = rpc.NewServer()
	s.rpcserv.Register(s.rpc)

	if httpaddr == rpcaddr {
		mux.Handle(rpc.DefaultRPCPath, s.rpcserv)
	} else {
		s.rpcserv.HandleHTTP(rpc.DefaultRPCPath, rpc.DefaultDebugPath)
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
				s.log.Printf("[INFO] purged %v old jobs from db, %v remain\n", npurged, nremain)
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
	j.Status = StatusQueued
	j.Submitted = time.Now()
	s.alljobs.Put(j)
	s.log.Printf("[SUBMIT] job %v\n", j.Id)

	if ch == nil {
		ch = make(chan *Job, 1)
	}
	s.submitjobs <- jobSubmit{j, ch}
	return ch
}

func (s *Server) Get(jid JobId) (*Job, error) {
	ch := make(chan *Job, 1)
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

func (s *Server) cleanQueue(delids ...JobId) {
	newqueue := make([]*Job, 0, len(s.queue))

	// remove jobs that don't have proper queued status
	for _, j := range s.queue {
		if j.Status == StatusQueued {
			newqueue = append(newqueue, j)
		} else {
			s.log.Printf("[GC] removed job with status %v from queue (id %v)\n", j.Status, j.Id)
		}
	}
	s.queue = newqueue
	newqueue = newqueue[:0]

	// remove named job ids from queue
	for _, j := range s.queue {
		skip := false
		for _, delid := range delids {
			if j.Id == delid {
				skip = true
				s.log.Printf("[GC] removed completed job from queue (id %v)\n", delid)
				break
			}
		}
		if !skip {
			newqueue = append(newqueue, j)
		}
	}
	s.queue = newqueue
}

// checkbeat checks for workers that have stopped responding and requeues their
// jobs to try again.
func (s *Server) checkbeat() {
	now := time.Now()
	for jid, b := range s.jobinfo {
		if now.Sub(b.Time) > beatLimit {
			j, ok := s.running[jid]
			if !ok {
				panic("server job 'running' and 'jobinfo' lists are out of sync")
			}

			delete(s.jobinfo, jid)
			delete(s.running, jid)
			s.log.Printf("[REQUEUE] job %v\n", jid)
			s.Stats.NRequeued++
			j.Status = StatusQueued
			s.queue = append([]*Job{j}, s.queue...)
			s.alljobs.Put(j)
		}
	}

	// also check to see if any submitchans are waiting on jobs to finnish
	// that we don't have record of them running in jobinfo
	for jid, ch := range s.submitchans {
		_, ok := s.jobinfo[jid]
		if !ok {
			// job is not currently running
			inqueue := false
			for _, qj := range s.queue {
				if jid == qj.Id {
					inqueue = true
					break
				}
			}

			if !inqueue {
				// job is also not queued
				s.log.Printf("[GC] removed conn waiting for dropped job %v\n", JobId(jid))
				s.Stats.NFailed++
				j, _ := s.alljobs.Get(jid)
				ch <- j
				close(ch)
				delete(s.submitchans, jid)
			}
		}
	}
}

func (s *Server) isBanned(wid WorkerId) bool {
	return s.workerFailures[wid] >= nfailban
}

func (s *Server) nBannedWorkers() int {
	n := 0
	for _, nfail := range s.workerFailures {
		if nfail >= nfailban {
			n++
		}
	}
	return n
}

func (s *Server) dispatcher() {
	beatcheck := time.NewTicker(beatCheckFreq)
	defer beatcheck.Stop()

	for {
		s.Stats.CurrQueued = len(s.queue)
		s.Stats.CurrRunning = len(s.jobinfo)
		s.Stats.NBanned = s.nBannedWorkers()

		select {
		case <-beatcheck.C:
			s.checkbeat()
		case <-s.reset:
			s.log.Printf("[RESET] removed %v queued jobs\n", len(s.queue))
			for _, j := range s.queue {
				j.Status = StatusFailed
				j.Stderr += "\nkilled by server reset\n"
				s.finnishJob(j)
			}
			s.queue = s.queue[:0]
		case <-s.kill:
			return
		case js := <-s.submitjobs:
			s.queue = append(s.queue, js.J)
			s.Stats.NSubmitted++
			if js.Result != nil {
				s.submitchans[js.J.Id] = js.Result
			}
		case req := <-s.retrievejobs:
			if j, ok := s.running[req.Id]; ok {
				s.log.Printf("[RETRIEVE] from run list job %v\n", j.Id)
				req.Resp <- j
			} else if j, err := s.alljobs.Get(req.Id); err == nil {
				s.log.Printf("[RETRIEVE] from db job %v\n", j.Id)
				req.Resp <- j
			} else {
				s.log.Printf("[RETRIEVE] error: job %v not found\n", req.Id)
				req.Resp <- nil
			}
		case j := <-s.pushjobs:
			if j.Status == StatusComplete {
				s.workerFailures[j.WorkerId] = 0
			} else if j.Status == StatusFailed {
				s.workerFailures[j.WorkerId]++
			}

			s.log.Printf("[PUSH] job %v\n", j.Id)
			if jj, ok := s.running[j.Id]; ok {
				// workers nilify the Infiles to reduce network traffic
				// we want to re-add the locally stored infiles back to keep
				// job data complete.
				j.Infiles = jj.Infiles
			} else {
				s.log.Printf("[PUSH] error: push for job not running (id=%v)\n", j.Id)
			}
			s.finnishJob(j)
		case req := <-s.fetchjobs:
			if s.isBanned(req.WorkerId) {
				s.log.Printf("[FETCH] no work for banned worker %v)\n", req.WorkerId)
				req.Ch <- nil
				continue
			} else if len(s.queue) == 0 {
				s.log.Printf("[FETCH] no work in queue (worker %v)\n", req.WorkerId)
				req.Ch <- nil
				continue
			}

			j := s.queue[0]
			s.queue = append([]*Job{}, s.queue[1:]...)
			s.log.Printf("[FETCH] job %v (worker %v)\n", j.Id, req.WorkerId)
			s.jobinfo[j.Id] = NewBeat(req.WorkerId, j.Id)
			s.running[j.Id] = j
			j.Fetched = time.Now()
			j.Status = StatusRunning
			s.alljobs.Put(j)
			req.Ch <- j
		case b := <-s.beat:
			oldb, ok := s.jobinfo[b.JobId]
			if !ok {
				// job was completed by another worker already
				s.log.Printf("[BEAT] sending kill signal: job %v already completed by another worker\n", b.JobId)
				b.kill <- true
				continue
			} else if oldb.WorkerId != b.WorkerId {
				// job has been reassigned to another worker
				s.log.Printf("[BEAT] sending kill signal: job %v was rescheduled to another worker\n", b.JobId)
				b.kill <- true
				continue
			}

			s.jobinfo[b.JobId] = b

			j, ok := s.running[b.JobId]
			if !ok {
				// don't kill the job because maybe the db just hasn't synced
				// fully yet.
				b.kill <- true
				s.log.Printf("[BEAT] sending kill signal: job %v not listed as running\n", b.JobId)
				continue
			}

			if j.Fetched.IsZero() {
				s.log.Printf("[BEAT] job %v (worker %v), ??? left of %v\n", b.JobId, b.WorkerId, j.Timeout)
			} else {
				s.log.Printf("[BEAT] job %v (worker %v), %v left of %v\n", b.JobId, b.WorkerId, j.Timeout-time.Now().Sub(j.Fetched), j.Timeout)
			}

			if time.Now().Sub(j.Fetched) > j.Timeout && j.Timeout > 0 && !j.Fetched.IsZero() {
				j.Status = StatusFailed
				s.finnishJob(j)
				s.log.Printf("[BEAT] sending kill signal: job %v timed out (worker %v)\n", b.JobId, b.WorkerId)
				b.kill <- true
			}
			b.kill <- false
		}
	}
}

func (s *Server) finnishJob(j *Job) {
	if j == nil {
		return
	}

	// put this first to get data in db as soon as possible.
	s.alljobs.Put(j)

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
	delete(s.running, j.Id)
	s.cleanQueue(j.Id)
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
