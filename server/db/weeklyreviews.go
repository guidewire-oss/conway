package db

import (
	"context"
	"encoding/json"
	"errors"

	"conway/server/planning"
	"github.com/jackc/pgx/v5"
)

type ActionTransition struct {
	ID         string `json:"id"`
	DecisionID string `json:"decisionId"`
	PlanID     string `json:"planId"`
	FromStatus string `json:"fromStatus"`
	Status     string `json:"status"`
	Version    int    `json:"version"`
	Evidence   string `json:"evidence"`
	CreatedBy  string `json:"createdBy"`
	CreatedAt  int64  `json:"createdAt"`
}
type ReviewPreview struct {
	planning.ReviewSummary
	Fingerprint string `json:"fingerprint"`
}
type ExecutionReview struct {
	ID             string        `json:"id"`
	PlanID         string        `json:"planId"`
	ReviewDate     string        `json:"reviewDate"`
	Timezone       string        `json:"timezone"`
	SnapshotID     string        `json:"snapshotId"`
	BaselineID     string        `json:"baselineId"`
	CreatedBy      string        `json:"createdBy"`
	CreatedAt      int64         `json:"createdAt"`
	OutcomeNote    string        `json:"outcomeNote"`
	NextCheckpoint string        `json:"nextCheckpoint"`
	Preview        ReviewPreview `json:"preview"`
}

