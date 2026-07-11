# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

BreadBot is a Discord bot (Go, `discordgo`) that detects bread in posted images via an external
inference microservice, scores its "roundness", and tracks per-user/server leaderboards. A
read-only HTTP stats API runs in the **same process**. This is a behavior-preserving Go port of an
original Python (discord.py) bot; source comments frequently reference the Python originals ("Ports
`_send_bread_message`", "Mirrors `is_bread_candidate`").

## Commands

Recipes live in the `Justfile` (run `just` to list). The essentials:

```sh
just run      # go run ./cmd/breadbot  (reads .env; needs a reachable inference service)
just test     # go test ./...
just check    # go vet ./... && go build ./...  (fast sanity check)
just build    # CGO_ENABLED=0 go build -o breadbot ./cmd/breadbot
```

Run a single test: `go test ./internal/bot -run TestName`.

Tests are self-contained and need no network or services: DB tests copy the committed
`dbdata/messages.db` fixture into a temp dir (see `openTestDB` in `db_test.go`), and the inference
client / HTTP handlers use `httptest` stubs.

## Build & deploy specifics

- **CGO-free by design.** The SQLite driver is `modernc.org/sqlite` (pure Go), so builds set
  `CGO_ENABLED=0` and produce a fully static binary. Do not introduce a CGO SQLite driver.
- **Container images are built with [ko](https://ko.build), not a Dockerfile** — there is
  intentionally no Dockerfile. `just image` builds into the local docker daemon as `ko.local:compose`
  (what `docker-compose.yml` expects); `just publish` pushes to GHCR. `.ko.yaml` configures the build.
- The **SQLite DB is runtime state and is never baked into the image** — mount it as a volume and
  point `DB_DATA_PATH` at the mount. `docker-compose.yml` bind-mounts `./.dbdatastuff` → `/app/dbdata`
  and reaches the inference service at `http://breadvision:8000`.

## Architecture

Single process, wired in `cmd/breadbot/main.go` in this order: load `.env` → load config → open DB
(auto-creates schema) → ensure `downloads/{,plots,predictions}` dirs → start HTTP server (goroutine)
→ open the discordgo session. Blocks on SIGINT/SIGTERM, then shuts down gracefully.

Package layout (`internal/`):

- **`config/`** — env-var config (`.env` auto-loaded for local dev via `dotenv.go`). `DISCORD_TOKEN`
  required; all else defaults. Channel/role lists use the Python-style `[1,2,3]` string format
  (`parseIntList`). See the README config table for every variable.
- **`db/`** — SQLite layer shared by both the bot and the HTTP server. Tables (`messages`,
  `discordusers`, plus `botstate` for catch-up) created idempotently. The schema started identical
  to the Python version, but has since diverged additively via **backward-compatible migrations**
  run on every `Open` (`migrateMessagesSchema`): the existing DB file is still reused in place.
  `messages` is now keyed by the composite **`(ogmessage_id, attachment_id)`** — one row per image
  attachment, since a message can carry several images each scored independently. A legacy
  single-key DB is rebuilt in place with old rows getting `attachment_id 0`. An `image_filename`
  column links a row to its annotated PNG under `downloads/predictions/` (for the frontend gallery).
  `ErrUserNotFound` mirrors the Python exception.
- **`inference/`** — HTTP client for the microservice. POSTs base64 image bytes to
  `{base}/predict/predict` (the doubled segment is intentional, matching the Python client).
- **`bot/`** — the discordgo session, event routing, bread pipeline, and commands.
- **`stats/`** — roundness-history PNG chart (`gonum.org/v1/plot`).
- **`httpserver/`** — read-only stats API (uses Go 1.22+ `net/http` `{id}` path patterns, no router).

### Message flow (`bot/`)

discordgo has **no command framework** (unlike discord.py's `commands.Bot`), so dispatch is explicit:

`onMessageCreate` (messages.go) → ignore self → upsert author into `discordusers` on *every* message
→ if it parses as a registered `$`-command (`parseCommand`/`isRegisteredCommand`) dispatch it
(commands.go), else fall through to `onPlainMessage` (bread.go). `parseCommand` mirrors discord.py:
only a message starting with `$` **and** naming a registered command (`help`/`hello`/`breadstats`) is
a command; `$unknown` and plain text both fall through to the bread pipeline.

Bread pipeline (bread.go): `isBreadCandidate` gates on allowed channel + author role + attachment,
then `sendBreadMessage` calls inference (always with threshold 0 — the bot filters client-side against
`BreadDetectionConfidence`), renders a verdict, replies, and persists stats. The whole plain-message
handler runs under a `recover()` guard so one failure never crashes the handler (mirrors the Python
broad try/except).

### Two non-obvious behaviors — read before touching bread.go

0. **Multi-image messages.** A message can carry several image attachments; each is downloaded,
   inferred, and persisted **independently**. Filenames are namespaced `{attachmentID}_{name}`
   (`attachmentFilename` in bread.go) because Discord filenames aren't unique — the old bare-name
   scheme let attachments clobber each other on disk (only the last image survived) and predictions
   collide across messages. The attachment id threads through the whole pipeline: it's half the DB
   composite key, half the predCache key, and the `savedAttachment.id` returned by `saveAttachments`.

1. **Prediction cache (`predcache.go`).** The service does a single-pass detection and returns *every*
   label, so re-running yields the same result. The bot keeps the last `predCacheSize` (currently 8)
   full predictions in an in-memory LRU keyed by **`predKey{ogMessageID, attachmentID}`** (so a
   multi-image message gets one entry per image, not one that clobbers the rest), storing the
   already-annotated image path. **Note the README still says "32" — the code is the source of truth
   (`predCacheSize`).**

2. **"Are you sure?" retry.** A reply containing "are you sure"/"no way" to one of the bot's own
   messages re-renders the verdict at the lower `OverrideDetectionConfidence`. It iterates **every**
   attachment of the original post: each cached image re-renders straight from the cached response +
   image (no inference call), and only the ones that miss (restart or eviction) get re-downloaded and
   re-inferred. `minConfidence` in `renderBreadMessage` gates **both** the is-it-bread decision and
   the per-label sentiments — the Python version only relaxed the sentiments (so the retry did
   nothing); the Go port fixes that.

`commands.go` similarly documents fixed-from-Python bugs in `$breadstats --top` (correct `n` parsing,
correct "Worst n" label). When editing these, preserve the fixes — don't regress to the Python behavior.

### ID handling

Discord snowflakes are strings on the wire and `int64` in the DB. `bot/ids.go` centralizes conversion:
`mustParseID` logs and returns 0 on malformed data rather than crashing the handler. A message fetched
via REST (`ChannelMessage`) has an empty `GuildID` unlike a gateway object — `handleAreYouSure`
backfills it (see the comment there) before persisting.

## HTTP API

Read-only, same process as the bot. `GET /healthz` is always unauthenticated (reports DB reachable +
Discord session ready). `/api/*` endpoints require `Authorization: Bearer <ADMIN_API_TOKEN>` **only
when `ADMIN_API_TOKEN` is set** (empty token = auth off). See README for the endpoint list.
