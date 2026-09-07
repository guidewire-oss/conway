package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
)

type PredictionRow struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	IssuedAt    int64           `json:"issuedAt"`
	Order       int64           `json:"order"`
	SnapshotID  string          `json:"snapshotId"`
	RequestHash string          `json:"-"`
	Data        json.RawMessage `json:"-"`
}
type PredictionSource struct {
	SourceID    string
	Fingerprint string
	StartedAt   int64
	FinishedAt  int64
}

// PredictionSnapshotSource uses the successful run's frozen configuration, not
// today's source settings. specs/028-portfolio-forecasts.md:205
func (d *DB) PredictionSnapshotSource(ctx context.Context, id string) (*PredictionSource, error) {
	var v PredictionSource
	var config []byte
	err := d.pool.QueryRow(ctx, `SELECT source_id,started_at,finished_at,
 (config - ARRAY['name','intervalHours','freshnessHours','enabled'])::text
 FROM evidence_runs WHERE snapshot_id=$1 AND status='succeeded' ORDER BY run_order DESC LIMIT 1`, id).Scan(&v.SourceID, &v.StartedAt, &v.FinishedAt, &config)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(config)
	v.Fingerprint = hex.EncodeToString(sum[:])
	return &v, nil
}
func (d *DB) Prediction(ctx context.Context, planID, id string) (*PredictionRow, error) {
	var v PredictionRow
	err := d.pool.QueryRow(ctx, `SELECT id,name,issued_at,recorded_order,snapshot_id,request_hash,data FROM plan_forecast_predictions WHERE plan_id=$1 AND id=$2`, planID, id).Scan(&v.ID, &v.Name, &v.IssuedAt, &v.Order, &v.SnapshotID, &v.RequestHash, &v.Data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &v, err
}
func (d *DB) Predictions(ctx context.Context, planID string, before int64) ([]PredictionRow, error) {
	rows, err := d.pool.Query(ctx, `SELECT id,name,issued_at,recorded_order,snapshot_id FROM plan_forecast_predictions WHERE plan_id=$1 AND ($2::bigint=0 OR recorded_order<$2) ORDER BY recorded_order DESC LIMIT 51`, planID, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PredictionRow{}
	for rows.Next() {
		var v PredictionRow
		if err := rows.Scan(&v.ID, &v.Name, &v.IssuedAt, &v.Order, &v.SnapshotID); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SavePrediction checks every stored scheduling input, including same-second
// edits, atomically with insertion. Existing keys never overwrite history.
func (d *DB) SavePrediction(ctx context.Context, p *PlanRow, v PredictionRow) (bool, error) {
	tag, err := d.pool.Exec(ctx, `INSERT INTO plan_forecast_predictions(id,plan_id,issued_at,name,snapshot_id,request_hash,data)
 SELECT $2,id,$3,$4,$5,$6,$7::jsonb FROM plans
 WHERE id=$1 AND owner=$8 AND teams IS NOT DISTINCT FROM $9::jsonb
 AND initiatives IS NOT DISTINCT FROM $10::jsonb AND scheduling IS NOT DISTINCT FROM $11::jsonb
 AND sites IS NOT DISTINCT FROM $12::jsonb AND horizon_weeks=$13 AND capacity_loss=$14
 ON CONFLICT(plan_id,id) DO NOTHING`, p.ID, v.ID, v.IssuedAt, v.Name, v.SnapshotID, v.RequestHash, v.Data, p.Owner, jsonbArg(p.Teams), jsonbArg(p.Initiatives), jsonbArg(p.Scheduling), jsonbArg(p.Sites), p.HorizonWeeks, p.CapacityLoss)
	return tag.RowsAffected() == 1, err
}
