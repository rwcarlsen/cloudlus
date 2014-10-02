
Example Sweep Analysis
=======================

Requirements:

* Cyclus and Cycamore (v1.1+)

* kitlus (with its agents) http://github.com/cyclus/kitlus

* cloudlus and cyan (dep of cloudlus) http://github.com/rwcarlsen/cyan

First, you need a cloudlus server running somewhere with one or more active
workers.  Then generate a bunch of job files for the sweep:

```bash
echo "generating job files"
while read line; do
    job=sweepjob-$n.json
    jobs="$jobs $job"
    cycdriver -gen $line > $job
    ((n++))
done <sweep.txt
```

The job generation uses `scenario.json`, `cyclus.xml.in`, and `sweep.txt` in
this folder.  `sweep.txt` was generated via `go run sweep.go` and a few text
tweeks.  After generating the jobs, you can submit them all to the running
cloudlus server:

```bash
cloudlus -addr="host:port" submit sweepjob-*.json
```

This results in a bunch of result json files.  You can then do something with
them like:

```bash
go run view.go -obj result-*.json > results.txt
```

