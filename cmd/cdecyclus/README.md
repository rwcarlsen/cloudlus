
cdecyclus
==========

The files in this directory allow you to create a self-contained, executable
package of cyclus with desired archetypes.  The package has no external
dependencies.  Requirements:

* `cyclus` must be on your path with all necessary archetypes in your
  `CYCLUS_PATH`.

* `cde` must be installed on your system (http://www.pgbovine.net/cde.html).

To generate the package:

1. Edit the sample-sim.xml file to use all archetypes/agents that you want to
   be accessible in the package.

2. Run `make`

3. A tarball is generated that contains all necessary files including a bash
   `cyclus` script that runs the packaged Cyclus.
