# Changelog

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
