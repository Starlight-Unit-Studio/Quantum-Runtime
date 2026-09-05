from pathlib import Path
import json


def write(path: str, content: str) -> None:
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(content, encoding="utf-8")


def replace_once(path: str, old: str, new: str) -> None:
    target = Path(path)
    text = target.read_text(encoding="utf-8")
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected exactly one replacement marker, found {count}: {old!r}")
    target.write_text(text.replace(old, new, 1), encoding="utf-8")


write("internal/backendcontract/contract.go", r'''package backendcontract

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
    BackendManaged   bool    `json:"backend_managed"`
    OverrideSupported Support `json:"override_supported"`
    OverrideVerified bool    `json:"override_verified"`
    NativeMax        int     `json:"native_max,omitempty"`
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
''')

write("internal/backendcontract/contract_test.go", r'''package backendcontract

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
            Text: SupportConditional,
            Architecture: ArchitectureCapabilities{Dense: SupportConditional, MoE: SupportConditional},
            MoE: MoECapabilities{ExpertOffload: SupportConditional, ExpertParallel: SupportUnknown},
            Speculative: SpeculativeCapabilities{MTP: SupportConditional, DraftModel: SupportConditional},
            Cache: CacheCapabilities{KVOffload: SupportConditional, PromptCache: SupportConditional},
            Multimodal: MultimodalCapabilities{Vision: SupportConditional, Audio: SupportConditional},
            Embeddings:       SupportConditional,
            Reranking:        SupportUnknown,
            ReasoningControl: SupportConditional,
            Tools: ToolCapabilities{Calling: SupportConditional, Streaming: SupportUnknown},
            StructuredOutput: SupportConditional,
            Streaming: StreamingCapabilities{Content: SupportSupported, Reasoning: SupportConditional, ToolArguments: SupportUnknown},
            Placement: PlacementCapabilities{CPU: SupportSupported, GPU: SupportConditional, Hybrid: SupportConditional},
            Context: ContextCapabilities{BackendManaged: true, OverrideSupported: SupportConditional},
        },
    }
}
''')

write("internal/backendrouter/router.go", r'''package backendrouter

import (
    "errors"
    "fmt"
    "sort"

    "github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/backendcontract"
    "github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/modelmanifest"
    "github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/modelpolicy"
    "github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/modelregistry"
)

var (
    ErrModelNotFound       = errors.New("model not found")
    ErrNoCompatibleBackend = errors.New("no compatible backend")
)

type Requirements struct {
    Capabilities []string `json:"capabilities,omitempty"`
}

type Plan struct {
    RequestedIdentifier   string                              `json:"requested_identifier"`
    CanonicalModelID      string                              `json:"canonical_model_id"`
    BackendID             string                              `json:"backend_id"`
    BackendKind           string                              `json:"backend_kind"`
    ArtifactURI           string                              `json:"artifact_uri"`
    RequiredCapabilities  []string                            `json:"required_capabilities,omitempty"`
    Context               backendcontract.ContextCapabilities `json:"context"`
    ModelPolicyIDs        []string                            `json:"model_policy_ids,omitempty"`
}

type Router struct {
    registry *modelregistry.Registry
    backends []backendcontract.Descriptor
    policies []modelpolicy.Policy
}

func New(registry *modelregistry.Registry, backends []backendcontract.Descriptor, policies []modelpolicy.Policy) (*Router, error) {
    if registry == nil {
        return nil, fmt.Errorf("registry is required")
    }
    copied := append([]backendcontract.Descriptor(nil), backends...)
    for _, descriptor := range copied {
        if err := descriptor.Validate(); err != nil {
            return nil, fmt.Errorf("backend %q: %w", descriptor.ID, err)
        }
    }
    sort.Slice(copied, func(i, j int) bool { return copied[i].ID < copied[j].ID })
    return &Router{registry: registry, backends: copied, policies: append([]modelpolicy.Policy(nil), policies...)}, nil
}

func (r *Router) Route(identifier string, requirements Requirements) (Plan, error) {
    manifest, ok := r.registry.Lookup(identifier)
    if !ok {
        return Plan{}, fmt.Errorf("%w: %s", ErrModelNotFound, identifier)
    }

    required := normalizedRequirements(requirements.Capabilities)
    candidates := candidateArtifacts(manifest)
    for _, candidate := range candidates {
        for _, descriptor := range r.backends {
            if descriptor.Kind != candidate.backendKind || descriptor.State == "disabled" || descriptor.State == "unavailable" {
                continue
            }
            if !requirementsSatisfied(manifest, descriptor, required) {
                continue
            }
            matchedPolicies := modelpolicy.Match(r.policies, manifest.Model.Architecture, manifest.Model.ParameterClass, descriptor.Kind)
            policyIDs := make([]string, 0, len(matchedPolicies))
            for _, policy := range matchedPolicies {
                policyIDs = append(policyIDs, policy.ID)
            }
            return Plan{
                RequestedIdentifier:  identifier,
                CanonicalModelID:     manifest.ID,
                BackendID:            descriptor.ID,
                BackendKind:          descriptor.Kind,
                ArtifactURI:          candidate.uri,
                RequiredCapabilities: required,
                Context:              descriptor.Capabilities.Context,
                ModelPolicyIDs:       policyIDs,
            }, nil
        }
    }
    return Plan{}, fmt.Errorf("%w for canonical model %q", ErrNoCompatibleBackend, manifest.ID)
}

type artifactCandidate struct {
    backendKind string
    uri         string
}

func candidateArtifacts(manifest modelmanifest.Manifest) []artifactCandidate {
    candidates := make([]artifactCandidate, 0, len(manifest.Artifacts))
    seen := map[string]struct{}{}
    for _, artifact := range manifest.Artifacts {
        kind := artifact.Backend
        if kind == "" {
            kind = manifest.Backend.Type
        }
        key := kind + "\x00" + artifact.URI
        if _, exists := seen[key]; exists {
            continue
        }
        seen[key] = struct{}{}
        candidates = append(candidates, artifactCandidate{backendKind: kind, uri: artifact.URI})
    }
    sort.SliceStable(candidates, func(i, j int) bool {
        if candidates[i].backendKind == candidates[j].backendKind {
            return candidates[i].uri < candidates[j].uri
        }
        return candidates[i].backendKind < candidates[j].backendKind
    })
    return candidates
}

func requirementsSatisfied(manifest modelmanifest.Manifest, descriptor backendcontract.Descriptor, required []string) bool {
    for _, name := range required {
        if !modelSupports(manifest, name) {
            return false
        }
        state := descriptor.Capabilities.State(name)
        if state != backendcontract.SupportSupported && state != backendcontract.SupportConditional {
            return false
        }
    }
    return true
}

func modelSupports(manifest modelmanifest.Manifest, name string) bool {
    switch name {
    case "inference.text":
        return manifest.Capabilities.Text
    case "architecture.dense":
        return manifest.Model.ArchitectureClass == "dense"
    case "architecture.moe":
        return manifest.Model.ArchitectureClass == "moe"
    case "multimodal.vision":
        return manifest.Capabilities.Vision
    case "multimodal.audio":
        return manifest.Capabilities.Audio
    case "embeddings":
        return manifest.Capabilities.Embeddings
    case "reranking":
        return manifest.Capabilities.Reranking
    case "reasoning.control":
        return manifest.Capabilities.Thinking || manifest.Capabilities.ReasoningControl
    case "tools.calling":
        return manifest.Capabilities.Tools
    case "tools.streaming":
        return manifest.Capabilities.Tools && manifest.Capabilities.ToolStreaming
    case "structured_output":
        return manifest.Capabilities.StructuredOutput
    default:
        // Execution-only capabilities are evaluated at the backend. Unknown model-level
        // names are deliberately rejected instead of being assumed compatible.
        switch name {
        case "moe.expert_offload", "moe.expert_parallel":
            return manifest.Model.ArchitectureClass == "moe"
        case "speculative.mtp", "speculative.draft_model", "cache.kv_offload", "cache.prompt_cache",
            "streaming.content", "streaming.reasoning", "streaming.tool_arguments",
            "placement.cpu", "placement.gpu", "placement.hybrid", "context.override":
            return true
        default:
            return false
        }
    }
}

func normalizedRequirements(values []string) []string {
    seen := map[string]struct{}{}
    normalized := make([]string, 0, len(values))
    for _, value := range values {
        if value == "" {
            continue
        }
        if _, exists := seen[value]; exists {
            continue
        }
        seen[value] = struct{}{}
        normalized = append(normalized, value)
    }
    sort.Strings(normalized)
    return normalized
}
''')

