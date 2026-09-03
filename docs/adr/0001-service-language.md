# ADR 0001: Go for the first Quantum Runtime service

Status: accepted for the foundation

## Decision

The first Quantum Runtime daemon is implemented in Go.

## Reasons

- one small deployable binary
- strong standard-library HTTP and concurrency support
- straightforward cancellation and streaming
- low operational complexity for current Linux servers
- easy unit and race testing
- no requirement to choose a final low-level inference engine yet

## Boundary

This does not require every later inference backend to be written in Go. Accelerator or model-engine components may run behind a stable backend boundary when that produces a better result.

The application-facing Runtime API remains independent from backend implementation language.
