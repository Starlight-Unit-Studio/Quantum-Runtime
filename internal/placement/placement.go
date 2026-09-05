package placement

import (
	"errors"
	"fmt"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/calibration"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostprofile"
)

const SchemaVersion = "quantum.runtime/placement-plan/v1alpha1"

type Request struct {
	ModelBytes        uint64 `json:"model_bytes"`
	MoEExpertBytes    uint64 `json:"moe_expert_bytes,omitempty"`
	KVCacheBytes      uint64 `json:"kv_cache_bytes,omitempty"`
	PrefixCacheBytes  uint64 `json:"prefix_cache_bytes,omitempty"`
	ProjectorBytes    uint64 `json:"projector_bytes,omitempty"`
	WorkspaceBytes    uint64 `json:"workspace_bytes,omitempty"`
	ColdBytes         uint64 `json:"cold_bytes,omitempty"`
	AllowAcceleration bool   `json:"allow_acceleration,omitempty"`
	AllowNVMeColdTier bool   `json:"allow_nvme_cold_tier,omitempty"`
	NVMeBudgetBytes   uint64 `json:"nvme_budget_bytes,omitempty"`
	ReserveRAMBytes   uint64 `json:"reserve_ram_bytes,omitempty"`
}

type Plan struct {
	SchemaVersion string           `json:"schema_version"`
	Decision      string           `json:"decision"`
	Reason        string           `json:"reason"`
	CPUFirst      bool             `json:"cpu_first"`
	RAM           TierAllocation   `json:"ram"`
	VRAM          TierAllocation   `json:"vram"`
	NVMe          TierAllocation   `json:"nvme"`
	Classes       []ClassPlacement `json:"classes"`
	Requirements  []string         `json:"requires_backend_capabilities,omitempty"`
	Warnings      []string         `json:"warnings,omitempty"`
}

type TierAllocation struct {
	AvailableBytes uint64 `json:"available_bytes"`
	AssignedBytes  uint64 `json:"assigned_bytes"`
}

type ClassPlacement struct {
	Class string `json:"class"`
	Tier  string `json:"tier"`
	Bytes uint64 `json:"bytes"`
}

func Build(host hostprofile.Profile, calibrationResult *calibration.Result, req Request) (Plan, error) {
	if err := validateRequest(req); err != nil {
		return Plan{}, err
	}
	plan := Plan{SchemaVersion: SchemaVersion, CPUFirst: true}
	availableRAM := host.Memory.AvailableBytes
	if availableRAM == 0 {
		availableRAM = host.Memory.TotalBytes
	}
	reserve := req.ReserveRAMBytes
	if reserve == 0 {
		reserve = defaultReserve(host.Memory.TotalBytes)
	}
	usableRAM := uint64(0)
	if availableRAM > reserve {
		usableRAM = availableRAM - reserve
	}
	plan.RAM.AvailableBytes = usableRAM

	var bestVRAM uint64
	for _, acc := range host.Accelerators {
		if acc.VRAMBytes > bestVRAM {
			bestVRAM = acc.VRAMBytes
		}
	}
	plan.VRAM.AvailableBytes = bestVRAM
	if req.AllowNVMeColdTier {
		plan.NVMe.AvailableBytes = req.NVMeBudgetBytes
	}

	hot := req.ModelBytes + req.MoEExpertBytes + req.KVCacheBytes + req.PrefixCacheBytes + req.ProjectorBytes + req.WorkspaceBytes
	if hot <= usableRAM {
		plan.Decision = "cpu_only"
		plan.Reason = "CPU-first hot working set fits in usable host RAM."
		assignHotToRAM(&plan, req)
		assignCold(&plan, req)
		if calibrationResult != nil && calibrationResult.MemoryBandwidthClass == "low" {
			plan.Warnings = append(plan.Warnings, "Host calibration reports low memory bandwidth; CPU-only capacity fit does not guarantee requested throughput.")
		}
		return plan, nil
	}

	if req.AllowAcceleration && bestVRAM > 0 {
		plan.Decision = "hybrid_candidate"
		plan.Reason = "CPU-first working set exceeds usable RAM; an accelerator tier is available for a pre-activation hybrid candidate."
		remainingVRAM := bestVRAM
		remainingRAM := usableRAM
		unplaced := false

		allocateWhole(&plan, "kv_cache", req.KVCacheBytes, &remainingVRAM, &remainingRAM, &unplaced)
		allocateWhole(&plan, "projector", req.ProjectorBytes, &remainingVRAM, &remainingRAM, &unplaced)
		allocateSplit(&plan, "model_weights", req.ModelBytes, &remainingVRAM, &remainingRAM, &unplaced)
		allocateSplit(&plan, "moe_experts", req.MoEExpertBytes, &remainingVRAM, &remainingRAM, &unplaced)
		allocateWhole(&plan, "prefix_cache", req.PrefixCacheBytes, &remainingVRAM, &remainingRAM, &unplaced)
		allocateWhole(&plan, "workspace", req.WorkspaceBytes, &remainingVRAM, &remainingRAM, &unplaced)

		if unplaced {
			plan.Decision = "capacity_exceeded"
			plan.Reason = "The requested hot working set does not fit in known RAM/VRAM tiers without unsafe active-state disk spilling."
		}
		plan.Requirements = append(plan.Requirements, "hybrid_pre_activation_placement")
		if req.MoEExpertBytes > 0 {
			plan.Requirements = append(plan.Requirements, "moe_expert_offload")
		}
		if req.KVCacheBytes > 0 && plan.VRAM.AssignedBytes > 0 {
			plan.Requirements = append(plan.Requirements, "kv_cache_placement")
		}
		assignCold(&plan, req)
		return plan, nil
	}

	plan.Decision = "capacity_exceeded"
	plan.Reason = "CPU-first working set exceeds usable RAM and no compatible accelerator capacity was requested/observed."
	assignUnplaced(&plan, req)
	assignCold(&plan, req)
	return plan, nil
}

