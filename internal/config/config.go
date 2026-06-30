// Package config loads and validates the bot's runtime configuration from the
// environment.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all runtime configuration for the bot.
type Config struct {
	// Token is the Discord bot token. Required.
	Token string
	// GuildID, if set, scopes slash-command registration to a single guild,
	// which makes command updates appear instantly (global commands can take up
	// to an hour to propagate). Leave empty to register commands globally.
	GuildID string
	// RequiredRoleID is the ID of the role a member must hold to use the bot's
	// commands. If empty, no role restriction is applied.
	RequiredRoleID string
	// DatabasePath is the filesystem path to the SQLite database.
	DatabasePath string
	// PollInterval is how often the scheduler checks for due messages.
	PollInterval time.Duration
}

// Load reads configuration from environment variables, applying defaults and
// validating required values.
func Load() (*Config, error) {
	cfg := &Config{
		Token:          os.Getenv("DISCORD_TOKEN"),
		GuildID:        os.Getenv("DISCORD_GUILD_ID"),
		RequiredRoleID: strings.TrimSpace(os.Getenv("REQUIRED_ROLE_ID")),
		DatabasePath:   envOrDefault("DATABASE_PATH", "data/schedule.db"),
		PollInterval:   envDurationOrDefault("POLL_INTERVAL", 15*time.Second),
	}

	if cfg.Token == "" {
		return nil, fmt.Errorf("DISCORD_TOKEN is required")
	}
	if cfg.PollInterval <= 0 {
		return nil, fmt.Errorf("POLL_INTERVAL must be positive, got %s", cfg.PollInterval)
	}

	return cfg, nil
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
