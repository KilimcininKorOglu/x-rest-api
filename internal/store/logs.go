package store

import (
	"database/sql"
	"time"
)

// InsertLog records one /v1 request.
func (s *Store) InsertLog(l RequestLog) error {
	_, err := s.db.Exec(
		`INSERT INTO request_logs
		 (api_key_id, account_id, method, path, query, status, duration_ms, upstream_status, error, remote_ip)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.APIKeyID, l.AccountID, l.Method, l.Path, l.Query, l.Status,
		l.DurationMS, l.UpstreamStatus, l.Error, l.RemoteIP)
	return err
}

// ListLogs returns request logs newest-first, filtered by path substring.
func (s *Store) ListLogs(f LogFilter) ([]RequestLog, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, ts, api_key_id, account_id, method, path, query, status,
		        duration_ms, upstream_status, error, remote_ip
		 FROM request_logs
		 WHERE (? = '' OR path LIKE '%' || ? || '%')
		 ORDER BY id DESC LIMIT ? OFFSET ?`,
		f.Path, f.Path, f.Limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RequestLog
	for rows.Next() {
		l, err := scanLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func scanLog(sc interface{ Scan(...any) error }) (RequestLog, error) {
	var l RequestLog
	var apiKeyID, accountID, upstream sql.NullInt64
	err := sc.Scan(&l.ID, &l.TS, &apiKeyID, &accountID, &l.Method, &l.Path, &l.Query,
		&l.Status, &l.DurationMS, &upstream, &l.Error, &l.RemoteIP)
	if err != nil {
		return RequestLog{}, err
	}
	if apiKeyID.Valid {
		l.APIKeyID = &apiKeyID.Int64
	}
	if accountID.Valid {
		l.AccountID = &accountID.Int64
	}
	if upstream.Valid {
		n := int(upstream.Int64)
		l.UpstreamStatus = &n
	}
	return l, nil
}

// CountLogs returns the total number of request logs.
func (s *Store) CountLogs() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM request_logs`).Scan(&n)
	return n, err
}

// DeleteLogsOlderThan removes logs older than the given number of days. A value
// of zero or less disables retention and deletes nothing.
func (s *Store) DeleteLogsOlderThan(days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	res, err := s.db.Exec(`DELETE FROM request_logs WHERE ts < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
