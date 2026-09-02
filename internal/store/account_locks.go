package store

import "time"

// Per-op account locks. x.com rate-limits each GraphQL operation separately, so
// an account rate-limited on one op stays usable for others. A lock records that
// one account is cooling down for one op until unlock_until.

// AccountLock is one active per-op lock on an account.
type AccountLock struct {
	Op          string
	UnlockUntil time.Time
}

// ListAvailableAccountsForOp returns enabled accounts that hold no active lock
// for op, so rotation only skips an account for the op it is cooling down on.
func (s *Store) ListAvailableAccountsForOp(op string) ([]Account, error) {
	rows, err := s.db.Query(
		`SELECT `+accountCols+` FROM accounts a
		 WHERE a.enabled = 1
		   AND NOT EXISTS (
		     SELECT 1 FROM account_locks l
		     WHERE l.account_id = a.id AND l.op = ? AND l.unlock_until > ?
		   )
		 ORDER BY a.id`, op, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectAccounts(rows)
}

// LockAccountOp cools an account down for one op until unlock_until (upsert).
func (s *Store) LockAccountOp(id int64, op string, until time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO account_locks(account_id, op, unlock_until) VALUES(?, ?, ?)
		 ON CONFLICT(account_id, op) DO UPDATE SET unlock_until = excluded.unlock_until`,
		id, op, until.UTC())
	return err
}

// ActiveLocksByAccount returns the currently active locks grouped by account id,
// for the admin accounts view.
func (s *Store) ActiveLocksByAccount() (map[int64][]AccountLock, error) {
	rows, err := s.db.Query(
		`SELECT account_id, op, unlock_until FROM account_locks
		 WHERE unlock_until > ? ORDER BY account_id, op`, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]AccountLock{}
	for rows.Next() {
		var id int64
		var l AccountLock
		if err := rows.Scan(&id, &l.Op, &l.UnlockUntil); err != nil {
			return nil, err
		}
		out[id] = append(out[id], l)
	}
	return out, rows.Err()
}

// PurgeExpiredAccountLocks deletes locks whose unlock time has passed, to keep
// the table small. It is safe to call periodically.
func (s *Store) PurgeExpiredAccountLocks() error {
	_, err := s.db.Exec(`DELETE FROM account_locks WHERE unlock_until <= ?`, time.Now().UTC())
	return err
}
