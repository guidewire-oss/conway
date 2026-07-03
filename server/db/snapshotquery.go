package db

import (
	"context"
	"time"
)

// Query methods that compute the heavy views from snapshot_issues in SQL
// (server-side filter/paginate) — replacing the old epics/wip/hygiene blobs.

// TaskRow is an issue with its blocking links attached (epic child / wip / etc.).
type TaskRow struct {
	Key       string     `json:"key"`
	Summary   string     `json:"summary"`
	Pod       string     `json:"pod"`
	Status    string     `json:"status"`
	Type      string     `json:"type"`
	Points    *float64   `json:"points"`
	Created   *time.Time `json:"created"`
	Resolved  *time.Time `json:"resolved"`
	Assignee  string     `json:"assignee"`
	Blocks    []string   `json:"blocks"`
	BlockedBy []string   `json:"blockedBy"`
}

// linksFor returns, for a set of issue keys, the blocks/blockedBy maps.
func (d *DB) linksFor(ctx context.Context, snapshotID string, keys []string) (blocks, blockedBy map[string][]string, err error) {
	blocks, blockedBy = map[string][]string{}, map[string][]string{}
	if len(keys) == 0 {
		return
	}
	rows, err := d.pool.Query(ctx,
		`SELECT blocker_key, blocked_key FROM snapshot_issue_links
		 WHERE snapshot_id=$1 AND (blocker_key = ANY($2) OR blocked_key = ANY($2))`, snapshotID, keys)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, nil, err
		}
		blocks[a] = append(blocks[a], b)       // a blocks b
		blockedBy[b] = append(blockedBy[b], a) // b is blocked by a
	}
	return blocks, blockedBy, rows.Err()
}

// EdgeIssue is one (blocker, blocked) issue pair behind an Org Network edge —
// the individual Jira links that were counted into that from->to pod's
// aggregated edge count (see jira.Aggregate).
type EdgeIssue struct {
	BlockerKey     string `json:"blockerKey"`
	BlockerSummary string `json:"blockerSummary"`
	BlockedKey     string `json:"blockedKey"`
	BlockedSummary string `json:"blockedSummary"`
}

