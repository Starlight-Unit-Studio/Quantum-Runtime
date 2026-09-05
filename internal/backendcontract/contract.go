package backendcontract

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

const ContractVersion = "quantum.runtime/backend/v1alpha1"

type Support string

const (
	SupportSupported   Support = "supported"
	SupportUnsupported Support = "unsupported"
	SupportConditional Support = "conditional"
	SupportUnknown     Support = "unknown"
)

type ArchitectureCapabilities struct {
	Dense Support `json:"dense"`
	MoE   Support `json:"moe"`
}

type MoECapabilities struct {
	ExpertOffload  Support `json:"expert_offload"`
	ExpertParallel Support `json:"expert_parallel"`
}

type SpeculativeCapabilities struct {
	MTP        Support `json:"mtp"`
	DraftModel Support `json:"draft_model"`
}

type CacheCapabilities struct {
	KVOffload   Support `json:"kv_offload"`
	PromptCache Support `json:"prompt_cache"`
}

type MultimodalCapabilities struct {
	Vision Support `json:"vision"`
	Audio  Support `json:"audio"`
}

type ToolCapabilities struct {
	Calling   Support `json:"calling"`
	Streaming Support `json:"streaming"`
}

type StreamingCapabilities struct {
	Content       Support `json:"content"`
	Reasoning     Support `json:"reasoning"`
	ToolArguments Support `json:"tool_arguments"`
}

type PlacementCapabilities struct {
	CPU    Support `json:"cpu"`
	GPU    Support `json:"gpu"`
	Hybrid Support `json:"hybrid"`
}

type ContextCapabilities struct {
	BackendManaged    bool    `json:"backend_managed"`
	OverrideSupported Support `json:"override_supported"`
	OverrideVerified  bool    `json:"override_verified"`
	NativeMax         int     `json:"native_max,omitempty"`
}

type Capabilities struct {
	Text                Support                  `json:"text"`
	Architecture        ArchitectureCapabilities `json:"architecture"`
	MoE                 MoECapabilities          `json:"moe"`
	QuantizationFormats []string                 `json:"quantization_formats,omitempty"`
	Speculative         SpeculativeCapabilities  `json:"speculative"`
	Cache               CacheCapabilities        `json:"cache"`
	Multimodal          MultimodalCapabilities   `json:"multimodal"`
	Embeddings          Support                  `json:"embeddings"`
	Reranking           Support                  `json:"reranking"`
	ReasoningControl    Support                  `json:"reasoning_control"`
	Tools               ToolCapabilities         `json:"tools"`
	StructuredOutput    Support                  `json:"structured_output"`
	Streaming           StreamingCapabilities    `json:"streaming"`
	Placement           PlacementCapabilities    `json:"placement"`
	Context             ContextCapabilities      `json:"context"`
}

type Descriptor struct {
	ContractVersion string       `json:"contract_version"`
	ID              string       `json:"id"`
	Kind            string       `json:"kind"`
	AdapterVersion  string       `json:"adapter_version"`
	ExecutionMode   string       `json:"execution_mode"`
	State           string       `json:"state"`
	Capabilities    Capabilities `json:"capabilities"`
}

type Backend interface {
	Do(context.Context, *http.Request) (*http.Response, error)
	Ready(context.Context) error
	Descriptor() Descriptor
}

func (d Descriptor) Validate() error {
	if d.ContractVersion != ContractVersion {
		return fmt.Errorf("contract_version must be %q", ContractVersion)
	}
	if !validToken(d.ID) {
		return fmt.Errorf("backend id %q is invalid", d.ID)
	}
	switch d.Kind {
	case "ollama-adapter", "llama.cpp", "mlx", "vllm", "external":
	default:
		return fmt.Errorf("backend kind %q is invalid", d.Kind)
	}
	if strings.TrimSpace(d.AdapterVersion) == "" {
		return fmt.Errorf("adapter_version is required")
	}
	switch d.ExecutionMode {
	case "external", "process", "library":
	default:
		return fmt.Errorf("execution_mode %q is invalid", d.ExecutionMode)
	}
	switch d.State {
	case "ready", "unknown", "unavailable", "disabled":
	default:
		return fmt.Errorf("backend state %q is invalid", d.State)
	}
	if err := d.Capabilities.Validate(); err != nil {
		return err
	}
	return nil
}

