
all: cyc-cde.tar.gz init.sh

cyc-cde.tar.gz: sample-sim.xml
	cde cyclus $<
	rm cyclus.sqlite
	mv cde-package cyc-cde
	tar -czf cyc-cde.tar.gz cyc-cde
	rm -rf cyc-cde
	rm -f cde.options
	rm cyclus

cyclus_path = $(subst /,\/,$(shell which cyclus))

init.sh: init.sh.in
	sed "s/{{CYCLUS_PATH}}/$(cyclus_path)/" $< > $@
	chmod a+x $@

clean:
	rm -rf cde.options cyc-cde.tar.gz init.sh cde-package cyclus.sqlite