// EdgeIssues returns the issue-link pairs whose blocker is in fromPod and
// whose blocked issue is in toPod — the drill-down behind a double-clicked
// Org Network edge.
func (d *DB) EdgeIssues(snapshotID, fromPod, toPod string) ([]EdgeIssue, error) {
	rows, err := d.pool.Query(context.Background(), `
		SELECT l.blocker_key, bl.summary, l.blocked_key, bd.summary
		FROM snapshot_issue_links l
		JOIN snapshot_issues bl ON bl.snapshot_id=l.snapshot_id AND bl.key=l.blocker_key
		JOIN snapshot_issues bd ON bd.snapshot_id=l.snapshot_id AND bd.key=l.blocked_key
		WHERE l.snapshot_id=$1 AND bl.pod=$2 AND bd.pod=$3
		ORDER BY l.blocker_key, l.blocked_key`, snapshotID, fromPod, toPod)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EdgeIssue{}
	for rows.Next() {
		var e EdgeIssue
		if err := rows.Scan(&e.BlockerKey, &e.BlockerSummary, &e.BlockedKey, &e.BlockedSummary); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// WipItem is one in-progress issue with its freeze verdict.
type WipItem struct {
	Key      string     `json:"key"`
	Summary  string     `json:"summary"`
	Assignee string     `json:"assignee"`
	Created  *time.Time `json:"-"`
	Updated  *time.Time `json:"-"`
	Blocks   []string   `json:"blocksKeys"`
	Verdict  string     `json:"verdict"`
}

// wipModeCond restricts an issue (aliased i, joined to its snapshot as sn) to
// those counted as "wip" under the snapshot's chosen counting rule — an
// in-progress epic and its in-progress children must not both count. See
// jira.WipModeLeaf / jira.WipModeEpicOrParentless.
const wipModeCond = `(
	  (sn.wip_mode <> 'epic_or_parentless' AND i.issue_type NOT IN ('Epic','Parent Epic'))
	  OR (sn.wip_mode = 'epic_or_parentless' AND (i.issue_type IN ('Epic','Parent Epic') OR i.parent_key = ''))
	)`

// WipPage returns a pod's in-progress issues, freeze-verdict-ordered + paginated,
// plus the totals. verdict: keep (blocks others) > freeze (stale/unassigned) > review.
func (d *DB) WipPage(snapshotID, pod string, limit, offset int) (items []WipItem, total, freezable int, err error) {
	ctx := context.Background()
	err = d.pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE NOT EXISTS(
		         SELECT 1 FROM snapshot_issue_links l WHERE l.snapshot_id=i.snapshot_id AND l.blocker_key=i.key)
		         AND (i.assignee='' OR (i.updated IS NOT NULL AND i.updated < now()-interval '14 days')))
		FROM snapshot_issues i JOIN snapshots sn ON sn.id=i.snapshot_id
		WHERE i.snapshot_id=$1 AND i.pod=$2 AND i.status_cat='indeterminate' AND `+wipModeCond, snapshotID, pod).Scan(&total, &freezable)
	if err != nil {
		return
	}
	rows, err := d.pool.Query(ctx, `
		SELECT key, summary, assignee, created, updated,
		  COALESCE((SELECT array_agg(l.blocked_key) FROM snapshot_issue_links l
		            WHERE l.snapshot_id=q.snapshot_id AND l.blocker_key=q.key), '{}') AS blocks,
		  CASE WHEN q.blocks_others THEN 'keep'
		       WHEN q.assignee='' OR (q.updated IS NOT NULL AND q.updated < now()-interval '14 days') THEN 'freeze'
		       ELSE 'review' END AS verdict
		FROM (
		  SELECT i.*, EXISTS(SELECT 1 FROM snapshot_issue_links l
		           WHERE l.snapshot_id=i.snapshot_id AND l.blocker_key=i.key) AS blocks_others
		  FROM snapshot_issues i JOIN snapshots sn ON sn.id=i.snapshot_id
		  WHERE i.snapshot_id=$1 AND i.pod=$2 AND i.status_cat='indeterminate' AND `+wipModeCond+`
		) q
		ORDER BY (CASE WHEN q.blocks_others THEN 2
		               WHEN q.assignee='' OR (q.updated IS NOT NULL AND q.updated < now()-interval '14 days') THEN 0
		               ELSE 1 END), q.updated NULLS FIRST
		LIMIT $3 OFFSET $4`, snapshotID, pod, limit, offset)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var it WipItem
		if err = rows.Scan(&it.Key, &it.Summary, &it.Assignee, &it.Created, &it.Updated, &it.Blocks, &it.Verdict); err != nil {
			return
		}
		items = append(items, it)
	}
	return items, total, freezable, rows.Err()
}

// WipSummary returns org-wide in-progress totals (all pods in the roster):
// total WIP and how many look freezable (stale/unassigned, nothing waiting).
func (d *DB) WipSummary(snapshotID string) (total, freezable int, err error) {
	err = d.pool.QueryRow(context.Background(), `
		SELECT count(*),
		       count(*) FILTER (WHERE NOT EXISTS(
		         SELECT 1 FROM snapshot_issue_links l WHERE l.snapshot_id=i.snapshot_id AND l.blocker_key=i.key)
		         AND (i.assignee='' OR (i.updated IS NOT NULL AND i.updated < now()-interval '14 days')))
		FROM snapshot_issues i JOIN snapshots sn ON sn.id=i.snapshot_id
		WHERE i.snapshot_id=$1 AND i.status_cat='indeterminate' AND `+wipModeCond+`
		  AND i.pod IN (SELECT name FROM snapshot_pods WHERE snapshot_id=i.snapshot_id)`,
		snapshotID).Scan(&total, &freezable)
	return
}

// FeverEpic is an in-flight epic + its child tasks (for the fever chart).
type FeverEpic struct {
	Epic       string    `json:"epic"`
	Name       string    `json:"name"`
	DueDate    string    `json:"duedate"`
	HasOutcome bool      `json:"hasOutcome"`
	Tasks      []TaskRow `json:"tasks"`
}

// FeverEpics returns the N newest in-flight epics that will render (≥1 child in
// a known pod), each with its child tasks + links. Honors the caller's N.
func (d *DB) FeverEpics(snapshotID string, n int) ([]FeverEpic, error) {
	ctx := context.Background()
	rows, err := d.pool.Query(ctx, `
		SELECT e.key, e.summary, e.due_date, (e.desc_len >= 40)
		FROM snapshot_issues e
		WHERE e.snapshot_id=$1 AND e.issue_type IN ('Epic','Parent Epic') AND e.status_cat <> 'done'
		  AND EXISTS(SELECT 1 FROM snapshot_issues c
		            WHERE c.snapshot_id=e.snapshot_id AND c.parent_key=e.key
		              AND c.pod IN (SELECT name FROM snapshot_pods WHERE snapshot_id=e.snapshot_id))
		ORDER BY NULLIF(regexp_replace(e.key, '\D', '', 'g'), '')::bigint DESC NULLS LAST
		LIMIT $2`, snapshotID, n)
	if err != nil {
		return nil, err
	}
	var epics []FeverEpic
	keyIdx := map[string]int{}
	for rows.Next() {
		var e FeverEpic
		if err := rows.Scan(&e.Epic, &e.Name, &e.DueDate, &e.HasOutcome); err != nil {
			rows.Close()
			return nil, err
		}
		keyIdx[e.Epic] = len(epics)
		epics = append(epics, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(epics) == 0 {
		return epics, nil
	}
	epicKeys := make([]string, len(epics))
	for i := range epics {
		epicKeys[i] = epics[i].Epic
	}
	tasks, taskKeys, byParent, err := d.tasksByParent(ctx, snapshotID, epicKeys)
	if err != nil {
		return nil, err
	}
	blocks, blockedBy, err := d.linksFor(ctx, snapshotID, taskKeys)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		tasks[i].Blocks = orEmptyStr(blocks[tasks[i].Key])
		tasks[i].BlockedBy = orEmptyStr(blockedBy[tasks[i].Key])
	}
	for parent, idxs := range byParent {
		if ei, ok := keyIdx[parent]; ok {
			for _, ti := range idxs {
				epics[ei].Tasks = append(epics[ei].Tasks, tasks[ti])
			}
		}
	}
	return epics, nil
}

// tasksByParent loads all child tasks of the given epics.
func (d *DB) tasksByParent(ctx context.Context, snapshotID string, epicKeys []string) (tasks []TaskRow, taskKeys []string, byParent map[string][]int, err error) {
	rows, err := d.pool.Query(ctx, `
		SELECT key, summary, pod, status, issue_type, points, created, resolved, assignee, parent_key
		FROM snapshot_issues WHERE snapshot_id=$1 AND parent_key = ANY($2)`, snapshotID, epicKeys)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	byParent = map[string][]int{}
	for rows.Next() {
		var t TaskRow
		var parent string
		if err := rows.Scan(&t.Key, &t.Summary, &t.Pod, &t.Status, &t.Type, &t.Points, &t.Created, &t.Resolved, &t.Assignee, &parent); err != nil {
			return nil, nil, nil, err
		}
		byParent[parent] = append(byParent[parent], len(tasks))
		taskKeys = append(taskKeys, t.Key)
		tasks = append(tasks, t)
	}
	return tasks, taskKeys, byParent, rows.Err()
}

// EpicWithTasks loads one epic + its tasks (Feature Simulator).
func (d *DB) EpicWithTasks(snapshotID, key string) (*FeverEpic, error) {
	ctx := context.Background()
	var e FeverEpic
	err := d.pool.QueryRow(ctx,
		`SELECT key, summary, due_date, (desc_len>=40) FROM snapshot_issues WHERE snapshot_id=$1 AND key=$2`,
		snapshotID, key).Scan(&e.Epic, &e.Name, &e.DueDate, &e.HasOutcome)
	if err != nil {
		return nil, err
	}
	tasks, taskKeys, _, err := d.tasksByParent(ctx, snapshotID, []string{key})
	if err != nil {
		return nil, err
	}
	blocks, blockedBy, err := d.linksFor(ctx, snapshotID, taskKeys)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		tasks[i].Blocks = orEmptyStr(blocks[tasks[i].Key])
		tasks[i].BlockedBy = orEmptyStr(blockedBy[tasks[i].Key])
	}
	e.Tasks = tasks
	return &e, nil
}

// IssueRef is a lightweight {key, summary, detail} for hygiene lists.
type IssueRef struct {
	Key     string `json:"key"`
	Summary string `json:"summary"`
	Detail  string `json:"detail"`
}

// UnassocEpics returns epics whose owner pod isn't a team in the snapshot
// (empty pod, and no child in a known pod).
func (d *DB) UnassocEpics(snapshotID string, limit int) ([]map[string]string, error) {
	rows, err := d.pool.Query(context.Background(), `
		SELECT e.key, e.summary, e.pod FROM snapshot_issues e
		WHERE e.snapshot_id=$1 AND e.issue_type IN ('Epic','Parent Epic')
		  AND e.pod NOT IN (SELECT name FROM snapshot_pods WHERE snapshot_id=e.snapshot_id)
		  AND NOT EXISTS(SELECT 1 FROM snapshot_issues c WHERE c.snapshot_id=e.snapshot_id AND c.parent_key=e.key
		                 AND c.pod IN (SELECT name FROM snapshot_pods WHERE snapshot_id=e.snapshot_id))
		ORDER BY NULLIF(regexp_replace(e.key,'\D','','g'),'')::bigint DESC NULLS LAST
		LIMIT $2`, snapshotID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var k, name, pod string
		if err := rows.Scan(&k, &name, &pod); err != nil {
			return nil, err
		}
		out = append(out, map[string]string{"key": k, "name": name, "pod": pod})
	}
	return out, rows.Err()
}

// EpicCount is the number of epics in the snapshot (for "N of M" labels).
func (d *DB) EpicCount(snapshotID string) (int, error) {
	var n int
	err := d.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM snapshot_issues WHERE snapshot_id=$1 AND issue_type IN ('Epic','Parent Epic')`, snapshotID).Scan(&n)
	return n, err
}

