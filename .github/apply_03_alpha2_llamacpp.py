from pathlib import Path
import json


def write(path: str, content: str) -> None:
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(content, encoding="utf-8")


def replace_once(path: str, old: str, new: str) -> None:
    target = Path(path)
    text = target.read_text(encoding="utf-8")
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected exactly one replacement marker, found {count}: {old!r}")
    target.write_text(text.replace(old, new, 1), encoding="utf-8")


write("internal/llamacpp/client.go", r'''package llamacpp

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
    Model  string                     `json:"model"`
    Input  json.RawMessage            `json:"input,omitempty"`
    Prompt string                     `json:"prompt,omitempty"`
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
            "Content-Type": []string{"application/x-ndjson"},
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
            "Content-Type": []string{"application/x-ndjson"},
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
''')

write("internal/llamacpp/client_test.go", r'''package llamacpp

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
        if flusher != nil { flusher.Flush() }
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
    if err != nil { t.Fatal(err) }
    data, _ := io.ReadAll(response.Body)
    response.Body.Close()
    if !strings.Contains(string(data), `"response":"result"`) {
        t.Fatalf("unexpected generate response: %s", data)
    }

    embed := httptest.NewRequest(http.MethodPost, "http://runtime/api/embed", strings.NewReader(`{"model":"ember-coreui:latest","input":["a","b"]}`))
    response, err = client.Do(context.Background(), embed)
    if err != nil { t.Fatal(err) }
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
    if err != nil { t.Fatal(err) }
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
    if err != nil { t.Fatal(err) }
    return NewWithClient(base, "test", "ember-coreui:latest", apiKey, server.Client())
}
''')

