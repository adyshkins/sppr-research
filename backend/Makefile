.PHONY: test vet build run

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

build:
	go build -o research-api ./cmd/research-api

run:
	go run ./cmd/research-api
