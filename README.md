# BreadBot

A Discord bot that detects bread in posted images, scores its "roundness" via an
inference microservice, and tracks per-user and server-wide leaderboards. It also
serves a **read-only HTTP stats API** and an embedded **web UI** (leaderboard,
per-user charts, image gallery) alongside the bot, in the same process.

This is the Go port of the original Python (discord.py) bot; behavior is preserved.

## How it works

- **Bread detection** — in configured channels, a post with an attachment from a
  user with an allowed role is sent to the inference microservice. The service
  returns every label with its confidence; the bot filters against
  `BREAD_DETECTION_CONFIDENCE`, replies with a verdict (and an annotated image +
  roundness, when available), records the result, and caches the full response
  in memory (see below).
- **Multiple images per message** — every image attachment on a post is
  downloaded, inferred, and scored **independently**. Each is stored as its own
  row and its own gallery image, so a message with several loaves produces
  several distinct results rather than only counting the last one.
- **"Are you sure?" retry** — reply to one of the bot's messages with "are you
  sure" / "no way" and it re-renders the verdict at the lower
  `OVERRIDE_DETECTION_CONFIDENCE`, so borderline breads pass and every label is
  mentioned. It re-renders every image of the original post, and the result is
  attributed to that post.
- **Prediction cache** — the inference service does its best single-pass
  detection and returns everything, so re-running it yields the same result.
  The bot therefore keeps the last 8 full predictions in an in-memory LRU keyed
  by `(message, attachment)`. An "are you sure" retry re-renders straight from
  this cache (reusing the already-annotated image, no second inference call). On
  a cache miss — e.g. after a restart or once 8 newer predictions have evicted
  it — it falls back to a fresh inference run at the relaxed confidence.
- **Catch up on startup** — the bot records the timestamp of the last message it
  processed. After (re)connecting it scans the most recent `CATCH_UP_LIMIT`
  messages in each bread channel and replays any posted after that timestamp
  through the normal pipeline, so images posted while it was offline still get a
  verdict. It only stores a single timestamp (not every message), and skips
  entirely on a fresh DB with no stored timestamp so it never replays full
  history.
- **Commands**
  - `$help` — list the available commands.
  - `$hello` — sanity check.
  - `$breadstats` — server best & worst leaderboards (defaults to top 3).
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
| `BREAD_DETECTION_CONFIDENCE` | `0.5` | min "bread" confidence to count as bread (and gate label mentions) |
| `OVERRIDE_DETECTION_CONFIDENCE` | `0.05` | relaxed threshold used on an "are you sure" retry |
| `CATCH_UP_LIMIT` | `50` | on startup, messages per bread channel to scan for ones missed while offline (`0` disables) |
| `DB_DATA_PATH` | `dbdata/messages.db` | SQLite file (reused from the Python bot; auto-migrated on open) |
| `DOWNLOADS_PATH` | `downloads/` | attachments, plots, annotated images (the gallery serves `predictions/` + `plots/` from here) |
| `INFERENCE_SERVICE_URL` | `http://localhost:8000` | microservice base URL |
| `ADMIN_API_ADDR` | `:8080` | HTTP server listen address (API + web UI) |
| `ADMIN_API_TOKEN` | _(unset)_ | if set, `/api/*` requires `Authorization: Bearer <token>` |
| `BASE_PATH` | _(unset)_ | URL prefix when served under a subpath behind a reverse proxy, e.g. `/breadbot` (leading slash added, trailing trimmed) |
| `DEBUG` | `false` | verbose logging; also enables permissive CORS for a local frontend dev server |

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

## Container image (ko)

