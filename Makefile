.PHONY: build run test lint tidy clean demo

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

# Boots a real seatkeyd, drives it and democli through Playwright, and
# converts the capture into docs/assets/demo.mp4 + demo.gif. See
# scripts/record-demo/README.md. Override the port with APP_PORT=<port> if
# :8080 is taken.
demo:
	cd scripts/record-demo && npm install && npx playwright install chromium && npm run record
