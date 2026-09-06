package db

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

type PlanSource struct {
	ID                    string `json:"id"`
	PlanID                string `json:"planId"`
	Kind                  string `json:"kind"`
	Provider              string `json:"provider"`
	SpreadsheetID         string `json:"spreadsheetId"`
	SpreadsheetURL        string `json:"spreadsheetUrl"`
	Range                 string `json:"range"`
	Mode                  string `json:"mode"`
	Status                string `json:"status"`
	PollMinutes           int    `json:"pollMinutes"`
	LastCheckedAt         int64  `json:"lastCheckedAt"`
	NextCheckAt           int64  `json:"nextCheckAt"`
	LastError             string `json:"lastError"`
	LatestVersionID       string `json:"latestVersionId"`
	AppliedVersionID      string `json:"appliedVersionId"`
	CheckpointFingerprint string `json:"checkpointFingerprint"`
	CreatedAt             int64  `json:"createdAt"`
}

type SourceVersion struct {
	ID          string          `json:"id"`
	SourceID    string          `json:"sourceId"`
	CapturedAt  int64           `json:"capturedAt"`
	ContentHash string          `json:"contentHash"`
	Rows        [][]string      `json:"rows,omitempty"`
	Parsed      json.RawMessage `json:"parsed,omitempty"`
	Valid       bool            `json:"valid"`
	Errors      []string        `json:"errors"`
	Warnings    []string        `json:"warnings"`
	Removals    []string        `json:"removals"`
	Count       int             `json:"count"`
}

type SourceApplication struct {
	ID                   string `json:"id"`
	SourceID             string `json:"sourceId"`
	VersionID            string `json:"versionId"`
	Actor                string `json:"actor"`
	Automatic            bool   `json:"automatic"`
	Restore              bool   `json:"restore"`
	AllowRemovals        bool   `json:"allowRemovals"`
	AppliedAt            int64  `json:"appliedAt"`
	PreviousFingerprint  string `json:"previousFingerprint"`
	ResultingFingerprint string `json:"resultingFingerprint"`
}

func (d *DB) CreatePlanSource(ctx context.Context, source PlanSource) error {
	b, err := json.Marshal(source)
	if err != nil {
		return err
	}
	_, err = d.pool.Exec(ctx, `INSERT INTO plan_sheet_sources(id,plan_id,kind,status,data) VALUES($1,$2,$3,$4,$5)`, source.ID, source.PlanID, source.Kind, source.Status, b)
	return err
}

func (d *DB) GetPlanSource(ctx context.Context, planID, id string) (*PlanSource, error) {
	var b []byte
	err := d.pool.QueryRow(ctx, `SELECT data FROM plan_sheet_sources WHERE id=$1 AND plan_id=$2`, id, planID).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var v PlanSource
	err = json.Unmarshal(b, &v)
	return &v, err
}

