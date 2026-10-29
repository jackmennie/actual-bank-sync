.PHONY: run setup sync build helper-install deb test fmt vet check lint lint-fix

VERSION ?= 0.1.0-1

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

deb:
	VERSION=$(VERSION) bash debian/build-package

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