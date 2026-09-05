package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/resourcecontrol"
)

func TestHostProfileEndpoint(t *testing.T) {
	runtimeResources = resourcecontrol.New()
	server := New(testConfig(t), &fakeUpstream{}, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodGet, "/v1/host", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["host_schema"] != "quantum.runtime/host-profile/v1alpha1" {
		t.Fatalf("unexpected host schema: %#v", payload["host_schema"])
	}
}

func TestHostCalibrationEndpoint(t *testing.T) {
	runtimeResources = resourcecontrol.New()
	server := New(testConfig(t), &fakeUpstream{}, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodPost, "/v1/host/calibrate", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "memory_bandwidth_class") || !strings.Contains(response.Body.String(), "best_workers") {
		t.Fatalf("calibration missing: %s", response.Body.String())
	}
}

func TestPlacementEndpointPrefersCPUFirst(t *testing.T) {
	runtimeResources = resourcecontrol.New()
	server := New(testConfig(t), &fakeUpstream{}, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodPost, "/v1/placement", strings.NewReader(`{"model_bytes":1048576,"allow_acceleration":true}`))
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"cpu_first":true`) || !strings.Contains(response.Body.String(), `"decision":"cpu_only"`) {
		t.Fatalf("CPU-first placement missing: %s", response.Body.String())
	}
}

func TestPlacementEndpointRejectsUnknownFields(t *testing.T) {
	runtimeResources = resourcecontrol.New()
	server := New(testConfig(t), &fakeUpstream{}, testBuild(), discardLogger())
	request := httptest.NewRequest(http.MethodPost, "/v1/placement", strings.NewReader(`{"model_bytes":1048576,"magic_gpu":true}`))
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
	}
}
