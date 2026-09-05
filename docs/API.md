# Quantum Runtime API

Status: `v1alpha1`

Quantum Runtime exposes native service inspection and model-registry endpoints plus a constrained Ollama compatibility surface.

## Native endpoints

### `GET /healthz`

Process liveness only. It does not claim that an inference backend is ready.

Example response:

```json
{
  "status": "alive",
  "service": "quantum-runtime",
  "version": "0.2.0-alpha.1"
}
```

### `GET /readyz`

Checks whether the configured initial backend answers its version probe. Returns HTTP 503 when inference is unavailable.

### `GET /v1/runtime`

Returns Runtime build identity and configured backend type.

### `GET /v1/capabilities`

Returns the implemented compatibility categories, streaming support and model-mutation policy.

### `GET /v1/models`

Returns the read-only Quantum Runtime model registry. Entries are sorted by canonical model identifier and expose summary metadata only.

Example shape:

```json
{
  "api_version": "v1alpha1",
  "schema_version": "quantum.runtime/model-manifest/v1alpha1",
  "count": 3,
  "models": [
    {
      "id": "ember-coreui",
      "display_name": "Ember CoreUI Model Profile",
      "aliases": ["ember-coreui:latest"],
      "backend": "ollama-adapter",
      "capabilities": ["text", "thinking", "vision"],
      "state": {
        "install": "external",
        "verification": "unverified",
        "lifecycle": "active"
      }
    }
  ]
}
```

The builtin examples are contract fixtures and product-profile references. An `unverified` manifest does not assert that unresolved source, context, quantization or capability metadata has been cryptographically verified against model artifacts.

### `GET /v1/models/{identifier}`

Returns the complete manifest for a canonical identifier or registered alias. Aliases resolve to the same canonical manifest without creating duplicate model identities.

Example:

```text
GET /v1/models/ember-coreui:latest
```

returns a payload containing canonical model id `ember-coreui` and the full `quantum.runtime/model-manifest/v1alpha1` object.

Unknown identifiers fail with HTTP 404:

```json
{
  "error": {
    "code": "model_not_found",
    "message": "The requested model is not present in the Quantum Runtime registry.",
    "request_id": "..."
  }
}
```

The registry is read-only in `0.2.0-alpha.1`. Install, remove, load and unload operations are intentionally not implemented yet.

## Model manifest contract

The machine-readable JSON Schema is:

```text
schema/model-manifest-v1alpha1.schema.json
```

Builtin example manifests live under:

```text
internal/modelregistry/data/
```

The contract separates:

- canonical runtime identity from application aliases
- source identity and revision
- backend type and artifact references
- optional SHA-256 artifact integrity
- architecture, parameter class, quantization and context metadata
- declared text, vision, audio, embeddings, tools and thinking capabilities
- Runtime compatibility bounds
- optional persona-package references
- install, verification and lifecycle state
- provenance and external license references

A manifest marked `verified` must use a resolved source revision and SHA-256 integrity for every artifact. Invalid identifiers, hashes, state combinations and Runtime version ranges fail closed in the Go validator.

Persona references identify a separate package. Runtime manifests do not contain mutable chats, user memory, credentials or application-specific system prompts.

## Ollama compatibility endpoints

Read and inference routes enabled in the current adoption backend:

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

The Ollama mutation switch is a compatibility-proxy policy only. It does not implement Quantum Runtime's future managed-model lifecycle.

## Streaming

Quantum Runtime forwards upstream streaming responses incrementally and flushes chunks to the client. It does not buffer a complete model answer before sending it.

## Authentication

The default listener is loopback-only and does not require a token.

When `QUANTUM_RUNTIME_AUTH_TOKEN` is configured, protected native and compatibility endpoints require:

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

CoreUI migration should initially continue to use `/api/chat`. A later native chat contract and OpenAI-compatible adapter will be specified independently rather than guessed from the proxy implementation.

## Runtime 0.3 backend and routing endpoints

`GET /v1/backends` returns configured backend descriptors using `quantum.runtime/backend/v1alpha1`.

`POST /v1/route` accepts a canonical model ID or alias plus optional required capability names. Example:

```json
{"model":"ember-coreui:latest","capabilities":["inference.text","multimodal.vision"]}
```

The response contains `requested_identifier`, `canonical_model_id`, selected backend/artifact, required capability set, backend context policy and matching model-policy IDs. Unknown capabilities fail closed with `422 no_compatible_backend`; missing models return `404 model_not_found`.

`GET /v1/model-policies` exposes machine-readable backend/model validation policies. The initial Gemma 4 + Ollama Turin policy records the minimal known-good sampling profile and classifies additional tuning knobs as blocked-unverified until isolated A/B validation.

`GET /v1/upstreams` exposes the tested-upstream ledger plus the subset currently eligible as `latest_known_good`. An `observed-unpinned` entry is informative only and cannot be promoted automatically.