# Config: introduce backend selection and llama.cpp connection settings.
replace_once(
    "internal/config/config.go",
    '''const (\n\tdefaultListenAddress    = "127.0.0.1:11450"\n\tdefaultUpstreamURL      = "http://127.0.0.1:11434"\n\tdefaultUpstreamTimeout  = 15 * time.Minute\n\tdefaultRequestBodyLimit = int64(128 << 20)\n)''',
    '''const (\n\tdefaultListenAddress    = "127.0.0.1:11450"\n\tdefaultBackend          = "ollama"\n\tdefaultUpstreamURL      = "http://127.0.0.1:11434"\n\tdefaultLlamaCPPURL      = "http://127.0.0.1:8080"\n\tdefaultUpstreamTimeout  = 15 * time.Minute\n\tdefaultRequestBodyLimit = int64(128 << 20)\n)''',
)
replace_once(
    "internal/config/config.go",
    '''type Config struct {\n\tListenAddress      string\n\tUpstreamURL        *url.URL\n\tUpstreamTimeout    time.Duration\n\tRequestBodyLimit   int64\n\tAllowModelMutation bool\n\tAuthToken          string\n}''',
    '''type Config struct {\n\tListenAddress      string\n\tBackend            string\n\tUpstreamURL        *url.URL\n\tLlamaCPPURL        *url.URL\n\tLlamaCPPModel      string\n\tLlamaCPPAPIKey     string\n\tUpstreamTimeout    time.Duration\n\tRequestBodyLimit   int64\n\tAllowModelMutation bool\n\tAuthToken          string\n}''',
)
replace_once(
    "internal/config/config.go",
    '''\tlisten := valueOrDefault(getenv("QUANTUM_RUNTIME_LISTEN"), defaultListenAddress)\n\tupstreamRaw := valueOrDefault(getenv("QUANTUM_RUNTIME_OLLAMA_URL"), defaultUpstreamURL)\n\tupstream, err := url.Parse(upstreamRaw)\n\tif err != nil {\n\t\treturn Config{}, fmt.Errorf("parse QUANTUM_RUNTIME_OLLAMA_URL: %w", err)\n\t}\n''',
    '''\tlisten := valueOrDefault(getenv("QUANTUM_RUNTIME_LISTEN"), defaultListenAddress)\n\tbackend := strings.ToLower(valueOrDefault(getenv("QUANTUM_RUNTIME_BACKEND"), defaultBackend))\n\tupstreamRaw := valueOrDefault(getenv("QUANTUM_RUNTIME_OLLAMA_URL"), defaultUpstreamURL)\n\tupstream, err := url.Parse(upstreamRaw)\n\tif err != nil {\n\t\treturn Config{}, fmt.Errorf("parse QUANTUM_RUNTIME_OLLAMA_URL: %w", err)\n\t}\n\tllamaRaw := valueOrDefault(getenv("QUANTUM_RUNTIME_LLAMA_CPP_URL"), defaultLlamaCPPURL)\n\tllamaURL, err := url.Parse(llamaRaw)\n\tif err != nil {\n\t\treturn Config{}, fmt.Errorf("parse QUANTUM_RUNTIME_LLAMA_CPP_URL: %w", err)\n\t}\n''',
)
replace_once(
    "internal/config/config.go",
    '''\tcfg := Config{\n\t\tListenAddress:      listen,\n\t\tUpstreamURL:        upstream,\n\t\tUpstreamTimeout:    upstreamTimeout,\n\t\tRequestBodyLimit:   bodyLimit,\n\t\tAllowModelMutation: allowMutation,\n\t\tAuthToken:          strings.TrimSpace(getenv("QUANTUM_RUNTIME_AUTH_TOKEN")),\n\t}''',
    '''\tcfg := Config{\n\t\tListenAddress:      listen,\n\t\tBackend:            backend,\n\t\tUpstreamURL:        upstream,\n\t\tLlamaCPPURL:        llamaURL,\n\t\tLlamaCPPModel:      strings.TrimSpace(getenv("QUANTUM_RUNTIME_LLAMA_CPP_MODEL")),\n\t\tLlamaCPPAPIKey:     strings.TrimSpace(getenv("QUANTUM_RUNTIME_LLAMA_CPP_API_KEY")),\n\t\tUpstreamTimeout:    upstreamTimeout,\n\t\tRequestBodyLimit:   bodyLimit,\n\t\tAllowModelMutation: allowMutation,\n\t\tAuthToken:          strings.TrimSpace(getenv("QUANTUM_RUNTIME_AUTH_TOKEN")),\n\t}''',
)
replace_once(
    "internal/config/config.go",
    '''\tif c.UpstreamURL == nil {\n\t\treturn errors.New("upstream URL is missing")\n\t}\n\tif c.UpstreamURL.Scheme != "http" && c.UpstreamURL.Scheme != "https" {\n\t\treturn errors.New("upstream URL must use http or https")\n\t}\n\tif c.UpstreamURL.Host == "" {\n\t\treturn errors.New("upstream URL host is missing")\n\t}\n\tif c.UpstreamURL.User != nil {\n\t\treturn errors.New("upstream URL must not contain credentials")\n\t}\n''',
    '''\tif c.Backend != "ollama" && c.Backend != "llama.cpp" {\n\t\treturn fmt.Errorf("QUANTUM_RUNTIME_BACKEND must be ollama or llama.cpp, got %q", c.Backend)\n\t}\n\tif err := validateHTTPURL("Ollama upstream", c.UpstreamURL); err != nil {\n\t\treturn err\n\t}\n\tif err := validateHTTPURL("llama.cpp upstream", c.LlamaCPPURL); err != nil {\n\t\treturn err\n\t}\n\tif c.Backend == "llama.cpp" && strings.TrimSpace(c.LlamaCPPModel) == "" {\n\t\treturn errors.New("QUANTUM_RUNTIME_LLAMA_CPP_MODEL is required when QUANTUM_RUNTIME_BACKEND=llama.cpp")\n\t}\n''',
)
replace_once(
    "internal/config/config.go",
    '''func isLoopbackListenAddress(address string) bool {''',
    '''func validateHTTPURL(name string, value *url.URL) error {\n\tif value == nil {\n\t\treturn fmt.Errorf("%s URL is missing", name)\n\t}\n\tif value.Scheme != "http" && value.Scheme != "https" {\n\t\treturn fmt.Errorf("%s URL must use http or https", name)\n\t}\n\tif value.Host == "" {\n\t\treturn fmt.Errorf("%s URL host is missing", name)\n\t}\n\tif value.User != nil {\n\t\treturn fmt.Errorf("%s URL must not contain credentials", name)\n\t}\n\treturn nil\n}\n\nfunc isLoopbackListenAddress(address string) bool {''',
)

