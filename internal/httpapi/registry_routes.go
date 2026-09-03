package httpapi

import (
	"net/http"
	"strings"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/modelmanifest"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/modelregistry"
)

var builtinModelRegistry = modelregistry.MustBuiltin()

func (s *Server) withRegistryRoutes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/models":
			s.handleModelList(w, r)
			return
		case strings.HasPrefix(r.URL.Path, "/v1/models/"):
			s.handleModelInspect(w, r)
			return
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func (s *Server) handleModelList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	if !s.authorized(r) {
		s.writeUnauthorized(w)
		return
	}
	models := builtinModelRegistry.List()
	s.writeJSON(w, http.StatusOK, map[string]any{
		"api_version":    "v1alpha1",
		"schema_version": modelmanifest.SchemaVersion,
		"count":          len(models),
		"models":         models,
	})
}

func (s *Server) handleModelInspect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	if !s.authorized(r) {
		s.writeUnauthorized(w)
		return
	}
	identifier := strings.TrimPrefix(r.URL.Path, "/v1/models/")
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		s.writeError(w, http.StatusNotFound, "model_not_found", "The requested model is not present in the Quantum Runtime registry.")
		return
	}
	manifest, ok := builtinModelRegistry.Lookup(identifier)
	if !ok {
		s.writeError(w, http.StatusNotFound, "model_not_found", "The requested model is not present in the Quantum Runtime registry.")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"api_version":         "v1alpha1",
		"schema_version":      modelmanifest.SchemaVersion,
		"requested_identifier": identifier,
		"model":               manifest,
	})
}
