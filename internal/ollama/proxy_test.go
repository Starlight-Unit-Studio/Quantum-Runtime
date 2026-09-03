package ollama

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestProxyForwardsApprovedRequestShapeWithoutRuntimeCredentials(t *testing.T) {
	var observedPath string
	var observedQuery string
	var observedAuthorization string
	var observedCookie string
	var observedContentType string
	var observedBody string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedPath = r.URL.Path
		observedQuery = r.URL.RawQuery
		observedAuthorization = r.Header.Get("Authorization")
		observedCookie = r.Header.Get("Cookie")
		observedContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		observedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	base, err := url.Parse(upstream.URL + "/root")
	if err != nil {
		t.Fatal(err)
	}
	proxy := NewProxyWithClient(base, "test", upstream.Client())

	source := httptest.NewRequest(http.MethodPost, "http://runtime.local/api/chat?trace=1", strings.NewReader(`{"model":"demo"}`))
	source.Header.Set("Authorization", "Bearer runtime-secret")
	source.Header.Set("Cookie", "session=secret")
	source.Header.Set("Content-Type", "application/json")

	response, err := proxy.Do(context.Background(), source)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer response.Body.Close()

	if observedPath != "/root/api/chat" {
		t.Fatalf("unexpected path: %q", observedPath)
	}
	if observedQuery != "trace=1" {
		t.Fatalf("unexpected query: %q", observedQuery)
	}
	if observedAuthorization != "" || observedCookie != "" {
		t.Fatalf("runtime credentials leaked upstream: auth=%q cookie=%q", observedAuthorization, observedCookie)
	}
	if observedContentType != "application/json" {
		t.Fatalf("content type not forwarded: %q", observedContentType)
	}
	if observedBody != `{"model":"demo"}` {
		t.Fatalf("unexpected body: %q", observedBody)
	}
}

func TestProxyReady(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"version":"test"}`)
	}))
	defer upstream.Close()

	base, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := NewProxyWithClient(base, "test", upstream.Client())
	if err := proxy.Ready(context.Background()); err != nil {
		t.Fatalf("Ready: %v", err)
	}
}
