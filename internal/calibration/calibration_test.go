package calibration

import (
	"context"
	"testing"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostprofile"
)

func TestRunIsBoundedAndReturnsRates(t *testing.T) {
	host := hostprofile.Profile{CPU: hostprofile.CPUProfile{PhysicalCores: 8, LogicalCores: 16}, Memory: hostprofile.MemoryProfile{AvailableBytes: 8 << 30}}
	started := time.Now()
	result := Run(context.Background(), host, 100*time.Millisecond)
	if time.Since(started) > 2*time.Second {
		t.Fatal("calibration exceeded bound")
	}
	if result.SchemaVersion != SchemaVersion {
		t.Fatalf("schema = %q", result.SchemaVersion)
	}
	if result.MemoryCopyBytesPerSec == 0 || result.MemoryReadBytesPerSec == 0 {
		t.Fatalf("rates missing: %+v", result)
	}
	if result.BestWorkers < 1 || len(result.Samples) < 1 {
		t.Fatalf("worker calibration missing: %+v", result)
	}
}

func TestWorkerCandidatesIncludeSMTThreadCount(t *testing.T) {
	got := workerCandidates(96, 192)
	if got[len(got)-1] != 192 {
		t.Fatalf("last candidate = %d, want 192", got[len(got)-1])
	}
}
