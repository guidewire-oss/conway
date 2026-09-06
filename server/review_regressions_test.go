package main

import (
	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Review regression import boundaries", func() {
	// per specs/017-planning-and-execution-usability.md:200
	// per specs/017-planning-and-execution-usability.md:88
	DescribeTable("refuses the same invalid explicit epic binding in preview and upload", func(save bool) {
		teams := []planning.Team{{Name: "Beacon", Tracks: 2}}
		inits := []planning.Initiative{{Name: "Atlas", EpicKeys: []string{"invalid key"}, Work: map[string]planning.TeamWork{"Beacon": {Weeks: 2, Estimated: true, InPath: true}}}}
		file := planning.WriteInitiativesXLSX(teams, inits)
		body, contentType := multipartFileG("file", "initiatives.xlsx", file)
		req := httptest.NewRequest(http.MethodPost, "/api/plan/one/initiatives/preview", body)
		req.Header.Set("Content-Type", contentType)
		rec := httptest.NewRecorder()
		stored, err := json.Marshal([]planning.Initiative{{Name: "Atlas", EpicKeys: []string{"PROJ-1"}}})
		Expect(err).NotTo(HaveOccurred())
		teamData, err := json.Marshal(teams)
		Expect(err).NotTo(HaveOccurred())
		plan := &db.PlanRow{ID: "one", HorizonWeeks: 26, Initiatives: stored, Teams: teamData}
		before := append([]byte(nil), plan.Initiatives...)
		srv := &server{}
		if save {
			srv.uploadPlanInitiatives(rec, req, plan, auth.Claims{})
		} else {
			srv.previewPlanInitiatives(rec, req, plan)
		}
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("epic key"))
		Expect([]byte(plan.Initiatives)).To(Equal(before))
	}, Entry("preview", false), Entry("upload", true))
})