// EpicStats returns org-level epic counts for the Data Quality cards.
func (d *DB) EpicStats(snapshotID string) (known, missing, overdue, noDue int, err error) {
	err = d.pool.QueryRow(context.Background(), `
		SELECT count(*),
		  count(*) FILTER (WHERE desc_len < 40),
		  count(*) FILTER (WHERE due_date <> '' AND due_date < to_char(now(),'YYYY-MM-DD')),
		  count(*) FILTER (WHERE due_date = '')
		FROM snapshot_issues
		WHERE snapshot_id=$1 AND issue_type IN ('Epic','Parent Epic') AND status_cat <> 'done'`,
		snapshotID).Scan(&known, &missing, &overdue, &noDue)
	return
}

// HygieneIssueLists returns a pod's data-quality problem issues by category.
func (d *DB) HygieneIssueLists(snapshotID, pod string) (map[string][]IssueRef, error) {
	ctx := context.Background()
	out := map[string][]IssueRef{"unsized": {}, "stale": {}, "unassigned": {}, "nooutcome": {}}
	collect := func(cat, q string, args ...any) error {
		rows, err := d.pool.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r IssueRef
			if err := rows.Scan(&r.Key, &r.Summary, &r.Detail); err != nil {
				return err
			}
			out[cat] = append(out[cat], r)
		}
		return rows.Err()
	}
	if err := collect("unsized", `
		SELECT key, left(summary,100), 'epic '||parent_key FROM snapshot_issues
		WHERE snapshot_id=$1 AND pod=$2 AND issue_type NOT IN ('Epic','Parent Epic')
		  AND status_cat<>'done' AND points IS NULL LIMIT 300`, snapshotID, pod); err != nil {
		return nil, err
	}
	if err := collect("stale", `
		SELECT i.key, left(i.summary,100), 'untouched '||round(extract(epoch from now()-i.updated)/86400)::text||'d'
		FROM snapshot_issues i JOIN snapshots sn ON sn.id=i.snapshot_id
		WHERE i.snapshot_id=$1 AND i.pod=$2 AND i.status_cat='indeterminate' AND `+wipModeCond+`
		  AND i.updated IS NOT NULL AND i.updated < now()-interval '14 days' LIMIT 300`, snapshotID, pod); err != nil {
		return nil, err
	}
	if err := collect("unassigned", `
		SELECT i.key, left(i.summary,100), 'in progress, no assignee' FROM snapshot_issues i
		JOIN snapshots sn ON sn.id=i.snapshot_id
		WHERE i.snapshot_id=$1 AND i.pod=$2 AND i.status_cat='indeterminate' AND i.assignee='' AND `+wipModeCond+` LIMIT 300`, snapshotID, pod); err != nil {
		return nil, err
	}
	if err := collect("nooutcome", `
		SELECT key, left(summary,100),
		  'no business outcome in description'||CASE WHEN due_date<>'' THEN ' · due '||due_date ELSE '' END
		FROM snapshot_issues e WHERE snapshot_id=$1 AND issue_type IN ('Epic','Parent Epic') AND desc_len < 40
		  AND (pod=$2 OR EXISTS(SELECT 1 FROM snapshot_issues c WHERE c.snapshot_id=e.snapshot_id AND c.parent_key=e.key AND c.pod=$2))
		LIMIT 300`, snapshotID, pod); err != nil {
		return nil, err
	}
	return out, nil
}

func orEmptyStr(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
