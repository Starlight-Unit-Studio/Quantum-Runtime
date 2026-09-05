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
  "version": "0.3.0-alpha.4"
}
```

### `GET /readyz`

Checks whether the configured inference backend is ready. Returns HTTP 503 when inference is unavailable.

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
  "count": 4,
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

The registry remains read-only. Install, remove, load and unload operations are intentionally separate from the current registry contract.

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

Quantum Runtime forwards or translates upstream streaming responses incrementally and flushes chunks to the client. It does not buffer a complete model answer before sending it.

## Authentication

The default listener is loopback-only and does not require a token.

When `QUANTUM_RUNTIME_AUTH_TOKEN` is configured, protected native and compatibility endpoints require:

```http
Authorization: Bearer <token>
```

The Runtime bearer token, cookies and forwarding headers are stripped before a request reaches an inference backend. Optional llama.cpp credentials use their own configuration and are never copied from Runtime bearer credentials.

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

## llama.cpp direct compatibility bridge (0.3.0-alpha.2+)

When `QUANTUM_RUNTIME_BACKEND=llama.cpp`, the existing application-facing Ollama compatibility routes are translated to a directly configured `llama-server`; Ollama is not present in that request path.

- `/api/chat` -> `/v1/chat/completions`
- `/api/generate` -> `/v1/completions`
- `/api/embed` and `/api/embeddings` -> `/v1/embeddings`
- `/api/tags`, `/api/show`, `/api/ps` and `/api/version` are synthesized from the configured Runtime model identity because llama.cpp does not expose Ollama's model-store contract

The configured `QUANTUM_RUNTIME_LLAMA_CPP_MODEL` must match the client-visible model identifier. A mismatched model request returns an error instead of silently substituting another canonical identity. Only `temperature`, `top_p` and `top_k` are normalized from the current Ollama `options` object in this direct path. Per-request `num_ctx`, predict/thread/repeat/seed/stop tuning is not injected.

Vision, tool calls/tool history, explicit reasoning control and structured-output requests currently return `422 unsupported_backend_capability` on this direct adapter. This is intentional: the backend contract must not claim a capability until Runtime preserves its semantics end to end. Streaming content is translated from llama.cpp SSE to Ollama NDJSON; llama.cpp `reasoning_content`/`reasoning` deltas are preserved as the Ollama-compatible `message.thinking` field when present.

## Host discovery, calibration and placement (0.3.0-alpha.3+)

`GET /v1/host` returns the current host profile using `quantum.runtime/host-profile/v1alpha1`. The Linux discovery path reports OS-visible CPU topology/features, NUMA nodes, RAM and huge-page metadata, storage/NVMe devices and visible accelerator metadata.

`POST /v1/host/calibrate` runs an explicit bounded synthetic memory copy/read and worker-count sweep. It is not a model tokens-per-second benchmark.

`POST /v1/placement` creates a pre-activation CPU-first capacity plan using separate model-weight, MoE-expert, KV-cache, prefix-cache, projector, workspace and explicit cold-tier memory classes. A visible accelerator does not automatically win, and hot inference state is never silently spilled to disk.

## Guest/process CPU limits (0.3.0-alpha.4)

`GET /v1/host`, calibration and placement responses now include a separate `limits` object using `quantum.runtime/host-limits/v1alpha1`.

The hardware profile and effective guest/process limits are deliberately distinct. Runtime evaluates:

- the process `Cpus_allowed_list`
- cgroup cpuset limits where exposed
- cgroup CPU quota as a separate scheduling signal
- virtualization evidence where available

A physical CPU model name never implies that all host cores are allocated to a virtual machine. For example, an AMD EPYC 9645 host may physically contain 96 cores while a KVM guest exposes and is allowed to use a much smaller dedicated CPU set.

## Application deployment profiles

`GET /v1/deployment-profiles` returns `quantum.runtime/deployment-profile/v1alpha1` profiles.

The builtin `ember-production` profile is application-specific and does not become a generic Quantum Runtime minimum. Its current policy is:

```text
memory minimum:       64 GiB
ECC:                  required
memory class:         DDR5 preferred
physical CPU cores:   8 minimum
reference clock:      ~2600 MHz advisory only
accelerator:          optional
model architecture:   MoE required
```

The raw clock reference is never used as a hard admission cutoff. Calibrated performance, CPU capabilities and memory/topology evidence are preferred when available.

## E.M.B.E.R. admission

`POST /v1/admission` evaluates a deployment profile against a canonical model and the current host/guest evidence.

Example for a provider-verified KVM deployment:

```json
{
  "profile": "ember-production",
  "model": "gemma4:26b-a4b-reference",
  "operator_evidence": {
    "ecc_verified": true,
    "memory_class": "ddr5",
    "dedicated_physical_cores": 20,
    "core_budget": 16
  }
}
```

The result uses `quantum.runtime/admission-result/v1alpha1` and returns one of:

- `admitted`
- `rejected`
- `needs_operator_evidence`

Properties that normal guest userspace cannot prove reliably, such as provider-guaranteed ECC or dedicated physical cores, remain explicit operator evidence rather than guessed facts. Unknown mandatory evidence therefore produces `needs_operator_evidence`. A dense model cannot satisfy a profile requiring `architecture_class=moe` merely because it belongs to an otherwise supported model family.

`core_budget` is an application scheduling boundary, not a claim about host topology. It lets an operator keep part of a guest allocation free for other services while admitting E.M.B.E.R. against a smaller production budget.

## Real-model CPU benchmark planning

`POST /v1/benchmark-plan` creates a repeatable worker-count experiment matrix for a canonical model. It does not start inference and does not invent benchmark values.

Example:

```json
{
  "model": "gemma4:26b-a4b-reference",
  "reserve_system_cores": 4,
  "minimum_workers": 8,
  "include_full_host_comparison": true
}
```

On a 20-CPU effective guest allocation, this produces:

```text
8 workers    production candidate
12 workers   production candidate
16 workers   production candidate
20 workers   full-host comparison
```

The benchmark plan is only an experiment definition. Prefill throughput, decode throughput, TTFT, memory footprint and the exact backend/model/quantization/context tuple still require real measurements before Runtime can promote a worker count or placement to latest-known-good.
