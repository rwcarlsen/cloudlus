package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"time"
)

var dashtmplstr = `
<table>
    <tr><th>Job ID</th><th>Status</th><th>Output</th></tr>

    {{ range $job := .}}
    <tr class="status-{{$job.Status}}">
        <td><a href="{{$job.Host}}/dashboard/infile/{{$job.Id}}">{{$job.Id}}</a></td>

        {{if eq $job.Status "complete"}}
        <td><a href="{{$job.Host}}/dashboard/output/{{$job.Id}}">{{$job.Status}}</a></td>
        {{else if eq $job.Status "failed"}}
        <td><a href="{{$job.Host}}/dashboard/output/{{$job.Id}}">{{$job.Status}}</a></td>
		{{else}}
        <td>{{$job.Status}}</td>
        {{end}}

        {{if eq $job.Status "complete"}}
        <td><a href="{{$job.Host}}/job/retrieve/{{$job.Id}}">Results</a></td>
        {{else}}
        <td></td>
        {{end}}
    </tr>
    {{ end }}
</table>
`
var tmpl = template.Must(template.New("dashtable").Parse(dashtmplstr))

type JobData struct {
	Id        string
	Status    string
	Submitted time.Time
	Host      string
}

type JobList []JobData

func (s JobList) Len() int      { return len(s) }
func (s JobList) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type ByTime struct{ JobList }

func (s ByTime) Less(i, j int) bool { return s.JobList[i].Submitted.Before(s.JobList[j].Submitted) }

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	jds := make(JobList, 0)
	for _, item := range s.alljobs.Items() {
		j := item.Value.(*Job)
		jd := JobData{
			Id:        fmt.Sprintf("%x", j.Id),
			Status:    j.Status,
			Submitted: j.Submitted,
			Host:      s.Host,
		}
		jds = append(jds, jd)
	}

	sort.Sort(ByTime{jds})

	// allow cross-domain ajax requests for the dashboard content
	w.Header().Add("Access-Control-Allow-Origin", "*")
	if err := tmpl.Execute(w, jds); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
	}
}

func (s *Server) dashmain(w http.ResponseWriter, r *http.Request) {

	home := `
<!DOCTYPE html>
<html class="no-js" lang="en-US">
<head>
    <title> Cyclus Run Dashboard </title>
    <script src="http://ajax.googleapis.com/ajax/libs/jquery/1.11.1/jquery.min.js"></script>
</head>
<body lang="en">

<div id="dashboard"></div>

<script>
	$('#dashboard').load("/dashboard");
</script>
</body>
</html>
`
	w.Write([]byte(home))
}

func (s *Server) dashboardInfile(w http.ResponseWriter, r *http.Request) {
	idstr := r.URL.Path[len("/dashboard/infile/"):]
	j, err := s.getjob(idstr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}

	w.Header().Add("Content-Type", "text/xml")
	w.Header().Add("Content-Disposition", fmt.Sprintf("filename=\"job-id-%x-infile.xml\"", j.Id))
	_, err = w.Write(j.Resources[DefaultInfile])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}
}

func (s *Server) dashboardOutput(w http.ResponseWriter, r *http.Request) {
	idstr := r.URL.Path[len("/dashboard/output/"):]
	j, err := s.getjob(idstr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}

	_, err = w.Write([]byte(j.Output))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}
}