write("internal/backendrouter/router_test.go", r'''package backendrouter

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
            Text: backendcontract.SupportConditional,
            Architecture: backendcontract.ArchitectureCapabilities{Dense: backendcontract.SupportConditional, MoE: backendcontract.SupportConditional},
            MoE: backendcontract.MoECapabilities{ExpertOffload: backendcontract.SupportConditional, ExpertParallel: backendcontract.SupportUnknown},
            Speculative: backendcontract.SpeculativeCapabilities{MTP: backendcontract.SupportConditional, DraftModel: backendcontract.SupportConditional},
            Cache: backendcontract.CacheCapabilities{KVOffload: backendcontract.SupportConditional, PromptCache: backendcontract.SupportConditional},
            Multimodal: backendcontract.MultimodalCapabilities{Vision: backendcontract.SupportConditional, Audio: backendcontract.SupportConditional},
            Embeddings:       backendcontract.SupportConditional,
            Reranking:        backendcontract.SupportUnknown,
            ReasoningControl: backendcontract.SupportConditional,
            Tools: backendcontract.ToolCapabilities{Calling: backendcontract.SupportConditional, Streaming: backendcontract.SupportUnknown},
            StructuredOutput: backendcontract.SupportConditional,
            Streaming: backendcontract.StreamingCapabilities{Content: backendcontract.SupportSupported, Reasoning: backendcontract.SupportConditional, ToolArguments: backendcontract.SupportUnknown},
            Placement: backendcontract.PlacementCapabilities{CPU: backendcontract.SupportSupported, GPU: backendcontract.SupportConditional, Hybrid: backendcontract.SupportConditional},
            Context: backendcontract.ContextCapabilities{BackendManaged: true, OverrideSupported: backendcontract.SupportConditional},
        },
    }
}
''')

write("internal/modelpolicy/policy.go", r'''package modelpolicy

import (
    "bytes"
    "embed"
    "encoding/json"
    "fmt"
    "io"
    "io/fs"
    "sort"
    "strings"
    "time"
)

const SchemaVersion = "quantum.runtime/model-policy/v1alpha1"

//go:embed data/*.json
var builtinFS embed.FS

type ContextPolicy struct {
    BackendManaged    bool `json:"backend_managed"`
    OverrideSupported bool `json:"override_supported"`
    OverrideVerified  bool `json:"override_verified"`
    NativeMax         int  `json:"native_max,omitempty"`
}

type ParameterRule struct {
    Name  string          `json:"name"`
    Class string          `json:"class"`
    Value json.RawMessage `json:"value,omitempty"`
}

type Validation struct {
    ObservedDate string   `json:"observed_date"`
    Sources      []string `json:"sources"`
    Notes        string   `json:"notes,omitempty"`
}

type Policy struct {
    SchemaVersion string          `json:"schema_version"`
    ID            string          `json:"id"`
    ModelFamily   string          `json:"model_family"`
    Variants      []string        `json:"variants"`
    BackendKind   string          `json:"backend_kind"`
    HardwareClass string          `json:"hardware_class"`
    Renderer      string          `json:"renderer,omitempty"`
    Parser        string          `json:"parser,omitempty"`
    Context       ContextPolicy   `json:"context"`
    Parameters    []ParameterRule `json:"parameters"`
    Validation    Validation      `json:"validation"`
}

func (p Policy) Validate() error {
    if p.SchemaVersion != SchemaVersion {
        return fmt.Errorf("schema_version must be %q", SchemaVersion)
    }
    if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.ModelFamily) == "" || strings.TrimSpace(p.BackendKind) == "" {
        return fmt.Errorf("id, model_family and backend_kind are required")
    }
    if len(p.Variants) == 0 {
        return fmt.Errorf("at least one variant is required")
    }
    if p.Context.NativeMax < 0 {
        return fmt.Errorf("context.native_max must not be negative")
    }
    if p.Context.OverrideVerified && !p.Context.OverrideSupported {
        return fmt.Errorf("context override cannot be verified when override_supported=false")
    }
    seenVariants := map[string]struct{}{}
    for _, variant := range p.Variants {
        if strings.TrimSpace(variant) == "" {
            return fmt.Errorf("variant must not be empty")
        }
        if _, exists := seenVariants[variant]; exists {
            return fmt.Errorf("variant %q is duplicated", variant)
        }
        seenVariants[variant] = struct{}{}
    }
    seenParameters := map[string]struct{}{}
    for _, parameter := range p.Parameters {
        if strings.TrimSpace(parameter.Name) == "" {
            return fmt.Errorf("parameter name must not be empty")
        }
        if _, exists := seenParameters[parameter.Name]; exists {
            return fmt.Errorf("parameter %q is duplicated", parameter.Name)
        }
        seenParameters[parameter.Name] = struct{}{}
        switch parameter.Class {
        case "required", "known-good", "optional", "blocked-unverified":
        default:
            return fmt.Errorf("parameter %q has invalid class %q", parameter.Name, parameter.Class)
        }
        if len(parameter.Value) != 0 && !json.Valid(parameter.Value) {
            return fmt.Errorf("parameter %q has invalid JSON value", parameter.Name)
        }
    }
    if _, err := time.Parse("2006-01-02", p.Validation.ObservedDate); err != nil {
        return fmt.Errorf("validation.observed_date must use YYYY-MM-DD")
    }
    if len(p.Validation.Sources) == 0 {
        return fmt.Errorf("at least one validation source is required")
    }
    return nil
}

func Builtin() ([]Policy, error) {
    entries, err := fs.ReadDir(builtinFS, "data")
    if err != nil {
        return nil, fmt.Errorf("read builtin model policies: %w", err)
    }
    policies := make([]Policy, 0, len(entries))
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
            continue
        }
        data, err := builtinFS.ReadFile("data/" + entry.Name())
        if err != nil {
            return nil, err
        }
        decoder := json.NewDecoder(bytes.NewReader(data))
        decoder.DisallowUnknownFields()
        var policy Policy
        if err := decoder.Decode(&policy); err != nil {
            return nil, fmt.Errorf("decode policy %s: %w", entry.Name(), err)
        }
        var trailing any
        if err := decoder.Decode(&trailing); err != io.EOF {
            return nil, fmt.Errorf("policy %s contains trailing JSON", entry.Name())
        }
        if err := policy.Validate(); err != nil {
            return nil, fmt.Errorf("policy %s: %w", entry.Name(), err)
        }
        policies = append(policies, policy)
    }
    sort.Slice(policies, func(i, j int) bool { return policies[i].ID < policies[j].ID })
    return policies, nil
}

func MustBuiltin() []Policy {
    policies, err := Builtin()
    if err != nil {
        panic(fmt.Sprintf("invalid builtin Quantum Runtime model policies: %v", err))
    }
    return policies
}

func Match(policies []Policy, modelFamily, variant, backendKind string) []Policy {
    matched := make([]Policy, 0)
    for _, policy := range policies {
        if policy.ModelFamily != modelFamily || policy.BackendKind != backendKind {
            continue
        }
        for _, candidate := range policy.Variants {
            if candidate == variant || candidate == "*" {
                matched = append(matched, policy)
                break
            }
        }
    }
    sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
    return matched
}
''')

write("internal/modelpolicy/policy_test.go", r'''package modelpolicy

import "testing"

func TestBuiltinPoliciesValidate(t *testing.T) {
    policies, err := Builtin()
    if err != nil {
        t.Fatalf("builtin policies rejected: %v", err)
    }
    if len(policies) == 0 {
        t.Fatal("expected at least one policy")
    }
}

func TestGemmaPolicyMatchesProfileDependentCoreUI(t *testing.T) {
    matched := Match(MustBuiltin(), "gemma4", "profile_dependent", "ollama-adapter")
    if len(matched) != 1 || matched[0].ID != "gemma4-ollama-turin-minimal" {
        t.Fatalf("unexpected policies: %#v", matched)
    }
}

func TestGemmaPolicyDoesNotLeakToOtherFamilies(t *testing.T) {
    if matched := Match(MustBuiltin(), "qwen", "30b-a3b", "ollama-adapter"); len(matched) != 0 {
        t.Fatalf("unexpected cross-family match: %#v", matched)
    }
}
''')

write("internal/modelpolicy/data/gemma4-ollama-turin.json", r'''{
  "schema_version": "quantum.runtime/model-policy/v1alpha1",
  "id": "gemma4-ollama-turin-minimal",
  "model_family": "gemma4",
  "variants": ["e2b", "e4b", "26b-a4b", "profile_dependent"],
  "backend_kind": "ollama-adapter",
  "hardware_class": "amd-epyc-turin-cpu",
  "renderer": "gemma4",
  "parser": "gemma4",
  "context": {
    "backend_managed": true,
    "override_supported": true,
    "override_verified": false
  },
  "parameters": [
    {"name": "temperature", "class": "known-good", "value": 1.0},
    {"name": "top_k", "class": "known-good", "value": 64},
    {"name": "top_p", "class": "known-good", "value": 0.95},
    {"name": "num_ctx", "class": "blocked-unverified"},
    {"name": "num_predict", "class": "blocked-unverified"},
    {"name": "num_thread", "class": "blocked-unverified"},
    {"name": "repeat_penalty", "class": "blocked-unverified"},
    {"name": "repeat_last_n", "class": "blocked-unverified"},
    {"name": "seed", "class": "blocked-unverified"},
    {"name": "stop", "class": "blocked-unverified"}
  ],
  "validation": {
    "observed_date": "2026-09-04",
    "sources": [
      "ember-coreui-extended-manual-validation",
      "game-ember-26b-downstream-confirmation"
    ],
    "notes": "The observed throughput gain belongs to the complete minimal profile. num_ctx is not recorded as the sole cause; each removed generation/context/predict control remains blocked-unverified until isolated controlled A/B validation."
  }
}
''')

