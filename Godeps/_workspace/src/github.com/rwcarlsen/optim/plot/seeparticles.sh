#!/bin/bash

sqlite3 -column $1 "select $2 from swarmbest" > best.dat
sqlite3 -column $1 "select $2 from swarmparticles where particle = $3" > p1.dat
sqlite3 -column $1 "select $2 from swarmparticles where particle = $4" > p2.dat
sqlite3 -column $1 "select $2 from swarmparticles where particle = $5" > p3.dat
sqlite3 -column $1 "select $2 from swarmparticles where particle = $6" > p4.dat
sqlite3 -column $1 "select $2 from swarmparticles where particle = $7" > p5.dat

gnuplot particles.gp

