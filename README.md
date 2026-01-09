# Adyen Slack Assistant

A Slack bot for Adyen payment troubleshooting with MCP integration, role-based authorization, and approval workflows.

## Features

- Natural language interaction with Adyen APIs via MCP
- Role-based access control (read, create, refund, cancel)
- Approval workflow for sensitive actions using Slack reactions
- Audit logging to dedicated Slack channel
- Event-driven architecture (Lambda + SQS)
- Thread-aware responses (always replies in threads)

## Architecture

```
Slack → API Gateway → Webhook Lambda → SQS → Processor Lambda → Adyen MCP
                                                      ↓
                                              LLM (OpenAI)
```

## Prerequisites

- Go 1.22+
- Node.js 20+ (for Adyen MCP)
- AWS CLI configured
- Serverless Framework 3.x

## Setup

### 1. Create Slack App

1. Go to https://api.slack.com/apps → Create New App
2. Enable Event Subscriptions
3. Subscribe to bot events: `app_mention`, `message.im`, `reaction_added`
4. Add OAuth Scopes: `app_mentions:read`, `chat:write`, `channels:history`, `groups:history`, `im:history`, `reactions:read`, `usergroups:read`
5. Install to workspace
6. Copy Bot Token and Signing Secret

### 2. Get Adyen API Key

1. Go to Adyen Customer Area → Developers → API credentials
2. Create a new webservice user
3. Assign roles: Checkout Webservice, Merchant PAL Webservice, Management API roles
4. Generate API key

### 3. Configure Permissions

Set the `PERMISSIONS_JSON` environment variable:

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

### 4. Deploy

```bash
# Set environment variables
export SLACK_BOT_TOKEN="xoxb-..."
export SLACK_SIGNING_SECRET="..."
export ADYEN_API_KEY="..."
export OPENAI_API_KEY="sk-..."
export PERMISSIONS_JSON='{"channels":["C..."],...}'

# Deploy
make deploy STAGE=dev
```

### 5. Configure Slack Event URL

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

# Run tests
make test

# View logs
make logs-processor STAGE=dev
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `SLACK_BOT_TOKEN` | Yes | Slack bot OAuth token |
| `SLACK_SIGNING_SECRET` | Yes | Slack signing secret |
| `ADYEN_API_KEY` | Yes | Adyen API key |
| `ADYEN_ENVIRONMENT` | No | `TEST` or `LIVE` (default: TEST) |
| `ADYEN_LIVE_PREFIX` | No | Required for LIVE environment |
| `OPENAI_API_KEY` | Yes | OpenAI API key |
| `OPENAI_MODEL` | No | Model to use (default: gpt-4o) |
| `PERMISSIONS_JSON` | No | JSON permissions config |

## License

MIT
