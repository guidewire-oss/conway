package main

// The one RunSpecs bootstrap for package main, and so the only file here
// permitted to import "testing" under the Go pack's dialect gate. The package's
// older stdlib tests stay as they are — see specs/002-factory-adoption.md Q2.

import (
	"net/http/httptest"
	"strings"
	"testing"

	"conway/server/auth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestServerSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "server suite")
}

// newMemStore: an in-memory auth store whose Save is a no-op — the handler's
// must(store.Save()) would log.Fatal (process exit) on a store with no path,
// which is exactly how the first version of these specs killed the suite.
func newMemStore() *auth.Store {
	st := auth.NewStore(nil)
	st.SetBackend(nopBackend{})
	return st
}

type nopBackend struct{}
func (nopBackend) Save(*auth.Store) error { return nil }
func (nopBackend) Load(*auth.Store) error { return nil }

// Extend with a custom horizon (the audit's expiry fix): an admin must be able
// to push a user out by weeks, not just the hardcoded +24h.
var _ = Describe("admin user extend", func() {
	It("extends by the hours given in the request body", func() {
		s := &server{store: newMemStore()}
		s.store.CreateUser("Bob", []string{"player"}, 1)
		before := s.store.Users["bob"].ExpiresAt

		req := httptest.NewRequest("POST", "/api/admin/users/bob/extend",
			strings.NewReader(`{"hours":168}`))
		rec := httptest.NewRecorder()
		s.handleUserItem(rec, req, auth.Claims{Sub: "admin", Roles: []string{"admin"}})

		Expect(rec.Code).To(Equal(200))
		after := s.store.Users["bob"].ExpiresAt
		Expect(after).To(Equal(before + 168*3600))
	})

	It("still defaults to 24h when no body is sent", func() {
		s := &server{store: newMemStore()}
		s.store.CreateUser("Bob", []string{"player"}, 1)
		before := s.store.Users["bob"].ExpiresAt

		req := httptest.NewRequest("POST", "/api/admin/users/bob/extend", nil)
		rec := httptest.NewRecorder()
		s.handleUserItem(rec, req, auth.Claims{Sub: "admin", Roles: []string{"admin"}})

		Expect(rec.Code).To(Equal(200))
		Expect(s.store.Users["bob"].ExpiresAt).To(Equal(before + 24*3600))
	})

	It("refuses a non-positive horizon rather than expiring the user now", func() {
		s := &server{store: newMemStore()}
		s.store.CreateUser("Bob", []string{"player"}, 1)
		before := s.store.Users["bob"].ExpiresAt

		req := httptest.NewRequest("POST", "/api/admin/users/bob/extend",
			strings.NewReader(`{"hours":-5}`))
		rec := httptest.NewRecorder()
		s.handleUserItem(rec, req, auth.Claims{Sub: "admin", Roles: []string{"admin"}})

		Expect(rec.Code).To(Equal(400))
		Expect(s.store.Users["bob"].ExpiresAt).To(Equal(before))
	})
})
