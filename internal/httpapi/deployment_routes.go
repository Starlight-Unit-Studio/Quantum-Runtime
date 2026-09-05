package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/admission"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/benchmarkplan"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/deploymentprofile"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostlimits"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostprofile"
)

var builtinDeploymentProfiles = deploymentprofile.MustBuiltin()

func (s *Server) withDeploymentRoutes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/deployment-profiles":
			s.handleDeploymentProfiles(w, r)
			return
		case "/v1/admission":
			s.handleAdmission(w, r)
			return
		case "/v1/benchmark-plan":
			s.handleBenchmarkPlan(w, r)
			return
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func (s *Server) handleDeploymentProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	if !s.authorized(r) {
		s.writeUnauthorized(w)
		return
	}
	profiles := builtinDeploymentProfiles.List()
	s.writeJSON(w, http.StatusOK, map[string]any{
		"api_version":    "v1alpha1",
		"schema_version": deploymentprofile.SchemaVersion,
		"count":          len(profiles),
		"profiles":       profiles,
	})
}

func (s *Server) handleAdmission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	if !s.authorized(r) {
		s.writeUnauthorized(w)
		return
	}
	if r.ContentLength > s.config.RequestBodyLimit {
		s.writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "The request body exceeds the configured limit.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.config.RequestBodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request struct {
		Profile  string                     `json:"profile"`
		Model    string                     `json:"model"`
		Evidence admission.OperatorEvidence `json:"operator_evidence,omitempty"`
	}
	if err := decoder.Decode(&request); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_admission_request", "The admission request must be valid JSON using the v1alpha1 contract.")
		return
	}
	profile, ok := builtinDeploymentProfiles.Lookup(strings.TrimSpace(request.Profile))
	if !ok {
		s.writeError(w, http.StatusNotFound, "deployment_profile_not_found", "The requested deployment profile does not exist.")
		return
	}
	model, ok := builtinModelRegistry.Lookup(strings.TrimSpace(request.Model))
	if !ok {
		s.writeError(w, http.StatusNotFound, "model_not_found", "The requested model is not present in the Quantum Runtime registry.")
		return
	}
	host := hostprofile.Discover()
	limits := hostlimits.Discover()
	result, err := admission.Evaluate(profile, host, limits, model, request.Evidence)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "admission_failed", err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"api_version":      "v1alpha1",
		"admission_schema": admission.SchemaVersion,
		"host_schema":      hostprofile.SchemaVersion,
		"limits_schema":    hostlimits.SchemaVersion,
		"host":             host,
		"limits":           limits,
		"result":           result,
	})
}

func (s *Server) handleBenchmarkPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	if !s.authorized(r) {
		s.writeUnauthorized(w)
		return
	}
	if r.ContentLength > s.config.RequestBodyLimit {
		s.writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "The request body exceeds the configured limit.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.config.RequestBodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request struct {
		Model                     string `json:"model"`
		CoreBudget                int    `json:"core_budget,omitempty"`
		ReserveSystemCores        int    `json:"reserve_system_cores,omitempty"`
		MinimumWorkers            int    `json:"minimum_workers,omitempty"`
		IncludeFullHostComparison bool   `json:"include_full_host_comparison,omitempty"`
	}
	if err := decoder.Decode(&request); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_benchmark_plan_request", "The benchmark-plan request must be valid JSON using the v1alpha1 contract.")
		return
	}
	model, ok := builtinModelRegistry.Lookup(strings.TrimSpace(request.Model))
	if !ok {
		s.writeError(w, http.StatusNotFound, "model_not_found", "The requested model is not present in the Quantum Runtime registry.")
		return
	}
	limits := hostlimits.Discover()
	plan, err := benchmarkplan.Build(limits, benchmarkplan.Request{
		CoreBudget:                request.CoreBudget,
		ReserveSystemCores:        request.ReserveSystemCores,
		MinimumWorkers:            request.MinimumWorkers,
		IncludeFullHostComparison: request.IncludeFullHostComparison,
	})
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_benchmark_plan_request", err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"api_version":        "v1alpha1",
		"benchmark_schema":   benchmarkplan.SchemaVersion,
		"canonical_model_id": model.ID,
		"limits":             limits,
		"plan":               plan,
	})
}
