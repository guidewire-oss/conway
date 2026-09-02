package oidc

import (
	"encoding/json"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	"strings"
)

// The OIDC/JWKS/JWT verification and PKCE exchange are exercised end to end
// against a mock IdP in package main (server/oidc_e2e_test.go); the underlying
// crypto is coreos/go-oidc + golang.org/x/oauth2. These tests cover the one
// Conway-specific piece: group-claim extraction and group -> role mapping.

// --- group claim extraction ----------------------------------------------

func rawClaims(m map[string]any) map[string]json.RawMessage {
	b, err := json.Marshal(m)
	if err != nil {
		Fail(err.Error())
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(b, &out); err != nil {
		Fail(err.Error())
	}
	return out
}

var _ = Describe("ExtractGroups", func() {
	It("behaves", func() {
		// array form
		if g := extractGroups(rawClaims(map[string]any{"groups": []string{"a", "b"}}), "groups"); strings.Join(g, ",") != "a,b" {
			Fail(fmt.Sprintf("array claim: got %v", g))
		}
		// bare-string form (some IdPs emit a single group as a string)
		if g := extractGroups(rawClaims(map[string]any{"groups": "solo"}), "groups"); len(g) != 1 || g[0] != "solo" {
			Fail(fmt.Sprintf("string claim: got %v", g))
		}
		// configurable claim name
		if g := extractGroups(rawClaims(map[string]any{"roles": []string{"x"}}), "roles"); len(g) != 1 || g[0] != "x" {
			Fail(fmt.Sprintf("custom claim: got %v", g))
		}
		// missing claim -> nil, not an error
		if g := extractGroups(rawClaims(map[string]any{"email": "a@b.com"}), "groups"); g != nil {
			Fail(fmt.Sprintf("missing claim should be nil, got %v", g))
		}
	})
})

// --- scopes --------------------------------------------------------------

var _ = Describe("ScopesDefault", func() {
	It("behaves", func() {
		if got := (&Config{}).scopes(); strings.Join(got, " ") != "openid profile email groups" {
			Fail(fmt.Sprintf("default scopes: got %v", got))
		}
	})
})

var _ = Describe("ScopesAlwaysIncludeOpenIDDeduped", func() {
	It("behaves", func() {
		// custom list omitting openid -> openid forced in, first
		got := (&Config{Scopes: []string{"profile", "email", "groups"}}).scopes()
		if got[0] != "openid" {
			Fail(fmt.Sprintf("openid must be forced in first: got %v", got))
		}
		if strings.Join(got, " ") != "openid profile email groups" {
			Fail(fmt.Sprintf("custom scopes: got %v", got))
		}
		// duplicates and blanks are dropped
		if got := (&Config{Scopes: []string{"openid", "openid", "", "email"}}).scopes(); strings.Join(got, " ") != "openid email" {
			Fail(fmt.Sprintf("dedup/blank handling: got %v", got))
		}
	})
})

// --- role mapping --------------------------------------------------------

var _ = Describe("ParseRoleMap", func() {
	It("behaves", func() {
		m := ParseRoleMap("conway-admins=admin, conway-facils=facilitator ,bad=nope,,malformed")
		if m["conway-admins"] != "admin" || m["conway-facils"] != "facilitator" {
			Fail(fmt.Sprintf("valid pairs not parsed: %v", m))
		}
		if _, ok := m["bad"]; ok {
			Fail("unknown role should be dropped")
		}
		if len(m) != 2 {
			Fail(fmt.Sprintf("expected 2 entries, got %d: %v", len(m), m))
		}
	})
})

var _ = Describe("RoleMapMapDedupAndPrecedence", func() {
	It("behaves", func() {
		m := ParseRoleMap("g-admin=admin,g-fac=facilitator,g-fac2=facilitator,g-mgr=manager")
		roles := m.Map([]string{"g-mgr", "g-fac", "g-admin", "g-fac2", "unmapped"})
		// deterministic order admin, facilitator, manager; dedup facilitator
		want := []string{"admin", "facilitator", "manager"}
		if strings.Join(roles, ",") != strings.Join(want, ",") {
			Fail(fmt.Sprintf("got %v want %v", roles, want))
		}
	})
})

var _ = Describe("RoleMapIsCaseInsensitive", func() {
	It("behaves", func() {
		// Map key casing and IdP group casing need not match: an Okta group
		// "Test-Conway-Admins" must satisfy a "test-conway-admins" map entry.
		m := ParseRoleMap("test-conway-admins=admin,Conway-Facilitators=facilitator")
		roles := m.Map([]string{"Everyone", "Test-Conway-Admins"})
		if len(roles) != 1 || roles[0] != "admin" {
			Fail(fmt.Sprintf("case-insensitive group match failed: got %v", roles))
		}
		// map-key casing is likewise normalized
		if r := m.Map([]string{"conway-facilitators"}); len(r) != 1 || r[0] != "facilitator" {
			Fail(fmt.Sprintf("case-insensitive map key failed: got %v", r))
		}
	})
})

var _ = Describe("RoleMapEmptyWhenNoMatch", func() {
	It("behaves", func() {
		m := ParseRoleMap("g-admin=admin")
		if roles := m.Map([]string{"everyone", "engineering"}); len(roles) != 0 {
			Fail(fmt.Sprintf("unrecognized groups must map to no roles, got %v", roles))
		}
	})
})
