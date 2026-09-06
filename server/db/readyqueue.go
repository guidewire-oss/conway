package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"conway/server/planning"
	"github.com/jackc/pgx/v5"
)

type ReadyQueueState struct {
	Order         int64                        `json:"-"`
	Confirmations []planning.ReadyConfirmation `json:"confirmations"`
	Decisions     []planning.ReleaseDecision   `json:"decisions"`
}
type ReadyQueueGuard struct {
	Plan  *PlanRow
	Order int64
}

// ReadyQueueHistory retains removed membership and selects latest records by
// persisted insertion order, not timestamps. specs/025-team-ready-work-queue.md:339
func (d *DB) ReadyQueueHistory(ctx context.Context, planID, team, initiative string) (ReadyQueueState, error) {
	out := ReadyQueueState{Confirmations: []planning.ReadyConfirmation{}, Decisions: []planning.ReleaseDecision{}}
	rows, err := d.pool.Query(ctx, `SELECT event_order,kind,data FROM plan_ready_queue_events WHERE plan_id=$1 AND ($2='' OR team=$2) AND ($3='' OR initiative=$3) ORDER BY event_order`, planID, team, initiative)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var order int64
		var kind string
		var raw []byte
		if err := rows.Scan(&order, &kind, &raw); err != nil {
			return out, err
		}
		out.Order = order
		if kind == "confirmation" {
			var value planning.ReadyConfirmation
			if err := json.Unmarshal(raw, &value); err != nil {
				return out, err
			}
			value.EventOrder = order
			out.Confirmations = append(out.Confirmations, value)
		} else {
			var value planning.ReleaseDecision
			if err := json.Unmarshal(raw, &value); err != nil {
				return out, err
			}
			value.EventOrder = order
			out.Decisions = append(out.Decisions, value)
		}
	}
	return out, rows.Err()
}

// AppendReadyQueueEvent is the only queue writer. Plan changes and any queue
// event invalidate a reviewed context before history is appended.
func (d *DB) AppendReadyQueueEvent(ctx context.Context, confirmation *planning.ReadyConfirmation, decision *planning.ReleaseDecision, guard ReadyQueueGuard) (bool, error) {
	if guard.Plan == nil || (confirmation == nil) == (decision == nil) {
		return false, fmt.Errorf("provide one queue event and its guarded plan")
	}
	var id, planID, team, initiative, kind string
	var at int64
	var value any
	if confirmation != nil {
		id, planID, team, initiative, kind, at, value = confirmation.ID, confirmation.PlanID, confirmation.Team, confirmation.Initiative, "confirmation", confirmation.CreatedAt, confirmation
	} else {
		id, planID, team, initiative, kind, at, value = decision.ID, decision.PlanID, decision.Team, decision.Initiative, decision.Decision, decision.CreatedAt, decision
	}
	if planID != guard.Plan.ID {
		return false, fmt.Errorf("queue event must belong to its guarded plan")
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	p := guard.Plan
	var guardedID string
	err = tx.QueryRow(ctx, `SELECT id FROM plans WHERE id=$1 AND teams IS NOT DISTINCT FROM $2::jsonb AND initiatives IS NOT DISTINCT FROM $3::jsonb AND scheduling IS NOT DISTINCT FROM $4::jsonb AND sites IS NOT DISTINCT FROM $5::jsonb AND horizon_weeks=$6 AND capacity_loss=$7 FOR UPDATE`, p.ID, jsonbArg(p.Teams), jsonbArg(p.Initiatives), jsonbArg(p.Scheduling), jsonbArg(p.Sites), p.HorizonWeeks, p.CapacityLoss).Scan(&guardedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var order int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(event_order),0) FROM plan_ready_queue_events WHERE plan_id=$1`, p.ID).Scan(&order); err != nil {
		return false, err
	}
	if order != guard.Order {
		return false, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO plan_ready_queue_events(id,plan_id,team,initiative,kind,created_at,data) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING event_order`, id, guardedID, team, initiative, kind, at, raw).Scan(&order)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	if confirmation != nil {
		confirmation.EventOrder = order
	} else {
		decision.EventOrder = order
	}
	return true, nil
}
