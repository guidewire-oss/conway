package main

import (
	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
	"errors"
	"net/http"
	"slices"
	"time"
)

// specs/028-portfolio-forecasts.md:122: a read-only computation still needs
// current plan ownership and current persisted manager authorization.
func (s *server) handleForecast(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r)
		return
	}
	if _, ok := s.forecastAccess(w, r, p, c); !ok {
		return
	}
	var settings planning.ForecastSettings
	if !decodeReviewBody(w, r, &settings) {
		return
	}
	inputs, err := s.planScheduleFor(p, scheduleRequest{})
	if err != nil {
		if message, invalid := windowError(err); invalid {
			http.Error(w, message, http.StatusBadRequest)
		} else {
			s.logger().Error().Str("plan", p.ID).Err(err).Msg("forecast inputs unreadable")
			http.Error(w, "Saved planning inputs could not be read. Retry or contact an administrator.", http.StatusInternalServerError)
		}
		return
	}
	result, err := planning.ComputeForecast(inputs, settings)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, result)
}

func (s *server) forecastAccess(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims) (auth.Claims, bool) {
	roles, err := s.db.EvidenceActor(r.Context(), c.Sub, time.Now().Unix())
	if err != nil {
		if errors.Is(err, db.ErrEvidenceOwner) {
			http.Error(w, "Current manager access is required. Sign in again.", http.StatusForbidden)
		} else {
			s.logger().Error().Err(err).Msg("forecast account lookup failed")
			http.Error(w, "Account access could not be checked. Retry when the service is available.", http.StatusInternalServerError)
		}
		return c, false
	}
	if c.GameID != "" || (!c.Has("manager") && !c.Has("admin")) || (!slices.Contains(roles, "manager") && !slices.Contains(roles, "admin")) || (p.Owner != c.Sub && !slices.Contains(roles, "admin")) {
		http.Error(w, "Manager access to this plan is required.", http.StatusForbidden)
		return c, false
	}
	c.Roles = roles
	return c, true
}
