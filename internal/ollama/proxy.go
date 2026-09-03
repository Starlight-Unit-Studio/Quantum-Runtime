package ollama

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

// Proxy is the first Quantum Runtime backend. It forwards only explicitly
// approved Ollama-compatible operations selected by the HTTP API layer.
type Proxy struct {
	baseURL *url.URL
	client  *http.Client
	version string
}

func NewProxy(baseURL *url.URL, version string) *Proxy {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 32
	transport.MaxIdleConnsPerHost = 16
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = true

	return &Proxy{
		baseURL: cloneURL(baseURL),
		client: &http.Client{
			Transport: transport,
		},
		version: version,
	}
}

// NewProxyWithClient is intended for tests and controlled embedding.
func NewProxyWithClient(baseURL *url.URL, version string, client *http.Client) *Proxy {
	if client == nil {
		client = http.DefaultClient
	}
	return &Proxy{baseURL: cloneURL(baseURL), client: client, version: version}
}

func (p *Proxy) Do(ctx context.Context, source *http.Request) (*http.Response, error) {
	if p == nil || p.baseURL == nil || p.client == nil {
		return nil, fmt.Errorf("ollama proxy is not configured")
	}

	target := cloneURL(p.baseURL)
	target.Path = joinURLPath(target.Path, source.URL.Path)
	target.RawQuery = source.URL.RawQuery

	request, err := http.NewRequestWithContext(ctx, source.Method, target.String(), source.Body)
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}
	request.ContentLength = source.ContentLength
	copyRequestHeaders(request.Header, source.Header)
	request.Header.Set("User-Agent", "Quantum-Runtime/"+p.version+" (Ollama compatibility)")

	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("ollama upstream request: %w", err)
	}
	return response, nil
}

func (p *Proxy) Ready(ctx context.Context) error {
	if p == nil || p.baseURL == nil || p.client == nil {
		return fmt.Errorf("ollama proxy is not configured")
	}

	target := cloneURL(p.baseURL)
	target.Path = joinURLPath(target.Path, "/api/version")
	target.RawQuery = ""

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fmt.Errorf("create readiness request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Quantum-Runtime/"+p.version+" readiness")

	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("ollama readiness request: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("ollama readiness returned HTTP %d", response.StatusCode)
	}
	return nil
}

func copyRequestHeaders(destination, source http.Header) {
	for key, values := range source {
		canonical := http.CanonicalHeaderKey(key)
		if _, blocked := hopByHopHeaders[canonical]; blocked {
			continue
		}
		switch canonical {
		case "Authorization", "Cookie", "Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto":
			continue
		}
		for _, value := range values {
			destination.Add(canonical, value)
		}
	}
}

func CopyResponseHeaders(destination, source http.Header) {
	for key, values := range source {
		canonical := http.CanonicalHeaderKey(key)
		if _, blocked := hopByHopHeaders[canonical]; blocked {
			continue
		}
		if canonical == "Set-Cookie" {
			continue
		}
		for _, value := range values {
			destination.Add(canonical, value)
		}
	}
}

func cloneURL(source *url.URL) *url.URL {
	if source == nil {
		return nil
	}
	copy := *source
	return &copy
}

func joinURLPath(basePath, requestPath string) string {
	basePath = strings.TrimSuffix(basePath, "/")
	requestPath = "/" + strings.TrimPrefix(requestPath, "/")
	if basePath == "" {
		return requestPath
	}
	return basePath + requestPath
}
