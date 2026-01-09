package slack

import (
	"github.com/getalternative/adyen-slack-assistant/internal/config"
	"github.com/slack-go/slack"
)

type Client struct {
	api *slack.Client
}

func New(cfg *config.Config) *Client {
	return &Client{
		api: slack.New(cfg.Slack.BotToken),
	}
}

// Message represents an incoming Slack message
type Message struct {
	Channel  string
	User     string
	Text     string
	Ts       string // Message timestamp
	ThreadTs string // Thread timestamp (empty if not in a thread)
}

// GetThreadTs returns the thread timestamp to reply to.
// If the message is already in a thread, returns that thread's ts.
// If the message is not in a thread, returns the message's ts to create a new thread.
func (m *Message) GetThreadTs() string {
	if m.ThreadTs != "" {
		return m.ThreadTs
	}
	return m.Ts
}

// Reply sends a message in the same thread as the original message.
// Always creates/continues a thread - never posts at channel level.
func (c *Client) Reply(msg *Message, text string) error {
	_, _, err := c.api.PostMessage(
		msg.Channel,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(msg.GetThreadTs()),
	)
	return err
}

// ReplyBlocks sends a message with blocks in the same thread.
func (c *Client) ReplyBlocks(msg *Message, text string, blocks ...slack.Block) error {
	_, _, err := c.api.PostMessage(
		msg.Channel,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(msg.GetThreadTs()),
		slack.MsgOptionBlocks(blocks...),
	)
	return err
}

// PostToChannel posts a message to a specific channel and thread.
func (c *Client) PostToChannel(channel, threadTs, text string) (string, error) {
	_, ts, err := c.api.PostMessage(
		channel,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTs),
	)
	return ts, err
}

// GetUserInfo retrieves user information
func (c *Client) GetUserInfo(userID string) (*slack.User, error) {
	return c.api.GetUserInfo(userID)
}

// GetUsergroupMembers retrieves members of a user group
func (c *Client) GetUsergroupMembers(groupID string) ([]string, error) {
	return c.api.GetUserGroupMembers(groupID)
}

// AddReaction adds a reaction to a message
func (c *Client) AddReaction(channel, timestamp, reaction string) error {
	return c.api.AddReaction(reaction, slack.ItemRef{
		Channel:   channel,
		Timestamp: timestamp,
	})
}