write("internal/upstreamledger/ledger.go", r'''package upstreamledger

import (
    "bytes"
    "embed"
    "encoding/json"
    "fmt"
    "io"
    "net/url"
    "sort"
    "strings"
    "time"
)

const SchemaVersion = "quantum.runtime/upstream-ledger/v1alpha1"

//go:embed data/ledger.json
var dataFS embed.FS

type Entry struct {
    ID                    string   `json:"id"`
    Project               string   `json:"project"`
    BackendKind           string   `json:"backend_kind"`
    SourceURL             string   `json:"source_url"`
    TestedRef             string   `json:"tested_ref,omitempty"`
    Status                string   `json:"status"`
    ObservedAt            string   `json:"observed_at,omitempty"`
    EnabledCapabilities   []string `json:"enabled_capabilities,omitempty"`
    DisabledCapabilities  []string `json:"disabled_capabilities,omitempty"`
    HardwareClasses       []string `json:"hardware_classes,omitempty"`
    LicenseRef            string   `json:"license_ref,omitempty"`
    Notes                 string   `json:"notes,omitempty"`
}

type Ledger struct {
    SchemaVersion string  `json:"schema_version"`
    Entries       []Entry `json:"entries"`
}

func (l Ledger) Validate() error {
    if l.SchemaVersion != SchemaVersion {
        return fmt.Errorf("schema_version must be %q", SchemaVersion)
    }
    if len(l.Entries) == 0 {
        return fmt.Errorf("ledger requires at least one entry")
    }
    seen := map[string]struct{}{}
    for _, entry := range l.Entries {
        if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.Project) == "" || strings.TrimSpace(entry.BackendKind) == "" {
            return fmt.Errorf("ledger entry id, project and backend_kind are required")
        }
        if _, exists := seen[entry.ID]; exists {
            return fmt.Errorf("duplicate ledger entry %q", entry.ID)
        }
        seen[entry.ID] = struct{}{}
        parsed, err := url.Parse(entry.SourceURL)
        if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
            return fmt.Errorf("ledger entry %q source_url must be absolute HTTPS", entry.ID)
        }
        switch entry.Status {
        case "tested", "observed-unpinned", "planned", "blocked":
        default:
            return fmt.Errorf("ledger entry %q has invalid status %q", entry.ID, entry.Status)
        }
        if entry.ObservedAt != "" {
            if _, err := time.Parse(time.RFC3339, entry.ObservedAt); err != nil {
                return fmt.Errorf("ledger entry %q observed_at must use RFC3339", entry.ID)
            }
        }
        if entry.Status == "tested" {
            if strings.TrimSpace(entry.TestedRef) == "" || entry.ObservedAt == "" || strings.TrimSpace(entry.LicenseRef) == "" {
                return fmt.Errorf("tested ledger entry %q requires tested_ref, observed_at and license_ref", entry.ID)
            }
        }
    }
    return nil
}

func Builtin() (Ledger, error) {
    data, err := dataFS.ReadFile("data/ledger.json")
    if err != nil {
        return Ledger{}, err
    }
    decoder := json.NewDecoder(bytes.NewReader(data))
    decoder.DisallowUnknownFields()
    var ledger Ledger
    if err := decoder.Decode(&ledger); err != nil {
        return Ledger{}, fmt.Errorf("decode upstream ledger: %w", err)
    }
    var trailing any
    if err := decoder.Decode(&trailing); err != io.EOF {
        return Ledger{}, fmt.Errorf("upstream ledger contains trailing JSON")
    }
    if err := ledger.Validate(); err != nil {
        return Ledger{}, err
    }
    sort.Slice(ledger.Entries, func(i, j int) bool { return ledger.Entries[i].ID < ledger.Entries[j].ID })
    return ledger, nil
}

func MustBuiltin() Ledger {
    ledger, err := Builtin()
    if err != nil {
        panic(fmt.Sprintf("invalid Quantum Runtime upstream ledger: %v", err))
    }
    return ledger
}

func (l Ledger) LatestKnownGood() []Entry {
    entries := make([]Entry, 0)
    for _, entry := range l.Entries {
        if entry.Status == "tested" {
            entries = append(entries, entry)
        }
    }
    sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
    return entries
}
''')

write("internal/upstreamledger/ledger_test.go", r'''package upstreamledger

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
''')

write("internal/upstreamledger/data/ledger.json", r'''{
  "schema_version": "quantum.runtime/upstream-ledger/v1alpha1",
  "entries": [
    {
      "id": "ollama-adoption-observed-2026-09-04",
      "project": "Ollama",
      "backend_kind": "ollama-adapter",
      "source_url": "https://github.com/ollama/ollama",
      "status": "observed-unpinned",
      "observed_at": "2026-09-04T00:00:00Z",
      "enabled_capabilities": [
        "inference.text",
        "multimodal.vision",
        "reasoning.control",
        "streaming.content"
      ],
      "disabled_capabilities": [
        "Gemma 4 context/generation/predict overrides remain blocked-unverified on the Turin profile until isolated controlled A/B validation"
      ],
      "hardware_classes": ["amd-epyc-turin-cpu"],
      "license_ref": "upstream-license-must-be-verified-at-pin",
      "notes": "Extended CoreUI validation and downstream Game confirmation exist, but the exact Ollama upstream version was not recorded. This observation is intentionally ineligible as a latest-known-good production pin."
    },
    {
      "id": "llama-cpp-native-planned",
      "project": "llama.cpp / ggml",
      "backend_kind": "llama.cpp",
      "source_url": "https://github.com/ggml-org/llama.cpp",
      "status": "planned",
      "enabled_capabilities": [],
      "disabled_capabilities": [],
      "hardware_classes": ["linux-cpu", "linux-hybrid"],
      "license_ref": "upstream-license-must-be-verified-at-pin",
      "notes": "First native portable backend priority. No upstream tag or commit is promoted until the Runtime conformance suite passes."
    }
  ]
}
''')

