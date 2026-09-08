package main

import (
	"encoding/json"
	"io"
	"net/http"

	"conway/server/auth"
	"conway/server/db"
)

type announcementAction struct {
	Type   string `json:"type"`
	Route  string `json:"route,omitempty"`
	Target string `json:"target,omitempty"`
	Parent string `json:"parent,omitempty"`
}

type featureAnnouncement struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Action      announcementAction `json:"action"`
	db.FeatureAnnouncementState
}

// specs/022-feature-announcements.md:192: stable versioned IDs and local actions.
// Availability is checked for reads AND acknowledgements, never trusted from UI.
func (s *server) announcementCatalog(c auth.Claims) []featureAnnouncement {
	features := []featureAnnouncement{}
	if c.Sub == "" || c.GameID != "" {
		return features
	}

	if c.Has("manager") || c.Has("admin") {
		if s.db != nil {
			features = append(features, featureAnnouncement{ID: "forecast-history-validation-v1", Title: "Validate forecasts across history", Description: "Check recorded predictions across distinct work groups. See monthly coverage, repeated work and evidence gaps without inflating counts from repeated forecasts.", Action: announcementAction{Type: "route", Route: "?view=plan&planView=forecast", Target: "view-forecast", Parent: "plan-btn"}})
			features = append(features, featureAnnouncement{ID: "forecast-prediction-history-v1", Title: "Learn from recorded predictions", Description: "Record a forecast before outcomes are known, then compare it with a later Jira capture. See observed coverage, pending work and explicit scope exclusions in Forecasts.", Action: announcementAction{Type: "route", Route: "?view=plan&planView=forecast", Target: "view-forecast", Parent: "plan-btn"}})
			features = append(features, featureAnnouncement{ID: "portfolio-forecasts-v1", Title: "Explore uncertainty before committing", Description: "Compare shorter estimates, your saved plan, and longer estimates with shared disruption. See sensitive commitments and inspect the evidence behind them.", Action: announcementAction{Type: "route", Route: "?view=plan&planView=forecast", Target: "view-forecast", Parent: "plan-btn"}})
			features = append(features, featureAnnouncement{ID: "planning-assistant-v1", Title: "Ask about your saved plan", Description: "Explain scheduling constraints, inspect agreement changes and prepare an evidence-linked review agenda from your plan.", Action: announcementAction{Type: "route", Route: "?view=plan&planView=assistant", Target: "view-assistant", Parent: "plan-btn"}})
			features = append(features, featureAnnouncement{ID: "reliable-evidence-v1", Title: "Keep review evidence current", Description: "Save a Jira capture source, schedule dated snapshots, recover failed attempts and inspect stable team identities from Measure > Snapshots.", Action: announcementAction{Type: "menu", Target: "obs-snapshots", Parent: "explore-btn"}})
		}
		features = append(features, featureAnnouncement{
			ID: "team-ready-work-v1", Title: "Review your team's next work",
			Description: "See the scheduled release window, confirm operational prerequisites, and record release or deferral decisions without changing the plan.",
			Action:      announcementAction{Type: "route", Route: "?view=plan&planView=ready", Target: "view-ready", Parent: "plan-btn"},
		})
		features = append(features, featureAnnouncement{
			ID: "execution-review-v1", Title: "Review execution against your plan",
			Description: "Open a plan's Review execution view to compare its baseline with an imported snapshot, inspect evidence gaps, and record review decisions.",
			Action:      announcementAction{Type: "route", Route: "?view=plan&planView=execution", Target: "view-execution", Parent: "plan-btn"},
		})
		features = append(features, featureAnnouncement{
			ID: "weekly-execution-review-v1", Title: "Complete a weekly execution review",
			Description: "Complete a weekly agenda, follow actions through resolution, and keep an immutable record of each review's evidence and outcomes.",
			Action:      announcementAction{Type: "route", Route: "?view=plan&planView=execution", Target: "view-execution", Parent: "plan-btn"},
		})
		if s.sheetsProvider != nil {
			features = append(features, featureAnnouncement{
				ID: "linked-sheet-import-v1", Title: "Refresh planning inputs from linked sheets",
				Description: "Open a plan's Linked sheets controls to connect a sheet, preview its current inputs, and choose when to apply them.",
				Action:      announcementAction{Type: "route", Route: "?view=plan", Target: "plan-linked-sheets", Parent: "plan-btn"},
			})
		}
	}
	features = append(features, featureAnnouncement{
		ID: "guide-navigation-v1", Title: "Find help where you work",
		Description: "Search the guide, follow a planning walkthrough, or open help for your current view from the Help menu.",
		Action:      announcementAction{Type: "menu", Target: "docs-btn", Parent: "help-btn"},
	})
	return features
}

func (s *server) handleAnnouncements(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r)
		return
	}
	if c.Sub == "" || c.GameID != "" {
		http.Error(w, "account authentication required", http.StatusUnauthorized)
		return
	}
	if s.db == nil {
		http.Error(w, "announcement persistence unavailable", http.StatusServiceUnavailable)
		return
	}
	states, err := s.db.LoadFeatureAnnouncements(r.Context(), c.Sub)
	if err != nil {
		s.announcementFailure(w, r, err)
		return
	}
	features := s.announcementCatalog(c)
	for i := range features {
		features[i].FeatureAnnouncementState = states[features[i].ID]
	}
	writeJSON(w, struct {
		Features []featureAnnouncement `json:"features"`
	}{features})
}

func (s *server) handleAnnouncementAck(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r)
		return
	}
	if c.Sub == "" || c.GameID != "" {
		http.Error(w, "account authentication required", http.StatusUnauthorized)
		return
	}
	var body struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		http.Error(w, "invalid acknowledgement", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "invalid acknowledgement", http.StatusBadRequest)
		return
	}
	if body.Kind != "announced" && body.Kind != "visited" {
		http.Error(w, "invalid acknowledgement kind", http.StatusBadRequest)
		return
	}
	eligible := false
	for _, feature := range s.announcementCatalog(c) {
		if feature.ID == body.ID {
			eligible = true
			break
		}
	}
	if !eligible {
		http.Error(w, "feature not found", http.StatusNotFound)
		return
	}
	if s.db == nil {
		http.Error(w, "announcement persistence unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := s.db.AcknowledgeFeatureAnnouncement(r.Context(), c.Sub, body.ID, body.Kind); err != nil {
		s.announcementFailure(w, r, err)
		return
	}
	states, err := s.db.LoadFeatureAnnouncements(r.Context(), c.Sub)
	if err != nil {
		s.announcementFailure(w, r, err)
		return
	}
	writeJSON(w, struct {
		ID string `json:"id"`
		db.FeatureAnnouncementState
	}{body.ID, states[body.ID]})
}

func (s *server) announcementFailure(w http.ResponseWriter, r *http.Request, err error) {
	s.logger().Error().Err(err).Str("path", r.URL.Path).Msg("announcement persistence failed")
	http.Error(w, "Announcement history could not be saved or loaded. Try again.", http.StatusServiceUnavailable)
}
