package db

import "context"

// ClonePlan copies a single consistent database row; agreements and decisions
// belong to the original plan (specs/017-planning-and-execution-usability.md:82).
func (d *DB) ClonePlan(source, id, owner, name string, now int64) (bool, error) {
	tag, err := d.pool.Exec(context.Background(), `INSERT INTO plans
		(id,owner,name,horizon_weeks,capacity_loss,teams,initiatives,scheduling,sites,roster_id,created_at,updated_at)
		SELECT $2,$3,$4,horizon_weeks,capacity_loss,teams,initiatives,scheduling,sites,roster_id,$5,$5
		FROM plans WHERE id=$1`, source, id, owner, name, now)
	return tag.RowsAffected() == 1, err
}

// SavePlanProposalIfUnchanged prevents applying a preview over newer edits.
// JSONB equality checks the complete stored inputs, including same-second edits.
func (d *DB) SavePlanProposalIfUnchanged(p *PlanRow, teams, inits []byte, now int64) (bool, error) {
	tag, err := d.pool.Exec(context.Background(), `UPDATE plans SET teams=$2,initiatives=$3,updated_at=$4
		WHERE id=$1 AND teams IS NOT DISTINCT FROM $5::jsonb
		AND initiatives IS NOT DISTINCT FROM $6::jsonb
		AND scheduling IS NOT DISTINCT FROM $7::jsonb
		AND sites IS NOT DISTINCT FROM $8::jsonb
		AND horizon_weeks=$9 AND capacity_loss=$10`, p.ID, jsonbArg(teams), jsonbArg(inits), now,
		jsonbArg(p.Teams), jsonbArg(p.Initiatives), jsonbArg(p.Scheduling), jsonbArg(p.Sites), p.HorizonWeeks, p.CapacityLoss)
	return tag.RowsAffected() == 1, err
}
