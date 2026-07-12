# BreadBot task runner. Run `just` (or `just help`) to list recipes.
#
# Container images are built with ko (no Dockerfile). Publishing pushes to GHCR;
# override the registry by exporting KO_DOCKER_REPO before calling a recipe.

# Registry for published images. Override with `KO_DOCKER_REPO=... just publish`.
export KO_DOCKER_REPO := env_var_or_default("KO_DOCKER_REPO", "ghcr.io/hawawa4/breadbotdiscord")

# Local docker tag used by docker-compose.yml.
compose_image := "ko.local:compose"

# Show available recipes.
help:
    @just --list

# Run the bot locally (reads .env). Requires a reachable inference service.
run:
    go run ./cmd/breadbot

# Run tests.
test:
    go test ./...

# Vet + build (fast sanity check). Uses whatever is in the embed dir already
# (a committed placeholder exists), so it doesn't rebuild the frontend.
check:
    go vet ./...
    go build ./...

# Build the Svelte SPA into internal/httpserver/frontend/dist (embedded by go:embed).
# `npm ci` is reproducible; falls back to `npm install` if there's no lockfile yet.
frontend:
    cd frontend && (npm ci || npm install)
    cd frontend && npm run build

# Build the static binary to ./breadbot (rebuilds the embedded frontend first).
build: frontend
    CGO_ENABLED=0 go build -o breadbot ./cmd/breadbot

# Build the image into the local docker daemon (tag ko.local:compose).
image: frontend
    KO_DOCKER_REPO=ko.local ko build --bare -t compose ./cmd/breadbot

# Log in to GHCR with the gh CLI token (needs `write:packages`: run `gh auth refresh -h github.com -s write:packages`).
login:
    gh auth token | docker login ghcr.io -u "$(gh api user -q .login)" --password-stdin

# Build AND publish the image to $KO_DOCKER_REPO (tags: latest + git sha).
# Rebuilds the embedded frontend, then logs in via `just login` so ko can push.
publish: frontend login
    ko build --bare -t latest -t "$(git rev-parse --short HEAD)" ./cmd/breadbot

# Build the compose image, then bring the stack up (bot + inference).
up: image
    docker compose up -d

# Tail the bot's logs.
logs:
    docker compose logs -f breadbot

# Stop the stack.
down:
    docker compose down

# Rebuild the image and recreate the running bot (pick up code changes).
redeploy: image
    docker compose up -d

# Remove the built binary.
clean:
    rm -f breadbot
