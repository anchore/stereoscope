TEMPDIR = ./.tmp
LINTCMD = $(TEMPDIR)/golangci-lint run --tests=false --config .golangci.yaml
BOLD := $(shell tput -T linux bold)
PURPLE := $(shell tput -T linux setaf 5)
GREEN := $(shell tput -T linux setaf 2)
RESET := $(shell tput -T linux sgr0)
TITLE := $(BOLD)$(PURPLE)
SUCCESS := $(BOLD)$(GREEN)

.PHONY: all boostrap lint lint-fix unit coverage integration check-pipeline clear-cache

all: lint unit integration
	@printf '$(SUCCESS)All checks pass!$(RESET)\n'

bootstrap:
	@printf '$(TITLE)Downloading dependencies$(RESET)\n'
	# install project dependencies
	go get ./...
	mkdir -p $(TEMPDIR)
	# install golangci-lint
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b .tmp/ v1.26.0
	# install go-acc
	GOPATH=$(shell realpath ${TEMPDIR}) GO111MODULE=off go get github.com/ory/go-acc

lint:
	@printf '$(TITLE)Running linters$(RESET)\n'
	$(LINTCMD)

lint-fix:
	@printf '$(TITLE)Running lint fixers$(RESET)\n'
	$(LINTCMD) --fix

unit:
	@printf '$(TITLE)Running unit tests$(RESET)\n'
	go test --race ./...

coverage:
	@printf '$(TITLE)Running unit tests + coverage$(RESET)\n'
	$(TEMPDIR)/bin/go-acc -o $(TEMPDIR)/coverage.txt ./...

# TODO: add benchmarks

integration:
	@printf '$(TITLE)Running integration tests...$(RESET)\n'
	go test -v -tags=integration ./integration

clear-cache:
	rm -f integration/test-fixtures/tar-cache/*.tar

check-pipeline:
	# note: this is meant for local development & testing of the pipeline, NOT to be run in CI
	circleci config process .circleci/config.yaml > .tmp/circleci.yaml
	circleci local execute -c .tmp/circleci.yaml --job run-static-analysis
	circleci local execute -c .tmp/circleci.yaml --job run-tests-1
	circleci local execute -c .tmp/circleci.yaml --job run-tests-2
	@printf '$(SUCCESS)pipeline checks pass!$(RESET)\n'
