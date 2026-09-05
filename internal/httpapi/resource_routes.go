package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/calibration"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostlimits"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostprofile"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/placement"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/resourcecontrol"
)

const hostCalibrationBudget = 400 * time.Millisecond

var runtimeResources = resourcecontrol.New()

func (s *Server) withResourceRoutes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/host":
			s.handleHostProfile(w, r)
			return
		case "/v1/host/calibrate":
			s.handleHostCalibration(w, r)
			return
		case "/v1/placement":
			s.handlePlacement(w, r)
			return
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func (s *Server) handleHostProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	if !s.authorized(r) {
		s.writeUnauthorized(w)
		return
	}
	profile := runtimeResources.Host()
	if err := profile.Validate(); err != nil {
		s.logger.Error("invalid host profile", "error", err)
		s.writeError(w, http.StatusInternalServerError, "invalid_host_profile", "The discovered host profile is invalid.")
		return
	}
	limits := hostlimits.Discover()
	if err := limits.Validate(); err != nil {
		s.logger.Error("invalid host limits", "error", err)
		s.writeError(w, http.StatusInternalServerError, "invalid_host_limits", "The discovered process/guest CPU limits are invalid.")
		return
	}
	last, calibrated := runtimeResources.LastCalibration()
	s.writeJSON(w, http.StatusOK, map[string]any{
		"api_version":        "v1alpha1",
		"host_schema":        hostprofile.SchemaVersion,
		"limits_schema":      hostlimits.SchemaVersion,
		"calibration_schema": calibration.SchemaVersion,
		"host":               profile,
		"limits":             limits,
		"calibrated":         calibrated,
		"last_calibration":   calibrationOrNil(last, calibrated),
	})
}

func (s *Server) handleHostCalibration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	if !s.authorized(r) {
		s.writeUnauthorized(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	host, result := runtimeResources.Calibrate(ctx, hostCalibrationBudget)
	if err := host.Validate(); err != nil {
		s.logger.Error("invalid host profile after calibration", "error", err)
		s.writeError(w, http.StatusInternalServerError, "invalid_host_profile", "The discovered host profile is invalid.")
		return
	}
	limits := hostlimits.Discover()
	s.writeJSON(w, http.StatusOK, map[string]any{
		"api_version": "v1alpha1",
		"host":        host,
		"limits":      limits,
		"calibration": result,
	})
}

func (s *Server) handlePlacement(w http.ResponseWriter, r *http.Request) {
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
	var request placement.Request
	if err := decoder.Decode(&request); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_placement_request", "The placement request must be valid JSON using the v1alpha1 placement contract.")
		return
	}
	host, last, plan, err := runtimeResources.Plan(request)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_placement_request", err.Error())
		return
	}
	if err := host.Validate(); err != nil {
		s.logger.Error("invalid host profile during placement", "error", err)
		s.writeError(w, http.StatusInternalServerError, "invalid_host_profile", "The discovered host profile is invalid.")
		return
	}
	limits := hostlimits.Discover()
	s.writeJSON(w, http.StatusOK, map[string]any{
		"api_version":      "v1alpha1",
		"placement_schema": placement.SchemaVersion,
		"host":             host,
		"limits":           limits,
		"calibration":      last,
		"plan":             plan,
	})
}

func calibrationOrNil(value calibration.Result, ok bool) any {
	if !ok {
		return nil
	}
	return value
}