func validateRequest(req Request) error {
	if req.ModelBytes == 0 {
		return errors.New("model_bytes must be greater than zero")
	}
	if req.AllowNVMeColdTier && req.ColdBytes > 0 && req.NVMeBudgetBytes == 0 {
		return errors.New("nvme_budget_bytes is required when the NVMe cold tier is enabled")
	}
	if req.ColdBytes > req.NVMeBudgetBytes && req.AllowNVMeColdTier {
		return fmt.Errorf("cold_bytes (%d) exceed nvme_budget_bytes (%d)", req.ColdBytes, req.NVMeBudgetBytes)
	}
	return nil
}

func defaultReserve(total uint64) uint64 {
	const twoGiB = uint64(2 << 30)
	if total == 0 {
		return twoGiB
	}
	reserve := total / 5
	if reserve < twoGiB {
		reserve = twoGiB
	}
	return reserve
}

func allocateWhole(plan *Plan, class string, bytes uint64, vram, ram *uint64, unplaced *bool) {
	if bytes == 0 {
		return
	}
	if bytes <= *vram {
		*vram -= bytes
		plan.VRAM.AssignedBytes += bytes
		plan.Classes = append(plan.Classes, ClassPlacement{Class: class, Tier: "vram", Bytes: bytes})
		return
	}
	if bytes <= *ram {
		*ram -= bytes
		plan.RAM.AssignedBytes += bytes
		plan.Classes = append(plan.Classes, ClassPlacement{Class: class, Tier: "ram", Bytes: bytes})
		return
	}
	*unplaced = true
	plan.Classes = append(plan.Classes, ClassPlacement{Class: class, Tier: "unplaced", Bytes: bytes})
}

func allocateSplit(plan *Plan, class string, bytes uint64, vram, ram *uint64, unplaced *bool) {
	if bytes == 0 {
		return
	}
	toVRAM := min(bytes, *vram)
	if toVRAM > 0 {
		*vram -= toVRAM
		bytes -= toVRAM
		plan.VRAM.AssignedBytes += toVRAM
		plan.Classes = append(plan.Classes, ClassPlacement{Class: class, Tier: "vram", Bytes: toVRAM})
	}
	toRAM := min(bytes, *ram)
	if toRAM > 0 {
		*ram -= toRAM
		bytes -= toRAM
		plan.RAM.AssignedBytes += toRAM
		plan.Classes = append(plan.Classes, ClassPlacement{Class: class, Tier: "ram", Bytes: toRAM})
	}
	if bytes > 0 {
		*unplaced = true
		plan.Classes = append(plan.Classes, ClassPlacement{Class: class, Tier: "unplaced", Bytes: bytes})
	}
}

func assignHotToRAM(plan *Plan, req Request) {
	for _, c := range []ClassPlacement{
		{Class: "model_weights", Tier: "ram", Bytes: req.ModelBytes},
		{Class: "moe_experts", Tier: "ram", Bytes: req.MoEExpertBytes},
		{Class: "kv_cache", Tier: "ram", Bytes: req.KVCacheBytes},
		{Class: "prefix_cache", Tier: "ram", Bytes: req.PrefixCacheBytes},
		{Class: "projector", Tier: "ram", Bytes: req.ProjectorBytes},
		{Class: "workspace", Tier: "ram", Bytes: req.WorkspaceBytes},
	} {
		if c.Bytes == 0 {
			continue
		}
		plan.Classes = append(plan.Classes, c)
		plan.RAM.AssignedBytes += c.Bytes
	}
}

func assignUnplaced(plan *Plan, req Request) {
	for _, c := range []ClassPlacement{
		{Class: "model_weights", Tier: "unplaced", Bytes: req.ModelBytes},
		{Class: "moe_experts", Tier: "unplaced", Bytes: req.MoEExpertBytes},
		{Class: "kv_cache", Tier: "unplaced", Bytes: req.KVCacheBytes},
		{Class: "prefix_cache", Tier: "unplaced", Bytes: req.PrefixCacheBytes},
		{Class: "projector", Tier: "unplaced", Bytes: req.ProjectorBytes},
		{Class: "workspace", Tier: "unplaced", Bytes: req.WorkspaceBytes},
	} {
		if c.Bytes > 0 {
			plan.Classes = append(plan.Classes, c)
		}
	}
}

func assignCold(plan *Plan, req Request) {
	if req.ColdBytes == 0 {
		return
	}
	if req.AllowNVMeColdTier && req.ColdBytes <= req.NVMeBudgetBytes {
		plan.Classes = append(plan.Classes, ClassPlacement{Class: "cold_weights_or_cache", Tier: "nvme", Bytes: req.ColdBytes})
		plan.NVMe.AssignedBytes += req.ColdBytes
		plan.Requirements = append(plan.Requirements, "explicit_nvme_cold_tier")
		return
	}
	plan.Classes = append(plan.Classes, ClassPlacement{Class: "cold_weights_or_cache", Tier: "unplaced", Bytes: req.ColdBytes})
	plan.Warnings = append(plan.Warnings, "Cold bytes were requested but the explicit NVMe tier is disabled or insufficient.")
}
