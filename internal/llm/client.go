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

// Client handles LLM interactions
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
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a function
type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCall represents a tool call from the LLM
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Message represents a chat message
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ChatRequest represents an OpenAI chat completion request
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
}

// ChatResponse represents an OpenAI chat completion response
type ChatResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
}

// Response from ProcessMessage
type Response struct {
	Text      string
	ToolCalls []ToolCall
}

// ProcessMessage sends a message to the LLM and returns the response
func (c *Client) ProcessMessage(ctx context.Context, userMessage string, tools []Tool, conversationHistory []Message) (*Response, error) {
	messages := append(conversationHistory, Message{
		Role:    "user",
		Content: userMessage,
	})

	// Add system prompt
	systemPrompt := Message{
		Role: "system",
		Content: `You are a helpful assistant that helps with Adyen payment operations.
You have access to Adyen tools for:
- Checking payment status
- Creating payment links
- Processing refunds
- Canceling payments
- Managing terminals
- Viewing webhook configurations

When users ask about payments, use the appropriate tool.
Be concise and helpful. Always confirm actions before executing them.
For destructive actions (refunds, cancellations), clearly state what will happen.`,
	}
	messages = append([]Message{systemPrompt}, messages...)

	reqBody := ChatRequest{
		Model:    c.cfg.LLM.Model,
		Messages: messages,
		Tools:    tools,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.LLM.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LLM API error (status %d): %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	choice := chatResp.Choices[0]
	return &Response{
		Text:      choice.Message.Content,
		ToolCalls: choice.Message.ToolCalls,
	}, nil
}
