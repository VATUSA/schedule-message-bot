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
  `DISCORD_TOKEN` is required; everything else has a default.
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

The Dockerfile produces a static binary in a distroless image and persists the
database under `/app/data` (mount a volume there). The CI deploy job will be
added later; PR verification (build/test/lint/docker build) lives in
`.github/workflows/ci.yml`.
