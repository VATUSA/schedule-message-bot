# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

A Discord bot for the VATUSA server that schedules messages to be posted to a
channel at a future time. Go + [`bwmarrin/discordgo`](https://github.com/bwmarrin/discordgo),
with scheduled messages persisted in SQLite (pure-Go `modernc.org/sqlite`, so
builds need no cgo). It replaces an earlier Node/discord.js prototype.

## Commands

```sh
go build ./...     # compile
go test ./...      # run tests
go test -race ./... # tests with the race detector (as CI does)
go vet ./...       # static analysis
golangci-lint run  # lint; config in .golangci.yml
go run .           # run locally (needs DISCORD_TOKEN in the environment)
```

## Architecture

The flow is intentionally simple and database-backed rather than relying on
in-memory timers, so it survives restarts.

- `main.go` — loads config, opens storage, starts the Discord session and the
  scheduler goroutine, and handles graceful shutdown on SIGINT/SIGTERM.
- `internal/config` — all configuration comes from environment variables.
  `DISCORD_TOKEN` is required; everything else has a default. A `.env` file in
  the working directory, if present, is loaded first (via `godotenv`) without
  overriding variables already set in the environment.
- `internal/storage` — SQLite-backed CRUD for `ScheduledMessage`. Status moves
  through `pending → sent | failed | cancelled`. Times are stored as Unix
  seconds (UTC). Uses a single open connection (SQLite has one writer).
- `internal/scheduler` — polls `storage.DueBefore(now)` every `POLL_INTERVAL`
  and calls a `Sender` to deliver each due message, then marks it sent/failed.
  `Sender` is an interface so the scheduler is testable without Discord.
- `internal/discord` — defines the slash commands (`commands.go`), handles
  interactions, enforces role-based authorization, and implements `Sender`
  (`bot.Send`).

## Conventions and constraints

- **Never hard-code the bot token** (the prototype did — that token is burned).
  It is read only from `DISCORD_TOKEN`.
- Times entered via `/schedule` are **UTC / Zulu**. Keep all internal time
  handling in UTC.
- Keep the bot to the **Guilds intent only** — it sends but never reads message
  content, so no privileged intents are required.
- `CGO_ENABLED=0` must keep working: do not introduce cgo-dependent SQLite
  drivers (e.g. `mattn/go-sqlite3`). The Dockerfile builds a static binary into
  a distroless image.
- When adding or changing a slash command, update the definition in
  `internal/discord/commands.go`; commands are registered on startup.
- Prefer adding tests for `storage` and `scheduler` (they have no Discord
  dependency). The `discord` package is thin glue and is left untested.

## Deployment

The Dockerfile produces a static binary in a distroless image (final stage is
named `app`, as the shared build workflow expects) and persists the database
under `/app/data` (mount a volume there).

Deployed to the VATUSA Kubernetes cluster via ArgoCD, with manifests in the
`vatusa/gitops` repo under `schedule-message-bot/` (base + `overlays/prod`).
The Deployment runs a **single replica** with a `Recreate` strategy — SQLite has
one writer and the scheduler must not run twice — backed by a `ReadWriteOnce`
PVC mounted at `/app/data`. Config is split: non-secret values
(`DATABASE_PATH`, `POLL_INTERVAL`) come from a ConfigMap; the secrets
(`DISCORD_TOKEN`, `DISCORD_GUILD_ID`, `REQUIRED_ROLE_IDS`) come from a
`schedule-message-bot-secrets` Secret that is created out-of-band in the cluster
(not stored in gitops).

CI/CD workflows (all reuse shared workflows from `vatusa/gitops`):

- `.github/workflows/ci.yml` — PR/branch verification: build, test (`-race`),
  `go vet`, `golangci-lint`, and a Docker smoke build (no push).
- `.github/workflows/ci-master.yml` — on push to `master`, builds and pushes the
  image to Docker Hub as `vatusa/schedule-message-bot:latest` and `:<sha>`, then
  pins the prod overlay in gitops to that SHA; ArgoCD then rolls it out.

This app has no dev environment, so a release is simply: push to `master` →
image builds and deploys straight to prod, no manual gate.
