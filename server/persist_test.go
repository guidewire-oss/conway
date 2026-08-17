package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"conway/server/game"
)

func newPersistServer(sp string) *server {
	return &server{
		teams:     map[string]map[string]json.RawMessage{},
		games:     map[string]map[string]*game.Game{},
		sessions:  map[string]*gameSession{},
		statePath: sp,
	}
}

// A pod replacement (redeploy / eviction) must not wipe live games: the snapshot
// has to round-trip per-game sessions + games + standings.
func TestStatePersistRoundTrip(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "game-state.json")
	s := newPersistServer(sp)
	s.sessions[defaultGameID] = &gameSession{Rounds: 4, Ap: 5, TimerSecs: 300, Open: true, OpenRound: 2}
	s.gmap(defaultGameID)["team1"] = &game.Game{Round: 2, TotalRounds: 4, ApPerRound: 5,
		Staged: []game.Move{{Lever: "wipLimit", Round: 2}}}
	s.tmap(defaultGameID)["team1"] = json.RawMessage(`{"round":2,"score":{"total":42}}`)
	s.saveState()

	if _, err := os.Stat(sp); err != nil {
		t.Fatalf("snapshot not written: %v", err)
	}

	// a brand-new pod boots against the same snapshot
	s2 := newPersistServer(sp)
	s2.loadState()

	if !s2.sess(defaultGameID).Open || s2.sess(defaultGameID).OpenRound != 2 {
		t.Fatalf("session not restored: %+v", s2.sess(defaultGameID))
	}
	g := s2.gmap(defaultGameID)["team1"]
	if g == nil || g.Round != 2 || len(g.Staged) != 1 {
		t.Fatalf("game/staged not restored: %+v", g)
	}
	if string(s2.tmap(defaultGameID)["team1"]) == "" {
		t.Fatalf("standings not restored")
	}
}

// A missing snapshot (truly fresh deploy) must be a clean no-op, not an error.
func TestLoadStateNoFileIsNoop(t *testing.T) {
	s := newPersistServer(filepath.Join(t.TempDir(), "absent.json"))
	s.loadState()
	if len(s.gmap(defaultGameID)) != 0 || s.sess(defaultGameID).OpenRound != 0 {
		t.Fatalf("fresh boot should stay empty, got games=%d round=%d",
			len(s.gmap(defaultGameID)), s.sess(defaultGameID).OpenRound)
	}
}
