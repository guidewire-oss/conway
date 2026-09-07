package db

import (
	"context"
	"conway/server/evidence"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
)

var ErrEvidenceConflict = errors.New("source changed or capture already running")
var ErrEvidenceOwner = errors.New("source owner no longer has manager access")

type EvidenceSource struct {
	ID           string          `json:"id"`
	Owner        string          `json:"owner"`
	Config       evidence.Config `json:"config"`
	Credential   []byte          `json:"-"`
	Version      int64           `json:"version"`
	NextAt       int64           `json:"nextAt"`
	ActiveRun    string          `json:"activeRun"`
	LeaseUntil   int64           `json:"leaseUntil"`
	LastSuccess  int64           `json:"lastSuccess"`
	LastSnapshot string          `json:"lastSnapshot"`
	LastStatus   string          `json:"lastStatus"`
	LastError    string          `json:"lastError"`
}
type EvidenceRun struct {
	ID         string          `json:"id"`
	SourceID   string          `json:"sourceId"`
	Status     string          `json:"status"`
	StartedAt  int64           `json:"startedAt"`
	FinishedAt int64           `json:"finishedAt"`
	SnapshotID string          `json:"snapshotId"`
	Error      string          `json:"error"`
	Config     evidence.Config `json:"config"`
	Version    int64           `json:"version"`
}

const evidenceColumns = `id,owner,config,credential,version,next_at,active_run,lease_until,last_success,last_snapshot,last_status,last_error`

