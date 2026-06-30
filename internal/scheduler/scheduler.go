// Package scheduler periodically dispatches due scheduled messages.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/vatusa/schedule-message-bot/internal/storage"
)

// Sender delivers a scheduled message to its destination channel. It is
// implemented by the Discord layer and kept as an interface so the scheduler
// can be tested in isolation.
type Sender interface {
	Send(m storage.ScheduledMessage) error
}

// Scheduler polls the store for due messages and sends them.
type Scheduler struct {
	store    *storage.Store
	sender   Sender
	interval time.Duration
	log      *slog.Logger
}

// New constructs a Scheduler.
func New(store *storage.Store, sender Sender, interval time.Duration, log *slog.Logger) *Scheduler {
	return &Scheduler{store: store, sender: sender, interval: interval, log: log}
}

// Run blocks until ctx is cancelled, dispatching due messages every interval.
// It performs an initial pass immediately on start so that messages which came
// due while the bot was offline are sent promptly.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.dispatchDue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.dispatchDue(ctx)
		}
	}
}

func (s *Scheduler) dispatchDue(ctx context.Context) {
	due, err := s.store.DueBefore(ctx, time.Now())
	if err != nil {
		s.log.Error("query due messages", "error", err)
		return
	}

	for _, m := range due {
		if ctx.Err() != nil {
			return
		}
		if err := s.sender.Send(m); err != nil {
			s.log.Error("send scheduled message", "id", m.ID, "channel", m.ChannelID, "error", err)
			if err := s.store.UpdateStatus(ctx, m.ID, storage.StatusFailed); err != nil {
				s.log.Error("mark message failed", "id", m.ID, "error", err)
			}
			continue
		}
		if err := s.store.UpdateStatus(ctx, m.ID, storage.StatusSent); err != nil {
			s.log.Error("mark message sent", "id", m.ID, "error", err)
		}
		s.log.Info("sent scheduled message", "id", m.ID, "channel", m.ChannelID)
	}
}
