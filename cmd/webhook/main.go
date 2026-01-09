package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/getalternative/adyen-slack-assistant/internal/config"
)

var (
	sqsClient *sqs.Client
	cfg       *config.Config
)

// SlackEvent represents a Slack event callback
type SlackEvent struct {
	Token       string          `json:"token"`
	Challenge   string          `json:"challenge"`
	Type        string          `json:"type"`
	TeamID      string          `json:"team_id"`
	Event       json.RawMessage `json:"event"`
	EventID     string          `json:"event_id"`
	EventTime   int64           `json:"event_time"`
	Authorizations []struct {
		UserID string `json:"user_id"`
	} `json:"authorizations"`
}

// MessageEvent represents a Slack message event
type MessageEvent struct {
	Type      string `json:"type"`
	Channel   string `json:"channel"`
	User      string `json:"user"`
	Text      string `json:"text"`
	Ts        string `json:"ts"`
	ThreadTs  string `json:"thread_ts"`
	BotID     string `json:"bot_id"`
	EventTs   string `json:"event_ts"`
	ChannelType string `json:"channel_type"`
}

// ReactionEvent represents a Slack reaction event
type ReactionEvent struct {
	Type     string `json:"type"`
	User     string `json:"user"`
	Reaction string `json:"reaction"`
	ItemUser string `json:"item_user"`
	Item     struct {
		Type    string `json:"type"`
		Channel string `json:"channel"`
		Ts      string `json:"ts"`
	} `json:"item"`
	EventTs string `json:"event_ts"`
}

// QueueMessage is the message format sent to SQS
type QueueMessage struct {
	Type      string          `json:"type"` // message, reaction_added
	Event     json.RawMessage `json:"event"`
	BotUserID string          `json:"botUserId"`
}

func init() {
	cfg = config.Load()

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.AWS.Region),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to load AWS config: %v", err))
	}

	sqsClient = sqs.NewFromConfig(awsCfg)
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Verify Slack signature
	if !verifySlackSignature(request) {
		return response(401, `{"error": "invalid signature"}`)
	}

	// Parse the event
	var slackEvent SlackEvent
	if err := json.Unmarshal([]byte(request.Body), &slackEvent); err != nil {
		return response(400, `{"error": "invalid request body"}`)
	}

	// Handle URL verification challenge
	if slackEvent.Type == "url_verification" {
		return response(200, fmt.Sprintf(`{"challenge": "%s"}`, slackEvent.Challenge))
	}

	// Handle event callbacks
	if slackEvent.Type == "event_callback" {
		// Get bot user ID
		botUserID := ""
		if len(slackEvent.Authorizations) > 0 {
			botUserID = slackEvent.Authorizations[0].UserID
		}

		// Determine event type
		var eventType struct {
			Type string `json:"type"`
		}
		json.Unmarshal(slackEvent.Event, &eventType)

		// Skip if it's a bot message
		var msgEvent MessageEvent
		json.Unmarshal(slackEvent.Event, &msgEvent)
		if msgEvent.BotID != "" {
			return response(200, `{"ok": true}`)
		}

		// Only process app_mention, message (DM), and reaction_added events
		validEvents := map[string]bool{
			"app_mention":    true,
			"message":        true,
			"reaction_added": true,
		}

		if !validEvents[eventType.Type] {
			return response(200, `{"ok": true}`)
		}

		// For message events, only process DMs (not channel messages without mention)
		if eventType.Type == "message" && msgEvent.ChannelType != "im" {
			return response(200, `{"ok": true}`)
		}

		// Queue the event for processing
		queueMsg := QueueMessage{
			Type:      eventType.Type,
			Event:     slackEvent.Event,
			BotUserID: botUserID,
		}

		if err := queueEvent(ctx, queueMsg); err != nil {
			fmt.Printf("Failed to queue event: %v\n", err)
			return response(500, `{"error": "failed to queue event"}`)
		}
	}

	// Return 200 immediately to acknowledge receipt
	return response(200, `{"ok": true}`)
}

func verifySlackSignature(request events.APIGatewayProxyRequest) bool {
	signingSecret := cfg.Slack.SigningSecret
	if signingSecret == "" {
		return true // Skip verification if not configured (dev mode)
	}

	timestamp := request.Headers["X-Slack-Request-Timestamp"]
	signature := request.Headers["X-Slack-Signature"]

	// Check timestamp is within 5 minutes
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix()-ts > 300 {
		return false
	}

	// Calculate expected signature
	baseString := fmt.Sprintf("v0:%s:%s", timestamp, request.Body)
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(baseString))
	expectedSig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSig))
}

func queueEvent(ctx context.Context, msg QueueMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    &cfg.AWS.SQSQueueURL,
		MessageBody: stringPtr(string(body)),
	})
	return err
}

func response(statusCode int, body string) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: body,
	}, nil
}

func stringPtr(s string) *string {
	return &s
}

func main() {
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		lambda.Start(handler)
	} else {
		// Local testing
		fmt.Println("Running locally - use serverless offline or deploy to AWS")
	}
}
