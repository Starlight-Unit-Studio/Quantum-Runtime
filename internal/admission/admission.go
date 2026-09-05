package admission

import (
	"fmt"
	"strings"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/deploymentprofile"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostlimits"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostprofile"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/modelmanifest"
)

const SchemaVersion = "quantum.runtime/admission-result/v1alpha1"

const (
	DecisionAdmitted              = "admitted"
	DecisionRejected              = "rejected"
	DecisionNeedsOperatorEvidence = "needs_operator_evidence"
)

type OperatorEvidence struct {
	ECCVerified            *bool  `json:"ecc_verified,omitempty"`
	MemoryClass            string `json:"memory_class,omitempty"`
	DedicatedPhysicalCores int    `json:"dedicated_physical_cores,omitempty"`
	CoreBudget             int    `json:"core_budget,omitempty"`
}

type Check struct {
	ID        string `json:"id"`
	Mandatory bool   `json:"mandatory"`
	State     string `json:"state"`
	Required  any    `json:"required,omitempty"`
	Observed  any    `json:"observed,omitempty"`
	Source    string `json:"source,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type Result struct {
	SchemaVersion    string  `json:"schema_version"`
	Decision         string  `json:"decision"`
	ProfileID        string  `json:"profile_id"`
	CanonicalModelID string  `json:"canonical_model_id"`
	Checks           []Check `json:"checks"`
}

func Evaluate(profile deploymentprofile.Profile, host hostprofile.Profile, limits hostlimits.Limits, model modelmanifest.Manifest, evidence OperatorEvidence) (Result, error) {
	if err := profile.Validate(); err != nil {
		return Result{}, fmt.Errorf("deployment profile: %w", err)
	}
	if err := host.Validate(); err != nil {
		return Result{}, fmt.Errorf("host profile: %w", err)
	}
	if err := limits.Validate(); err != nil {
		return Result{}, fmt.Errorf("host limits: %w", err)
	}
	if err := model.Validate(); err != nil {
		return Result{}, fmt.Errorf("model manifest: %w", err)
	}
	if evidence.DedicatedPhysicalCores < 0 || evidence.CoreBudget < 0 {
		return Result{}, fmt.Errorf("operator core evidence must not be negative")
	}

	result := Result{
		SchemaVersion:    SchemaVersion,
		ProfileID:        profile.ID,
		CanonicalModelID: model.ID,
	}

	requiredArchitecture := strings.ToLower(strings.TrimSpace(profile.Model.ArchitectureClass))
	observedArchitecture := strings.ToLower(strings.TrimSpace(model.Model.ArchitectureClass))
	architecturePass := requiredArchitecture == "any" || observedArchitecture == requiredArchitecture
	result.Checks = append(result.Checks, booleanCheck(
		"model.architecture_class",
		true,
		architecturePass,
		requiredArchitecture,
		observedArchitecture,
		"model_manifest",
		"The application profile is admitted by architecture capability, not by model-name matching.",
	))

	result.Checks = append(result.Checks, booleanCheck(
		"hardware.memory.minimum_bytes",
		true,
		host.Memory.TotalBytes >= profile.Hardware.Memory.MinimumBytes,
		profile.Hardware.Memory.MinimumBytes,
		host.Memory.TotalBytes,
		"host_profile",
		"Installed/visible memory capacity is evaluated independently from current free-memory pressure.",
	))

	physicalCores := host.CPU.PhysicalCores
	physicalSource := "host_topology"
	if evidence.DedicatedPhysicalCores > 0 {
		physicalCores = evidence.DedicatedPhysicalCores
		physicalSource = "operator_evidence"
	}
	result.Checks = append(result.Checks, booleanCheck(
		"hardware.cpu.minimum_physical_cores",
		true,
		physicalCores >= profile.Hardware.CPU.MinimumPhysicalCores,
		profile.Hardware.CPU.MinimumPhysicalCores,
		physicalCores,
		physicalSource,
		"On virtualized hosts, provider dedication cannot be inferred from the CPU model name; operator evidence may replace guest-topology evidence.",
	))

	effectiveBudget := limits.EffectiveLogicalCPUs
	budgetSource := "process_affinity/cgroup"
	if evidence.CoreBudget > 0 {
		effectiveBudget = evidence.CoreBudget
		budgetSource = "operator_core_budget"
	}
	budgetPass := effectiveBudget >= profile.Hardware.CPU.MinimumPhysicalCores && effectiveBudget <= limits.EffectiveLogicalCPUs
	result.Checks = append(result.Checks, booleanCheck(
		"hardware.cpu.runtime_budget",
		true,
		budgetPass,
		map[string]any{"minimum": profile.Hardware.CPU.MinimumPhysicalCores, "maximum_visible": limits.EffectiveLogicalCPUs},
		effectiveBudget,
		budgetSource,
		"The Runtime budget prevents a profile from assuming all host-model cores are available to the guest or application.",
	))

	if profile.Hardware.Memory.ECCRequired {
		check := Check{
			ID:        "hardware.memory.ecc",
			Mandatory: true,
			Required:  true,
			Source:    "operator_evidence",
			Detail:    "ECC is not guessed from ordinary guest userspace when trustworthy evidence is unavailable.",
		}
		if evidence.ECCVerified == nil {
			check.State = "unknown"
			check.Observed = "unknown"
		} else {
			check.Observed = *evidence.ECCVerified
			if *evidence.ECCVerified {
				check.State = "pass"
			} else {
				check.State = "fail"
			}
		}
		result.Checks = append(result.Checks, check)
	}

	if preferred := strings.ToLower(strings.TrimSpace(profile.Hardware.Memory.PreferredClass)); preferred != "" {
		observed := strings.ToLower(strings.TrimSpace(evidence.MemoryClass))
		check := Check{
			ID:        "hardware.memory.preferred_class",
			Mandatory: false,
			Required:  preferred,
			Source:    "operator_evidence",
			Detail:    "Memory generation is a preference for this profile, not a generic Runtime hard gate.",
		}
		switch {
		case observed == "":
			check.State = "unknown"
			check.Observed = "unknown"
		case observed == preferred:
			check.State = "pass"
			check.Observed = observed
		default:
			check.State = "preference_miss"
			check.Observed = observed
		}
		result.Checks = append(result.Checks, check)
	}

	result.Checks = append(result.Checks, Check{
		ID:        "hardware.accelerator.required",
		Mandatory: profile.Hardware.Accelerator.Required,
		State:     acceleratorState(profile, host),
		Required:  profile.Hardware.Accelerator.Required,
		Observed:  len(host.Accelerators),
		Source:    "host_profile",
		Detail:    "CPU-only remains a first-class profile when an accelerator is not required.",
	})

	if profile.Hardware.CPU.ReferenceMinClockMHz > 0 {
		result.Checks = append(result.Checks, Check{
			ID:        "hardware.cpu.reference_clock_mhz",
			Mandatory: false,
			State:     "advisory",
			Required:  profile.Hardware.CPU.ReferenceMinClockMHz,
			Observed:  "not_used_for_admission",
			Source:    "profile_reference",
			Detail:    "Raw clock frequency is historical/advisory only; calibrated throughput and topology take precedence.",
		})
	}

	result.Decision = decide(result.Checks)
	return result, nil
}

func booleanCheck(id string, mandatory, pass bool, required, observed any, source, detail string) Check {
	state := "fail"
	if pass {
		state = "pass"
	}
	return Check{
		ID:        id,
		Mandatory: mandatory,
		State:     state,
		Required:  required,
		Observed:  observed,
		Source:    source,
		Detail:    detail,
	}
}

func acceleratorState(profile deploymentprofile.Profile, host hostprofile.Profile) string {
	if !profile.Hardware.Accelerator.Required {
		return "pass"
	}
	if len(host.Accelerators) > 0 {
		return "pass"
	}
	return "fail"
}

func decide(checks []Check) string {
	unknownMandatory := false
	for _, check := range checks {
		if !check.Mandatory {
			continue
		}
		if check.State == "fail" {
			return DecisionRejected
		}
		if check.State == "unknown" {
			unknownMandatory = true
		}
	}
	if unknownMandatory {
		return DecisionNeedsOperatorEvidence
	}
	return DecisionAdmitted
}
