# ADR 0002: Begin with an Ollama adoption backend

Status: accepted for the foundation

## Decision

Quantum Runtime first operates as a constrained service in front of an existing local Ollama installation.

## Reasons

- CoreUI and Game can adopt the Runtime endpoint immediately
- request, streaming, cancellation and timeout behavior can be tested with real models
- the public Runtime contract can stabilize before native backend work
- current installations remain non-destructive
- backend replacement becomes an internal Runtime change rather than an application rewrite

## Rejected alternative

Building the complete native inference engine before exposing any Runtime service would postpone all integration learning and create a larger untested change.

## Limitation

The foundation is not independent inference. Documentation and status output must state that the active backend is `ollama-adapter`.
