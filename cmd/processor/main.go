package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/getalternative/adyen-slack-assistant/internal/adyen"
	"github.com/getalternative/adyen-slack-assistant/internal/approval"
	"github.com/getalternative/adyen-slack-assistant/internal/audit"
	"github.com/getalternative/adyen-slack-assistant/internal/config"
	"github.com/getalternative/adyen-slack-assistant/internal/llm"
	"github.com/getalternative/adyen-slack-assistant/internal/permissions"
	slackClient "github.com/getalternative/adyen-slack-assistant/internal/slack"
)

var (
	cfg         *config.Config
	slack       *slackClient.Client
	llmClient   *llm.Client
	adyenClient *adyen.Client
	permChecker *permissions.Checker
	approvalMgr *approval.Manager
	auditLogger *audit.Logger
)

// QueueMessage is the message format from SQS
type QueueMessage struct {
	Type      string          `json:"type"`
	Event     json.RawMessage `json:"event"`
	BotUserID string          `json:"botUserId"`
}

// MessageEvent represents a Slack message event
type MessageEvent struct {
	Type      string `json:"type"`
	Channel   string `json:"channel"`
	User      string `json:"user"`
	Text      string `json:"text"`
	Ts        string `json:"ts"`
	ThreadTs  string `json:"thread_ts"`
}

// ReactionEvent represents a Slack reaction event
type ReactionEvent struct {
	Type     string `json:"type"`
	User     string `json:"user"`
	Reaction string `json:"reaction"`
	Item     struct {
		Type    string `json:"type"`
		Channel string `json:"channel"`
		Ts      string `json:"ts"`
	} `json:"item"`
}

func init() {
	cfg = config.Load()
	slack = slackClient.New(cfg)
	llmClient = llm.New(cfg)
	permChecker = permissions.New(cfg, slack)
	auditLogger = audit.New(cfg, slack)

	var err error
	adyenClient, err = adyen.New(cfg)
	if err != nil {
		panic(fmt.Sprintf("failed to create Adyen client: %v", err))
	}

	approvalMgr, err = approval.New(cfg, slack)
	if err != nil {
		panic(fmt.Sprintf("failed to create approval manager: %v", err))
	}
}

func handler(ctx context.Context, sqsEvent events.SQSEvent) error {
	// Start Adyen MCP server for this invocation
	if err := adyenClient.Start(ctx); err != nil {
		return fmt.Errorf("failed to start Adyen MCP: %w", err)
	}
	defer adyenClient.Stop()

	for _, record := range sqsEvent.Records {
		var queueMsg QueueMessage
		if err := json.Unmarshal([]byte(record.Body), &queueMsg); err != nil {
			fmt.Printf("Failed to parse queue message: %v\n", err)
			continue
		}

		switch queueMsg.Type {
		case "app_mention", "message":
			if err := handleMessage(ctx, queueMsg); err != nil {
				fmt.Printf("Failed to handle message: %v\n", err)
			}
		case "reaction_added":
			if err := handleReaction(ctx, queueMsg); err != nil {
				fmt.Printf("Failed to handle reaction: %v\n", err)
			}
		}
	}

	return nil
}

