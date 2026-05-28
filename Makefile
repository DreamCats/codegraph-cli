.PHONY: build test install

build:
	go build -o codegraph ./cmd/codegraph

test:
	go test ./...

install:
	go install ./cmd/codegraph
