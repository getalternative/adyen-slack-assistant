package adyen

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/getalternative/adyen-slack-assistant/internal/config"
	"github.com/getalternative/adyen-slack-assistant/internal/llm"
)

// Client wraps the Adyen MCP server
type Client struct {
	cfg       *config.Config
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	mu        sync.Mutex
	requestID int64
	tools     []llm.Tool
}

// MCPRequest represents a JSON-RPC request to MCP
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// MCPResponse represents a JSON-RPC response from MCP
type MCPResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *MCPError       `json:"error,omitempty"`
}

// MCPError represents an error from MCP
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ToolsResult represents the result of listing tools
type ToolsResult struct {
	Tools []MCPTool `json:"tools"`
}

// MCPTool represents a tool from MCP
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// CallToolParams represents parameters for calling a tool
type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// CallToolResult represents the result of a tool call
type CallToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock represents a content block in the result
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// New creates a new Adyen MCP client
func New(cfg *config.Config) (*Client, error) {
	return &Client{cfg: cfg}, nil
}

// Start starts the MCP server process
func (c *Client) Start(ctx context.Context) error {
	args := []string{
		"-y", "@adyen/mcp",
		"--adyenApiKey=" + c.cfg.Adyen.APIKey,
		"--env=" + c.cfg.Adyen.Environment,
	}

	if c.cfg.Adyen.Environment == "LIVE" && c.cfg.Adyen.LivePrefix != "" {
		args = append(args, "--livePrefix="+c.cfg.Adyen.LivePrefix)
	}

	c.cmd = exec.CommandContext(ctx, "npx", args...)

	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	c.stdin = stdin

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	c.stdout = bufio.NewReader(stdout)

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start MCP server: %w", err)
	}

	// Initialize the connection
	if err := c.initialize(); err != nil {
		c.Stop()
		return fmt.Errorf("failed to initialize MCP: %w", err)
	}

	// Load available tools
	if err := c.loadTools(); err != nil {
		c.Stop()
		return fmt.Errorf("failed to load tools: %w", err)
	}

	return nil
}

// Stop stops the MCP server process
func (c *Client) Stop() error {
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}

// GetTools returns the available tools in LLM format
func (c *Client) GetTools() []llm.Tool {
	return c.tools
}

// CallTool calls an Adyen MCP tool
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      atomic.AddInt64(&c.requestID, 1),
		Method:  "tools/call",
		Params: CallToolParams{
			Name:      name,
			Arguments: arguments,
		},
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return "", err
	}

	var result CallToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("failed to parse tool result: %w", err)
	}

	if result.IsError {
		if len(result.Content) > 0 {
			return "", fmt.Errorf("tool error: %s", result.Content[0].Text)
		}
		return "", fmt.Errorf("tool execution failed")
	}

	// Combine all text content
	var text string
	for _, block := range result.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	return text, nil
}

// initialize sends the initialize request to MCP
func (c *Client) initialize() error {
	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      atomic.AddInt64(&c.requestID, 1),
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "adyen-slack-assistant",
				"version": "1.0.0",
			},
		},
	}

	_, err := c.sendRequest(req)
	return err
}

// loadTools fetches and converts available tools
func (c *Client) loadTools() error {
	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      atomic.AddInt64(&c.requestID, 1),
		Method:  "tools/list",
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return err
	}

	var result ToolsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("failed to parse tools: %w", err)
	}

	// Convert to LLM tool format
	c.tools = make([]llm.Tool, len(result.Tools))
	for i, tool := range result.Tools {
		c.tools[i] = llm.Tool{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		}
	}

	return nil
}

// sendRequest sends a JSON-RPC request and waits for response
func (c *Client) sendRequest(req MCPRequest) (*MCPResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Write request with newline delimiter
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response line
	line, err := c.stdout.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var resp MCPResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp, nil
}
