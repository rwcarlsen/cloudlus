#!/bin/bash

sqlite3 -column $1 "select $2 from swarmparticles" > all.dat

gnuplot all.gp

