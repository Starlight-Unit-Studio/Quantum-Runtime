package llamacpp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestReadyUsesLlamaHealthWithoutLeakingRuntimeCredentials(t *testing.T) {
	var auth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		auth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	}))
	defer upstream.Close()

	client := newTestClient(t, upstream, "server-secret")
	if err := client.Ready(context.Background()); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if auth != "Bearer server-secret" {
		t.Fatalf("unexpected llama.cpp auth header: %q", auth)
	}
}

func TestChatNonStreamingTranslatesOllamaToOpenAIAndBack(t *testing.T) {
	var observed map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&observed); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"Hallo ß","reasoning_content":"gedanke"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":4}}`)
	}))
	defer upstream.Close()

	client := newTestClient(t, upstream, "")
	source := httptest.NewRequest(http.MethodPost, "http://runtime/api/chat", strings.NewReader(`{"model":"ember-coreui:latest","messages":[{"role":"system","content":"Deutsch"},{"role":"user","content":"Grüße"}],"stream":false,"options":{"temperature":1.0,"top_k":64,"top_p":0.95}}`))
	source.Header.Set("Authorization", "Bearer runtime-secret")

	response, err := client.Do(context.Background(), source)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", response.StatusCode)
	}
	if observed["model"] != "ember-coreui:latest" || observed["stream"] != false {
		t.Fatalf("unexpected upstream payload: %#v", observed)
	}
	if _, exists := observed["num_ctx"]; exists {
		t.Fatalf("num_ctx must not be injected: %#v", observed)
	}
	if _, exists := observed["num_predict"]; exists {
		t.Fatalf("num_predict must not be injected: %#v", observed)
	}
	body, _ := io.ReadAll(response.Body)
	var translated map[string]any
	if err := json.Unmarshal(body, &translated); err != nil {
		t.Fatalf("decode translated response: %v body=%s", err, body)
	}
	message := translated["message"].(map[string]any)
	if message["content"] != "Hallo ß" || message["thinking"] != "gedanke" {
		t.Fatalf("unexpected translated message: %#v", message)
	}
	if translated["prompt_eval_count"].(float64) != 11 || translated["eval_count"].(float64) != 4 {
		t.Fatalf("usage not preserved: %#v", translated)
	}
}

func TestChatStreamingTranslatesSSEToOllamaNDJSON(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"eins\"},\"finish_reason\":null}]}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"denk\"},\"finish_reason\":null}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	client := newTestClient(t, upstream, "")
	source := httptest.NewRequest(http.MethodPost, "http://runtime/api/chat", strings.NewReader(`{"model":"ember-coreui:latest","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	response, err := client.Do(context.Background(), source)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"content":"eins"`) || !strings.Contains(text, `"thinking":"denk"`) || !strings.Contains(text, `"done":true`) {
		t.Fatalf("unexpected translated stream: %s", text)
	}
}

func TestChatFailsClosedOnUnsupportedCapabilitiesAndOptions(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unsupported request must not reach llama.cpp")
	}))
	defer upstream.Close()
	client := newTestClient(t, upstream, "")

	cases := []string{
		`{"model":"ember-coreui:latest","messages":[{"role":"user","content":"hi","images":["abc"]}]}`,
		`{"model":"ember-coreui:latest","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function"}]}`,
		`{"model":"ember-coreui:latest","messages":[{"role":"user","content":"hi"}],"think":true}`,
		`{"model":"ember-coreui:latest","messages":[{"role":"user","content":"hi"}],"options":{"num_ctx":16384}}`,
	}
	for _, body := range cases {
		source := httptest.NewRequest(http.MethodPost, "http://runtime/api/chat", strings.NewReader(body))
		response, err := client.Do(context.Background(), source)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for %s, got %d", body, response.StatusCode)
		}
	}
}

func TestGenerateAndEmbeddingsTranslate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/completions":
			_, _ = io.WriteString(w, `{"choices":[{"text":"result","finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
		case "/v1/embeddings":
			_, _ = io.WriteString(w, `{"data":[{"index":1,"embedding":[3,4]},{"index":0,"embedding":[1,2]}],"usage":{"prompt_tokens":5,"total_tokens":5}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	client := newTestClient(t, upstream, "")

	generate := httptest.NewRequest(http.MethodPost, "http://runtime/api/generate", strings.NewReader(`{"model":"ember-coreui:latest","prompt":"test","stream":false}`))
	response, err := client.Do(context.Background(), generate)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(data), `"response":"result"`) {
		t.Fatalf("unexpected generate response: %s", data)
	}

	embed := httptest.NewRequest(http.MethodPost, "http://runtime/api/embed", strings.NewReader(`{"model":"ember-coreui:latest","input":["a","b"]}`))
	response, err = client.Do(context.Background(), embed)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(data), `"embeddings":[[1,2],[3,4]]`) {
		t.Fatalf("unexpected embedding response: %s", data)
	}
}

func TestConfiguredModelIdentityIsNotSilentlySubstituted(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("mismatched model must not reach llama.cpp")
	}))
	defer upstream.Close()
	client := newTestClient(t, upstream, "")
	source := httptest.NewRequest(http.MethodPost, "http://runtime/api/chat", strings.NewReader(`{"model":"other-model","messages":[{"role":"user","content":"hi"}]}`))
	response, err := client.Do(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.StatusCode)
	}
}

func TestDescriptorIsFailClosedForUnnormalizedFeatures(t *testing.T) {
	upstreamURL, _ := url.Parse("http://127.0.0.1:8080")
	descriptor := New(upstreamURL, "test", "ember-coreui:latest", "").Descriptor()
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("descriptor: %v", err)
	}
	if descriptor.Capabilities.Text != "supported" || descriptor.Capabilities.Multimodal.Vision != "unsupported" || descriptor.Capabilities.Tools.Calling != "unsupported" {
		t.Fatalf("unexpected descriptor: %#v", descriptor.Capabilities)
	}
}

func newTestClient(t *testing.T, server *httptest.Server, apiKey string) *Client {
	t.Helper()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return NewWithClient(base, "test", "ember-coreui:latest", apiKey, server.Client())
}