write("internal/modelmanifest/manifest.go", r'''package modelmanifest

import (
    "fmt"
    "net/url"
    "regexp"
    "sort"
    "strconv"
    "strings"
    "time"
)

const SchemaVersion = "quantum.runtime/model-manifest/v1alpha1"

var (
    canonicalIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
    aliasPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,159}$`)
    labelPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$`)
    digestPattern      = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
    versionPattern     = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
)

type Manifest struct {
    SchemaVersion string        `json:"schema_version"`
    ID            string        `json:"id"`
    DisplayName   string        `json:"display_name"`
    Aliases       []string      `json:"aliases,omitempty"`
    Source        Source        `json:"source"`
    Backend       Backend       `json:"backend"`
    Artifacts     []Artifact    `json:"artifacts"`
    Model         Model         `json:"model"`
    Capabilities  Capabilities  `json:"capabilities"`
    Compatibility Compatibility `json:"compatibility"`
    Persona       *PersonaRef   `json:"persona,omitempty"`
    State         State         `json:"state"`
    Provenance    Provenance    `json:"provenance"`
}

type Source struct {
    Provider  string `json:"provider"`
    Reference string `json:"reference"`
    Revision  string `json:"revision"`
}

type Backend struct {
    Type string `json:"type"`
}

type Artifact struct {
    URI     string `json:"uri"`
    SHA256  string `json:"sha256,omitempty"`
    Backend string `json:"backend,omitempty"`
    Format  string `json:"format,omitempty"`
    Role    string `json:"role,omitempty"`
}

type ExpertTopology struct {
    Total  int `json:"total,omitempty"`
    Active int `json:"active,omitempty"`
    Shared int `json:"shared,omitempty"`
}

type ModelContextPolicy struct {
    NativeMax          int  `json:"native_max,omitempty"`
    BackendManaged     bool `json:"backend_managed,omitempty"`
    OverrideSupported  bool `json:"override_supported,omitempty"`
    OverrideVerified   bool `json:"override_verified,omitempty"`
}

type Model struct {
    Architecture      string             `json:"architecture"`
    ArchitectureClass string             `json:"architecture_class,omitempty"`
    ParameterClass    string             `json:"parameter_class"`
    TotalParametersB  float64            `json:"total_parameters_b,omitempty"`
    ActiveParametersB float64            `json:"active_parameters_b,omitempty"`
    Experts           *ExpertTopology    `json:"experts,omitempty"`
    Quantization      string             `json:"quantization"`
    ContextWindow     int                `json:"context_window"`
    ContextPolicy     ModelContextPolicy `json:"context_policy,omitempty"`
}

type Capabilities struct {
    Text             bool `json:"text"`
    Vision           bool `json:"vision"`
    Audio            bool `json:"audio"`
    Embeddings       bool `json:"embeddings"`
    Reranking        bool `json:"reranking,omitempty"`
    Tools            bool `json:"tools"`
    ToolStreaming    bool `json:"tool_streaming,omitempty"`
    Thinking         bool `json:"thinking"`
    ReasoningControl bool `json:"reasoning_control,omitempty"`
    StructuredOutput bool `json:"structured_output,omitempty"`
}

type Compatibility struct {
    MinRuntimeVersion string `json:"min_runtime_version"`
    MaxRuntimeVersion string `json:"max_runtime_version,omitempty"`
}

type PersonaRef struct {
    Package string `json:"package"`
    Version string `json:"version"`
    SHA256  string `json:"sha256,omitempty"`
}

type State struct {
    Install      string `json:"install"`
    Verification string `json:"verification"`
    Lifecycle    string `json:"lifecycle"`
}

type Provenance struct {
    Publisher   string `json:"publisher"`
    SourceURL   string `json:"source_url,omitempty"`
    LicenseRef  string `json:"license_ref,omitempty"`
    RetrievedAt string `json:"retrieved_at,omitempty"`
}

func (m Manifest) Validate() error {
    if m.SchemaVersion != SchemaVersion {
        return fmt.Errorf("schema_version must be %q", SchemaVersion)
    }
    if !canonicalIDPattern.MatchString(m.ID) {
        return fmt.Errorf("id %q is not a valid canonical model identifier", m.ID)
    }
    if strings.TrimSpace(m.DisplayName) == "" || len(m.DisplayName) > 160 {
        return fmt.Errorf("display_name must contain 1 to 160 characters")
    }
    if err := validateAliases(m.ID, m.Aliases); err != nil {
        return err
    }
    if err := validateSource(m.Source, m.State.Verification); err != nil {
        return err
    }
    if err := validateBackend(m.Backend); err != nil {
        return err
    }
    if err := validateArtifacts(m.Artifacts, m.State.Verification); err != nil {
        return err
    }
    if err := validateModel(m.Model); err != nil {
        return err
    }
    if !m.Capabilities.Text && !m.Capabilities.Vision && !m.Capabilities.Audio && !m.Capabilities.Embeddings && !m.Capabilities.Reranking && !m.Capabilities.Tools && !m.Capabilities.Thinking && !m.Capabilities.ReasoningControl && !m.Capabilities.StructuredOutput {
        return fmt.Errorf("at least one capability must be declared")
    }
    if m.Capabilities.ToolStreaming && !m.Capabilities.Tools {
        return fmt.Errorf("tool_streaming requires tools=true")
    }
    if err := validateCompatibility(m.Compatibility); err != nil {
        return err
    }
    if m.Persona != nil {
        if err := validatePersona(*m.Persona); err != nil {
            return err
        }
    }
    if err := validateState(m.State); err != nil {
        return err
    }
    if err := validateProvenance(m.Provenance); err != nil {
        return err
    }
    return nil
}

func validateAliases(id string, aliases []string) error {
    seen := map[string]struct{}{id: {}}
    for _, alias := range aliases {
        if !aliasPattern.MatchString(alias) {
            return fmt.Errorf("alias %q is invalid", alias)
        }
        if _, exists := seen[alias]; exists {
            return fmt.Errorf("alias %q is duplicated or equals the canonical id", alias)
        }
        seen[alias] = struct{}{}
    }
    return nil
}

func validateSource(source Source, verification string) error {
    if !labelPattern.MatchString(source.Provider) {
        return fmt.Errorf("source.provider %q is invalid", source.Provider)
    }
    if strings.TrimSpace(source.Reference) == "" || len(source.Reference) > 300 {
        return fmt.Errorf("source.reference must contain 1 to 300 characters")
    }
    if strings.TrimSpace(source.Revision) == "" || len(source.Revision) > 200 {
        return fmt.Errorf("source.revision must contain 1 to 200 characters")
    }
    if verification == "verified" && strings.EqualFold(source.Revision, "unresolved") {
        return fmt.Errorf("verified manifests require an immutable source revision")
    }
    return nil
}

func validateBackend(backend Backend) error {
    switch backend.Type {
    case "ollama-adapter", "llama.cpp", "mlx", "vllm", "gguf", "safetensors", "external":
        return nil
    default:
        return fmt.Errorf("backend.type %q is not supported by schema v1alpha1", backend.Type)
    }
}

func validateArtifacts(artifacts []Artifact, verification string) error {
    if len(artifacts) == 0 {
        return fmt.Errorf("at least one artifact reference is required")
    }
    seen := make(map[string]struct{}, len(artifacts))
    for _, artifact := range artifacts {
        parsed, err := url.Parse(artifact.URI)
        if err != nil || parsed.Scheme == "" || strings.ContainsAny(artifact.URI, "\r\n\t ") {
            return fmt.Errorf("artifact uri %q is invalid", artifact.URI)
        }
        key := artifact.Backend + "\x00" + artifact.URI
        if _, exists := seen[key]; exists {
            return fmt.Errorf("artifact uri %q is duplicated for backend %q", artifact.URI, artifact.Backend)
        }
        seen[key] = struct{}{}
        if artifact.SHA256 != "" && !digestPattern.MatchString(artifact.SHA256) {
            return fmt.Errorf("artifact %q has an invalid sha256 digest", artifact.URI)
        }
        if verification == "verified" && artifact.SHA256 == "" {
            return fmt.Errorf("verified manifests require a sha256 digest for artifact %q", artifact.URI)
        }
        if artifact.Backend != "" {
            if err := validateBackend(Backend{Type: artifact.Backend}); err != nil {
                return fmt.Errorf("artifact %q: %w", artifact.URI, err)
            }
        }
        if artifact.Format != "" && !labelPattern.MatchString(artifact.Format) {
            return fmt.Errorf("artifact %q format %q is invalid", artifact.URI, artifact.Format)
        }
        if artifact.Role != "" && !labelPattern.MatchString(artifact.Role) {
            return fmt.Errorf("artifact %q role %q is invalid", artifact.URI, artifact.Role)
        }
    }
    return nil
}

func validateModel(model Model) error {
    if !labelPattern.MatchString(model.Architecture) {
        return fmt.Errorf("model.architecture %q is invalid", model.Architecture)
    }
    if model.ArchitectureClass != "" && !contains([]string{"dense", "moe", "unknown"}, model.ArchitectureClass) {
        return fmt.Errorf("model.architecture_class %q is invalid", model.ArchitectureClass)
    }
    if !labelPattern.MatchString(model.ParameterClass) {
        return fmt.Errorf("model.parameter_class %q is invalid", model.ParameterClass)
    }
    if !labelPattern.MatchString(model.Quantization) {
        return fmt.Errorf("model.quantization %q is invalid", model.Quantization)
    }
    if model.ContextWindow < 1 || model.ContextWindow > 10_000_000 {
        return fmt.Errorf("model.context_window must be between 1 and 10000000")
    }
    if model.TotalParametersB < 0 || model.ActiveParametersB < 0 {
        return fmt.Errorf("model parameter counts must not be negative")
    }
    if model.TotalParametersB > 0 && model.ActiveParametersB > model.TotalParametersB {
        return fmt.Errorf("model.active_parameters_b must not exceed total_parameters_b")
    }
    if model.Experts != nil {
        if model.ArchitectureClass != "moe" {
            return fmt.Errorf("model.experts requires architecture_class=moe")
        }
        if model.Experts.Total < 1 || model.Experts.Active < 1 || model.Experts.Active > model.Experts.Total || model.Experts.Shared < 0 || model.Experts.Shared > model.Experts.Total {
            return fmt.Errorf("model.experts topology is invalid")
        }
    }
    if model.ContextPolicy.NativeMax < 0 {
        return fmt.Errorf("model.context_policy.native_max must not be negative")
    }
    if model.ContextPolicy.OverrideVerified && !model.ContextPolicy.OverrideSupported {
        return fmt.Errorf("model context override cannot be verified when override_supported=false")
    }
    return nil
}

func validateCompatibility(compat Compatibility) error {
    min, err := parseVersion(compat.MinRuntimeVersion)
    if err != nil {
        return fmt.Errorf("compatibility.min_runtime_version: %w", err)
    }
    if compat.MaxRuntimeVersion == "" {
        return nil
    }
    max, err := parseVersion(compat.MaxRuntimeVersion)
    if err != nil {
        return fmt.Errorf("compatibility.max_runtime_version: %w", err)
    }
    if compareVersion(max, min) < 0 {
        return fmt.Errorf("compatibility.max_runtime_version must not be lower than min_runtime_version")
    }
    return nil
}

func validatePersona(persona PersonaRef) error {
    if !canonicalIDPattern.MatchString(persona.Package) {
        return fmt.Errorf("persona.package %q is invalid", persona.Package)
    }
    if _, err := parseVersion(persona.Version); err != nil {
        return fmt.Errorf("persona.version: %w", err)
    }
    if persona.SHA256 != "" && !digestPattern.MatchString(persona.SHA256) {
        return fmt.Errorf("persona.sha256 is invalid")
    }
    return nil
}

func validateState(state State) error {
    if !contains([]string{"external", "available", "installed"}, state.Install) {
        return fmt.Errorf("state.install %q is invalid", state.Install)
    }
    if !contains([]string{"unverified", "verified", "failed"}, state.Verification) {
        return fmt.Errorf("state.verification %q is invalid", state.Verification)
    }
    if !contains([]string{"active", "deprecated", "blocked"}, state.Lifecycle) {
        return fmt.Errorf("state.lifecycle %q is invalid", state.Lifecycle)
    }
    if state.Verification == "failed" && state.Lifecycle == "active" {
        return fmt.Errorf("a failed verification state cannot be active")
    }
    return nil
}

func validateProvenance(provenance Provenance) error {
    if strings.TrimSpace(provenance.Publisher) == "" || len(provenance.Publisher) > 200 {
        return fmt.Errorf("provenance.publisher must contain 1 to 200 characters")
    }
    if provenance.SourceURL != "" {
        u, err := url.Parse(provenance.SourceURL)
        if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
            return fmt.Errorf("provenance.source_url must be an absolute HTTP(S) URL")
        }
    }
    if len(provenance.LicenseRef) > 160 {
        return fmt.Errorf("provenance.license_ref is too long")
    }
    if provenance.RetrievedAt != "" {
        if _, err := time.Parse(time.RFC3339, provenance.RetrievedAt); err != nil {
            return fmt.Errorf("provenance.retrieved_at must use RFC3339")
        }
    }
    return nil
}

func contains(values []string, value string) bool {
    for _, candidate := range values {
        if candidate == value {
            return true
        }
    }
    return false
}

type semanticVersion struct {
    major      uint64
    minor      uint64
    patch      uint64
    prerelease []string
}

func parseVersion(value string) (semanticVersion, error) {
    if !versionPattern.MatchString(value) {
        return semanticVersion{}, fmt.Errorf("%q is not a supported semantic version", value)
    }
    parts := strings.SplitN(value, "-", 2)
    core := strings.Split(parts[0], ".")
    if len(core) != 3 {
        return semanticVersion{}, fmt.Errorf("%q is not a supported semantic version", value)
    }
    numbers := make([]uint64, 3)
    for i, part := range core {
        if len(part) > 1 && part[0] == '0' {
            return semanticVersion{}, fmt.Errorf("%q contains a leading-zero version component", value)
        }
        n, err := strconv.ParseUint(part, 10, 64)
        if err != nil {
            return semanticVersion{}, fmt.Errorf("%q contains an invalid version component", value)
        }
        numbers[i] = n
    }
    parsed := semanticVersion{major: numbers[0], minor: numbers[1], patch: numbers[2]}
    if len(parts) == 2 {
        parsed.prerelease = strings.Split(parts[1], ".")
        for _, identifier := range parsed.prerelease {
            if identifier == "" {
                return semanticVersion{}, fmt.Errorf("%q contains an empty prerelease identifier", value)
            }
            if isNumeric(identifier) && len(identifier) > 1 && identifier[0] == '0' {
                return semanticVersion{}, fmt.Errorf("%q contains a leading-zero prerelease number", value)
            }
        }
    }
    return parsed, nil
}

func compareVersion(left, right semanticVersion) int {
    for _, pair := range [][2]uint64{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
        if pair[0] < pair[1] {
            return -1
        }
        if pair[0] > pair[1] {
            return 1
        }
    }
    if len(left.prerelease) == 0 && len(right.prerelease) == 0 {
        return 0
    }
    if len(left.prerelease) == 0 {
        return 1
    }
    if len(right.prerelease) == 0 {
        return -1
    }
    limit := len(left.prerelease)
    if len(right.prerelease) < limit {
        limit = len(right.prerelease)
    }
    for i := 0; i < limit; i++ {
        l, rr := left.prerelease[i], right.prerelease[i]
        if l == rr {
            continue
        }
        ln, rn := isNumeric(l), isNumeric(rr)
        switch {
        case ln && rn:
            li, _ := strconv.ParseUint(l, 10, 64)
            ri, _ := strconv.ParseUint(rr, 10, 64)
            if li < ri {
                return -1
            }
            return 1
        case ln:
            return -1
        case rn:
            return 1
        case l < rr:
            return -1
        default:
            return 1
        }
    }
    if len(left.prerelease) < len(right.prerelease) {
        return -1
    }
    if len(left.prerelease) > len(right.prerelease) {
        return 1
    }
    return 0
}

func isNumeric(value string) bool {
    if value == "" {
        return false
    }
    for _, r := range value {
        if r < '0' || r > '9' {
            return false
        }
    }
    return true
}

func CapabilityNames(capabilities Capabilities) []string {
    values := make([]string, 0, 10)
    if capabilities.Text {
        values = append(values, "text")
    }
    if capabilities.Vision {
        values = append(values, "vision")
    }
    if capabilities.Audio {
        values = append(values, "audio")
    }
    if capabilities.Embeddings {
        values = append(values, "embeddings")
    }
    if capabilities.Reranking {
        values = append(values, "reranking")
    }
    if capabilities.Tools {
        values = append(values, "tools")
    }
    if capabilities.ToolStreaming {
        values = append(values, "tool-streaming")
    }
    if capabilities.Thinking {
        values = append(values, "thinking")
    }
    if capabilities.ReasoningControl {
        values = append(values, "reasoning-control")
    }
    if capabilities.StructuredOutput {
        values = append(values, "structured-output")
    }
    sort.Strings(values)
    return values
}
''')

