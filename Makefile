.PHONY: build run test lint tidy clean

MODULE := github.com/Laaaaksh/seatkey
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/seatkeyd ./cmd/seatkeyd
	go build -ldflags "$(LDFLAGS)" -o bin/democli ./cmd/democli

run:
	go run ./cmd/seatkeyd

test:
	go test ./... -race -cover

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

clean:
	rm -rf bin/
	go clean -testcache
