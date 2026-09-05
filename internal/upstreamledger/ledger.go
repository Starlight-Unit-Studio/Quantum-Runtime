package upstreamledger

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
	ID                   string   `json:"id"`
	Project              string   `json:"project"`
	BackendKind          string   `json:"backend_kind"`
	SourceURL            string   `json:"source_url"`
	TestedRef            string   `json:"tested_ref,omitempty"`
	Status               string   `json:"status"`
	ObservedAt           string   `json:"observed_at,omitempty"`
	EnabledCapabilities  []string `json:"enabled_capabilities,omitempty"`
	DisabledCapabilities []string `json:"disabled_capabilities,omitempty"`
	HardwareClasses      []string `json:"hardware_classes,omitempty"`
	LicenseRef           string   `json:"license_ref,omitempty"`
	Notes                string   `json:"notes,omitempty"`
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