# Extend manifest tests without replacing the existing coverage.
replace_once(
    "internal/modelmanifest/manifest_test.go",
    "func TestSemanticPrereleaseOrdering(t *testing.T) {",
    r'''func TestMultiBackendArtifactAndMoEMetadata(t *testing.T) {
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

func TestSemanticPrereleaseOrdering(t *testing.T) {''',
)

write("schema/model-manifest-v1alpha1.schema.json", r'''{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://starlight-unit.de/schemas/quantum-runtime/model-manifest-v1alpha1.schema.json",
  "title": "Quantum Runtime Model Manifest v1alpha1",
  "type": "object",
  "additionalProperties": false,
  "required": ["schema_version", "id", "display_name", "source", "backend", "artifacts", "model", "capabilities", "compatibility", "state", "provenance"],
  "properties": {
    "schema_version": {"const": "quantum.runtime/model-manifest/v1alpha1"},
    "id": {"type": "string", "pattern": "^[a-z0-9][a-z0-9._-]{0,127}$"},
    "display_name": {"type": "string", "minLength": 1, "maxLength": 160},
    "aliases": {"type": "array", "uniqueItems": true, "items": {"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9._:/-]{0,159}$"}},
    "source": {
      "type": "object", "additionalProperties": false, "required": ["provider", "reference", "revision"],
      "properties": {
        "provider": {"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$"},
        "reference": {"type": "string", "minLength": 1, "maxLength": 300},
        "revision": {"type": "string", "minLength": 1, "maxLength": 200}
      }
    },
    "backend": {
      "type": "object", "additionalProperties": false, "required": ["type"],
      "properties": {"type": {"enum": ["ollama-adapter", "llama.cpp", "mlx", "vllm", "gguf", "safetensors", "external"]}}
    },
    "artifacts": {
      "type": "array", "minItems": 1,
      "items": {
        "type": "object", "additionalProperties": false, "required": ["uri"],
        "properties": {
          "uri": {"type": "string", "minLength": 3},
          "sha256": {"type": "string", "pattern": "^sha256:[a-f0-9]{64}$"},
          "backend": {"enum": ["ollama-adapter", "llama.cpp", "mlx", "vllm", "gguf", "safetensors", "external"]},
          "format": {"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$"},
          "role": {"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$"}
        }
      }
    },
    "model": {
      "type": "object", "additionalProperties": false,
      "required": ["architecture", "parameter_class", "quantization", "context_window"],
      "properties": {
        "architecture": {"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$"},
        "architecture_class": {"enum": ["dense", "moe", "unknown"]},
        "parameter_class": {"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$"},
        "total_parameters_b": {"type": "number", "minimum": 0},
        "active_parameters_b": {"type": "number", "minimum": 0},
        "experts": {
          "type": "object", "additionalProperties": false,
          "properties": {
            "total": {"type": "integer", "minimum": 1},
            "active": {"type": "integer", "minimum": 1},
            "shared": {"type": "integer", "minimum": 0}
          }
        },
        "quantization": {"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$"},
        "context_window": {"type": "integer", "minimum": 1, "maximum": 10000000},
        "context_policy": {
          "type": "object", "additionalProperties": false,
          "properties": {
            "native_max": {"type": "integer", "minimum": 0},
            "backend_managed": {"type": "boolean"},
            "override_supported": {"type": "boolean"},
            "override_verified": {"type": "boolean"}
          }
        }
      }
    },
    "capabilities": {
      "type": "object", "additionalProperties": false,
      "required": ["text", "vision", "audio", "embeddings", "tools", "thinking"],
      "properties": {
        "text": {"type": "boolean"}, "vision": {"type": "boolean"}, "audio": {"type": "boolean"},
        "embeddings": {"type": "boolean"}, "reranking": {"type": "boolean"}, "tools": {"type": "boolean"},
        "tool_streaming": {"type": "boolean"}, "thinking": {"type": "boolean"},
        "reasoning_control": {"type": "boolean"}, "structured_output": {"type": "boolean"}
      },
      "anyOf": [
        {"properties": {"text": {"const": true}}}, {"properties": {"vision": {"const": true}}},
        {"properties": {"audio": {"const": true}}}, {"properties": {"embeddings": {"const": true}}},
        {"properties": {"reranking": {"const": true}}}, {"properties": {"tools": {"const": true}}},
        {"properties": {"thinking": {"const": true}}}, {"properties": {"reasoning_control": {"const": true}}},
        {"properties": {"structured_output": {"const": true}}}
      ]
    },
    "compatibility": {
      "type": "object", "additionalProperties": false, "required": ["min_runtime_version"],
      "properties": {
        "min_runtime_version": {"type": "string", "pattern": "^[0-9]+\\.[0-9]+\\.[0-9]+(?:-[0-9A-Za-z.-]+)?$"},
        "max_runtime_version": {"type": "string", "pattern": "^[0-9]+\\.[0-9]+\\.[0-9]+(?:-[0-9A-Za-z.-]+)?$"}
      }
    },
    "persona": {
      "type": "object", "additionalProperties": false, "required": ["package", "version"],
      "properties": {
        "package": {"type": "string", "pattern": "^[a-z0-9][a-z0-9._-]{0,127}$"},
        "version": {"type": "string", "pattern": "^[0-9]+\\.[0-9]+\\.[0-9]+(?:-[0-9A-Za-z.-]+)?$"},
        "sha256": {"type": "string", "pattern": "^sha256:[a-f0-9]{64}$"}
      }
    },
    "state": {
      "type": "object", "additionalProperties": false, "required": ["install", "verification", "lifecycle"],
      "properties": {
        "install": {"enum": ["external", "available", "installed"]},
        "verification": {"enum": ["unverified", "verified", "failed"]},
        "lifecycle": {"enum": ["active", "deprecated", "blocked"]}
      },
      "not": {"properties": {"verification": {"const": "failed"}, "lifecycle": {"const": "active"}}, "required": ["verification", "lifecycle"]}
    },
    "provenance": {
      "type": "object", "additionalProperties": false, "required": ["publisher"],
      "properties": {
        "publisher": {"type": "string", "minLength": 1, "maxLength": 200},
        "source_url": {"type": "string", "format": "uri"},
        "license_ref": {"type": "string", "maxLength": 160},
        "retrieved_at": {"type": "string", "format": "date-time"}
      }
    }
  },
  "allOf": [{
    "if": {"properties": {"state": {"properties": {"verification": {"const": "verified"}}, "required": ["verification"]}}},
    "then": {"properties": {"artifacts": {"items": {"required": ["uri", "sha256"]}}, "source": {"properties": {"revision": {"not": {"const": "unresolved"}}}}}}
  }]
}
''')

