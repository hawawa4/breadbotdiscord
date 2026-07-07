# BreadBot — Python → Go Migration Plan

## Goal
Reimplement the existing Python discord.py bot in Go, preserving all current behavior, and add a **thin read-only HTTP server** running alongside the bot so we can later build an admin panel on top of it.

## Confirmed decisions
- **Discord:** `bwmarrin/discordgo`
- **SQLite:** `modernc.org/sqlite` (pure Go, no CGO) — reuse the existing `dbdata/messages.db` unchanged
- **HTTP server:** read-only stats API + `/healthz`
- **Charting:** render the roundness history PNG server-side in Go (attach to the Discord message, matching current behavior)

## Open questions to confirm at start of execution
1. **`$breadstats --top` bugs** — plan defaults to **fixing** them (parse the real `n` arg instead of `args[2]`; label the worst list with the actual `n` instead of hardcoded "Worst 3"). Confirm or request bug-for-bug parity.
2. **Go module path** — need the `go.mod` module name (e.g. `github.com/hawawa4/breadbotdiscord`).

---

## Source-of-truth: current Python implementation (files)
- `src/main.py` — entry point (create DB → mkdir downloads → run bot)
- `src/registry.py` — DI container wiring settings/db/inference/bot
- `src/settings.py` — pydantic-settings config (has `env_prefix="__"` quirk; `.env` uses unprefixed names)
- `src/discordclient/service.py` — bot, commands, event handlers
- `src/discordclient/plain_message.py` — `FreeMessageHandler`: detection + response text (the live logic)
- `src/inference/predict.py` — microservice HTTP client + request/response models
- `src/inference/mapper.py` — `ResultsMapper`: **dead code**, do NOT port
- `src/db/service.py` — SQLite access + all queries
- `src/db/models.py` — `Message` / `User` models + SQL helpers
- `src/stats/plots.py` — matplotlib/seaborn roundness plot

---

## Target Go layout

```
breadbotdiscord/
  go.mod / go.sum
  cmd/breadbot/main.go        # entrypoint: load config, open DB, start HTTP server + bot
  internal/
    config/config.go          # env-based config (replaces settings.py)
    db/
      db.go                   # sqlite open + schema create (CREATE TABLE IF NOT EXISTS)
      messages.go             # message upserts + roundness queries
      users.go                # user upsert + select
      models.go               # Message, User structs + row scanning
    inference/
      client.go               # HTTP client for the microservice (POST /predict/predict)
    bot/
      bot.go                  # discordgo session, intents, handler registration, on_ready
      messages.go             # onMessage: route commands vs plain messages
      bread.go                # candidate detection + "are you sure" + compute pipeline
      responses.go            # confidence->sentiment, roundness/label message text
      commands.go             # $hello, $breadstats (--history/--self/--top)
    stats/
      plot.go                 # roundness history PNG
    httpserver/
      server.go               # chi/net-http router, mounts handlers, healthz
      handlers.go             # read-only stats JSON endpoints
  Dockerfile                  # multi-stage static Go build (fixes the stale uvicorn CMD)
  .env.example                # documented env vars
  dbdata/messages.db          # existing data, reused as-is
  downloads/                  # runtime image + plot output (created on startup)
```

Python `src/` stays during development for reference, then is removed at the parity/cleanup phase.

---

## Behavior to preserve (parity checklist)

### Discord event flow (`on_message`)
1. Ignore the bot's own messages.
2. Upsert author into `discordusers` on **every** message (name + nickname).
3. If content is a valid `$`-command → dispatch command. Otherwise → plain-message path.

### Plain-message path (`predict`)
- **Bread candidate** = channel ∈ `DISCORD_BREAD_CHANNELS` **and** author has a role in `DISCORD_BREAD_ROLE` **and** ≥1 attachment.
  → download each attachment to `downloads/`, run inference at `BREAD_DETECTION_CONFIDENCE` (0.5), reply + persist.
