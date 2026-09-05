package backendrouter

import (
	"errors"
	"testing"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/backendcontract"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/modelpolicy"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/modelregistry"
)

func TestRoutePreservesCanonicalIdentity(t *testing.T) {
	registry := modelregistry.MustBuiltin()
	router, err := New(registry, []backendcontract.Descriptor{ollamaDescriptor()}, modelpolicy.MustBuiltin())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := router.Route("ember-coreui:latest", Requirements{Capabilities: []string{"inference.text", "multimodal.vision"}})
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}
	if plan.CanonicalModelID != "ember-coreui" {
		t.Fatalf("canonical identity changed: %q", plan.CanonicalModelID)
	}
	if plan.BackendKind != "ollama-adapter" {
		t.Fatalf("unexpected backend: %q", plan.BackendKind)
	}
	if len(plan.ModelPolicyIDs) == 0 || plan.ModelPolicyIDs[0] != "gemma4-ollama-turin-minimal" {
		t.Fatalf("expected Gemma 4 policy, got %#v", plan.ModelPolicyIDs)
	}
}

func TestRouteRejectsUnsupportedModelCapability(t *testing.T) {
	registry := modelregistry.MustBuiltin()
	router, err := New(registry, []backendcontract.Descriptor{ollamaDescriptor()}, modelpolicy.MustBuiltin())
	if err != nil {
		t.Fatal(err)
	}
	_, err = router.Route("ember-coreui", Requirements{Capabilities: []string{"multimodal.audio"}})
	if !errors.Is(err, ErrNoCompatibleBackend) {
		t.Fatalf("expected no compatible backend, got %v", err)
	}
}

func TestRouteFailsClosedForUnknownCapability(t *testing.T) {
	registry := modelregistry.MustBuiltin()
	router, err := New(registry, []backendcontract.Descriptor{ollamaDescriptor()}, modelpolicy.MustBuiltin())
	if err != nil {
		t.Fatal(err)
	}
	_, err = router.Route("ember-coreui", Requirements{Capabilities: []string{"future.magic"}})
	if !errors.Is(err, ErrNoCompatibleBackend) {
		t.Fatalf("expected fail-closed route, got %v", err)
	}
}

func ollamaDescriptor() backendcontract.Descriptor {
	return backendcontract.Descriptor{
		ContractVersion: backendcontract.ContractVersion,
		ID:              "ollama",
		Kind:            "ollama-adapter",
		AdapterVersion:  "test",
		ExecutionMode:   "external",
		State:           "unknown",
		Capabilities: backendcontract.Capabilities{
			Text:             backendcontract.SupportConditional,
			Architecture:     backendcontract.ArchitectureCapabilities{Dense: backendcontract.SupportConditional, MoE: backendcontract.SupportConditional},
			MoE:              backendcontract.MoECapabilities{ExpertOffload: backendcontract.SupportConditional, ExpertParallel: backendcontract.SupportUnknown},
			Speculative:      backendcontract.SpeculativeCapabilities{MTP: backendcontract.SupportConditional, DraftModel: backendcontract.SupportConditional},
			Cache:            backendcontract.CacheCapabilities{KVOffload: backendcontract.SupportConditional, PromptCache: backendcontract.SupportConditional},
			Multimodal:       backendcontract.MultimodalCapabilities{Vision: backendcontract.SupportConditional, Audio: backendcontract.SupportConditional},
			Embeddings:       backendcontract.SupportConditional,
			Reranking:        backendcontract.SupportUnknown,
			ReasoningControl: backendcontract.SupportConditional,
			Tools:            backendcontract.ToolCapabilities{Calling: backendcontract.SupportConditional, Streaming: backendcontract.SupportUnknown},
			StructuredOutput: backendcontract.SupportConditional,
			Streaming:        backendcontract.StreamingCapabilities{Content: backendcontract.SupportSupported, Reasoning: backendcontract.SupportConditional, ToolArguments: backendcontract.SupportUnknown},
			Placement:        backendcontract.PlacementCapabilities{CPU: backendcontract.SupportSupported, GPU: backendcontract.SupportConditional, Hybrid: backendcontract.SupportConditional},
			Context:          backendcontract.ContextCapabilities{BackendManaged: true, OverrideSupported: backendcontract.SupportConditional},
		},
	}
}
