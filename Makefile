.PHONY: build run test clean

BINARY := secha
CMD := ./cmd/secha

build:
	go build -o bin/$(BINARY) $(CMD)

run:
	go run $(CMD)

test:
	go test ./...

clean:
	rm -rf bin/
