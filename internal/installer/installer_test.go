package installer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	commands [][]string
	fail     map[string]error
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	command := append([]string{name}, args...)
	r.commands = append(r.commands, command)
	key := strings.Join(command, " ")
	if err := r.fail[key]; err != nil {
		return "", err
	}
	switch key {
	case "systemctl --version":
		return "systemd 256", nil
	case "getent group quantum-runtime":
		return "quantum-runtime:x:999:", nil
	case "id -u quantum-runtime":
		return "999", nil
	default:
		return "", nil
	}
}

type fakeProber struct {
	fail map[string]error
	seen []string
}

func (p *fakeProber) Probe(_ context.Context, url string) error {
	p.seen = append(p.seen, url)
	if err := p.fail[url]; err != nil {
		return err
	}
	return nil
}

func testManager(t *testing.T) (*Manager, *fakeRunner, *fakeProber) {
	t.Helper()
	runner := &fakeRunner{fail: map[string]error{}}
	prober := &fakeProber{fail: map[string]error{}}
	manager := &Manager{
		Layout:      NewLayout(t.TempDir()),
		Runner:      runner,
		Prober:      prober,
		Version:     "0.2.0-alpha.2",
		Now:         func() time.Time { return time.Date(2026, 9, 3, 20, 0, 0, 0, time.UTC) },
		RequireRoot: false,
	}
	return manager, runner, prober
}

func writeRuntimeBinary(t *testing.T, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, "quantum-runtime-source")
	if err := os.WriteFile(path, []byte(contents), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPreflightDoesNotMutateFilesystem(t *testing.T) {
	manager, runner, _ := testManager(t)
	root := manager.Layout.Root
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Preflight(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanInstall || result.Ollama.Ownership != External || result.Runtime.Ownership != Disabled {
		t.Fatalf("unexpected preflight: %#v", result)
	}
	after, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("preflight mutated filesystem: before=%v after=%v", before, after)
	}
	for _, command := range runner.commands {
		if command[0] != "systemctl" || len(command) != 2 || command[1] != "--version" {
			t.Fatalf("preflight executed mutating/unexpected command: %v", command)
		}
	}
}

func TestInstallAndUninstallAreIdempotentAndPreserveConfig(t *testing.T) {
	manager, _, _ := testManager(t)
	source := writeRuntimeBinary(t, t.TempDir(), "runtime-v2")

	if err := manager.Install(context.Background(), InstallOptions{RuntimeBinary: source, StartService: true}); err != nil {
		t.Fatalf("first install: %v", err)
	}
	firstConfig, err := os.ReadFile(manager.Layout.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.Layout.Config, append(firstConfig, []byte("# local override\n")...), 0640); err != nil {
		t.Fatal(err)
	}

	if err := manager.Install(context.Background(), InstallOptions{RuntimeBinary: source, StartService: true}); err != nil {
		t.Fatalf("second install: %v", err)
	}
	configAfterUpdate, err := os.ReadFile(manager.Layout.Config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configAfterUpdate), "# local override") {
		t.Fatal("update overwrote local Runtime configuration")
	}

	if err := manager.Uninstall(context.Background(), false); err != nil {
		t.Fatalf("first uninstall: %v", err)
	}
	if err := manager.Uninstall(context.Background(), false); err != nil {
		t.Fatalf("second uninstall: %v", err)
	}
	if fileExists(manager.Layout.Binary) || fileExists(manager.Layout.Unit) || fileExists(manager.Layout.Marker) {
		t.Fatal("managed Runtime files remained after uninstall")
	}
	if !fileExists(manager.Layout.Config) {
		t.Fatal("normal uninstall removed local Runtime configuration")
	}
}

func TestInstallRefusesExternallyOwnedRuntime(t *testing.T) {
	manager, _, _ := testManager(t)
	if err := os.MkdirAll(filepath.Dir(manager.Layout.Binary), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.Layout.Binary, []byte("external"), 0755); err != nil {
		t.Fatal(err)
	}
	source := writeRuntimeBinary(t, t.TempDir(), "managed")
	if err := manager.Install(context.Background(), InstallOptions{RuntimeBinary: source, StartService: true}); err == nil || !strings.Contains(err.Error(), "externally owned") {
		t.Fatalf("expected external ownership refusal, got %v", err)
	}
	data, _ := os.ReadFile(manager.Layout.Binary)
	if string(data) != "external" {
		t.Fatal("external Runtime binary was modified")
	}
}

