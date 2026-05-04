package openrouter

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

const defaultBaseURL = "https://openrouter.ai/api/v1"

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &Client{
		baseURL: defaultBaseURL,
		apiKey:  strings.TrimSpace(apiKey),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) ChatCompletion(ctx context.Context, req chatprovider.ChatRequest) (*chatprovider.ChatResponse, error) {
	if c.apiKey == "" {
		return nil, providererror.New("OpenRouter", providererror.KindAuthentication, "OpenRouter API key is missing. Add it in Settings before starting a session.")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal openrouter request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create openrouter request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, providererror.New("OpenRouter", providererror.KindNetwork, "Could not reach OpenRouter. Check your connection and try again.").WithDetail(err.Error())
	}
	defer resp.Body.Close()

	var decoded chatprovider.ChatResponse
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := readErrorBody(resp.Body)
		return nil, providererror.FromHTTP("OpenRouter", resp.StatusCode, detail)
	}

	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode openrouter response: %w", err)
	}

	return &decoded, nil
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
