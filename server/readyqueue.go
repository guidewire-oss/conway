package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
)

func decodeReadyQueueBody(w http.ResponseWriter, r *http.Request, body any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(body); err != nil {
		http.Error(w, "provide a valid next-work request", http.StatusBadRequest)
		return false
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "provide one JSON object", http.StatusBadRequest)
		return false
	}
	return true
}

// Queue decisions authorize an operational choice; they never save schedule
// inputs or imply observed starts. specs/025-team-ready-work-queue.md:305
func (s *server) handleReadyQueue(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims, sub string) {
	if (!c.Has("manager") && !c.Has("admin")) || c.GameID != "" || (p.Owner != c.Sub && !c.Has("admin")) {
		http.Error(w, "manager access to this plan is required", http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet && sub == "ready-queue/history" {
		team, initiative := r.URL.Query().Get("team"), r.URL.Query().Get("initiative")
		if strings.TrimSpace(team) == "" || strings.TrimSpace(initiative) == "" || len(team) > 200 || len(initiative) > 1000 {
			http.Error(w, "choose the team and initiative whose history you want", http.StatusBadRequest)
			return
		}
		history, err := s.db.ReadyQueueHistory(r.Context(), p.ID, team, initiative)
		if err != nil {
			s.readyQueueFailure(w, err)
			return
		}
		writeJSON(w, history)
		return
	}
	if r.Method == http.MethodGet && sub == "ready-queue" {
		week, err := strconv.Atoi(r.URL.Query().Get("asOfWeek"))
		if err != nil {
			http.Error(w, "choose an explicit integer as-of planning week", http.StatusBadRequest)
			return
		}
		queue, _, code, msg := s.prepareReadyQueue(r.Context(), p, r.URL.Query().Get("team"), week)
		if code != http.StatusOK {
			http.Error(w, msg, code)
			return
		}
		writeJSON(w, queue)
		return
	}
	if r.Method != http.MethodPost || (sub != "ready-queue/confirmations" && sub != "ready-queue/decisions") {
		methodNotAllowed(w, r)
		return
	}
	var team, initiative, expected, kind, owner, evidence string
	var week *int
	var checks []planning.ReadyCheck
	if sub == "ready-queue/confirmations" {
		var body struct {
			Team                string                `json:"team"`
			Initiative          string                `json:"initiative"`
			AsOfWeek            *int                  `json:"asOfWeek"`
			ExpectedFingerprint string                `json:"expectedFingerprint"`
			Checks              []planning.ReadyCheck `json:"checks"`
		}
		if !decodeReadyQueueBody(w, r, &body) {
			return
		}
		team, initiative, week, expected, checks = body.Team, body.Initiative, body.AsOfWeek, body.ExpectedFingerprint, body.Checks
		kind = "confirmation"
	} else {
		var body struct {
			Team                string `json:"team"`
			Initiative          string `json:"initiative"`
			AsOfWeek            *int   `json:"asOfWeek"`
			ExpectedFingerprint string `json:"expectedFingerprint"`
			Decision            string `json:"decision"`
			Owner               string `json:"owner"`
			Evidence            string `json:"evidence"`
		}
		if !decodeReadyQueueBody(w, r, &body) {
			return
		}
		team, initiative, week, expected, kind, owner, evidence = body.Team, body.Initiative, body.AsOfWeek, body.ExpectedFingerprint, body.Decision, strings.TrimSpace(body.Owner), strings.TrimSpace(body.Evidence)
		if (kind != "release" && kind != "defer" && kind != "reconsider") || owner == "" || evidence == "" || len(owner) > 500 || len(evidence) > 10000 {
			http.Error(w, "choose release, defer or reconsider with an owner and evidence", http.StatusBadRequest)
			return
		}
	}
	if week == nil || expected == "" || len(expected) > 200 || strings.TrimSpace(initiative) == "" || len(initiative) > 1000 {
		http.Error(w, "a current queue fingerprint, initiative and explicit week are required", http.StatusBadRequest)
		return
	}
	queue, guard, code, msg := s.prepareReadyQueue(r.Context(), p, team, *week)
	if code != http.StatusOK {
		http.Error(w, msg, code)
		return
	}
	if queue.Fingerprint != expected {
		http.Error(w, "the planning or readiness context changed; refresh the queue before saving", http.StatusConflict)
		return
	}
	var item *planning.ReadyQueueItem
	for i := range queue.Items {
		if queue.Items[i].Initiative == initiative {
			item = &queue.Items[i]
			break
		}
	}
	if item == nil {
		http.Error(w, "this initiative is not assigned to the selected team in this plan", http.StatusNotFound)
		return
	}
	if kind == "release" && !item.CanRelease {
		http.Error(w, "this work is not currently eligible for release; review its queue reasons", http.StatusBadRequest)
		return
	}
	if (kind == "defer" || kind == "reconsider") && (item.State == "in_progress" || item.State == "complete") {
		http.Error(w, "reported carryover or complete context cannot be deferred or reconsidered", http.StatusBadRequest)
		return
	}
	if kind == "defer" && item.State == "deferred" {
		http.Error(w, "this work is already deferred; reconsider it explicitly", http.StatusBadRequest)
		return
	}
	if kind == "reconsider" && (item.LastDecision == nil || item.LastDecision.Decision != "defer") {
		http.Error(w, "only a deliberate deferral can be reconsidered", http.StatusBadRequest)
		return
	}
	var confirmation *planning.ReadyConfirmation
	var decision *planning.ReleaseDecision
	var response any
	if kind == "confirmation" {
		var err error
		checks, err = planning.ValidateReadyChecks(checks)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		confirmation = &planning.ReadyConfirmation{ID: newID(), PlanID: p.ID, Team: team, Initiative: initiative, AsOfWeek: *week, PlanFingerprint: queue.Context.PlanFingerprint, Checks: checks, CreatedBy: c.Sub, CreatedAt: time.Now().Unix()}
		response = confirmation
	} else {
		decision = &planning.ReleaseDecision{ID: newID(), PlanID: p.ID, Team: team, Initiative: initiative, AsOfWeek: *week, PlanFingerprint: queue.Context.PlanFingerprint, Decision: kind, Owner: owner, Evidence: evidence, CreatedBy: c.Sub, CreatedAt: time.Now().Unix()}
		response = decision
	}
	ok, err := s.db.AppendReadyQueueEvent(r.Context(), confirmation, decision, guard)
	if err != nil {
		s.readyQueueFailure(w, err)
		return
	}
	if !ok {
		http.Error(w, "the planning or readiness context changed; refresh the queue before saving", http.StatusConflict)
		return
	}
	writeJSON(w, response)
}

func (s *server) prepareReadyQueue(ctx context.Context, p *db.PlanRow, team string, week int) (planning.ReadyQueue, db.ReadyQueueGuard, int, string) {
	if len(team) > 200 {
		return planning.ReadyQueue{}, db.ReadyQueueGuard{}, http.StatusBadRequest, "team name is too long"
	}
	inputs, err := s.planScheduleFor(p, scheduleRequest{})
	if err != nil {
		return planning.ReadyQueue{}, db.ReadyQueueGuard{}, http.StatusBadRequest, err.Error()
	}
	history, err := s.db.ReadyQueueHistory(ctx, p.ID, "", "")
	if err != nil {
		return planning.ReadyQueue{}, db.ReadyQueueGuard{}, http.StatusInternalServerError, "could not load next-work history"
	}
	// Confirmation freshness and optimistic writes use the same complete saved
	// planning context, including site hours and the containing plan identity.
	// specs/025-team-ready-work-queue.md:333
	planRaw, err := json.Marshal(struct {
		ID                                    string
		Teams, Initiatives, Scheduling, Sites json.RawMessage
		HorizonWeeks, CapacityLoss            float64
	}{p.ID, p.Teams, p.Initiatives, p.Scheduling, p.Sites, p.HorizonWeeks, p.CapacityLoss})
	if err != nil {
		return planning.ReadyQueue{}, db.ReadyQueueGuard{}, http.StatusInternalServerError, "could not identify planning inputs"
	}
	planSum := sha256.Sum256(planRaw)
	queue, err := planning.BuildReadyQueue(planning.ReadyQueueInput{PlanID: p.ID, PlanFingerprint: hex.EncodeToString(planSum[:]), Inputs: inputs, Schedule: inputs.RecomputeWith(planning.ScheduleOptions{}), Team: team, AsOfWeek: week, Readiness: history.Confirmations, Decisions: history.Decisions})
	if err != nil {
		return queue, db.ReadyQueueGuard{}, http.StatusBadRequest, err.Error()
	}
	// Raw site configuration also participates in the atomic plan guard.
	raw, _ := json.Marshal(struct {
		Queue string
		Order int64
		Sites json.RawMessage
	}{queue.Fingerprint, history.Order, p.Sites})
	sum := sha256.Sum256(raw)
	queue.Fingerprint = hex.EncodeToString(sum[:])
	return queue, db.ReadyQueueGuard{Plan: p, Order: history.Order}, http.StatusOK, ""
}

func (s *server) readyQueueFailure(w http.ResponseWriter, err error) {
	s.logger().Error().Err(err).Msg("next-work request failed")
	http.Error(w, "could not save or load next work; retry after refreshing the queue", http.StatusInternalServerError)
}
