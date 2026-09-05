package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/backendcontract"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/config"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/ollama"
)

func TestHealthEndpoint(t *testing.T) {
	server := New(testConfig(t), &fakeUpstream{}, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	if response.Header().Get("X-Quantum-Request-ID") == "" {
		t.Fatal("missing request ID")
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["service"] != "quantum-runtime" {
		t.Fatalf("unexpected service: %#v", payload["service"])
	}
}

func TestReadyEndpointReportsBackendFailure(t *testing.T) {
	server := New(testConfig(t), &fakeUpstream{readyErr: errors.New("offline")}, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status: %d", response.Code)
	}
}

func TestCompatibilityChatForwardsAndStreamsResponse(t *testing.T) {
	var upstreamAuthorization string
	var upstreamBody string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuthorization = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		upstreamBody = string(body)
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, `{"message":{"content":"one"},"done":false}`+"\n")
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = io.WriteString(w, `{"message":{"content":"two"},"done":true}`+"\n")
	}))
	defer upstreamServer.Close()

	base, err := url.Parse(upstreamServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t)
	cfg.AuthToken = "runtime-secret"
	proxy := ollama.NewProxyWithClient(base, "test", upstreamServer.Client())
	server := New(cfg, proxy, testBuild(), discardLogger())

	request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"model":"ember-coreui:latest","stream":true}`))
	request.Header.Set("Authorization", "Bearer runtime-secret")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
	}
	if upstreamAuthorization != "" {
		t.Fatalf("runtime bearer token leaked upstream: %q", upstreamAuthorization)
	}
	if upstreamBody != `{"model":"ember-coreui:latest","stream":true}` {
		t.Fatalf("unexpected upstream body: %q", upstreamBody)
	}
	if got := response.Body.String(); !strings.Contains(got, `"content":"one"`) || !strings.Contains(got, `"content":"two"`) {
		t.Fatalf("stream not forwarded: %q", got)
	}
}

func TestCoreUIChatPayloadRemainsOpaqueToRuntime(t *testing.T) {
	fixture, err := os.ReadFile("testdata/coreui-chat-request.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var observed []byte
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"message":{"content":"ok"},"done":true}`)
	}))
	defer upstreamServer.Close()

	base, err := url.Parse(upstreamServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := ollama.NewProxyWithClient(base, "test", upstreamServer.Client())
	server := New(testConfig(t), proxy, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(string(fixture)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
	}
	if string(observed) != string(fixture) {
		t.Fatalf("CoreUI payload changed in transit\nwant: %s\n got: %s", fixture, observed)
	}
}

func TestCompatibilityRequiresConfiguredBearerToken(t *testing.T) {
	cfg := testConfig(t)
	cfg.AuthToken = "runtime-secret"
	server := New(cfg, &fakeUpstream{}, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{}`))
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", response.Code)
	}
}

func TestModelMutationIsDisabledByDefault(t *testing.T) {
	server := New(testConfig(t), &fakeUpstream{}, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodPost, "/api/pull", strings.NewReader(`{"name":"model"}`))
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("unexpected status: %d", response.Code)
	}
}

func TestRequestBodyLimitRejectsKnownOversizeBody(t *testing.T) {
	cfg := testConfig(t)
	cfg.RequestBodyLimit = 1 << 20
	server := New(cfg, &fakeUpstream{}, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader("x"))
	request.ContentLength = cfg.RequestBodyLimit + 1
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("unexpected status: %d", response.Code)
	}
}

func TestUnsupportedCompatibilityRouteIsNotProxied(t *testing.T) {
	upstream := &fakeUpstream{}
	server := New(testConfig(t), upstream, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodPost, "/api/unknown", strings.NewReader(`{}`))
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	if upstream.calls != 0 {
		t.Fatalf("unsupported route reached upstream %d times", upstream.calls)
	}
}

func TestBackendContractEndpoint(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			_, _ = io.WriteString(w, `{"version":"test"}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstreamServer.Close()
	base, _ := url.Parse(upstreamServer.URL)
	server := New(testConfig(t), ollama.NewProxyWithClient(base, "test", upstreamServer.Client()), testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodGet, "/v1/backends", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), backendcontract.ContractVersion) || !strings.Contains(response.Body.String(), "ollama-adapter") {
		t.Fatalf("backend contract missing: %s", response.Body.String())
	}
}

func TestRouteEndpointPreservesCanonicalModelIdentity(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{}`) }))
	defer upstreamServer.Close()
	base, _ := url.Parse(upstreamServer.URL)
	server := New(testConfig(t), ollama.NewProxyWithClient(base, "test", upstreamServer.Client()), testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodPost, "/v1/route", strings.NewReader(`{"model":"ember-coreui:latest","capabilities":["inference.text","multimodal.vision"]}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"canonical_model_id":"ember-coreui"`) {
		t.Fatalf("canonical identity missing: %s", response.Body.String())
	}
}

func TestRouteEndpointFailsClosedOnUnknownCapability(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{}`) }))
	defer upstreamServer.Close()
	base, _ := url.Parse(upstreamServer.URL)
	server := New(testConfig(t), ollama.NewProxyWithClient(base, "test", upstreamServer.Client()), testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodPost, "/v1/route", strings.NewReader(`{"model":"ember-coreui","capabilities":["future.magic"]}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
	}
}

func TestPolicyAndUpstreamEndpoints(t *testing.T) {
	server := New(testConfig(t), &fakeUpstream{}, testBuild(), discardLogger())
	for _, path := range []string{"/v1/model-policies", "/v1/upstreams"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s unexpected status: %d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	upstream, err := url.Parse("http://127.0.0.1:11434")
	if err != nil {
		t.Fatal(err)
	}
	return config.Config{
		ListenAddress:      "127.0.0.1:11450",
		UpstreamURL:        upstream,
		UpstreamTimeout:    time.Minute,
		RequestBodyLimit:   128 << 20,
		AllowModelMutation: false,
	}
}

func testBuild() BuildInfo {
	return BuildInfo{Version: "test", Commit: "test", BuildDate: "test"}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeUpstream struct {
	readyErr error
	calls    int
}

func (f *fakeUpstream) Descriptor() backendcontract.Descriptor {
	return backendcontract.Descriptor{
		ContractVersion: backendcontract.ContractVersion,
		ID:              "test-backend",
		Kind:            "external",
		AdapterVersion:  "test",
		ExecutionMode:   "external",
		State:           "unknown",
		Capabilities: backendcontract.Capabilities{
			Text:             backendcontract.SupportSupported,
			Architecture:     backendcontract.ArchitectureCapabilities{Dense: backendcontract.SupportConditional, MoE: backendcontract.SupportConditional},
			MoE:              backendcontract.MoECapabilities{ExpertOffload: backendcontract.SupportUnknown, ExpertParallel: backendcontract.SupportUnknown},
			Speculative:      backendcontract.SpeculativeCapabilities{MTP: backendcontract.SupportUnknown, DraftModel: backendcontract.SupportUnknown},
			Cache:            backendcontract.CacheCapabilities{KVOffload: backendcontract.SupportUnknown, PromptCache: backendcontract.SupportUnknown},
			Multimodal:       backendcontract.MultimodalCapabilities{Vision: backendcontract.SupportUnknown, Audio: backendcontract.SupportUnknown},
			Embeddings:       backendcontract.SupportUnknown,
			Reranking:        backendcontract.SupportUnknown,
			ReasoningControl: backendcontract.SupportUnknown,
			Tools:            backendcontract.ToolCapabilities{Calling: backendcontract.SupportUnknown, Streaming: backendcontract.SupportUnknown},
			StructuredOutput: backendcontract.SupportUnknown,
			Streaming:        backendcontract.StreamingCapabilities{Content: backendcontract.SupportSupported, Reasoning: backendcontract.SupportUnknown, ToolArguments: backendcontract.SupportUnknown},
			Placement:        backendcontract.PlacementCapabilities{CPU: backendcontract.SupportConditional, GPU: backendcontract.SupportUnknown, Hybrid: backendcontract.SupportUnknown},
			Context:          backendcontract.ContextCapabilities{BackendManaged: true, OverrideSupported: backendcontract.SupportUnknown},
		},
	}
}

func (f *fakeUpstream) Ready(context.Context) error {
	return f.readyErr
}

func (f *fakeUpstream) Do(context.Context, *http.Request) (*http.Response, error) {
	f.calls++
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}, nil
}
