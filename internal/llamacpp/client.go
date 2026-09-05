package llamacpp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/backendcontract"
)

// Client is the first direct llama.cpp backend adapter. It talks to llama-server
// without an Ollama daemon in the path while keeping Quantum Runtime's existing
// application-facing Ollama compatibility endpoints stable for current clients.
type Client struct {
	baseURL *url.URL
	client  *http.Client
	version string
	model   string
	apiKey  string
}

func New(baseURL *url.URL, version, model, apiKey string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 32
	transport.MaxIdleConnsPerHost = 16
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = true
	return &Client{
		baseURL: cloneURL(baseURL),
		client:  &http.Client{Transport: transport},
		version: version,
		model:   strings.TrimSpace(model),
		apiKey:  strings.TrimSpace(apiKey),
	}
}

// NewWithClient is intended for protocol tests and controlled embedding.
func NewWithClient(baseURL *url.URL, version, model, apiKey string, client *http.Client) *Client {
	if client == nil {
		client = http.DefaultClient
	}
	return &Client{
		baseURL: cloneURL(baseURL),
		client:  client,
		version: version,
		model:   strings.TrimSpace(model),
		apiKey:  strings.TrimSpace(apiKey),
	}
}

func (c *Client) Descriptor() backendcontract.Descriptor {
	return backendcontract.Descriptor{
		ContractVersion: backendcontract.ContractVersion,
		ID:              "llama.cpp",
		Kind:            "llama.cpp",
		AdapterVersion:  c.version,
		ExecutionMode:   "external",
		State:           "unknown",
		Capabilities: backendcontract.Capabilities{
			Text: backendcontract.SupportSupported,
			Architecture: backendcontract.ArchitectureCapabilities{
				Dense: backendcontract.SupportConditional,
				MoE:   backendcontract.SupportConditional,
			},
			MoE: backendcontract.MoECapabilities{
				ExpertOffload:  backendcontract.SupportConditional,
				ExpertParallel: backendcontract.SupportUnknown,
			},
			QuantizationFormats: []string{"gguf"},
			Speculative: backendcontract.SpeculativeCapabilities{
				MTP:        backendcontract.SupportUnknown,
				DraftModel: backendcontract.SupportConditional,
			},
			Cache: backendcontract.CacheCapabilities{
				KVOffload:   backendcontract.SupportSupported,
				PromptCache: backendcontract.SupportConditional,
			},
			Multimodal: backendcontract.MultimodalCapabilities{
				Vision: backendcontract.SupportUnsupported,
				Audio:  backendcontract.SupportUnsupported,
			},
			Embeddings:       backendcontract.SupportConditional,
			Reranking:        backendcontract.SupportUnsupported,
			ReasoningControl: backendcontract.SupportUnsupported,
			Tools: backendcontract.ToolCapabilities{
				Calling:   backendcontract.SupportUnsupported,
				Streaming: backendcontract.SupportUnsupported,
			},
			StructuredOutput: backendcontract.SupportUnsupported,
			Streaming: backendcontract.StreamingCapabilities{
				Content:       backendcontract.SupportSupported,
				Reasoning:     backendcontract.SupportConditional,
				ToolArguments: backendcontract.SupportUnsupported,
			},
			Placement: backendcontract.PlacementCapabilities{
				CPU:    backendcontract.SupportSupported,
				GPU:    backendcontract.SupportConditional,
				Hybrid: backendcontract.SupportConditional,
			},
			Context: backendcontract.ContextCapabilities{
				BackendManaged:    true,
				OverrideSupported: backendcontract.SupportUnsupported,
				OverrideVerified:  false,
			},
		},
	}
}

