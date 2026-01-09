# Adyen Slack Assistant

A Slack bot for Adyen payment troubleshooting with MCP integration, role-based authorization, and approval workflows.

## Features

- Natural language interaction with Adyen APIs via MCP
- Role-based access control (read, create, refund, cancel)
- Approval workflow for sensitive actions using Slack reactions
- Audit logging to dedicated Slack channel
- Event-driven architecture (Lambda + SQS)
- Thread-aware responses (always replies in threads)
- Secrets managed via Doppler

## Architecture

```
Slack → API Gateway → Webhook Lambda → SQS → Processor Lambda → Adyen MCP
                                                      ↓
                                                    LLM
```

## Prerequisites

- Go 1.22+
- Node.js 20+ (for Adyen MCP)
- AWS CLI configured
- Serverless Framework 3.x
- Doppler CLI

## Setup

### 1. Setup Doppler

```bash
# Install Doppler CLI (macOS)
brew install dopplerhq/cli/doppler

# Login to Doppler
doppler login

# Setup project (creates doppler.yml)
doppler setup
```

### 2. Configure Doppler Secrets

Add these secrets in your Doppler project (`adyen-slack-assistant`):

| Secret | Description |
|--------|-------------|
| `SLACK_BOT_TOKEN` | Slack bot OAuth token (xoxb-...) |
| `SLACK_SIGNING_SECRET` | Slack app signing secret |
| `ADYEN_API_KEY` | Adyen API key |
| `ADYEN_ENVIRONMENT` | `TEST` or `LIVE` |
| `ADYEN_LIVE_PREFIX` | Live URL prefix (only for LIVE) |
| `OPENAI_API_KEY` | OpenAI API key |
| `OPENAI_MODEL` | Model name (default: gpt-4o) |
| `PERMISSIONS_JSON` | JSON permissions config (see below) |

### 3. Create Slack App

1. Go to https://api.slack.com/apps → Create New App
2. Enable Event Subscriptions
3. Subscribe to bot events: `app_mention`, `message.im`, `reaction_added`
4. Add OAuth Scopes: `app_mentions:read`, `chat:write`, `channels:history`, `groups:history`, `im:history`, `reactions:read`, `usergroups:read`
5. Install to workspace
6. Copy Bot Token and Signing Secret → Add to Doppler

### 4. Get Adyen API Key

1. Go to Adyen Customer Area → Developers → API credentials
2. Create a new webservice user
3. Assign roles: Checkout Webservice, Merchant PAL Webservice, Management API roles
4. Generate API key → Add to Doppler

### 5. Configure Permissions

Add `PERMISSIONS_JSON` to Doppler:

```json
{
  "channels": ["C12345678"],
  "roles": {
    "admin": {
      "users": ["U12345678", "U87654321"],
      "groups": ["S11111111"]
    }
  },
  "actions": {
    "refund": {"level": "admin", "approve": true, "maxAmount": 10000},
    "cancel": {"level": "admin", "approve": true, "maxAmount": 0},
    "create": {"level": "admin", "approve": false, "maxAmount": 0},
    "read": {"level": "any", "approve": false, "maxAmount": 0}
  },
  "auditChannel": "C99999999"
}
```

### 6. Deploy

```bash
# Deploy to dev
make deploy

# Deploy to production
make deploy-prod
```

### 7. Configure Slack Event URL

After deployment, copy the webhook URL from the output and set it as the Request URL in your Slack app's Event Subscriptions.

## Permission Levels

| Action | Level | Approval | Description |
|--------|-------|----------|-------------|
| `read` | any | No | View payment status, list terminals, etc. |
| `create` | admin | No | Create payment links, sessions |
| `refund` | admin | Yes* | Refund payments |
| `cancel` | admin | Yes | Cancel payments |

*Refunds under `maxAmount` (default €100) don't require approval.

## Approval Workflow

1. User requests a sensitive action (e.g., refund)
2. Bot posts approval request in thread
3. Admin reacts with ✅ to approve or ❌ to reject
4. Bot executes action if approved
5. All actions logged to audit channel

## Development

```bash
# Install dependencies
make deps

# Build
make build

# Run locally with Doppler
make run-local

# Run tests
make test

# View Doppler secrets
make doppler-secrets
```

## Doppler Environments

| Config | Usage |
|--------|-------|
| `dev` | Development/staging |
| `prod` | Production |

## License

MIT
