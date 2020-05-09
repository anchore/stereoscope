TEMPDIR = ./.tmp
LINTCMD = $(TEMPDIR)/golangci-lint run --tests=false --config .golangci.yaml

.PHONY: all boostrap lint lint-fix unit coverage integration

all: lint unit integration

bootstrap:
	# install project dependencies
	go get ./...
	mkdir -p $(TEMPDIR)
	# install golangci-lint
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b .tmp/ v1.26.0
	# install go-acc
	GOPATH=$(shell realpath ${TEMPDIR}) GO111MODULE=off go get github.com/ory/go-acc

lint:
	$(LINTCMD)

lint-fix:
	$(LINTCMD) --fix

unit:
	go test -v --race ./...

coverage:
	$(TEMPDIR)/bin/go-acc -o $(TEMPDIR)/coverage.txt ./...

# TODO: add benchmarks

integration:
	go test -v -tags=integration ./integration