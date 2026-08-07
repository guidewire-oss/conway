package oidc

import (
	"encoding/json"
	"strings"
	"testing"
)

// The OIDC/JWKS/JWT verification and PKCE exchange are exercised end to end
// against a mock IdP in package main (server/oidc_e2e_test.go); the underlying
// crypto is coreos/go-oidc + golang.org/x/oauth2. These tests cover the one
// Conway-specific piece: group-claim extraction and group -> role mapping.

// --- group claim extraction ----------------------------------------------

func rawClaims(t *testing.T, m map[string]any) map[string]json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestExtractGroups(t *testing.T) {
	// array form
	if g := extractGroups(rawClaims(t, map[string]any{"groups": []string{"a", "b"}}), "groups"); strings.Join(g, ",") != "a,b" {
		t.Fatalf("array claim: got %v", g)
	}
	// bare-string form (some IdPs emit a single group as a string)
	if g := extractGroups(rawClaims(t, map[string]any{"groups": "solo"}), "groups"); len(g) != 1 || g[0] != "solo" {
		t.Fatalf("string claim: got %v", g)
	}
	// configurable claim name
	if g := extractGroups(rawClaims(t, map[string]any{"roles": []string{"x"}}), "roles"); len(g) != 1 || g[0] != "x" {
		t.Fatalf("custom claim: got %v", g)
	}
	// missing claim -> nil, not an error
	if g := extractGroups(rawClaims(t, map[string]any{"email": "a@b.com"}), "groups"); g != nil {
		t.Fatalf("missing claim should be nil, got %v", g)
	}
}

// --- role mapping --------------------------------------------------------

func TestParseRoleMap(t *testing.T) {
	m := ParseRoleMap("conway-admins=admin, conway-facils=facilitator ,bad=nope,,malformed")
	if m["conway-admins"] != "admin" || m["conway-facils"] != "facilitator" {
		t.Fatalf("valid pairs not parsed: %v", m)
	}
	if _, ok := m["bad"]; ok {
		t.Fatal("unknown role should be dropped")
	}
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(m), m)
	}
}

func TestRoleMapMapDedupAndPrecedence(t *testing.T) {
	m := ParseRoleMap("g-admin=admin,g-fac=facilitator,g-fac2=facilitator,g-mgr=manager")
	roles := m.Map([]string{"g-mgr", "g-fac", "g-admin", "g-fac2", "unmapped"})
	// deterministic order admin, facilitator, manager; dedup facilitator
	want := []string{"admin", "facilitator", "manager"}
	if strings.Join(roles, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", roles, want)
	}
}

func TestRoleMapIsCaseInsensitive(t *testing.T) {
	// Map key casing and IdP group casing need not match: an Okta group
	// "Test-Conway-Admins" must satisfy a "test-conway-admins" map entry.
	m := ParseRoleMap("test-conway-admins=admin,Conway-Facilitators=facilitator")
	roles := m.Map([]string{"Everyone", "Test-Conway-Admins"})
	if len(roles) != 1 || roles[0] != "admin" {
		t.Fatalf("case-insensitive group match failed: got %v", roles)
	}
	// map-key casing is likewise normalized
	if r := m.Map([]string{"conway-facilitators"}); len(r) != 1 || r[0] != "facilitator" {
		t.Fatalf("case-insensitive map key failed: got %v", r)
	}
}

func TestRoleMapEmptyWhenNoMatch(t *testing.T) {
	m := ParseRoleMap("g-admin=admin")
	if roles := m.Map([]string{"everyone", "engineering"}); len(roles) != 0 {
		t.Fatalf("unrecognized groups must map to no roles, got %v", roles)
	}
}
