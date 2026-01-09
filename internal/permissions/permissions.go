package permissions

import (
	"github.com/getalternative/adyen-slack-assistant/internal/config"
)

// IsWriteAction returns true if the action modifies data
var writeActions = map[string]bool{
	"refund_payment":           true,
	"cancel_payment":           true,
	"create_payment_link":      true,
	"create_payment_session":   true,
	"expire_payment_link":      true,
	"update_terminal_settings": true,
}

// Result represents the outcome of a permission check
type Result struct {
	Allowed bool
	Reason  string
}

// Checker handles permission validation
type Checker struct {
	cfg *config.Config
}

// New creates a new permission checker
func New(cfg *config.Config) *Checker {
	return &Checker{cfg: cfg}
}

// Check validates if a user can perform an action
// Admins: read + write
// Others: read only
func (c *Checker) Check(userID, channelID, action string) Result {
	perms := c.cfg.Permissions

	// Channel restriction (if configured)
	if len(perms.Channels) > 0 && !contains(perms.Channels, channelID) {
		return Result{Allowed: false, Reason: "This bot can only be used in authorized channels."}
	}

	// Check if it's a write action
	if writeActions[action] {
		if !c.IsAdmin(userID) {
			return Result{Allowed: false, Reason: "Only admins can perform this action."}
		}
	}

	return Result{Allowed: true}
}

// IsAdmin checks if a user is an admin
func (c *Checker) IsAdmin(userID string) bool {
	return contains(c.cfg.Permissions.Admins, userID)
}

// GetAdmins returns all admin user IDs
func (c *Checker) GetAdmins() []string {
	return c.cfg.Permissions.Admins
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
