package modelmanifest

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
	URI    string `json:"uri"`
	SHA256 string `json:"sha256,omitempty"`
}

type Model struct {
	Architecture   string `json:"architecture"`
	ParameterClass string `json:"parameter_class"`
	Quantization   string `json:"quantization"`
	ContextWindow  int    `json:"context_window"`
}

type Capabilities struct {
	Text       bool `json:"text"`
	Vision     bool `json:"vision"`
	Audio      bool `json:"audio"`
	Embeddings bool `json:"embeddings"`
	Tools      bool `json:"tools"`
	Thinking   bool `json:"thinking"`
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
	if !m.Capabilities.Text && !m.Capabilities.Vision && !m.Capabilities.Audio && !m.Capabilities.Embeddings && !m.Capabilities.Tools && !m.Capabilities.Thinking {
		return fmt.Errorf("at least one capability must be declared")
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
	case "ollama-adapter", "gguf", "safetensors", "external":
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
		if _, exists := seen[artifact.URI]; exists {
			return fmt.Errorf("artifact uri %q is duplicated", artifact.URI)
		}
		seen[artifact.URI] = struct{}{}
		if artifact.SHA256 != "" && !digestPattern.MatchString(artifact.SHA256) {
			return fmt.Errorf("artifact %q has an invalid sha256 digest", artifact.URI)
		}
		if verification == "verified" && artifact.SHA256 == "" {
			return fmt.Errorf("verified manifests require a sha256 digest for artifact %q", artifact.URI)
		}
	}
	return nil
}

func validateModel(model Model) error {
	if !labelPattern.MatchString(model.Architecture) {
		return fmt.Errorf("model.architecture %q is invalid", model.Architecture)
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
		l, r := left.prerelease[i], right.prerelease[i]
		if l == r {
			continue
		}
		ln, rn := isNumeric(l), isNumeric(r)
		switch {
		case ln && rn:
			li, _ := strconv.ParseUint(l, 10, 64)
			ri, _ := strconv.ParseUint(r, 10, 64)
			if li < ri {
				return -1
			}
			return 1
		case ln:
			return -1
		case rn:
			return 1
		case l < r:
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
	values := make([]string, 0, 6)
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
	if capabilities.Tools {
		values = append(values, "tools")
	}
	if capabilities.Thinking {
		values = append(values, "thinking")
	}
	sort.Strings(values)
	return values
}
