package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vatusa/schedule-message-bot/internal/storage"
)

type fakeSender struct {
	mu   sync.Mutex
	sent []int64
	fail bool
}

func (f *fakeSender) Send(m storage.ScheduledMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("boom")
	}
	f.sent = append(f.sent, m.ID)
	return nil
}

func newStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDispatchDueSendsAndMarksSent(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	sender := &fakeSender{}

	id, err := store.Create(ctx, &storage.ScheduledMessage{
		ChannelID: "c", Content: "hi", SendAt: time.Now().Add(-time.Minute), CreatedBy: "u",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	New(store, sender, time.Second, discardLogger()).dispatchDue(ctx)

	if len(sender.sent) != 1 || sender.sent[0] != id {
		t.Fatalf("expected message %d sent, got %v", id, sender.sent)
	}
	got, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != storage.StatusSent {
		t.Errorf("status = %q, want %q", got.Status, storage.StatusSent)
	}
}

func TestDispatchDueMarksFailedOnError(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	sender := &fakeSender{fail: true}

	id, err := store.Create(ctx, &storage.ScheduledMessage{
		ChannelID: "c", Content: "hi", SendAt: time.Now().Add(-time.Minute), CreatedBy: "u",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	New(store, sender, time.Second, discardLogger()).dispatchDue(ctx)

	got, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != storage.StatusFailed {
		t.Errorf("status = %q, want %q", got.Status, storage.StatusFailed)
	}
}

func TestDispatchDueIgnoresFutureMessages(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	sender := &fakeSender{}

	if _, err := store.Create(ctx, &storage.ScheduledMessage{
		ChannelID: "c", Content: "later", SendAt: time.Now().Add(time.Hour), CreatedBy: "u",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	New(store, sender, time.Second, discardLogger()).dispatchDue(ctx)

	if len(sender.sent) != 0 {
		t.Errorf("expected no messages sent, got %v", sender.sent)
	}
}