func (c *Client) Ready(ctx context.Context) error {
	if err := c.validate(); err != nil {
		return err
	}
	request, err := c.newRequest(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("llama.cpp health request: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("llama.cpp health returned HTTP %d", response.StatusCode)
	}
	return nil
}

func (c *Client) Do(ctx context.Context, source *http.Request) (*http.Response, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	if source == nil || source.URL == nil {
		return nil, fmt.Errorf("source request is missing")
	}
	switch source.URL.Path {
	case "/api/chat":
		return c.doChat(ctx, source)
	case "/api/generate":
		return c.doGenerate(ctx, source)
	case "/api/embed", "/api/embeddings":
		return c.doEmbeddings(ctx, source)
	case "/api/tags":
		return c.modelTags(), nil
	case "/api/show":
		return c.modelShow(source)
	case "/api/ps":
		return c.modelProcesses(), nil
	case "/api/version":
		return jsonResponse(http.StatusOK, map[string]any{"version": "llama.cpp-via-quantum-runtime/" + c.version}), nil
	case "/api/pull", "/api/create", "/api/copy", "/api/delete":
		return jsonResponse(http.StatusNotImplemented, map[string]any{
			"error": "model mutation is not implemented by the llama.cpp direct adapter",
		}), nil
	default:
		return jsonResponse(http.StatusNotFound, map[string]any{
			"error": "unsupported llama.cpp compatibility route",
		}), nil
	}
}

func (c *Client) validate() error {
	if c == nil || c.baseURL == nil || c.client == nil {
		return fmt.Errorf("llama.cpp backend is not configured")
	}
	if c.model == "" {
		return fmt.Errorf("llama.cpp backend model is not configured")
	}
	return nil
}

type ollamaMessage struct {
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	Images    []string        `json:"images,omitempty"`
	ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
}

type ollamaChatRequest struct {
	Model    string                     `json:"model"`
	Messages []ollamaMessage            `json:"messages"`
	Stream   *bool                      `json:"stream,omitempty"`
	Options  map[string]json.RawMessage `json:"options,omitempty"`
	Format   json.RawMessage            `json:"format,omitempty"`
	Tools    json.RawMessage            `json:"tools,omitempty"`
	Think    json.RawMessage            `json:"think,omitempty"`
}

func (c *Client) doChat(ctx context.Context, source *http.Request) (*http.Response, error) {
	var input ollamaChatRequest
	if err := decodeRequest(source.Body, &input); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid Ollama chat JSON: " + err.Error()}), nil
	}
	if response := c.validateRequestedModel(input.Model); response != nil {
		return response, nil
	}
	if len(input.Messages) == 0 {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "messages are required"}), nil
	}
	if rawPresent(input.Tools) {
		return unsupported("tools.calling is not yet normalized by the llama.cpp direct adapter"), nil
	}
	if rawPresent(input.Think) {
		return unsupported("reasoning.control is not yet normalized by the llama.cpp direct adapter"), nil
	}
	if rawPresent(input.Format) {
		return unsupported("structured_output is not yet normalized by the llama.cpp direct adapter"), nil
	}
	messages := make([]map[string]any, 0, len(input.Messages))
	for _, message := range input.Messages {
		switch message.Role {
		case "system", "user", "assistant":
		default:
			return unsupported("tool and custom message roles are not yet normalized by the llama.cpp direct adapter"), nil
		}
		if len(message.Images) > 0 {
			return unsupported("multimodal.vision is not yet normalized by the llama.cpp direct adapter"), nil
		}
		if rawPresent(message.ToolCalls) {
			return unsupported("tool call history is not yet normalized by the llama.cpp direct adapter"), nil
		}
		messages = append(messages, map[string]any{"role": message.Role, "content": message.Content})
	}

	stream := true
	if input.Stream != nil {
		stream = *input.Stream
	}
	payload := map[string]any{
		"model":    c.model,
		"messages": messages,
		"stream":   stream,
	}
	if response := applySafeOptions(payload, input.Options); response != nil {
		return response, nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := c.newRequest(ctx, http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("llama.cpp chat request: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response, nil
	}
	if stream {
		return translateChatStream(response, input.Model), nil
	}
	return translateChatResponse(response, input.Model)
}

type ollamaGenerateRequest struct {
	Model   string                     `json:"model"`
	Prompt  string                     `json:"prompt"`
	Stream  *bool                      `json:"stream,omitempty"`
	Options map[string]json.RawMessage `json:"options,omitempty"`
	Images  []string                   `json:"images,omitempty"`
	Format  json.RawMessage            `json:"format,omitempty"`
}

func (c *Client) doGenerate(ctx context.Context, source *http.Request) (*http.Response, error) {
	var input ollamaGenerateRequest
	if err := decodeRequest(source.Body, &input); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid Ollama generate JSON: " + err.Error()}), nil
	}
	if response := c.validateRequestedModel(input.Model); response != nil {
		return response, nil
	}
	if len(input.Images) > 0 {
		return unsupported("multimodal.vision is not yet normalized by the llama.cpp direct adapter"), nil
	}
	if rawPresent(input.Format) {
		return unsupported("structured_output is not yet normalized by the llama.cpp direct adapter"), nil
	}
	stream := true
	if input.Stream != nil {
		stream = *input.Stream
	}
	payload := map[string]any{
		"model":  c.model,
		"prompt": input.Prompt,
		"stream": stream,
	}
	if response := applySafeOptions(payload, input.Options); response != nil {
		return response, nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := c.newRequest(ctx, http.MethodPost, "/v1/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("llama.cpp completion request: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response, nil
	}
	if stream {
		return translateGenerateStream(response, input.Model), nil
	}
	return translateGenerateResponse(response, input.Model)
}

type ollamaEmbedRequest struct {
	Model   string                     `json:"model"`
	Input   json.RawMessage            `json:"input,omitempty"`
	Prompt  string                     `json:"prompt,omitempty"`
	Options map[string]json.RawMessage `json:"options,omitempty"`
}

func (c *Client) doEmbeddings(ctx context.Context, source *http.Request) (*http.Response, error) {
	var input ollamaEmbedRequest
	if err := decodeRequest(source.Body, &input); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid Ollama embedding JSON: " + err.Error()}), nil
	}
	if response := c.validateRequestedModel(input.Model); response != nil {
		return response, nil
	}
	if len(input.Options) > 0 {
		return unsupported("embedding options are not yet normalized by the llama.cpp direct adapter"), nil
	}
	var embeddingInput any
	if source.URL.Path == "/api/embeddings" {
		embeddingInput = input.Prompt
	} else {
		if !rawPresent(input.Input) {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "input is required"}), nil
		}
		if err := json.Unmarshal(input.Input, &embeddingInput); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "input must be valid JSON"}), nil
		}
	}
	payload := map[string]any{"model": c.model, "input": embeddingInput}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := c.newRequest(ctx, http.MethodPost, "/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("llama.cpp embedding request: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response, nil
	}
	defer response.Body.Close()
	var upstream struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(response.Body).Decode(&upstream); err != nil {
		return nil, fmt.Errorf("decode llama.cpp embedding response: %w", err)
	}
	sort.Slice(upstream.Data, func(i, j int) bool { return upstream.Data[i].Index < upstream.Data[j].Index })
	embeddings := make([][]float64, 0, len(upstream.Data))
	for _, item := range upstream.Data {
		embeddings = append(embeddings, item.Embedding)
	}
	if source.URL.Path == "/api/embeddings" {
		if len(embeddings) == 0 {
			return nil, fmt.Errorf("llama.cpp embedding response contained no vectors")
		}
		return jsonResponse(http.StatusOK, map[string]any{"embedding": embeddings[0]}), nil
	}
	return jsonResponse(http.StatusOK, map[string]any{
		"model":             input.Model,
		"embeddings":        embeddings,
		"prompt_eval_count": upstream.Usage.PromptTokens,
	}), nil
}

