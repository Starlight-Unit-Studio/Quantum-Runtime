package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListenAddress    = "127.0.0.1:11450"
	defaultBackend          = "ollama"
	defaultUpstreamURL      = "http://127.0.0.1:11434"
	defaultLlamaCPPURL      = "http://127.0.0.1:8080"
	defaultUpstreamTimeout  = 15 * time.Minute
	defaultRequestBodyLimit = int64(128 << 20)
)

// Config contains the first runtime service configuration. The initial alpha
// uses an Ollama compatibility backend while preserving Quantum Runtime's own
// stable process and API boundary.
type Config struct {
	ListenAddress      string
	Backend            string
	UpstreamURL        *url.URL
	LlamaCPPURL        *url.URL
	LlamaCPPModel      string
	LlamaCPPAPIKey     string
	UpstreamTimeout    time.Duration
	RequestBodyLimit   int64
	AllowModelMutation bool
	AuthToken          string
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return LoadWith(os.Getenv)
}

// LoadWith exists to keep configuration parsing deterministic and unit-testable.
func LoadWith(getenv func(string) string) (Config, error) {
	if getenv == nil {
		return Config{}, errors.New("environment reader is nil")
	}

	listen := valueOrDefault(getenv("QUANTUM_RUNTIME_LISTEN"), defaultListenAddress)
	backend := strings.ToLower(valueOrDefault(getenv("QUANTUM_RUNTIME_BACKEND"), defaultBackend))
	upstreamRaw := valueOrDefault(getenv("QUANTUM_RUNTIME_OLLAMA_URL"), defaultUpstreamURL)
	upstream, err := url.Parse(upstreamRaw)
	if err != nil {
		return Config{}, fmt.Errorf("parse QUANTUM_RUNTIME_OLLAMA_URL: %w", err)
	}
	llamaRaw := valueOrDefault(getenv("QUANTUM_RUNTIME_LLAMA_CPP_URL"), defaultLlamaCPPURL)
	llamaURL, err := url.Parse(llamaRaw)
	if err != nil {
		return Config{}, fmt.Errorf("parse QUANTUM_RUNTIME_LLAMA_CPP_URL: %w", err)
	}

	upstreamTimeout, err := durationOrDefault(getenv("QUANTUM_RUNTIME_UPSTREAM_TIMEOUT"), defaultUpstreamTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("parse QUANTUM_RUNTIME_UPSTREAM_TIMEOUT: %w", err)
	}

	bodyLimit, err := int64OrDefault(getenv("QUANTUM_RUNTIME_MAX_REQUEST_BYTES"), defaultRequestBodyLimit)
	if err != nil {
		return Config{}, fmt.Errorf("parse QUANTUM_RUNTIME_MAX_REQUEST_BYTES: %w", err)
	}

	allowMutation, err := boolOrDefault(getenv("QUANTUM_RUNTIME_ALLOW_MODEL_MUTATION"), false)
	if err != nil {
		return Config{}, fmt.Errorf("parse QUANTUM_RUNTIME_ALLOW_MODEL_MUTATION: %w", err)
	}

	cfg := Config{
		ListenAddress:      listen,
		Backend:            backend,
		UpstreamURL:        upstream,
		LlamaCPPURL:        llamaURL,
		LlamaCPPModel:      strings.TrimSpace(getenv("QUANTUM_RUNTIME_LLAMA_CPP_MODEL")),
		LlamaCPPAPIKey:     strings.TrimSpace(getenv("QUANTUM_RUNTIME_LLAMA_CPP_API_KEY")),
		UpstreamTimeout:    upstreamTimeout,
		RequestBodyLimit:   bodyLimit,
		AllowModelMutation: allowMutation,
		AuthToken:          strings.TrimSpace(getenv("QUANTUM_RUNTIME_AUTH_TOKEN")),
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate rejects configurations that would accidentally expose an unauthenticated
// model service to a network.
func (c Config) Validate() error {
	if strings.TrimSpace(c.ListenAddress) == "" {
		return errors.New("listen address is empty")
	}
	if c.Backend != "ollama" && c.Backend != "llama.cpp" {
		return fmt.Errorf("QUANTUM_RUNTIME_BACKEND must be ollama or llama.cpp, got %q", c.Backend)
	}
	if err := validateHTTPURL("Ollama upstream", c.UpstreamURL); err != nil {
		return err
	}
	if err := validateHTTPURL("llama.cpp upstream", c.LlamaCPPURL); err != nil {
		return err
	}
	if c.Backend == "llama.cpp" && strings.TrimSpace(c.LlamaCPPModel) == "" {
		return errors.New("QUANTUM_RUNTIME_LLAMA_CPP_MODEL is required when QUANTUM_RUNTIME_BACKEND=llama.cpp")
	}
	if c.UpstreamTimeout <= 0 {
		return errors.New("upstream timeout must be positive")
	}
	if c.RequestBodyLimit < 1<<20 {
		return errors.New("request body limit must be at least 1 MiB")
	}
	if !isLoopbackListenAddress(c.ListenAddress) && strings.TrimSpace(c.AuthToken) == "" {
		return errors.New("non-loopback listen address requires QUANTUM_RUNTIME_AUTH_TOKEN")
	}
	return nil
}

func validateHTTPURL(name string, value *url.URL) error {
	if value == nil {
		return fmt.Errorf("%s URL is missing", name)
	}
	if value.Scheme != "http" && value.Scheme != "https" {
		return fmt.Errorf("%s URL must use http or https", name)
	}
	if value.Host == "" {
		return fmt.Errorf("%s URL host is missing", name)
	}
	if value.User != nil {
		return fmt.Errorf("%s URL must not contain credentials", name)
	}
	return nil
}

func isLoopbackListenAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func durationOrDefault(value string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func int64OrDefault(value string, fallback int64) (int64, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func boolOrDefault(value string, fallback bool) (bool, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, err
	}
	return parsed, nil
}
