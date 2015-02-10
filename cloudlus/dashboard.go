package cloudlus

import (
	"fmt"
	"html/template"
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
        <td><a href="{{$job.Host}}/api/v1/job-outfiles/{{$job.Id}}">Results</a></td>
        {{else}}
        <td></td>
        {{end}}
    </tr>
    {{ end }}
</table>
`
var tmpl = template.Must(template.New("dashtable").Parse(dashtmplstr))
var hometmpl = template.Must(template.New("home").Parse(home))

const ncompleted = 100

type JobData struct {
	Id        string
	Status    string
	Submitted time.Time
	Host      string
}

type JobList []*Job

func (s JobList) Len() int      { return len(s) }
func (s JobList) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type BySubmitted struct{ JobList }

func (s BySubmitted) Less(i, j int) bool { return s.JobList[i].Submitted.After(s.JobList[j].Submitted) }

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	jobs, _ := s.alljobs.Current()
	completed, _ := s.alljobs.Recent(ncompleted)
	jobs = append(jobs, completed...)
	sort.Sort(BySubmitted{jobs})

	jds := []JobData{}
	for _, j := range jobs {
		jd := JobData{
			Id:        fmt.Sprintf("%v", j.Id),
			Status:    j.Status,
			Submitted: j.Submitted,
			Host:      s.Host,
		}
		jds = append(jds, jd)
	}

	// allow cross-domain ajax requests for the dashboard content
	w.Header().Add("Access-Control-Allow-Origin", "*")
	if err := tmpl.Execute(w, jds); err != nil {
		httperror(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) dashmain(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")
	err := hometmpl.Execute(w, s)
	if err != nil {
		httperror(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) dashboardInfile(w http.ResponseWriter, r *http.Request) {
	idstr := r.URL.Path[len("/dashboard/infile/"):]
	j, err := s.getjob(idstr)
	if err != nil {
		httperror(w, err.Error(), http.StatusInternalServerError)
		return
	} else if j == nil {
		httperror(w, "job id not found", http.StatusBadRequest)
		return
	}

	w.Header().Add("Content-Type", "text/xml")
	w.Header().Add("Content-Disposition", fmt.Sprintf("filename=\"job-id-%v-infile.xml\"", j.Id))
	if len(j.Infiles) == 0 {
		fmt.Fprint(w, "[job contains no input data]")
	} else {
		w.Write(j.Infiles[0].Data)
	}
}

func (s *Server) dashboardOutput(w http.ResponseWriter, r *http.Request) {
	idstr := r.URL.Path[len("/dashboard/output/"):]
	j, err := s.getjob(idstr)
	if err != nil {
		httperror(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = w.Write([]byte(j.Stdout))
	if err != nil {
		httperror(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write([]byte(j.Stderr))
	if err != nil {
		httperror(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) dashboardDefaultInfile(w http.ResponseWriter, r *http.Request) {
	// allow cross-domain ajax requests for the dashboard content
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Content-Type", "text/plain")
	_, err := w.Write([]byte(defaultInfile))
	if err != nil {
		httperror(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

const defaultInfile = `<simulation>
  <control>
    <duration>100</duration>
    <startmonth>1</startmonth>
    <startyear>2000</startyear>
  </control>

  <archetypes>
    <spec> <lib>agents</lib><name>Source</name> </spec>
    <spec> <lib>agents</lib><name>Sink</name> </spec>
    <spec> <lib>agents</lib><name>NullRegion</name> </spec>
    <spec> <lib>agents</lib><name>NullInst</name> </spec>
  </archetypes>

  <facility>
    <name>Source</name>
    <config>
      <Source>
        <commod>commodity</commod>
        <recipe_name>commod_recipe</recipe_name>
        <capacity>1.00</capacity>
      </Source>
    </config>
  </facility>

  <facility>
    <name>Sink</name>
    <config>
      <Sink>
        <in_commods>
          <val>commodity</val>
        </in_commods>
        <capacity>1.00</capacity>
      </Sink>
    </config>
  </facility>

  <region>
    <name>SingleRegion</name>
    <config> <NullRegion/> </config>
    <institution>
      <name>SingleInstitution</name>
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
      <config> <NullInst/> </config>
    </institution>
  </region>

  <recipe>
    <name>commod_recipe</name>
    <basis>mass</basis>
    <nuclide> <id>010010000</id> <comp>1</comp> </nuclide>
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

		#stats,#since {
			width:80%;
			margin:auto;
			text-align:left;
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

    <div id="stats">
		<ul>
			<li>
				{{.Stats.CurrRunning}} jobs currently running
			</li>
			<li>
				{{.Stats.CurrQueued}} jobs currently queued
			</li>
			<li>
				{{.Stats.NRequeued}} jobs requeued
			</li>
			<li>
				{{.Stats.NSubmitted}} jobs received
			</li>
			<li>
				{{.Stats.NCompleted}} jobs completed
			</li>
			<li>
				{{.Stats.NFailed}} jobs failed
			</li>
			<li>
				{{.Stats.NPurged}} old jobs purged.
			</li>
		</ul>
	</div>

    <br>
    <div id="dashboard"></div>
    <br>

	<div id="since">
	<i>Server running since {{.Stats.Started}}</i>
	</div>

    <script> 
        var server = "{{.Host}}"

        function submitJob() {
            var text = $('#infile-box').val();
            $.post(server + "/api/v1/job-infile", text, function(data) {
                var resp = JSON.parse(data)
                $('#jobid').text(resp.Id);
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
