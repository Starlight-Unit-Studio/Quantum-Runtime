package benchmarkplan

import (
	"fmt"
	"sort"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostlimits"
)

const SchemaVersion = "quantum.runtime/model-benchmark-plan/v1alpha1"

type Request struct {
	CoreBudget                int  `json:"core_budget,omitempty"`
	ReserveSystemCores        int  `json:"reserve_system_cores,omitempty"`
	MinimumWorkers            int  `json:"minimum_workers,omitempty"`
	IncludeFullHostComparison bool `json:"include_full_host_comparison,omitempty"`
}

type Candidate struct {
	Workers int    `json:"workers"`
	Mode    string `json:"mode"`
}

type Plan struct {
	SchemaVersion        string      `json:"schema_version"`
	EffectiveHostCPUs    int         `json:"effective_host_cpus"`
	ProductionMaxWorkers int         `json:"production_max_workers"`
	ReservedSystemCores  int         `json:"reserved_system_cores"`
	Candidates           []Candidate `json:"candidates"`
	Notes                []string    `json:"notes,omitempty"`
}

func Build(limits hostlimits.Limits, req Request) (Plan, error) {
	if err := limits.Validate(); err != nil {
		return Plan{}, err
	}
	if req.CoreBudget < 0 || req.ReserveSystemCores < 0 || req.MinimumWorkers < 0 {
		return Plan{}, fmt.Errorf("benchmark core values must not be negative")
	}
	effective := limits.EffectiveLogicalCPUs
	productionMax := effective - req.ReserveSystemCores
	if productionMax < 1 {
		return Plan{}, fmt.Errorf("system core reserve leaves no CPU capacity for benchmarking")
	}
	if req.CoreBudget > 0 {
		if req.CoreBudget > effective {
			return Plan{}, fmt.Errorf("core_budget exceeds effective host CPUs")
		}
		if req.CoreBudget < productionMax {
			productionMax = req.CoreBudget
		}
	}
	minimum := req.MinimumWorkers
	if minimum == 0 {
		minimum = 4
	}
	if minimum > productionMax {
		minimum = productionMax
	}

	canonical := []int{1, 2, 4, 8, 12, 16, 20, 24, 32, 48, 64, 96, 128, 192}
	set := map[int]string{}
	for _, workers := range canonical {
		if workers >= minimum && workers <= productionMax {
			set[workers] = "production"
		}
	}
	set[productionMax] = "production"
	if req.IncludeFullHostComparison && effective > productionMax {
		set[effective] = "full_host_comparison"
	}

	workers := make([]int, 0, len(set))
	for value := range set {
		workers = append(workers, value)
	}
	sort.Ints(workers)
	candidates := make([]Candidate, 0, len(workers))
	for _, value := range workers {
		candidates = append(candidates, Candidate{Workers: value, Mode: set[value]})
	}

	plan := Plan{
		SchemaVersion:        SchemaVersion,
		EffectiveHostCPUs:    effective,
		ProductionMaxWorkers: productionMax,
		ReservedSystemCores:  req.ReserveSystemCores,
		Candidates:           candidates,
		Notes: []string{
			"This plan defines thread-count candidates only; it does not claim model throughput before measurement.",
			"Prefill and decode must be recorded separately for each backend/model/quantization/context tuple.",
		},
	}
	return plan, nil
}