func handleMessage(ctx context.Context, queueMsg QueueMessage) error {
	var event MessageEvent
	if err := json.Unmarshal(queueMsg.Event, &event); err != nil {
		return fmt.Errorf("failed to parse message event: %w", err)
	}

	// Remove bot mention from text
	text := strings.TrimSpace(event.Text)
	if queueMsg.BotUserID != "" {
		text = strings.ReplaceAll(text, fmt.Sprintf("<@%s>", queueMsg.BotUserID), "")
		text = strings.TrimSpace(text)
	}

	// Create message object for replies
	msg := &slackClient.Message{
		Channel:  event.Channel,
		User:     event.User,
		Text:     text,
		Ts:       event.Ts,
		ThreadTs: event.ThreadTs,
	}

	// Get available tools from Adyen MCP
	tools := adyenClient.GetTools()

	// Process with LLM
	response, err := llmClient.ProcessMessage(ctx, text, tools, nil)
	if err != nil {
		slack.Reply(msg, fmt.Sprintf("Sorry, I encountered an error: %s", err.Error()))
		return err
	}

	// If no tool calls, just reply with the text
	if len(response.ToolCalls) == 0 {
		return slack.Reply(msg, response.Text)
	}

	// Process tool calls
	for _, toolCall := range response.ToolCalls {
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			continue
		}

		// Extract amount if present (for permission checks)
		amount := extractAmount(args)

		// Check permissions
		permResult := permChecker.Check(event.User, event.Channel, toolCall.Function.Name, amount)

		if !permResult.Allowed {
			auditLogger.LogDenied(event.User, toolCall.Function.Name, event.Channel, permResult.Reason)
			return slack.Reply(msg, fmt.Sprintf("Permission denied: %s", permResult.Reason))
		}

		if permResult.NeedsApproval {
			// Request approval
			err := approvalMgr.RequestApproval(ctx, msg, toolCall.Function.Name, args, amount, permResult.Approvers)
			if err != nil {
				return slack.Reply(msg, fmt.Sprintf("Failed to request approval: %s", err.Error()))
			}
			return nil // Wait for approval via reaction
		}

		// Execute the tool
		result, err := adyenClient.CallTool(ctx, toolCall.Function.Name, args)
		if err != nil {
			auditLogger.LogError(event.User, toolCall.Function.Name, event.Channel, err.Error())
			return slack.Reply(msg, fmt.Sprintf("Tool execution failed: %s", err.Error()))
		}

		// Log success and reply
		auditLogger.LogAllowed(event.User, toolCall.Function.Name, event.Channel, "Executed successfully")

		// Format the result nicely
		replyText := formatToolResult(toolCall.Function.Name, result)
		return slack.Reply(msg, replyText)
	}

	return nil
}

func handleReaction(ctx context.Context, queueMsg QueueMessage) error {
	var event ReactionEvent
	if err := json.Unmarshal(queueMsg.Event, &event); err != nil {
		return fmt.Errorf("failed to parse reaction event: %w", err)
	}

	// Process the reaction through approval manager
	req, decision, err := approvalMgr.HandleReaction(ctx, event.Reaction, event.User, event.Item.Channel, event.Item.Ts)
	if err != nil {
		return err
	}

	if req == nil {
		return nil // Not a pending approval
	}

	msg := &slackClient.Message{
		Channel:  req.Channel,
		ThreadTs: req.ThreadTs,
	}

	if decision == "rejected" {
		auditLogger.LogRejected(req.RequestedBy, req.Action, req.Channel, event.User)
		return slack.Reply(msg, fmt.Sprintf("Request rejected by <@%s>", event.User))
	}

	// Approved - execute the action
	auditLogger.LogApproved(req.RequestedBy, req.Action, req.Channel, event.User, "Approval granted")

	slack.Reply(msg, fmt.Sprintf("Approved by <@%s>. Processing...", event.User))

	// Execute the tool
	result, err := adyenClient.CallTool(ctx, req.Action, req.Params)
	if err != nil {
		auditLogger.LogError(req.RequestedBy, req.Action, req.Channel, err.Error())
		return slack.Reply(msg, fmt.Sprintf("Execution failed: %s", err.Error()))
	}

	replyText := formatToolResult(req.Action, result)
	return slack.Reply(msg, replyText)
}

func extractAmount(args map[string]interface{}) int {
	// Try common amount field patterns
	if amount, ok := args["amount"].(map[string]interface{}); ok {
		if value, ok := amount["value"].(float64); ok {
			return int(value)
		}
	}
	if amount, ok := args["amount"].(float64); ok {
		return int(amount)
	}
	return 0
}

func formatToolResult(toolName string, result string) string {
	// Add some formatting based on tool type
	prefix := ":white_check_mark: "

	if strings.Contains(toolName, "refund") {
		prefix = ":money_with_wings: *Refund processed*\n"
	} else if strings.Contains(toolName, "cancel") {
		prefix = ":no_entry_sign: *Payment cancelled*\n"
	} else if strings.Contains(toolName, "create") {
		prefix = ":link: *Created successfully*\n"
	} else if strings.Contains(toolName, "get") || strings.Contains(toolName, "list") {
		prefix = ":mag: "
	}

	return prefix + "```\n" + result + "\n```"
}

func main() {
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		lambda.Start(handler)
	} else {
		fmt.Println("Running locally - use serverless offline or deploy to AWS")
	}
}
