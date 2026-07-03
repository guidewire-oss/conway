package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// GameRow is a facilitator-owned game instance (its own teams, rounds, and
// leaderboard live alongside it). Multiple games run concurrently.
type GameRow struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Name      string `json:"name"`
	JoinCode  string `json:"joinCode"`
	Rounds    int    `json:"rounds"`
	Ap        int    `json:"ap"`
	TimerSecs int    `json:"timerSecs"`
	Open      bool   `json:"open"`
	OpenRound int    `json:"openRound"`
	Deadline  int64  `json:"deadline"`
	Scenario  string `json:"scenario"` // seed: default | balanced | constrained | crisis | jira | plan:<id>
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
}

func scanGame(row pgx.Row) (*GameRow, error) {
	var g GameRow
	err := row.Scan(&g.ID, &g.Owner, &g.Name, &g.JoinCode, &g.Rounds, &g.Ap, &g.TimerSecs,
		&g.Open, &g.OpenRound, &g.Deadline, &g.Scenario, &g.CreatedAt, &g.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &g, err
}

const gameCols = `id,owner,name,join_code,rounds,ap,timer_secs,open,open_round,deadline,scenario,created_at,expires_at`

func (d *DB) CreateGame(g GameRow) error {
	if g.Scenario == "" {
		g.Scenario = "default"
	}
	_, err := d.pool.Exec(context.Background(),
		`INSERT INTO games(`+gameCols+`) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		g.ID, g.Owner, g.Name, g.JoinCode, g.Rounds, g.Ap, g.TimerSecs, g.Open, g.OpenRound, g.Deadline, g.Scenario, g.CreatedAt, g.ExpiresAt)
	return err
}

func (d *DB) ListGames(owner string, all bool) ([]GameRow, error) {
	q := `SELECT ` + gameCols + ` FROM games`
	var args []any
	if !all {
		q += ` WHERE owner=$1`
		args = append(args, owner)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := d.pool.Query(context.Background(), q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GameRow{}
	for rows.Next() {
		g, err := scanGame(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

func (d *DB) GetGame(id string) (*GameRow, error) {
	return scanGame(d.pool.QueryRow(context.Background(), `SELECT `+gameCols+` FROM games WHERE id=$1`, id))
}

func (d *DB) GetGameByCode(code string) (*GameRow, error) {
	return scanGame(d.pool.QueryRow(context.Background(), `SELECT `+gameCols+` FROM games WHERE join_code=$1`, code))
}

// UpdateGameSession persists the per-game session state (rounds/ap/timer + the
// open/round/deadline run state).
func (d *DB) UpdateGameSession(g *GameRow) error {
	_, err := d.pool.Exec(context.Background(),
		`UPDATE games SET name=$2,rounds=$3,ap=$4,timer_secs=$5,open=$6,open_round=$7,deadline=$8,expires_at=$9 WHERE id=$1`,
		g.ID, g.Name, g.Rounds, g.Ap, g.TimerSecs, g.Open, g.OpenRound, g.Deadline, g.ExpiresAt)
	return err
}

// UpdateGameMeta persists facilitator-editable settings (name, round rules,
// scenario). The open/round/deadline run state is owned by UpdateGameSession.
func (d *DB) UpdateGameMeta(g *GameRow) error {
	_, err := d.pool.Exec(context.Background(),
		`UPDATE games SET name=$2,rounds=$3,ap=$4,timer_secs=$5,scenario=$6 WHERE id=$1`,
		g.ID, g.Name, g.Rounds, g.Ap, g.TimerSecs, g.Scenario)
	return err
}

// GameNameTaken reports whether owner already has a game named name (case-
// insensitive), ignoring exceptID (pass "" when creating).
func (d *DB) GameNameTaken(owner, name, exceptID string) (bool, error) {
	var n int
	err := d.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM games WHERE owner=$1 AND lower(name)=lower($2) AND id<>$3`,
		owner, name, exceptID).Scan(&n)
	return n > 0, err
}

func (d *DB) DeleteGame(id string) error {
	_, err := d.pool.Exec(context.Background(), `DELETE FROM games WHERE id=$1`, id)
	return err
}

// GameTeam is a pre-registered team in a game's roster, with its own join code.
type GameTeam struct {
	GameID    string `json:"gameId"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	CreatedAt int64  `json:"createdAt"`
}

func (d *DB) AddGameTeam(t GameTeam) error {
	_, err := d.pool.Exec(context.Background(),
		`INSERT INTO game_teams(game_id,name,code,created_at) VALUES($1,$2,$3,$4)`,
		t.GameID, t.Name, t.Code, t.CreatedAt)
	return err
}

func (d *DB) ListGameTeams(gameID string) ([]GameTeam, error) {
	rows, err := d.pool.Query(context.Background(),
		`SELECT game_id,name,code,created_at FROM game_teams WHERE game_id=$1 ORDER BY created_at`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GameTeam{}
	for rows.Next() {
		var t GameTeam
		if err := rows.Scan(&t.GameID, &t.Name, &t.Code, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (d *DB) DeleteGameTeam(gameID, name string) error {
	_, err := d.pool.Exec(context.Background(), `DELETE FROM game_teams WHERE game_id=$1 AND name=$2`, gameID, name)
	return err
}

func (d *DB) GameTeamByCode(code string) (*GameTeam, error) {
	var t GameTeam
	err := d.pool.QueryRow(context.Background(),
		`SELECT game_id,name,code,created_at FROM game_teams WHERE code=$1`, code).
		Scan(&t.GameID, &t.Name, &t.Code, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &t, err
}
