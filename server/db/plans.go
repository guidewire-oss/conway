package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// PlanRow is a planning workspace. Teams and Initiatives are stored as JSON
// documents (the planning package's structs) so this layer stays decoupled from
// the planning types.
type PlanRow struct {
	ID              string  `json:"id"`
	Owner           string  `json:"owner"`
	Name            string  `json:"name"`
	HorizonWeeks    float64 `json:"horizonWeeks"`
	CapacityLoss    float64 `json:"capacityLoss"`
	Teams           []byte  `json:"-"`               // JSON []planning.Team
	Initiatives     []byte  `json:"-"`               // JSON []planning.Initiative
	RosterID        string  `json:"rosterId"`        // reference/label only — teams is a frozen copy, not a live join
	TeamCount       int     `json:"teamCount"`       // populated by ListPlans
	InitiativeCount int     `json:"initiativeCount"` // populated by ListPlans
	CreatedAt       int64   `json:"createdAt"`
	UpdatedAt       int64   `json:"updatedAt"`
}

func (d *DB) CreatePlan(p PlanRow) error {
	_, err := d.pool.Exec(context.Background(),
		`INSERT INTO plans(id,owner,name,horizon_weeks,capacity_loss,created_at,updated_at)
		 VALUES($1,$2,$3,$4,$5,$6,$6)`,
		p.ID, p.Owner, p.Name, p.HorizonWeeks, p.CapacityLoss, p.CreatedAt)
	return err
}

// ListPlans returns plan metadata (no teams/initiatives blobs) — all plans for
// admins, otherwise only the owner's.
func (d *DB) ListPlans(owner string, all bool) ([]PlanRow, error) {
	q := `SELECT id,owner,name,horizon_weeks,capacity_loss,
	             COALESCE(jsonb_array_length(teams),0),
	             COALESCE(jsonb_array_length(initiatives),0),
	             roster_id,created_at,updated_at FROM plans`
	args := []any{}
	if !all {
		q += ` WHERE owner=$1`
		args = append(args, owner)
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := d.pool.Query(context.Background(), q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlanRow{}
	for rows.Next() {
		var p PlanRow
		if err := rows.Scan(&p.ID, &p.Owner, &p.Name, &p.HorizonWeeks, &p.CapacityLoss,
			&p.TeamCount, &p.InitiativeCount, &p.RosterID, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) GetPlan(id string) (*PlanRow, error) {
	var p PlanRow
	err := d.pool.QueryRow(context.Background(),
		`SELECT id,owner,name,horizon_weeks,capacity_loss,teams,initiatives,roster_id,created_at,updated_at
		 FROM plans WHERE id=$1`, id).
		Scan(&p.ID, &p.Owner, &p.Name, &p.HorizonWeeks, &p.CapacityLoss, &p.Teams, &p.Initiatives, &p.RosterID, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *DB) SavePlanTeams(id string, teams []byte, updated int64) error {
	_, err := d.pool.Exec(context.Background(),
		`UPDATE plans SET teams=$2, updated_at=$3 WHERE id=$1`, id, jsonbArg(teams), updated)
	return err
}

// SetPlanRosterTeams saves a frozen copy of a roster's pods as the plan's team
// structure, and records which roster it came from (label only — not a live
// join, so later edits to the roster don't silently change the plan).
func (d *DB) SetPlanRosterTeams(id, rosterID string, teams []byte, updated int64) error {
	_, err := d.pool.Exec(context.Background(),
		`UPDATE plans SET teams=$2, roster_id=$3, updated_at=$4 WHERE id=$1`, id, jsonbArg(teams), rosterID, updated)
	return err
}

func (d *DB) SavePlanInitiatives(id string, inits []byte, updated int64) error {
	_, err := d.pool.Exec(context.Background(),
		`UPDATE plans SET initiatives=$2, updated_at=$3 WHERE id=$1`, id, jsonbArg(inits), updated)
	return err
}

func (d *DB) UpdatePlanMeta(id, name string, horizon, loss float64, updated int64) error {
	_, err := d.pool.Exec(context.Background(),
		`UPDATE plans SET name=$2, horizon_weeks=$3, capacity_loss=$4, updated_at=$5 WHERE id=$1`,
		id, name, horizon, loss, updated)
	return err
}

func (d *DB) DeletePlan(id string) error {
	_, err := d.pool.Exec(context.Background(), `DELETE FROM plans WHERE id=$1`, id)
	return err
}

// jsonbArg passes JSON to a jsonb column as text (pgx would otherwise send raw
// bytes as bytea). nil stays NULL.
func jsonbArg(b []byte) any {
	if b == nil {
		return nil
	}
	return string(b)
}
