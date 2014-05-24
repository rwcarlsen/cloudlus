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
var hometmpl = template.Must(template.New("home").Parse(home))

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

func (s ByTime) Less(i, j int) bool { return s.JobList[i].Submitted.After(s.JobList[j].Submitted) }

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
	w.Header().Add("Access-Control-Allow-Origin", "*")
	err := hometmpl.Execute(w, s.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
	}
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

func (s *Server) dashboardDefaultInfile(w http.ResponseWriter, r *http.Request) {
	// allow cross-domain ajax requests for the dashboard content
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Content-Type", "text/plain")
	_, err := w.Write([]byte(defaultInfile))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}
}

const defaultInfile = `<?xml version="1.0"?>
<!-- 1 Source, 1 Sink -->

<simulation>
  <control>
    <duration>100</duration>
    <startmonth>1</startmonth>
    <startyear>2000</startyear>
    <decay>0</decay>
  </control>

  <commodity>
    <name>commodity</name>
  </commodity>

  <facility>
    <name>Source</name>
    <module>
      <lib>agents</lib>
      <agent>Source</agent>
    </module>
    <agent>
      <Source>
        <commod>commodity</commod>
        <recipe_name>commod_recipe</recipe_name>
        <capacity>1.00</capacity>
      </Source>
    </agent>
  </facility>

  <facility>
    <name>Sink</name>
    <module>
      <lib>agents</lib>
      <agent>Sink</agent>
    </module>
    <agent>
      <Sink>
        <in_commods>
          <val>commodity</val>
        </in_commods>
        <capacity>1.00</capacity>
      </Sink>
    </agent>
  </facility>

  <region>
    <name>SingleRegion</name>
    <module>
      <lib>agents</lib>
      <agent>NullRegion</agent>
    </module>
    <allowedfacility>Source</allowedfacility>
    <allowedfacility>Sink</allowedfacility>
    <agent>
      <NullRegion/>
    </agent>
    <institution>
      <name>SingleInstitution</name>
      <module>
        <lib>agents</lib>
        <agent>NullInst</agent>
      </module>
      <availableprototype>Source</availableprototype>
      <availableprototype>Sink</availableprototype>
      <initialfacilitylist>
        <entry>
          <prototype>Source</prototype>
          <number>1</number>
        </entry>
        <entry>
          <prototype>Sink</prototype>
          <number>1</number>
        </entry>
      </initialfacilitylist>
      <agent>
        <NullInst/>
      </agent>
    </institution>
  </region>

  <recipe>
    <name>commod_recipe</name>
    <basis>mass</basis>
    <nuclide>
      <id>010010000</id>
      <comp>1</comp>
    </nuclide>
  </recipe>

</simulation>
`

const home = `
<!DOCTYPE html>
<html class="no-js" lang="en-US">
<head>
    <title> Cyclus Run Dashboard </title>
    <script src="http://ajax.googleapis.com/ajax/libs/jquery/1.11.1/jquery.min.js"></script>
	<style>
		#dashboard table {
			width:80%;
			border-color:#a9a9a9;
			color:#333333;
			border-collapse:collapse;
			margin:auto;
			border-width:1px;
			text-align:center;
		}
		#dashboard th {
			padding:4px;
			border-style:solid;
			border-color:#a9a9a9;
			border-width:1px;
			background-color:#b8b8b8;
			text-align:left;
		}
		#dashboard tr {
			background-color:#ffffff;
			text-align:center;
		}
		#dashboard td {
			padding:4px;
			border-color:#a9a9a9;
			border-style:solid;
			border-width:1px;
			text-align:center;
		}
		#dashboard tr.status-complete {
			background-color:#E0FFC2;
		}
		#dashboard tr.status-queued {
			background-color:#FFFFCC;
		}
		#dashboard tr.status-running {
			background-color:#D1F0FF;
		}
		#dashboard tr.status-failed {
			background-color:#F0C2B2;
		}

		#infile-form {
			width:80%;
			margin:auto;
		}
		#infile-form textarea {
			width: 100%;
		}
	</style>

</head>
<body lang="en">

    <br>
    <div id="infile-form">
    Cyclus input file: <br>
    <textarea id="infile-box" name="infile" rows=20></textarea>
    <br><button onclick="submitJob()">Submit</button><label>    Job Id: </label><label id="jobid"></label>
    </div>

    <br>
    <div id="dashboard"></div>
    <br>

    <script> 
        var server = "{{.}}"

        function submitJob() {
            var text = $('#infile-box').val();
            $.post(server + "/job/submit-infile", text, function(data) {
                $('#jobid').text(data);
                $('#dashboard').load(server + "/dashboard");
            })
        }
        function loadDash() {
            $('#dashboard').load(server + "/dashboard", function() {
                setTimeout("loadDash()", 30000)
            });
        }
        function loadDefaultInfile() {
            $.get(server + "/dashboard/default-infile", function( data ) {
                $('#infile-box').text(data);
            })
        }

        loadDefaultInfile();
        loadDash();
    </script>

</body>
</html>
`
