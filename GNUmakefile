BIN_DIR = $(CURDIR)/bin

all: build

build:
	GOBIN=$(BIN_DIR) go install $(CURDIR)/...

check: vet

vet:
	go vet $(CURDIR)/...

test:
	go test $(CURDIR)/...

.PHONY: all build check vet test