func (c *Client) modelTags() *http.Response {
	return jsonResponse(http.StatusOK, map[string]any{
		"models": []any{map[string]any{
			"name":        c.model,
			"model":       c.model,
			"modified_at": time.Now().UTC().Format(time.RFC3339Nano),
			"size":        0,
			"digest":      "",
			"details": map[string]any{
				"format":             "gguf",
				"family":             "runtime-configured",
				"parameter_size":     "unknown",
				"quantization_level": "unknown",
			},
		}},
	})
}

func (c *Client) modelShow(source *http.Request) (*http.Response, error) {
	var input struct {
		Model string `json:"model"`
		Name  string `json:"name"`
	}
	if err := decodeRequest(source.Body, &input); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid Ollama show JSON: " + err.Error()}), nil
	}
	requested := input.Model
	if requested == "" {
		requested = input.Name
	}
	if response := c.validateRequestedModel(requested); response != nil {
		return response, nil
	}
	return jsonResponse(http.StatusOK, map[string]any{
		"details": map[string]any{
			"format":             "gguf",
			"family":             "runtime-configured",
			"parameter_size":     "unknown",
			"quantization_level": "unknown",
		},
		"model_info":   map[string]any{},
		"capabilities": []string{"completion", "embedding"},
	}), nil
}

