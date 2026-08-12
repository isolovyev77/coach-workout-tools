.PHONY: build test lint install clean

build:
	go build -o bin/cap-pp-cli ./cmd/cap-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/cap-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/cap-pp-mcp ./cmd/cap-pp-mcp

install-mcp:
	go install ./cmd/cap-pp-mcp

build-all: build build-mcp
