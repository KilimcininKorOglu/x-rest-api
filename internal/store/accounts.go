package store

import (
	"database/sql"
	"time"
)

const accountCols = `id, label, auth_token, ct0, enabled, status, error_msg, cooldown_until, last_used_at, daily_count, daily_date, created_at`

// scanAccount reads one account row.
func scanAccount(sc interface{ Scan(...any) error }) (Account, error) {
	var a Account
	var enabled int
	var cooldown, lastUsed sql.NullTime
	err := sc.Scan(&a.ID, &a.Label, &a.AuthToken, &a.CT0, &enabled, &a.Status,
		&a.ErrorMsg, &cooldown, &lastUsed, &a.DailyCount, &a.DailyDate, &a.CreatedAt)
	if err != nil {
		return Account{}, err
	}
	a.Enabled = enabled != 0
	if cooldown.Valid {
		a.CooldownUntil = &cooldown.Time
	}
	if lastUsed.Valid {
		a.LastUsedAt = &lastUsed.Time
	}
	return a, nil
}

// DailyStamp is the UTC date key (YYYY-MM-DD) used for the per-account daily
// request counter.
func DailyStamp(t time.Time) string { return t.UTC().Format("2006-01-02") }

// CreateAccount inserts a new account and returns its id.
func (s *Store) CreateAccount(label, authToken, ct0 string, enabled bool) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO accounts(label, auth_token, ct0, enabled) VALUES(?, ?, ?, ?)`,
		label, authToken, ct0, boolInt(enabled))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListAccounts returns all accounts ordered by id.
func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT ` + accountCols + ` FROM accounts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return collectAccounts(rows)
}

// ListAvailableAccounts returns enabled accounts whose cooldown has passed.
func (s *Store) ListAvailableAccounts() ([]Account, error) {
	rows, err := s.db.Query(
		`SELECT `+accountCols+` FROM accounts
		 WHERE enabled = 1 AND (cooldown_until IS NULL OR cooldown_until < ?)
		 ORDER BY id`, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return collectAccounts(rows)
}

func collectAccounts(rows *sql.Rows) ([]Account, error) {
	var out []Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAccount returns one account by id.
func (s *Store) GetAccount(id int64) (Account, error) {
	row := s.db.QueryRow(`SELECT `+accountCols+` FROM accounts WHERE id = ?`, id)
	return scanAccount(row)
}

// GetAccountByLabel returns one account by label.
func (s *Store) GetAccountByLabel(label string) (Account, error) {
	row := s.db.QueryRow(`SELECT `+accountCols+` FROM accounts WHERE label = ?`, label)
	return scanAccount(row)
}

// UpdateAccount updates the editable fields of an account.
func (s *Store) UpdateAccount(id int64, label, authToken, ct0 string, enabled bool) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET label = ?, auth_token = ?, ct0 = ?, enabled = ? WHERE id = ?`,
		label, authToken, ct0, boolInt(enabled), id)
	return err
}

// DeleteAccount removes an account and its per-op locks. The locks also cascade
// via the foreign key, but the explicit delete keeps cleanup independent of the
// foreign_keys pragma.
func (s *Store) DeleteAccount(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM account_locks WHERE account_id = ?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	return err
}

// MarkAccountUsed records the last successful use, clears any error status, and
// bumps the daily request counter (resetting it when the UTC date rolled over).
func (s *Store) MarkAccountUsed(id int64) error {
	now := time.Now().UTC()
	today := DailyStamp(now)
	_, err := s.db.Exec(
		`UPDATE accounts SET last_used_at = ?, status = 'ok',
		   daily_count = CASE WHEN daily_date = ? THEN daily_count + 1 ELSE 1 END,
		   daily_date = ?
		 WHERE id = ?`,
		now, today, today, id)
	return err
}

// DisableAccount turns an account off with a reason, for a ban or auth failure.
// The operator re-enables it from the admin panel after fixing the cookies.
func (s *Store) DisableAccount(id int64, reason string) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET enabled = 0, status = 'error', error_msg = ? WHERE id = ?`,
		reason, id)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
