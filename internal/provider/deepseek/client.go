package deepseek

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

const defaultBaseURL = "https://api.deepseek.com"

type Client struct {
	baseURL      string
	apiKey       string
	providerName string
	httpClient   *http.Client
}

func NewClient(apiKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &Client{
		baseURL:      defaultBaseURL,
		apiKey:       strings.TrimSpace(apiKey),
		providerName: "DeepSeek",
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) ChatCompletion(ctx context.Context, req chatprovider.ChatRequest) (*chatprovider.ChatResponse, error) {
	if c.apiKey == "" {
		return nil, providererror.New(c.provider(), providererror.KindAuthentication, c.provider()+" API key is missing. Add it in Settings before starting a session.")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal deepseek request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create deepseek request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, providererror.New(c.provider(), providererror.KindNetwork, "Could not reach "+c.provider()+". Check your connection and try again.").WithDetail(err.Error())
	}
	defer resp.Body.Close()

	var decoded chatprovider.ChatResponse
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := readErrorBody(resp.Body)
		return nil, providererror.FromHTTP(c.provider(), resp.StatusCode, detail)
	}

	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode deepseek response: %w", err)
	}

	return &decoded, nil
}

func (c *Client) provider() string {
	if strings.TrimSpace(c.providerName) == "" {
		return "Provider"
	}
	return strings.TrimSpace(c.providerName)
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
