package modelpolicy

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
