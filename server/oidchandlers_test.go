package main

import (
	"testing"
	"time"

	"conway/oidc"
)

func TestFlowStoreSingleUse(t *testing.T) {
	s := &server{oidcFlows: map[string]*oidcFlow{}}
	s.putFlow("state-1", &oidcFlow{nonce: "n", verifier: "v", expires: time.Now().Add(time.Minute)})
	if f := s.takeFlow("state-1"); f == nil || f.nonce != "n" {
		t.Fatal("first take should return the flow")
	}
	if f := s.takeFlow("state-1"); f != nil {
		t.Fatal("second take must be nil (single-use)")
	}
	if f := s.takeFlow("never-existed"); f != nil {
		t.Fatal("unknown state must be nil")
	}
}

func TestFlowStoreExpiry(t *testing.T) {
	s := &server{oidcFlows: map[string]*oidcFlow{}}
	s.putFlow("old", &oidcFlow{expires: time.Now().Add(-time.Second)})
	if f := s.takeFlow("old"); f != nil {
		t.Fatal("expired flow must not be returned")
	}
}

func TestSSOUsernameAndDisplay(t *testing.T) {
	// email preferred, lower-cased
	id := &oidc.Identity{Subject: "00u1", Email: "Dana@Acme.com", Name: "Dana Ops"}
	if got := ssoUsername(id); got != "dana@acme.com" {
		t.Fatalf("username: got %q", got)
	}
	if got := ssoDisplay(id); got != "Dana Ops" {
		t.Fatalf("display: got %q", got)
	}
	// no email -> subject-based key; no name -> falls back to username
	id2 := &oidc.Identity{Subject: "00u2"}
	if got := ssoUsername(id2); got != "sso:00u2" {
		t.Fatalf("username fallback: got %q", got)
	}
	if got := ssoDisplay(id2); got != "sso:00u2" {
		t.Fatalf("display fallback: got %q", got)
	}
}

// A user whose groups map to no Conway role must be denied and never provisioned.
func TestSSODenyProducesNoRoles(t *testing.T) {
	m := oidc.ParseRoleMap("conway-admins=admin")
	if roles := m.Map([]string{"everyone"}); len(roles) != 0 {
		t.Fatalf("no recognized group must yield no roles, got %v", roles)
	}
}