func (c *Client) modelProcesses() *http.Response {
	return jsonResponse(http.StatusOK, map[string]any{
		"models": []any{map[string]any{
			"name":       c.model,
			"model":      c.model,
			"size":       0,
			"size_vram":  0,
			"digest":     "",
			"expires_at": time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano),
			"details": map[string]any{
				"format": "gguf",
				"family": "runtime-configured",
			},
		}},
	})
}

func (c *Client) validateRequestedModel(requested string) *http.Response {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "model is required"})
	}
	if requested != c.model {
		return jsonResponse(http.StatusNotFound, map[string]any{
			"error": fmt.Sprintf("model %q is not the configured llama.cpp model %q", requested, c.model),
		})
	}
	return nil
}

func applySafeOptions(payload map[string]any, options map[string]json.RawMessage) *http.Response {
	if len(options) == 0 {
		return nil
	}
	allowed := map[string]string{
		"temperature": "temperature",
		"top_p":       "top_p",
		"top_k":       "top_k",
	}
	keys := make([]string, 0, len(options))
	for key := range options {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		target, ok := allowed[key]
		if !ok {
			return unsupported("llama.cpp direct adapter does not yet normalize option " + key)
		}
		var value any
		if err := json.Unmarshal(options[key], &value); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid option " + key})
		}
		payload[target] = value
	}
	return nil
}

func unsupported(message string) *http.Response {
	return jsonResponse(http.StatusUnprocessableEntity, map[string]any{
		"error": map[string]any{
			"code":    "unsupported_backend_capability",
			"message": message,
		},
	})
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	target := cloneURL(c.baseURL)
	target.Path = joinURLPath(target.Path, path)
	target.RawQuery = ""
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create llama.cpp request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Quantum-Runtime/"+c.version+" (llama.cpp direct adapter)")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	return request, nil
}

func decodeRequest(body io.Reader, destination any) error {
	if body == nil {
		return fmt.Errorf("request body is required")
	}
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("request contains trailing JSON")
	}
	return nil
}

