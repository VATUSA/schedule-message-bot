// Package storage persists scheduled messages in a SQLite database so that
// pending messages survive bot restarts.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no cgo)
)

// Status represents the lifecycle state of a scheduled message.
type Status string

// Lifecycle states a scheduled message moves through.
const (
	StatusPending   Status = "pending"
	StatusSent      Status = "sent"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// ScheduledMessage is a single message queued to be sent at a future time.
type ScheduledMessage struct {
	ID        int64
	ChannelID string
	GuildID   string
	Content   string
	ImageURL  string
	SendAt    time.Time
	CreatedBy string
	CreatedAt time.Time
	Status    Status
}

// Store wraps the database connection and provides persistence operations.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS scheduled_messages (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	channel_id TEXT    NOT NULL,
	guild_id   TEXT    NOT NULL DEFAULT '',
	content    TEXT    NOT NULL,
	image_url  TEXT    NOT NULL DEFAULT '',
	send_at    INTEGER NOT NULL,
	created_by TEXT    NOT NULL,
	created_at INTEGER NOT NULL,
	status     TEXT    NOT NULL DEFAULT 'pending'
);
CREATE INDEX IF NOT EXISTS idx_scheduled_messages_due
	ON scheduled_messages (status, send_at);
`

// Open opens (creating if necessary) the SQLite database at path and ensures
// the schema is present.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// SQLite supports only one writer; a single connection avoids
	// "database is locked" errors under concurrent access.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Create inserts a new pending scheduled message and returns its assigned ID.
func (s *Store) Create(ctx context.Context, m *ScheduledMessage) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO scheduled_messages
			(channel_id, guild_id, content, image_url, send_at, created_by, created_at, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ChannelID, m.GuildID, m.Content, m.ImageURL,
		m.SendAt.Unix(), m.CreatedBy, time.Now().Unix(), StatusPending,
	)
	if err != nil {
		return 0, fmt.Errorf("insert scheduled message: %w", err)
	}
	return res.LastInsertId()
}

// ListPending returns all pending messages ordered by send time. If guildID is
// non-empty, results are restricted to that guild.
func (s *Store) ListPending(ctx context.Context, guildID string) ([]ScheduledMessage, error) {
	query := `SELECT id, channel_id, guild_id, content, image_url, send_at, created_by, created_at, status
		FROM scheduled_messages WHERE status = ?`
	args := []any{StatusPending}
	if guildID != "" {
		query += ` AND guild_id = ?`
		args = append(args, guildID)
	}
	query += ` ORDER BY send_at ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query pending: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ScheduledMessage
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DueBefore returns all pending messages whose send time is at or before t.
func (s *Store) DueBefore(ctx context.Context, t time.Time) ([]ScheduledMessage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, channel_id, guild_id, content, image_url, send_at, created_by, created_at, status
		 FROM scheduled_messages
		 WHERE status = ? AND send_at <= ?
		 ORDER BY send_at ASC`,
		StatusPending, t.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("query due: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ScheduledMessage
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Get returns the scheduled message with the given ID, or sql.ErrNoRows if it
// does not exist.
func (s *Store) Get(ctx context.Context, id int64) (ScheduledMessage, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, channel_id, guild_id, content, image_url, send_at, created_by, created_at, status
		 FROM scheduled_messages WHERE id = ?`, id)
	return scanMessage(row)
}

// UpdateStatus sets the status of the message with the given ID.
func (s *Store) UpdateStatus(ctx context.Context, id int64, status Status) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE scheduled_messages SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

// Cancel marks a pending message as cancelled. It reports whether a pending
// message with that ID existed.
func (s *Store) Cancel(ctx context.Context, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE scheduled_messages SET status = ?
		 WHERE id = ? AND status = ?`, StatusCancelled, id, StatusPending)
	if err != nil {
		return false, fmt.Errorf("cancel message: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// scanner abstracts over *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanMessage(sc scanner) (ScheduledMessage, error) {
	var (
		m                 ScheduledMessage
		sendAt, createdAt int64
	)
	err := sc.Scan(&m.ID, &m.ChannelID, &m.GuildID, &m.Content, &m.ImageURL,
		&sendAt, &m.CreatedBy, &createdAt, &m.Status)
	if err != nil {
		return ScheduledMessage{}, err
	}
	m.SendAt = time.Unix(sendAt, 0).UTC()
	m.CreatedAt = time.Unix(createdAt, 0).UTC()
	return m, nil
}