func TestInstallRollsBackPreviousManagedFilesWhenActivationFails(t *testing.T) {
	manager, _, prober := testManager(t)
	oldSource := writeRuntimeBinary(t, t.TempDir(), "old-runtime")
	if err := manager.Install(context.Background(), InstallOptions{RuntimeBinary: oldSource, StartService: true}); err != nil {
		t.Fatalf("seed install: %v", err)
	}
	oldUnit, err := os.ReadFile(manager.Layout.Unit)
	if err != nil {
		t.Fatal(err)
	}
	oldMarker, err := os.ReadFile(manager.Layout.Marker)
	if err != nil {
		t.Fatal(err)
	}

	newSource := writeRuntimeBinary(t, t.TempDir(), "new-runtime")
	prober.fail[DefaultRuntimeURL+"/healthz"] = errors.New("activation failed")
	if err := manager.Install(context.Background(), InstallOptions{RuntimeBinary: newSource, StartService: true}); err == nil {
		t.Fatal("expected activation failure")
	}

	binary, _ := os.ReadFile(manager.Layout.Binary)
	unit, _ := os.ReadFile(manager.Layout.Unit)
	markerData, _ := os.ReadFile(manager.Layout.Marker)
	if string(binary) != "old-runtime" {
		t.Fatalf("rollback did not restore old binary: %q", string(binary))
	}
	if !reflect.DeepEqual(unit, oldUnit) {
		t.Fatal("rollback did not restore old unit")
	}
	if !reflect.DeepEqual(markerData, oldMarker) {
		t.Fatal("rollback did not restore old ownership marker")
	}
}

func TestInstallerNeverMutatesOllama(t *testing.T) {
	manager, runner, _ := testManager(t)
	source := writeRuntimeBinary(t, t.TempDir(), "runtime")
	if err := manager.Install(context.Background(), InstallOptions{RuntimeBinary: source, StartService: true}); err != nil {
		t.Fatal(err)
	}
	for _, command := range runner.commands {
		joined := strings.Join(command, " ")
		if strings.Contains(joined, "ollama") {
			t.Fatalf("installer issued a command targeting Ollama: %v", command)
		}
	}
}

func TestCoreUIProfilesSwitchWithoutRebuild(t *testing.T) {
	runtimeProfile, err := CoreUIProfile("runtime")
	if err != nil {
		t.Fatal(err)
	}
	directProfile, err := CoreUIProfile("ollama")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"COREUI_OLLAMA_URL", "STU_EMBER_OLLAMA_URL"} {
		if !strings.Contains(runtimeProfile, key+"=http://127.0.0.1:11450/api/chat") {
			t.Fatalf("runtime profile missing %s", key)
		}
		if !strings.Contains(directProfile, key+"=http://127.0.0.1:11434/api/chat") {
			t.Fatalf("direct profile missing %s", key)
		}
	}
}

func TestUninstallNeverPurgesPreexistingConfig(t *testing.T) {
	manager, _, _ := testManager(t)
	if err := os.MkdirAll(manager.Layout.ConfigDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.Layout.Config, []byte("custom=true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := writeRuntimeBinary(t, t.TempDir(), "runtime")
	if err := manager.Install(context.Background(), InstallOptions{RuntimeBinary: source, StartService: true}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Uninstall(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(manager.Layout.Config)
	if err != nil {
		t.Fatalf("preexisting config was removed: %v", err)
	}
	if string(data) != "custom=true\n" {
		t.Fatalf("preexisting config changed: %q", string(data))
	}
}

func TestFailedFirstInstallRollsBackCreatedConfig(t *testing.T) {
	manager, _, prober := testManager(t)
	source := writeRuntimeBinary(t, t.TempDir(), "runtime")
	prober.fail[DefaultRuntimeURL+"/readyz"] = errors.New("not ready")
	if err := manager.Install(context.Background(), InstallOptions{RuntimeBinary: source, StartService: true}); err == nil {
		t.Fatal("expected activation failure")
	}
	for _, path := range []string{manager.Layout.Binary, manager.Layout.Config, manager.Layout.Unit, manager.Layout.Marker} {
		if fileExists(path) {
			t.Fatalf("rollback left created path %s", path)
		}
	}
}

func TestInstallerOwnedConfigRemainsOwnedAcrossUpdate(t *testing.T) {
	manager, _, _ := testManager(t)
	source := writeRuntimeBinary(t, t.TempDir(), "runtime")
	if err := manager.Install(context.Background(), InstallOptions{RuntimeBinary: source, StartService: true}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Install(context.Background(), InstallOptions{RuntimeBinary: source, StartService: true}); err != nil {
		t.Fatal(err)
	}
	managed, err := readMarker(manager.Layout.Marker)
	if err != nil {
		t.Fatal(err)
	}
	if !managed.ConfigOwned {
		t.Fatal("installer lost ownership knowledge for its own config during update")
	}
	if err := manager.Uninstall(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if fileExists(manager.Layout.Config) {
		t.Fatal("purge-managed-config did not remove installer-owned config")
	}
}

func TestSystemdUnitMatchesDeploymentFile(t *testing.T) {
	data, err := os.ReadFile("../../deploy/systemd/quantum-runtime.service")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != SystemdUnit() {
		t.Fatal("embedded installer unit and deploy/systemd unit drifted apart")
	}
}
