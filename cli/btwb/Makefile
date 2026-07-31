.PHONY: build test lint install clean

build:
	go build -o bin/btwb-pp-cli ./cmd/btwb-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/btwb-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/btwb-pp-mcp ./cmd/btwb-pp-mcp

install-mcp:
	go install ./cmd/btwb-pp-mcp

build-all: build build-mcp
