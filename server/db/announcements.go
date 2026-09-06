package db

import (
	"context"
	"fmt"
)

// FeatureAnnouncementState keeps presentation and actual use independent.
type FeatureAnnouncementState struct {
	Announced bool `json:"announced"`
	Visited   bool `json:"visited"`
}

func (d *DB) LoadFeatureAnnouncements(ctx context.Context, subject string) (map[string]FeatureAnnouncementState, error) {
	rows, err := d.pool.Query(ctx, `SELECT feature_id, announced_at IS NOT NULL, visited_at IS NOT NULL FROM feature_announcement_state WHERE subject=$1`, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := make(map[string]FeatureAnnouncementState)
	for rows.Next() {
		var id string
		var state FeatureAnnouncementState
		if err := rows.Scan(&id, &state.Announced, &state.Visited); err != nil {
			return nil, err
		}
		states[id] = state
	}
	return states, rows.Err()
}

// specs/022-feature-announcements.md:164: retain first timestamps under retries
// and concurrent requests; neither acknowledgement changes the other field.
func (d *DB) AcknowledgeFeatureAnnouncement(ctx context.Context, subject, id, kind string) error {
	var query string
	switch kind {
	case "announced":
		query = `INSERT INTO feature_announcement_state (subject,feature_id,announced_at) VALUES ($1,$2,now()) ON CONFLICT (subject,feature_id) DO UPDATE SET announced_at=COALESCE(feature_announcement_state.announced_at,EXCLUDED.announced_at)`
	case "visited":
		query = `INSERT INTO feature_announcement_state (subject,feature_id,visited_at) VALUES ($1,$2,now()) ON CONFLICT (subject,feature_id) DO UPDATE SET visited_at=COALESCE(feature_announcement_state.visited_at,EXCLUDED.visited_at)`
	default:
		return fmt.Errorf("invalid acknowledgement kind")
	}
	_, err := d.pool.Exec(ctx, query, subject, id)
	return err
}
