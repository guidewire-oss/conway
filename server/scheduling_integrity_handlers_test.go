package main

import (
	"bytes"
	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
	"encoding/json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http/httptest"
)

var _ = Describe("Scheduling identity boundaries", func() {
	duplicates := func() []planning.Initiative {
		return []planning.Initiative{
			{Name: "Alpha", Work: map[string]planning.TeamWork{"Atlas": {Weeks: 1, Estimated: true, InPath: true}}},
			{Name: " alpha ", Work: map[string]planning.TeamWork{"Atlas": {Weeks: 20, Estimated: true, InPath: true}}},
		}
	}
	DescribeTable("rejects duplicate names in a spreadsheet before preview or persistence", func(save bool) {
		teams := []planning.Team{{Name: "Atlas", Tracks: 2}}
		file := planning.WriteInitiativesXLSX(teams, duplicates())
		body, ct := multipartFileG("file", "initiatives.xlsx", file)
		req := httptest.NewRequest("POST", "/api/plan/one/initiatives", body)
		req.Header.Set("Content-Type", ct)
		rec := httptest.NewRecorder()
		srv := &server{}
		plan := &db.PlanRow{ID: "one", HorizonWeeks: 26}
		if save {
			srv.uploadPlanInitiatives(rec, req, plan, auth.Claims{})
		} else {
			srv.previewPlanInitiatives(rec, req, plan)
		}
		Expect(rec.Code).To(Equal(400))
		Expect(rec.Body.String()).To(ContainSubstring("duplicate"))
		Expect(plan.Initiatives).To(BeNil())
	}, Entry("preview", false), Entry("save", true))
	DescribeTable("rejects duplicate draft rows before computing a schedule or simulation", func(simulation bool) {
		payload, _ := json.Marshal(map[string]any{"initiatives": duplicates()})
		req := httptest.NewRequest("POST", "/api/plan/one/schedule", bytes.NewReader(payload))
		rec := httptest.NewRecorder()
		srv := &server{}
		plan := &db.PlanRow{ID: "one", HorizonWeeks: 26}
		if simulation {
			srv.simulatePlan(rec, req, plan)
		} else {
			srv.schedulePlan(rec, req, plan, auth.Claims{})
		}
		Expect(rec.Code).To(Equal(400))
		Expect(rec.Body.String()).To(ContainSubstring("duplicate"))
		Expect(plan.Initiatives).To(BeNil())
	}, Entry("schedule", false), Entry("simulation", true))
})
