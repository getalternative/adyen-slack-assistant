# Adyen Slack Assistant

Slack bot for Adyen payment troubleshooting using MCP.

## Features

- Natural language queries to Adyen APIs
- Admins can read + write, others read-only
- Audit logging to Slack channel
- Thread-aware responses

## Architecture

```
Slack → API Gateway → Lambda (webhook) → SQS → Lambda (processor) → Adyen MCP
```

## Setup

### 1. Doppler

```bash
brew install dopplerhq/cli/doppler
doppler login
doppler setup
```

### 2. Add secrets to Doppler

| Secret | Description |
|--------|-------------|
| `SLACK_BOT_TOKEN` | Bot token (xoxb-...) |
| `SLACK_SIGNING_SECRET` | Signing secret |
| `ADYEN_API_KEY` | Adyen API key |
| `ADYEN_ENVIRONMENT` | TEST or LIVE |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `PERMISSIONS_JSON` | See below |

### 3. Permissions JSON

```json
{
  "channels": ["C0123456789"],
  "admins": ["U0123456789", "U9876543210"],
  "auditChannel": "C9999999999"
}
```

- `channels` - Where bot can be used (empty = everywhere)
- `admins` - User IDs who can do write operations (refund, cancel, create)
- `auditChannel` - Where to log all actions

**Find IDs:**
- Channel: Right-click → View details → scroll to bottom
- User: Click profile → ⋮ → Copy member ID

### 4. Deploy

```bash
make deploy        # dev
make deploy-prod   # prod
```

### 5. Slack App Setup

1. Create app at https://api.slack.com/apps
2. Enable Event Subscriptions → set webhook URL from deploy output
3. Subscribe to: `app_mention`, `message.im`
4. Add scopes: `app_mentions:read`, `chat:write`, `im:history`
5. Install to workspace

## Permissions

| User | Can Do |
|------|--------|
| Admin | Read + Write (refund, cancel, create) |
| Others | Read only (status, list) |

## Development

```bash
make deps       # install dependencies
make build      # build binaries
make run-local  # run with Doppler
```
