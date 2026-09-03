package modelregistry

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/modelmanifest"
)

//go:embed data/*.json
var builtinFS embed.FS

type Registry struct {
	byID    map[string]modelmanifest.Manifest
	aliases map[string]string
	ordered []string
}

type Entry struct {
	ID           string              `json:"id"`
	DisplayName  string              `json:"display_name"`
	Aliases      []string            `json:"aliases,omitempty"`
	Backend      string              `json:"backend"`
	Capabilities []string            `json:"capabilities"`
	State        modelmanifest.State `json:"state"`
}

func New(manifests []modelmanifest.Manifest) (*Registry, error) {
	registry := &Registry{
		byID:    make(map[string]modelmanifest.Manifest, len(manifests)),
		aliases: make(map[string]string),
		ordered: make([]string, 0, len(manifests)),
	}
	for _, manifest := range manifests {
		if err := manifest.Validate(); err != nil {
			return nil, fmt.Errorf("manifest %q: %w", manifest.ID, err)
		}
		if _, exists := registry.byID[manifest.ID]; exists {
			return nil, fmt.Errorf("duplicate canonical model id %q", manifest.ID)
		}
		if canonical, exists := registry.aliases[manifest.ID]; exists {
			return nil, fmt.Errorf("canonical model id %q conflicts with alias owned by %q", manifest.ID, canonical)
		}
		registry.byID[manifest.ID] = manifest
		registry.ordered = append(registry.ordered, manifest.ID)
		for _, alias := range manifest.Aliases {
			if _, exists := registry.byID[alias]; exists {
				return nil, fmt.Errorf("alias %q conflicts with a canonical model id", alias)
			}
			if canonical, exists := registry.aliases[alias]; exists {
				return nil, fmt.Errorf("alias %q is already owned by %q", alias, canonical)
			}
			registry.aliases[alias] = manifest.ID
		}
	}
	sort.Strings(registry.ordered)
	return registry, nil
}

func Builtin() (*Registry, error) {
	entries, err := fs.ReadDir(builtinFS, "data")
	if err != nil {
		return nil, fmt.Errorf("read builtin model manifests: %w", err)
	}
	manifests := make([]modelmanifest.Manifest, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := builtinFS.ReadFile("data/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read builtin manifest %s: %w", entry.Name(), err)
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		var manifest modelmanifest.Manifest
		if err := decoder.Decode(&manifest); err != nil {
			return nil, fmt.Errorf("decode builtin manifest %s: %w", entry.Name(), err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			if err == nil {
				return nil, fmt.Errorf("builtin manifest %s contains trailing JSON values", entry.Name())
			}
			return nil, fmt.Errorf("decode trailing data in builtin manifest %s: %w", entry.Name(), err)
		}
		manifests = append(manifests, manifest)
	}
	if len(manifests) == 0 {
		return nil, fmt.Errorf("no builtin model manifests were found")
	}
	return New(manifests)
}

func MustBuiltin() *Registry {
	registry, err := Builtin()
	if err != nil {
		panic(fmt.Sprintf("invalid builtin Quantum Runtime model registry: %v", err))
	}
	return registry
}

func (r *Registry) List() []Entry {
	entries := make([]Entry, 0, len(r.ordered))
	for _, id := range r.ordered {
		manifest := r.byID[id]
		entries = append(entries, Entry{
			ID:           manifest.ID,
			DisplayName:  manifest.DisplayName,
			Aliases:      append([]string(nil), manifest.Aliases...),
			Backend:      manifest.Backend.Type,
			Capabilities: modelmanifest.CapabilityNames(manifest.Capabilities),
			State:        manifest.State,
		})
	}
	return entries
}

func (r *Registry) Lookup(identifier string) (modelmanifest.Manifest, bool) {
	if manifest, ok := r.byID[identifier]; ok {
		return manifest, true
	}
	canonical, ok := r.aliases[identifier]
	if !ok {
		return modelmanifest.Manifest{}, false
	}
	manifest, ok := r.byID[canonical]
	return manifest, ok
}

func (r *Registry) Len() int {
	return len(r.ordered)
}