# Config tests.
replace_once(
    "internal/config/config_test.go",
    '''\tif cfg.ListenAddress != "127.0.0.1:11450" {\n\t\tt.Fatalf("unexpected listen address: %q", cfg.ListenAddress)\n\t}\n''',
    '''\tif cfg.ListenAddress != "127.0.0.1:11450" {\n\t\tt.Fatalf("unexpected listen address: %q", cfg.ListenAddress)\n\t}\n\tif cfg.Backend != "ollama" {\n\t\tt.Fatalf("unexpected default backend: %q", cfg.Backend)\n\t}\n\tif got := cfg.LlamaCPPURL.String(); got != "http://127.0.0.1:8080" {\n\t\tt.Fatalf("unexpected llama.cpp URL: %q", got)\n\t}\n''',
)
replace_once(
    "internal/config/config_test.go",
    '''func TestLoadWithRejectsInvalidUpstreamScheme(t *testing.T) {''',
    '''func TestLoadWithLlamaCPPBackend(t *testing.T) {\n\tcfg, err := LoadWith(envMap(map[string]string{\n\t\t"QUANTUM_RUNTIME_BACKEND":             "llama.cpp",\n\t\t"QUANTUM_RUNTIME_LLAMA_CPP_URL":       "http://127.0.0.1:9090",\n\t\t"QUANTUM_RUNTIME_LLAMA_CPP_MODEL":     "ember-coreui:latest",\n\t\t"QUANTUM_RUNTIME_LLAMA_CPP_API_KEY":   "llama-secret",\n\t}))\n\tif err != nil {\n\t\tt.Fatalf("llama.cpp config: %v", err)\n\t}\n\tif cfg.Backend != "llama.cpp" || cfg.LlamaCPPModel != "ember-coreui:latest" || cfg.LlamaCPPAPIKey != "llama-secret" {\n\t\tt.Fatalf("unexpected llama.cpp config: %#v", cfg)\n\t}\n\tif got := cfg.LlamaCPPURL.String(); got != "http://127.0.0.1:9090" {\n\t\tt.Fatalf("unexpected llama.cpp URL: %q", got)\n\t}\n}\n\nfunc TestLoadWithLlamaCPPRequiresModelIdentity(t *testing.T) {\n\t_, err := LoadWith(envMap(map[string]string{\n\t\t"QUANTUM_RUNTIME_BACKEND": "llama.cpp",\n\t}))\n\tif err == nil || !strings.Contains(err.Error(), "QUANTUM_RUNTIME_LLAMA_CPP_MODEL") {\n\t\tt.Fatalf("expected llama.cpp model requirement, got %v", err)\n\t}\n}\n\nfunc TestLoadWithRejectsUnknownBackend(t *testing.T) {\n\t_, err := LoadWith(envMap(map[string]string{\n\t\t"QUANTUM_RUNTIME_BACKEND": "mystery",\n\t}))\n\tif err == nil || !strings.Contains(err.Error(), "QUANTUM_RUNTIME_BACKEND") {\n\t\tt.Fatalf("expected backend validation failure, got %v", err)\n\t}\n}\n\nfunc TestLoadWithRejectsInvalidUpstreamScheme(t *testing.T) {''',
)

# Runtime entrypoint selects either direct llama.cpp or Ollama adoption backend.
replace_once(
    "cmd/quantum-runtime/main.go",
    '''\t"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/httpapi"\n\t"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/ollama"''',
    '''\t"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/httpapi"\n\t"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/llamacpp"\n\t"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/ollama"''',
)
replace_once(
    "cmd/quantum-runtime/main.go",
    '''\t\tfmt.Printf("configuration valid: listen=%s upstream=%s mutation=%t auth=%t\\n",\n\t\t\tcfg.ListenAddress,\n\t\t\tcfg.UpstreamURL.Redacted(),\n\t\t\tcfg.AllowModelMutation,\n\t\t\tcfg.AuthToken != "",\n\t\t)''',
    '''\t\tfmt.Printf("configuration valid: listen=%s backend=%s ollama=%s llama_cpp=%s mutation=%t auth=%t\\n",\n\t\t\tcfg.ListenAddress,\n\t\t\tcfg.Backend,\n\t\t\tcfg.UpstreamURL.Redacted(),\n\t\t\tcfg.LlamaCPPURL.Redacted(),\n\t\t\tcfg.AllowModelMutation,\n\t\t\tcfg.AuthToken != "",\n\t\t)''',
)
replace_once(
    "cmd/quantum-runtime/main.go",
    '''\tlogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))\n\tbackend := ollama.NewProxy(cfg.UpstreamURL, buildinfo.Version)\n\tapi := httpapi.New(cfg, backend, httpapi.BuildInfo{''',
    '''\tlogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))\n\tvar backend httpapi.Upstream\n\tswitch cfg.Backend {\n\tcase "llama.cpp":\n\t\tbackend = llamacpp.New(cfg.LlamaCPPURL, buildinfo.Version, cfg.LlamaCPPModel, cfg.LlamaCPPAPIKey)\n\tdefault:\n\t\tbackend = ollama.NewProxy(cfg.UpstreamURL, buildinfo.Version)\n\t}\n\tapi := httpapi.New(cfg, backend, httpapi.BuildInfo{''',
)
replace_once(
    "cmd/quantum-runtime/main.go",
    '''\t\tlogger.Info("Quantum Runtime starting",\n\t\t\t"version", buildinfo.Version,\n\t\t\t"listen", cfg.ListenAddress,\n\t\t\t"backend", "ollama-adapter",\n\t\t\t"model_mutation", cfg.AllowModelMutation,\n\t\t)''',
    '''\t\tlogger.Info("Quantum Runtime starting",\n\t\t\t"version", buildinfo.Version,\n\t\t\t"listen", cfg.ListenAddress,\n\t\t\t"backend", backend.Descriptor().Kind,\n\t\t\t"model_mutation", cfg.AllowModelMutation,\n\t\t)''',
)

