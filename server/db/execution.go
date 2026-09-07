package db

import "context"

func (d *DB) HygieneIssueCounts(snapshotID, pod string) (map[string]int, error) {
	out := map[string]int{"unsized": 0, "stale": 0, "unassigned": 0, "nooutcome": 0}
	queries := map[string]string{
		"unsized":    `SELECT count(*) FROM snapshot_issues WHERE snapshot_id=$1 AND ($2='' OR pod=$2) AND issue_type NOT IN ('Epic','Parent Epic') AND status_cat<>'done' AND points IS NULL`,
		"stale":      `SELECT count(*) FROM snapshot_issues i JOIN snapshots sn ON sn.id=i.snapshot_id WHERE i.snapshot_id=$1 AND ($2='' OR i.pod=$2) AND i.status_cat='indeterminate' AND ` + wipModeCond + ` AND i.updated IS NOT NULL AND i.updated < now()-interval '14 days'`,
		"unassigned": `SELECT count(*) FROM snapshot_issues i JOIN snapshots sn ON sn.id=i.snapshot_id WHERE i.snapshot_id=$1 AND ($2='' OR i.pod=$2) AND i.status_cat='indeterminate' AND i.assignee='' AND ` + wipModeCond,
		"nooutcome":  `SELECT count(*) FROM snapshot_issues e WHERE snapshot_id=$1 AND issue_type IN ('Epic','Parent Epic') AND desc_len<40 AND ($2='' OR pod=$2 OR EXISTS(SELECT 1 FROM snapshot_issues c WHERE c.snapshot_id=e.snapshot_id AND c.parent_key=e.key AND c.pod=$2))`,
	}
	for key, query := range queries {
		var count int
		if err := d.pool.QueryRow(context.Background(), query, snapshotID, pod).Scan(&count); err != nil {
			return nil, err
		}
		out[key] = count
	}
	return out, nil
}

// ExecutionIssues reads complete evidence, never the paginated UI issue list.
func (d *DB) ExecutionIssues(snapshotID string) ([]IssueRow, error) {
	rows, err := d.pool.Query(context.Background(), `SELECT key,pod,issue_type,status,status_cat,summary,parent_key,created,updated,resolved FROM snapshot_issues WHERE snapshot_id=$1 ORDER BY key`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IssueRow{}
	for rows.Next() {
		var i IssueRow
		if err := rows.Scan(&i.Key, &i.Pod, &i.IssueType, &i.Status, &i.StatusCat, &i.Summary, &i.ParentKey, &i.Created, &i.Updated, &i.Resolved); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

type ExecutionDecision struct {
	ID         string `json:"id"`
	PlanID     string `json:"planId"`
	Action     string `json:"action"`
	Owner      string `json:"owner"`
	ReviewDate string `json:"reviewDate"`
	Rationale  string `json:"rationale"`
	Initiative string `json:"initiative"`
	SnapshotID string `json:"snapshotId"`
	BaselineID string `json:"baselineId"`
	CreatedBy  string `json:"createdBy"`
	CreatedAt  int64  `json:"createdAt"`
	Status     string `json:"status"`
	Version    int    `json:"version"`
}

func (d *DB) AppendExecutionDecision(v ExecutionDecision) error {
	ctx := context.Background()
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	if err = tx.QueryRow(ctx, `SELECT id FROM plans WHERE id=$1 FOR UPDATE`, v.PlanID).Scan(&id); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO plan_execution_decisions (id,plan_id,action,owner,review_date,rationale,initiative,snapshot_id,baseline_id,created_by,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, v.ID, v.PlanID, v.Action, v.Owner, v.ReviewDate, v.Rationale, v.Initiative, v.SnapshotID, v.BaselineID, v.CreatedBy, v.CreatedAt)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO plan_execution_action_state(decision_id,plan_id) VALUES($1,$2)`, v.ID, v.PlanID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const decisionSelect = `SELECT d.id,d.plan_id,d.action,d.owner,d.review_date,d.rationale,d.initiative,d.snapshot_id,d.baseline_id,d.created_by,d.created_at,COALESCE(s.status,'open'),COALESCE(s.version,1) FROM plan_execution_decisions d LEFT JOIN plan_execution_action_state s ON s.decision_id=d.id AND s.plan_id=d.plan_id`

func (d *DB) ExecutionDecisions(planID string) ([]ExecutionDecision, error) {
	rows, err := d.pool.Query(context.Background(), decisionSelect+` WHERE d.plan_id=$1 ORDER BY d.created_at DESC,d.id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExecutionDecision{}
	for rows.Next() {
		var v ExecutionDecision
		if err := rows.Scan(&v.ID, &v.PlanID, &v.Action, &v.Owner, &v.ReviewDate, &v.Rationale, &v.Initiative, &v.SnapshotID, &v.BaselineID, &v.CreatedBy, &v.CreatedAt, &v.Status, &v.Version); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
