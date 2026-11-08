package control

import (
	"context"
	"errors"
)

// LegacyTraceIDs is an owner-scoped cursor for the offline migration command.
// Freeze the old writer or replay a stable snapshot: this is not CDC.
func (s *ClickHouseTraceStore) LegacyTraceIDs(ctx context.Context, owner, after string, limit int) ([]string, error) {
	if s.otel || owner == "" || limit < 1 || limit > 1000 {
		return nil, errors.New("legacy replay requires an owner and a page size from 1 to 1000")
	}
	rows, e := s.conn.Query(ctx, "SELECT DISTINCT trace_id FROM catena_spans FINAL WHERE owner_id = ? AND trace_id > ? ORDER BY trace_id LIMIT ?", owner, after, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
