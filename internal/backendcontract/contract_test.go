package backendcontract

import "testing"

func TestDescriptorValidation(t *testing.T) {
	descriptor := testDescriptor()
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("valid descriptor rejected: %v", err)
	}
}

func TestDescriptorFailsClosedOnInvalidSupport(t *testing.T) {
	descriptor := testDescriptor()
	descriptor.Capabilities.Tools.Streaming = Support("maybe")
	if err := descriptor.Validate(); err == nil {
		t.Fatal("expected invalid support state to fail")
	}
}

func TestVerifiedContextOverrideRequiresSupportedOverride(t *testing.T) {
	descriptor := testDescriptor()
	descriptor.Capabilities.Context.OverrideVerified = true
	descriptor.Capabilities.Context.OverrideSupported = SupportConditional
	if err := descriptor.Validate(); err == nil {
		t.Fatal("expected conditional override to reject verified=true")
	}
}

func TestCapabilityStateUnknownFailsExplicitly(t *testing.T) {
	if got := testDescriptor().Capabilities.State("future.capability"); got != SupportUnknown {
		t.Fatalf("unexpected state: %q", got)
	}
}

func testDescriptor() Descriptor {
	return Descriptor{
		ContractVersion: ContractVersion,
		ID:              "ollama",
		Kind:            "ollama-adapter",
		AdapterVersion:  "test",
		ExecutionMode:   "external",
		State:           "unknown",
		Capabilities: Capabilities{
			Text:             SupportConditional,
			Architecture:     ArchitectureCapabilities{Dense: SupportConditional, MoE: SupportConditional},
			MoE:              MoECapabilities{ExpertOffload: SupportConditional, ExpertParallel: SupportUnknown},
			Speculative:      SpeculativeCapabilities{MTP: SupportConditional, DraftModel: SupportConditional},
			Cache:            CacheCapabilities{KVOffload: SupportConditional, PromptCache: SupportConditional},
			Multimodal:       MultimodalCapabilities{Vision: SupportConditional, Audio: SupportConditional},
			Embeddings:       SupportConditional,
			Reranking:        SupportUnknown,
			ReasoningControl: SupportConditional,
			Tools:            ToolCapabilities{Calling: SupportConditional, Streaming: SupportUnknown},
			StructuredOutput: SupportConditional,
			Streaming:        StreamingCapabilities{Content: SupportSupported, Reasoning: SupportConditional, ToolArguments: SupportUnknown},
			Placement:        PlacementCapabilities{CPU: SupportSupported, GPU: SupportConditional, Hybrid: SupportConditional},
			Context:          ContextCapabilities{BackendManaged: true, OverrideSupported: SupportConditional},
		},
	}
}
