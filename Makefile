VERSION  = v$(shell date +%Y%m%d%H%M)
BINARY  := lognoodler
IMAGE   := bugroger/lognoodler
PACKAGE := github.com/bugroger/lognoodler
GOFILES=$(wildcard $(addsuffix /*.go,$(shell find . -type d))) go.mod go.sum

.PHONY: all dev clean

all: build 

binaries: bin/darwin/amd64/$(BINARY)

bin/%: GOOS=$(shell echo $(basename $(@D)) | cut -d/ -f2)
bin/%: GOARCH=$(shell echo $(basename $(@D)) | cut -d/ -f3)
bin/%: $(GOFILES) Makefile
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
		-ldflags='-X $(PACKAGE)/main.VERSION=$(VERSION)' \
		-installsuffix=static -v -o=$@ .

build: GOVERSION ?= 1.12
build: Dockerfile 
	docker build --build-arg GOVERSION=$(GOVERSION) -t $(IMAGE)-cache .
	docker run --rm -v $(PWD):/src -v $(PWD)/.cache:/root/.cache -w /src $(IMAGE)-cache make binaries

dev: build 
	while true; do while IFS= read -r line; do printf '%s\n' "$$line"; sleep 0.1; done < test.txt; done | bin/darwin/amd64/$(BINARY)

clean:
	rm -rf bin/*