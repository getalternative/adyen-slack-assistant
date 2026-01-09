package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/getalternative/adyen-slack-assistant/internal/config"
)

const anthropicAPIURL = "https://api.anthropic.com/v1/messages"

// Client handles LLM interactions via Anthropic API
type Client struct {
	cfg        *config.Config
	httpClient *http.Client
}

// New creates a new LLM client
func New(cfg *config.Config) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{},
	}
}

// Tool represents an available tool/function
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ToolCall represents a tool call from the LLM
type ToolCall struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// Message represents a chat message
type Message struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a content block in a message
type ContentBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Content   string                 `json:"content,omitempty"`
}

// AnthropicRequest represents a request to Anthropic API
type AnthropicRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
	Tools     []Tool    `json:"tools,omitempty"`
}

// AnthropicResponse represents a response from Anthropic API
type AnthropicResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	StopReason   string         `json:"stop_reason"`
	StopSequence string         `json:"stop_sequence,omitempty"`
}

// Response from ProcessMessage
type Response struct {
	Text      string
	ToolCalls []ToolCall
}

// ProcessMessage sends a message to the LLM and returns the response
func (c *Client) ProcessMessage(ctx context.Context, userMessage string, tools []Tool, conversationHistory []Message) (*Response, error) {
	// Build messages
	messages := append(conversationHistory, Message{
		Role: "user",
		Content: []ContentBlock{
			{Type: "text", Text: userMessage},
		},
	})

	systemPrompt := `You are a helpful assistant that helps with Adyen payment operations.
You have access to Adyen tools for:
- Checking payment status
- Creating payment links
- Processing refunds
- Canceling payments
- Managing terminals
- Viewing webhook configurations

When users ask about payments, use the appropriate tool.
Be concise and helpful. Always confirm actions before executing them.
For destructive actions (refunds, cancellations), clearly state what will happen.`

	reqBody := AnthropicRequest{
		Model:     c.cfg.LLM.Model,
		MaxTokens: 1024,
		System:    systemPrompt,
		Messages:  messages,
		Tools:     tools,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.cfg.LLM.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var anthropicResp AnthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Parse response
	response := &Response{}
	for _, block := range anthropicResp.Content {
		switch block.Type {
		case "text":
			response.Text += block.Text
		case "tool_use":
			response.ToolCalls = append(response.ToolCalls, ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}

	return response, nil
}

// ConvertToolsFromMCP converts MCP tools to Anthropic format
func ConvertToolsFromMCP(mcpTools []Tool) []Tool {
	// Anthropic uses the same format, just ensure input_schema is set
	return mcpTools
}
