.PHONY: all build clean install
all:
	BUILD=true TEST=true cofunc run build/make.flowl

build:
	BUILD=true cofunc run build/make.flowl

test:
	TEST=true cofunc run build/make.flowl

clean:
	rm -rf bin/

install:
	rm -f ${HOME}/local/bin/cofunc
	cp bin/cofunc ${HOME}/local/bin/