package cloudlus

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/rpc"
	"time"

	"github.com/rwcarlsen/gocache"
)

const MB = 1 << 20

type WorkerId [16]byte
type JobId [16]byte

const beatInterval = 60 * time.Second

type Server struct {
	serv         *http.Server
	Host         string
	submitjobs   chan jobSubmit
	submitchans  map[[16]byte]chan *Job
	retrievejobs chan jobRequest
	pushjobs     chan *Job
	fetchjobs    chan workRequest
	queue        []*Job
	alljobs      *cache.LRUCache
	rpc          *RPC
	jobinfo      map[JobId]Beat // map[Worker]Job
	beat         chan Beat
	rpcaddr      string
}

func NewServer(httpaddr, rpcaddr string) *Server {
	s := &Server{
		submitjobs:   make(chan jobSubmit),
		submitchans:  map[[16]byte]chan *Job{},
		retrievejobs: make(chan jobRequest),
		pushjobs:     make(chan *Job),
		fetchjobs:    make(chan workRequest),
		alljobs:      cache.NewLRUCache(500 * MB),
		jobinfo:      map[JobId]Beat{},
		beat:         make(chan Beat),
		rpcaddr:      rpcaddr,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.dashmain)
	mux.HandleFunc("/api/v1/job/", s.handleJob)
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
		return nil, fmt.Errorf("unknown job id %x", j)
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
					v, ok := s.alljobs.Get(jid)
					if !ok {
						log.Printf("cannot find job %v for reassignment", jid)
					} else {
						fmt.Printf("requeuing job %v\n", jid)
						j := v.(*Job)
						j.Status = StatusQueued
						s.queue = append([]*Job{j}, s.queue...)
						delete(s.jobinfo, jid)
					}
				}
			}
		default: // don't block
		}

		select {
		case js := <-s.submitjobs:
			fmt.Printf("job %x submitted\n", js.J.Id)
			j := js.J
			if js.Result != nil {
				s.submitchans[j.Id] = js.Result
			}
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
		case j := <-s.pushjobs:
			fmt.Printf("job %x pushed by worker\n", j.Id)
			if v, ok := s.alljobs.Get(j.Id); ok {
				// workers nilify the Infiles to reduce network traffic
				// we want to re-add the locally stored infiles back to keep
				// job data complete.
				j.Infiles = v.(*Job).Infiles
			}

			if ch, ok := s.submitchans[j.Id]; ok {
				ch <- j
				close(ch)
				delete(s.submitchans, j.Id)
			}
			delete(s.jobinfo, j.Id)
			s.alljobs.Set(j.Id, j)
		case req := <-s.fetchjobs:
			var j *Job

			// skip jobs that were finished by a worker reassigned *from*
			for i, job := range s.queue {
				v, ok := s.alljobs.Get(job.Id)
				if ok && v.(*Job).Status == StatusQueued {
					j = v.(*Job)
					s.queue = s.queue[i+1:]
					break
				}
			}

			if j == nil {
				s.queue = nil
			} else {
				fmt.Printf("job %x fetched by worker\n", j.Id)
				s.jobinfo[j.Id] = NewBeat(req.WorkerId, j.Id)
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

func httperror(w http.ResponseWriter, msg string, code int) {
	http.Error(w, msg, http.StatusBadRequest)
	log.Print(msg)
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" || r.Method == "" {
		idstr := r.URL.Path[len("/api/v1/job/"):]
		j, err := s.getjob(idstr)
		if err != nil {
			httperror(w, err.Error(), http.StatusBadRequest)
			return
		}

		var data []byte
		if j.Status == StatusComplete || j.Status == StatusFailed {
			data, err = json.Marshal(j)
		} else {
			// only send in+out files if
			data, err = json.Marshal(NewJobStat(j))
		}

		if err != nil {
			httperror(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Add("Content-Disposition", fmt.Sprintf("filename=\"job-%x.json\"", j.Id))
		w.Write(data)
	} else if r.Method == "POST" {
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			httperror(w, err.Error(), http.StatusBadRequest)
			return
		}

		j := &Job{}
		if err := json.Unmarshal(data, &j); err != nil {
			httperror(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.createJob(r, w, j)
	}
}

func (s *Server) createJob(r *http.Request, w http.ResponseWriter, j *Job) {
	s.Start(j, nil)

	j, err := s.Get(j.Id)
	if err != nil {
		httperror(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := json.Marshal(j)
	if err != nil {
		httperror(w, err.Error(), http.StatusBadRequest)
		return
	}

	jid := fmt.Sprintf("%x", j.Id)

	w.Header().Set("Location", r.Host+"/api/v1/job/"+jid)
	// allow cross-domain ajax requests for job submission
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusCreated)
	w.Write(data)
}

func (s *Server) handleSubmitInfile(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		httperror(w, err.Error(), http.StatusBadRequest)
		return
	}

	j := NewJobDefault(data)
	s.createJob(r, w, j)
}

func (s *Server) handleRetrieveZip(w http.ResponseWriter, r *http.Request) {
	idstr := r.URL.Path[len("/api/v1/job-outfiles/"):]
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

	w.Header().Add("Content-Disposition", fmt.Sprintf("filename=\"results-%x.zip\"", j.Id))

	// return single zip file
	var buf bytes.Buffer
	zipbuf := zip.NewWriter(&buf)
	for _, fd := range j.Outfiles {
		f, err := zipbuf.Create(fd.Name)
		if err != nil {
			log.Print(err)
			return
		}
		_, err = f.Write(fd.Data)
		if err != nil {
			log.Print(err)
			return
		}
	}
	err = zipbuf.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}

	_, err = io.Copy(w, &buf)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}
}

func (s *Server) getjob(idstr string) (*Job, error) {
	uid, err := hex.DecodeString(idstr)
	if err != nil {
		return nil, fmt.Errorf("malformed job id %v", idstr)
	}

	var id JobId
	copy(id[:], uid)
	return s.Get(id)
}

type RPC struct {
	s *Server
}

func (r *RPC) Heartbeat(b Beat, unused *int) error {
	r.s.beat <- b
	return nil
}

// Submit j via rpc and block until complete returning the result job.
func (r *RPC) Submit(j *Job, result **Job) error {
	*result = r.s.Run(j)
	return nil
}

func (r *RPC) Retrieve(j JobId, result **Job) error {
	var err error
	*result, err = r.s.Get(j)
	if err != nil {
		return err
	}
	return nil
}

var nojoberr = errors.New("no jobs available to run")

func (r *RPC) Fetch(wid [16]byte, j **Job) error {
	fmt.Printf("got work request from worker %x\n", wid)
	req := workRequest{wid, make(chan *Job)}
	r.s.fetchjobs <- req
	*j = <-req.Ch
	if *j == nil {
		return nojoberr
	}

	return nil
}

func (r *RPC) Push(j *Job, unused *int) error {
	fmt.Printf("received job %x back from worker %x\n", j.Id, j.WorkerId)
	r.s.pushjobs <- j
	return nil
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
type Beat struct {
	Time     time.Time
	WorkerId WorkerId
	JobId    JobId
}

func NewBeat(w WorkerId, j JobId) Beat {
	return Beat{Time: time.Now(), WorkerId: w, JobId: j}
}