func scanEvidence(row pgx.Row) (*EvidenceSource, error) {
	var s EvidenceSource
	var config []byte
	err := row.Scan(&s.ID, &s.Owner, &config, &s.Credential, &s.Version, &s.NextAt, &s.ActiveRun, &s.LeaseUntil, &s.LastSuccess, &s.LastSnapshot, &s.LastStatus, &s.LastError)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(config, &s.Config); err != nil {
		return nil, err
	}
	return &s, nil
}
func (d *DB) GetEvidenceSource(ctx context.Context, id string) (*EvidenceSource, error) {
	return scanEvidence(d.pool.QueryRow(ctx, `SELECT `+evidenceColumns+` FROM evidence_sources WHERE id=$1`, id))
}
func (d *DB) ListEvidenceSources(ctx context.Context, owner string, all bool) ([]EvidenceSource, error) {
	rows, err := d.pool.Query(ctx, `SELECT `+evidenceColumns+` FROM evidence_sources WHERE owner=$1 OR $2 ORDER BY lower(config->>'name'),id`, owner, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EvidenceSource{}
	for rows.Next() {
		s, e := scanEvidence(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}
func (d *DB) CreateEvidenceSource(ctx context.Context, s EvidenceSource) error {
	config, err := json.Marshal(s.Config)
	if err != nil {
		return err
	}
	_, err = d.pool.Exec(ctx, `INSERT INTO evidence_sources(id,owner,config,credential,next_at) VALUES($1,$2,$3,$4,$5)`, s.ID, s.Owner, config, s.Credential, s.NextAt)
	return err
}
func (d *DB) UpdateEvidenceSource(ctx context.Context, s EvidenceSource, now int64) error {
	config, err := json.Marshal(s.Config)
	if err != nil {
		return err
	}
	tag, err := d.pool.Exec(ctx, `UPDATE evidence_sources SET config=$2,credential=$3,version=version+1,next_at=CASE WHEN config->'intervalHours' IS NOT DISTINCT FROM $2::jsonb->'intervalHours' AND config->'enabled' IS NOT DISTINCT FROM $2::jsonb->'enabled' THEN next_at ELSE $4 END WHERE id=$1 AND version=$5 AND (active_run='' OR lease_until<=$6)`, s.ID, config, s.Credential, s.NextAt, s.Version, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrEvidenceConflict
	}
	return nil
}
func evidenceOwner(ctx context.Context, tx pgx.Tx, owner string, now int64) error {
	var roles []string
	var expiry int64
	err := tx.QueryRow(ctx, `SELECT COALESCE(roles,ARRAY[role]),expires_at FROM accounts WHERE username=$1 FOR SHARE`, owner).Scan(&roles, &expiry)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEvidenceOwner
	}
	if err != nil {
		return err
	}
	if expiry != 0 && expiry <= now {
		return ErrEvidenceOwner
	}
	for _, r := range roles {
		if r == "manager" || r == "admin" {
			return nil
		}
	}
	return ErrEvidenceOwner
}

// per specs/026-reliable-evidence-foundation.md:128
func (d *DB) ClaimEvidence(ctx context.Context, id, run string, now int64, manual bool) (*EvidenceSource, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	s, err := scanEvidence(tx.QueryRow(ctx, `SELECT `+evidenceColumns+` FROM evidence_sources WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, ErrEvidenceConflict
	}
	if s.ActiveRun != "" && s.LeaseUntil > now {
		return nil, ErrEvidenceConflict
	}
	if !manual && (!s.Config.Enabled || s.Config.IntervalHours == 0 || (s.NextAt > now && s.ActiveRun == "")) {
		return nil, ErrEvidenceConflict
	}
	if s.ActiveRun != "" {
		if _, err = tx.Exec(ctx, `UPDATE evidence_runs SET status='interrupted',finished_at=$2,error='Capture interrupted. Retry uses a new attempt.' WHERE id=$1 AND status='running'`, s.ActiveRun, now); err != nil {
			return nil, err
		}
	}
	ownerErr := evidenceOwner(ctx, tx, s.Owner, now)
	if ownerErr != nil && !errors.Is(ownerErr, ErrEvidenceOwner) {
		return nil, ownerErr
	}
	config, _ := json.Marshal(s.Config)
	status, msg, finish, active, lease := "running", "", int64(0), run, now+1200
	if ownerErr != nil {
		status, msg, finish, active, lease = "failed", "Source owner no longer has manager access. Restore access before retrying.", now, "", 0
	}
	if _, err = tx.Exec(ctx, `INSERT INTO evidence_runs(id,source_id,status,started_at,finished_at,error,config,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, run, id, status, now, finish, msg, config, s.Version); err != nil {
		return nil, err
	}
	next := now + int64(max(s.Config.IntervalHours, 1))*3600
	if _, err = tx.Exec(ctx, `UPDATE evidence_sources SET active_run=$2,lease_until=$3,last_status=$4,last_error=$5,next_at=$6 WHERE id=$1`, id, active, lease, status, msg, next); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	if ownerErr != nil {
		return nil, ownerErr
	}
	s.ActiveRun = run
	s.LeaseUntil = lease
	return s, nil
}
func (d *DB) EvidenceRuns(ctx context.Context, id string) ([]EvidenceRun, error) {
	rows, err := d.pool.Query(ctx, `SELECT id,source_id,status,started_at,finished_at,snapshot_id,error,config,version FROM evidence_runs WHERE source_id=$1 ORDER BY run_order DESC LIMIT 20`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EvidenceRun{}
	for rows.Next() {
		var r EvidenceRun
		var config []byte
		if err = rows.Scan(&r.ID, &r.SourceID, &r.Status, &r.StartedAt, &r.FinishedAt, &r.SnapshotID, &r.Error, &config, &r.Version); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(config, &r.Config); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FinishEvidence fences obsolete workers and commits evidence and attempt state together.
// per specs/026-reliable-evidence-foundation.md:128
func (d *DB) FinishEvidence(ctx context.Context, id, run string, now int64, snapshot SnapshotRow, data SnapshotData, identities []byte, message string) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	s, err := scanEvidence(tx.QueryRow(ctx, `SELECT `+evidenceColumns+` FROM evidence_sources WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return err
	}
	if s == nil || s.ActiveRun != run || s.LeaseUntil <= now {
		return ErrEvidenceConflict
	}
	if message == "" {
		if e := evidenceOwner(ctx, tx, s.Owner, now); e != nil {
			if !errors.Is(e, ErrEvidenceOwner) {
				return e
			}
			message = "Source owner no longer has manager access. Restore access before retrying."
		}
	}
	status := "failed"
	snapID := ""
	lastSuccess, lastSnapshot := s.LastSuccess, s.LastSnapshot
	if message == "" {
		if snapshot.ID == "" || len(identities) == 0 {
			return fmt.Errorf("capture evidence is incomplete")
		}
		status = "succeeded"
		snapID = snapshot.ID
		lastSuccess = now
		lastSnapshot = snapID
		if _, err = tx.Exec(ctx, `INSERT INTO snapshots(id,owner,name,scope,source,roster_id,wip_mode,created_at) VALUES($1,$2,$3,$4,'jira',$5,$6,$7)`, snapshot.ID, s.Owner, snapshot.Name, nullJSON(snapshot.Scope), s.Config.RosterID, s.Config.WipMode, snapshot.CreatedAt); err != nil {
			return err
		}
		if err = writeSnapshotData(ctx, tx, snapID, data); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO snapshot_docs(snapshot_id,path,body) VALUES($1,'identities.json',$2)`, snapID, identities); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE evidence_runs SET status=$2,finished_at=$3,snapshot_id=$4,error=$5 WHERE id=$1`, run, status, now, snapID, message); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE evidence_sources SET active_run='',lease_until=0,last_status=$2,last_error=$3,last_success=$4,last_snapshot=$5,next_at=$6 WHERE id=$1`, id, status, message, lastSuccess, lastSnapshot, now+int64(max(s.Config.IntervalHours, 1))*3600); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d *DB) EvidenceActor(ctx context.Context, owner string, now int64) ([]string, error) {
	var roles []string
	var expiry int64
	err := d.pool.QueryRow(ctx, `SELECT COALESCE(roles,ARRAY[role]),expires_at FROM accounts WHERE username=$1`, owner).Scan(&roles, &expiry)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEvidenceOwner
	}
	if err != nil {
		return nil, err
	}
	if expiry != 0 && expiry <= now {
		return nil, ErrEvidenceOwner
	}
	for _, role := range roles {
		if role == "manager" || role == "admin" {
			return roles, nil
		}
	}
	return nil, ErrEvidenceOwner
}
