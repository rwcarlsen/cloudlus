#!/bin/bash

n=1
jobs=""

echo "generating job files"
while read line; do
    job=sweepjob-$n.json
    jobs="$jobs $job"
    cycdriver -gen $line > $job
    ((n++))
done <$1

echo "running jobs"

results=$(cloudlus submit sweepjob-*.json)

echo "extracting results"

for job in $results; do
    dir=$(cloudlus unpack $job)
    out=$(cat $dir/out.txt)
    echo "$dir $out"
done

