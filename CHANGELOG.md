# Changelog


## 0.3.0-alpha.1 - 2026-09-05

- Added `quantum.runtime/backend/v1alpha1` with explicit supported, unsupported, conditional and unknown capability states instead of optimistic booleans.
- Added deterministic backend routing that resolves aliases to one canonical model identity, evaluates model + backend capabilities, selects an artifact/backend pair and fails closed when a requested capability is unknown or unsupported.
- Extended model-manifest v1alpha1 compatibly with artifact-specific backend/format/role metadata, dense/MoE architecture class, optional parameter/expert metadata, context-policy state, reranking/tool-streaming/reasoning-control/structured-output capability flags.
- Added a Gemma 4 26B A4B MoE reference profile without making Gemma the Runtime default.
- Added a machine-readable Gemma 4 + Ollama Turin minimal policy. `temperature=1.0`, `top_k=64` and `top_p=0.95` are known-good; context/predict/thread/repeat/seed/stop controls remain blocked-unverified by default. The policy explicitly does not claim `num_ctx` was the sole cause of the observed speedup.
- Added a machine-readable upstream ledger. The existing Ollama validation is intentionally `observed-unpinned` because the exact tested upstream version was not recorded, so it cannot silently become a latest-known-good production pin. llama.cpp/ggml is recorded as the first planned native portable backend.
- Added `/v1/backends`, `/v1/route`, `/v1/model-policies` and `/v1/upstreams` while preserving existing Ollama compatibility forwarding.
- Kept CPU-first placement visible in backend capabilities and did not introduce generic shell execution, TCI privilege access or application memory/state into the Runtime layer.

## 0.2.0-alpha.2

First non-destructive Linux service packaging and optional Ember CoreUI adoption profile.

### Added

- dedicated `quantum-runtime-installer` command with `preflight`, `status`, `install`, `repair`, `uninstall` and `coreui-profile` operations
- dedicated `quantum-runtime` system user and group creation
- hardened systemd service profile
- protected `/etc/quantum-runtime` environment and ownership-marker layout
- ownership classification using `managed`, `external` and `disabled`
- read-only detection of the existing local Ollama adoption backend
- atomic Runtime binary/unit replacement
- preservation of pre-existing Runtime configuration during updates
- activation health and readiness gates
- rollback of Runtime binary, unit, config and ownership marker on failed activation
- idempotent install and uninstall behavior
- uninstall protection for externally owned Runtime files, Ollama and model data
- explicit CoreUI profiles for Quantum Runtime and direct Ollama modes
- representative CoreUI chat end-to-end compatibility test
- systemd syntax verification
- version-consistency checks between `VERSION`, Runtime and installer binaries
- installer binary and deployment/profile files in Linux release archives
- release-time build metadata stamping for Runtime and installer binaries

### Safety boundaries

- the installer never pulls, updates, restarts, renames or deletes Ollama
- the installer never pulls, replaces, renames or deletes Ollama models
- no CoreUI database, account, session, upload, memory or secret is modified
- CoreUI adoption remains an explicit operator choice and can be reversed without rebuilding CoreUI
- existing Runtime files without the installer ownership marker are treated as `external` and are never overwritten or removed
- pre-existing Runtime configuration is never purged by uninstall
- remote Runtime exposure is not enabled automatically

## 0.2.0-alpha.1

First versioned Quantum Runtime model identity and registry contract.

### Added

- `quantum.runtime/model-manifest/v1alpha1` typed Go contract
- machine-readable JSON Schema at `schema/model-manifest-v1alpha1.schema.json`
- canonical model identifiers plus application/backend aliases
- source identity, revision, artifact and optional SHA-256 integrity metadata
- architecture, parameter class, quantization and context metadata
- text, vision, audio, embeddings, tools and thinking capability declarations
- Runtime compatibility bounds using validated semantic versions
- separate persona-package references without embedding user memory, credentials or prompts
- install, verification and lifecycle state contracts
- provenance and external-license metadata fields
- fail-closed validation for identifiers, hashes, version ranges and invalid state combinations
- embedded read-only registry with generic, Ember CoreUI and future Quantum CoreOS Gemma 4 e4b TCI examples
- `GET /v1/models`
- `GET /v1/models/{identifier}` with canonical and alias lookup
- stable `model_not_found` Runtime error response
- registry and HTTP contract tests

### Boundaries

- builtin manifests marked `unverified` are contract/profile examples, not cryptographic claims about unresolved model artifacts
- the Gemma 4 e4b profile is specific to the future Quantum CoreOS TCI and does not become the generic Runtime default
- persona packages remain separate from model identity and mutable user memory
- model download, install, remove, load and unload remain out of scope for this release
- independent native inference remains a later milestone

## 0.1.0-alpha.1

Initial Quantum Runtime foundation.

### Added

- standalone Go daemon
- loopback-safe configuration validation
- native liveness, readiness, runtime and capability endpoints
- constrained Ollama adoption backend
- chat, generation, embedding and model-read compatibility routes
- model-mutation policy disabled by default
- streamed response forwarding and cancellation propagation
- bearer-token boundary for non-loopback operation
- request IDs and prompt-free access logging
- tests, race checks, vet, formatting verification and CI
- architecture, API, security, roadmap and decision records

### Licensing

- adopted the Starlight Unit Studios Quantum Runtime Community Source License 1.0
- added controlling German and translated English license texts
- added license history, copyright, notice, trademark and third-party notice files
- added CI verification that required legal files are present and internally consistent

### Release engineering

- added `CITATION.cff`
- added automated GitHub prerelease/release publication
- added Linux amd64 and arm64 release archives with SHA-256 sums
- documented the Zenodo archival flow

### Not yet implemented

- independent native inference
- managed model store
- Quantum native chat API
- OpenAI-compatible API
- GPU scheduling
- Quantum CoreOS TCI runtime integration
