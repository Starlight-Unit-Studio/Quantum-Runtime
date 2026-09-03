#!/bin/sh
set -eu

sh scripts/verify-legal.sh
sh scripts/verify-systemd.sh

if [ -n "$(gofmt -l .)" ]; then
  echo "Go files require gofmt:" >&2
  gofmt -l . >&2
  exit 1
fi

expected_version="$(tr -d '[:space:]' < VERSION)"
set -- $(go run ./cmd/quantum-runtime -version)
runtime_version="$3"
installer_version="$(go run ./cmd/quantum-runtime-installer version)"

if [ "$runtime_version" != "$expected_version" ]; then
  echo "Runtime build version mismatch: VERSION=$expected_version binary=$runtime_version" >&2
  exit 1
fi
if [ "$installer_version" != "$expected_version" ]; then
  echo "Installer build version mismatch: VERSION=$expected_version installer=$installer_version" >&2
  exit 1
fi

go vet ./...
go test -race ./...
go build -trimpath -o /tmp/quantum-runtime-verify ./cmd/quantum-runtime
go build -trimpath -o /tmp/quantum-runtime-installer-verify ./cmd/quantum-runtime-installer
rm -f /tmp/quantum-runtime-verify /tmp/quantum-runtime-installer-verify

echo "Quantum Runtime verification passed."
