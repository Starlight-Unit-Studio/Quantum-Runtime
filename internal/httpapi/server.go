package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/config"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/ollama"
)

type Upstream interface {
	Do(context.Context, *http.Request) (*http.Response, error)
	Ready(context.Context) error
}

type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

type Server struct {
	config   config.Config
	upstream Upstream
	build    BuildInfo
	logger   *slog.Logger
	handler  http.Handler
}

type routePolicy struct {
	method   string
	mutation bool
}

var compatibilityRoutes = map[string]routePolicy{
	"/api/chat":       {method: http.MethodPost},
	"/api/generate":   {method: http.MethodPost},
	"/api/embed":      {method: http.MethodPost},
	"/api/embeddings": {method: http.MethodPost},
	"/api/tags":       {method: http.MethodGet},
	"/api/show":       {method: http.MethodPost},
	"/api/ps":         {method: http.MethodGet},
	"/api/version":    {method: http.MethodGet},
	"/api/pull":       {method: http.MethodPost, mutation: true},
	"/api/create":     {method: http.MethodPost, mutation: true},
	"/api/copy":       {method: http.MethodPost, mutation: true},
	"/api/delete":     {method: http.MethodDelete, mutation: true},
}

func New(cfg config.Config, upstream Upstream, build BuildInfo, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	s := &Server{
		config:   cfg,
		upstream: upstream,
		build:    build,
		logger:   logger,
	}
	s.handler = s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/readyz", s.handleReady)
	mux.HandleFunc("/v1/runtime", s.handleRuntime)
	mux.HandleFunc("/v1/capabilities", s.handleCapabilities)
	mux.HandleFunc("/api/", s.handleCompatibility)
	mux.HandleFunc("/", s.handleNotFound)
	return s.withRequestContext(s.withAccessLog(s.withRegistryRoutes(mux)))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"status":  "alive",
		"service": "quantum-runtime",
		"version": s.build.Version,
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	if s.upstream == nil {
		s.writeError(w, http.StatusServiceUnavailable, "upstream_unavailable", "No inference backend is configured.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.upstream.Ready(ctx); err != nil {
		s.logger.Warn("runtime not ready", "request_id", requestIDFromContext(r.Context()), "error", err)
		s.writeError(w, http.StatusServiceUnavailable, "upstream_unavailable", "The configured inference backend is not ready.")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ready",
		"backend": "ollama-adapter",
	})
}

func (s *Server) handleRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	if !s.authorized(r) {
		s.writeUnauthorized(w)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"name":        "Quantum Runtime",
		"service":     "quantum-runtime",
		"version":     s.build.Version,
		"commit":      s.build.Commit,
		"build_date":  s.build.BuildDate,
		"api_version": "v1alpha1",
		"backend": map[string]any{
			"type": "ollama-adapter",
		},
	})
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	if !s.authorized(r) {
		s.writeUnauthorized(w)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"native_api": "v1alpha1",
		"compatibility": []string{
			"ollama-api-chat",
			"ollama-api-generate",
			"ollama-api-embeddings",
			"ollama-api-model-read",
		},
		"backend":        "ollama-adapter",
		"model_mutation": s.config.AllowModelMutation,
		"streaming":      true,
	})
}

func (s *Server) handleCompatibility(w http.ResponseWriter, r *http.Request) {
	policy, ok := compatibilityRoutes[r.URL.Path]
	if !ok {
		s.writeError(w, http.StatusNotFound, "unsupported_compatibility_route", "This Ollama compatibility route is not supported.")
		return
	}
	if r.Method != policy.method {
		s.writeMethodNotAllowed(w, r, policy.method)
		return
	}
	if !s.authorized(r) {
		s.writeUnauthorized(w)
		return
	}
	if policy.mutation && !s.config.AllowModelMutation {
		s.writeError(w, http.StatusForbidden, "model_mutation_disabled", "Model mutation endpoints are disabled by policy.")
		return
	}
	if s.upstream == nil {
		s.writeError(w, http.StatusServiceUnavailable, "upstream_unavailable", "No inference backend is configured.")
		return
	}
	if r.ContentLength > s.config.RequestBodyLimit {
		s.writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "The request body exceeds the configured limit.")
		return
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, s.config.RequestBodyLimit)
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.config.UpstreamTimeout)
	defer cancel()
	response, err := s.upstream.Do(ctx, r)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			s.writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "The request body exceeds the configured limit.")
			return
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			s.writeError(w, http.StatusGatewayTimeout, "upstream_timeout", "The inference backend did not complete in time.")
			return
		}
		s.logger.Error("upstream request failed", "request_id", requestIDFromContext(r.Context()), "path", r.URL.Path, "error", err)
		s.writeError(w, http.StatusBadGateway, "upstream_error", "The inference backend request failed.")
		return
	}
	defer response.Body.Close()

	ollama.CopyResponseHeaders(w.Header(), response.Header)
	w.Header().Set("X-Quantum-Runtime-Version", s.build.Version)
	w.WriteHeader(response.StatusCode)

	writer := io.Writer(w)
	if flusher, ok := w.(http.Flusher); ok {
		writer = &flushingWriter{writer: w, flusher: flusher}
	}
	if _, err := io.CopyBuffer(writer, response.Body, make([]byte, 32*1024)); err != nil {
		s.logger.Warn("upstream response copy interrupted", "request_id", requestIDFromContext(r.Context()), "path", r.URL.Path, "error", err)
	}
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, http.StatusNotFound, "not_found", "The requested Quantum Runtime endpoint does not exist.")
}

func (s *Server) authorized(r *http.Request) bool {
	token := strings.TrimSpace(s.config.AuthToken)
	if token == "" {
		return true
	}
	provided := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(provided, prefix) {
		return false
	}
	return subtleEqual(strings.TrimSpace(strings.TrimPrefix(provided, prefix)), token)
}

func subtleEqual(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func (s *Server) writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="Quantum Runtime"`)
	s.writeError(w, http.StatusUnauthorized, "unauthorized", "A valid Quantum Runtime bearer token is required.")
}

func (s *Server) writeMethodNotAllowed(w http.ResponseWriter, r *http.Request, allowed string) {
	w.Header().Set("Allow", allowed)
	s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", fmt.Sprintf("Use %s for this endpoint.", allowed))
}

func (s *Server) writeError(w http.ResponseWriter, status int, code, message string) {
	s.writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":       code,
			"message":    message,
			"request_id": requestIDFromHeader(w.Header()),
		},
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Quantum-Runtime-Version", s.build.Version)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.logger.Warn("write JSON response", "error", err)
	}
}

func (s *Server) withRequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		w.Header().Set("X-Quantum-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		s.logger.Info("http request",
			"request_id", requestIDFromContext(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

type flushingWriter struct {
	writer  io.Writer
	flusher http.Flusher
}

func (w *flushingWriter) Write(data []byte) (int, error) {
	n, err := w.writer.Write(data)
	w.flusher.Flush()
	return n, err
}

type requestIDKey struct{}

func requestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func requestIDFromHeader(header http.Header) string {
	return header.Get("X-Quantum-Request-ID")
}

func newRequestID() string {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes[:])
}
