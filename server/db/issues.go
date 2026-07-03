package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// IssueRow is one Jira issue captured in a snapshot (the queryable source of
// truth that replaces the epics/wip/hygiene JSON blobs).
type IssueRow struct {
	Key       string
	Pod       string
	IssueType string
	Status    string
	StatusCat string
	Assignee  string
	Points    *float64
	Summary   string
	DescLen   int
	ParentKey string
	Created   *time.Time
	Updated   *time.Time
	Resolved  *time.Time
	DueDate   string
}

// PodRow is per-pod structure (from the roster); PodStatRow / EdgeRow /
// PodHygieneRow are the aggregates materialized at import.
type PodRow struct {
	Name     string
	Location string
	Pairing  bool
	DevCount int
	Streams  int
	Sre      bool
}
type PodStatRow struct {
	Pod             string
	Resolved180d    int
	P50, P85, Mean  float64
	Mu, Sigma       float64
	WipCount        int
	ThroughputPerWk float64
}
type EdgeRow struct {
	From, To string
	Count    int
}
type PodHygieneRow struct {
	Pod                                                   string
	SizedPct, MedianPoints, StaleWipPct, UnassignedWipPct *float64
	LinkDensity, Score                                    *float64
	SampleSized, WipCount                                 int
}

// --- reads (the dynamic docs are generated from these on the fly) ---

