package store

import "time"

// QueryIDOverride is a persisted queryId override for one GraphQL operation.
type QueryIDOverride struct {
	Operation string
	QueryID   string
	UpdatedAt time.Time
}

// UpsertQueryIDs stores queryId overrides for the given operations.
func (s *Store) UpsertQueryIDs(ids map[string]string) error {
	for op, qid := range ids {
		_, err := s.db.Exec(
			`INSERT INTO query_ids(operation, query_id, updated_at) VALUES(?, ?, ?)
			 ON CONFLICT(operation) DO UPDATE SET query_id = excluded.query_id, updated_at = excluded.updated_at`,
			op, qid, time.Now().UTC())
		if err != nil {
			return err
		}
	}
	return nil
}

// AllQueryIDs returns every stored queryId override as operation -> queryId.
func (s *Store) AllQueryIDs() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT operation, query_id FROM query_ids`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var op, qid string
		if err := rows.Scan(&op, &qid); err != nil {
			return nil, err
		}
		out[op] = qid
	}
	return out, rows.Err()
}

// ListQueryIDs returns overrides with timestamps, for the admin view.
func (s *Store) ListQueryIDs() ([]QueryIDOverride, error) {
	rows, err := s.db.Query(`SELECT operation, query_id, updated_at FROM query_ids ORDER BY operation`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []QueryIDOverride
	for rows.Next() {
		var o QueryIDOverride
		if err := rows.Scan(&o.Operation, &o.QueryID, &o.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