# Update built-in manifests to expose generic architecture/context metadata without inventing exact unsupported facts.
for path in [
    "internal/modelregistry/data/ember-coreui.json",
    "internal/modelregistry/data/quantum-tci-gemma4-e4b.json",
    "internal/modelregistry/data/generic-model.json",
]:
    data = json.loads(Path(path).read_text(encoding="utf-8"))
    model = data["model"]
    if model["architecture"] == "gemma4":
        model.setdefault("architecture_class", "unknown")
        model.setdefault("context_policy", {"backend_managed": True, "override_supported": True, "override_verified": False})
        data["capabilities"].setdefault("reasoning_control", bool(data["capabilities"].get("thinking")))
        data["capabilities"].setdefault("structured_output", False)
        if path.endswith("ember-coreui.json"):
            data["artifacts"][0]["backend"] = "ollama-adapter"
            data["artifacts"][0]["format"] = "ollama"
            data["artifacts"][0]["role"] = "inference"
    else:
        model.setdefault("architecture_class", "unknown")
    Path(path).write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

write("internal/modelregistry/data/gemma4-26b-a4b-reference.json", r'''{
  "schema_version": "quantum.runtime/model-manifest/v1alpha1",
  "id": "gemma4-26b-a4b-reference",
  "display_name": "Gemma 4 26B A4B MoE Reference Profile",
  "aliases": ["gemma4:26b-a4b-reference"],
  "source": {
    "provider": "profile",
    "reference": "gemma4/26b-a4b",
    "revision": "unresolved"
  },
  "backend": {"type": "external"},
  "artifacts": [{"uri": "profile://gemma4/26b-a4b", "role": "inference"}],
  "model": {
    "architecture": "gemma4",
    "architecture_class": "moe",
    "parameter_class": "26b-a4b",
    "total_parameters_b": 26,
    "active_parameters_b": 4,
    "quantization": "profile_dependent",
    "context_window": 8192,
    "context_policy": {
      "backend_managed": true,
      "override_supported": true,
      "override_verified": false
    }
  },
  "capabilities": {
    "text": true,
    "vision": true,
    "audio": false,
    "embeddings": false,
    "tools": true,
    "thinking": true,
    "reasoning_control": true,
    "structured_output": true
  },
  "compatibility": {"min_runtime_version": "0.3.0-alpha.1"},
  "state": {"install": "available", "verification": "unverified", "lifecycle": "active"},
  "provenance": {"publisher": "Starlight Unit Studios"}
}
''')

# Ollama adapter now reports deterministic backend-contract capabilities.
replace_once(
    "internal/ollama/proxy.go",
    '"time"\n)',
    '"time"\n\n\t"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/backendcontract"\n)',
)
insert_marker = "func (p *Proxy) Ready(ctx context.Context) error {"
descriptor_code = r'''func (p *Proxy) Descriptor() backendcontract.Descriptor {
    version := "development"
    if p != nil && strings.TrimSpace(p.version) != "" {
        version = p.version
    }
    return backendcontract.Descriptor{
        ContractVersion: backendcontract.ContractVersion,
        ID:              "ollama",
        Kind:            "ollama-adapter",
        AdapterVersion:  version,
        ExecutionMode:   "external",
        State:           "unknown",
        Capabilities: backendcontract.Capabilities{
            Text: backendcontract.SupportConditional,
            Architecture: backendcontract.ArchitectureCapabilities{
                Dense: backendcontract.SupportConditional,
                MoE:   backendcontract.SupportConditional,
            },
            MoE: backendcontract.MoECapabilities{
                ExpertOffload:  backendcontract.SupportConditional,
                ExpertParallel: backendcontract.SupportUnknown,
            },
            Speculative: backendcontract.SpeculativeCapabilities{
                MTP:        backendcontract.SupportConditional,
                DraftModel: backendcontract.SupportConditional,
            },
            Cache: backendcontract.CacheCapabilities{
                KVOffload:   backendcontract.SupportConditional,
                PromptCache: backendcontract.SupportConditional,
            },
            Multimodal: backendcontract.MultimodalCapabilities{
                Vision: backendcontract.SupportConditional,
                Audio:  backendcontract.SupportConditional,
            },
            Embeddings:       backendcontract.SupportConditional,
            Reranking:        backendcontract.SupportUnknown,
            ReasoningControl: backendcontract.SupportConditional,
            Tools: backendcontract.ToolCapabilities{
                Calling:   backendcontract.SupportConditional,
                Streaming: backendcontract.SupportUnknown,
            },
            StructuredOutput: backendcontract.SupportConditional,
            Streaming: backendcontract.StreamingCapabilities{
                Content:       backendcontract.SupportSupported,
                Reasoning:     backendcontract.SupportConditional,
                ToolArguments: backendcontract.SupportUnknown,
            },
            Placement: backendcontract.PlacementCapabilities{
                CPU:    backendcontract.SupportSupported,
                GPU:    backendcontract.SupportConditional,
                Hybrid: backendcontract.SupportConditional,
            },
            Context: backendcontract.ContextCapabilities{
                BackendManaged:    true,
                OverrideSupported: backendcontract.SupportConditional,
                OverrideVerified:  false,
            },
        },
    }
}

'''
replace_once("internal/ollama/proxy.go", insert_marker, descriptor_code + insert_marker)

