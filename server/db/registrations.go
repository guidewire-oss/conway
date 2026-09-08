package db

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
)

var ErrRegistrationLimit = errors.New("registered model limit exceeded")

type RegistrationRow struct {
	ID          string
	ReferenceID string
	RequestHash string
	Data        json.RawMessage
}

func (d *DB) ForecastRegistration(ctx context.Context, planID, id string) (*RegistrationRow, error) {
	var row RegistrationRow
	err := d.pool.QueryRow(ctx, `SELECT id,reference_id,request_hash,data FROM plan_forecast_registrations WHERE plan_id=$1 AND id=$2`, planID, id).Scan(&row.ID, &row.ReferenceID, &row.RequestHash, &row.Data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &row, err
}

func (d *DB) ForecastRegistrations(ctx context.Context, planID, reference string) ([]RegistrationRow, error) {
	rows, err := d.pool.Query(ctx, `SELECT id,reference_id,request_hash,data FROM plan_forecast_registrations WHERE plan_id=$1 AND reference_id=$2 ORDER BY (data->>'registeredAt')::bigint DESC,id LIMIT 101`, planID, reference)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RegistrationRow{}
	for rows.Next() {
		var row RegistrationRow
		if err := rows.Scan(&row.ID, &row.ReferenceID, &row.RequestHash, &row.Data); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// specs/031-prospective-forecast-registration.md:112: insertion is immutable and
// the database stamps the registration at insertion rather than request start.
func (d *DB) SaveForecastRegistration(ctx context.Context, p *PlanRow, row RegistrationRow) (bool, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	err = tx.QueryRow(ctx, `SELECT id FROM plans WHERE id=$1 AND owner=$2 FOR UPDATE`, p.ID, p.Owner).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plan_forecast_registrations WHERE plan_id=$1 AND id=$2)`, p.ID, row.ID).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM plan_forecast_registrations WHERE plan_id=$1 AND reference_id=$2`, p.ID, row.ReferenceID).Scan(&count); err != nil {
		return false, err
	}
	if count >= 100 {
		return false, ErrRegistrationLimit
	}
	tag, err := tx.Exec(ctx, `INSERT INTO plan_forecast_registrations(plan_id,reference_id,id,request_hash,data)
 SELECT id,$2,$3,$4,jsonb_set($5::jsonb,'{registeredAt}',to_jsonb(floor(extract(epoch FROM clock_timestamp()))::bigint)) FROM plans WHERE id=$1 AND owner=$6
 ON CONFLICT(plan_id,id) DO NOTHING`, p.ID, row.ReferenceID, row.ID, row.RequestHash, row.Data, p.Owner)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, tx.Commit(ctx)
}
