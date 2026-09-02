package store

import (
	"database/sql"
	"errors"
	"time"
)

// CountAdmins returns how many admin users exist (0 means first-run setup).
func (s *Store) CountAdmins() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM admin_users`).Scan(&n)
	return n, err
}

// CreateAdmin inserts an admin user with an already-hashed password.
func (s *Store) CreateAdmin(username, passwordHash string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO admin_users(username, password_hash) VALUES(?, ?)`,
		username, passwordHash)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetAdminByUsername returns an admin user by username.
func (s *Store) GetAdminByUsername(username string) (AdminUser, error) {
	var u AdminUser
	err := s.db.QueryRow(
		`SELECT id, username, password_hash, created_at FROM admin_users WHERE username = ?`,
		username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	return u, err
}

// CreateSession stores a server-side admin session and returns its id.
func (s *Store) CreateSession(id string, adminUserID int64, expiresAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO admin_sessions(id, admin_user_id, expires_at) VALUES(?, ?, ?)`,
		id, adminUserID, expiresAt.UTC())
	return err
}

// SessionAdminID returns the admin user id for a valid, unexpired session.
func (s *Store) SessionAdminID(sessionID string) (int64, bool) {
	var adminID int64
	var expires time.Time
	err := s.db.QueryRow(
		`SELECT admin_user_id, expires_at FROM admin_sessions WHERE id = ?`,
		sessionID).Scan(&adminID, &expires)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return 0, false
	}
	if time.Now().UTC().After(expires) {
		return 0, false
	}
	return adminID, true
}

// DeleteSession removes a session (logout).
func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM admin_sessions WHERE id = ?`, id)
	return err
}
