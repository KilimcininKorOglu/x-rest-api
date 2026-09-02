package store

import (
	"fmt"
	"strings"
)

// schema is applied idempotently on every Open.
const schema = `
CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS admin_users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS admin_sessions (
  id            TEXT PRIMARY KEY,
  admin_user_id INTEGER NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at    DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS accounts (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  label          TEXT NOT NULL UNIQUE,
  auth_token     TEXT NOT NULL,
  ct0            TEXT NOT NULL,
  enabled        INTEGER NOT NULL DEFAULT 1,
  status         TEXT NOT NULL DEFAULT 'ok',
  error_msg      TEXT NOT NULL DEFAULT '',
  cooldown_until DATETIME,
  last_used_at   DATETIME,
  daily_count    INTEGER NOT NULL DEFAULT 0,
  daily_date     TEXT NOT NULL DEFAULT '',
  created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS api_keys (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  name             TEXT NOT NULL,
  key              TEXT NOT NULL UNIQUE,
  enabled          INTEGER NOT NULL DEFAULT 1,
  can_write        INTEGER NOT NULL DEFAULT 0,
  bound_account_id INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
  last_used_at     DATETIME,
  created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS request_logs (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  ts              DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  api_key_id      INTEGER,
  account_id      INTEGER,
  method          TEXT NOT NULL,
  path            TEXT NOT NULL,
  query           TEXT NOT NULL DEFAULT '',
  status          INTEGER NOT NULL,
  duration_ms     INTEGER NOT NULL,
  upstream_status INTEGER,
  error           TEXT NOT NULL DEFAULT '',
  remote_ip       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_request_logs_ts ON request_logs(ts DESC);

CREATE TABLE IF NOT EXISTS query_ids (
  operation  TEXT PRIMARY KEY,
  query_id   TEXT NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS account_locks (
  account_id   INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  op           TEXT NOT NULL,
  unlock_until DATETIME NOT NULL,
  PRIMARY KEY (account_id, op)
);
`

// migrate applies the schema and additive column migrations.
func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	// Additive columns for databases created before these existed. SQLite has no
	// ADD COLUMN IF NOT EXISTS, so ignore the duplicate-column error.
	addCol := func(ddl, tag string) error {
		if _, err := s.db.Exec(ddl); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("store: migrate %s: %w", tag, err)
		}
		return nil
	}
	for _, c := range []struct{ ddl, tag string }{
		{`ALTER TABLE accounts ADD COLUMN error_msg TEXT NOT NULL DEFAULT ''`, "error_msg"},
		{`ALTER TABLE accounts ADD COLUMN daily_count INTEGER NOT NULL DEFAULT 0`, "daily_count"},
		{`ALTER TABLE accounts ADD COLUMN daily_date TEXT NOT NULL DEFAULT ''`, "daily_date"},
	} {
		if err := addCol(c.ddl, c.tag); err != nil {
			return err
		}
	}
	return nil
}
