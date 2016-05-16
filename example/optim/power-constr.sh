#!/bin/bash

simdur=1200

# number of months between facility deployments
deploy_period=24

# LWR capacity in MWe
lwr_cap=900

# number of reactors initially deployed
n_initial=20

# +/- fraction of power curve that defines min/max bounds
errband=0.15

# % annual growth
growthrate=.01


echo -n '"MinPower": ['
seq $simdur | awk 'NR == 1 {mult=1} NR % '$deploy_period' == 1 {printf "%.10g, ", '$n_initial'*'$lwr_cap'*mult*(1-'$errband')} {mult *= (1+'$growthrate'/12)}' | sed 's/, *$//'
echo '],'
echo -n '"MaxPower": ['
seq $simdur | awk 'NR == 1 {mult=1} NR % '$deploy_period' == 1 {printf "%.10g, ", '$n_initial'*'$lwr_cap'*mult*(1+'$errband')} {mult *= (1+'$growthrate'/12)}' | sed 's/, *$//'
echo '],'
