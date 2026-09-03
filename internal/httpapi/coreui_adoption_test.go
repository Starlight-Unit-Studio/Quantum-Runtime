package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/ollama"
)

func TestCoreUIAdoptionCompletesRepresentativeChatThroughRuntime(t *testing.T) {
	fixture, err := os.ReadFile("testdata/coreui-chat-request.json")
	if err != nil {
		t.Fatalf("read CoreUI fixture: %v", err)
	}

	var observed []byte
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected upstream path %q", r.URL.Path)
		}
		observed, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"message":{"content":"Quantum Runtime adoption ok"},"done":true}`)
	}))
	defer ollamaServer.Close()

	upstream, err := url.Parse(ollamaServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := ollama.NewProxyWithClient(upstream, "test", ollamaServer.Client())
	runtime := New(testConfig(t), proxy, testBuild(), discardLogger())
	runtimeServer := httptest.NewServer(runtime.Handler())
	defer runtimeServer.Close()

	request, err := http.NewRequest(http.MethodPost, runtimeServer.URL+"/api/chat", strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := runtimeServer.Client().Do(request)
	if err != nil {
		t.Fatalf("CoreUI request through Quantum Runtime failed: %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected Runtime status %d body=%s", response.StatusCode, body)
	}
	if string(observed) != string(fixture) {
		t.Fatalf("CoreUI payload changed in transit\nwant: %s\n got: %s", fixture, observed)
	}
	if !strings.Contains(string(body), "Quantum Runtime adoption ok") {
		t.Fatalf("representative CoreUI chat did not complete: %s", body)
	}
}
