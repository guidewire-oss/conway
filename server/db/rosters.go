package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// RosterRow is a reusable team-structure definition. Pods is a JSON []NetPod
// (the main package's shape) so this layer stays decoupled from it.
type RosterRow struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Name      string `json:"name"`
	Pods      []byte `json:"-"` // JSON []NetPod
	PodCount  int    `json:"podCount"`
	Public    bool   `json:"public"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

func (d *DB) CreateRoster(r RosterRow) error {
	_, err := d.pool.Exec(context.Background(),
		`INSERT INTO rosters(id,owner,name,pods,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$5)`,
		r.ID, r.Owner, r.Name, nullJSON(r.Pods), r.CreatedAt)
	return err
}

func (d *DB) ListRosters(owner string, all bool) ([]RosterRow, error) {
	q := `SELECT id,owner,name,COALESCE(jsonb_array_length(pods),0),public,created_at,updated_at FROM rosters`
	var args []any
	if !all {
		q += ` WHERE owner=$1 OR public`
		args = append(args, owner)
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := d.pool.Query(context.Background(), q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RosterRow{}
	for rows.Next() {
		var r RosterRow
		if err := rows.Scan(&r.ID, &r.Owner, &r.Name, &r.PodCount, &r.Public, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) SetRosterPublic(id string, public bool) error {
	_, err := d.pool.Exec(context.Background(), `UPDATE rosters SET public=$2 WHERE id=$1`, id, public)
	return err
}

func (d *DB) GetRoster(id string) (*RosterRow, error) {
	var r RosterRow
	err := d.pool.QueryRow(context.Background(),
		`SELECT id,owner,name,pods,public,created_at,updated_at FROM rosters WHERE id=$1`, id).
		Scan(&r.ID, &r.Owner, &r.Name, &r.Pods, &r.Public, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) UpdateRoster(id, name string, pods []byte, updatedAt int64) error {
	_, err := d.pool.Exec(context.Background(),
		`UPDATE rosters SET name=$2,pods=$3,updated_at=$4 WHERE id=$1`, id, name, nullJSON(pods), updatedAt)
	return err
}

func (d *DB) DeleteRoster(id string) error {
	_, err := d.pool.Exec(context.Background(), `DELETE FROM rosters WHERE id=$1`, id)
	return err
}
