# Quantum Runtime

Quantum Runtime is the independent AI runtime and model-service project of Starlight Unit Studios.

Current version: `0.1.0-alpha.1`

## Current alpha scope

This first foundation establishes the stable service boundary that Ember CoreUI, STΛRLIGHT UNIT The Game and later Quantum CoreOS can target.

The alpha currently runs in **Ollama adoption mode**:

```text
Ember CoreUI / Game / client
              |
              v
      Quantum Runtime :11450
              |
              v
       existing Ollama :11434
```

Quantum Runtime already owns the public endpoint, request policy, authentication boundary, health reporting, timeouts and compatibility allowlist. It does not yet execute model inference independently. A pluggable native inference backend is a later phase.

This staged design lets current applications adopt the Quantum Runtime contract without waiting for the final inference engine and without coupling Quantum Runtime to Quantum CoreOS.

## Foundation capabilities

- standalone Go service with no third-party Go dependencies
- loopback-only default on `127.0.0.1:11450`
- Ollama-compatible chat, generation, embedding and model-read routes
- transparent streamed response forwarding
- model mutation routes disabled by default
- bearer-token requirement for any non-loopback bind
- request-size and upstream-timeout policy
- liveness, readiness, version and capability endpoints
- request IDs and structured logs without prompt bodies
- graceful shutdown
- unit, race, vet, formatting and build verification

## Run locally

An existing Ollama service may remain on its default local endpoint.

```bash
go run ./cmd/quantum-runtime
```

Check the service:

```bash
curl -s http://127.0.0.1:11450/healthz
curl -s http://127.0.0.1:11450/readyz
curl -s http://127.0.0.1:11450/v1/runtime
```

Run all verification:

```bash
./scripts/verify.sh
```

## Ember CoreUI adoption

The current Ember CoreUI server-side Ollama call can be routed through Quantum Runtime without requiring Quantum CoreOS:

```text
COREUI_OLLAMA_URL=http://127.0.0.1:11450/api/chat
```

During this first alpha, Ollama still performs the actual inference behind Quantum Runtime. Later backends must preserve the same application-facing contract.

Ember CoreUI remains an independent Repack and may continue to use Ollama directly when Quantum Runtime is not selected.

## Model policy

Quantum Runtime is model-neutral. It must not force one model on every consumer.

The future Quantum CoreOS TCI profile targets **Gemma 4 e4b** with its own personality package and operating-system integration. That target belongs to the CoreOS TCI profile, not to the generic Runtime default.

CoreUI, the Game and other clients keep their own model identity, persona package, context and policy.

## Configuration

Copy or adapt `config/quantum-runtime.env.example` for service deployment.

| Variable | Default | Purpose |
|---|---:|---|
| `QUANTUM_RUNTIME_LISTEN` | `127.0.0.1:11450` | HTTP listen address |
| `QUANTUM_RUNTIME_OLLAMA_URL` | `http://127.0.0.1:11434` | Initial adoption backend |
| `QUANTUM_RUNTIME_UPSTREAM_TIMEOUT` | `15m` | Maximum backend request duration |
| `QUANTUM_RUNTIME_MAX_REQUEST_BYTES` | `134217728` | Request body limit |
| `QUANTUM_RUNTIME_ALLOW_MODEL_MUTATION` | `false` | Enable pull/create/copy/delete proxy routes |
| `QUANTUM_RUNTIME_AUTH_TOKEN` | empty | Required for any non-loopback bind |

The process rejects a network-wide bind when no bearer token is configured.

## Project boundaries

Quantum Runtime is developed and released independently.

```text
Quantum Runtime
    model lifecycle, inference APIs, streaming, backend adapters

Quantum Control
    server, hosting, service, database, backup and update management

Quantum CoreOS
    final operating-system integration of released Runtime and Control modules
```

Quantum CoreOS may provide optimized Runtime service profiles, GPU policy and local IPC later, but it must consume the upstream Quantum Runtime project rather than maintaining a private fork.

## Documentation

- `docs/ARCHITECTURE.md`
- `docs/API.md`
- `docs/ROADMAP.md`
- `docs/SECURITY.md`
- `docs/adr/0001-service-language.md`
- `docs/adr/0002-adoption-backend-first.md`
- `docs/adr/0003-model-neutral-runtime.md`

## Licensing status

No project license has been selected in this foundation yet. A deliberate license decision is required before a public binary release.
