package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// SnapshotRow is a dated capture of the org network. The documents (pods,
// stats, edges, hygiene, epics, …) live in snapshot_docs as path -> JSON blob;
// this row is the metadata both Observe and Train key off.
type SnapshotRow struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Name      string `json:"name"`
	Scope     []byte `json:"-"`        // JSON []string of project keys (null for baseline)
	Source    string `json:"source"`   // baseline | jira | template
	Public    bool   `json:"public"`   // shared: visible to (and seedable by) everyone
	RosterID  string `json:"rosterId"` // roster the structure came from (jira imports)
	WipMode   string `json:"wipMode"`  // leaf | epic_or_parentless — see jira.WipMode*
	CreatedAt int64  `json:"createdAt"`
	DocCount  int    `json:"docCount"` // populated by ListSnapshots
}

func (d *DB) CreateSnapshot(s SnapshotRow) error {
	_, err := d.pool.Exec(context.Background(),
		`INSERT INTO snapshots(id,owner,name,scope,source,roster_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		s.ID, s.Owner, s.Name, nullJSON(s.Scope), s.Source, s.RosterID, s.CreatedAt)
	return err
}

func (d *DB) SetSnapshotRoster(id, rosterID string) error {
	_, err := d.pool.Exec(context.Background(), `UPDATE snapshots SET roster_id=$2 WHERE id=$1`, id, rosterID)
	return err
}

// nullJSON keeps an empty/zero blob as SQL NULL so jsonb columns stay valid.
func nullJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func (d *DB) GetSnapshot(id string) (*SnapshotRow, error) {
	var s SnapshotRow
	err := d.pool.QueryRow(context.Background(),
		`SELECT id,owner,name,scope,source,public,roster_id,wip_mode,created_at FROM snapshots WHERE id=$1`, id).
		Scan(&s.ID, &s.Owner, &s.Name, &s.Scope, &s.Source, &s.Public, &s.RosterID, &s.WipMode, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListSnapshots returns snapshot metadata (no doc bodies), newest first. The
// baseline and any public snapshot are visible to everyone; otherwise admins see
// all and owners see their own.
func (d *DB) ListSnapshots(owner string, all bool) ([]SnapshotRow, error) {
	q := `SELECT s.id,s.owner,s.name,s.scope,s.source,s.public,s.roster_id,s.created_at,
	             (SELECT count(*) FROM snapshot_docs WHERE snapshot_id=s.id)
	      FROM snapshots s`
	var args []any
	if !all {
		q += ` WHERE s.owner=$1 OR s.source='baseline' OR s.public`
		args = append(args, owner)
	}
	q += ` ORDER BY s.created_at DESC`
	rows, err := d.pool.Query(context.Background(), q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SnapshotRow{}
	for rows.Next() {
		var s SnapshotRow
		if err := rows.Scan(&s.ID, &s.Owner, &s.Name, &s.Scope, &s.Source, &s.Public, &s.RosterID, &s.CreatedAt, &s.DocCount); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (d *DB) SetSnapshotPublic(id string, public bool) error {
	_, err := d.pool.Exec(context.Background(), `UPDATE snapshots SET public=$2 WHERE id=$1`, id, public)
	return err
}

func (d *DB) DeleteSnapshot(id string) error {
	_, err := d.pool.Exec(context.Background(), `DELETE FROM snapshots WHERE id=$1`, id)
	return err
}

func (d *DB) RenameSnapshot(id, name string) error {
	_, err := d.pool.Exec(context.Background(), `UPDATE snapshots SET name=$2 WHERE id=$1`, id, name)
	return err
}

// PutSnapshotDoc upserts one document (path -> JSON body) into a snapshot.
func (d *DB) PutSnapshotDoc(snapshotID, path string, body []byte) error {
	_, err := d.pool.Exec(context.Background(),
		`INSERT INTO snapshot_docs(snapshot_id,path,body) VALUES($1,$2,$3)
		 ON CONFLICT (snapshot_id,path) DO UPDATE SET body=EXCLUDED.body`,
		snapshotID, path, body)
	return err
}

// GetSnapshotDoc returns one document body, or (nil,nil) if absent.
func (d *DB) GetSnapshotDoc(snapshotID, path string) ([]byte, error) {
	var body []byte
	err := d.pool.QueryRow(context.Background(),
		`SELECT body FROM snapshot_docs WHERE snapshot_id=$1 AND path=$2`, snapshotID, path).Scan(&body)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return body, err
}

// ListSnapshotDocPaths returns the document paths held by a snapshot.
func (d *DB) ListSnapshotDocPaths(snapshotID string) ([]string, error) {
	rows, err := d.pool.Query(context.Background(),
		`SELECT path FROM snapshot_docs WHERE snapshot_id=$1 ORDER BY path`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
