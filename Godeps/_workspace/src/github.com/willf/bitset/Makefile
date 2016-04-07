# MAKEFILE
#
# @author      Nicola Asuni <nicola@cognitivelogic.com>
# @link        https://github.com/willf/bitset
# ------------------------------------------------------------------------------

# List special make targets that are not associated with files
.PHONY: help all qa test format fmtcheck vet lint coverage docs deps clean nuke

# Use bash as shell (Note: Ubuntu now uses dash which doesn't support PIPESTATUS).
SHELL=/bin/bash

# Project owner
OWNER=willf

# Project name
PROJECT=bitset

# Name of RPM or DEB package
PKGNAME=${OWNER}-${PROJECT}

# Go lang path. Set if necessary
# GOPATH=$(shell readlink -f $(shell pwd)/../../../../)

# Current directory
CURRENTDIR=$(shell pwd)

# --- MAKE TARGETS ---

# Display general help about this command
help:
	@echo ""
	@echo "$(PROJECT) Makefile."
	@echo "The following commands are available:"
	@echo ""
	@echo "    make qa         : Run all the tests"
	@echo "    make test       : Run the unit tests"
	@echo ""
	@echo "    make format     : Format the source code"
	@echo "    make fmtcheck   : Check if the source code has been formatted"
	@echo "    make vet        : Check for syntax errors"
	@echo "    make lint       : Check for style errors"
	@echo "    make coverage   : Generate the coverage report"
	@echo ""
	@echo "    make docs       : Generate source code documentation"
	@echo ""
	@echo "    make deps       : Get the dependencies"
	@echo "    make clean      : Remove any build artifact"
	@echo "    make nuke       : Deletes any intermediate file"
	@echo ""

# Alias for help target
all: help

# Run the unit tests
test:
	@mkdir -p target/test
	@mkdir -p target/report
	GOPATH=$(GOPATH) go test -covermode=count -coverprofile=target/report/coverage.out -bench=. -race -v ./... | tee >(PATH=$(GOPATH)/bin:$(PATH) go-junit-report > target/test/report.xml); test $${PIPESTATUS[0]} -eq 0

# Format the source code
format:
	@find ./ -type f -name "*.go" -exec gofmt -w {} \;

# Check if the source code has been formatted
fmtcheck:
	@mkdir -p target
	@find ./ -type f -name "*.go" -exec gofmt -d {} \; | tee target/format.diff
	@test ! -s target/format.diff || { echo "ERROR: the source code has not been formatted - please use 'make format' or 'gofmt'"; exit 1; }

# Check for syntax errors
vet:
	GOPATH=$(GOPATH) go vet ./...

# Check for style errors
lint:
	GOPATH=$(GOPATH) PATH=$(GOPATH)/bin:$(PATH) golint ./...

# Generate the coverage report
coverage:
	GOPATH=$(GOPATH) go tool cover -html=target/report/coverage.out -o target/report/coverage.html

# Generate source docs
docs:
	@mkdir -p target/docs
	nohup sh -c 'GOPATH=$(GOPATH) godoc -http=127.0.0.1:6060' > target/godoc_server.log 2>&1 &
	wget --directory-prefix=target/docs/ --execute robots=off --retry-connrefused --recursive --no-parent --adjust-extension --page-requisites --convert-links http://127.0.0.1:6060/pkg/github.com/${OWNER}/${PROJECT}/ ; kill -9 `lsof -ti :6060`
	@echo '<html><head><meta http-equiv="refresh" content="0;./127.0.0.1:6060/pkg/github.com/'${OWNER}'/'${PROJECT}'/index.html"/></head><a href="./127.0.0.1:6060/pkg/github.com/'${OWNER}'/'${PROJECT}'/index.html">'${PKGNAME}' Documentation ...</a></html>' > target/docs/index.html

# Alias to run targets: fmtcheck test vet lint coverage
qa: fmtcheck test vet lint coverage

# --- INSTALL ---

# Get the dependencies
deps:
	GOPATH=$(GOPATH) go get ./...
	GOPATH=$(GOPATH) go get github.com/golang/lint/golint
	GOPATH=$(GOPATH) go get github.com/jstemmer/go-junit-report
	GOPATH=$(GOPATH) go get github.com/axw/gocov/gocov

# Remove any build artifact
clean:
	GOPATH=$(GOPATH) go clean ./...

# Deletes any intermediate file
nuke:
	rm -rf ./target
	GOPATH=$(GOPATH) go clean -i ./...
