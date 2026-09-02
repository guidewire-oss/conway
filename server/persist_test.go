package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"conway/server/game"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newPersistServer(sp string) *server {
	return &server{
		teams:     map[string]map[string]json.RawMessage{},
		games:     map[string]map[string]*game.Game{},
		sessions:  map[string]*gameSession{},
		statePath: sp,
	}
}

var _ = Describe("game-state persistence", func() {
	It("round-trips sessions, games and standings across a pod replacement", func() {
		sp := filepath.Join(GinkgoT().TempDir(), "game-state.json")
		s := newPersistServer(sp)
		s.sessions[defaultGameID] = &gameSession{Rounds: 4, Ap: 5, TimerSecs: 300, Open: true, OpenRound: 2}
		s.gmap(defaultGameID)["team1"] = &game.Game{Round: 2, TotalRounds: 4, ApPerRound: 5,
			Staged: []game.Move{{Lever: "wipLimit", Round: 2}}}
		s.tmap(defaultGameID)["team1"] = json.RawMessage(`{"round":2,"score":{"total":42}}`)
		s.saveState()

		_, err := os.Stat(sp)
		Expect(err).NotTo(HaveOccurred(), "snapshot not written")

		// a brand-new pod boots against the same snapshot
		s2 := newPersistServer(sp)
		s2.loadState()

		Expect(s2.sess(defaultGameID).Open).To(BeTrue())
		Expect(s2.sess(defaultGameID).OpenRound).To(Equal(2), "session not restored")
		g := s2.gmap(defaultGameID)["team1"]
		Expect(g).NotTo(BeNil())
		Expect(g.Round).To(Equal(2))
		Expect(g.Staged).To(HaveLen(1), "staged move not restored")
		Expect(string(s2.tmap(defaultGameID)["team1"])).NotTo(BeEmpty(), "standings not restored")
	})

	It("treats a missing snapshot as a clean no-op", func() {
		s := newPersistServer(filepath.Join(GinkgoT().TempDir(), "absent.json"))
		s.loadState()
		Expect(s.gmap(defaultGameID)).To(BeEmpty())
		Expect(s.sess(defaultGameID).OpenRound).To(Equal(0), "fresh boot should stay empty")
	})
})
