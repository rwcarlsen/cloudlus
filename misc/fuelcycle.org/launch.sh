#!/bin/bash

for i in $(seq 1 $1); do
	mkdir worker-$i
	bash -c "cd worker-$i; cp ../cyc-cde.tar.gz ./; tar -xzf cyc-cde.tar.gz; ./init.sh &> worker-$i.log &"
done
