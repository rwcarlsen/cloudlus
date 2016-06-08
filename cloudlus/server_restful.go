package cloudlus

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

func httperror(w http.ResponseWriter, msg string, code int) {
	http.Error(w, msg, http.StatusBadRequest)
	log.Print(msg)
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" || r.Method == "" {
		idstr := r.URL.Path[len("/api/v1/job/"):]

		jid, err := DecodeJobId(idstr)
		if err != nil {
			httperror(w, err.Error(), http.StatusBadRequest)
			return
		}

		j, err := s.Get(jid)
		if err != nil {
			httperror(w, err.Error(), http.StatusBadRequest)
			return
		}

		var data []byte
		if j.Done() {
			data, err = json.Marshal(j)
		} else {
			data, err = json.Marshal(NewJobStat(j))
		}

		if err != nil {
			httperror(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Add("Content-Disposition", fmt.Sprintf("filename=\"job-%v.json\"", j.Id))
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

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	s.ResetQueue()
}

func (s *Server) handleJobStat(w http.ResponseWriter, r *http.Request) {
	idstr := r.URL.Path[len("/api/v1/job-stat/"):]

	jid, err := DecodeJobId(idstr)
	if err != nil {
		httperror(w, err.Error(), http.StatusBadRequest)
		return
	}

	j, err := s.Get(jid)
	if err != nil {
		httperror(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := json.Marshal(NewJobStat(j))
	if err != nil {
		httperror(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Write(data)
}

func (s *Server) handleServerStats(w http.ResponseWriter, r *http.Request) {

  data, err := json.Marshal(s.Stats)
	if err != nil {
		httperror(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Write(data)
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

	jid := fmt.Sprintf("%v", j.Id)

	w.Header().Set("Location", r.Host+"/api/v1/job/"+jid)

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

func (s *Server) handleOutfiles(w http.ResponseWriter, r *http.Request) {
	idstr := r.URL.Path[len("/api/v1/job-outfiles/"):]
	jid, err := DecodeJobId(idstr)
	if err != nil {
		httperror(w, err.Error(), http.StatusBadRequest)
		return
	}

	if r.Method == "POST" {
		fname := outfileName(jid)
		f, err := os.Create(fname)
		if err != nil {
			msg := fmt.Sprintf("job %v outfile subission failed: %v", idstr, err)
			httperror(w, msg, http.StatusBadRequest)
			return
		}
		defer f.Close()

		_, err = io.Copy(f, r.Body)
		if err != nil {
			msg := fmt.Sprintf("job %v outfile subission failed: %v", idstr, err)
			httperror(w, msg, http.StatusBadRequest)
			return
		}
	} else if r.Method == "GET" {
		if j, err := s.Get(jid); err != nil {
			s.log.Printf("[REST] warning: /api/v1/job-outfiles/ request for job not in db (id=%v)\n", jid)
		} else if j.Status != StatusComplete {
			s.log.Printf("[REST] warning: /api/v1/job-outfiles/ request for potentially incomplete job")
		}

		w.Header().Add("Content-Disposition", fmt.Sprintf("filename=\"results-%v.zip\"", jid))

		f, err := os.Open(outfileName(jid))
		if err != nil {
			msg := fmt.Sprintf("[REST] error: job %v output files not found", jid)
			httperror(w, msg, http.StatusBadRequest)
			return
		}
		defer f.Close()

		_, err = io.Copy(w, f)
		if err != nil {
			httperror(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
