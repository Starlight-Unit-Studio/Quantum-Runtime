package benchmarkplan

import (
	"reflect"
	"testing"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostlimits"
)

func TestCurrentTwentyCoreHostPlan(t *testing.T) {
	limits := hostlimits.Limits{SchemaVersion: hostlimits.SchemaVersion, EffectiveLogicalCPUs: 20}
	plan, err := Build(limits, Request{ReserveSystemCores: 4, MinimumWorkers: 8, IncludeFullHostComparison: true})
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for _, candidate := range plan.Candidates {
		got = append(got, candidate.Workers)
	}
	want := []int{8, 12, 16, 20}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
	if plan.ProductionMaxWorkers != 16 {
		t.Fatalf("production max = %d", plan.ProductionMaxWorkers)
	}
	if plan.Candidates[len(plan.Candidates)-1].Mode != "full_host_comparison" {
		t.Fatalf("20-core candidate must be comparison: %+v", plan.Candidates)
	}
}

func TestBudgetCannotExceedVisibleCPUs(t *testing.T) {
	limits := hostlimits.Limits{SchemaVersion: hostlimits.SchemaVersion, EffectiveLogicalCPUs: 12}
	if _, err := Build(limits, Request{CoreBudget: 16}); err == nil {
		t.Fatal("expected core budget error")
	}
}
