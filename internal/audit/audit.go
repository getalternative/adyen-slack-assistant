package audit

import (
	"fmt"
	"time"

	"github.com/getalternative/adyen-slack-assistant/internal/config"
	slackClient "github.com/getalternative/adyen-slack-assistant/internal/slack"
)

// EventType represents the type of audit event
type EventType string

const (
	EventAllowed  EventType = "allowed"
	EventDenied   EventType = "denied"
	EventApproved EventType = "approved"
	EventRejected EventType = "rejected"
	EventError    EventType = "error"
)

// Entry represents an audit log entry
type Entry struct {
	Timestamp  time.Time
	UserID     string
	Action     string
	Channel    string
	EventType  EventType
	ApprovedBy string
	Details    string
}

// Logger handles audit logging to Slack
type Logger struct {
	cfg   *config.Config
	slack *slackClient.Client
}

// New creates a new audit logger
func New(cfg *config.Config, slack *slackClient.Client) *Logger {
	return &Logger{cfg: cfg, slack: slack}
}

// Log sends an audit entry to the audit channel
func (l *Logger) Log(entry Entry) error {
	channel := l.cfg.Permissions.AuditChannel
	if channel == "" {
		return nil // No audit channel configured
	}

	emoji := l.getEmoji(entry.EventType)
	text := l.formatEntry(entry, emoji)

	_, err := l.slack.PostToChannel(channel, "", text)
	return err
}

// LogAllowed logs a successful action
func (l *Logger) LogAllowed(userID, action, channel, details string) error {
	return l.Log(Entry{
		Timestamp: time.Now(),
		UserID:    userID,
		Action:    action,
		Channel:   channel,
		EventType: EventAllowed,
		Details:   details,
	})
}

// LogDenied logs a denied action
func (l *Logger) LogDenied(userID, action, channel, reason string) error {
	return l.Log(Entry{
		Timestamp: time.Now(),
		UserID:    userID,
		Action:    action,
		Channel:   channel,
		EventType: EventDenied,
		Details:   reason,
	})
}

// LogApproved logs an approved action
func (l *Logger) LogApproved(userID, action, channel, approvedBy, details string) error {
	return l.Log(Entry{
		Timestamp:  time.Now(),
		UserID:     userID,
		Action:     action,
		Channel:    channel,
		EventType:  EventApproved,
		ApprovedBy: approvedBy,
		Details:    details,
	})
}

// LogRejected logs a rejected action
func (l *Logger) LogRejected(userID, action, channel, rejectedBy string) error {
	return l.Log(Entry{
		Timestamp:  time.Now(),
		UserID:     userID,
		Action:     action,
		Channel:    channel,
		EventType:  EventRejected,
		ApprovedBy: rejectedBy, // reusing field for rejector
		Details:    "Request rejected",
	})
}

// LogError logs an error
func (l *Logger) LogError(userID, action, channel, errMsg string) error {
	return l.Log(Entry{
		Timestamp: time.Now(),
		UserID:    userID,
		Action:    action,
		Channel:   channel,
		EventType: EventError,
		Details:   errMsg,
	})
}

func (l *Logger) getEmoji(eventType EventType) string {
	switch eventType {
	case EventAllowed:
		return ":white_check_mark:"
	case EventDenied:
		return ":no_entry:"
	case EventApproved:
		return ":heavy_check_mark:"
	case EventRejected:
		return ":x:"
	case EventError:
		return ":warning:"
	default:
		return ":grey_question:"
	}
}

func (l *Logger) formatEntry(entry Entry, emoji string) string {
	timestamp := entry.Timestamp.UTC().Format("2006-01-02 15:04:05 UTC")

	base := fmt.Sprintf("%s *%s* | `%s`\n"+
		"*User:* <@%s> | *Channel:* <#%s>\n"+
		"*Time:* %s",
		emoji,
		entry.EventType,
		entry.Action,
		entry.UserID,
		entry.Channel,
		timestamp,
	)

	if entry.ApprovedBy != "" {
		verb := "Approved by"
		if entry.EventType == EventRejected {
			verb = "Rejected by"
		}
		base += fmt.Sprintf("\n*%s:* <@%s>", verb, entry.ApprovedBy)
	}

	if entry.Details != "" {
		base += fmt.Sprintf("\n*Details:* %s", entry.Details)
	}

	return base
}
