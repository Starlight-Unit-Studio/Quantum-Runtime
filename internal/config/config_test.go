package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadWithDefaults(t *testing.T) {
	cfg, err := LoadWith(func(string) string { return "" })
	if err != nil {
		t.Fatalf("LoadWith defaults: %v", err)
	}
	if cfg.ListenAddress != "127.0.0.1:11450" {
		t.Fatalf("unexpected listen address: %q", cfg.ListenAddress)
	}
	if got := cfg.UpstreamURL.String(); got != "http://127.0.0.1:11434" {
		t.Fatalf("unexpected upstream URL: %q", got)
	}
	if cfg.UpstreamTimeout != 15*time.Minute {
		t.Fatalf("unexpected upstream timeout: %s", cfg.UpstreamTimeout)
	}
	if cfg.RequestBodyLimit != 128<<20 {
		t.Fatalf("unexpected request body limit: %d", cfg.RequestBodyLimit)
	}
	if cfg.AllowModelMutation {
		t.Fatal("model mutation must be disabled by default")
	}
}

func TestLoadWithRejectsUnauthenticatedNetworkBind(t *testing.T) {
	_, err := LoadWith(envMap(map[string]string{
		"QUANTUM_RUNTIME_LISTEN": "0.0.0.0:11450",
	}))
	if err == nil || !strings.Contains(err.Error(), "requires QUANTUM_RUNTIME_AUTH_TOKEN") {
		t.Fatalf("expected unauthenticated bind error, got %v", err)
	}
}

func TestLoadWithAllowsAuthenticatedNetworkBind(t *testing.T) {
	cfg, err := LoadWith(envMap(map[string]string{
		"QUANTUM_RUNTIME_LISTEN":     "0.0.0.0:11450",
		"QUANTUM_RUNTIME_AUTH_TOKEN": "test-secret",
	}))
	if err != nil {
		t.Fatalf("authenticated network bind: %v", err)
	}
	if cfg.AuthToken != "test-secret" {
		t.Fatalf("unexpected token value")
	}
}

func TestLoadWithRejectsInvalidUpstreamScheme(t *testing.T) {
	_, err := LoadWith(envMap(map[string]string{
		"QUANTUM_RUNTIME_OLLAMA_URL": "file:///tmp/ollama.sock",
	}))
	if err == nil || !strings.Contains(err.Error(), "http or https") {
		t.Fatalf("expected invalid scheme error, got %v", err)
	}
}

func TestLoadWithParsesExplicitPolicy(t *testing.T) {
	cfg, err := LoadWith(envMap(map[string]string{
		"QUANTUM_RUNTIME_UPSTREAM_TIMEOUT":     "45s",
		"QUANTUM_RUNTIME_MAX_REQUEST_BYTES":    "4194304",
		"QUANTUM_RUNTIME_ALLOW_MODEL_MUTATION": "true",
	}))
	if err != nil {
		t.Fatalf("explicit policy: %v", err)
	}
	if cfg.UpstreamTimeout != 45*time.Second {
		t.Fatalf("unexpected timeout: %s", cfg.UpstreamTimeout)
	}
	if cfg.RequestBodyLimit != 4<<20 {
		t.Fatalf("unexpected limit: %d", cfg.RequestBodyLimit)
	}
	if !cfg.AllowModelMutation {
		t.Fatal("expected model mutation to be enabled")
	}
}

func envMap(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
