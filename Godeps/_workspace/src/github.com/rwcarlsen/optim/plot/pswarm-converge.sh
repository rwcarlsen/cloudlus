#!/bin/bash

sqlite3 -column $1 "select sum(p1.nsearch+p1.npoll),p1.val from patterninfo as p1 inner join patterninfo as p2 on p1.iter <= p2.iter GROUP BY p2.iter" > converge.dat

gnuplot -e "plot 'converge.dat' using 1:2 with linespoints; pause -1"