# Server: use backend contract, route planner, model policy and ledger endpoints while keeping compatibility forwarding intact.
replace_once(
    "internal/httpapi/server.go",
    '"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/config"\n\t"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/ollama"',
    '"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/backendcontract"\n\t"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/backendrouter"\n\t"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/config"\n\t"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/modelpolicy"\n\t"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/ollama"\n\t"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/upstreamledger"',
)
replace_once(
    "internal/httpapi/server.go",
    '''type Upstream interface {
\tDo(context.Context, *http.Request) (*http.Response, error)
\tReady(context.Context) error
}''',
    '''type Upstream interface {
\tbackendcontract.Backend
}''',
)
replace_once(
    "internal/httpapi/server.go",
    '''\tmux.HandleFunc("/v1/runtime", s.handleRuntime)
\tmux.HandleFunc("/v1/capabilities", s.handleCapabilities)
\tmux.HandleFunc("/api/", s.handleCompatibility)''',
    '''\tmux.HandleFunc("/v1/runtime", s.handleRuntime)
\tmux.HandleFunc("/v1/capabilities", s.handleCapabilities)
\tmux.HandleFunc("/v1/backends", s.handleBackends)
\tmux.HandleFunc("/v1/route", s.handleRoute)
\tmux.HandleFunc("/v1/model-policies", s.handleModelPolicies)
\tmux.HandleFunc("/v1/upstreams", s.handleUpstreams)
\tmux.HandleFunc("/api/", s.handleCompatibility)''',
)
replace_once(
    "internal/httpapi/server.go",
    '''\ts.writeJSON(w, http.StatusOK, map[string]any{
\t\t"status":  "ready",
\t\t"backend": "ollama-adapter",
\t})''',
    '''\tdescriptor := s.upstream.Descriptor()
\ts.writeJSON(w, http.StatusOK, map[string]any{
\t\t"status":  "ready",
\t\t"backend": descriptor.Kind,
\t})''',
)
replace_once(
    "internal/httpapi/server.go",
    '''\ts.writeJSON(w, http.StatusOK, map[string]any{
\t\t"name":        "Quantum Runtime",
\t\t"service":     "quantum-runtime",
\t\t"version":     s.build.Version,
\t\t"commit":      s.build.Commit,
\t\t"build_date":  s.build.BuildDate,
\t\t"api_version": "v1alpha1",
\t\t"backend": map[string]any{
\t\t\t"type": "ollama-adapter",
\t\t},
\t})''',
    '''\tbackend := any(nil)
\tif s.upstream != nil {
\t\tdescriptor := s.upstream.Descriptor()
\t\tbackend = descriptor
\t}
\ts.writeJSON(w, http.StatusOK, map[string]any{
\t\t"name":             "Quantum Runtime",
\t\t"service":          "quantum-runtime",
\t\t"version":          s.build.Version,
\t\t"commit":           s.build.Commit,
\t\t"build_date":       s.build.BuildDate,
\t\t"api_version":      "v1alpha1",
\t\t"backend_contract": backendcontract.ContractVersion,
\t\t"backend":          backend,
\t})''',
)
replace_once(
    "internal/httpapi/server.go",
    '''\ts.writeJSON(w, http.StatusOK, map[string]any{
\t\t"native_api": "v1alpha1",
\t\t"compatibility": []string{
\t\t\t"ollama-api-chat",
\t\t\t"ollama-api-generate",
\t\t\t"ollama-api-embeddings",
\t\t\t"ollama-api-model-read",
\t\t},
\t\t"backend":        "ollama-adapter",
\t\t"model_mutation": s.config.AllowModelMutation,
\t\t"streaming":      true,
\t})''',
    '''\tbackend := any(nil)
\tif s.upstream != nil {
\t\tbackend = s.upstream.Descriptor()
\t}
\ts.writeJSON(w, http.StatusOK, map[string]any{
\t\t"native_api":       "v1alpha1",
\t\t"backend_contract": backendcontract.ContractVersion,
\t\t"compatibility": []string{
\t\t\t"ollama-api-chat",
\t\t\t"ollama-api-generate",
\t\t\t"ollama-api-embeddings",
\t\t\t"ollama-api-model-read",
\t\t},
\t\t"backend":        backend,
\t\t"model_mutation": s.config.AllowModelMutation,
\t\t"streaming":      true,
\t})''',
)
# Insert new handlers before compatibility handler.
new_handlers = r'''func (s *Server) handleBackends(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        s.writeMethodNotAllowed(w, r, http.MethodGet)
        return
    }
    if !s.authorized(r) {
        s.writeUnauthorized(w)
        return
    }
    backends := []backendcontract.Descriptor{}
    if s.upstream != nil {
        descriptor := s.upstream.Descriptor()
        if err := descriptor.Validate(); err != nil {
            s.logger.Error("invalid backend descriptor", "error", err)
            s.writeError(w, http.StatusInternalServerError, "invalid_backend_descriptor", "The configured backend descriptor is invalid.")
            return
        }
        backends = append(backends, descriptor)
    }
    s.writeJSON(w, http.StatusOK, map[string]any{
        "api_version":      "v1alpha1",
        "contract_version": backendcontract.ContractVersion,
        "count":            len(backends),
        "backends":         backends,
    })
}

func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        s.writeMethodNotAllowed(w, r, http.MethodPost)
        return
    }
    if !s.authorized(r) {
        s.writeUnauthorized(w)
        return
    }
    if r.ContentLength > s.config.RequestBodyLimit {
        s.writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "The request body exceeds the configured limit.")
        return
    }
    r.Body = http.MaxBytesReader(w, r.Body, s.config.RequestBodyLimit)
    decoder := json.NewDecoder(r.Body)
    decoder.DisallowUnknownFields()
    var request struct {
        Model        string   `json:"model"`
        Capabilities []string `json:"capabilities,omitempty"`
    }
    if err := decoder.Decode(&request); err != nil {
        s.writeError(w, http.StatusBadRequest, "invalid_route_request", "The route request must be valid JSON with model and optional capabilities.")
        return
    }
    if strings.TrimSpace(request.Model) == "" {
        s.writeError(w, http.StatusBadRequest, "invalid_route_request", "model is required.")
        return
    }
    descriptors := []backendcontract.Descriptor{}
    if s.upstream != nil {
        descriptors = append(descriptors, s.upstream.Descriptor())
    }
    router, err := backendrouter.New(builtinModelRegistry, descriptors, modelpolicy.MustBuiltin())
    if err != nil {
        s.logger.Error("construct backend router", "error", err)
        s.writeError(w, http.StatusInternalServerError, "router_unavailable", "The backend router is not available.")
        return
    }
    plan, err := router.Route(request.Model, backendrouter.Requirements{Capabilities: request.Capabilities})
    if err != nil {
        if errors.Is(err, backendrouter.ErrModelNotFound) {
            s.writeError(w, http.StatusNotFound, "model_not_found", "The requested model is not present in the Quantum Runtime registry.")
            return
        }
        if errors.Is(err, backendrouter.ErrNoCompatibleBackend) {
            s.writeError(w, http.StatusUnprocessableEntity, "no_compatible_backend", "No configured backend satisfies the canonical model and requested capabilities.")
            return
        }
        s.writeError(w, http.StatusInternalServerError, "route_failed", "Backend routing failed.")
        return
    }
    s.writeJSON(w, http.StatusOK, map[string]any{"api_version": "v1alpha1", "plan": plan})
}

func (s *Server) handleModelPolicies(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        s.writeMethodNotAllowed(w, r, http.MethodGet)
        return
    }
    if !s.authorized(r) {
        s.writeUnauthorized(w)
        return
    }
    policies := modelpolicy.MustBuiltin()
    s.writeJSON(w, http.StatusOK, map[string]any{
        "api_version":     "v1alpha1",
        "schema_version":  modelpolicy.SchemaVersion,
        "count":           len(policies),
        "policies":        policies,
    })
}

func (s *Server) handleUpstreams(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        s.writeMethodNotAllowed(w, r, http.MethodGet)
        return
    }
    if !s.authorized(r) {
        s.writeUnauthorized(w)
        return
    }
    ledger := upstreamledger.MustBuiltin()
    s.writeJSON(w, http.StatusOK, map[string]any{
        "api_version":        "v1alpha1",
        "schema_version":     ledger.SchemaVersion,
        "entries":            ledger.Entries,
        "latest_known_good":  ledger.LatestKnownGood(),
    })
}

'''
replace_once("internal/httpapi/server.go", "func (s *Server) handleCompatibility(w http.ResponseWriter, r *http.Request) {", new_handlers + "func (s *Server) handleCompatibility(w http.ResponseWriter, r *http.Request) {")

# fake upstream now satisfies backend contract.
replace_once(
    "internal/httpapi/server_test.go",
    "func (f *fakeUpstream) Ready(context.Context) error {",
    r'''func (f *fakeUpstream) Descriptor() backendcontract.Descriptor {
    return backendcontract.Descriptor{
        ContractVersion: backendcontract.ContractVersion,
        ID:              "test-backend",
        Kind:            "external",
        AdapterVersion:  "test",
        ExecutionMode:   "external",
        State:           "unknown",
        Capabilities: backendcontract.Capabilities{
            Text: backendcontract.SupportSupported,
            Architecture: backendcontract.ArchitectureCapabilities{Dense: backendcontract.SupportConditional, MoE: backendcontract.SupportConditional},
            MoE: backendcontract.MoECapabilities{ExpertOffload: backendcontract.SupportUnknown, ExpertParallel: backendcontract.SupportUnknown},
            Speculative: backendcontract.SpeculativeCapabilities{MTP: backendcontract.SupportUnknown, DraftModel: backendcontract.SupportUnknown},
            Cache: backendcontract.CacheCapabilities{KVOffload: backendcontract.SupportUnknown, PromptCache: backendcontract.SupportUnknown},
            Multimodal: backendcontract.MultimodalCapabilities{Vision: backendcontract.SupportUnknown, Audio: backendcontract.SupportUnknown},
            Embeddings:       backendcontract.SupportUnknown,
            Reranking:        backendcontract.SupportUnknown,
            ReasoningControl: backendcontract.SupportUnknown,
            Tools: backendcontract.ToolCapabilities{Calling: backendcontract.SupportUnknown, Streaming: backendcontract.SupportUnknown},
            StructuredOutput: backendcontract.SupportUnknown,
            Streaming: backendcontract.StreamingCapabilities{Content: backendcontract.SupportSupported, Reasoning: backendcontract.SupportUnknown, ToolArguments: backendcontract.SupportUnknown},
            Placement: backendcontract.PlacementCapabilities{CPU: backendcontract.SupportConditional, GPU: backendcontract.SupportUnknown, Hybrid: backendcontract.SupportUnknown},
            Context: backendcontract.ContextCapabilities{BackendManaged: true, OverrideSupported: backendcontract.SupportUnknown},
        },
    }
}

func (f *fakeUpstream) Ready(context.Context) error {''',
)
replace_once(
    "internal/httpapi/server_test.go",
    '"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/config"',
    '"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/backendcontract"\n\t"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/config"',
)
# Add endpoint tests before testConfig.
replace_once(
    "internal/httpapi/server_test.go",
    "func testConfig(t *testing.T) config.Config {",
    r'''func TestBackendContractEndpoint(t *testing.T) {
    upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/api/version" {
            _, _ = io.WriteString(w, `{"version":"test"}`)
            return
        }
        _, _ = io.WriteString(w, `{}`)
    }))
    defer upstreamServer.Close()
    base, _ := url.Parse(upstreamServer.URL)
    server := New(testConfig(t), ollama.NewProxyWithClient(base, "test", upstreamServer.Client()), testBuild(), discardLogger())
    request := httptest.NewRequest(http.MethodGet, "/v1/backends", nil)
    response := httptest.NewRecorder()
    server.Handler().ServeHTTP(response, request)
    if response.Code != http.StatusOK {
        t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
    }
    if !strings.Contains(response.Body.String(), backendcontract.ContractVersion) || !strings.Contains(response.Body.String(), "ollama-adapter") {
        t.Fatalf("backend contract missing: %s", response.Body.String())
    }
}

func TestRouteEndpointPreservesCanonicalModelIdentity(t *testing.T) {
    upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{}`) }))
    defer upstreamServer.Close()
    base, _ := url.Parse(upstreamServer.URL)
    server := New(testConfig(t), ollama.NewProxyWithClient(base, "test", upstreamServer.Client()), testBuild(), discardLogger())
    request := httptest.NewRequest(http.MethodPost, "/v1/route", strings.NewReader(`{"model":"ember-coreui:latest","capabilities":["inference.text","multimodal.vision"]}`))
    response := httptest.NewRecorder()
    server.Handler().ServeHTTP(response, request)
    if response.Code != http.StatusOK {
        t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
    }
    if !strings.Contains(response.Body.String(), `"canonical_model_id":"ember-coreui"`) {
        t.Fatalf("canonical identity missing: %s", response.Body.String())
    }
}

func TestRouteEndpointFailsClosedOnUnknownCapability(t *testing.T) {
    upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{}`) }))
    defer upstreamServer.Close()
    base, _ := url.Parse(upstreamServer.URL)
    server := New(testConfig(t), ollama.NewProxyWithClient(base, "test", upstreamServer.Client()), testBuild(), discardLogger())
    request := httptest.NewRequest(http.MethodPost, "/v1/route", strings.NewReader(`{"model":"ember-coreui","capabilities":["future.magic"]}`))
    response := httptest.NewRecorder()
    server.Handler().ServeHTTP(response, request)
    if response.Code != http.StatusUnprocessableEntity {
        t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
    }
}

func TestPolicyAndUpstreamEndpoints(t *testing.T) {
    server := New(testConfig(t), &fakeUpstream{}, testBuild(), discardLogger())
    for _, path := range []string{"/v1/model-policies", "/v1/upstreams"} {
        request := httptest.NewRequest(http.MethodGet, path, nil)
        response := httptest.NewRecorder()
        server.Handler().ServeHTTP(response, request)
        if response.Code != http.StatusOK {
            t.Fatalf("%s unexpected status: %d body=%s", path, response.Code, response.Body.String())
        }
    }
}

func testConfig(t *testing.T) config.Config {''',
)

