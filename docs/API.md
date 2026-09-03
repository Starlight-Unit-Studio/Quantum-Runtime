# Quantum Runtime API

Status: `v1alpha1`

The current alpha exposes native service inspection endpoints and a constrained Ollama compatibility surface.

## Native endpoints

### `GET /healthz`

Process liveness only. It does not claim that an inference backend is ready.

Example response:

```json
{
  "status": "alive",
  "service": "quantum-runtime",
  "version": "0.1.0-alpha.1"
}
```

### `GET /readyz`

Checks whether the configured initial backend answers its version probe. Returns HTTP 503 when inference is unavailable.

### `GET /v1/runtime`

Returns Runtime build identity and configured backend type.

### `GET /v1/capabilities`

Returns the implemented compatibility categories, streaming support and model-mutation policy.

## Ollama compatibility endpoints

Read and inference routes enabled in the foundation:

```text
POST   /api/chat
POST   /api/generate
POST   /api/embed
POST   /api/embeddings
GET    /api/tags
POST   /api/show
GET    /api/ps
GET    /api/version
```

Model mutation routes exist but are denied unless `QUANTUM_RUNTIME_ALLOW_MODEL_MUTATION=true`:

```text
POST   /api/pull
POST   /api/create
POST   /api/copy
DELETE /api/delete
```

Any unlisted `/api/*` path fails closed and is never forwarded.

## Streaming

Quantum Runtime forwards upstream streaming responses incrementally and flushes chunks to the client. It does not buffer a complete model answer before sending it.

## Authentication

The default listener is loopback-only and does not require a token.

When `QUANTUM_RUNTIME_AUTH_TOKEN` is configured, protected endpoints require:

```http
Authorization: Bearer <token>
```

The Runtime bearer token, cookies and forwarding headers are stripped before a request reaches the initial Ollama backend.

`/healthz` and `/readyz` remain available for local service supervision.

## Error shape

Runtime-generated errors use:

```json
{
  "error": {
    "code": "model_mutation_disabled",
    "message": "Model mutation endpoints are disabled by policy.",
    "request_id": "..."
  }
}
```

Upstream Ollama responses are preserved for compatibility after a request has been accepted and forwarded.

## Compatibility promise

`v1alpha1` is intentionally not a stable 1.0 contract. Changes must be documented in `CHANGELOG.md`.

CoreUI migration should initially use `/api/chat`. A later native chat contract and OpenAI-compatible adapter will be specified independently rather than guessed from the proxy implementation.
