package installer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	ServiceName       = "quantum-runtime.service"
	RuntimeUser       = "quantum-runtime"
	RuntimeGroup      = "quantum-runtime"
	DefaultListen     = "127.0.0.1:11450"
	DefaultRuntimeURL = "http://127.0.0.1:11450"
	DefaultOllamaURL  = "http://127.0.0.1:11434"
)

type Ownership string

const (
	Managed  Ownership = "managed"
	External Ownership = "external"
	Disabled Ownership = "disabled"
)

type Layout struct {
	Root      string
	Binary    string
	ConfigDir string
	Config    string
	Unit      string
	Marker    string
}

func NewLayout(root string) Layout {
	join := func(path string) string {
		if root == "" || root == "/" {
			return path
		}
		return filepath.Join(root, strings.TrimPrefix(path, "/"))
	}
	return Layout{
		Root:      root,
		Binary:    join("/usr/local/bin/quantum-runtime"),
		ConfigDir: join("/etc/quantum-runtime"),
		Config:    join("/etc/quantum-runtime/quantum-runtime.env"),
		Unit:      join("/etc/systemd/system/quantum-runtime.service"),
		Marker:    join("/etc/quantum-runtime/.managed.json"),
	}
}

type Runner interface {
	Run(context.Context, string, ...string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			return "", err
		}
		return text, fmt.Errorf("%s %v: %w: %s", name, args, err, text)
	}
	return text, nil
}

type Prober interface {
	Probe(context.Context, string) error
}

type HTTPProber struct {
	Client *http.Client
}

func (p HTTPProber) Probe(ctx context.Context, rawURL string) error {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("probe %s returned HTTP %d", rawURL, response.StatusCode)
	}
	return nil
}

type Manager struct {
	Layout      Layout
	Runner      Runner
	Prober      Prober
	Version     string
	RuntimeURL  string
	OllamaURL   string
	Now         func() time.Time
	RequireRoot bool
}

type ComponentStatus struct {
	ID        string    `json:"id"`
	Ownership Ownership `json:"ownership"`
	Available bool      `json:"available"`
	Detail    string    `json:"detail,omitempty"`
}

type Status struct {
	Runtime ComponentStatus `json:"runtime"`
	Ollama  ComponentStatus `json:"ollama"`
	Health  bool            `json:"health"`
	Ready   bool            `json:"ready"`
}

type Preflight struct {
	SystemdAvailable bool            `json:"systemd_available"`
	Ollama           ComponentStatus `json:"ollama"`
	Runtime          ComponentStatus `json:"runtime"`
	CanInstall       bool            `json:"can_install"`
	Warnings         []string        `json:"warnings,omitempty"`
}

type InstallOptions struct {
	RuntimeBinary string
	StartService  bool
}

type marker struct {
	Version      string   `json:"version"`
	ManagedFiles []string `json:"managed_files"`
	ConfigOwned  bool     `json:"config_owned"`
	InstalledAt  string   `json:"installed_at"`
}

type snapshot struct {
	binary       []byte
	binaryMode   os.FileMode
	binaryExists bool
	config       []byte
	configMode   os.FileMode
	configExists bool
	unit         []byte
	unitMode     os.FileMode
	unitExists   bool
	marker       []byte
	markerMode   os.FileMode
	markerExists bool
}

func (m *Manager) normalize() {
	if m.Runner == nil {
		m.Runner = ExecRunner{}
	}
	if m.Prober == nil {
		m.Prober = HTTPProber{}
	}
	if m.RuntimeURL == "" {
		m.RuntimeURL = DefaultRuntimeURL
	}
	if m.OllamaURL == "" {
		m.OllamaURL = DefaultOllamaURL
	}
	if m.Now == nil {
		m.Now = time.Now
	}
}

func (m *Manager) Preflight(ctx context.Context) (Preflight, error) {
	m.normalize()
	runtime := m.runtimeOwnership()
	ollama := m.ollamaStatus(ctx)
	_, systemdErr := m.Runner.Run(ctx, "systemctl", "--version")
	result := Preflight{
		SystemdAvailable: systemdErr == nil,
		Ollama:           ollama,
		Runtime:          runtime,
	}
	if systemdErr != nil {
		result.Warnings = append(result.Warnings, "systemctl is not available")
	}
	if ollama.Ownership == Disabled {
		result.Warnings = append(result.Warnings, "Ollama adoption backend is not reachable on the configured local endpoint")
	}
	if runtime.Ownership == External {
		result.Warnings = append(result.Warnings, "existing Quantum Runtime files are not owned by this installer")
	}
	result.CanInstall = result.SystemdAvailable && ollama.Available && runtime.Ownership != External
	return result, nil
}