# Version and release docs.
Path("VERSION").write_text("0.3.0-alpha.1\n", encoding="utf-8")
replace_once("internal/buildinfo/buildinfo.go", 'Version   = "0.2.0-alpha.2"', 'Version   = "0.3.0-alpha.1"')
replace_once("README.md", 'Current version: `0.2.0-alpha.2`', 'Current version: `0.3.0-alpha.1`')
replace_once(
    "README.md",
    "Quantum Runtime now owns three reusable boundaries:\n",
    "Quantum Runtime 0.3 adds the first model- and engine-neutral backend capability foundation while retaining the non-destructive Ollama adoption path. It now owns these reusable boundaries:\n",
)
replace_once(
    "README.md",
    "- versioned `quantum.runtime/model-manifest/v1alpha1` contract and read-only registry\n",
    "- versioned `quantum.runtime/model-manifest/v1alpha1` contract and read-only registry\n- versioned `quantum.runtime/backend/v1alpha1` capability contract with explicit supported/unsupported/conditional/unknown states\n- deterministic capability router that preserves canonical model identity and fails closed on unsupported requirements\n- machine-readable model/backend policy for the validated Gemma 4 + Ollama minimal profile\n- machine-readable upstream ledger where unpinned observations cannot become latest-known-good production pins\n",
)
replace_once(
    "README.md",
    "curl -s http://127.0.0.1:11450/v1/models\n",
    "curl -s http://127.0.0.1:11450/v1/models\ncurl -s http://127.0.0.1:11450/v1/backends\ncurl -s http://127.0.0.1:11450/v1/model-policies\ncurl -s http://127.0.0.1:11450/v1/upstreams\n",
)
replace_once(
    "README.md",
    "The manifest separates canonical model identity from aliases, source revision, backend, artifacts, SHA-256 integrity, capabilities, compatibility, persona package, lifecycle state and provenance.",
    "The manifest separates canonical model identity from aliases, source revision, backend, artifacts, SHA-256 integrity, capabilities, compatibility, persona package, lifecycle state and provenance. In 0.3, artifacts may additionally declare their own backend/format/role, allowing one canonical identity to acquire multiple backend artifacts without changing the client-facing model ID. Generic architecture metadata now distinguishes dense, MoE and unknown profiles and can carry context-policy and expert-topology data without exposing family-specific fields in the public API.",
)

# Add a focused 0.3 section to the changelog.
changelog = Path("CHANGELOG.md")
text = changelog.read_text(encoding="utf-8")
marker = "# Changelog\n"
section = r'''

## 0.3.0-alpha.1 - 2026-09-05

- Added `quantum.runtime/backend/v1alpha1` with explicit supported, unsupported, conditional and unknown capability states instead of optimistic booleans.
- Added deterministic backend routing that resolves aliases to one canonical model identity, evaluates model + backend capabilities, selects an artifact/backend pair and fails closed when a requested capability is unknown or unsupported.
- Extended model-manifest v1alpha1 compatibly with artifact-specific backend/format/role metadata, dense/MoE architecture class, optional parameter/expert metadata, context-policy state, reranking/tool-streaming/reasoning-control/structured-output capability flags.
- Added a Gemma 4 26B A4B MoE reference profile without making Gemma the Runtime default.
- Added a machine-readable Gemma 4 + Ollama Turin minimal policy. `temperature=1.0`, `top_k=64` and `top_p=0.95` are known-good; context/predict/thread/repeat/seed/stop controls remain blocked-unverified by default. The policy explicitly does not claim `num_ctx` was the sole cause of the observed speedup.
- Added a machine-readable upstream ledger. The existing Ollama validation is intentionally `observed-unpinned` because the exact tested upstream version was not recorded, so it cannot silently become a latest-known-good production pin. llama.cpp/ggml is recorded as the first planned native portable backend.
- Added `/v1/backends`, `/v1/route`, `/v1/model-policies` and `/v1/upstreams` while preserving existing Ollama compatibility forwarding.
- Kept CPU-first placement visible in backend capabilities and did not introduce generic shell execution, TCI privilege access or application memory/state into the Runtime layer.
'''
if "## 0.3.0-alpha.1" not in text:
    if marker not in text:
        raise SystemExit("CHANGELOG marker missing")
    text = text.replace(marker, marker + section, 1)
changelog.write_text(text, encoding="utf-8")

# Docs are appended with additive 0.3 contracts to avoid rewriting established 0.2 behavior.
for path, heading, body in [
    ("docs/ARCHITECTURE.md", "## Runtime 0.3 backend capability layer", r'''
Quantum Runtime 0.3 introduces `quantum.runtime/backend/v1alpha1` between the canonical model registry and concrete execution engines. Backend capabilities use four explicit states: `supported`, `unsupported`, `conditional`, and `unknown`. `conditional` means the engine can provide the feature only when the selected model/artifact satisfies the corresponding requirement; `unknown` never satisfies a route request.

The first configured backend remains the external Ollama adoption adapter. The router is intentionally separate from transport forwarding so future llama.cpp, MLX and vLLM adapters can be added without changing canonical model identity or client APIs. A model may now carry artifact-specific backend metadata. Routing evaluates canonical model metadata, artifact compatibility, requested capabilities and backend state, then returns a deterministic plan. It never silently substitutes a different canonical model.

CPU execution is a first-class placement capability. GPU and hybrid placement remain optional/conditional and are not assumed to be the default. The 0.3.0-alpha.1 slice does not yet execute llama.cpp natively; it establishes the interface and routing boundary required for that next step.
'''),
    ("docs/API.md", "## Runtime 0.3 backend and routing endpoints", r'''
`GET /v1/backends` returns configured backend descriptors using `quantum.runtime/backend/v1alpha1`.

`POST /v1/route` accepts a canonical model ID or alias plus optional required capability names. Example:

```json
{"model":"ember-coreui:latest","capabilities":["inference.text","multimodal.vision"]}
```

The response contains `requested_identifier`, `canonical_model_id`, selected backend/artifact, required capability set, backend context policy and matching model-policy IDs. Unknown capabilities fail closed with `422 no_compatible_backend`; missing models return `404 model_not_found`.

`GET /v1/model-policies` exposes machine-readable backend/model validation policies. The initial Gemma 4 + Ollama Turin policy records the minimal known-good sampling profile and classifies additional tuning knobs as blocked-unverified until isolated A/B validation.

`GET /v1/upstreams` exposes the tested-upstream ledger plus the subset currently eligible as `latest_known_good`. An `observed-unpinned` entry is informative only and cannot be promoted automatically.
'''),
    ("docs/ROADMAP.md", "## 0.3 implementation status", r'''
`0.3.0-alpha.1` lands the backend interface, deterministic capability states, route planner, model/backend policy registry and upstream ledger. The Ollama adoption backend remains available and CoreUI/Game can continue using the existing compatibility endpoint.

Next 0.3 slices should implement the first native portable llama.cpp/ggml adapter, host hardware discovery/calibration, CPU-first/tiered placement policy, actual backend selection for multiple artifacts of one canonical model, and the conformance matrix from issue #8. MLX and vLLM follow after the portable local path is stable.
'''),
]:
    target = Path(path)
    doc = target.read_text(encoding="utf-8")
    if heading not in doc:
        target.write_text(doc.rstrip() + "\n\n" + heading + "\n\n" + body.strip() + "\n", encoding="utf-8")

print("Runtime 0.3 backend contract foundation applied")
