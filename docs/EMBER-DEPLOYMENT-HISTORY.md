# E.M.B.E.R. deployment history and reference classes

Status: operator-observed deployment evidence used to guide Runtime profiles and benchmark planning. These observations are not generic Quantum Runtime minimum requirements.

## Why this history matters

E.M.B.E.R. has run across several hosting generations. The history demonstrates that two different questions must remain separate:

1. **Can the application construct run?** Small models plus RAG-Lite, long-term memory and the surrounding application stack can run on substantially smaller systems.
2. **Does the host satisfy the current supported E.M.B.E.R. production/intelligence profile?** The current production profile is deliberately stricter and requires a MoE model plus its application-level hardware baseline.

Quantum Runtime must never turn the production E.M.B.E.R. profile into a global Runtime minimum.

## Earliest Netcup root-server generation

The studio reports that the E.M.B.E.R. construct already operated on the smallest Netcup root-server tier in use at the time (about EUR 12/month) when paired with sufficiently small models. RAG-Lite and long-term memory were functional.

The exact CPU/RAM specification for that historical machine has not yet been recorded in the Runtime evidence set. Runtime therefore does **not** invent or infer concrete minimum values from this observation.

Classification: **functional compatibility evidence only**.

## Netcup RS 4000 G12

Observed configuration:

```text
Host CPU class: AMD EPYC 9645 (Turin)
Allocated CPU:  12 dedicated cores
Memory:         32 GB DDR5 ECC
Storage:        1 TB NVMe
Model:          Gemma 3 12B Q4
```

This generation is reported as the point at which the E.M.B.E.R. construct became practically usable with Gemma 3 12B Q4 rather than merely operational with smaller models.

Classification: **historical practical-use reference**, not the current production profile.

## Netcup RS 12000 G12 - current primary reference

Operator-provided configuration:

```text
Host CPU class:     AMD EPYC 9645 (Turin), 96-core physical host processor
Guest allocation:   20 dedicated CPU cores
Ember core budget:  16 cores
Memory:             96 GB DDR5 ECC
Storage:            approximately 3 TB NVMe SSD, RAID-10
Network:            2.5 Gbit/s
Virtualization:     KVM
Operating system:   Debian 13 Trixie
Datacenter:         Nuremberg, Germany
```

The remaining CPU capacity outside the 16-core Ember budget is intentionally available to the studio website, game and system workload.

The important Runtime rule is that the CPU model name must **not** turn into a 96-core guest allocation. On KVM, placement and benchmark planning use process affinity, cgroup/cpuset limits and guest-visible topology. Provider guarantees such as "dedicated cores", ECC and memory generation may be supplied as operator evidence when ordinary guest userspace cannot prove them reliably.

Classification: **current primary production/reference host**.

## Benchmark implications

For the current 20-core guest allocation with four cores reserved for other services, the preferred first real-model CPU comparison matrix is:

```text
8 workers   production candidate
12 workers  production candidate
16 workers  primary Ember production budget
20 workers  full-guest comparison only
```

Prefill and decode must be recorded separately. A larger worker count is not assumed to be faster, especially on memory-bandwidth-sensitive and MoE workloads.

## Evidence policy

- Historical screenshots/operator records may establish an observed configuration or behavior.
- Missing values remain unknown; they are not reconstructed from product names or current provider offerings.
- Provider/virtualization properties that the guest cannot prove are explicit operator evidence, not silently discovered facts.
- Historical ability to run a smaller dense model does not satisfy the current `ember-production` MoE requirement.
- Performance observations become latest-known-good tuning data only after the backend/model/quantization/context/host tuple is recorded reproducibly.
