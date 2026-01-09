package config

import (
	"encoding/json"
	"os"
	"sync"
)

type Config struct {
	Slack       SlackConfig       `json:"slack"`
	Adyen       AdyenConfig       `json:"adyen"`
	LLM         LLMConfig         `json:"llm"`
	Permissions PermissionsConfig `json:"permissions"`
	AWS         AWSConfig         `json:"aws"`
}

type SlackConfig struct {
	BotToken      string `json:"botToken"`
	SigningSecret string `json:"signingSecret"`
}

type AdyenConfig struct {
	APIKey      string `json:"apiKey"`
	Environment string `json:"environment"` // TEST or LIVE
	LivePrefix  string `json:"livePrefix"`
}

type LLMConfig struct {
	APIKey string `json:"apiKey"`
	Model  string `json:"model"`
}

type PermissionsConfig struct {
	Channels     []string          `json:"channels"`
	Roles        RolesConfig       `json:"roles"`
	Actions      map[string]Action `json:"actions"`
	AuditChannel string            `json:"auditChannel"`
}

type RolesConfig struct {
	Admin RoleDefinition `json:"admin"`
}

type RoleDefinition struct {
	Users  []string `json:"users"`
	Groups []string `json:"groups"`
}

type Action struct {
	Level     string `json:"level"`     // any, admin
	Approve   bool   `json:"approve"`   // requires approval
	MaxAmount int    `json:"maxAmount"` // threshold in cents (0 = always approve)
}

type AWSConfig struct {
	Region        string `json:"region"`
	DynamoDBTable string `json:"dynamoDBTable"`
	SQSQueueURL   string `json:"sqsQueueURL"`
}

var (
	cfg  *Config
	once sync.Once
)

func Load() *Config {
	once.Do(func() {
		cfg = &Config{
			Slack: SlackConfig{
				BotToken:      getEnv("SLACK_BOT_TOKEN", ""),
				SigningSecret: getEnv("SLACK_SIGNING_SECRET", ""),
			},
			Adyen: AdyenConfig{
				APIKey:      getEnv("ADYEN_API_KEY", ""),
				Environment: getEnv("ADYEN_ENVIRONMENT", "TEST"),
				LivePrefix:  getEnv("ADYEN_LIVE_PREFIX", ""),
			},
			LLM: LLMConfig{
				APIKey: getEnv("ANTHROPIC_API_KEY", ""),
				Model:  getEnv("ANTHROPIC_MODEL", "claude-sonnet-4-20250514"),
			},
			Permissions: loadPermissions(),
			AWS: AWSConfig{
				Region:        getEnv("AWS_REGION", "eu-west-1"),
				DynamoDBTable: getEnv("DYNAMODB_TABLE", "adyen-slack-approvals"),
				SQSQueueURL:   getEnv("SQS_QUEUE_URL", ""),
			},
		}
	})
	return cfg
}

func loadPermissions() PermissionsConfig {
	// Default permissions - override with PERMISSIONS_JSON env var or file
	defaultPerms := PermissionsConfig{
		Channels: []string{}, // PLACEHOLDER: Add your channel IDs
		Roles: RolesConfig{
			Admin: RoleDefinition{
				Users:  []string{}, // PLACEHOLDER: Add admin user IDs
				Groups: []string{}, // PLACEHOLDER: Add admin group IDs
			},
		},
		Actions: map[string]Action{
			"refund": {Level: "admin", Approve: true, MaxAmount: 10000},  // €100
			"cancel": {Level: "admin", Approve: true, MaxAmount: 0},
			"create": {Level: "admin", Approve: false, MaxAmount: 0},
			"read":   {Level: "any", Approve: false, MaxAmount: 0},
		},
		AuditChannel: "", // PLACEHOLDER: Add audit channel ID
	}

	// Try to load from environment variable
	if permJSON := os.Getenv("PERMISSIONS_JSON"); permJSON != "" {
		var perms PermissionsConfig
		if err := json.Unmarshal([]byte(permJSON), &perms); err == nil {
			return perms
		}
	}

	return defaultPerms
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