func (d *DB) GetExecutionDecision(ctx context.Context, planID, id string) (*ExecutionDecision, error) {
	var v ExecutionDecision
	err := d.pool.QueryRow(ctx, decisionSelect+` WHERE d.plan_id=$1 AND d.id=$2`, planID, id).Scan(&v.ID, &v.PlanID, &v.Action, &v.Owner, &v.ReviewDate, &v.Rationale, &v.Initiative, &v.SnapshotID, &v.BaselineID, &v.CreatedBy, &v.CreatedAt, &v.Status, &v.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &v, err
}

func (d *DB) ActionTransitions(ctx context.Context, planID, decisionID string) ([]ActionTransition, error) {
	rows, err := d.pool.Query(ctx, `SELECT id,decision_id,plan_id,from_status,status,version,evidence,created_by,created_at FROM plan_execution_action_transitions WHERE plan_id=$1 AND decision_id=$2 ORDER BY version`, planID, decisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ActionTransition{}
	for rows.Next() {
		var v ActionTransition
		if err := rows.Scan(&v.ID, &v.DecisionID, &v.PlanID, &v.FromStatus, &v.Status, &v.Version, &v.Evidence, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// TransitionAction serializes with review completion on the plan row. A stale
// version cannot update its projection or append an event.
func (d *DB) TransitionAction(ctx context.Context, event ActionTransition, expectedVersion int) (bool, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var planID string
	if err = tx.QueryRow(ctx, `SELECT id FROM plans WHERE id=$1 FOR UPDATE`, event.PlanID).Scan(&planID); err != nil {
		return false, err
	}
	var from string
	err = tx.QueryRow(ctx, `UPDATE plan_execution_action_state SET status=$4,version=version+1 WHERE plan_id=$1 AND decision_id=$2 AND version=$3 AND status=$5 RETURNING $5::text`, event.PlanID, event.DecisionID, expectedVersion, event.Status, event.FromStatus).Scan(&from)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO plan_execution_action_transitions(id,decision_id,plan_id,from_status,status,version,evidence,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, event.ID, event.DecisionID, event.PlanID, from, event.Status, expectedVersion+1, event.Evidence, event.CreatedBy, event.CreatedAt)
	if err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (d *DB) ExecutionReviews(ctx context.Context, planID string) ([]ExecutionReview, error) {
	rows, err := d.pool.Query(ctx, `SELECT data FROM plan_execution_reviews WHERE plan_id=$1 ORDER BY completion_order DESC`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExecutionReview{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var v ExecutionReview
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (d *DB) GetExecutionReview(ctx context.Context, planID, id string) (*ExecutionReview, error) {
	var raw []byte
	err := d.pool.QueryRow(ctx, `SELECT data FROM plan_execution_reviews WHERE plan_id=$1 AND id=$2`, planID, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var v ExecutionReview
	err = json.Unmarshal(raw, &v)
	return &v, err
}
func (d *DB) LatestExecutionReview(ctx context.Context, planID string) (*ExecutionReview, error) {
	var raw []byte
	err := d.pool.QueryRow(ctx, `SELECT data FROM plan_execution_reviews WHERE plan_id=$1 ORDER BY completion_order DESC LIMIT 1`, planID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var v ExecutionReview
	err = json.Unmarshal(raw, &v)
	return &v, err
}

// ReviewGuard holds database state read alongside the preview. The transaction
// repeats these checks while holding the plan lock shared by action mutations.
type ReviewGuard struct {
	Plan       *PlanRow
	Baseline   *BaselineRow
	Snapshot   *SnapshotRow
	Issues     []IssueRow
	Actions    []ExecutionDecision
	PreviousID string
}

func (d *DB) CompleteExecutionReview(ctx context.Context, review ExecutionReview, guard ReviewGuard) (bool, error) {
	if guard.Plan == nil || review.PlanID != guard.Plan.ID {
		return false, errors.New("review must belong to its guarded plan")
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	p := guard.Plan
	var id string
	err = tx.QueryRow(ctx, `SELECT id FROM plans WHERE id=$1 AND teams IS NOT DISTINCT FROM $2::jsonb AND initiatives IS NOT DISTINCT FROM $3::jsonb AND scheduling IS NOT DISTINCT FROM $4::jsonb AND sites IS NOT DISTINCT FROM $5::jsonb AND horizon_weeks=$6 AND capacity_loss=$7 FOR UPDATE`, p.ID, jsonbArg(p.Teams), jsonbArg(p.Initiatives), jsonbArg(p.Scheduling), jsonbArg(p.Sites), p.HorizonWeeks, p.CapacityLoss).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var baselineID, baselineFingerprint, baselineName string
	err = tx.QueryRow(ctx, `SELECT id,fingerprint,name FROM plan_baselines WHERE plan_id=$1 AND active FOR SHARE`, p.ID).Scan(&baselineID, &baselineFingerprint, &baselineName)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if guard.Baseline == nil {
		if baselineID != "" {
			return false, nil
		}
	} else if baselineID != guard.Baseline.ID || baselineFingerprint != guard.Baseline.Fingerprint || baselineName != guard.Baseline.Name {
		return false, nil
	}
	var previousID string
	err = tx.QueryRow(ctx, `SELECT id FROM plan_execution_reviews WHERE plan_id=$1 ORDER BY completion_order DESC LIMIT 1`, p.ID).Scan(&previousID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if previousID != guard.PreviousID {
		return false, nil
	}
	rows, err := tx.Query(ctx, decisionSelect+` WHERE d.plan_id=$1 ORDER BY d.created_at DESC,d.id`, p.ID)
	if err != nil {
		return false, err
	}
	actions := []ExecutionDecision{}
	for rows.Next() {
		var v ExecutionDecision
		if err := rows.Scan(&v.ID, &v.PlanID, &v.Action, &v.Owner, &v.ReviewDate, &v.Rationale, &v.Initiative, &v.SnapshotID, &v.BaselineID, &v.CreatedBy, &v.CreatedAt, &v.Status, &v.Version); err != nil {
			rows.Close()
			return false, err
		}
		actions = append(actions, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	current, _ := json.Marshal(actions)
	expected, _ := json.Marshal(guard.Actions)
	if string(current) != string(expected) {
		return false, nil
	}
	if guard.Snapshot != nil {
		var snap SnapshotRow
		err = tx.QueryRow(ctx, `SELECT id,owner,name,source,public,created_at FROM snapshots WHERE id=$1 FOR SHARE`, guard.Snapshot.ID).Scan(&snap.ID, &snap.Owner, &snap.Name, &snap.Source, &snap.Public, &snap.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if snap.Owner != guard.Snapshot.Owner || snap.Public != guard.Snapshot.Public || snap.CreatedAt != guard.Snapshot.CreatedAt || snap.Source != guard.Snapshot.Source || snap.Name != guard.Snapshot.Name {
			return false, nil
		}
		issueRows, err := tx.Query(ctx, `SELECT key,pod,issue_type,status,status_cat,summary,parent_key,created,updated,resolved FROM snapshot_issues WHERE snapshot_id=$1 ORDER BY key FOR SHARE`, snap.ID)
		if err != nil {
			return false, err
		}
		issues := []IssueRow{}
		for issueRows.Next() {
			var i IssueRow
			if err := issueRows.Scan(&i.Key, &i.Pod, &i.IssueType, &i.Status, &i.StatusCat, &i.Summary, &i.ParentKey, &i.Created, &i.Updated, &i.Resolved); err != nil {
				issueRows.Close()
				return false, err
			}
			issues = append(issues, i)
		}
		err = issueRows.Err()
		issueRows.Close()
		if err != nil {
			return false, err
		}
		a, _ := json.Marshal(issues)
		b, _ := json.Marshal(guard.Issues)
		if string(a) != string(b) {
			return false, nil
		}
	}
	raw, err := json.Marshal(review)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO plan_execution_reviews(id,plan_id,created_at,data) VALUES($1,$2,$3,$4)`, review.ID, p.ID, review.CreatedAt, raw)
	if err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