# Version bump.
write("VERSION", "0.3.0-alpha.2\n")
replace_once("internal/buildinfo/buildinfo.go", 'Version   = "0.3.0-alpha.1"', 'Version   = "0.3.0-alpha.2"')

# Upstream ledger: implemented protocol path but intentionally unpinned until hardware conformance.
ledger_path = Path("internal/upstreamledger/data/ledger.json")
ledger = json.loads(ledger_path.read_text(encoding="utf-8"))
for entry in ledger["entries"]:
    if entry["id"] == "llama-cpp-native-planned":
        entry["id"] = "llama-cpp-direct-adapter-observed-2026-09-05"
        entry["status"] = "observed-unpinned"
        entry["observed_at"] = "2026-09-05T00:00:00Z"
        entry["enabled_capabilities"] = [
            "inference.text",
            "embeddings",
            "streaming.content",
            "placement.cpu",
            "placement.gpu",
            "placement.hybrid",
        ]
        entry["disabled_capabilities"] = [
            "vision, tool calls, reasoning control and structured-output normalization fail closed in the initial direct adapter",
            "context overrides are backend-managed and are not injected per request",
        ]
        entry["license_ref"] = "MIT; upstream copyright 2023-2026 The ggml authors"
        entry["notes"] = "Runtime 0.3.0-alpha.2 implements a direct llama-server HTTP adapter for chat, completions, embeddings and model-read compatibility without Ollama in the request path. The protocol implementation is unit-tested against fixtures, but no upstream tag/commit or hardware/model run is promoted to latest-known-good until the conformance matrix passes."
        break
else:
    raise SystemExit("llama.cpp planned ledger entry not found")