func (m *Manager) Status(ctx context.Context) Status {
	m.normalize()
	status := Status{
		Runtime: m.runtimeOwnership(),
		Ollama:  m.ollamaStatus(ctx),
	}
	status.Health = m.Prober.Probe(ctx, strings.TrimRight(m.RuntimeURL, "/")+"/healthz") == nil
	status.Ready = m.Prober.Probe(ctx, strings.TrimRight(m.RuntimeURL, "/")+"/readyz") == nil
	return status
}

func (m *Manager) Install(ctx context.Context, options InstallOptions) error {
	m.normalize()
	if err := m.requireRoot(); err != nil {
		return err
	}
	if strings.TrimSpace(options.RuntimeBinary) == "" {
		return errors.New("runtime binary path is required")
	}
	sourceInfo, err := os.Stat(options.RuntimeBinary)
	if err != nil {
		return fmt.Errorf("inspect runtime binary: %w", err)
	}
	if !sourceInfo.Mode().IsRegular() {
		return errors.New("runtime binary must be a regular file")
	}

	preflight, err := m.Preflight(ctx)
	if err != nil {
		return err
	}
	if !preflight.SystemdAvailable {
		return errors.New("systemd is required for the current Linux package profile")
	}
	if !preflight.Ollama.Available {
		return errors.New("existing Ollama adoption backend is not ready; installation made no changes")
	}
	if preflight.Runtime.Ownership == External {
		return errors.New("refusing to overwrite externally owned Quantum Runtime files")
	}

	old, err := captureSnapshot(m.Layout)
	if err != nil {
		return fmt.Errorf("capture rollback snapshot: %w", err)
	}
	configExisted := old.configExists
	configOwned := !configExisted
	if old.markerExists {
		previous, markerErr := readMarker(m.Layout.Marker)
		if markerErr != nil {
			return markerErr
		}
		configOwned = previous.ConfigOwned
	}
	rollback := func(cause error) error {
		restoreErr := restoreSnapshot(m.Layout, old)
		_, _ = m.Runner.Run(context.Background(), "systemctl", "daemon-reload")
		if old.markerExists {
			_, _ = m.Runner.Run(context.Background(), "systemctl", "restart", ServiceName)
		} else {
			_, _ = m.Runner.Run(context.Background(), "systemctl", "disable", "--now", ServiceName)
		}
		if restoreErr != nil {
			return fmt.Errorf("%v; rollback failed: %w", cause, restoreErr)
		}
		return cause
	}

	if err := m.ensureIdentity(ctx); err != nil {
		return rollback(err)
	}
	if err := os.MkdirAll(filepath.Dir(m.Layout.Binary), 0755); err != nil {
		return rollback(fmt.Errorf("create binary directory: %w", err))
	}
	if err := os.MkdirAll(m.Layout.ConfigDir, 0750); err != nil {
		return rollback(fmt.Errorf("create config directory: %w", err))
	}
	if err := copyAtomic(options.RuntimeBinary, m.Layout.Binary, 0755); err != nil {
		return rollback(fmt.Errorf("install runtime binary: %w", err))
	}
	if !configExisted {
		if err := writeAtomic(m.Layout.Config, []byte(defaultEnvironment()), 0640); err != nil {
			return rollback(fmt.Errorf("create Runtime environment file: %w", err))
		}
	}
	if err := writeAtomic(m.Layout.Unit, []byte(SystemdUnit()), 0644); err != nil {
		return rollback(fmt.Errorf("install systemd unit: %w", err))
	}
	managed := marker{
		Version:      m.Version,
		ManagedFiles: []string{m.Layout.Binary, m.Layout.Unit},
		ConfigOwned:  configOwned,
		InstalledAt:  m.Now().UTC().Format(time.RFC3339),
	}
	markerData, err := json.MarshalIndent(managed, "", "  ")
	if err != nil {
		return rollback(fmt.Errorf("encode ownership marker: %w", err))
	}
	markerData = append(markerData, '\n')
	if err := writeAtomic(m.Layout.Marker, markerData, 0600); err != nil {
		return rollback(fmt.Errorf("write ownership marker: %w", err))
	}
	if _, err := m.Runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return rollback(fmt.Errorf("reload systemd: %w", err))
	}
	if options.StartService {
		if _, err := m.Runner.Run(ctx, "systemctl", "enable", "--now", ServiceName); err != nil {
			return rollback(fmt.Errorf("enable Quantum Runtime service: %w", err))
		}
		if err := m.waitReady(ctx); err != nil {
			return rollback(fmt.Errorf("Quantum Runtime activation failed: %w", err))
		}
	}
	return nil
}

