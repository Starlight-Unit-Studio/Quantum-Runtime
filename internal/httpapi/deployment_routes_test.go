package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeploymentProfilesEndpoint(t *testing.T) {
	server := New(testConfig(t), &fakeUpstream{}, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodGet, "/v1/deployment-profiles", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "ember-production") {
		t.Fatalf("E.M.B.E.R. profile missing: %s", response.Body.String())
	}
}

func TestAdmissionUnknownModelFailsClosed(t *testing.T) {
	server := New(testConfig(t), &fakeUpstream{}, testBuild(), discardLogger())
	body := `{"profile":"ember-production","model":"does-not-exist"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/admission", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestBenchmarkPlanPreservesCanonicalModelIdentity(t *testing.T) {
	server := New(testConfig(t), &fakeUpstream{}, testBuild(), discardLogger())
	body := `{"model":"gemma4:26b-a4b-reference","minimum_workers":1}`
	request := httptest.NewRequest(http.MethodPost, "/v1/benchmark-plan", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"canonical_model_id":"gemma4-26b-a4b-reference"`) {
		t.Fatalf("canonical identity missing: %s", response.Body.String())
	}
}
