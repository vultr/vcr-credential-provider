GOARCH ?= amd64

.PHONY: build build-linux clean test lint

build:
	go build -trimpath -o dist/vcr-credential-provider ./cmd

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build -trimpath -o dist/vcr-credential-provider ./cmd

clean:
	go clean ./...
	rm -rf dist

test:
	go test -race ./...

lint:
	golangci-lint run --timeout=5m ./...
