.PHONY: build run test clean

BINARY := secha
CMD := ./cmd/secha

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(CMD)

run:
	go run -ldflags "$(LDFLAGS)" $(CMD)

test:
	go test ./...

clean:
	rm -rf bin/