func (d *DB) ListPlanSources(ctx context.Context, planID string) ([]PlanSource, error) {
	rows, err := d.pool.Query(ctx, `SELECT data FROM plan_sheet_sources WHERE plan_id=$1 ORDER BY id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PlanSource{}
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		var v PlanSource
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// Due sources must still belong to a current, unexpired manager/admin account.
func (d *DB) DuePlanSources(ctx context.Context, now int64) ([]PlanSource, error) {
	rows, err := d.pool.Query(ctx, `SELECT s.data FROM plan_sheet_sources s JOIN plans p ON p.id=s.plan_id JOIN accounts a ON a.username=p.owner
	 WHERE s.status='active' AND s.lease_until<$1 AND COALESCE((s.data->>'nextCheckAt')::bigint,0)<=$1
	 AND (a.expires_at=0 OR a.expires_at>$1) AND COALESCE(a.roles,ARRAY[a.role]) && ARRAY['manager','admin']::text[]
	 ORDER BY (s.data->>'nextCheckAt')::bigint LIMIT 25`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PlanSource{}
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		var v PlanSource
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (d *DB) LeasePlanSource(ctx context.Context, id, token string, now int64) (bool, error) {
	tag, err := d.pool.Exec(ctx, `UPDATE plan_sheet_sources SET lease_token=$2,lease_until=$3+120 WHERE id=$1 AND lease_until<$3`, id, token, now)
	return tag.RowsAffected() == 1, err
}

func (d *DB) ReleasePlanSource(ctx context.Context, id, token string) error {
	_, err := d.pool.Exec(ctx, `UPDATE plan_sheet_sources SET lease_token='',lease_until=0 WHERE id=$1 AND lease_token=$2`, id, token)
	return err
}

func (d *DB) SavePlanSource(ctx context.Context, source PlanSource, token string) (bool, error) {
	b, err := json.Marshal(source)
	if err != nil {
		return false, err
	}
	tag, err := d.pool.Exec(ctx, `UPDATE plan_sheet_sources SET status=$3,data=$4 WHERE id=$1 AND lease_token=$2`, source.ID, token, source.Status, b)
	return tag.RowsAffected() == 1, err
}

func (d *DB) GetSourceVersion(ctx context.Context, sourceID, id string) (*SourceVersion, error) {
	var b []byte
	err := d.pool.QueryRow(ctx, `SELECT data FROM plan_sheet_versions WHERE id=$1 AND source_id=$2`, id, sourceID).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var v SourceVersion
	err = json.Unmarshal(b, &v)
	return &v, err
}

func (d *DB) ListSourceVersions(ctx context.Context, sourceID string) ([]SourceVersion, error) {
	rows, err := d.pool.Query(ctx, `SELECT data - 'rows' - 'parsed' FROM plan_sheet_versions WHERE source_id=$1 ORDER BY captured_at DESC,id DESC`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SourceVersion{}
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		var v SourceVersion
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// Capture and source pointer advance together; the lease serializes consecutive
// hash comparisons (specs/023-linked-google-sheets.md:220).
func (d *DB) CaptureSourceVersion(ctx context.Context, source PlanSource, version SourceVersion, token string) (bool, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	sb, err := json.Marshal(source)
	if err != nil {
		return false, err
	}
	vb, err := json.Marshal(version)
	if err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `UPDATE plan_sheet_sources SET data=$3 WHERE id=$1 AND lease_token=$2`, source.ID, token, sb)
	if err != nil || tag.RowsAffected() != 1 {
		return false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO plan_sheet_versions(id,source_id,captured_at,data) VALUES($1,$2,$3,$4)`, version.ID, source.ID, version.CapturedAt, vb); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// ApplySourceVersion atomically checks the full stored plan, updates only the
// requested inputs, and records provenance; agreement tables are never touched.
// specs/023-linked-google-sheets.md:241
func (d *DB) ApplySourceVersion(ctx context.Context, p *PlanRow, source PlanSource, teams, inits []byte, application SourceApplication, token string) (bool, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	sb, err := json.Marshal(source)
	if err != nil {
		return false, err
	}
	ab, err := json.Marshal(application)
	if err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `UPDATE plan_sheet_sources SET data=$3 WHERE id=$1 AND lease_token=$2`, source.ID, token, sb)
	if err != nil || tag.RowsAffected() != 1 {
		return false, err
	}
	tag, err = tx.Exec(ctx, `UPDATE plans SET teams=$2,initiatives=$3,updated_at=$4 WHERE id=$1
	 AND teams IS NOT DISTINCT FROM $5::jsonb AND initiatives IS NOT DISTINCT FROM $6::jsonb
	 AND scheduling IS NOT DISTINCT FROM $7::jsonb AND sites IS NOT DISTINCT FROM $8::jsonb
	 AND horizon_weeks=$9 AND capacity_loss=$10`, p.ID, jsonbArg(teams), jsonbArg(inits), application.AppliedAt, jsonbArg(p.Teams), jsonbArg(p.Initiatives), jsonbArg(p.Scheduling), jsonbArg(p.Sites), p.HorizonWeeks, p.CapacityLoss)
	if err != nil || tag.RowsAffected() != 1 {
		return false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO plan_sheet_applications(id,source_id,version_id,applied_at,data) VALUES($1,$2,$3,$4,$5)`, application.ID, source.ID, application.VersionID, application.AppliedAt, ab); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
