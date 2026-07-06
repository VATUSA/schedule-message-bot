// Command schedule-message-bot runs a Discord bot that schedules messages to be
// posted to a channel at a future time.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/vatusa/schedule-message-bot/internal/config"
	"github.com/vatusa/schedule-message-bot/internal/discord"
	"github.com/vatusa/schedule-message-bot/internal/scheduler"
	"github.com/vatusa/schedule-message-bot/internal/storage"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(log); err != nil {
		log.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if dir := filepath.Dir(cfg.DatabasePath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	store, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	bot, err := discord.New(cfg.Token, cfg.GuildID, cfg.RequiredRoleIDs, store, log)
	if err != nil {
		return err
	}
	if err := bot.Open(); err != nil {
		return err
	}
	defer func() { _ = bot.Close() }()

	// Cancel the context on SIGINT/SIGTERM to trigger a graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sched := scheduler.New(store, bot, cfg.PollInterval, log)
	go sched.Run(ctx)

	log.Info("bot running; press Ctrl+C to stop")
	<-ctx.Done()
	log.Info("shutting down")
	return nil
}
