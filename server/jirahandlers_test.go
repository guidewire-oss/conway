package main

import (
	"net/http/httptest"
	"strings"

	"conway/server/auth"
	"conway/server/db"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("the Jira import structure", func() {
	It("requires a roster — imports without one are refused", func() {
		s := &server{}
		_, err := s.importStructure(nil)
		Expect(err).To(HaveOccurred(), "a roster is mandatory for JIRA imports")
	})

	It("clamps a negative dev count to zero", func() {
		s := &server{}
		pods, err := s.importStructure([]NetPod{{Name: "Alpha", DevCount: -3}})
		Expect(err).NotTo(HaveOccurred())
		Expect(pods[0].DevCount).To(Equal(0))
	})

	// A JIRA import without a rosterId must be rejected before ever contacting
	// Jira — team composition drifts over time, so a snapshot must be pinned to
	// a dated roster for planning to stay accurate.
	It("rejects an import without a rosterId before contacting Jira", func() {
		s := &server{db: &db.DB{}, store: auth.NewStore([]byte("x"))}
		body := `{"name":"Q3 import","projects":["PROJ"]}`
		req := httptest.NewRequest("POST", "/api/snapshots/import", strings.NewReader(body))
		rec := httptest.NewRecorder()
		s.handleSnapshotImport(rec, req, auth.Claims{Sub: "mgr1"})
		Expect(rec.Code).To(Equal(400))
		Expect(strings.ToLower(rec.Body.String())).To(ContainSubstring("roster"),
			"the error must name the missing roster")
	})
})
