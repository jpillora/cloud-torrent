.PHONY: help all test format fmtcheck vet lint     qa deps clean nuke








# Display general help about this command
help:
	@echo ""
	@echo "The following commands are available:"
	@echo ""
	@echo "    make qa          : Run all the tests"
	@echo "    make test        : Run the unit tests"
	@echo ""
	@echo "    make format      : Format the source code"
	@echo "    make fmtcheck    : Check if the source code has been formatted"
	@echo "    make vet         : Check for suspicious constructs"
	@echo "    make lint        : Check for style errors"
	@echo ""
	@echo "    make deps        : Get the dependencies"
	@echo "    make clean       : Remove any build artifact"
	@echo "    make nuke        : Deletes any intermediate file"
	@echo ""
	@echo "    make fuzz       : Fuzzy testing"
	@echo ""

# Alias for help target
all: help
test: 
	go test
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





# Alias to run all quality-assurance checks
qa: fmtcheck test vet lint    

# --- INSTALL ---

# Get the dependencies
deps:
	GOPATH=$(GOPATH) go get github.com/smartystreets/goconvey/convey
	GOPATH=$(GOPATH) go get github.com/willf/bitset 
	GOPATH=$(GOPATH) go get github.com/golang/lint/golint
	GOPATH=$(GOPATH) go get github.com/mschoch/smat
	GOPATH=$(GOPATH) go get github.com/dvyukov/go-fuzz/go-fuzz
	GOPATH=$(GOPATH) go get github.com/dvyukov/go-fuzz/go-fuzz-build

fuzz:
	go test -tags=gofuzz -run=TestGenerateSmatCorpus
	go-fuzz-build github.com/RoaringBitmap/roaring
	go-fuzz -bin=./roaring-fuzz.zip -workdir=workdir/ -timeout=200

# Remove any build artifact
clean:
	GOPATH=$(GOPATH) go clean ./...

# Deletes any intermediate file
nuke:
	rm -rf ./target
	GOPATH=$(GOPATH) go clean -i ./...