func (d *DB) Pods(snapshotID string) ([]PodRow, error) {
	rows, err := d.pool.Query(context.Background(),
		`SELECT name,location,pairing,dev_count,streams,sre FROM snapshot_pods WHERE snapshot_id=$1 ORDER BY name`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PodRow{}
	for rows.Next() {
		var p PodRow
		if err := rows.Scan(&p.Name, &p.Location, &p.Pairing, &p.DevCount, &p.Streams, &p.Sre); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) PodStats(snapshotID string) ([]PodStatRow, error) {
	rows, err := d.pool.Query(context.Background(),
		`SELECT pod,resolved_180d,p50,p85,mean,mu,sigma,wip_count,throughput_per_week FROM snapshot_pod_stats WHERE snapshot_id=$1`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PodStatRow{}
	for rows.Next() {
		var s PodStatRow
		if err := rows.Scan(&s.Pod, &s.Resolved180d, &s.P50, &s.P85, &s.Mean, &s.Mu, &s.Sigma, &s.WipCount, &s.ThroughputPerWk); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (d *DB) Edges(snapshotID string) ([]EdgeRow, error) {
	rows, err := d.pool.Query(context.Background(),
		`SELECT from_pod,to_pod,count FROM snapshot_edges WHERE snapshot_id=$1`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EdgeRow{}
	for rows.Next() {
		var e EdgeRow
		if err := rows.Scan(&e.From, &e.To, &e.Count); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *DB) Hygiene(snapshotID string) ([]PodHygieneRow, error) {
	rows, err := d.pool.Query(context.Background(),
		`SELECT pod,sized_pct,median_points,stale_wip_pct,unassigned_wip_pct,link_density,score,sample_sized,wip_count FROM snapshot_pod_hygiene WHERE snapshot_id=$1`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PodHygieneRow{}
	for rows.Next() {
		var h PodHygieneRow
		if err := rows.Scan(&h.Pod, &h.SizedPct, &h.MedianPoints, &h.StaleWipPct, &h.UnassignedWipPct,
			&h.LinkDensity, &h.Score, &h.SampleSized, &h.WipCount); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// SetSnapshotPods rewrites just the pod structure (roster re-association), in a
// transaction, leaving issues/stats/edges/hygiene intact.
func (d *DB) SetSnapshotPods(snapshotID string, pods []PodRow) error {
	ctx := context.Background()
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM snapshot_pods WHERE snapshot_id=$1`, snapshotID); err != nil {
		return err
	}
	rows := make([][]any, len(pods))
	for i, p := range pods {
		rows[i] = []any{snapshotID, p.Name, p.Location, p.Pairing, p.DevCount, p.Streams, p.Sre}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"snapshot_pods"},
		[]string{"snapshot_id", "name", "location", "pairing", "dev_count", "streams", "sre"}, pgx.CopyFromRows(rows)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SnapshotData bundles all of a snapshot's dynamic rows for one atomic write.
type SnapshotData struct {
	Issues  []IssueRow
	Links   [][2]string
	Pods    []PodRow
	Stats   []PodStatRow
	Edges   []EdgeRow
	Hygiene []PodHygieneRow
}

// CreateSnapshotWithData inserts the snapshot row AND all its dynamic data in a
// single transaction — so a crash mid-import commits nothing (no half-snapshot).
func (d *DB) CreateSnapshotWithData(s SnapshotRow, data SnapshotData) error {
	ctx := context.Background()
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	wipMode := s.WipMode
	if wipMode == "" {
		wipMode = "leaf"
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO snapshots(id,owner,name,scope,source,roster_id,wip_mode,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		s.ID, s.Owner, s.Name, nullJSON(s.Scope), s.Source, s.RosterID, wipMode, s.CreatedAt); err != nil {
		return err
	}
	if err := writeSnapshotData(ctx, tx, s.ID, data); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ReplaceSnapshotData rewrites an existing snapshot's dynamic data (used by
// roster re-association / template edits).
func (d *DB) ReplaceSnapshotData(snapshotID string, data SnapshotData) error {
	ctx := context.Background()
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := writeSnapshotData(ctx, tx, snapshotID, data); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// writeSnapshotData clears and bulk-loads (COPY) the six per-snapshot data tables.
func writeSnapshotData(ctx context.Context, tx pgx.Tx, snapshotID string, d SnapshotData) error {
	for _, t := range []string{"snapshot_issues", "snapshot_issue_links", "snapshot_pods",
		"snapshot_pod_stats", "snapshot_edges", "snapshot_pod_hygiene"} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+t+" WHERE snapshot_id=$1", snapshotID); err != nil {
			return err
		}
	}
	issues, links, pods, stats, edges, hygiene := d.Issues, d.Links, d.Pods, d.Stats, d.Edges, d.Hygiene

	copyIn := func(table string, cols []string, rows [][]any) error {
		_, err := tx.CopyFrom(ctx, pgx.Identifier{table}, cols, pgx.CopyFromRows(rows))
		return err
	}

	ir := make([][]any, len(issues))
	for i, s := range issues {
		ir[i] = []any{snapshotID, s.Key, s.Pod, s.IssueType, s.Status, s.StatusCat, s.Assignee,
			s.Points, s.Summary, s.DescLen, s.ParentKey, s.Created, s.Updated, s.Resolved, s.DueDate}
	}
	if err := copyIn("snapshot_issues", []string{"snapshot_id", "key", "pod", "issue_type", "status",
		"status_cat", "assignee", "points", "summary", "desc_len", "parent_key", "created", "updated",
		"resolved", "due_date"}, ir); err != nil {
		return err
	}

	lr := make([][]any, 0, len(links))
	seen := map[[2]string]bool{}
	for _, l := range links {
		if l[0] == "" || l[1] == "" || seen[l] {
			continue
		}
		seen[l] = true
		lr = append(lr, []any{snapshotID, l[0], l[1]})
	}
	if err := copyIn("snapshot_issue_links", []string{"snapshot_id", "blocker_key", "blocked_key"}, lr); err != nil {
		return err
	}

	pr := make([][]any, len(pods))
	for i, p := range pods {
		pr[i] = []any{snapshotID, p.Name, p.Location, p.Pairing, p.DevCount, p.Streams, p.Sre}
	}
	if err := copyIn("snapshot_pods", []string{"snapshot_id", "name", "location", "pairing", "dev_count", "streams", "sre"}, pr); err != nil {
		return err
	}

	sr := make([][]any, len(stats))
	for i, s := range stats {
		sr[i] = []any{snapshotID, s.Pod, s.Resolved180d, s.P50, s.P85, s.Mean, s.Mu, s.Sigma, s.WipCount, s.ThroughputPerWk}
	}
	if err := copyIn("snapshot_pod_stats", []string{"snapshot_id", "pod", "resolved_180d", "p50", "p85",
		"mean", "mu", "sigma", "wip_count", "throughput_per_week"}, sr); err != nil {
		return err
	}

	er := make([][]any, len(edges))
	for i, e := range edges {
		er[i] = []any{snapshotID, e.From, e.To, e.Count}
	}
	if err := copyIn("snapshot_edges", []string{"snapshot_id", "from_pod", "to_pod", "count"}, er); err != nil {
		return err
	}

	hr := make([][]any, len(hygiene))
	for i, h := range hygiene {
		hr[i] = []any{snapshotID, h.Pod, h.SizedPct, h.MedianPoints, h.StaleWipPct, h.UnassignedWipPct,
			h.LinkDensity, h.Score, h.SampleSized, h.WipCount}
	}
	if err := copyIn("snapshot_pod_hygiene", []string{"snapshot_id", "pod", "sized_pct", "median_points",
		"stale_wip_pct", "unassigned_wip_pct", "link_density", "score", "sample_sized", "wip_count"}, hr); err != nil {
		return err
	}
	return nil
}
