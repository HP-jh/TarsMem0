.PHONY: build test clean run

BINARY=tarsmem0
VERSION=$(shell grep 'var Version' main.go | cut -d'"' -f2)

build:
	go build -ldflags "-s -w -X main.Version=$(VERSION)" -o $(BINARY) .

test:
	go test -race ./...

clean:
	rm -f $(BINARY)

run: build
	./$(BINARY)
