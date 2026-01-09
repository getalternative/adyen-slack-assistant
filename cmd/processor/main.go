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
	Type     string `json:"type"`
	Channel  string `json:"channel"`
	User     string `json:"user"`
	Text     string `json:"text"`
	Ts       string `json:"ts"`
	ThreadTs string `json:"thread_ts"`
}

func init() {
	cfg = config.Load()
	slack = slackClient.New(cfg)
	llmClient = llm.New(cfg)
	permChecker = permissions.New(cfg)
	auditLogger = audit.New(cfg, slack)

	var err error
	adyenClient, err = adyen.New(cfg)
	if err != nil {
		panic(fmt.Sprintf("failed to create Adyen client: %v", err))
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

		if queueMsg.Type == "app_mention" || queueMsg.Type == "message" {
			if err := handleMessage(ctx, queueMsg); err != nil {
				fmt.Printf("Failed to handle message: %v\n", err)
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
		args := toolCall.Input

		// Check permissions (admins can write, others read-only)
		permResult := permChecker.Check(event.User, event.Channel, toolCall.Name)
		if !permResult.Allowed {
			auditLogger.LogDenied(event.User, toolCall.Name, event.Channel, permResult.Reason)
			return slack.Reply(msg, permResult.Reason)
		}

		// Execute the tool
		result, err := adyenClient.CallTool(ctx, toolCall.Name, args)
		if err != nil {
			auditLogger.LogError(event.User, toolCall.Name, event.Channel, err.Error())
			return slack.Reply(msg, fmt.Sprintf("Error: %s", err.Error()))
		}

		// Log and reply
		auditLogger.LogAllowed(event.User, toolCall.Name, event.Channel, "OK")
		return slack.Reply(msg, formatResult(toolCall.Name, result))
	}

	return nil
}

func formatResult(toolName string, result string) string {
	prefix := ""
	if strings.Contains(toolName, "refund") {
		prefix = "*Refund processed*\n"
	} else if strings.Contains(toolName, "cancel") {
		prefix = "*Payment cancelled*\n"
	} else if strings.Contains(toolName, "create") {
		prefix = "*Created*\n"
	}
	return prefix + "```\n" + result + "\n```"
}

func main() {
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		lambda.Start(handler)
	} else {
		fmt.Println("Run with: doppler run -- go run ./cmd/processor")
	}
}
