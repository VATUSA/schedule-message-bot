# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.26-alpine AS build

WORKDIR /src

# Cache dependencies separately from source for faster rebuilds.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO is disabled: modernc.org/sqlite is pure Go, so we get a static binary.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bot . \
	&& mkdir -p /out/data

# ---- Runtime stage ----
# Named `app` because the shared gitops build workflow (common-build.yml) builds
# with `target: app`.
FROM gcr.io/distroless/static-debian12:nonroot AS app

WORKDIR /app

# Persist the SQLite database here; mount a volume at /app/data in production.
ENV DATABASE_PATH=/app/data/schedule.db

COPY --from=build /out/bot /app/bot
# Pre-create the data directory owned by the nonroot user so the bot can write
# its database even when no named volume is mounted.
COPY --from=build --chown=nonroot:nonroot /out/data /app/data

USER nonroot:nonroot
ENTRYPOINT ["/app/bot"]
