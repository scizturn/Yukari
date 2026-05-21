.PHONY: test build run

test:
	go test ./...

build:
	go build -o bin/yukari ./cmd/yukari

run:
	go run ./cmd/yukari
