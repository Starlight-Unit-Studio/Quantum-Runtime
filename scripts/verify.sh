#!/bin/sh
set -eu

if [ -n "$(gofmt -l .)" ]; then
  echo "Go files require gofmt:" >&2
  gofmt -l . >&2
  exit 1
fi

go vet ./...
go test -race ./...
go build -trimpath -o /tmp/quantum-runtime-verify ./cmd/quantum-runtime
rm -f /tmp/quantum-runtime-verify

echo "Quantum Runtime verification passed."
