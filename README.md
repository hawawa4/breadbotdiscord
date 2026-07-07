# BreadBot

A Discord bot that detects bread in posted images, scores its "roundness" via an
inference microservice, and tracks per-user and server-wide leaderboards. It also
serves a thin **read-only HTTP stats API** alongside the bot for building an admin
panel on top.

This is the Go port of the original Python (discord.py) bot; behavior is preserved.

## How it works

- **Bread detection** — in configured channels, a post with an attachment from a
  user with an allowed role is sent to the inference microservice. The bot replies
  with a verdict (and an annotated image + roundness, when available) and records
  the result.
- **"Are you sure?" retry** — reply to one of the bot's messages with "are you
  sure" / "no way" and it re-runs detection at a lower confidence, attributing the
  result to the original post.
- **Commands**
  - `$hello` — sanity check.
  - `$breadstats --self` — your best & worst roundness.
  - `$breadstats --top [n]` — server best & worst leaderboards (n clamped to 10).
  - `$breadstats --history` — a PNG chart of your last 50 roundness scores.

## Configuration

Copy `.env.example` to `.env` and fill it in (a `.env` is loaded automatically for
local runs; in production, set the variables in the environment). `DISCORD_TOKEN`
is required; everything else has a default.

| Variable | Default | Notes |
|---|---|---|
| `DISCORD_TOKEN` | — | **required** |
| `DISCORD_BREAD_CHANNELS` | `[]` | channel ids, `[1,2,3]` format |
| `DISCORD_BREAD_ROLE` | `[]` | role ids, `[1,2,3]` format |
| `BREAD_DETECTION_CONFIDENCE` | `0.5` | "bread" label threshold |
| `OVERRIDE_DETECTION_CONFIDENCE` | `0.1` | threshold on "are you sure" retry |
| `DB_DATA_PATH` | `dbdata/messages.db` | SQLite file (reused from the Python bot) |
| `DOWNLOADS_PATH` | `downloads/` | attachments, plots, annotated images |
| `INFERENCE_SERVICE_URL` | `http://localhost:8000` | microservice base URL |
| `ADMIN_API_ADDR` | `:8080` | stats API listen address |
| `ADMIN_API_TOKEN` | _(unset)_ | if set, `/api/*` requires `Authorization: Bearer <token>` |
| `DEBUG` | `false` | verbose logging |

## Run locally

Requires Go 1.26+ and a reachable inference microservice.

```sh
cp .env.example .env      # then edit .env
go run ./cmd/breadbot
```

## Test

```sh
go test ./...
```

Tests are self-contained: DB tests run against a temp copy of the committed
`dbdata/messages.db`, and the inference client / HTTP handlers use httptest stubs.

## Build

```sh
CGO_ENABLED=0 go build -o breadbot ./cmd/breadbot
```

The pure-Go SQLite driver (`modernc.org/sqlite`) means no CGO and a fully static
binary.

## Docker

```sh
docker build -t breadbot .
docker run --rm \
  -e DISCORD_TOKEN=... \
  -e DISCORD_BREAD_CHANNELS='[123]' \
  -e DISCORD_BREAD_ROLE='[456]' \
  -e INFERENCE_SERVICE_URL=http://inference:8000 \
  -p 8080:8080 \
  breadbot
```

The image is a multi-stage build onto `distroless/static` (small, nonroot).

## Read-only HTTP API

Runs in the same process as the bot. All responses are JSON.

| Endpoint | Description |
|---|---|
| `GET /healthz` | liveness — reports DB reachable + Discord session ready |
| `GET /api/leaderboard?order=max\|min&n=` | server roundness leaderboard |
| `GET /api/users/{id}/roundness` | a user's min/max + last-50 history |
| `GET /api/users/{id}` | cached user info |
| `GET /api/messages/{ogmessage_id}` | a single message's stats |

`/healthz` is always unauthenticated; `/api/*` requires the bearer token only when
`ADMIN_API_TOKEN` is set.

## Layout

```
cmd/breadbot/        entrypoint (config -> db -> http server -> discord session)
internal/config/     env + .env config
internal/db/         SQLite layer (shared by bot and HTTP server)
internal/inference/  microservice HTTP client
internal/bot/        discordgo session, message routing, bread pipeline, commands
internal/stats/      roundness-history PNG chart
internal/httpserver/ read-only stats API
```
