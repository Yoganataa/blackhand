package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct { DB *sql.DB }

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "db"), 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "db", "data.sqlite"))
	if err != nil { return nil, fmt.Errorf("open sqlite: %w", err) }
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	if _, err := db.Exec("PRAGMA busy_timeout = 5000; PRAGMA foreign_keys = ON;"); err != nil {
		db.Close(); return nil, fmt.Errorf("configure sqlite: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil { db.Close(); return nil, fmt.Errorf("ping sqlite: %w", err) }
	if err := migrate(ctx, db); err != nil { db.Close(); return nil, err }
	return &Store{DB: db}, nil
}
func (s *Store) Close() error { return s.DB.Close() }
func migrate(ctx context.Context, db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS app_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value_json TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS api_keys (id TEXT PRIMARY KEY, key_hash TEXT NOT NULL UNIQUE, name TEXT NOT NULL, active INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS usage_events (id INTEGER PRIMARY KEY AUTOINCREMENT, created_at TEXT NOT NULL, provider TEXT NOT NULL, model TEXT NOT NULL, status INTEGER NOT NULL, prompt_tokens INTEGER NOT NULL DEFAULT 0, completion_tokens INTEGER NOT NULL DEFAULT 0, cost REAL NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS idx_usage_events_created_at ON usage_events(created_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_provider_model ON usage_events(provider, model);
CREATE TABLE IF NOT EXISTS proxy_pools (id TEXT PRIMARY KEY, name TEXT NOT NULL, proxy_url TEXT NOT NULL, no_proxy TEXT NOT NULL DEFAULT '', type TEXT NOT NULL DEFAULT 'http', active INTEGER NOT NULL DEFAULT 1, strict_proxy INTEGER NOT NULL DEFAULT 0, test_status TEXT NOT NULL DEFAULT 'unknown', last_tested_at TEXT, last_error TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);`
	if _, err := db.ExecContext(ctx, schema); err != nil { return fmt.Errorf("apply initial blackhand schema: %w", err) }
	return nil
}
