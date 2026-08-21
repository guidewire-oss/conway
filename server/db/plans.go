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
	Scheduling      []byte  `json:"-"`               // JSON planning.SchedulingParams
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
		`SELECT id,owner,name,horizon_weeks,capacity_loss,teams,initiatives,scheduling,roster_id,created_at,updated_at
		 FROM plans WHERE id=$1`, id).
		Scan(&p.ID, &p.Owner, &p.Name, &p.HorizonWeeks, &p.CapacityLoss, &p.Teams, &p.Initiatives, &p.Scheduling, &p.RosterID, &p.CreatedAt, &p.UpdatedAt)
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

// SavePlanScheduling stores the plan-level scheduling policy (§8's PATCH
// /api/plan/{id}/scheduling). Written whole, like teams and initiatives.
func (d *DB) SavePlanScheduling(id string, params []byte, updated int64) error {
	_, err := d.pool.Exec(context.Background(),
		`UPDATE plans SET scheduling=$2, updated_at=$3 WHERE id=$1`, id, jsonbArg(params), updated)
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

// BaselineRow is one saved baseline. The blobs are omitted from list queries,
// which is why they are []byte here rather than typed: this layer stores them, it
// does not read inside them.
type BaselineRow struct {
	ID          string `json:"id"`
	PlanID      string `json:"planId"`
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	CreatedAt   int64  `json:"createdAt"`
	CreatedBy   string `json:"createdBy"`
	Fingerprint string `json:"fingerprint"`
	ComparedTo  string `json:"comparedTo,omitempty"`
	Inputs      []byte `json:"-"`
	Schedule    []byte `json:"-"`
}

// SaveBaseline inserts a baseline and settles which one is active, in one
// transaction (FR-031).
//
// The first baseline a plan saves becomes active whatever the caller asked for: a
// plan with baselines and no active one has no answer for "what are actuals
// measured against" (§11 Decision 27).
//
// It also resolves ComparedTo inside the transaction rather than trusting a value
// the handler read beforehand: two saves racing would otherwise both record the
// same superseded baseline, and one of them would be wrong.
//
// Returns whether the row ended up active, because that is not always what the
// caller asked for.
func (d *DB) SaveBaseline(b BaselineRow, makeActive bool) (bool, error) {
	ctx := context.Background()
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	// Discarded on purpose: after a successful Commit this returns ErrTxClosed, so
	// checking it would mean reporting a non-failure on the happy path.
	defer func() { _ = tx.Rollback(ctx) }()

	var existing int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM plan_baselines WHERE plan_id=$1`, b.PlanID).Scan(&existing); err != nil {
		return false, err
	}
	active := makeActive || existing == 0

	// The baseline this one supersedes, read under the same transaction and locked,
	// so a concurrent save cannot slip a different active one in between.
	superseded := ""
	err = tx.QueryRow(ctx,
		`SELECT id FROM plan_baselines WHERE plan_id=$1 AND active FOR UPDATE`, b.PlanID).Scan(&superseded)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}

	if active {
		// Clear first: the partial unique index would otherwise reject the insert.
		if _, err := tx.Exec(ctx,
			`UPDATE plan_baselines SET active=false WHERE plan_id=$1 AND active`, b.PlanID); err != nil {
			return false, err
		}
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO plan_baselines(id,plan_id,name,active,created_at,created_by,fingerprint,compared_to,inputs,schedule)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		b.ID, b.PlanID, b.Name, active, b.CreatedAt, b.CreatedBy, b.Fingerprint, superseded,
		jsonbArg(b.Inputs), jsonbArg(b.Schedule)); err != nil {
		return false, err
	}
	return active, tx.Commit(ctx)
}

// ListBaselines returns baseline metadata newest-first, without the blobs — a
// list view needs the name, who saved it and whether it is active, not 140KB of
// frozen schedule per row.
func (d *DB) ListBaselines(planID string) ([]BaselineRow, error) {
	rows, err := d.pool.Query(context.Background(),
		`SELECT id,plan_id,name,active,created_at,created_by,fingerprint,compared_to
		 FROM plan_baselines WHERE plan_id=$1 ORDER BY created_at DESC, id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BaselineRow{}
	for rows.Next() {
		var b BaselineRow
		if err := rows.Scan(&b.ID, &b.PlanID, &b.Name, &b.Active, &b.CreatedAt,
			&b.CreatedBy, &b.Fingerprint, &b.ComparedTo); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetBaseline returns one baseline with its blobs, or nil when it does not belong
// to this plan — scoped by plan_id so a baseline id from another plan cannot be
// read by guessing it.
func (d *DB) GetBaseline(planID, id string) (*BaselineRow, error) {
	var b BaselineRow
	err := d.pool.QueryRow(context.Background(),
		`SELECT id,plan_id,name,active,created_at,created_by,fingerprint,compared_to,inputs,schedule
		 FROM plan_baselines WHERE plan_id=$1 AND id=$2`, planID, id).
		Scan(&b.ID, &b.PlanID, &b.Name, &b.Active, &b.CreatedAt, &b.CreatedBy,
			&b.Fingerprint, &b.ComparedTo, &b.Inputs, &b.Schedule)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// ActiveBaseline returns the plan's active baseline, or nil when it has none.
func (d *DB) ActiveBaseline(planID string) (*BaselineRow, error) {
	var id string
	err := d.pool.QueryRow(context.Background(),
		`SELECT id FROM plan_baselines WHERE plan_id=$1 AND active`, planID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return d.GetBaseline(planID, id)
}

// SetBaselineActive moves the active flag, in one transaction so the partial
// unique index never sees two. Returns false when the baseline is not this plan's.
func (d *DB) SetBaselineActive(planID, id string) (bool, error) {
	ctx := context.Background()
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // ErrTxClosed after Commit; not a failure
	if _, err := tx.Exec(ctx,
		`UPDATE plan_baselines SET active=false WHERE plan_id=$1 AND active`, planID); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE plan_baselines SET active=true WHERE plan_id=$1 AND id=$2`, planID, id)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil // rolled back by the deferred Rollback
	}
	return true, tx.Commit(ctx)
}

// RenameBaseline changes only the name. Everything else about a saved baseline is
// immutable (FR-030), which is why there is no general update.
func (d *DB) RenameBaseline(planID, id, name string) (bool, error) {
	tag, err := d.pool.Exec(context.Background(),
		`UPDATE plan_baselines SET name=$3 WHERE plan_id=$1 AND id=$2`, planID, id, name)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
