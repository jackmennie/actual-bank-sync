.PHONY: run setup sync build helper-install test fmt vet check lint lint-fix

run:
	go run ./cmd/actual-bank-sync

setup:
	go run ./cmd/actual-bank-sync setup

sync:
	go run ./cmd/actual-bank-sync sync $(ARGS)

build:
	mkdir -p bin
	go build -o bin/actual-bank-sync ./cmd/actual-bank-sync

helper-install:
	npm ci --prefix helper

lint:
	golangci-lint run

lint-fix:
	golangci-lint fmt

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

check: fmt vet test