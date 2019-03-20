DATE    = $(shell date +%Y%m%d%H%M)
VERSION = v$(DATE)

GOVERSION ?= 1.12
GOOS      ?= darwin
GOARCH    ?= amd64

PACKAGES := $(shell find . -type d)
GOFILES  := $(wildcard $(addsuffix /*.go,$(PACKAGES)))

BINARY  := slurpily
IMAGE   := bugroger/slurpily
PACKAGE := github.com/bugroger/slurpily

.PHONY: all build binaries clean 

all: build 

binaries: bin/darwin/amd64/slurpily 

bin/%: GOOS=  $(shell echo $(basename $(@D)) | cut -d/ -f2)
bin/%: GOARCH=$(shell echo $(basename $(@D)) | cut -d/ -f3)
bin/%: $(GOFILES) Makefile
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags='-X github.com/bugroger/slurpily/main.VERSION=$(VERSION)' -installsuffix=static -v -o=$@ .

build: 
	docker build --build-arg GOVERSION=$(GOVERSION) -t $(IMAGE)-cache .
	docker run --rm -v $(PWD):/src -v $(PWD)/.cache:/root/.cache -w /src $(IMAGE)-cache make binaries

dev: build 
	while true; do while read line; do echo $$line; sleep 0.1; done < test.txt; done | bin/darwin/amd64/slurpily

clean:
	rm -rf bin/*