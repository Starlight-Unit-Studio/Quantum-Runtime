package deploymentprofile

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
)

const SchemaVersion = "quantum.runtime/deployment-profile/v1alpha1"

//go:embed data/*.json
var builtinFS embed.FS

type Profile struct {
	SchemaVersion string               `json:"schema_version"`
	ID            string               `json:"id"`
	DisplayName   string               `json:"display_name"`
	Tier          string               `json:"tier"`
	Description   string               `json:"description"`
	Hardware      HardwareRequirements `json:"hardware"`
	Model         ModelRequirements    `json:"model"`
}

type HardwareRequirements struct {
	Memory      MemoryRequirements      `json:"memory"`
	CPU         CPURequirements         `json:"cpu"`
	Accelerator AcceleratorRequirements `json:"accelerator"`
}

type MemoryRequirements struct {
	MinimumBytes   uint64 `json:"minimum_bytes"`
	ECCRequired    bool   `json:"ecc_required"`
	PreferredClass string `json:"preferred_class,omitempty"`
}

type CPURequirements struct {
	MinimumPhysicalCores int `json:"minimum_physical_cores"`
	ReferenceMinClockMHz int `json:"reference_min_clock_mhz,omitempty"`
}

type AcceleratorRequirements struct {
	Required bool `json:"required"`
}

type ModelRequirements struct {
	ArchitectureClass string `json:"architecture_class"`
}

type Registry struct {
	byID    map[string]Profile
	ordered []string
}

func (p Profile) Validate() error {
	if p.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version must be %q", SchemaVersion)
	}
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(p.DisplayName) == "" {
		return fmt.Errorf("display_name is required")
	}
	if p.Tier != "production" && p.Tier != "compatibility" && p.Tier != "experimental" {
		return fmt.Errorf("unsupported tier %q", p.Tier)
	}
	if p.Hardware.Memory.MinimumBytes == 0 {
		return fmt.Errorf("hardware.memory.minimum_bytes must be positive")
	}
	if p.Hardware.CPU.MinimumPhysicalCores < 1 {
		return fmt.Errorf("hardware.cpu.minimum_physical_cores must be positive")
	}
	if p.Hardware.CPU.ReferenceMinClockMHz < 0 {
		return fmt.Errorf("hardware.cpu.reference_min_clock_mhz must not be negative")
	}
	class := strings.ToLower(strings.TrimSpace(p.Model.ArchitectureClass))
	if class != "dense" && class != "moe" && class != "any" {
		return fmt.Errorf("unsupported model architecture_class %q", p.Model.ArchitectureClass)
	}
	return nil
}

func Builtin() (*Registry, error) {
	entries, err := fs.ReadDir(builtinFS, "data")
	if err != nil {
		return nil, fmt.Errorf("read builtin deployment profiles: %w", err)
	}
	registry := &Registry{byID: map[string]Profile{}}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := builtinFS.ReadFile("data/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read deployment profile %s: %w", entry.Name(), err)
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		var profile Profile
		if err := decoder.Decode(&profile); err != nil {
			return nil, fmt.Errorf("decode deployment profile %s: %w", entry.Name(), err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			if err == nil {
				return nil, fmt.Errorf("deployment profile %s contains trailing JSON values", entry.Name())
			}
			return nil, fmt.Errorf("decode trailing data in deployment profile %s: %w", entry.Name(), err)
		}
		if err := profile.Validate(); err != nil {
			return nil, fmt.Errorf("deployment profile %s: %w", entry.Name(), err)
		}
		if _, exists := registry.byID[profile.ID]; exists {
			return nil, fmt.Errorf("duplicate deployment profile %q", profile.ID)
		}
		registry.byID[profile.ID] = profile
		registry.ordered = append(registry.ordered, profile.ID)
	}
	if len(registry.byID) == 0 {
		return nil, fmt.Errorf("no builtin deployment profiles were found")
	}
	sort.Strings(registry.ordered)
	return registry, nil
}

func MustBuiltin() *Registry {
	registry, err := Builtin()
	if err != nil {
		panic(fmt.Sprintf("invalid builtin deployment profiles: %v", err))
	}
	return registry
}

func (r *Registry) Lookup(id string) (Profile, bool) {
	profile, ok := r.byID[strings.TrimSpace(id)]
	return profile, ok
}

func (r *Registry) List() []Profile {
	profiles := make([]Profile, 0, len(r.ordered))
	for _, id := range r.ordered {
		profiles = append(profiles, r.byID[id])
	}
	return profiles
}

func (r *Registry) Len() int {
	return len(r.ordered)
}
