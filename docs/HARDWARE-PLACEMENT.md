# Hardware discovery, calibration and CPU-first placement

Status: `v1alpha1` foundation introduced in `0.3.0-alpha.3`.

Quantum Runtime treats CPU/RAM as the mandatory baseline and GPUs as optional accelerators. Placement is decided before model activation; this layer does not migrate arbitrary live inference state between tiers.

## Host profile

`GET /v1/host` reports the current host using `quantum.runtime/host-profile/v1alpha1`.

The Linux discovery path currently records:

- OS and architecture
- CPU vendor/model, physical and logical CPU counts, threads per core and OS-visible instruction features
- NUMA node count, CPU lists and node memory where exposed by sysfs
- total/available RAM and huge-page metadata
- block devices, NVMe classification, rotational flag, capacity and NUMA locality where exposed
- DRM-visible AMD/NVIDIA/Intel accelerator devices, optional VRAM total and NUMA locality where exposed

Discovery is fail-soft: unavailable optional kernel/sysfs data becomes a warning instead of being guessed. CPU and memory invariants still validate fail-closed before the profile is returned as usable.

No hardware name alone selects an optimized kernel. A Turin-class EPYC host is useful evidence, but Runtime must confirm OS-visible features and later real-model conformance results.

## Bounded calibration

`POST /v1/host/calibrate` runs a short bounded memory copy/read calibration and records a worker-count sweep across representative CPU counts, including physical-core and SMT/logical-CPU candidates where present.

The initial calibration is intentionally lightweight. It provides:

- per-worker memory copy throughput
- per-worker memory read throughput
- the best observed worker count for the synthetic memory test
- a coarse memory-bandwidth class

This is not yet a model tokens-per-second benchmark and must not be presented as one. It is a host-resource signal used to avoid naive assumptions such as "all logical CPUs are always faster". Real prefill/decode calibration remains tied to backend/model/quantization/context profiles in later slices.

Calibration is explicit rather than automatic at daemon startup so a bounded synthetic benchmark does not unexpectedly consume host resources during service activation.

## Placement plan

`POST /v1/placement` accepts an explicit capacity request using separate memory classes:

- `model_bytes`
- `moe_expert_bytes`
- `kv_cache_bytes`
- `prefix_cache_bytes`
- `projector_bytes`
- `workspace_bytes`
- `cold_bytes`

The first planner follows these rules:

1. Reserve host RAM for the OS/operator workload.
2. Build a CPU-only candidate first.
3. If the hot working set fits usable RAM, choose `cpu_only` even when a GPU exists.
4. Only when CPU-only capacity does not fit and acceleration is explicitly allowed may the planner build a `hybrid_candidate`.
5. Model weights and MoE expert weights may be split across VRAM/RAM as a candidate; active caches/workspaces remain whole-tier allocations in this slice.
6. NVMe is an explicit secondary tier only for operator-declared cold bytes. Hot model execution state is never silently spilled to disk.
7. If known RAM/VRAM capacity cannot hold hot state, return `capacity_exceeded` instead of relying on uncontrolled OS swap.

A `hybrid_candidate` is not yet a claim that GPU execution is faster. Runtime must still require backend capability support and measured performance before such a placement can become the preferred production plan.

## API examples

Inspect the host:

```bash
curl -s http://127.0.0.1:11450/v1/host
```

Run bounded calibration:

```bash
curl -s -X POST http://127.0.0.1:11450/v1/host/calibrate
```

Ask for a CPU-first capacity plan:

```bash
curl -s -X POST http://127.0.0.1:11450/v1/placement \
  -H 'Content-Type: application/json' \
  -d '{
    "model_bytes": 25769803776,
    "moe_expert_bytes": 8589934592,
    "kv_cache_bytes": 2147483648,
    "allow_acceleration": true
  }'
```

## Current boundaries

`0.3.0-alpha.3` does not yet:

- launch or reconfigure llama-server from a placement plan
- set NUMA affinity or thread pinning automatically
- estimate model/KV sizes from GGUF metadata automatically
- benchmark real model prefill/decode throughput
- persist host calibration across daemon restarts
- measure NVMe throughput or enforce a disk cache directory/quota/journal
- query vendor APIs for dynamic free VRAM
- guarantee hybrid execution is faster than CPU-only

Those boundaries are deliberate. The purpose of this slice is to establish truthful host/resource contracts before backend-specific activation and real-model auto-tuning are layered on top.

## Turin reference track

AMD EPYC Turin remains the Tier-1 CPU-only reference target from issue #12. The generic API contains no AMD-only fields. Turin-specific quality comes from feeding its OS-visible CPU features, NUMA topology, memory capacity/bandwidth and later real-model benchmark results through the same generic host/calibration/placement contracts.
