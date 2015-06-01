
cyclus_path = $(subst /,\/,$(shell which cyclus))
cycdriver_path = $(subst /,\/,$(shell which cycdriver))
cyan_path = $(subst /,\/,$(shell which cyan))

all: cyc-cde.tar.gz init.sh

cyc-cde.tar.gz: sample-sim.xml
	cde cyclus $< 1> /dev/null
	- cde cycdriver -h
	mv cde-package cyc-cde
	tar -czf cyc-cde.tar.gz cyc-cde
	@rm cyclus.sqlite
	@rm -rf cyc-cde
	@rm -f cde.options

init.sh: init.sh.in
	sed "s/{{CYCLUS_PATH}}/$(cyclus_path)/" $< > tmpinit.sh
	sed "s/{{CYCDRIVER_PATH}}/$(cycdriver_path)/" tmpinit.sh > $@
	sed "s/{{CYAN_PATH}}/$(cyan_path)/" tmpinit.sh > $@
	rm -f tmpinit.sh
	chmod a+x $@

clean:
	rm -rf cde.options cyc-cde.tar.gz init.sh cde-package cyclus.sqlite cyc-cde cycdriver cyclus