- **"Are you sure" retry** = message is a reply to a bot message **and** text contains "are you sure" / "no way".
  → resolve the *original* bread message (reply → bot reply → original), re-run inference at `OVERRIDE_DETECTION_CONFIDENCE` (0.1) on **the new message's** attachments, persist against the original message id.
- Wrap the whole thing so one failure logs and doesn't crash the handler (matches the current broad try/except). Show a typing indicator during inference.

### Response decision tree (`compute_bread_message_for_file`)
- `labels` has `"bread"`:
  - `labels["bread"] > 0.5`:
    - build label sentence ("This is certainly bread! " + per-label sentiment for confidence ≥ `min_confidence`)
    - if annotated `image` present → save it, append roundness sentence, return annotated image
    - else → return original file + the "dough/though" pun (". I couldn't find the shape dough. (Get it? Though - dough ehehehehe)")
  - else → "This is only very mildly bread. Metaphysical bread even." (original file)
- no bread label → "This isn't bread at all!" (original file)

**`map_confidence_to_sentiment` exact table** (replace `_`→space in label first):
- `<0.3` → `"{label}, need help"`
- `<0.4` → `"not sure about {label}"`
- `<0.5` → `"{label} is unlikely"`
- `<0.6` → `"slightly possible {label}"`
- `<0.7` → `"moderately likely {label}"`
- `<0.8` → `"probably {label}"`
- `<0.9` → `"fairly confident in {label}"`
- `<1.0` → `"pretty sure it's {label}"`
- `>=1.0` → `"Confirmed that it's {label}"`

**`get_message_from_roundness`**: `roundness is None` → `"I don't think this bread is round at all..."`; else `"This bread seems {roundness*100:.2f}% round. Anything over an 80% is pretty close to a sphere!"`.

### Commands
- `$hello` → "Hello!" (reply).
- `$breadstats`:
  - no arg → "Not enough arguments"
  - `--history` → render roundness-history PNG, attach it
  - `--self` → min & max roundness for caller (percent + jump URLs)
  - `--top [n]` / anything else → server leaderboard (best + worst)
  - Number parsing: n from the arg, clamp >10 → 10 with joke text `" (You're asking too much, nobody has seen a top {limit} ever)"`, invalid → 3 with `" (You didn't enter a valid number. Shame on you)"`.
  - **Bug to FIX (per decision):** Python reads `args[2]` (should be the actual n arg) and hardcodes "Worst 3" — fix to parse the real n and label worst list with the same n.

### Persistence (reuse existing DB, identical schema)
```sql
CREATE TABLE IF NOT EXISTS messages (
    ogmessage_id INTEGER PRIMARY KEY,
    replymessage_jump_url TEXT,
    replymessage_id INTEGER,
    author_id INTEGER,
    channel_id INTEGER,
    guild_id INTEGER,
    roundness REAL,
    labels_json TEXT
);
CREATE TABLE IF NOT EXISTS discordusers (
    author_id INTEGER PRIMARY KEY,
    author_nickname TEXT,
    author_name TEXT
);
```
- `upsert_message_stats(ogmessage_id, roundness, labels_json)` — `labels_json` = JSON string of `{label: confidence}`.
- `upsert_message_discordinfo(ogmessage_id, replymessage_jump_url, replymessage_id, author_id, channel_id, guild_id)`.
- `upsert_user_info(user)` on every message.
- Queries: min/max roundness per user (`WHERE author_id=? AND roundness NOT NULL ORDER BY roundness {dir}, ogmessage_id {dir} LIMIT 1`); min/max leaderboard (`WHERE roundness not null ORDER BY roundness {dir} LIMIT ?`); last-50 history (`WHERE roundness not null AND author_id=? ORDER BY ogmessage_id DESC LIMIT 50`, returned as `[(index, roundness)]`).
- Both upserts key on `ogmessage_id` and use `ON CONFLICT ... DO UPDATE`.

