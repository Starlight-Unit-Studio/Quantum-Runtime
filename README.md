# Quantum Runtime

[![DOI](https://zenodo.org/badge/1356057370.svg)](https://doi.org/10.5281/zenodo.22287448)

Quantum Runtime is the independent AI runtime and model-service project of Starlight Unit Studios.

Current version: `0.3.0-alpha.3`

## Current alpha scope

Quantum Runtime 0.3 adds the first model- and engine-neutral backend capability foundation while retaining the non-destructive Ollama adoption path. It now owns these reusable boundaries:

1. the inference service endpoint used by Ember CoreUI, STΛRLIGHT UNIT The Game and later Quantum CoreOS
2. the versioned model identity and read-only registry contract
3. a non-destructive Linux service package that can be installed beside an existing Ollama service
4. host-resource discovery, bounded calibration and a CPU-first placement contract for RAM/VRAM/NVMe candidates

Quantum Runtime now has two local execution paths. Ollama remains the default adoption/fallback mode, while a direct llama.cpp path can talk to `llama-server` without an Ollama daemon in the inference request path:

```text
Ember CoreUI / Game / client
              |
              v
      Quantum Runtime :11450
          /           \
         v             v
 llama-server :8080   Ollama :11434
 direct local path    adoption/fallback
```

The initial llama.cpp adapter deliberately uses the external `llama-server` process boundary rather than vendoring or forking upstream code. Quantum Runtime owns the application-facing endpoint, request policy, authentication boundary, compatibility translation and capability reporting; llama.cpp owns the low-level GGUF inference engine. Ollama remains the default until an operator explicitly selects `llama.cpp`.

## Current capabilities

- standalone Go Runtime service with no third-party Go dependencies
- versioned `quantum.runtime/model-manifest/v1alpha1` contract and read-only registry
- versioned `quantum.runtime/backend/v1alpha1` capability contract with explicit supported/unsupported/conditional/unknown states
- deterministic capability router that preserves canonical model identity and fails closed on unsupported requirements
- machine-readable model/backend policy for the validated Gemma 4 + Ollama minimal profile
- machine-readable upstream ledger where unpinned observations cannot become latest-known-good production pins
- Ollama-compatible chat, generation, embedding and model-read routes
- first direct llama.cpp/ggml backend path using `llama-server`, with Ollama absent from the request path when selected
- Ollama chat/generate/embed compatibility translation to llama.cpp OpenAI-style endpoints
- fail-closed handling for llama.cpp features not yet normalized: vision, tools, reasoning control and structured output
- Linux host discovery for CPU topology/features, NUMA, RAM/huge pages, block/NVMe devices and visible accelerator/VRAM metadata
- explicit bounded memory-bandwidth/worker-count calibration rather than automatic startup benchmarking
- versioned CPU-first placement plans that prefer RAM/CPU when the hot set fits and only consider hybrid acceleration after CPU capacity fails
- explicit NVMe cold-tier placement; hot execution state is never silently spilled to disk
- streamed forwarding/translation, cancellation, body limits and backend timeouts
- loopback-only default on `127.0.0.1:11450`
- bearer-token requirement for any non-loopback bind
- model mutation disabled by default
- dedicated `quantum-runtime-installer` command
- inspect-only preflight and machine-readable status
- dedicated `quantum-runtime` system user/group
- hardened systemd service
- `managed`, `external` and `disabled` ownership states
- transactional binary/unit/config/marker rollback on activation failure
- preservation of existing Runtime configuration during updates
- optional Ember CoreUI Quantum Runtime and direct-Ollama profiles
- idempotent install/uninstall behavior with external Ollama/model protection
- automated Linux amd64/arm64 GitHub releases with SHA-256 sums and Zenodo archival support

## Run locally

```bash
go run ./cmd/quantum-runtime
```

Check the service, registry and host-resource contracts:

```bash
curl -s http://127.0.0.1:11450/healthz
curl -s http://127.0.0.1:11450/readyz
curl -s http://127.0.0.1:11450/v1/runtime
curl -s http://127.0.0.1:11450/v1/models
curl -s http://127.0.0.1:11450/v1/backends
curl -s http://127.0.0.1:11450/v1/model-policies
curl -s http://127.0.0.1:11450/v1/upstreams
curl -s http://127.0.0.1:11450/v1/host
curl -s -X POST http://127.0.0.1:11450/v1/host/calibrate
```

Run all verification:

```bash
./scripts/verify.sh
```

## Linux installation

Release archives contain both `quantum-runtime` and `quantum-runtime-installer`.

Inspect a host without changing it:

```bash
sudo ./quantum-runtime-installer preflight
sudo ./quantum-runtime-installer preflight --json
```

Install beside an existing local Ollama service:

```bash
sudo ./quantum-runtime-installer install --runtime-binary ./quantum-runtime
```

The installer refuses to overwrite Runtime files that it does not own, preserves existing local Runtime configuration, validates `/healthz` and `/readyz`, and restores the previous managed Runtime state when activation fails. It never manages Ollama or Ollama model files.

See `docs/INSTALLATION.md` for install, status, repair, uninstall and rollback behavior.

## Model registry

The machine-readable schema lives at:

```text
schema/model-manifest-v1alpha1.schema.json
```

Builtin contract/profile examples live under `internal/modelregistry/data/` for a generic model, Ember CoreUI and the future Quantum CoreOS Gemma 4 e4b TCI profile.

The manifest separates canonical model identity from aliases, source revision, backend, artifacts, SHA-256 integrity, capabilities, compatibility, persona package, lifecycle state and provenance. In 0.3, artifacts may additionally declare their own backend/format/role, allowing one canonical identity to acquire multiple backend artifacts without changing the client-facing model ID. Generic architecture metadata now distinguishes dense, MoE and unknown profiles and can carry context-policy and expert-topology data without exposing family-specific fields in the public API. An `unverified` example is a contract/profile reference, not a cryptographic claim about unresolved model artifacts.

## Direct llama.cpp backend

`0.3.0-alpha.2` and later can use an already running `llama-server` directly. The Runtime does not download, replace or manage the llama.cpp binary or GGUF files in this slice. Configure the server with an API-visible alias that matches the model identifier used by the client, for example:

```bash
llama-server -m /path/model.gguf --host 127.0.0.1 --port 8080 --alias ember-coreui:latest
```

Then start Quantum Runtime with:

```bash
QUANTUM_RUNTIME_BACKEND=llama.cpp \
QUANTUM_RUNTIME_LLAMA_CPP_URL=http://127.0.0.1:8080 \
QUANTUM_RUNTIME_LLAMA_CPP_MODEL=ember-coreui:latest \
go run ./cmd/quantum-runtime
```

If the llama.cpp server uses `--api-key`, set `QUANTUM_RUNTIME_LLAMA_CPP_API_KEY` locally. Runtime bearer credentials are never reused as llama.cpp credentials. The initial bridge supports text chat, text generation, embeddings and model-read compatibility. Unsupported modalities/capabilities fail closed instead of being silently dropped.

## CPU-first host placement

`0.3.0-alpha.3` adds the first generic hardware/resource contract. `GET /v1/host` reports OS-visible CPU, NUMA, RAM, storage and accelerator metadata. `POST /v1/host/calibrate` runs an explicit bounded synthetic memory/worker sweep. `POST /v1/placement` creates a pre-activation capacity plan for model weights, MoE experts, caches, projectors, workspace and an optional explicit cold NVMe tier.

CPU/RAM remains the baseline. A visible GPU does not automatically win: if the hot working set fits usable RAM, the first planner returns `cpu_only`. Hybrid placement is only considered after CPU-only capacity fails and acceleration was explicitly allowed. NVMe is never used as an implicit escape hatch for hot state.

This is a truthful capacity/host foundation, not yet a claim of measured model throughput. Real backend/model prefill/decode calibration, NUMA affinity, thread pinning, GGUF size estimation and backend activation from placement plans remain later Runtime 0.3 slices. See `docs/HARDWARE-PLACEMENT.md`.

## Ember CoreUI adoption

Ember CoreUI remains an independent Repack. Quantum Runtime is optional and Quantum CoreOS is not required.

Route CoreUI through Quantum Runtime:

```text
COREUI_OLLAMA_URL=http://127.0.0.1:11450/api/chat
STU_EMBER_OLLAMA_URL=http://127.0.0.1:11450/api/chat
```

Return to direct Ollama without rebuilding CoreUI:

```text
COREUI_OLLAMA_URL=http://127.0.0.1:11434/api/chat
STU_EMBER_OLLAMA_URL=http://127.0.0.1:11434/api/chat
```

Both profiles are provided under `profiles/coreui/`, and the installer can print either profile without editing CoreUI:

```bash
./quantum-runtime-installer coreui-profile --mode runtime
./quantum-runtime-installer coreui-profile --mode ollama
```

Quantum Runtime does not edit CoreUI databases, accounts, sessions, uploads, memories, secrets or model data.

## Model policy

Quantum Runtime is model-neutral. It does not force one model on every consumer.

The future Quantum CoreOS TCI profile targets **Gemma 4 e4b** and references its own TCI persona package. That target belongs to the CoreOS TCI profile, not to the generic Runtime default.

## Configuration

| Variable | Default | Purpose |
|---|---:|---|
| `QUANTUM_RUNTIME_LISTEN` | `127.0.0.1:11450` | HTTP listen address |
| `QUANTUM_RUNTIME_BACKEND` | `ollama` | Active backend: `ollama` or `llama.cpp` |
| `QUANTUM_RUNTIME_OLLAMA_URL` | `http://127.0.0.1:11434` | Ollama adoption/fallback endpoint |
| `QUANTUM_RUNTIME_LLAMA_CPP_URL` | `http://127.0.0.1:8080` | Direct llama-server endpoint |
| `QUANTUM_RUNTIME_LLAMA_CPP_MODEL` | empty | Required model/API alias when `llama.cpp` is selected |
| `QUANTUM_RUNTIME_LLAMA_CPP_API_KEY` | empty | Optional llama-server API key; never logged |
| `QUANTUM_RUNTIME_UPSTREAM_TIMEOUT` | `15m` | Maximum backend request duration |
| `QUANTUM_RUNTIME_MAX_REQUEST_BYTES` | `134217728` | Request body limit |
| `QUANTUM_RUNTIME_ALLOW_MODEL_MUTATION` | `false` | Compatibility mutation proxy policy |
| `QUANTUM_RUNTIME_AUTH_TOKEN` | empty | Required for any non-loopback bind |

The process rejects a network-wide bind when no bearer token is configured.

## Project boundaries

```text
Quantum Runtime
    model identity, lifecycle, inference APIs, streaming, backend adapters,
    host discovery, calibration and placement policy

Quantum Control
    server, hosting, service, database, backup and update management

Quantum CoreOS
    final operating-system integration of released Runtime and Control modules
```

Quantum CoreOS consumes released Quantum Runtime packages rather than maintaining a private fork.

## Documentation

- `docs/ARCHITECTURE.md`
- `docs/API.md`
- `docs/HARDWARE-PLACEMENT.md`
- `docs/INSTALLATION.md`
- `docs/ROADMAP.md`
- `docs/SECURITY.md`
- `docs/RELEASING.md`
- `docs/LICENSE-POLICY.md`

## License

Quantum Runtime project-owned code is licensed under the **Starlight Unit Studios Quantum Runtime Community Source License 1.0**.

Private/internal use, internal commercial use and Integrated Application Use are permitted under the license conditions. Quantum Runtime itself may not be sold, white-labeled or offered as a paid standalone general-purpose Runtime service. Third-party inference engines, models, model weights, datasets and tools retain their own terms.

The legally controlling German text is `LICENSE.de.md`; `LICENSE.md` is an English convenience translation. This is a custom Source Available license and is not an OSI-approved open-source license.