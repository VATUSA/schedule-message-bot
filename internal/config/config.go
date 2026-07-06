// Package config loads and validates the bot's runtime configuration from the
// environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration for the bot.
type Config struct {
	// Token is the Discord bot token. Required.
	Token string
	// GuildID, if set, scopes slash-command registration to a single guild,
	// which makes command updates appear instantly (global commands can take up
	// to an hour to propagate). Leave empty to register commands globally.
	GuildID string
	// RequiredRoleIDs is the set of role IDs, any one of which a member must
	// hold to use the bot's commands. If empty, no role restriction is applied.
	RequiredRoleIDs []string
	// DatabasePath is the filesystem path to the SQLite database.
	DatabasePath string
	// PollInterval is how often the scheduler checks for due messages.
	PollInterval time.Duration
}

// Load reads configuration from environment variables, applying defaults and
// validating required values. If a .env file is present in the working
// directory, its values are loaded first without overriding variables already
// set in the environment.
func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("loading .env: %w", err)
	}

	cfg := &Config{
		Token:           os.Getenv("DISCORD_TOKEN"),
		GuildID:         os.Getenv("DISCORD_GUILD_ID"),
		RequiredRoleIDs: splitAndTrim(os.Getenv("REQUIRED_ROLE_IDS")),
		DatabasePath:    envOrDefault("DATABASE_PATH", "data/schedule.db"),
		PollInterval:    envDurationOrDefault("POLL_INTERVAL", 15*time.Second),
	}

	if cfg.Token == "" {
		return nil, fmt.Errorf("DISCORD_TOKEN is required")
	}
	if cfg.PollInterval <= 0 {
		return nil, fmt.Errorf("POLL_INTERVAL must be positive, got %s", cfg.PollInterval)
	}

	return cfg, nil
}

// splitAndTrim splits a comma-separated list, trimming whitespace and dropping
// empty entries. It returns nil for an empty or whitespace-only input.
func splitAndTrim(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDurationOrDefault(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
