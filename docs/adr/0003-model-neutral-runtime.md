# ADR 0003: Keep Quantum Runtime model-neutral

Status: accepted for the foundation

## Decision

Quantum Runtime does not force Gemma 4 e4b or any other model as its universal default.

## Reasons

- Ember CoreUI, the Game and the Quantum CoreOS TCI have different identities and workloads
- hardware profiles differ
- model source tags and quantization are deployment decisions
- a stable runtime must support multiple model families and future backends

## CoreOS relationship

Quantum CoreOS will define a TCI profile targeting Gemma 4 e4b with a dedicated personality package. Runtime provides the serving, lifecycle and capability layer for that profile.
