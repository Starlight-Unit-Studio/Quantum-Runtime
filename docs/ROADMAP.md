# Quantum Runtime Roadmap

## 0.1 - service boundary and adoption backend

- standalone daemon
- native health, version and capability endpoints
- loopback-safe defaults
- constrained Ollama compatibility proxy
- streaming and cancellation
- model-mutation policy
- tests and CI

This phase makes Quantum Runtime usable as an integration layer, not yet as an independent inference engine.

## 0.2 - model registry and lifecycle contracts

- model identity and alias rules
- manifest schema
- list, inspect, install, remove, load and unload contracts
- checksums and atomic staging
- disk-space and retention policy
- compatibility response tests for current CoreUI and Game traffic

## 0.3 - native backend interface

- backend capability interface
- first local inference backend adapter
- managed process lifecycle
- context and KV-cache ownership
- GPU/CPU selection
- cancellation under real generation load
- deterministic backend conformance suite

The initial native backend may reuse a mature low-level inference engine. Quantum Runtime does not need to reimplement accelerator kernels in its first generation.

## 0.4 - native API and client adapters

- Quantum native chat streaming contract
- OpenAI-compatible adapter
- formal Ollama compatibility test matrix
- embeddings
- multimodal capability negotiation
- audio capability negotiation
- structured model and runtime metrics

## 0.5 - first product integrations

- optional Ember CoreUI Runtime profile
- optional STΛRLIGHT UNIT Game Runtime profile
- migration and rollback from direct Ollama endpoints
- independent persona and context isolation
- installer ownership states: `managed`, `external`, `disabled`

## 0.6 - scheduling foundation

- GPU and VRAM inventory
- model residency policy
- queues and priorities
- per-application limits
- shared model process where safe
- TCI responsiveness class

## 0.7 and later - Quantum CoreOS preparation

- hardened service package
- local IPC exploration
- Quantum Control health integration
- Gemma 4 e4b CoreOS TCI profile support
- compatibility matrix consumed by Quantum CoreOS

Quantum CoreOS integration begins only after Runtime and Control exist as independently testable projects.
