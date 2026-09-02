package store

import (
	"database/sql"
	"time"
)

const apiKeyCols = `id, name, key, enabled, can_write, bound_account_id, last_used_at, created_at`

func scanAPIKey(sc interface{ Scan(...any) error }) (APIKey, error) {
	var k APIKey
	var enabled, canWrite int
	var bound sql.NullInt64
	var lastUsed sql.NullTime
	err := sc.Scan(&k.ID, &k.Name, &k.Key, &enabled, &canWrite, &bound, &lastUsed, &k.CreatedAt)
	if err != nil {
		return APIKey{}, err
	}
	k.Enabled = enabled != 0
	k.CanWrite = canWrite != 0
	if bound.Valid {
		k.BoundAccountID = &bound.Int64
	}
	if lastUsed.Valid {
		k.LastUsedAt = &lastUsed.Time
	}
	return k, nil
}

// CreateAPIKey inserts a new key.
func (s *Store) CreateAPIKey(name, key string, canWrite bool, boundAccountID *int64) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO api_keys(name, key, can_write, bound_account_id) VALUES(?, ?, ?, ?)`,
		name, key, boolInt(canWrite), boundAccountID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListAPIKeys returns all keys ordered by id.
func (s *Store) ListAPIKeys() ([]APIKey, error) {
	rows, err := s.db.Query(`SELECT ` + apiKeyCols + ` FROM api_keys ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// GetAPIKeyByKey looks up an enabled key by its plaintext value.
func (s *Store) GetAPIKeyByKey(key string) (APIKey, error) {
	row := s.db.QueryRow(`SELECT `+apiKeyCols+` FROM api_keys WHERE key = ?`, key)
	return scanAPIKey(row)
}

// UpdateAPIKey updates the editable fields of a key.
func (s *Store) UpdateAPIKey(id int64, name string, enabled, canWrite bool, boundAccountID *int64) error {
	_, err := s.db.Exec(
		`UPDATE api_keys SET name = ?, enabled = ?, can_write = ?, bound_account_id = ? WHERE id = ?`,
		name, boolInt(enabled), boolInt(canWrite), boundAccountID, id)
	return err
}

// DeleteAPIKey removes a key.
func (s *Store) DeleteAPIKey(id int64) error {
	_, err := s.db.Exec(`DELETE FROM api_keys WHERE id = ?`, id)
	return err
}

// MarkAPIKeyUsed records the last use timestamp.
func (s *Store) MarkAPIKeyUsed(id int64) error {
	_, err := s.db.Exec(`UPDATE api_keys SET last_used_at = ? WHERE id = ?`, time.Now().UTC(), id)
	return err
}
