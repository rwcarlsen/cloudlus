
Cloudlus
=========

This is a collection of tools for running Cyclus simulations in the cloud and
in high-throughput computing environments.  After installing the Go
toolchain (http://golang.org/doc/install) - likely available as a standard
package in your favorite package manager - all you need to do is:

```bash
go get github.com/rwcarlsen/cloudlus/...
```

This provides a few basic comands.  The `cloudlus` command provides the
ability to run a remote execution server.  This server handles the dispatch
and distribution of cyclus jobs to *workers* that can also be deployed with
this command.  Their can be an arbitrary number of workers and they can live
locally on the same machine as the server or somewhere else in the nets.
Cyclus jobs can be deployed and fetched to/from the server using the
`cloudlus` command.  The server also provides a simple RESTful api.  The api consists of the following endpoints:

* GET to `[host]/api/v1/job/[job-id]` returns a JSON object in the response
  body with information about the job status and any output data+files if it
  finished running.  If the job has not finnished running yet, you can check
  the *Status* field of the JSON object.  A status of "complete" or "failed"
  indicates the job has finished running.  The returned JSON object roughly
  has the following schema:

```json
{
    "Id": "b1cd52ea474d4f58849082b54b16914c",
    "Cmd": [
        "[command]",
        "[arg1]",
        "[arg2]",
        "[...]",
    ],
    "Infiles": [
        {
            "Name": "input.xml",
            "Data": "base64 encoded string of file contents",
            "Cache": false
        }
    ],
    "Outfiles": [
        {
            "Name": "cyclus.sqlite",
            "Data": "base64 encoded string of file contents",
            "Cache": false
        }
    ],
    "Status": "complete",
    "Stdout": "standard output from the job process",
    "Stderr": "standard error from the job process",
    "Timeout": 600000000000,
    "Submitted": "2014-09-30T22:59:54.061622259-05:00",
    "Started": "2014-09-30T23:00:02.743536714-05:00",
    "Finished": "2014-09-30T23:00:09.029352256-05:00",
    "WorkerId": "024b7ff3f85047dcba19abbf011ebd53",
    "Note": ""
}
```

* GET to `[host]/api/v1/job-outfiles/[job-id]` returns a zip-file of the
  output files for the job in the response body.

* POST to `[host]/api/v1/job-infile` creates a new default cyclus simulation
  job.  The request body is the raw bytes of the simulation input file. The
  *Location* field in the response header contains the URL endpoint where the
  created job status can be retrieved.  The response body contains a JSON
  object representing the created job.

* POST to `[host]/api/v1/job` submits a new job to be run.  The job must be
  specified as a JSON object present in the request body.  The job format is:

```json
{
    "Id": "[hex-encoded-random-uuid]",
    "Cmd": [
        "[command]",
        "[arg1]",
        "[arg2]",
        "[...]",
    ],
    "Infiles": [
        {
            "Name": "input.xml",
            "Data": "base64 encoded string of file contents"
        }
    ],
    "Outfiles": [
        {
            "Name": "cyclus.sqlite"
        }
    ],
    "Note": "extra notes about this job"
}
```

