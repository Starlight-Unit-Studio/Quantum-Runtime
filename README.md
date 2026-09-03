# Quantum Runtime

Quantum Runtime is the independent AI runtime and model-service project of Starlight Unit Studios.

Current version: `0.2.0-alpha.2`

## Current alpha scope

Quantum Runtime now owns three reusable boundaries:

1. the inference service endpoint used by Ember CoreUI, STΛRLIGHT UNIT The Game and later Quantum CoreOS
2. the versioned model identity and read-only registry contract
3. a non-destructive Linux service package that can be installed beside an existing Ollama service

Inference still runs in **Ollama adoption mode**:

```text
Ember CoreUI / Game / client
              |
              v
      Quantum Runtime :11450
              |
              v
       existing Ollama :11434
```

Quantum Runtime owns the application-facing endpoint, request policy, authentication boundary, health reporting, timeouts, compatibility allowlist, model-manifest contract and Runtime service lifecycle. It does not yet execute model inference independently.

## Current capabilities

- standalone Go Runtime service with no third-party Go dependencies
- versioned `quantum.runtime/model-manifest/v1alpha1` contract and read-only registry
- Ollama-compatible chat, generation, embedding and model-read routes
- streamed forwarding, cancellation, body limits and backend timeouts
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

Check the service and registry:

```bash
curl -s http://127.0.0.1:11450/healthz
curl -s http://127.0.0.1:11450/readyz
curl -s http://127.0.0.1:11450/v1/runtime
curl -s http://127.0.0.1:11450/v1/models
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

The manifest separates canonical model identity from aliases, source revision, backend, artifacts, SHA-256 integrity, capabilities, compatibility, persona package, lifecycle state and provenance. An `unverified` example is a contract/profile reference, not a cryptographic claim about unresolved model artifacts.

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
| `QUANTUM_RUNTIME_OLLAMA_URL` | `http://127.0.0.1:11434` | Initial adoption backend |
| `QUANTUM_RUNTIME_UPSTREAM_TIMEOUT` | `15m` | Maximum backend request duration |
| `QUANTUM_RUNTIME_MAX_REQUEST_BYTES` | `134217728` | Request body limit |
| `QUANTUM_RUNTIME_ALLOW_MODEL_MUTATION` | `false` | Compatibility mutation proxy policy |
| `QUANTUM_RUNTIME_AUTH_TOKEN` | empty | Required for any non-loopback bind |

The process rejects a network-wide bind when no bearer token is configured.

## Project boundaries

```text
Quantum Runtime
    model identity, lifecycle, inference APIs, streaming, backend adapters

Quantum Control
    server, hosting, service, database, backup and update management

Quantum CoreOS
    final operating-system integration of released Runtime and Control modules
```

Quantum CoreOS consumes released Quantum Runtime packages rather than maintaining a private fork.

## Documentation

- `docs/ARCHITECTURE.md`
- `docs/API.md`
- `docs/INSTALLATION.md`
- `docs/ROADMAP.md`
- `docs/SECURITY.md`
- `docs/RELEASING.md`
- `docs/LICENSE-POLICY.md`

## License

Quantum Runtime project-owned code is licensed under the **Starlight Unit Studios Quantum Runtime Community Source License 1.0**.

Private/internal use, internal commercial use and Integrated Application Use are permitted under the license conditions. Quantum Runtime itself may not be sold, white-labeled or offered as a paid standalone general-purpose Runtime service. Third-party inference engines, models, model weights, datasets and tools retain their own terms.

The legally controlling German text is `LICENSE.de.md`; `LICENSE.md` is an English convenience translation. This is a custom Source Available license and is not an OSI-approved open-source license.