func (m *Manager) Repair(ctx context.Context, options InstallOptions) error {
	m.normalize()
	if m.runtimeOwnership().Ownership != Managed {
		return errors.New("repair is only available for installer-managed Quantum Runtime instances")
	}
	return m.Install(ctx, options)
}

func (m *Manager) Uninstall(ctx context.Context, purgeConfig bool) error {
	m.normalize()
	if err := m.requireRoot(); err != nil {
		return err
	}
	state := m.runtimeOwnership()
	if state.Ownership == Disabled {
		return nil
	}
	if state.Ownership == External {
		return errors.New("refusing to uninstall externally owned Quantum Runtime files")
	}

	managed, err := readMarker(m.Layout.Marker)
	if err != nil {
		return err
	}
	_, _ = m.Runner.Run(ctx, "systemctl", "disable", "--now", ServiceName)
	for _, path := range managed.ManagedFiles {
		if path != m.Layout.Binary && path != m.Layout.Unit {
			return fmt.Errorf("ownership marker contains unexpected managed path %q", path)
		}
		if err := removeIfExists(path); err != nil {
			return err
		}
	}
	if purgeConfig && managed.ConfigOwned {
		if err := removeIfExists(m.Layout.Config); err != nil {
			return err
		}
	}
	if err := removeIfExists(m.Layout.Marker); err != nil {
		return err
	}
	_, _ = m.Runner.Run(ctx, "systemctl", "daemon-reload")
	return nil
}

func CoreUIProfile(mode string) (string, error) {
	var endpoint string
	switch mode {
	case "runtime":
		endpoint = "http://127.0.0.1:11450/api/chat"
	case "ollama":
		endpoint = "http://127.0.0.1:11434/api/chat"
	default:
		return "", fmt.Errorf("unsupported CoreUI profile mode %q", mode)
	}
	return fmt.Sprintf("COREUI_OLLAMA_URL=%s\nSTU_EMBER_OLLAMA_URL=%s\n", endpoint, endpoint), nil
}

func SystemdUnit() string {
	return `[Unit]
Description=Quantum Runtime local AI service
Documentation=https://github.com/Starlight-Unit-Studio/Quantum-Runtime
After=network-online.target ollama.service
Wants=network-online.target

[Service]
Type=simple
User=quantum-runtime
Group=quantum-runtime
EnvironmentFile=/etc/quantum-runtime/quantum-runtime.env
ExecStart=/usr/local/bin/quantum-runtime
Restart=on-failure
RestartSec=3s
TimeoutStartSec=60s
TimeoutStopSec=30s
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
ProtectClock=true
RestrictSUIDSGID=true
LockPersonality=true
RestrictRealtime=true
CapabilityBoundingSet=
AmbientCapabilities=
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
SystemCallArchitectures=native

[Install]
WantedBy=multi-user.target
`
}

func defaultEnvironment() string {
	return `# Quantum Runtime managed defaults. Existing files are preserved during updates.
QUANTUM_RUNTIME_LISTEN=127.0.0.1:11450
QUANTUM_RUNTIME_OLLAMA_URL=http://127.0.0.1:11434
QUANTUM_RUNTIME_UPSTREAM_TIMEOUT=15m
QUANTUM_RUNTIME_MAX_REQUEST_BYTES=134217728
QUANTUM_RUNTIME_ALLOW_MODEL_MUTATION=false
`
}