ledger_path.write_text(json.dumps(ledger, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

# README and docs.
replace_once("README.md", "Current version: `0.3.0-alpha.1`", "Current version: `0.3.0-alpha.2`")
replace_once(
    "README.md",
    '''Inference still runs in **Ollama adoption mode**:\n\n```text\nEmber CoreUI / Game / client\n              |\n              v\n      Quantum Runtime :11450\n              |\n              v\n       existing Ollama :11434\n```\n\nQuantum Runtime owns the application-facing endpoint, request policy, authentication boundary, health reporting, timeouts, compatibility allowlist, model-manifest contract and Runtime service lifecycle. It does not yet execute model inference independently.''',
    '''Quantum Runtime now has two local execution paths. Ollama remains the default adoption/fallback mode, while a direct llama.cpp path can talk to `llama-server` without an Ollama daemon in the inference request path:\n\n```text\nEmber CoreUI / Game / client\n              |\n              v\n      Quantum Runtime :11450\n          /           \\\n         v             v\n llama-server :8080   Ollama :11434\n direct local path    adoption/fallback\n```\n\nThe initial llama.cpp adapter deliberately uses the external `llama-server` process boundary rather than vendoring or forking upstream code. Quantum Runtime owns the application-facing endpoint, request policy, authentication boundary, compatibility translation and capability reporting; llama.cpp owns the low-level GGUF inference engine. Ollama remains the default until an operator explicitly selects `llama.cpp`.''',
)
replace_once(
    "README.md",
    '''- Ollama-compatible chat, generation, embedding and model-read routes\n- streamed forwarding, cancellation, body limits and backend timeouts''',
    '''- Ollama-compatible chat, generation, embedding and model-read routes\n- first direct llama.cpp/ggml backend path using `llama-server`, with Ollama absent from the request path when selected\n- Ollama chat/generate/embed compatibility translation to llama.cpp OpenAI-style endpoints\n- fail-closed handling for llama.cpp features not yet normalized: vision, tools, reasoning control and structured output\n- streamed forwarding/translation, cancellation, body limits and backend timeouts''',
)
replace_once(
    "README.md",
    '''## Ember CoreUI adoption\n''',
    '''## Direct llama.cpp backend\n\n`0.3.0-alpha.2` can use an already running `llama-server` directly. The Runtime does not download, replace or manage the llama.cpp binary or GGUF files in this slice. Configure the server with an API-visible alias that matches the model identifier used by the client, for example:\n\n```bash\nllama-server -m /path/model.gguf --host 127.0.0.1 --port 8080 --alias ember-coreui:latest\n```\n\nThen start Quantum Runtime with:\n\n```bash\nQUANTUM_RUNTIME_BACKEND=llama.cpp \\\nQUANTUM_RUNTIME_LLAMA_CPP_URL=http://127.0.0.1:8080 \\\nQUANTUM_RUNTIME_LLAMA_CPP_MODEL=ember-coreui:latest \\\ngo run ./cmd/quantum-runtime\n```\n\nIf the llama.cpp server uses `--api-key`, set `QUANTUM_RUNTIME_LLAMA_CPP_API_KEY` locally. Runtime bearer credentials are never reused as llama.cpp credentials. The initial bridge supports text chat, text generation, embeddings and model-read compatibility. Unsupported modalities/capabilities fail closed instead of being silently dropped.\n\n## Ember CoreUI adoption\n''',
)
replace_once(
    "README.md",
    '''| `QUANTUM_RUNTIME_LISTEN` | `127.0.0.1:11450` | HTTP listen address |\n| `QUANTUM_RUNTIME_OLLAMA_URL` | `http://127.0.0.1:11434` | Initial adoption backend |''',
    '''| `QUANTUM_RUNTIME_LISTEN` | `127.0.0.1:11450` | HTTP listen address |\n| `QUANTUM_RUNTIME_BACKEND` | `ollama` | Active backend: `ollama` or `llama.cpp` |\n| `QUANTUM_RUNTIME_OLLAMA_URL` | `http://127.0.0.1:11434` | Ollama adoption/fallback endpoint |\n| `QUANTUM_RUNTIME_LLAMA_CPP_URL` | `http://127.0.0.1:8080` | Direct llama-server endpoint |\n| `QUANTUM_RUNTIME_LLAMA_CPP_MODEL` | empty | Required model/API alias when `llama.cpp` is selected |\n| `QUANTUM_RUNTIME_LLAMA_CPP_API_KEY` | empty | Optional llama-server API key; never logged |''',
)

# API docs: add direct adapter behavior and fail-closed scope.
api_path = Path("docs/API.md")
api_text = api_path.read_text(encoding="utf-8")
api_text += '''\n\n## llama.cpp direct compatibility bridge (0.3.0-alpha.2)\n\nWhen `QUANTUM_RUNTIME_BACKEND=llama.cpp`, the existing application-facing Ollama compatibility routes are translated to a directly configured `llama-server`; Ollama is not present in that request path.\n\n- `/api/chat` -> `/v1/chat/completions`\n- `/api/generate` -> `/v1/completions`\n- `/api/embed` and `/api/embeddings` -> `/v1/embeddings`\n- `/api/tags`, `/api/show`, `/api/ps` and `/api/version` are synthesized from the configured Runtime model identity because llama.cpp does not expose Ollama's model-store contract\n\nThe configured `QUANTUM_RUNTIME_LLAMA_CPP_MODEL` must match the client-visible model identifier. A mismatched model request returns an error instead of silently substituting another canonical identity. Only `temperature`, `top_p` and `top_k` are normalized from the current Ollama `options` object in this first direct slice. Per-request `num_ctx`, predict/thread/repeat/seed/stop tuning is not injected.\n\nVision, tool calls/tool history, explicit reasoning control and structured-output requests currently return `422 unsupported_backend_capability` on this direct adapter. This is intentional: the backend contract must not claim a capability until Runtime preserves its semantics end to end. Streaming content is translated from llama.cpp SSE to Ollama NDJSON; llama.cpp `reasoning_content`/`reasoning` deltas are preserved as the Ollama-compatible `message.thinking` field when present.\n'''
api_path.write_text(api_text, encoding="utf-8")

arch_path = Path("docs/ARCHITECTURE.md")
arch_text = arch_path.read_text(encoding="utf-8")
arch_text += '''\n\n## Direct portable backend slice (0.3.0-alpha.2)\n\nThe first native portable execution path is a direct process boundary to `llama-server`. "Native" here means Quantum Runtime can execute through the low-level llama.cpp/GGUF engine without Ollama in the request path; it does not mean llama.cpp has been copied into the Go process or vendored into this repository. This keeps upstream licensing/versioning independent and preserves a replaceable adapter boundary.\n\nThe active backend is selected explicitly at Runtime startup. `ollama` remains the default adoption/fallback mode. Selecting `llama.cpp` requires an explicit API model alias, and the adapter refuses requests for a different identity. This alpha does not yet perform automatic live fallback between simultaneous backends; deterministic multi-backend scheduling remains a later 0.3 slice after hardware discovery/calibration and artifact placement are implemented.\n\nThe direct adapter intentionally fails closed for capability shapes that are not normalized end to end yet. A backend having an upstream feature is not enough: Runtime only advertises it when the compatibility bridge can preserve the feature without flattening or silently changing semantics.\n'''
arch_path.write_text(arch_text, encoding="utf-8")

# Third-party notice: external llama.cpp connector, no vendoring.
replace_once(
    "THIRD_PARTY_NOTICES.md",
    '''Quantum Runtime can connect to an existing Ollama service in adoption mode. Ollama is not distributed by this repository and remains subject to its own license and notices.\n\nQuantum Runtime does not currently distribute Gemma, other model weights, datasets, or a third-party inference engine.''',
    '''Quantum Runtime can connect to an existing Ollama service in adoption mode. Ollama is not distributed by this repository and remains subject to its own license and notices.\n\nQuantum Runtime `0.3.0-alpha.2` can also connect directly to an operator-provided `llama-server` from the llama.cpp/ggml project. llama.cpp is not vendored or distributed by this repository in this release. Upstream llama.cpp is MIT-licensed and carries copyright notice `Copyright (c) 2023-2026 The ggml authors`; operators remain responsible for the exact upstream version they install and its accompanying notices.\n\nQuantum Runtime does not currently distribute Gemma, other model weights, datasets, or a third-party inference engine.''',
)

# Changelog entry and update old not-implemented line.
replace_once(
    "CHANGELOG.md",
    "# Changelog\n\n",
    '''# Changelog\n\n## 0.3.0-alpha.2 - 2026-09-05\n\n- Added the first direct llama.cpp/ggml execution adapter using the external `llama-server` process boundary; selecting it removes Ollama from the inference request path without vendoring upstream code.\n- Added explicit `QUANTUM_RUNTIME_BACKEND=ollama|llama.cpp` selection while keeping Ollama as the default adoption/fallback mode.\n- Added direct Ollama-compatibility translation for chat, generation and embeddings using llama.cpp `/v1/chat/completions`, `/v1/completions` and `/v1/embeddings`, plus synthetic model-read compatibility responses.\n- Preserved streaming content by translating llama.cpp SSE to Ollama NDJSON and preserved reasoning text as `message.thinking` when upstream emits `reasoning_content`/`reasoning`.\n- Enforced model-identity matching between `QUANTUM_RUNTIME_LLAMA_CPP_MODEL` and the client-visible request model; no silent model substitution is allowed.\n- Kept the first llama.cpp bridge deliberately fail-closed for vision, tools/tool history, explicit reasoning control and structured output until those semantics are normalized end to end.\n- Kept per-request context/predict/thread/repeat/seed/stop tuning out of the direct bridge; only `temperature`, `top_p` and `top_k` are normalized in this slice.\n- Added optional separate llama-server API-key configuration. Runtime bearer credentials are never forwarded as llama.cpp credentials.\n- Updated the upstream ledger from planned to implemented-but-unpinned protocol evidence. No llama.cpp tag/commit is promoted to latest-known-good until real model/hardware conformance passes.\n\n''',
)
replace_once(
    "CHANGELOG.md",
    '''- independent native inference\n- managed model store''',
    '''- fully managed/bundled native inference engine lifecycle (the first direct llama.cpp process adapter now exists)\n- managed model store''',
)

print("Runtime 0.3.0-alpha.2 llama.cpp direct adapter patch applied")
