#!/bin/bash

sqlite3 -column $1 "select $2 from swarmparticles where iter=1" > all.dat
gnuplot -e "plot 'all.dat' using 1:2 with points ps 1 pt 1; pause -1"