func (m *Manager) runtimeOwnership() ComponentStatus {
	markerExists := fileExists(m.Layout.Marker)
	binaryExists := fileExists(m.Layout.Binary)
	unitExists := fileExists(m.Layout.Unit)
	switch {
	case markerExists:
		return ComponentStatus{ID: "quantum-runtime", Ownership: Managed, Available: binaryExists && unitExists, Detail: ServiceName}
	case binaryExists || unitExists:
		return ComponentStatus{ID: "quantum-runtime", Ownership: External, Available: true, Detail: "existing files are not owned by the Quantum Runtime installer"}
	default:
		return ComponentStatus{ID: "quantum-runtime", Ownership: Disabled, Available: false}
	}
}

func (m *Manager) ollamaStatus(ctx context.Context) ComponentStatus {
	endpoint := strings.TrimRight(m.OllamaURL, "/") + "/api/version"
	err := m.Prober.Probe(ctx, endpoint)
	if err != nil {
		return ComponentStatus{ID: "ollama", Ownership: Disabled, Available: false, Detail: endpoint}
	}
	return ComponentStatus{ID: "ollama", Ownership: External, Available: true, Detail: endpoint}
}

func (m *Manager) ensureIdentity(ctx context.Context) error {
	if _, err := m.Runner.Run(ctx, "getent", "group", RuntimeGroup); err != nil {
		if _, createErr := m.Runner.Run(ctx, "groupadd", "--system", RuntimeGroup); createErr != nil {
			return fmt.Errorf("create system group: %w", createErr)
		}
	}
	if _, err := m.Runner.Run(ctx, "id", "-u", RuntimeUser); err != nil {
		if _, createErr := m.Runner.Run(ctx, "useradd", "--system", "--gid", RuntimeGroup, "--home-dir", "/var/lib/quantum-runtime", "--shell", "/usr/sbin/nologin", RuntimeUser); createErr != nil {
			return fmt.Errorf("create system user: %w", createErr)
		}
	}
	return nil
}

func (m *Manager) waitReady(ctx context.Context) error {
	healthURL := strings.TrimRight(m.RuntimeURL, "/") + "/healthz"
	readyURL := strings.TrimRight(m.RuntimeURL, "/") + "/readyz"
	var last error
	for attempt := 0; attempt < 20; attempt++ {
		if err := m.Prober.Probe(ctx, healthURL); err != nil {
			last = err
		} else if err := m.Prober.Probe(ctx, readyURL); err != nil {
			last = err
		} else {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	if last == nil {
		last = errors.New("runtime did not become ready")
	}
	return last
}

func (m *Manager) requireRoot() error {
	if !m.RequireRoot {
		return nil
	}
	if os.Geteuid() != 0 {
		return errors.New("this operation must run as root")
	}
	return nil
}

func readMarker(path string) (marker, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return marker{}, fmt.Errorf("read ownership marker: %w", err)
	}
	var value marker
	if err := json.Unmarshal(data, &value); err != nil {
		return marker{}, fmt.Errorf("decode ownership marker: %w", err)
	}
	return value, nil
}

func captureSnapshot(layout Layout) (snapshot, error) {
	var result snapshot
	var err error
	result.binary, result.binaryMode, result.binaryExists, err = readOptional(layout.Binary)
	if err != nil {
		return result, err
	}
	result.config, result.configMode, result.configExists, err = readOptional(layout.Config)
	if err != nil {
		return result, err
	}
	result.unit, result.unitMode, result.unitExists, err = readOptional(layout.Unit)
	if err != nil {
		return result, err
	}
	result.marker, result.markerMode, result.markerExists, err = readOptional(layout.Marker)
	return result, err
}

func restoreSnapshot(layout Layout, state snapshot) error {
	for _, entry := range []struct {
		path   string
		data   []byte
		mode   os.FileMode
		exists bool
	}{
		{layout.Binary, state.binary, state.binaryMode, state.binaryExists},
		{layout.Config, state.config, state.configMode, state.configExists},
		{layout.Unit, state.unit, state.unitMode, state.unitExists},
		{layout.Marker, state.marker, state.markerMode, state.markerExists},
	} {
		if entry.exists {
			if err := writeAtomic(entry.path, entry.data, entry.mode); err != nil {
				return err
			}
		} else if err := removeIfExists(entry.path); err != nil {
			return err
		}
	}
	return nil
}

func readOptional(path string) ([]byte, os.FileMode, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, false, err
	}
	return data, info.Mode().Perm(), true, nil
}

func copyAtomic(source, destination string, mode os.FileMode) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return writeAtomic(destination, data, mode)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".qrtmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
