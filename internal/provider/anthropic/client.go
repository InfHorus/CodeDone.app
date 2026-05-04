package anthropic

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

const defaultBaseURL = "https://api.anthropic.com"
const anthropicVersion = "2023-06-01"

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
		return nil, providererror.New("Anthropic", providererror.KindAuthentication, "Anthropic API key is missing. Add it in Settings before starting a session.")
	}
	body, err := buildAnthropicRequest(req)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create anthropic request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, providererror.New("Anthropic", providererror.KindNetwork, "Could not reach Anthropic. Check your connection and try again.").WithDetail(err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, providererror.FromHTTP("Anthropic", resp.StatusCode, readErrorBody(resp.Body))
	}

	var decoded anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode anthropic response: %w", err)
	}
	return convertAnthropicResponse(decoded), nil
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	ToolChoice  any                `json:"tool_choice,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

type anthropicBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthropicResponse struct {
	ID           string           `json:"id"`
	Model        string           `json:"model"`
	Content      []anthropicBlock `json:"content"`
	StopReason   string           `json:"stop_reason"`
	StopSequence string           `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func buildAnthropicRequest(req chatprovider.ChatRequest) (anthropicRequest, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	out := anthropicRequest{
		Model:     strings.TrimSpace(req.Model),
		MaxTokens: maxTokens,
		Messages:  make([]anthropicMessage, 0, len(req.Messages)),
	}
	if req.Temperature > 0 {
		temp := req.Temperature
		out.Temperature = &temp
	}
	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			text := contentToString(msg.Content)
			if text != "" {
				if out.System != "" {
					out.System += "\n\n"
				}
				out.System += text
			}
		case "assistant":
			out.Messages = append(out.Messages, anthropicMessage{Role: "assistant", Content: assistantContent(msg)})
		case "tool":
			out.Messages = append(out.Messages, anthropicMessage{Role: "user", Content: []anthropicBlock{{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   contentToString(msg.Content),
			}}})
		default:
			out.Messages = append(out.Messages, anthropicMessage{Role: "user", Content: contentToString(msg.Content)})
		}
	}
	for _, tool := range req.Tools {
		out.Tools = append(out.Tools, anthropicTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}
	if len(out.Tools) > 0 && req.ToolChoice != nil {
		if choice, ok := req.ToolChoice.(string); ok && choice == "auto" {
			out.ToolChoice = map[string]string{"type": "auto"}
		}
	}
	if out.Model == "" {
		return out, fmt.Errorf("anthropic model is empty")
	}
	return out, nil
}

func assistantContent(msg chatprovider.Message) any {
	blocks := []anthropicBlock{}
	if text := contentToString(msg.Content); text != "" {
		blocks = append(blocks, anthropicBlock{Type: "text", Text: text})
	}
	for _, call := range msg.ToolCalls {
		input := json.RawMessage([]byte(call.Function.Arguments))
		if !json.Valid(input) {
			input = json.RawMessage(`{}`)
		}
		id := call.ID
		if id == "" {
			id = "toolu_" + call.Function.Name
		}
		blocks = append(blocks, anthropicBlock{
			Type:  "tool_use",
			ID:    id,
			Name:  call.Function.Name,
			Input: input,
		})
	}
	if len(blocks) == 0 {
		return ""
	}
	return blocks
}

func convertAnthropicResponse(resp anthropicResponse) *chatprovider.ChatResponse {
	textParts := []string{}
	calls := []chatprovider.ToolCall{}
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				textParts = append(textParts, block.Text)
			}
		case "tool_use":
			args := string(block.Input)
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			calls = append(calls, chatprovider.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: chatprovider.ToolFunctionCall{
					Name:      block.Name,
					Arguments: args,
				},
			})
		}
	}
	out := &chatprovider.ChatResponse{
		ID:    resp.ID,
		Model: resp.Model,
	}
	out.Choices = append(out.Choices, struct {
		Index   int `json:"index"`
		Message struct {
			Role      string                  `json:"role"`
			Content   any                     `json:"content"`
			ToolCalls []chatprovider.ToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	}{})
	out.Choices[0].Index = 0
	out.Choices[0].Message.Role = "assistant"
	out.Choices[0].Message.Content = strings.Join(textParts, "\n")
	out.Choices[0].Message.ToolCalls = calls
	out.Choices[0].FinishReason = resp.StopReason
	out.Usage.PromptTokens = resp.Usage.InputTokens
	out.Usage.CompletionTokens = resp.Usage.OutputTokens
	out.Usage.TotalTokens = resp.Usage.InputTokens + resp.Usage.OutputTokens
	return out
}

func contentToString(content any) string {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case nil:
		return ""
	default:
		data, _ := json.Marshal(v)
		return strings.TrimSpace(string(data))
	}
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
			if typ, ok := v["type"].(string); ok && strings.TrimSpace(typ) != "" {
				return strings.TrimSpace(typ)
			}
		}
	}
	return raw
}