func translateChatResponse(response *http.Response, model string) (*http.Response, error) {
	defer response.Body.Close()
	var upstream struct {
		Choices []struct {
			Message struct {
				Role             string `json:"role"`
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				Reasoning        string `json:"reasoning"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(response.Body).Decode(&upstream); err != nil {
		return nil, fmt.Errorf("decode llama.cpp chat response: %w", err)
	}
	if len(upstream.Choices) == 0 {
		return nil, fmt.Errorf("llama.cpp chat response contained no choices")
	}
	choice := upstream.Choices[0]
	message := map[string]any{"role": "assistant", "content": choice.Message.Content}
	reasoning := choice.Message.ReasoningContent
	if reasoning == "" {
		reasoning = choice.Message.Reasoning
	}
	if reasoning != "" {
		message["thinking"] = reasoning
	}
	payload := map[string]any{
		"model":             model,
		"created_at":        time.Now().UTC().Format(time.RFC3339Nano),
		"message":           message,
		"done":              true,
		"prompt_eval_count": upstream.Usage.PromptTokens,
		"eval_count":        upstream.Usage.CompletionTokens,
	}
	if choice.FinishReason != "" {
		payload["done_reason"] = choice.FinishReason
	}
	return jsonResponse(http.StatusOK, payload), nil
}

func translateGenerateResponse(response *http.Response, model string) (*http.Response, error) {
	defer response.Body.Close()
	var upstream struct {
		Choices []struct {
			Text         string `json:"text"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(response.Body).Decode(&upstream); err != nil {
		return nil, fmt.Errorf("decode llama.cpp completion response: %w", err)
	}
	if len(upstream.Choices) == 0 {
		return nil, fmt.Errorf("llama.cpp completion response contained no choices")
	}
	choice := upstream.Choices[0]
	payload := map[string]any{
		"model":             model,
		"created_at":        time.Now().UTC().Format(time.RFC3339Nano),
		"response":          choice.Text,
		"done":              true,
		"prompt_eval_count": upstream.Usage.PromptTokens,
		"eval_count":        upstream.Usage.CompletionTokens,
	}
	if choice.FinishReason != "" {
		payload["done_reason"] = choice.FinishReason
	}
	return jsonResponse(http.StatusOK, payload), nil
}

func translateChatStream(response *http.Response, model string) *http.Response {
	reader, writer := io.Pipe()
	go func() {
		defer response.Body.Close()
		defer writer.Close()
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		done := false
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				if !done {
					if err := writeChatChunk(writer, model, "", "", true, "stop"); err != nil {
						_ = writer.CloseWithError(err)
						return
					}
				}
				return
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content          string          `json:"content"`
						ReasoningContent string          `json:"reasoning_content"`
						Reasoning        string          `json:"reasoning"`
						ToolCalls        json.RawMessage `json:"tool_calls"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				_ = writer.CloseWithError(fmt.Errorf("decode llama.cpp chat stream: %w", err))
				return
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]
			if rawPresent(choice.Delta.ToolCalls) {
				_ = writeStreamError(writer, "llama.cpp produced tool-call streaming that this adapter does not yet normalize")
				return
			}
			reasoning := choice.Delta.ReasoningContent
			if reasoning == "" {
				reasoning = choice.Delta.Reasoning
			}
			if choice.Delta.Content != "" || reasoning != "" {
				if err := writeChatChunk(writer, model, choice.Delta.Content, reasoning, false, ""); err != nil {
					_ = writer.CloseWithError(err)
					return
				}
			}
			if choice.FinishReason != nil {
				done = true
				if err := writeChatChunk(writer, model, "", "", true, *choice.FinishReason); err != nil {
					_ = writer.CloseWithError(err)
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			_ = writer.CloseWithError(fmt.Errorf("read llama.cpp chat stream: %w", err))
			return
		}
		if !done {
			_ = writeChatChunk(writer, model, "", "", true, "stop")
		}
	}()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":  []string{"application/x-ndjson"},
			"Cache-Control": []string{"no-cache"},
		},
		Body: reader,
	}
}

func translateGenerateStream(response *http.Response, model string) *http.Response {
	reader, writer := io.Pipe()
	go func() {
		defer response.Body.Close()
		defer writer.Close()
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		done := false
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				if !done {
					if err := writeGenerateChunk(writer, model, "", true, "stop"); err != nil {
						_ = writer.CloseWithError(err)
					}
				}
				return
			}
			var chunk struct {
				Choices []struct {
					Text         string  `json:"text"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				_ = writer.CloseWithError(fmt.Errorf("decode llama.cpp completion stream: %w", err))
				return
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]
			if choice.Text != "" {
				if err := writeGenerateChunk(writer, model, choice.Text, false, ""); err != nil {
					_ = writer.CloseWithError(err)
					return
				}
			}
			if choice.FinishReason != nil {
				done = true
				if err := writeGenerateChunk(writer, model, "", true, *choice.FinishReason); err != nil {
					_ = writer.CloseWithError(err)
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			_ = writer.CloseWithError(fmt.Errorf("read llama.cpp completion stream: %w", err))
			return
		}
		if !done {
			_ = writeGenerateChunk(writer, model, "", true, "stop")
		}
	}()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":  []string{"application/x-ndjson"},
			"Cache-Control": []string{"no-cache"},
		},
		Body: reader,
	}
}

func writeChatChunk(writer io.Writer, model, content, reasoning string, done bool, doneReason string) error {
	message := map[string]any{"role": "assistant", "content": content}
	if reasoning != "" {
		message["thinking"] = reasoning
	}
	payload := map[string]any{
		"model":      model,
		"created_at": time.Now().UTC().Format(time.RFC3339Nano),
		"message":    message,
		"done":       done,
	}
	if done && doneReason != "" {
		payload["done_reason"] = doneReason
	}
	return writeNDJSON(writer, payload)
}

func writeGenerateChunk(writer io.Writer, model, text string, done bool, doneReason string) error {
	payload := map[string]any{
		"model":      model,
		"created_at": time.Now().UTC().Format(time.RFC3339Nano),
		"response":   text,
		"done":       done,
	}
	if done && doneReason != "" {
		payload["done_reason"] = doneReason
	}
	return writeNDJSON(writer, payload)
}

func writeStreamError(writer io.Writer, message string) error {
	return writeNDJSON(writer, map[string]any{"error": message})
}

func writeNDJSON(writer io.Writer, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = writer.Write(data)
	return err
}

func jsonResponse(status int, payload any) *http.Response {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{"error":"encode response"}`)
		status = http.StatusInternalServerError
	}
	return &http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(bytes.NewReader(data)),
	}
}

func rawPresent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("false")) || bytes.Equal(trimmed, []byte("[]")) || bytes.Equal(trimmed, []byte("{}")) || bytes.Equal(trimmed, []byte(`""`)) {
		return false
	}
	return true
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
