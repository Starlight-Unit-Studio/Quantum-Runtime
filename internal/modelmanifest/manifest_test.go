package modelmanifest

import (
	"strings"
	"testing"
)

func TestValidManifest(t *testing.T) {
	manifest := validManifest()
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
}

func TestManifestValidationFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*Manifest)
		contains string
	}{
		{
			name:     "invalid canonical id",
			mutate:   func(m *Manifest) { m.ID = "Bad Model" },
			contains: "canonical model identifier",
		},
		{
			name:     "invalid artifact hash",
			mutate:   func(m *Manifest) { m.Artifacts[0].SHA256 = "sha256:not-a-digest" },
			contains: "invalid sha256",
		},
		{
			name:     "no capabilities",
			mutate:   func(m *Manifest) { m.Capabilities = Capabilities{} },
			contains: "at least one capability",
		},
		{
			name: "maximum runtime below minimum",
			mutate: func(m *Manifest) {
				m.Compatibility.MinRuntimeVersion = "0.2.0-alpha.1"
				m.Compatibility.MaxRuntimeVersion = "0.1.9"
			},
			contains: "must not be lower",
		},
		{
			name:     "failed verification cannot remain active",
			mutate:   func(m *Manifest) { m.State.Verification = "failed" },
			contains: "cannot be active",
		},
		{
			name:     "invalid persona digest",
			mutate:   func(m *Manifest) { m.Persona = &PersonaRef{Package: "persona", Version: "1.0.0", SHA256: "bad"} },
			contains: "persona.sha256",
		},
		{
			name:     "invalid semantic version",
			mutate:   func(m *Manifest) { m.Compatibility.MinRuntimeVersion = "02.0.0" },
			contains: "leading-zero version component",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validManifest()
			test.mutate(&manifest)
			err := manifest.Validate()
			if err == nil {
				t.Fatal("expected validation failure")
			}
			if !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestVerifiedManifestRequiresIntegrityAndImmutableRevision(t *testing.T) {
	manifest := validManifest()
	manifest.State.Verification = "verified"
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "immutable source revision") {
		t.Fatalf("expected unresolved revision failure, got %v", err)
	}

	manifest.Source.Revision = "0123456789abcdef"
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "require a sha256") {
		t.Fatalf("expected missing digest failure, got %v", err)
	}

	manifest.Artifacts[0].SHA256 = "sha256:" + strings.Repeat("a", 64)
	if err := manifest.Validate(); err != nil {
		t.Fatalf("verified manifest rejected after integrity data supplied: %v", err)
	}
}

func TestMultiBackendArtifactAndMoEMetadata(t *testing.T) {
	manifest := validManifest()
	manifest.Backend = Backend{Type: "external"}
	manifest.Artifacts = []Artifact{
		{URI: "ollama:test-model:latest", Backend: "ollama-adapter", Format: "ollama", Role: "inference"},
		{URI: "file:///models/test-model.gguf", Backend: "llama.cpp", Format: "gguf", Role: "inference"},
	}
	manifest.Model.ArchitectureClass = "moe"
	manifest.Model.TotalParametersB = 26
	manifest.Model.ActiveParametersB = 4
	manifest.Model.Experts = &ExpertTopology{Total: 8, Active: 2, Shared: 1}
	manifest.Model.ContextPolicy = ModelContextPolicy{BackendManaged: true, OverrideSupported: true}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("multi-backend MoE manifest rejected: %v", err)
	}
}

func TestDenseModelRejectsExpertTopology(t *testing.T) {
	manifest := validManifest()
	manifest.Model.ArchitectureClass = "dense"
	manifest.Model.Experts = &ExpertTopology{Total: 8, Active: 2}
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "architecture_class=moe") {
		t.Fatalf("expected dense expert topology rejection, got %v", err)
	}
}

func TestSemanticPrereleaseOrdering(t *testing.T) {
	manifest := validManifest()
	manifest.Compatibility.MinRuntimeVersion = "0.2.0-alpha.2"
	manifest.Compatibility.MaxRuntimeVersion = "0.2.0-alpha.1"
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected lower prerelease maximum to be rejected")
	}

	manifest.Compatibility.MaxRuntimeVersion = "0.2.0"
	if err := manifest.Validate(); err != nil {
		t.Fatalf("stable release should be newer than prerelease: %v", err)
	}
}

func validManifest() Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion,
		ID:            "test-model",
		DisplayName:   "Test Model",
		Aliases:       []string{"test-model:latest"},
		Source: Source{
			Provider:  "test",
			Reference: "test/model",
			Revision:  "unresolved",
		},
		Backend:   Backend{Type: "external"},
		Artifacts: []Artifact{{URI: "profile://test/model"}},
		Model: Model{
			Architecture:   "test",
			ParameterClass: "small",
			Quantization:   "none",
			ContextWindow:  8192,
		},
		Capabilities:  Capabilities{Text: true},
		Compatibility: Compatibility{MinRuntimeVersion: "0.2.0-alpha.1"},
		State: State{
			Install:      "available",
			Verification: "unverified",
			Lifecycle:    "active",
		},
		Provenance: Provenance{Publisher: "Starlight Unit Studios"},
	}
}
