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
	"strings"
)

var _ = Describe("remedy preview and application guards", func() {
	var srv *server
	var plan *db.PlanRow
	BeforeEach(func() {
		teams, inits := planning.Demo()
		tb, err := json.Marshal(teams)
		Expect(err).NotTo(HaveOccurred())
		ib, err := json.Marshal(inits)
		Expect(err).NotTo(HaveOccurred())
		sb, err := json.Marshal(planning.DemoScheduling())
		Expect(err).NotTo(HaveOccurred())
		plan = &db.PlanRow{ID: "plan1", Teams: tb, Initiatives: ib, Scheduling: sb, HorizonWeeks: 26, CapacityLoss: 0.1}
		srv = &server{}
	})
	It("previews with no database or persistence and leaves the saved row byte-identical", func() {
		in, err := srv.planScheduleFor(plan, scheduleRequest{})
		Expect(err).NotTo(HaveOccurred())
		offered := planning.ComputeRemedies(in.Teams, in.Initiatives, in.Params, in.Scheduling, nil)
		Expect(offered).NotTo(BeEmpty())
		original, err := json.Marshal(plan)
		Expect(err).NotTo(HaveOccurred())
		body, err := json.Marshal(remedyProposalRequest{Remedy: offered[0]})
		Expect(err).NotTo(HaveOccurred())
		rec := httptest.NewRecorder()
		srv.previewPlanRemedy(rec, httptest.NewRequest("POST", "/api/plan/plan1/schedule/remedies/preview", bytes.NewReader(body)), plan, auth.Claims{})
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		var result struct {
			Fingerprint   string `json:"fingerprint"`
			Before, After planning.Schedule
		}
		Expect(json.Unmarshal(rec.Body.Bytes(), &result)).To(Succeed())
		Expect(result.Fingerprint).To(Equal(in.Fingerprint()))
		Expect(result.Before.Initiatives).NotTo(BeEmpty())
		Expect(result.After.Initiatives).NotTo(BeEmpty())
		current, err := json.Marshal(plan)
		Expect(err).NotTo(HaveOccurred())
		Expect(current).To(Equal(original))
	})
	It("rejects missing or stale preview identity before any persistence", func() {
		for _, body := range []string{`{"remedy":{}}`, `{"remedy":{},"fingerprint":"outdated"}`} {
			rec := httptest.NewRecorder()
			srv.applyPlanRemedy(rec, httptest.NewRequest("POST", "/", strings.NewReader(body)), plan, auth.Claims{})
			Expect(rec.Code).To(Equal(409))
		}
	})
	It("rejects unknown fields, concatenated objects and oversized requests", func() {
		for _, body := range []string{`{"remedy":{},"unreviewedInputs":[]}`, `{"remedy":{}} {"remedy":{}}`, `{"remedy":{},"fingerprint":"` + strings.Repeat("x", 70<<10) + `"}`} {
			rec := httptest.NewRecorder()
			srv.previewPlanRemedy(rec, httptest.NewRequest("POST", "/", strings.NewReader(body)), plan, auth.Claims{})
			Expect(rec.Code).To(Equal(400))
		}
	})
})
