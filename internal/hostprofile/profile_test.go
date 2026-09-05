package hostprofile

import "testing"

func TestDiscoverReturnsValidProfile(t *testing.T) {
	p := Discover()
	if err := p.Validate(); err != nil {
		t.Fatalf("profile invalid: %v", err)
	}
	if p.SchemaVersion != SchemaVersion {
		t.Fatalf("schema = %q", p.SchemaVersion)
	}
	if p.CPU.LogicalCores < 1 {
		t.Fatalf("logical cores = %d", p.CPU.LogicalCores)
	}
}

func TestHasFeatureUsesSortedFeatures(t *testing.T) {
	p := Profile{CPU: CPUProfile{Features: []string{"avx2", "avx512f"}}}
	if !HasFeature(p, "AVX2") {
		t.Fatal("expected avx2")
	}
	if HasFeature(p, "sve") {
		t.Fatal("unexpected sve")
	}
}
