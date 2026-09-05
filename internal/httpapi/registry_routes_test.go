package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelListEndpoint(t *testing.T) {
	server := New(testConfig(t), &fakeUpstream{}, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		SchemaVersion string `json:"schema_version"`
		Count         int    `json:"count"`
		Models        []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.SchemaVersion != "quantum.runtime/model-manifest/v1alpha1" {
		t.Fatalf("unexpected schema version %q", payload.SchemaVersion)
	}
	if payload.Count != 4 || len(payload.Models) != 4 {
		t.Fatalf("unexpected registry size count=%d len=%d", payload.Count, len(payload.Models))
	}
}

func TestModelInspectResolvesAlias(t *testing.T) {
	server := New(testConfig(t), &fakeUpstream{}, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodGet, "/v1/models/ember-coreui:latest", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		RequestedIdentifier string `json:"requested_identifier"`
		Model               struct {
			ID string `json:"id"`
		} `json:"model"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.RequestedIdentifier != "ember-coreui:latest" || payload.Model.ID != "ember-coreui" {
		t.Fatalf("unexpected alias resolution: %#v", payload)
	}
}

func TestModelInspectUsesStableNotFoundError(t *testing.T) {
	server := New(testConfig(t), &fakeUpstream{}, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodGet, "/v1/models/does-not-exist", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	var payload struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error.Code != "model_not_found" {
		t.Fatalf("unexpected error code %q", payload.Error.Code)
	}
	if payload.Error.RequestID == "" {
		t.Fatal("registry error did not include request id")
	}
}

func TestModelRegistryHonorsRuntimeAuthentication(t *testing.T) {
	cfg := testConfig(t)
	cfg.AuthToken = "runtime-secret"
	server := New(cfg, &fakeUpstream{}, testBuild(), discardLogger())

	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized without token, got %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	request.Header.Set("Authorization", "Bearer runtime-secret")
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected authenticated request to succeed, got %d", response.Code)
	}
}
