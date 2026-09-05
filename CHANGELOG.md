# Changelog

## 0.3.0-alpha.4 - 2026-09-06

- Added `quantum.runtime/deployment-profile/v1alpha1` with a builtin `ember-production` application profile kept separate from generic Quantum Runtime minimums.
- Encoded the current supported E.M.B.E.R. production requirements as 64 GiB memory, ECC required, DDR5 preferred, 8 physical cores, GPU optional and MoE architecture mandatory; the historical ~2.6 GHz reference remains advisory only.
- Added `quantum.runtime/admission-result/v1alpha1` with explicit `admitted`, `rejected` and `needs_operator_evidence` decisions and per-requirement evidence instead of guessing missing ECC/provider facts.
- Added guest/process CPU-limit discovery from process affinity, cgroup cpusets and CPU quota plus virtualization evidence so an EPYC host model name cannot imply that all host-physical cores are allocated to a KVM guest.
- Added operator core-budget evidence so application admission can reserve guest CPU capacity for other services without changing global Runtime topology.
- Added `quantum.runtime/model-benchmark-plan/v1alpha1` to generate repeatable CPU worker-count matrices without claiming unmeasured throughput; a 20-core guest with four reserved system cores produces the intended 8/12/16 production candidates plus an optional 20-core full-host comparison.
- Added `/v1/deployment-profiles`, `/v1/admission` and `/v1/benchmark-plan`; `/v1/host`, calibration and placement responses now also expose guest/process CPU limits.
- Added `docs/EMBER-DEPLOYMENT-HISTORY.md` distinguishing historical functional compatibility with small models/RAG-Lite/LTM from the current supported E.M.B.E.R. production/intelligence profile.
- Recorded the RS 4000 G12 (12 dedicated cores, 32 GB DDR5 ECC, 1 TB NVMe, Gemma 3 12B Q4) as a historical practical-use reference and the RS 12000 G12 (20 dedicated guest cores, 96 GB DDR5 ECC, ~3 TB NVMe RAID-10, KVM, Debian 13, 16-core Ember budget) as the current primary reference host.
- Kept exact hardware for the earliest low-cost Netcup root-server generation unknown rather than reconstructing missing values from current product offerings.

## 0.3.0-alpha.3 - 2026-09-05

- Added `quantum.runtime/host-profile/v1alpha1` Linux discovery for CPU vendor/model/features, physical/logical CPUs, SMT ratio, NUMA nodes, RAM/huge-page metadata, block/NVMe devices and visible AMD/NVIDIA/Intel accelerator metadata.
- Added explicit bounded host calibration with representative physical-core and logical/SMT worker candidates, recording synthetic memory copy/read throughput and the best observed worker count without benchmarking automatically at daemon startup.
- Added `quantum.runtime/placement-plan/v1alpha1` with separate model-weight, MoE-expert, KV-cache, prefix-cache, projector, workspace and cold-tier memory classes.
- Implemented the first CPU-first placement policy: RAM/CPU is evaluated first and remains selected when the hot working set fits, even when an accelerator is visible.
- Added pre-activation hybrid candidates only after CPU-only capacity fails and acceleration is explicitly allowed; model/MoE weights may split across RAM/VRAM while cache/workspace classes remain whole-tier in this slice.
- Added an explicit NVMe cold tier. Hot execution state is never silently spilled to disk; insufficient RAM/VRAM capacity returns `capacity_exceeded` instead of relying on uncontrolled swap.
- Added `/v1/host`, `/v1/host/calibrate` and `/v1/placement` plus an in-memory resource-control boundary for the last bounded calibration.
- Added `docs/HARDWARE-PLACEMENT.md` documenting the generic CPU-first contract, current scope limits and the AMD EPYC Turin Tier-1 reference track.
- Kept real-model prefill/decode benchmarking, NUMA/thread affinity, GGUF/KV size estimation, persistent calibration and automatic backend activation out of this slice until they can be validated independently.

## 0.3.0-alpha.2 - 2026-09-05

- Added the first direct llama.cpp/ggml execution adapter using the external `llama-server` process boundary; selecting it removes Ollama from the inference request path without vendoring upstream code.
- Added explicit `QUANTUM_RUNTIME_BACKEND=ollama|llama.cpp` selection while keeping Ollama as the default adoption/fallback mode.
- Added direct Ollama-compatibility translation for chat, generation and embeddings using llama.cpp `/v1/chat/completions`, `/v1/completions` and `/v1/embeddings`, plus synthetic model-read compatibility responses.
- Preserved streaming content by translating llama.cpp SSE to Ollama NDJSON and preserved reasoning text as `message.thinking` when upstream emits `reasoning_content`/`reasoning`.
- Enforced model-identity matching between `QUANTUM_RUNTIME_LLAMA_CPP_MODEL` and the client-visible request model; no silent model substitution is allowed.
- Kept the first llama.cpp bridge deliberately fail-closed for vision, tools/tool history, explicit reasoning control and structured output until those semantics are normalized end to end.
- Kept per-request context/predict/thread/repeat/seed/stop tuning out of the direct bridge; only `temperature`, `top_p` and `top_k` are normalized in this slice.
- Added optional separate llama-server API-key configuration. Runtime bearer credentials are never forwarded as llama.cpp credentials.
- Updated the upstream ledger from planned to implemented-but-unpinned protocol evidence. No llama.cpp tag/commit is promoted to latest-known-good until real model/hardware conformance passes.

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

- fully managed/bundled native inference engine lifecycle (the first direct llama.cpp process adapter now exists)
- managed model store
- Quantum native chat API
- OpenAI-compatible API
- GPU scheduling
- Quantum CoreOS TCI runtime integration
