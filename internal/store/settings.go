package store

import (
	"database/sql"
	"errors"
	"strconv"
)

// Setting keys used across the app.
const (
	SettingProxy          = "proxy"
	SettingUserAgent      = "user_agent"
	SettingTxID           = "tx_id"
	SettingEnableWrites   = "enable_writes"
	SettingLogRetention   = "log_retention_days"
	SettingPublicFallback = "enable_public_fallback"
	// SettingDailyRequestLimit caps successful reads per account per UTC day
	// (0 = unlimited), so a single account is not overused in one day.
	SettingDailyRequestLimit = "daily_request_limit"
)

// GetSetting returns the value for key, or def when the key is absent.
func (s *Store) GetSetting(key, def string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return def, nil
	}
	if err != nil {
		return def, err
	}
	return v, nil
}

// SetSetting upserts a setting.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

// GetSettingBool returns a boolean setting.
func (s *Store) GetSettingBool(key string, def bool) bool {
	v, err := s.GetSetting(key, "")
	if err != nil || v == "" {
		return def
	}
	return v == "true" || v == "1"
}

// GetSettingInt returns an integer setting.
func (s *Store) GetSettingInt(key string, def int) int {
	v, err := s.GetSetting(key, "")
	if err != nil || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// AllSettings returns every stored setting.
func (s *Store) AllSettings() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}
