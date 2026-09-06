package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
)

func readProposalBody(w http.ResponseWriter, r *http.Request, body any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(body); err != nil {
		http.Error(w, "Invalid proposal: "+err.Error(), http.StatusBadRequest)
		return false
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		http.Error(w, "Send one proposal object", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *server) clonePlanScenario(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims) {
	var body struct {
		Name string `json:"name"`
	}
	if !readProposalBody(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len([]rune(name)) > 100 {
		http.Error(w, "Name the scenario using 1–100 characters", http.StatusBadRequest)
		return
	}
	id := newID()
	ok, err := s.db.ClonePlan(p.ID, id, c.Sub, name, time.Now().Unix())
	if err != nil {
		http.Error(w, "Could not create the scenario", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "Source plan no longer exists", http.StatusConflict)
		return
	}
	s.recordEvent(c.Sub, "scenario_created", id, map[string]any{"sourcePlan": p.ID})
	writeJSON(w, map[string]any{"id": id, "name": name})
}

type remedyProposalRequest struct {
	Remedy      planning.Remedy `json:"remedy"`
	Fingerprint string          `json:"fingerprint,omitempty"`
}

func (s *server) previewPlanRemedy(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims) {
	s.planRemedyProposal(w, r, p, c, false)
}
func (s *server) applyPlanRemedy(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims) {
	s.planRemedyProposal(w, r, p, c, true)
}

func (s *server) planRemedyProposal(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims, apply bool) {
	var body remedyProposalRequest
	if !readProposalBody(w, r, &body) {
		return
	}
	in, err := s.planScheduleFor(p, scheduleRequest{})
	if err != nil {
		http.Error(w, "Could not read current planning inputs", http.StatusBadRequest)
		return
	}
	if apply && (body.Fingerprint == "" || body.Fingerprint != in.Fingerprint()) {
		http.Error(w, "The working plan changed. Preview this remedy again before applying it.", http.StatusConflict)
		return
	}
	out, before, after, err := planning.PreviewRemedy(in, body.Remedy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if !apply {
		s.recordEvent(c.Sub, "remedy_previewed", p.ID, map[string]any{"kind": body.Remedy.Kind})
		writeJSON(w, map[string]any{"fingerprint": in.Fingerprint(), "remedy": body.Remedy,
			"before": before, "after": after, "comparison": planning.CompareToBaseline(before, after)})
		return
	}
	tb, err := json.Marshal(out.Teams)
	if err != nil {
		http.Error(w, "Invalid team inputs", http.StatusBadRequest)
		return
	}
	ib, err := json.Marshal(out.Initiatives)
	if err != nil {
		http.Error(w, "Invalid initiative inputs", http.StatusBadRequest)
		return
	}
	ok, err := s.db.SavePlanProposalIfUnchanged(p, tb, ib, time.Now().Unix())
	if err != nil {
		http.Error(w, "Could not save the proposed change", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "The plan changed while applying. Preview again.", http.StatusConflict)
		return
	}
	s.recordEvent(c.Sub, "remedy_applied", p.ID, map[string]any{"kind": body.Remedy.Kind})
	writeJSON(w, map[string]any{"teams": out.Teams, "initiatives": out.Initiatives, "schedule": after})
}
