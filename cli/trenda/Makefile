.PHONY: build test lint install clean

build:
	go build -o bin/trenda-pp-cli ./cmd/trenda-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/trenda-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/trenda-pp-mcp ./cmd/trenda-pp-mcp

install-mcp:
	go install ./cmd/trenda-pp-mcp

build-all: build build-mcp
