package upstreamledger

import "testing"

func TestBuiltinLedgerValidates(t *testing.T) {
	ledger, err := Builtin()
	if err != nil {
		t.Fatalf("builtin ledger rejected: %v", err)
	}
	if len(ledger.Entries) < 2 {
		t.Fatalf("expected adoption and planned backends, got %d", len(ledger.Entries))
	}
}

func TestObservedUnpinnedIsNotLatestKnownGood(t *testing.T) {
	ledger := MustBuiltin()
	if got := ledger.LatestKnownGood(); len(got) != 0 {
		t.Fatalf("unversioned observations must not become latest-known-good: %#v", got)
	}
}