### Config (env vars — keep current names, drop the `__` prefix quirk)
`DEBUG`, `BREAD_DETECTION_CONFIDENCE` (0.5), `OVERRIDE_DETECTION_CONFIDENCE` (0.1), `DISCORD_TOKEN` (required), `DISCORD_BREAD_CHANNELS` (list of ids, `[1,2,3]` format), `DISCORD_BREAD_ROLE` (list of ids), `DB_DATA_PATH` (default `dbdata/messages.db`), `DOWNLOADS_PATH` (default `downloads/`), `INFERENCE_SERVICE_URL` (default `http://localhost:8000`).
- Parse `[1,2,3]` list format. Omit `FILTER_*` vars (not in the live path) unless found to be needed.

### Inference client
- `POST {INFERENCE_SERVICE_URL}/predict/predict`, JSON `{"image": "<base64 of raw image bytes>"}`, 30s timeout, non-200 → error.
- Response: `{image?: base64 (annotated), roundness?: float, labels?: {str:float}}`.
- Save annotated image (base64-decode) to `downloads/predictions/<name>`.

---

## New: thin HTTP server (read-only)

Runs in a goroutine alongside the bot (single process). `net/http` + a light router (chi or stdlib mux). Optional shared-token auth via `ADMIN_API_TOKEN` env (off if unset).

Endpoints (all JSON):
- `GET /healthz` — liveness (report DB reachable + Discord session ready)
- `GET /api/leaderboard?order=max|min&n=` — leaderboard rows
- `GET /api/users/{id}/roundness` — min/max + last-50 history for a user
- `GET /api/users/{id}` — cached user info
- `GET /api/messages/{ogmessage_id}` — single message stats

Bot and HTTP server share the `internal/db` layer. No write/admin actions this phase.

---

## Build / run / deploy
- `go.mod` module TBD (see open questions), Go 1.22+.
- **Dockerfile:** multi-stage — `golang:1.22` build → `CGO_ENABLED=0 go build` → tiny `distroless`/`alpine` final image, `CMD ["/breadbot"]`. Replaces the broken `uvicorn main:app` CMD.
- Startup order: load config → open DB + create tables → ensure `downloads/`, `downloads/plots`, `downloads/predictions` exist → start HTTP server (goroutine) → open discordgo session (blocks).
- Graceful shutdown on SIGINT/SIGTERM (close session + HTTP server + DB).

---

## Implementation phases
1. **Scaffold**: `go.mod`, config, DB layer (open + schema + all queries) against existing `messages.db`, models. Verify queries return expected rows from the committed DB.
2. **Inference client**: port request/response + base64 image save; unit-test against a stub server.
3. **Bot core**: discordgo session, intents (message content), `on_message` routing, user upsert.
4. **Bread pipeline**: candidate detection, attachment download, compute tree, response text, persistence, "are you sure" retry.
5. **Commands**: `$hello`, `$breadstats` (self/top/history), PNG plot.
6. **HTTP server**: healthz + read-only stats endpoints sharing the DB layer.
7. **Docker + docs**: multi-stage Dockerfile, `.env.example`, README run instructions.
8. **Parity pass + cleanup**: manual smoke test against a test guild + the microservice, then remove Python `src/`, `pyproject.toml`, `uv.lock`, stale Docker CMD.

---

## Verification
- Phase-by-phase: `go build ./...` + `go vet`; targeted `go test` for config parsing, the confidence→sentiment table, DB queries (against a temp copy of `messages.db`), and inference client (httptest stub).
- End-to-end smoke test in a test Discord server with the real inference microservice, exercising: a bread post, a non-bread image, an "are you sure" reply, and all `$breadstats` flags.

## Notes / bugs found (fix in port unless told otherwise)
- `_breadstats_top` reads `args[2]` for `n` (should be the n arg) and hardcodes "Worst 3".
- Stale `uvicorn` dependency and wrong Docker `CMD` in the Python repo — not carried over.
- `inference/mapper.py` (`ResultsMapper`) is dead code — not ported.
