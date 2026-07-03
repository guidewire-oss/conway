package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	"conway/auth"
	"conway/db"
)

func TestImportStructureRequiresRoster(t *testing.T) {
	s := &server{}
	if _, err := s.importStructure(nil); err == nil {
		t.Fatal("expected an error when no roster is given — a roster is mandatory for JIRA imports")
	}
}

func TestImportStructureClampsNegativeDevCount(t *testing.T) {
	s := &server{}
	pods, err := s.importStructure([]NetPod{{Name: "Alpha", DevCount: -3}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pods[0].DevCount != 0 {
		t.Fatalf("DevCount = %d, want clamped to 0", pods[0].DevCount)
	}
}

// A JIRA import without a rosterId must be rejected before ever contacting
// Jira — team composition drifts over time, so a snapshot must be pinned to
// a dated roster for planning to stay accurate.
func TestHandleSnapshotImportRequiresRosterID(t *testing.T) {
	s := &server{db: &db.DB{}, store: auth.NewStore([]byte("x"))}
	body := `{"name":"Q3 import","projects":["the reference plan"]}`
	req := httptest.NewRequest("POST", "/api/snapshots/import", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleSnapshotImport(rec, req, auth.Claims{Sub: "mgr1"})
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "roster") {
		t.Fatalf("error message = %q, want it to mention the missing roster", rec.Body.String())
	}
}
