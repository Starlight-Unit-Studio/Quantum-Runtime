package placement

import (
	"testing"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostprofile"
)

func TestCPUOnlyPreferredWhenWorkingSetFits(t *testing.T) {
	host := hostprofile.Profile{Memory: hostprofile.MemoryProfile{TotalBytes: 64 << 30, AvailableBytes: 60 << 30}, Accelerators: []hostprofile.AcceleratorProfile{{Kind: "gpu", Vendor: "AMD", VRAMBytes: 24 << 30}}}
	plan, err := Build(host, nil, Request{ModelBytes: 24 << 30, KVCacheBytes: 2 << 30, AllowAcceleration: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Decision != "cpu_only" {
		t.Fatalf("decision = %q", plan.Decision)
	}
	if plan.VRAM.AssignedBytes != 0 {
		t.Fatalf("unexpected VRAM assignment = %d", plan.VRAM.AssignedBytes)
	}
}

func TestHybridCandidateOnlyWhenCPUDoesNotFit(t *testing.T) {
	host := hostprofile.Profile{Memory: hostprofile.MemoryProfile{TotalBytes: 32 << 30, AvailableBytes: 28 << 30}, Accelerators: []hostprofile.AcceleratorProfile{{Kind: "gpu", Vendor: "NVIDIA", VRAMBytes: 24 << 30}}}
	plan, err := Build(host, nil, Request{ModelBytes: 30 << 30, KVCacheBytes: 2 << 30, AllowAcceleration: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Decision != "hybrid_candidate" {
		t.Fatalf("decision = %q, reason=%s", plan.Decision, plan.Reason)
	}
	if plan.VRAM.AssignedBytes == 0 || plan.RAM.AssignedBytes == 0 {
		t.Fatalf("expected split placement: %+v", plan)
	}
}

func TestNVMeOnlyAcceptsExplicitColdTier(t *testing.T) {
	host := hostprofile.Profile{Memory: hostprofile.MemoryProfile{TotalBytes: 64 << 30, AvailableBytes: 60 << 30}}
	plan, err := Build(host, nil, Request{ModelBytes: 8 << 30, ColdBytes: 16 << 30, AllowNVMeColdTier: true, NVMeBudgetBytes: 32 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if plan.NVMe.AssignedBytes != 16<<30 {
		t.Fatalf("nvme = %d", plan.NVMe.AssignedBytes)
	}
}

func TestHotStateIsNeverSpilledToDisk(t *testing.T) {
	host := hostprofile.Profile{Memory: hostprofile.MemoryProfile{TotalBytes: 16 << 30, AvailableBytes: 12 << 30}}
	plan, err := Build(host, nil, Request{ModelBytes: 20 << 30, AllowNVMeColdTier: true, NVMeBudgetBytes: 100 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Decision != "capacity_exceeded" {
		t.Fatalf("decision = %q", plan.Decision)
	}
	if plan.NVMe.AssignedBytes != 0 {
		t.Fatalf("hot state spilled to NVMe: %+v", plan)
	}
}