func (c Capabilities) Validate() error {
	supports := []struct {
		name  string
		value Support
	}{
		{"text", c.Text},
		{"architecture.dense", c.Architecture.Dense},
		{"architecture.moe", c.Architecture.MoE},
		{"moe.expert_offload", c.MoE.ExpertOffload},
		{"moe.expert_parallel", c.MoE.ExpertParallel},
		{"speculative.mtp", c.Speculative.MTP},
		{"speculative.draft_model", c.Speculative.DraftModel},
		{"cache.kv_offload", c.Cache.KVOffload},
		{"cache.prompt_cache", c.Cache.PromptCache},
		{"multimodal.vision", c.Multimodal.Vision},
		{"multimodal.audio", c.Multimodal.Audio},
		{"embeddings", c.Embeddings},
		{"reranking", c.Reranking},
		{"reasoning.control", c.ReasoningControl},
		{"tools.calling", c.Tools.Calling},
		{"tools.streaming", c.Tools.Streaming},
		{"structured_output", c.StructuredOutput},
		{"streaming.content", c.Streaming.Content},
		{"streaming.reasoning", c.Streaming.Reasoning},
		{"streaming.tool_arguments", c.Streaming.ToolArguments},
		{"placement.cpu", c.Placement.CPU},
		{"placement.gpu", c.Placement.GPU},
		{"placement.hybrid", c.Placement.Hybrid},
		{"context.override", c.Context.OverrideSupported},
	}
	for _, item := range supports {
		if !validSupport(item.value) {
			return fmt.Errorf("capability %s has invalid support state %q", item.name, item.value)
		}
	}
	if c.Context.NativeMax < 0 {
		return fmt.Errorf("context.native_max must not be negative")
	}
	if c.Context.OverrideVerified && c.Context.OverrideSupported != SupportSupported {
		return fmt.Errorf("context override cannot be verified unless override support is supported")
	}
	seen := map[string]struct{}{}
	for _, format := range c.QuantizationFormats {
		normalized := strings.TrimSpace(format)
		if normalized == "" {
			return fmt.Errorf("quantization format must not be empty")
		}
		if _, exists := seen[normalized]; exists {
			return fmt.Errorf("quantization format %q is duplicated", normalized)
		}
		seen[normalized] = struct{}{}
	}
	return nil
}

func (c Capabilities) State(name string) Support {
	switch name {
	case "inference.text":
		return c.Text
	case "architecture.dense":
		return c.Architecture.Dense
	case "architecture.moe":
		return c.Architecture.MoE
	case "moe.expert_offload":
		return c.MoE.ExpertOffload
	case "moe.expert_parallel":
		return c.MoE.ExpertParallel
	case "speculative.mtp":
		return c.Speculative.MTP
	case "speculative.draft_model":
		return c.Speculative.DraftModel
	case "cache.kv_offload":
		return c.Cache.KVOffload
	case "cache.prompt_cache":
		return c.Cache.PromptCache
	case "multimodal.vision":
		return c.Multimodal.Vision
	case "multimodal.audio":
		return c.Multimodal.Audio
	case "embeddings":
		return c.Embeddings
	case "reranking":
		return c.Reranking
	case "reasoning.control":
		return c.ReasoningControl
	case "tools.calling":
		return c.Tools.Calling
	case "tools.streaming":
		return c.Tools.Streaming
	case "structured_output":
		return c.StructuredOutput
	case "streaming.content":
		return c.Streaming.Content
	case "streaming.reasoning":
		return c.Streaming.Reasoning
	case "streaming.tool_arguments":
		return c.Streaming.ToolArguments
	case "placement.cpu":
		return c.Placement.CPU
	case "placement.gpu":
		return c.Placement.GPU
	case "placement.hybrid":
		return c.Placement.Hybrid
	case "context.override":
		return c.Context.OverrideSupported
	default:
		return SupportUnknown
	}
}

func (c Capabilities) DeclaredNames() []string {
	names := []string{
		"inference.text", "architecture.dense", "architecture.moe",
		"moe.expert_offload", "moe.expert_parallel", "speculative.mtp",
		"speculative.draft_model", "cache.kv_offload", "cache.prompt_cache",
		"multimodal.vision", "multimodal.audio", "embeddings", "reranking",
		"reasoning.control", "tools.calling", "tools.streaming", "structured_output",
		"streaming.content", "streaming.reasoning", "streaming.tool_arguments",
		"placement.cpu", "placement.gpu", "placement.hybrid", "context.override",
	}
	sort.Strings(names)
	return names
}

func validSupport(value Support) bool {
	switch value {
	case SupportSupported, SupportUnsupported, SupportConditional, SupportUnknown:
		return true
	default:
		return false
	}
}

func validToken(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			if i == 0 && (r == '.' || r == '_' || r == '-') {
				return false
			}
			continue
		}
		return false
	}
	return true
}