Images are built with [ko](https://ko.build) — no Dockerfile, no docker daemon.
Because the binary is CGO-free, ko builds a minimal static `distroless` image
straight from the Go module (see `.ko.yaml`).

```sh
go install github.com/google/ko@latest        # if not installed

export KO_DOCKER_REPO=ghcr.io/hawawa4/breadbotdiscord
ko build ./cmd/breadbot                        # build + push to $KO_DOCKER_REPO
ko build --local ./cmd/breadbot                # or build into the local docker daemon
```

The **SQLite DB is not baked into the image** — it is mutable runtime state, so
mount it as a volume and point `DB_DATA_PATH` at the mount. The schema
auto-creates on first start, so an empty volume is fine.

```sh
docker run --rm \
  -e DISCORD_TOKEN=... \
  -e DISCORD_BREAD_CHANNELS='[123]' \
  -e DISCORD_BREAD_ROLE='[456]' \
  -e INFERENCE_SERVICE_URL=http://inference:8000 \
  -e DB_DATA_PATH=/app/dbdata/messages.db \
  -v "$PWD/dbdata:/app/dbdata" \
  -p 8080:8080 \
  "$(KO_DOCKER_REPO=ko.local ko build --bare ./cmd/breadbot)"
```

## docker-compose

`docker-compose.yml` runs the bot alongside the `breadvision` inference service.
Build the bot image with ko first (it loads into the local docker daemon), then
bring the stack up:

```sh
cp .env.example .env      # fill in DISCORD_TOKEN etc.
KO_DOCKER_REPO=ko.local ko build --bare -t compose ./cmd/breadbot
docker compose up -d
```

The bot's DB persists in `./.dbdatastuff/` (bind-mounted to `/app/dbdata`), it
reaches the inference service at `http://breadvision:8000`, and the stats API is
exposed on `:8080`. For a registry-based deploy, set `KO_DOCKER_REPO` to your
registry and update the `image:` reference in the compose file to the pushed tag.

## Web UI + HTTP API

Both run in the same process as the bot, on `ADMIN_API_ADDR`. The web UI is a
single-page app compiled into the binary via `go:embed`
(`internal/httpserver/frontend/dist`) and served at the root; the JSON API lives
under `/api`. When `BASE_PATH` is set, everything is mounted under that prefix
and the reverse proxy forwards the full path (the server strips the prefix once).

`/healthz` is always unauthenticated (and reachable at both the root and the
prefix so infra health checks don't need to know the subpath). `/api/*` requires
`Authorization: Bearer <ADMIN_API_TOKEN>` only when that token is set. All API
responses are JSON. **Discord snowflake ids are serialized as JSON strings**, not
numbers, so a browser client doesn't lose precision above 2^53.

| Endpoint | Description |
|---|---|
| `GET /healthz` | liveness — reports DB reachable + Discord session ready |
| `GET /api/stats/summary` | server-wide aggregates (scored count, distinct users, avg/max roundness) |
| `GET /api/leaderboard?order=max\|min&n=` | server roundness leaderboard (`n` clamped 1..100) |
| `GET /api/users?limit=&offset=` | paginated user directory (`limit` clamped 1..200) |
| `GET /api/users/{id}/roundness` | a user's min/max + last-50 history |
| `GET /api/users/{id}` | cached user info |
| `GET /api/messages/{ogmessage_id}` | a message's per-image stats (`{ogmessage_id, rows:[...]}`) |
| `GET /api/images/predictions/{name}` | an annotated prediction PNG from `DOWNLOADS_PATH/predictions/` |
| `GET /api/images/plots/{name}` | a roundness-history plot PNG from `DOWNLOADS_PATH/plots/` |

## Layout

```
cmd/breadbot/        entrypoint (config -> db -> http server -> discord session)
internal/config/     env + .env config
internal/db/         SQLite layer (shared by bot and HTTP server)
internal/inference/  microservice HTTP client
internal/bot/        discordgo session, message routing, bread pipeline, commands
internal/stats/      roundness-history PNG chart
internal/httpserver/ HTTP server: read-only stats API + embedded web UI
  frontend/dist/     compiled SPA embedded via go:embed (built from frontend/)
```

The web UI source lives at the repo root in `frontend/` (a Svelte app); its
build output is written to `internal/httpserver/frontend/dist/` so `go:embed`
can compile it into the binary. A committed placeholder `index.html` keeps a
clean checkout building before the SPA is ever built.
