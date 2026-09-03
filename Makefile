SHELL := /bin/sh

.PHONY: fmt fmt-check vet test race build verify

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

fmt-check:
	@test -z "$$(gofmt -l .)" || { echo "Go files require gofmt:"; gofmt -l .; exit 1; }

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

build:
	mkdir -p bin
	go build -trimpath -o bin/quantum-runtime ./cmd/quantum-runtime
	go build -trimpath -o bin/quantum-runtime-installer ./cmd/quantum-runtime-installer

verify:
	./scripts/verify.sh
