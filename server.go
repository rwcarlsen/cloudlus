package cloudlus

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/rpc"
	"os"
	"time"

	"github.com/rwcarlsen/gocache"
)

type JobRequest struct {
	Id   [16]byte
	Resp chan *Job
}

type JobSubmit struct {
	J      *Job
	Result chan *Job
}

type WorkRequest chan *Job

type Server struct {
	serv         *http.Server
	Host         string
	submitjobs   chan JobSubmit
	submitchans  map[[16]byte]chan *Job
	retrievejobs chan JobRequest
	pushjobs     chan *Job
	fetchjobs    chan WorkRequest
	statjobs     chan JobRequest
	queue        []*Job

	alljobs *cache.LRUCache
}

const MB = 1 << 20

func NewServer(addr string) *Server {
	s := &Server{
		submitjobs:   make(chan JobSubmit),
		submitchans:  map[[16]byte]chan *Job{},
		retrievejobs: make(chan JobRequest),
		statjobs:     make(chan JobRequest),
		pushjobs:     make(chan *Job),
		fetchjobs:    make(chan WorkRequest),
		alljobs:      cache.NewLRUCache(500 * MB),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.dashmain)
	mux.HandleFunc("/job/submit", s.submit)
	mux.HandleFunc("/job/submit-infile", s.submitInfile)
	mux.HandleFunc("/job/retrieve/", s.retrieve)
	mux.HandleFunc("/job/status/", s.status)
	mux.HandleFunc("/work/fetch", s.fetch)
	mux.HandleFunc("/work/push", s.push)
	mux.HandleFunc("/dashboard", s.dashboard)
	mux.HandleFunc("/dashboard/", s.dashboard)
	mux.HandleFunc("/dashboard/infile/", s.dashboardInfile)
	mux.HandleFunc("/dashboard/output/", s.dashboardOutput)
	mux.HandleFunc("/dashboard/default-infile", s.dashboardDefaultInfile)
	mux.Handle(rpc.DefaultRPCPath, rpc.DefaultServer)

	rpc.Register(s)

	s.serv = &http.Server{Addr: addr, Handler: mux}
	return s
}

type Beat struct {
	WorkerId [16]byte
	Busy     bool
	CurrJob  [16]byte
}

func (s *Server) Heartbeat(b Beat, unused *int) error {
	panic("not implemented")
}

// Submit j via rpc and block until complete returning the result job.
func (s *Server) Submit(j *Job, result **Job) error {
	ch := make(chan *Job)
	s.submitjobs <- JobSubmit{j, ch}
	*result = <-ch
	return nil
}

func (s *Server) Run() error {
	go s.dispatcher()
	return s.serv.ListenAndServe()
}

func (s *Server) dispatcher() {
	for {
		select {
		case js := <-s.submitjobs:
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
		case req := <-s.statjobs:
			if v, ok := s.alljobs.Get(req.Id); ok {
				req.Resp <- v.(*Job)
			} else {
				req.Resp <- nil
			}
		case j := <-s.pushjobs:
			if ch, ok := s.submitchans[j.Id]; ok {
				ch <- j
				delete(s.submitchans, j.Id)
			}
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

	j := &Job{}
	if err := json.Unmarshal(data, &j); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}

	s.submitjobs <- JobSubmit{j, nil}

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
	s.submitjobs <- JobSubmit{j, nil}

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

	jj := &Job{Id: j.Id, Status: j.Status}
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

	j := &Job{}
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

func writefile(fname string, buf *tar.Writer) error {
	f, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer f.Close()

	// make the tar header
	info, err := f.Stat()
	if err != nil {
		return err
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}

	// write the header and file data to the tar archive
	if err := buf.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := io.Copy(buf, f); err != nil {
		return err
	}
	return nil
}
