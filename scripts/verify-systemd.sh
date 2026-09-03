#!/bin/sh
set -eu

if ! command -v systemd-analyze >/dev/null 2>&1; then
  echo "systemd-analyze not available; skipping systemd syntax verification."
  exit 0
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT HUP INT TERM
unit="$tmpdir/quantum-runtime.service"
sed 's#^ExecStart=/usr/local/bin/quantum-runtime$#ExecStart=/bin/true#' deploy/systemd/quantum-runtime.service > "$unit"
systemd-analyze verify "$unit"
echo "Quantum Runtime systemd unit verification passed."
