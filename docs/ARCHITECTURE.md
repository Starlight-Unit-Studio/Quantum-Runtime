# Quantum Runtime Architecture

Status: `0.1.0-alpha.1` foundation

## Responsibility

Quantum Runtime owns local AI model serving and inference transport. It does not own application personalities, user memories, web hosting, databases, desktop behavior or privileged operating-system administration.

## Long-term shape

```text
Clients
  |
  +-- Ember CoreUI
  +-- STΛRLIGHT UNIT Game
  +-- Quantum CoreOS TCI
  +-- other documented API consumers
  |
  v
Quantum Runtime API
  |
  +-- authentication and policy
  +-- request validation and cancellation
  +-- streaming transport
  +-- model identity and lifecycle
  +-- capability discovery
  +-- metrics and health
  |
  v
Backend interface
  |
  +-- Ollama adoption adapter, initial
  +-- native local inference backend, later
  +-- optional accelerator-specific backends, later
  |
  v
CPU / GPU / accelerator
```

## Current foundation

The first alpha intentionally implements the outer service boundary before a native backend.

```text
client -> Quantum Runtime -> allowlisted Ollama route -> local Ollama
```

This gives current applications a migration target and lets us test cancellation, streaming, security and API compatibility using real workloads.

It is not presented as completed independent inference.

## Packages

```text
cmd/quantum-runtime
    process startup, signals and HTTP server lifecycle

internal/config
    environment parsing and fail-closed exposure validation

internal/httpapi
    native service endpoints, compatibility policy, auth and transport

internal/ollama
    narrowly scoped initial adoption backend

internal/buildinfo
    version and release build metadata
```

## Stable boundaries

### Client boundary

Clients talk to Quantum Runtime through documented HTTP APIs. They do not import Runtime internals or assume a specific backend process.

### Backend boundary

The API layer knows only the backend interface. A native backend can replace the initial Ollama adapter without changing CoreUI or Game integration.

### Personality boundary

A model recipe or persona package belongs to the consuming product profile. Quantum Runtime stores and serves model identities, but it does not turn every consumer into the same character.

The Quantum CoreOS TCI will use Gemma 4 e4b and a dedicated personality package. Ember CoreUI and the Game retain their own identities and context services.

### Privilege boundary

Quantum Runtime never becomes the privileged system broker. Quantum Control and later `qcored` own typed administrative operations. Model output is never executed as a root command.

## Request lifecycle

1. HTTP request enters the Quantum Runtime listener.
2. Runtime creates a request ID.
3. Authentication and route policy are evaluated.
4. Method, mutation policy and body-size limits are checked.
5. A bounded context carries cancellation and timeout to the backend.
6. The backend response is copied without buffering the entire generation.
7. Streaming chunks are flushed to the client.
8. Logs contain route and status metadata, never prompt bodies.

## Failure behavior

- unsupported compatibility routes fail closed with HTTP 404
- wrong methods return HTTP 405 and an `Allow` header
- disabled model mutation returns HTTP 403
- unavailable backend returns HTTP 503 or 502 depending on stage
- upstream timeout returns HTTP 504 before response streaming begins
- client cancellation propagates to the upstream request context
- the service does not silently fall back to a different model

## Future data layout

The current adapter requires no model store of its own. A later managed installation should use conventional separation:

```text
/etc/quantum-runtime/       configuration and policy
/var/lib/quantum-runtime/   registry, manifests and managed state
/var/cache/quantum-runtime/ derived caches
/var/log/quantum-runtime/   service logs where journald is not used
/run/quantum-runtime/       sockets and transient process state
```

Model files must never be mixed with application chats, memories or secrets.

## Runtime 0.3 backend capability layer

Quantum Runtime 0.3 introduces `quantum.runtime/backend/v1alpha1` between the canonical model registry and concrete execution engines. Backend capabilities use four explicit states: `supported`, `unsupported`, `conditional`, and `unknown`. `conditional` means the engine can provide the feature only when the selected model/artifact satisfies the corresponding requirement; `unknown` never satisfies a route request.

The first configured backend remains the external Ollama adoption adapter. The router is intentionally separate from transport forwarding so future llama.cpp, MLX and vLLM adapters can be added without changing canonical model identity or client APIs. A model may now carry artifact-specific backend metadata. Routing evaluates canonical model metadata, artifact compatibility, requested capabilities and backend state, then returns a deterministic plan. It never silently substitutes a different canonical model.

CPU execution is a first-class placement capability. GPU and hybrid placement remain optional/conditional and are not assumed to be the default. The 0.3.0-alpha.1 slice does not yet execute llama.cpp natively; it establishes the interface and routing boundary required for that next step.
