package admission

import (
	"testing"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/deploymentprofile"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostlimits"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostprofile"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/modelmanifest"
)

func TestEmberProductionNeedsECCEvidence(t *testing.T) {
	profile := emberProfile(t)
	host := suitableHost()
	limits := suitableLimits()
	model := moeModel()
	result, err := Evaluate(profile, host, limits, model, OperatorEvidence{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionNeedsOperatorEvidence {
		t.Fatalf("decision = %q", result.Decision)
	}
}

func TestEmberProductionAdmitsVerifiedCurrentClass(t *testing.T) {
	profile := emberProfile(t)
	host := suitableHost()
	limits := suitableLimits()
	model := moeModel()
	ecc := true
	result, err := Evaluate(profile, host, limits, model, OperatorEvidence{
		ECCVerified:            &ecc,
		MemoryClass:            "ddr5",
		DedicatedPhysicalCores: 20,
		CoreBudget:             16,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionAdmitted {
		t.Fatalf("decision = %q checks=%+v", result.Decision, result.Checks)
	}
}

func TestDenseModelCannotSatisfyMoEProfile(t *testing.T) {
	profile := emberProfile(t)
	host := suitableHost()
	limits := suitableLimits()
	model := moeModel()
	model.Model.ArchitectureClass = "dense"
	ecc := true
	result, err := Evaluate(profile, host, limits, model, OperatorEvidence{ECCVerified: &ecc})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionRejected {
		t.Fatalf("decision = %q", result.Decision)
	}
}

func TestOperatorCoreBudgetCanProtectSharedServices(t *testing.T) {
	profile := emberProfile(t)
	host := suitableHost()
	limits := suitableLimits()
	model := moeModel()
	ecc := true
	result, err := Evaluate(profile, host, limits, model, OperatorEvidence{ECCVerified: &ecc, CoreBudget: 4})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionRejected {
		t.Fatalf("decision = %q", result.Decision)
	}
}

func emberProfile(t *testing.T) deploymentprofile.Profile {
	t.Helper()
	registry, err := deploymentprofile.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := registry.Lookup("ember-production")
	if !ok {
		t.Fatal("ember-production missing")
	}
	return profile
}

func suitableHost() hostprofile.Profile {
	return hostprofile.Profile{
		SchemaVersion: hostprofile.SchemaVersion,
		CPU:           hostprofile.CPUProfile{PhysicalCores: 20, LogicalCores: 20, ThreadsPerCore: 1},
		Memory:        hostprofile.MemoryProfile{TotalBytes: 96 << 30, AvailableBytes: 80 << 30},
	}
}

func suitableLimits() hostlimits.Limits {
	return hostlimits.Limits{SchemaVersion: hostlimits.SchemaVersion, EffectiveLogicalCPUs: 20}
}

func moeModel() modelmanifest.Manifest {
	return modelmanifest.Manifest{
		SchemaVersion: modelmanifest.SchemaVersion,
		ID:            "test-moe",
		DisplayName:   "Test MoE",
		Source:        modelmanifest.Source{Provider: "profile", Reference: "test/moe", Revision: "unresolved"},
		Backend:       modelmanifest.Backend{Type: "external"},
		Artifacts:     []modelmanifest.Artifact{{URI: "profile://test/moe"}},
		Model: modelmanifest.Model{
			Architecture:      "test",
			ArchitectureClass: "moe",
			ParameterClass:    "test",
			Quantization:      "q4",
			ContextWindow:     8192,
		},
		Capabilities:  modelmanifest.Capabilities{Text: true},
		Compatibility: modelmanifest.Compatibility{MinRuntimeVersion: "0.3.0-alpha.1"},
		State:         modelmanifest.State{Install: "available", Verification: "unverified", Lifecycle: "active"},
		Provenance:    modelmanifest.Provenance{Publisher: "test"},
	}
}
