package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func sampleMessage(sendAt time.Time) *ScheduledMessage {
	return &ScheduledMessage{
		ChannelID: "chan-1",
		GuildID:   "guild-1",
		Content:   "hello world",
		SendAt:    sendAt,
		CreatedBy: "user-1",
	}
}

func TestCreateAndGet(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	want := sampleMessage(time.Now().Add(time.Hour).UTC())
	id, err := store.Create(ctx, want)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}

	got, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != want.Content || got.ChannelID != want.ChannelID {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
	if got.Status != StatusPending {
		t.Errorf("status = %q, want %q", got.Status, StatusPending)
	}
	if !got.SendAt.Equal(want.SendAt.Truncate(time.Second)) {
		t.Errorf("send_at = %v, want %v", got.SendAt, want.SendAt.Truncate(time.Second))
	}
}

func TestDueBefore(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	past, err := store.Create(ctx, sampleMessage(time.Now().Add(-time.Minute)))
	if err != nil {
		t.Fatalf("Create past: %v", err)
	}
	if _, err := store.Create(ctx, sampleMessage(time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("Create future: %v", err)
	}

	due, err := store.DueBefore(ctx, time.Now())
	if err != nil {
		t.Fatalf("DueBefore: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("expected 1 due message, got %d", len(due))
	}
	if due[0].ID != past {
		t.Errorf("due id = %d, want %d", due[0].ID, past)
	}
}

func TestCancel(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	id, err := store.Create(ctx, sampleMessage(time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ok, err := store.Cancel(ctx, id)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if !ok {
		t.Fatal("expected Cancel to report success")
	}

	// Cancelling again should report no pending message.
	ok, err = store.Cancel(ctx, id)
	if err != nil {
		t.Fatalf("Cancel again: %v", err)
	}
	if ok {
		t.Error("expected second Cancel to report no pending message")
	}

	// A cancelled message must not be returned as due.
	due, err := store.DueBefore(ctx, time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatalf("DueBefore: %v", err)
	}
	if len(due) != 0 {
		t.Errorf("expected no due messages after cancel, got %d", len(due))
	}
}

func TestListPendingFiltersByGuildAndStatus(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	if _, err := store.Create(ctx, sampleMessage(time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("Create: %v", err)
	}
	other := sampleMessage(time.Now().Add(time.Hour))
	other.GuildID = "guild-2"
	if _, err := store.Create(ctx, other); err != nil {
		t.Fatalf("Create other: %v", err)
	}
	sent := sampleMessage(time.Now().Add(time.Hour))
	sentID, err := store.Create(ctx, sent)
	if err != nil {
		t.Fatalf("Create sent: %v", err)
	}
	if err := store.UpdateStatus(ctx, sentID, StatusSent); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	pending, err := store.ListPending(ctx, "guild-1")
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending message for guild-1, got %d", len(pending))
	}
	if pending[0].GuildID != "guild-1" || pending[0].Status != StatusPending {
		t.Errorf("unexpected message: %+v", pending[0])
	}
}
