
Remote Execution for fuelcycle.org
===================================

This directory contains tools needed to deploy the cyclus remote execution
environment to the cloud.  Updating/launching the remote execution
functionality is as follows:

* make sure cde is installed (http://www.pgbovine.net/cde.html)

* Build and install Cyclus and Cycamore.  The cyclus binary must be on your
  PATH.

* Build/install the cloudlus binary (in this repository).  This requires the
  go toolchain to be installed (golang.org).  From the repository root
  directory run `go install ./cmd/cloudlus`.  Make sure the cloudlus binary is
  on your PATH

* Run `make` to build the cyclus worker environment tarball.

* Copy `cyc-cde.tar.gz`, `launch.sh`, and the cloudlus binary  to the cloud
  server - e.g. `scp cyc-cde.tar.gz launch.sh $(which cloudlus) [server]:./`

* On the cloud server run:

  ```
  ./cloudlus -addr=0.0.0.0:80 serve -host cycrun.fuelcycle.org:80 -dblimit 2000
  ./launch.sh N   # where N is the number of workers to run (e.g. 2)
  ```

Note that for the last commands to work, you must have already killed any
previously running cloudlus server and worker processes.  Files in this
directory are:

* `Makefile` is used to build a new `cyc-cde.tar.gz` archive containing the
  entire cyclus environment.  Cyclus and Cycamore and cloudlus must all be
  installed on your system and the binaries must be in your PATH.  Just run
  `make`.

* `launch.sh` takes one integer argument specifying the number of remote
  execution simulation workers to spin up/run.  Launches several cloudlus
  remote execution workers (with cyclus environments) using the
  `cyc-cde.tar.gz` archives.

* `sample-sim.xml` is used to construct the cyclus tarball (i.e. via the
  makefile).  If any new archetypes are added to Cyclus or Cycamore, their
  specs will need to be added to the `archetypes` section of the input file. 

* `init.sh.in` is a templated script used to launch workers - you shouldn't
  need to ever mess with this.

