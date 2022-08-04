TEMPDIR = ./.tmp
RESULTSDIR = test/results
COVER_REPORT = $(RESULTSDIR)/unit-coverage-details.txt
COVER_TOTAL = $(RESULTSDIR)/unit-coverage-summary.txt
LINTCMD = $(TEMPDIR)/golangci-lint run --tests=false --config .golangci.yaml
BOLD := $(shell tput -T linux bold)
PURPLE := $(shell tput -T linux setaf 5)
GREEN := $(shell tput -T linux setaf 2)
CYAN := $(shell tput -T linux setaf 6)
RED := $(shell tput -T linux setaf 1)
RESET := $(shell tput -T linux sgr0)
TITLE := $(BOLD)$(PURPLE)
SUCCESS := $(BOLD)$(GREEN)
# the quality gate lower threshold for unit test total % coverage (by function statements)
COVERAGE_THRESHOLD := 48

ifeq "$(strip $(VERSION))" ""
    override VERSION = $(shell git describe --always --tags --dirty)
endif

ifndef TEMPDIR
    $(error TEMPDIR is not set)
endif

ifndef REF_NAME
	REF_NAME = $(VERSION)
endif

define title
    @printf '$(TITLE)$(1)$(RESET)\n'
endef

.PHONY: all
all: static-analysis test ## Run all checks (linting, all tests, and dependencies license checks)
	@printf '$(SUCCESS)All checks pass!$(RESET)\n'

.PHONY: test
test: unit integration benchmark ## Run all levels of test

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "$(BOLD)$(CYAN)%-25s$(RESET)%s\n", $$1, $$2}'

.PHONY: ci-bootstrap
ci-bootstrap: bootstrap
	sudo apt install -y bc
	curl -sLO https://github.com/sylabs/singularity/releases/download/v3.10.0/singularity-ce_3.10.0-focal_amd64.deb && sudo apt-get install -y -f ./singularity-ce_3.10.0-focal_amd64.deb

$(RESULTSDIR):
	mkdir -p $(RESULTSDIR)

.PHONY: boostrap
bootstrap: $(RESULTSDIR) ## Download and install all project dependencies (+ prep tooling in the ./tmp dir)
	$(call title,Downloading dependencies)
	@pwd
	# prep temp dirs
	mkdir -p $(TEMPDIR)
	mkdir -p $(RESULTSDIR)
	# install go dependencies
	go mod download
	# install utilities
	[ -f "$(TEMPDIR)/benchstat" ] || GO111MODULE=off GOBIN=$(shell realpath $(TEMPDIR)) go get -u golang.org/x/perf/cmd/benchstat
	[ -f "$(TEMPDIR)/golangci" ] || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(TEMPDIR)/ v1.47.2
	[ -f "$(TEMPDIR)/bouncer" ] || curl -sSfL https://raw.githubusercontent.com/wagoodman/go-bouncer/master/bouncer.sh | sh -s -- -b $(TEMPDIR)/ v0.4.0

.PHONY: static-analysis
static-analysis: check-licenses lint

.PHONY: lint
lint: ## Run gofmt + golangci lint checks
	$(call title,Running linters)
	@printf "files with gofmt issues: [$(shell gofmt -l -s .)]\n"
	@test -z "$(shell gofmt -l -s .)"
	$(LINTCMD)

.PHONY: lint-fix
lint-fix: ## Auto-format all source code + run golangci lint fixers
	$(call title,Running lint fixers)
	gofmt -w -s .
	$(LINTCMD) --fix
	go mod tidy

.PHONY: check-licenses
check-licenses:
	$(call title,Validating licenses for go dependencies)
	$(TEMPDIR)/bouncer check

.PHONY: unit
unit: $(RESULTSDIR) ## Run unit tests (with coverage)
	$(call title,Running unit tests)
	go test --race -coverprofile $(COVER_REPORT) $(shell go list ./... | grep -v anchore/stereoscope/test/integration)
	@go tool cover -func $(COVER_REPORT) | grep total |  awk '{print substr($$3, 1, length($$3)-1)}' > $(COVER_TOTAL)
	@echo "Coverage: $$(cat $(COVER_TOTAL))"
	@if [ $$(echo "$$(cat $(COVER_TOTAL)) >= $(COVERAGE_THRESHOLD)" | bc -l) -ne 1 ]; then echo "$(RED)$(BOLD)Failed coverage quality gate (> $(COVERAGE_THRESHOLD)%)$(RESET)" && false; fi

.PHONY: benchmark
benchmark: $(RESULTSDIR) ## Run benchmark tests and compare against the baseline (if available)
	$(call title,Running benchmark tests)
	go test -cpu 2 -p 1 -run=^Benchmark -bench=. -count=5 -benchmem ./... | tee $(RESULTSDIR)/benchmark-$(REF_NAME).txt
	(test -s $(RESULTSDIR)/benchmark-main.txt && \
		$(TEMPDIR)/benchstat $(RESULTSDIR)/benchmark-main.txt $(RESULTSDIR)/benchmark-$(REF_NAME).txt || \
		$(TEMPDIR)/benchstat $(RESULTSDIR)/benchmark-$(REF_NAME).txt) \
			| tee $(RESULTSDIR)/benchstat.txt

.PHONY: show-benchstat
show-benchstat:
	@cat $(RESULTSDIR)/benchstat.txt

# note: this is used by CI to determine if the integration test fixture cache (docker image tars) should be busted
.PHONY: integration-fingerprint
integration-fingerprint:
	find test/integration/test-fixtures/image-* -type f -exec md5sum {} + | awk '{print $1}' | sort | md5sum | tee test/integration/test-fixtures/cache.fingerprint

.PHONY: integration-tools-fingerprint
integration-tools-fingerprint:
	@cd test/integration/tools && make fingerprint

.PHONY: integration-tools
integration-tools:
	@cd test/integration/tools && make

.PHONY: integration-tools
integration-tools-load:
	@cd test/integration/tools && make load-cache

.PHONY: integration-tools
integration-tools-save:
	@cd test/integration/tools && make save-cache

.PHONY: integration
integration: integration-tools ## Run integration tests
	$(call title,Running integration tests)
	go test -v ./test/integration

.PHONY: clear-test-cache
clear-test-cache: ## Delete all test cache (built docker image tars)
	find . -type f -wholename "**/test-fixtures/cache/*.tar" -delete
