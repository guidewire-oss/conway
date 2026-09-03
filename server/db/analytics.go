// Usage analytics storage (spec 016): an append-only event stream.
package db

import (
	"context"
	"time"
)

// AnalyticsEvent is one business event. Meta carries per-event dimensions
// (pod counts, initiative counts, rules, durations) as JSON.
type AnalyticsEvent struct {
	ID       int64     `json:"id"`
	TS       time.Time `json:"ts"`
	Username string    `json:"username"`
	Event    string    `json:"event"`
	PlanID   string    `json:"planId"`
	Meta     []byte    `json:"meta,omitempty"`
}

// InsertAnalyticsEvent appends one event. Fire-and-forget by design: an
// analytics failure must never fail the request it describes.
func (d *DB) InsertAnalyticsEvent(ev AnalyticsEvent) error {
	_, err := d.pool.Exec(context.Background(),
		`INSERT INTO analytics_events (username, event, plan_id, meta) VALUES ($1, $2, $3, $4)`,
		ev.Username, ev.Event, ev.PlanID, jsonbArg(ev.Meta))
	return err
}

// AnalyticsEventRow is one row of the range query, aggregated lightly in SQL
// to keep the payload bounded: the day bucket, not the timestamp.
type AnalyticsEventRow struct {
	TS       time.Time `json:"ts"`
	Username string    `json:"username"`
	Event    string    `json:"event"`
	PlanID   string    `json:"planId"`
	Meta     []byte    `json:"meta,omitempty"`
}

// ListAnalyticsEvents returns the events in [from, to), newest first. The
// analytics endpoint aggregates in Go — the row count stays small because
// the page's range selectors bound it, and Go aggregation keeps one code
// path (no SQL dialect to drift from the metrics call sites).
func (d *DB) ListAnalyticsEvents(from, to time.Time) ([]AnalyticsEventRow, error) {
	rows, err := d.pool.Query(context.Background(),
		`SELECT ts, username, event, plan_id, meta FROM analytics_events
		 WHERE ts >= $1 AND ts < $2 ORDER BY ts DESC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AnalyticsEventRow{}
	for rows.Next() {
		var r AnalyticsEventRow
		if err := rows.Scan(&r.TS, &r.Username, &r.Event, &r.PlanID, &r.Meta); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
