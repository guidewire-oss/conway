package main

import (
	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("execution review access and decisions", func() {
	It("does not expose private snapshot evidence to another plan owner", func() {
		snap := &db.SnapshotRow{Owner: "owner"}
		viewer := auth.Claims{Sub: "viewer"}
		Expect(canReadExecutionSnapshot(snap, viewer)).To(BeFalse())
		Expect(canReadExecutionSnapshot(nil, viewer)).To(BeFalse())
		Expect(canReadExecutionSnapshot(snap, auth.Claims{Sub: "owner"})).To(BeTrue())
		snap.Public = true
		Expect(canReadExecutionSnapshot(snap, viewer)).To(BeTrue())
	})
	It("requires actionable decisions with a valid owner, rationale and review date", func() {
		v := db.ExecutionDecision{Action: "Review dependency", Owner: "Team lead", Rationale: "Awaiting upstream readiness", ReviewDate: "2026-09-12", Initiative: "Atlas"}
		inits := []planning.Initiative{{Name: "Atlas"}}
		Expect(validateExecutionDecision(&v, inits)).To(Succeed())
		v.ReviewDate = "2026-02-30"
		Expect(validateExecutionDecision(&v, inits)).NotTo(Succeed())
		v.ReviewDate = "2026-09-12"
		v.Initiative = "Missing"
		Expect(validateExecutionDecision(&v, inits)).NotTo(Succeed())
		v.Initiative = "Atlas"
		v.Action = " "
		Expect(validateExecutionDecision(&v, inits)).NotTo(Succeed())
	})
})
