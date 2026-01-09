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
	Token     string          `json:"token"`
	Challenge string          `json:"challenge"`
	Type      string          `json:"type"`
	Event     json.RawMessage `json:"event"`
	Authorizations []struct {
		UserID string `json:"user_id"`
	} `json:"authorizations"`
}

// MessageEvent for filtering
type MessageEvent struct {
	Type        string `json:"type"`
	BotID       string `json:"bot_id"`
	ChannelType string `json:"channel_type"`
}

// QueueMessage is sent to SQS
type QueueMessage struct {
	Type      string          `json:"type"`
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

	var slackEvent SlackEvent
	if err := json.Unmarshal([]byte(request.Body), &slackEvent); err != nil {
		return response(400, `{"error": "invalid request"}`)
	}

	// URL verification challenge
	if slackEvent.Type == "url_verification" {
		return response(200, fmt.Sprintf(`{"challenge": "%s"}`, slackEvent.Challenge))
	}

	// Handle events
	if slackEvent.Type == "event_callback" {
		var msgEvent MessageEvent
		json.Unmarshal(slackEvent.Event, &msgEvent)

		// Skip bot messages
		if msgEvent.BotID != "" {
			return response(200, `{"ok": true}`)
		}

		// Only process app_mention and DMs
		if msgEvent.Type != "app_mention" && (msgEvent.Type != "message" || msgEvent.ChannelType != "im") {
			return response(200, `{"ok": true}`)
		}

		// Get bot user ID
		botUserID := ""
		if len(slackEvent.Authorizations) > 0 {
			botUserID = slackEvent.Authorizations[0].UserID
		}

		// Queue for processing
		queueMsg := QueueMessage{
			Type:      msgEvent.Type,
			Event:     slackEvent.Event,
			BotUserID: botUserID,
		}

		if err := queueEvent(ctx, queueMsg); err != nil {
			return response(500, `{"error": "queue failed"}`)
		}
	}

	return response(200, `{"ok": true}`)
}

func verifySlackSignature(request events.APIGatewayProxyRequest) bool {
	secret := cfg.Slack.SigningSecret
	if secret == "" {
		return true
	}

	timestamp := request.Headers["X-Slack-Request-Timestamp"]
	signature := request.Headers["X-Slack-Signature"]

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Now().Unix()-ts > 300 {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("v0:%s:%s", timestamp, request.Body)))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected))
}

func queueEvent(ctx context.Context, msg QueueMessage) error {
	body, _ := json.Marshal(msg)
	_, err := sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    &cfg.AWS.SQSQueueURL,
		MessageBody: stringPtr(string(body)),
	})
	return err
}

func response(code int, body string) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{
		StatusCode: code,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       body,
	}, nil
}

func stringPtr(s string) *string { return &s }

func main() {
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		lambda.Start(handler)
	} else {
		fmt.Println("Deploy to AWS Lambda")
	}
}
