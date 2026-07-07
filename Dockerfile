# Multi-stage build: compile a static, CGO-free binary, then ship it on a
# minimal distroless base. Replaces the old Python/uvicorn image.

FROM golang:1.26 AS build
WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

# Build. CGO disabled -> modernc.org/sqlite (pure Go) means a fully static binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /breadbot ./cmd/breadbot

# Runtime image: distroless static (CA certs for HTTPS to Discord + the
# inference service; nonroot for safety).
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

# Bring over the binary and the existing database.
COPY --from=build /breadbot /breadbot
COPY --chown=nonroot:nonroot dbdata/ /app/dbdata/

# The read-only stats API.
EXPOSE 8080

# Config comes from the environment (see .env.example / README).
CMD ["/breadbot"]
