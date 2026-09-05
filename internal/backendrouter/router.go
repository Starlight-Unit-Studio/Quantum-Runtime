package backendrouter

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
	RequestedIdentifier  string                              `json:"requested_identifier"`
	CanonicalModelID     string                              `json:"canonical_model_id"`
	BackendID            string                              `json:"backend_id"`
	BackendKind          string                              `json:"backend_kind"`
	ArtifactURI          string                              `json:"artifact_uri"`
	RequiredCapabilities []string                            `json:"required_capabilities,omitempty"`
	Context              backendcontract.ContextCapabilities `json:"context"`
	ModelPolicyIDs       []string                            `json:"model_policy_ids,omitempty"`
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
