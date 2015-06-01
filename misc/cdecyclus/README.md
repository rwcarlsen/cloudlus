
cdecyclus
==========

The files in this directory allow you to create a self-contained, executable
package of cyclus with desired archetypes.  The package also contains
cycdriver. The package has no external dependencies.  The process also
generates an `init.sh` file which should be passed as an argument to
`condorbots -init` - or it must be executed by the executable file run by the
condor job. Its purpose is to create shortcuts to the packaged `cyclus` and
`cycdriver` commands in the condor node's working directory so that cloudlus
workers can run them easily/directly. Requirements:

* `cyclus` must be on your path with all necessary archetypes in your
  `CYCLUS_PATH`.

* `cycdriver` must also be on your path.

* `cde` must be installed on your system (http://www.pgbovine.net/cde.html).

To generate the package:

1. Edit the sample-sim.xml file to use all archetypes/agents that you want to
   be accessible in the package.

2. Run `make`.  Note that there is an alternate `make worker` target that
generates a single tar archige with the init script inside it.  The init
script in this case additionally starts a cloudlus worker talking to a server
at cycrun.fuelcycle.org.

3. A tarball is generated that contains all necessary files including a bash
   `init.sh` script for generating cyclus and cycdriver command runners on condor
   execute nodes.
