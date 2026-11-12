package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Stat struct {
	ItemID string
	Online bool
	Bytes  int64
	Error  string
	At     int64
}

type StatsStore struct {
	db *sql.DB
}

func OpenStats(path string) (*StatsStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, stmt := range []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
		`CREATE TABLE IF NOT EXISTS capacity_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id TEXT NOT NULL,
    online INTEGER NOT NULL,
    bytes INTEGER NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    recorded_at INTEGER NOT NULL
)`,
		`CREATE INDEX IF NOT EXISTS idx_capacity_item_time ON capacity_stats(item_id, recorded_at)`,
		`CREATE TABLE IF NOT EXISTS targets (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    kind INTEGER NOT NULL,
    dsn TEXT NOT NULL,
    created_at INTEGER NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS admin_auth (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    access_key_hash TEXT NOT NULL,
    totp_secret TEXT NOT NULL,
    last_step INTEGER NOT NULL DEFAULT 0
)`,
		`CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,
    expires_at INTEGER NOT NULL
)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("初始化 sqlite: %w", err)
		}
	}
	return &StatsStore{db: db}, nil
}

func (s *StatsStore) Close() error {
	return s.db.Close()
}

func (s *StatsStore) Insert(ctx context.Context, stat Stat) error {
	if stat.At == 0 {
		stat.At = time.Now().Unix()
	}
	online := 0
	if stat.Online {
		online = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO capacity_stats (item_id, online, bytes, error, recorded_at) VALUES (?, ?, ?, ?, ?)`,
		stat.ItemID, online, stat.Bytes, stat.Error, stat.At,
	)
	return err
}

func (s *StatsStore) Latest(ctx context.Context) (map[string]Stat, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT item_id, online, bytes, error, recorded_at
FROM capacity_stats
WHERE id IN (SELECT MAX(id) FROM capacity_stats GROUP BY item_id)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Stat{}
	for rows.Next() {
		var stat Stat
		var online int
		if err := rows.Scan(&stat.ItemID, &online, &stat.Bytes, &stat.Error, &stat.At); err != nil {
			return nil, err
		}
		stat.Online = online == 1
		out[stat.ItemID] = stat
	}
	return out, rows.Err()
}

func (s *StatsStore) History(ctx context.Context, itemID string, limit int) ([]Stat, error) {
	if limit <= 0 {
		limit = 168
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT online, bytes, error, recorded_at
FROM capacity_stats
WHERE item_id = ?
ORDER BY recorded_at DESC, id DESC
LIMIT ?`, itemID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Stat{}
	for rows.Next() {
		var stat Stat
		var online int
		if err := rows.Scan(&online, &stat.Bytes, &stat.Error, &stat.At); err != nil {
			return nil, err
		}
		stat.ItemID = itemID
		stat.Online = online == 1
		out = append(out, stat)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (s *StatsStore) DeleteItem(ctx context.Context, itemID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM capacity_stats WHERE item_id = ?`, itemID)
	return err
}

func newID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func (s *StatsStore) InsertTarget(ctx context.Context, item itemDoc) (itemDoc, error) {
	if item.ID == "" {
		id, err := newID()
		if err != nil {
			return itemDoc{}, err
		}
		item.ID = id
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO targets (id, name, kind, dsn, created_at) VALUES (?, ?, ?, ?, ?)`,
		item.ID, item.Name, item.Type, item.DSN, time.Now().Unix(),
	)
	return item, err
}

func (s *StatsStore) ListTargets(ctx context.Context) ([]itemDoc, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, kind, dsn FROM targets ORDER BY name, id LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []itemDoc{}
	for rows.Next() {
		var item itemDoc
		if err := rows.Scan(&item.ID, &item.Name, &item.Type, &item.DSN); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *StatsStore) GetTarget(ctx context.Context, id string) (itemDoc, error) {
	var item itemDoc
	err := s.db.QueryRowContext(ctx, `SELECT id, name, kind, dsn FROM targets WHERE id = ?`, id).
		Scan(&item.ID, &item.Name, &item.Type, &item.DSN)
	if errors.Is(err, sql.ErrNoRows) {
		return itemDoc{}, errNotFound
	}
	return item, err
}

func (s *StatsStore) UpdateTarget(ctx context.Context, item itemDoc) error {
	res, err := s.db.ExecContext(ctx, `UPDATE targets SET name = ?, kind = ?, dsn = ? WHERE id = ?`, item.Name, item.Type, item.DSN, item.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errNotFound
	}
	return nil
}

func (s *StatsStore) DeleteTarget(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM targets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM capacity_stats WHERE item_id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
