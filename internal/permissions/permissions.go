package permissions

import (
	"github.com/getalternative/adyen-slack-assistant/internal/config"
	slackClient "github.com/getalternative/adyen-slack-assistant/internal/slack"
)

// ActionType maps Adyen MCP tools to action categories
var ActionType = map[string]string{
	// Read operations - anyone in allowed channels
	"get_payment_status":    "read",
	"get_payment_details":   "read",
	"list_payment_methods":  "read",
	"list_terminals":        "read",
	"get_terminal_details":  "read",
	"get_webhooks":          "read",
	"list_merchants":        "read",
	"get_merchant_details":  "read",

	// Create operations - admin only
	"create_payment_link":    "create",
	"create_payment_session": "create",

	// Destructive operations - admin + approval
	"refund_payment":            "refund",
	"cancel_payment":            "cancel",
	"expire_payment_link":       "cancel",
	"update_terminal_settings":  "create",
}

// Result represents the outcome of a permission check
type Result struct {
	Allowed       bool
	NeedsApproval bool
	Reason        string
	Approvers     []string // User IDs who can approve
}

// Checker handles permission validation
type Checker struct {
	cfg    *config.Config
	slack  *slackClient.Client
}

// New creates a new permission checker
func New(cfg *config.Config, slack *slackClient.Client) *Checker {
	return &Checker{cfg: cfg, slack: slack}
}

// Check validates if a user can perform an action
func (c *Checker) Check(userID, channelID, action string, amount int) Result {
	perms := c.cfg.Permissions

	// 1. Channel restriction
	if !contains(perms.Channels, channelID) && len(perms.Channels) > 0 {
		return Result{
			Allowed: false,
			Reason:  "This bot can only be used in authorized channels.",
		}
	}

	// 2. Get action type (default to read if unknown)
	actionType := ActionType[action]
	if actionType == "" {
		actionType = "read"
	}

	// 3. Get action config
	actionCfg, exists := perms.Actions[actionType]
	if !exists {
		actionCfg = config.Action{Level: "any", Approve: false}
	}

	// 4. Anyone can read
	if actionCfg.Level == "any" {
		return Result{Allowed: true}
	}

	// 5. Check if user is admin
	isAdmin := c.isAdmin(userID)
	if !isAdmin {
		return Result{
			Allowed: false,
			Reason:  "Only admins can perform this action.",
		}
	}

	// 6. Check if approval is needed
	needsApproval := actionCfg.Approve
	if actionCfg.MaxAmount > 0 && amount <= actionCfg.MaxAmount {
		needsApproval = false // Under threshold, no approval needed
	}

	if needsApproval {
		approvers := c.getOtherAdmins(userID)
		return Result{
			Allowed:       true,
			NeedsApproval: true,
			Approvers:     approvers,
		}
	}

	return Result{Allowed: true}
}

// isAdmin checks if a user is an admin (by user ID or group membership)
func (c *Checker) isAdmin(userID string) bool {
	adminCfg := c.cfg.Permissions.Roles.Admin

	// Check direct user ID
	if contains(adminCfg.Users, userID) {
		return true
	}

	// Check group membership
	for _, groupID := range adminCfg.Groups {
		members, err := c.slack.GetUsergroupMembers(groupID)
		if err != nil {
			continue
		}
		if contains(members, userID) {
			return true
		}
	}

	return false
}

// IsAdmin is a public wrapper for admin check
func (c *Checker) IsAdmin(userID string) bool {
	return c.isAdmin(userID)
}

// getOtherAdmins returns admin user IDs excluding the requesting user
func (c *Checker) getOtherAdmins(excludeUserID string) []string {
	var admins []string
	for _, userID := range c.cfg.Permissions.Roles.Admin.Users {
		if userID != excludeUserID {
			admins = append(admins, userID)
		}
	}
	return admins
}

// GetAdmins returns all admin user IDs
func (c *Checker) GetAdmins() []string {
	return c.cfg.Permissions.Roles.Admin.Users
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
