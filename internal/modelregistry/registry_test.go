package modelregistry

import (
	"testing"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/modelmanifest"
)

func TestBuiltinRegistryLoadsExamples(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatalf("load builtin registry: %v", err)
	}
	if registry.Len() != 3 {
		t.Fatalf("expected 3 builtin manifests, got %d", registry.Len())
	}

	manifest, ok := registry.Lookup("ember-coreui:latest")
	if !ok {
		t.Fatal("expected Ember CoreUI alias to resolve")
	}
	if manifest.ID != "ember-coreui" {
		t.Fatalf("alias resolved to unexpected id %q", manifest.ID)
	}

	tci, ok := registry.Lookup("quantum-tci-gemma4-e4b")
	if !ok {
		t.Fatal("expected TCI profile to exist")
	}
	if tci.Persona == nil || tci.Persona.Package != "quantum-tci-standard" {
		t.Fatalf("unexpected TCI persona reference: %#v", tci.Persona)
	}
	if tci.State.Verification != "unverified" {
		t.Fatalf("future TCI example must not claim verified model metadata: %q", tci.State.Verification)
	}
}

func TestListIsStableAndSorted(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	entries := registry.List()
	if len(entries) != 3 {
		t.Fatalf("unexpected registry length %d", len(entries))
	}
	for i := 1; i < len(entries); i++ {
		if entries[i-1].ID > entries[i].ID {
			t.Fatalf("registry is not sorted: %q before %q", entries[i-1].ID, entries[i].ID)
		}
	}
}

func TestRegistryRejectsAliasConflicts(t *testing.T) {
	left := testManifest("model-a", "shared:latest")
	right := testManifest("model-b", "shared:latest")
	if _, err := New([]modelmanifest.Manifest{left, right}); err == nil {
		t.Fatal("expected duplicate alias to be rejected")
	}
}

func TestRegistryRejectsCanonicalIDAliasCollision(t *testing.T) {
	left := testManifest("model-a", "model-b")
	right := testManifest("model-b", "model-b:latest")
	if _, err := New([]modelmanifest.Manifest{left, right}); err == nil {
		t.Fatal("expected canonical id and alias collision to be rejected")
	}
}

func testManifest(id, alias string) modelmanifest.Manifest {
	return modelmanifest.Manifest{
		SchemaVersion: modelmanifest.SchemaVersion,
		ID:            id,
		DisplayName:   id,
		Aliases:       []string{alias},
		Source: modelmanifest.Source{
			Provider:  "test",
			Reference: id,
			Revision:  "unresolved",
		},
		Backend:   modelmanifest.Backend{Type: "external"},
		Artifacts: []modelmanifest.Artifact{{URI: "profile://test/" + id}},
		Model: modelmanifest.Model{
			Architecture:   "test",
			ParameterClass: "small",
			Quantization:   "none",
			ContextWindow:  8192,
		},
		Capabilities:  modelmanifest.Capabilities{Text: true},
		Compatibility: modelmanifest.Compatibility{MinRuntimeVersion: "0.2.0-alpha.1"},
		State: modelmanifest.State{
			Install:      "available",
			Verification: "unverified",
			Lifecycle:    "active",
		},
		Provenance: modelmanifest.Provenance{Publisher: "test"},
	}
}
