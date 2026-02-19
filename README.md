# Twitch Channel Points Miner — Go Edition

A high-performance Go rewrite of the [Twitch Channel Points Miner v2](https://github.com/rdavydov/Twitch-Channel-Points-Miner-v2). Mines channel points, claims bonuses, places predictions, joins raids, claims drops, and more — all with a fraction of the resource usage.

## Features

- **Multi-account support** — run multiple Twitch accounts from a single binary
- **Channel points mining** — automatic minute-watched events, bonus claims, watch streaks
- **Predictions** — configurable betting strategies (SMART, HIGH_ODDS, MOST_VOTED, etc.)
- **Drops** — automatic campaign sync and drop claiming
- **Raids** — automatic raid joining
- **Community moments** — automatic moment claiming
- **Community goals** — automatic goal contributions
- **Category watcher** — auto-discover streamers by game category
- **Notifications** — Telegram, Discord, Webhook, Matrix, Pushover, Gotify
- **Analytics dashboard** — built-in web UI for monitoring
- **Fly.io ready** — deploy with a single command

## Resource Comparison

| Resource     | Python (per account) | Go (per account)  |
| ------------ | -------------------- | ----------------- |
| Memory       | ~80–120 MB           | ~5–15 MB          |
| Docker image | ~800 MB              | ~10–15 MB         |
| Startup time | ~5–10 s              | < 100 ms          |
| Threads      | 60+                  | ~10–20 goroutines |

## Running Locally

**Prerequisites:** [Go 1.24+](https://go.dev/dl/)

**Unix (macOS/Linux):**

```bash
./scripts/run.sh
```

**Windows:**

```batch
scripts\run.bat
```

**With custom flags:**

```bash
./scripts/run.sh -config configs -port 9090 -log-level debug
```

> The scripts build the binary and run it in one step. You can also build manually with `go build -o twitch-miner ./cmd/miner`.

### Flags

| Flag         | Default   | Description                          |
| ------------ | --------- | ------------------------------------ |
| `-config`    | `configs` | Path to the configuration directory  |
| `-port`      | `8080`    | Port for the health/analytics server |
| `-log-level` | `INFO`    | Log level: DEBUG, INFO, WARN, ERROR  |

## Configuration

Create one YAML file per account in the `configs/` directory. **The filename (without extension) becomes the Twitch username** — no `username` field is needed in the YAML.

For example, to add an account for Twitch user `guliveer_`, create `configs/guliveer_.yaml`.

```bash
# Copy the example and customize for your account
cp configs/example.yaml.example configs/your_twitch_username.yaml
```

See [`configs/example.yaml.example`](configs/example.yaml.example) for the full schema. Files with a `.yaml.example` extension are not loaded as configs — only `.yaml` and `.yml` files are loaded.

### Quick Start

```yaml
# configs/your_twitch_username.yaml
# The filename IS the username — no username field needed.

features:
  claim_drops_startup: false
  enable_analytics: true

priority:
  - STREAK
  - DROPS
  - ORDER

streamer_defaults:
  make_predictions: true
  follow_raid: true
  claim_drops: true
  claim_moments: true
  watch_streak: true
  chat: "ONLINE"
  bet:
    strategy: "SMART"
    percentage: 5
    max_points: 50000
    delay: 6
    delay_mode: "FROM_END"

streamers:
  - username: "streamer1"
  - username: "streamer2"
    settings:
      make_predictions: false
```

### Environment Variables

Secrets and auth tokens are injected via environment variables. Per-account variables **require** the `_<USERNAME>` suffix (uppercase) to scope them to the correct account.

| Variable                         | Description                                         |
| -------------------------------- | --------------------------------------------------- |
| `TWITCH_AUTH_TOKEN_<USERNAME>`   | OAuth token (fallback for headless auth)            |
| `TWITCH_PASSWORD_<USERNAME>`     | Twitch password (last-resort auth, may require 2FA) |
| `TELEGRAM_TOKEN_<USERNAME>`      | Telegram bot token                                  |
| `TELEGRAM_CHAT_ID_<USERNAME>`    | Telegram chat ID                                    |
| `DISCORD_WEBHOOK_<USERNAME>`     | Discord webhook URL                                 |
| `WEBHOOK_URL_<USERNAME>`         | Generic webhook URL                                 |
| `MATRIX_HOMESERVER_<USERNAME>`   | Matrix homeserver URL                               |
| `MATRIX_ROOM_ID_<USERNAME>`      | Matrix room ID                                      |
| `MATRIX_ACCESS_TOKEN_<USERNAME>` | Matrix access token                                 |
| `PUSHOVER_TOKEN_<USERNAME>`      | Pushover API token                                  |
| `PUSHOVER_USER_KEY_<USERNAME>`   | Pushover user key                                   |
| `GOTIFY_URL_<USERNAME>`          | Gotify server URL                                   |
| `GOTIFY_TOKEN_<USERNAME>`        | Gotify app token                                    |
| `PORT`                           | HTTP server port                                    |
| `LOG_LEVEL`                      | Log level override                                  |

For example, for user `guliveer_` the Telegram token variable is `TELEGRAM_TOKEN_GULIVEER_` and the auth token variable is `TWITCH_AUTH_TOKEN_GULIVEER_`.

## Notifications

The miner supports multiple notification providers. Configure them in your account YAML file under the `notifications` key. Sensitive credentials (tokens, API keys) are injected via environment variables — see [Environment Variables](#environment-variables) above.

### Supported Providers

| Provider | Config key | Required env vars                                                                             |
| -------- | ---------- | --------------------------------------------------------------------------------------------- |
| Telegram | `telegram` | `TELEGRAM_TOKEN_<USERNAME>`, `TELEGRAM_CHAT_ID_<USERNAME>`                                    |
| Discord  | `discord`  | `DISCORD_WEBHOOK_<USERNAME>`                                                                  |
| Gotify   | `gotify`   | `GOTIFY_URL_<USERNAME>`, `GOTIFY_TOKEN_<USERNAME>`                                            |
| Pushover | `pushover` | `PUSHOVER_TOKEN_<USERNAME>`, `PUSHOVER_USER_KEY_<USERNAME>`                                   |
| Matrix   | `matrix`   | `MATRIX_HOMESERVER_<USERNAME>`, `MATRIX_ROOM_ID_<USERNAME>`, `MATRIX_ACCESS_TOKEN_<USERNAME>` |
| Webhook  | `webhook`  | `WEBHOOK_URL_<USERNAME>`                                                                      |

> Replace `<USERNAME>` with the Twitch username in **UPPERCASE**. For example, user `guliveer_` → `TELEGRAM_TOKEN_GULIVEER_`.

### Example: Telegram

```yaml
notifications:
  telegram:
    enabled: true
    token: "YOUR_BOT_TOKEN"
    chat_id: "YOUR_CHAT_ID"
    events:
      - "DROP_CLAIM"
      - "DROP_STATUS"
      - "STREAMER_ONLINE"
      - "STREAMER_OFFLINE"
      - "BET_WIN"
      - "BET_LOSE"
    disable_notification: false
```

> **Tip:** The `token` and `chat_id` fields in YAML are optional — if omitted, the miner reads them from `TELEGRAM_TOKEN_<USERNAME>` and `TELEGRAM_CHAT_ID_<USERNAME>` environment variables instead. This is the recommended approach for production/headless deployments.

### Event Filtering

The `events` list controls which events trigger a notification. If the list is **empty or omitted**, **all** events will be notified.

**Available events:**

| Event              | Description                         |
| ------------------ | ----------------------------------- |
| `DROP_CLAIM`       | A drop was claimed                  |
| `DROP_STATUS`      | Drop progress update                |
| `STREAMER_ONLINE`  | A streamer went live                |
| `STREAMER_OFFLINE` | A streamer went offline             |
| `BONUS_CLAIM`      | Channel points bonus claimed        |
| `JOIN_RAID`        | Joined a raid                       |
| `MOMENT_CLAIM`     | Community moment claimed            |
| `BET_START`        | A prediction started                |
| `BET_WIN`          | A prediction was won                |
| `BET_LOSE`         | A prediction was lost               |
| `BET_REFUND`       | A prediction was refunded           |
| `BET_FILTERS`      | Prediction skipped due to filters   |
| `CHAT_MENTION`     | Your username was mentioned in chat |
| `TEST`             | Test notification (see below)       |

### Testing Notifications

The miner exposes a `POST /api/test-notification` endpoint on the analytics server to verify your notification setup. It sends a test message to **all** enabled notification providers, **bypassing event filters**.

```bash
curl -X POST http://localhost:8080/api/test-notification
```

A successful response looks like:

```json
{
  "status": "ok",
  "message": "Test notification sent to all enabled notifiers"
}
```

If some providers fail, you'll get a partial status with error details:

```json
{
  "status": "partial",
  "errors": ["telegram: 401 Unauthorized"]
}
```

> **Note:** Replace `8080` with your configured port (the `-port` flag or `PORT` env var). This endpoint is useful for verifying that tokens, chat IDs, and webhook URLs are correctly configured before relying on notifications in production.

## Authentication

Authentication is automatic — on first run the miner walks through a priority chain until one method succeeds:

| Priority | Method                                     | Description                                                                                                                             |
| -------- | ------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------- |
| 1        | **Cookie file**                            | Saved from a previous successful login. Reused automatically.                                                                           |
| 2        | **`TWITCH_AUTH_TOKEN_<USERNAME>` env var** | Fallback — checked directly in [`Login()`](internal/auth/auth.go:93) via `os.Getenv()`.                                                 |
| 3        | **`TWITCH_PASSWORD_<USERNAME>` env var**   | Last resort — password login, checked via `os.Getenv()` in [`Login()`](internal/auth/auth.go:93). May require 2FA.                      |
| 4        | **Device code flow**                       | Interactive — displays a code in the terminal and waits for you to activate it at [twitch.tv/activate](https://www.twitch.tv/activate). |

Once authenticated by **any** method, the token is validated against the Twitch OAuth2 endpoint to confirm it belongs to the expected user (derived from the config filename). If there's a mismatch — for example, you completed the device code flow with the wrong Twitch account — the system will show a clear error like:

> `authenticated as "wrong_user" but config expects "your_username" — please log in with the correct account`

The validated token is then saved to a cookie file and reused on subsequent starts — so the device code flow is typically a one-time step.

> **⚠️ Warning:** Password login (step 3) may trigger Twitch's two-factor authentication (2FA) prompt, making it less reliable in fully headless environments. **Prefer `TWITCH_AUTH_TOKEN_<USERNAME>` over `TWITCH_PASSWORD_<USERNAME>` whenever possible.** Use the password method only as a last resort when you cannot obtain an OAuth token.

### When to use the env vars

The `TWITCH_AUTH_TOKEN_<USERNAME>` env var is the **recommended** fallback when the interactive device code flow is impractical:

- **Headless deployments** — servers or containers without interactive terminal access (e.g., Fly.io, cloud VMs)
- **Multi-account setups** — pre-seed tokens for several accounts without running the device flow for each
- **CI/CD environments** — automated pipelines where no human is present to complete the device flow

The variable name is `TWITCH_AUTH_TOKEN_` followed by the **uppercase** username with hyphens replaced by underscores. Examples:

| Username    | Env var                       |
| ----------- | ----------------------------- |
| `guliveer_` | `TWITCH_AUTH_TOKEN_GULIVEER_` |
| `my-user`   | `TWITCH_AUTH_TOKEN_MY_USER`   |

> **Note:** Both `TWITCH_AUTH_TOKEN_<USERNAME>` and `TWITCH_PASSWORD_<USERNAME>` are read directly in the auth flow (`os.Getenv()`), not through the config layer or `applyEnvOverrides()`.

## Docker

```bash
# Build
docker build -t twitch-miner-go .

# Run
docker run -d \
  -p 8080:8080 \
  -v miner_data:/data \
  twitch-miner-go

# Run with auth token (recommended — for headless environments)
docker run -d \
  -p 8080:8080 \
  -v miner_data:/data \
  -e TWITCH_AUTH_TOKEN_YOUR_USERNAME=your_oauth_token \
  twitch-miner-go

# Run with password (optional — last resort, may require 2FA)
docker run -d \
  -p 8080:8080 \
  -v miner_data:/data \
  -e TWITCH_PASSWORD_YOUR_USERNAME=your_twitch_password \
  twitch-miner-go
```

## Deploy to Fly.io

The repo includes [`fly.toml`](fly.toml) — the Fly.io deployment config. You can customize the app name, region, and VM settings directly in this file.

### Setup

```bash
# 1. Copy the example account config and customize (filename = your Twitch username)
cp configs/example.yaml.example configs/your_twitch_username.yaml

# 2. Install flyctl
curl -L https://fly.io/install.sh | sh

# 3. Login
fly auth login

# 4. Create the app (first time only)
fly launch --no-deploy

# 5. Create a volume for persistent data
fly volumes create miner_data --region fra --size 1

# 6. (Optional) Set auth token for headless login — skips the interactive device code flow (recommended)
fly secrets set TWITCH_AUTH_TOKEN_YOUR_USERNAME=your_oauth_token

# 7. (Optional) Set password for last-resort login — less reliable than auth token, may require 2FA
fly secrets set TWITCH_PASSWORD_YOUR_USERNAME=your_twitch_password

# 8. Set notification secrets (replace YOUR_USERNAME with your Twitch username in uppercase)
fly secrets set TELEGRAM_TOKEN_YOUR_USERNAME=your_bot_token
fly secrets set TELEGRAM_CHAT_ID_YOUR_USERNAME=your_chat_id
```

### Manual Deploy

```bash
fly deploy

# View logs
fly logs

# Check health
curl https://your-app-name.fly.dev/health
```

### Automatic Deploy (CI/CD)

The repo includes a [GitHub Actions workflow](.github/workflows/deploy.yml) that automatically deploys to Fly.io on every push to `main`.

To enable it:

1. **Generate a Fly.io deploy token:**

   ```bash
   fly tokens create deploy -x 999999h
   ```

2. **Add the token as a GitHub Secret:**

   Go to your repo → **Settings** → **Secrets and variables** → **Actions** → **New repository secret**
   - **Name:** `FLY_API_TOKEN`
   - **Value:** the token from step 1

Once configured, every push to `main` will trigger an automatic deployment.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full system design, concurrency model, and data flow diagrams.

## License

Same as the upstream project.
