// Package local provides a client for self-hosted, OpenAI-compatible LLM
// servers such as vLLM, llama.cpp (llama-server), Ollama, and LM Studio.
// These expose the same /chat/completions surface as OpenAI (streaming, tool
// calls, JSON mode), so the request/response shapes mirror the OpenAI client.
// The two differences from a cloud provider are that the base URL is supplied
// by the user (their machine's host:port) and that the API key is optional —
// most local servers run without auth, so the Authorization header is only set
// when a key is configured.
package local

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	chatprovider "codedone/internal/provider/chat"
	"codedone/internal/provider/providererror"
)

// defaultBaseURL points at vLLM's default port; used only when the user leaves
// the base URL blank.
const defaultBaseURL = "http://localhost:8000/v1"

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type chatRequest struct {
	Model               string                        `json:"model"`
	Messages            []chatprovider.Message        `json:"messages"`
	Tools               []chatprovider.ToolDefinition `json:"tools,omitempty"`
	ToolChoice          any                           `json:"tool_choice,omitempty"`
	Stream              bool                          `json:"stream"`
	MaxCompletionTokens int                           `json:"max_completion_tokens,omitempty"`
	Temperature         *float64                      `json:"temperature,omitempty"`
	ReasoningEffort     string                        `json:"reasoning_effort,omitempty"`
}

func NewClient(baseURL, apiKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(apiKey),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) ChatCompletion(ctx context.Context, req chatprovider.ChatRequest) (*chatprovider.ChatResponse, error) {
	body, err := json.Marshal(localRequest(req))
	if err != nil {
		return nil, fmt.Errorf("marshal local request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create local request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, providererror.New("Local", providererror.KindNetwork, "Could not reach the local server at "+c.baseURL+". Make sure it is running and the Base URL in Settings is correct.").WithDetail(err.Error())
	}
	defer resp.Body.Close()

	var decoded chatprovider.ChatResponse
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, providererror.FromHTTP("Local", resp.StatusCode, readErrorBody(resp.Body))
	}

	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode local response: %w", err)
	}

	return &decoded, nil
}

func localRequest(req chatprovider.ChatRequest) chatRequest {
	out := chatRequest{
		Model:               strings.TrimSpace(req.Model),
		Messages:            req.Messages,
		Tools:               req.Tools,
		ToolChoice:          req.ToolChoice,
		Stream:              req.Stream,
		MaxCompletionTokens: req.MaxTokens,
		ReasoningEffort:     strings.TrimSpace(req.ReasoningEffort),
	}
	if req.Temperature > 0 {
		temp := req.Temperature
		out.Temperature = &temp
	}
	return out
}

func readErrorBody(r io.Reader) string {
	data, err := io.ReadAll(io.LimitReader(r, 16*1024))
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return ""
	}
	var body struct {
		Error any `json:"error"`
	}
	if err := json.Unmarshal(data, &body); err == nil && body.Error != nil {
		switch v := body.Error.(type) {
		case string:
			return strings.TrimSpace(v)
		case map[string]any:
			if msg, ok := v["message"].(string); ok && strings.TrimSpace(msg) != "" {
				return strings.TrimSpace(msg)
			}
			if code, ok := v["code"].(string); ok && strings.TrimSpace(code) != "" {
				return strings.TrimSpace(code)
			}
		}
	}
	return raw
}
