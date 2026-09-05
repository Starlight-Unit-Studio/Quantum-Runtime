package hostlimits

import "testing"

func TestCountCPUList(t *testing.T) {
	count, err := countCPUList("0-3,8,10-11")
	if err != nil {
		t.Fatal(err)
	}
	if count != 7 {
		t.Fatalf("count = %d, want 7", count)
	}
}

func TestCountCPUListRejectsBrokenRange(t *testing.T) {
	if _, err := countCPUList("4-2"); err == nil {
		t.Fatal("expected invalid range error")
	}
}

func TestVirtualizationKind(t *testing.T) {
	if got := virtualizationKind("QEMU Standard PC"); got != "kvm" {
		t.Fatalf("kind = %q, want kvm", got)
	}
	if got := virtualizationKind("physical server"); got != "" {
		t.Fatalf("kind = %q, want empty", got)
	}
}
