package cloudlus

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
)

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

func (s *Server) handleJobStat(w http.ResponseWriter, r *http.Request) {
	idstr := r.URL.Path[len("/api/v1/job-stat/"):]
	j, err := s.getjob(idstr)
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

	w.Header().Add("Content-Disposition", fmt.Sprintf("filename=\"results-%v.zip\"", j.Id))

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
