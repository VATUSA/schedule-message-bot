# schedule-message-bot

[![CI](https://github.com/vatusa/schedule-message-bot/actions/workflows/ci.yml/badge.svg)](https://github.com/vatusa/schedule-message-bot/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/vatusa/schedule-message-bot.svg)](https://pkg.go.dev/github.com/vatusa/schedule-message-bot)
[![Go Report Card](https://goreportcard.com/badge/github.com/vatusa/schedule-message-bot)](https://goreportcard.com/report/github.com/vatusa/schedule-message-bot)
[![Go Version](https://img.shields.io/github/go-mod/go-version/vatusa/schedule-message-bot)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A Discord bot for the [VATUSA](https://www.vatusa.net) server that lets authorized
staff schedule messages to be posted to a channel at a future time.

Built in Go with [`bwmarrin/discordgo`](https://github.com/bwmarrin/discordgo).
Scheduled messages are persisted in SQLite (via the pure-Go
[`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) driver), so they
survive bot restarts and deploys.

## Commands

All commands are Discord slash commands. If `ALLOWED_ROLE_IDS` is configured,
only members holding one of those roles may use them.

| Command | Description |
| --- | --- |
| `/schedule date:<YYYY-MM-DD> time:<HH:MM> message:<text> channel:<#channel> [image:<url>]` | Schedule a message. `date` and `time` are interpreted as **UTC / Zulu**. Returns a schedule ID. |
| `/cancel id:<id>` | Cancel a pending scheduled message by its ID. |
| `/list` | List all pending scheduled messages for the current server (ephemeral). |

`date` + `time` are parsed as UTC; for example `2026-02-10` + `18:30` schedules
the message for `2026-02-10 18:30Z`. Times in the past are rejected. An optional
direct `image` URL is appended so Discord embeds it.

## Configuration

Configuration is read from environment variables (see [`.env.example`](.env.example)):

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `DISCORD_TOKEN` | **yes** | — | Discord bot token. |
| `DISCORD_GUILD_ID` | no | — | Register commands to a single guild for instant updates. Empty registers globally (propagation can take up to an hour). |
| `ALLOWED_ROLE_IDS` | no | — | Comma-separated role IDs permitted to use commands. Empty allows all members. |
| `DATABASE_PATH` | no | `data/schedule.db` | Path to the SQLite database file. |
| `POLL_INTERVAL` | no | `15s` | How often the scheduler checks for due messages (a Go duration). |

The token is **only** read from the environment — never hard-code it.

## Discord application setup

1. Create an application and bot at the
   [Discord Developer Portal](https://discord.com/developers/applications).
2. Copy the **bot token** into `DISCORD_TOKEN`.
3. Invite the bot to your server with the `bot` and `applications.commands`
   scopes. It needs permission to **View Channels** and **Send Messages** in any
   target channel.
4. Slash commands are registered automatically on startup. The bot requires only
   the Guilds gateway intent (no privileged intents).

## Running locally

```sh
# Provide configuration (copy and edit the example).
cp .env.example .env
# Export it into your shell, then run:
set -a; source .env; set +a   # bash/zsh
go run .
```

## Running with Docker

```sh
docker build -t schedule-message-bot .
docker run --rm \
  -e DISCORD_TOKEN=... \
  -e DISCORD_GUILD_ID=... \
  -e ALLOWED_ROLE_IDS=...,... \
  -v "$(pwd)/data:/app/data" \
  schedule-message-bot
```

Mount a volume at `/app/data` so the SQLite database persists across container
restarts.

## Development

```sh
go build ./...   # compile
go test ./...    # run tests
go vet ./...     # static checks
golangci-lint run  # lint (see .golangci.yml)
```

CI runs build, tests (with `-race`), `go vet`, `golangci-lint`, and a Docker
build on every pull request — see [`.github/workflows/ci.yml`](.github/workflows/ci.yml).

## Project layout

```
main.go                      Entry point: config, wiring, graceful shutdown.
internal/config              Environment-based configuration.
internal/storage             SQLite persistence for scheduled messages.
internal/scheduler           Polls storage and dispatches due messages.
internal/discord             Slash commands, interaction handlers, delivery.
```

## How it works

`/schedule` writes a pending row to SQLite. A background scheduler polls the
database every `POLL_INTERVAL` and sends any message whose time has arrived,
marking it `sent` (or `failed`). Because state lives in the database rather than
in-memory timers, a restart simply resumes — messages that came due while the
bot was offline are sent on the next poll.

## License

[MIT](LICENSE) © Matt Boulanger
