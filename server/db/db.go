// Package db is the Postgres persistence backend for Conway. It implements
// auth.Backend (accounts + signing secret) and stores the live game snapshot,
// replacing the JSON-file-on-PVC persistence so state survives pod replacement
// (redeploy / eviction) and can later be shared by multiple replicas.
//
// Phase 0 keeps the complex game snapshot as a JSONB document; the relational
// tables for the planning feature land in Phase 1.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver for goose
	"github.com/pressly/goose/v3"

	"conway/auth"
)

type DB struct{ pool *pgxpool.Pool }

// versioned schema migrations, embedded so the binary self-migrates on boot.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// baselineSeedSQL is a small synthetic demo org (NOT a goose migration — it's
// data, not schema, and applying it is conditional on CONWAY_SEED_BASELINE).
// Idempotent (ON CONFLICT DO NOTHING), so it's also safe to apply by hand
// anytime: psql $DATABASE_URL -f server/db/seed/baseline.sql
//
//go:embed seed/baseline.sql
var baselineSeedSQL string

// Open connects, waits for the database to accept connections (CNPG may still be
// starting), and runs the goose migrations.
func Open(ctx context.Context, url string) (*DB, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	var perr error
	for i := 0; i < 60; i++ { // ~2min: tolerate CNPG still provisioning on first deploy
		if perr = pool.Ping(ctx); perr == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if perr != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", perr)
	}
	if err := migrate(url); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &DB{pool: pool}, nil
}

// migrate runs the embedded goose migrations over a short-lived database/sql
// connection (goose needs *sql.DB; the app itself uses the pgx pool).
func migrate(url string) error {
	sqldb, err := sql.Open("pgx", url)
	if err != nil {
		return err
	}
	defer sqldb.Close()
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(sqldb, "migrations")
}

func (d *DB) Close() { d.pool.Close() }

// SeedBaseline applies the embedded baseline demo dataset. Idempotent — safe
// to call every boot regardless of whether it was already applied.
func (d *DB) SeedBaseline(ctx context.Context) error {
	_, err := d.pool.Exec(ctx, baselineSeedSQL)
	return err
}

// ---- auth.Backend: accounts + signing secret ----------------------------

// Save persists the whole account set + signing secret (a small set; a
// truncate+insert in one transaction mirrors the file store's whole-state write).
func (d *DB) Save(s *auth.Store) error {
	ctx := context.Background()
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`INSERT INTO app_kv(key,val) VALUES('secret',$1)
		 ON CONFLICT(key) DO UPDATE SET val=EXCLUDED.val`, s.Secret); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM accounts`); err != nil {
		return err
	}
	for _, u := range s.Users {
		primary := "player" // legacy NOT NULL column; the roles[] array is authoritative
		if len(u.Roles) > 0 {
			primary = u.Roles[0]
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO accounts(username,display,role,roles,salt,hash,expires_at,created_at)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
			u.Username, u.Display, primary, u.Roles, u.Salt, u.Hash, u.ExpiresAt, u.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Load fills the store from the DB (empty tables == fresh start, not an error).
func (d *DB) Load(s *auth.Store) error {
	ctx := context.Background()
	var secret []byte
	if err := d.pool.QueryRow(ctx, `SELECT val FROM app_kv WHERE key='secret'`).Scan(&secret); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	if len(secret) > 0 {
		s.Secret = secret
	}
	// COALESCE migrates any legacy single-role rows into the roles[] array.
	rows, err := d.pool.Query(ctx,
		`SELECT username,display,COALESCE(roles, ARRAY[role]),salt,hash,expires_at,created_at FROM accounts`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if s.Users == nil {
		s.Users = map[string]*auth.User{}
	}
	for rows.Next() {
		u := &auth.User{}
		if err := rows.Scan(&u.Username, &u.Display, &u.Roles, &u.Salt, &u.Hash, &u.ExpiresAt, &u.CreatedAt); err != nil {
			return err
		}
		s.Users[u.Username] = u
	}
	if s.NowUnix == nil {
		s.NowUnix = func() int64 { return time.Now().Unix() }
	}
	return rows.Err()
}

// ---- live game snapshot -------------------------------------------------

func (d *DB) SaveGameState(b []byte) error {
	_, err := d.pool.Exec(context.Background(),
		`INSERT INTO app_kv(key,val) VALUES('game_state',$1)
		 ON CONFLICT(key) DO UPDATE SET val=EXCLUDED.val`, b)
	return err
}

// LoadGameState returns the snapshot, or (nil,nil) when none has been saved.
func (d *DB) LoadGameState() ([]byte, error) {
	var b []byte
	err := d.pool.QueryRow(context.Background(), `SELECT val FROM app_kv WHERE key='game_state'`).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return b, err
}
