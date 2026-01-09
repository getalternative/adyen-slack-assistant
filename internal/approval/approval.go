package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/getalternative/adyen-slack-assistant/internal/config"
	slackClient "github.com/getalternative/adyen-slack-assistant/internal/slack"
)

const (
	ApprovalTTL = 15 * time.Minute
)

// Request represents a pending approval
type Request struct {
	ID           string                 `json:"id"`          // Message timestamp used as ID
	Channel      string                 `json:"channel"`
	ThreadTs     string                 `json:"threadTs"`
	RequestedBy  string                 `json:"requestedBy"`
	Action       string                 `json:"action"`
	Params       map[string]interface{} `json:"params"`
	Amount       int                    `json:"amount"`
	ExpiresAt    int64                  `json:"expiresAt"`
	Approvers    []string               `json:"approvers"`
}

// Manager handles approval workflows
type Manager struct {
	cfg      *config.Config
	slack    *slackClient.Client
	dynamodb *dynamodb.Client
}

// New creates a new approval manager
func New(cfg *config.Config, slack *slackClient.Client) (*Manager, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.AWS.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &Manager{
		cfg:      cfg,
		slack:    slack,
		dynamodb: dynamodb.NewFromConfig(awsCfg),
	}, nil
}

// RequestApproval creates a new approval request and notifies approvers
func (m *Manager) RequestApproval(ctx context.Context, msg *slackClient.Message, action string, params map[string]interface{}, amount int, approvers []string) error {
	// Send approval message in thread
	approverMentions := make([]string, len(approvers))
	for i, a := range approvers {
		approverMentions[i] = fmt.Sprintf("<@%s>", a)
	}

	text := fmt.Sprintf("*Approval Required*\n\n"+
		"*Action:* `%s`\n"+
		"*Amount:* %s\n"+
		"*Requested by:* <@%s>\n\n"+
		"React with :white_check_mark: to approve or :x: to reject\n"+
		"Waiting for: %s\n"+
		"_Expires in 15 minutes_",
		action,
		formatAmount(amount),
		msg.User,
		strings.Join(approverMentions, ", "),
	)

	ts, err := m.slack.PostToChannel(msg.Channel, msg.GetThreadTs(), text)
	if err != nil {
		return fmt.Errorf("failed to post approval message: %w", err)
	}

	// Store pending approval in DynamoDB
	req := Request{
		ID:          ts,
		Channel:     msg.Channel,
		ThreadTs:    msg.GetThreadTs(),
		RequestedBy: msg.User,
		Action:      action,
		Params:      params,
		Amount:      amount,
		ExpiresAt:   time.Now().Add(ApprovalTTL).Unix(),
		Approvers:   approvers,
	}

	if err := m.store(ctx, req); err != nil {
		return fmt.Errorf("failed to store approval: %w", err)
	}

	return nil
}

// HandleReaction processes a reaction event (approve/reject)
func (m *Manager) HandleReaction(ctx context.Context, reaction, userID, channel, messageTs string) (*Request, string, error) {
	// Normalize reaction name
	reaction = strings.TrimPrefix(reaction, ":")
	reaction = strings.TrimSuffix(reaction, ":")

	// Only handle approval/rejection reactions
	isApprove := reaction == "white_check_mark" || reaction == "+1" || reaction == "heavy_check_mark"
	isReject := reaction == "x" || reaction == "-1" || reaction == "no_entry"

	if !isApprove && !isReject {
		return nil, "", nil
	}

	// Get pending approval
	req, err := m.get(ctx, messageTs)
	if err != nil {
		return nil, "", err
	}
	if req == nil {
		return nil, "", nil // Not a pending approval message
	}

	// Check if expired
	if time.Now().Unix() > req.ExpiresAt {
		m.delete(ctx, messageTs)
		return nil, "", fmt.Errorf("approval request has expired")
	}

	// Check if user is an approver
	if !contains(req.Approvers, userID) {
		return nil, "", nil // Ignore reactions from non-approvers
	}

	// Delete the pending approval
	if err := m.delete(ctx, messageTs); err != nil {
		return nil, "", fmt.Errorf("failed to delete approval: %w", err)
	}

	if isApprove {
		return req, "approved", nil
	}
	return req, "rejected", nil
}

// store saves a pending approval to DynamoDB
func (m *Manager) store(ctx context.Context, req Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	_, err = m.dynamodb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(m.cfg.AWS.DynamoDBTable),
		Item: map[string]types.AttributeValue{
			"pk":        &types.AttributeValueMemberS{Value: req.ID},
			"data":      &types.AttributeValueMemberS{Value: string(data)},
			"expiresAt": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", req.ExpiresAt)},
		},
	})
	return err
}

// get retrieves a pending approval from DynamoDB
func (m *Manager) get(ctx context.Context, id string) (*Request, error) {
	result, err := m.dynamodb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(m.cfg.AWS.DynamoDBTable),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, nil
	}

	dataAttr, ok := result.Item["data"].(*types.AttributeValueMemberS)
	if !ok {
		return nil, fmt.Errorf("invalid data format")
	}

	var req Request
	if err := json.Unmarshal([]byte(dataAttr.Value), &req); err != nil {
		return nil, err
	}

	return &req, nil
}

// delete removes a pending approval from DynamoDB
func (m *Manager) delete(ctx context.Context, id string) error {
	_, err := m.dynamodb.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(m.cfg.AWS.DynamoDBTable),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: id},
		},
	})
	return err
}

func formatAmount(cents int) string {
	if cents == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.2f", float64(cents)/100)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
