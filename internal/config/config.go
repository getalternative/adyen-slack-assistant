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
	Channels     []string `json:"channels"`
	Admins       []string `json:"admins"` // User IDs who can read+write
	AuditChannel string   `json:"auditChannel"`
}

type AWSConfig struct {
	Region      string `json:"region"`
	SQSQueueURL string `json:"sqsQueueURL"`
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
				Region:      getEnv("AWS_REGION", "eu-west-1"),
				SQSQueueURL: getEnv("SQS_QUEUE_URL", ""),
			},
		}
	})
	return cfg
}

func loadPermissions() PermissionsConfig {
	// Default permissions - override with PERMISSIONS_JSON env var or file
	defaultPerms := PermissionsConfig{
		Channels:     []string{},
		Admins:       []string{},
		AuditChannel: "",
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
