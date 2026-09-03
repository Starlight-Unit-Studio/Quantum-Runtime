# Quantum Runtime Security Baseline

Status: `0.1.0-alpha.1`

## Threat model

Quantum Runtime receives model prompts, images, documents encoded in requests and model-management operations. It must assume that all client content and all model output are untrusted.

## Current controls

- listener defaults to `127.0.0.1:11450`
- non-loopback binding is rejected unless a bearer token is configured
- only explicit compatibility routes can reach the initial backend
- model mutation is disabled by default
- request bodies have a configurable hard limit
- backend requests carry client cancellation and a maximum duration
- Runtime bearer tokens, cookies and proxy-forwarding headers are not sent upstream
- logs contain method, path, status, duration and request ID, but not prompt bodies
- upstream credentials embedded in URLs are rejected
- no model output is executed as a command

## Deployment rules

1. Keep the service loopback-only unless remote access is intentionally designed.
2. Put any remote deployment behind TLS and an authenticated reverse proxy.
3. Do not expose the initial Ollama backend separately once Runtime owns the public local contract.
4. Keep model mutation disabled for ordinary CoreUI chat operation.
5. Never place bearer tokens in repository files, images, release archives or shell history.
6. Use separate Unix users and filesystem permissions for Runtime and applications.
7. Treat model files and model recipes as supply-chain inputs that require hashes and provenance.
8. Do not grant Runtime root privileges merely to access a GPU.

## Future controls

Before native model management is considered production-ready, the project needs:

- signed or hash-pinned model manifests
- atomic model installation and rollback
- quota and disk-space policy
- per-client authorization scopes
- Unix-socket transport for local privileged integrations
- structured audit events for model lifecycle changes
- sandbox boundaries for converters and tool execution
- release signing and reproducible build work

## Reporting

Do not publish active credentials or exploitable private deployment details in a public issue. Use the official Starlight Unit Studios contact channel for sensitive reports until a dedicated security address is documented.
