package main

import (
	"time"

	"conway/server/oidc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("the OIDC flow store", func() {
	It("hands each flow out exactly once", func() {
		s := &server{oidcFlows: map[string]*oidcFlow{}}
		s.putFlow("state-1", &oidcFlow{nonce: "n", verifier: "v", expires: time.Now().Add(time.Minute)})
		f := s.takeFlow("state-1")
		Expect(f).NotTo(BeNil())
		Expect(f.nonce).To(Equal("n"))
		Expect(s.takeFlow("state-1")).To(BeNil(), "second take must be nil (single-use)")
		Expect(s.takeFlow("never-existed")).To(BeNil(), "unknown state must be nil")
	})

	It("refuses expired flows", func() {
		s := &server{oidcFlows: map[string]*oidcFlow{}}
		s.putFlow("old", &oidcFlow{expires: time.Now().Add(-time.Second)})
		Expect(s.takeFlow("old")).To(BeNil(), "expired flow must not be returned")
	})
})

var _ = Describe("SSO identity mapping", func() {
	It("prefers the lower-cased email for username, the name for display", func() {
		id := &oidc.Identity{Subject: "00u1", Email: "Dana@Acme.com", Name: "Dana Ops"}
		Expect(ssoUsername(id)).To(Equal("dana@acme.com"))
		Expect(ssoDisplay(id)).To(Equal("Dana Ops"))
	})

	It("falls back to a subject-based key when email or name are missing", func() {
		id2 := &oidc.Identity{Subject: "00u2"}
		Expect(ssoUsername(id2)).To(Equal("sso:00u2"))
		Expect(ssoDisplay(id2)).To(Equal("sso:00u2"))
	})

	// A user whose groups map to no Conway role must be denied and never provisioned.
	It("maps unrecognized groups to no roles", func() {
		m := oidc.ParseRoleMap("conway-admins=admin")
		Expect(m.Map([]string{"everyone"})).To(BeEmpty(),
			"no recognized group must yield no roles")
	})
})
