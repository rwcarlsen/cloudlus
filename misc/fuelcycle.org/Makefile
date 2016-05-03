
cyclus_path = $(subst /,\/,$(shell which cyclus))
cycobj_path = $(subst /,\/,$(shell which cycobj))
cyan_path = $(subst /,\/,$(shell which cyan))
cloudlus_path = $(subst /,\/,$(shell which cloudlus))

worker: cyc-cde init.sh
	@echo "./cloudlus -addr=cycrun.fuelcycle.org:80 work -interval 3s -timeout=12s -whitelist=cyclus,cyan,cycobj" >> init.sh
	@cp $(cloudlus_path) ./
	tar -czf cyc-cde.tar.gz cyc-cde cloudlus init.sh
	@rm -rf cloudlus
	@rm -rf cyc-cde
	@rm -f cde.options

cyc-cde.tar.gz: cyc-cde
	tar -czf cyc-cde.tar.gz cyc-cde
	@rm -rf cyc-cde
	@rm -f cde.options

cyc-cde: sample-sim.xml
	cde cyclus $< 1> /dev/null
	- cde cycobj -h
	- cde cyan -h
	mv cde-package cyc-cde
	@rm cyclus.sqlite

init.sh: init.sh.in
	sed "s/{{CYCLUS_PATH}}/$(cyclus_path)/" $< > tmpinit.sh
	sed "s/{{CYCOBJ_PATH}}/$(cycobj_path)/" tmpinit.sh > tmpinit2.sh
	sed "s/{{CYAN_PATH}}/$(cyan_path)/" tmpinit2.sh > $@
	rm -f tmpinit.sh tmpinit2.sh
	chmod a+x $@

clean:
	rm -rf cde.options cyc-cde.tar.gz init.sh cde-package cyclus.sqlite cyc-cde cycobj cyclus

