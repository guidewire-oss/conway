package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"conway/server/oidc"
)

// SSO (OpenID Connect) login. The browser starts at /api/oidc/start, is sent to
// the org IdP, and returns to /api/oidc/callback with an authorization code. The
// server verifies the ID token, maps a groups claim to Conway roles, provisions
// the account just-in-time, and mints the same HMAC session token the password
// path issues — so everything downstream (withAuth, role gates) is unchanged.
//
// Only staff roles (admin/facilitator/manager) come from SSO. Teams still join a
// game by code. The built-in admin password account is retained as break-glass.

// oidcFlow is one in-flight login, held server-side (the callback carries no
// bearer). Keyed by the opaque state; short-lived.
type oidcFlow struct {
	nonce    string
	verifier string
	expires  time.Time
}

const oidcFlowTTL = 10 * time.Minute

// oidcStateCookie binds a login flow to the browser that started it. Its value
// must equal the callback's state param, defeating login-CSRF / session-swap
// (an attacker's callback URL replayed in a victim's browser carries no matching
// cookie). HttpOnly + SameSite=Lax: JS never needs it, and Lax still sends it on
// the top-level GET redirect back from the IdP.
const oidcStateCookie = "conway_oidc_state"

// buildOIDC constructs an OIDC provider from CONWAY_OIDC_* env, or returns nil
// (with a log line) when SSO is not configured or discovery fails — the server
// then runs with password login only rather than refusing to boot.
func buildOIDC(ctx context.Context, publicURL string) *oidc.Provider {
	issuer := os.Getenv("CONWAY_OIDC_ISSUER")
	clientID := os.Getenv("CONWAY_OIDC_CLIENT_ID")
	if issuer == "" || clientID == "" {
		return nil
	}
	cfg := oidc.Config{
		Issuer:       strings.TrimRight(issuer, "/"),
		ClientID:     clientID,
		ClientSecret: os.Getenv("CONWAY_OIDC_CLIENT_SECRET"),
		RedirectURI:  strings.TrimRight(publicURL, "/") + "/api/oidc/callback",
		GroupsClaim:  os.Getenv("CONWAY_OIDC_GROUPS_CLAIM"),
		RoleMap:      oidc.ParseRoleMap(os.Getenv("CONWAY_OIDC_ROLE_MAP")),
	}
	if s := os.Getenv("CONWAY_OIDC_SCOPES"); s != "" {
		cfg.Scopes = strings.Fields(s)
	}
	if len(cfg.RoleMap) == 0 {
		log.Printf("warning: CONWAY_OIDC_ROLE_MAP is empty — no SSO user can be granted a role; SSO disabled")
		return nil
	}
	// Bound discovery so an unreachable IdP can't hang server startup (go-oidc
	// uses http.DefaultClient, which has no timeout). Later JWKS fetches run on
	// the request context, not this one.
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	p, err := oidc.NewProvider(dctx, cfg)
	if err != nil {
		log.Printf("warning: OIDC discovery failed (%v) — SSO disabled, password login still works", err)
		return nil
	}
	authMode := "public client, PKCE only"
	if cfg.ClientSecret != "" {
		authMode = "confidential client, PKCE + secret"
	}
	log.Printf("OIDC SSO configured (issuer %s, redirect %s, %s)", cfg.Issuer, cfg.RedirectURI, authMode)
	return p
}

// oidcEnabled reports whether SSO is available (for /api/config and the UI).
func (s *server) oidcEnabled() bool { return s.oidc != nil }

// handleOIDCStart begins a login: mint state/nonce/PKCE, stash the flow, and
// return the provider authorize URL for the browser to navigate to.
func (s *server) handleOIDCStart(w http.ResponseWriter, _ *http.Request) {
	if s.oidc == nil {
		http.Error(w, "SSO is not configured", http.StatusServiceUnavailable)
		return
	}
	state, err := oidc.NewState()
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	nonce, err := oidc.NewState()
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	verifier := oidc.NewVerifier()
	s.putFlow(state, &oidcFlow{nonce: nonce, verifier: verifier, expires: time.Now().Add(oidcFlowTTL)})
	s.setStateCookie(w, state)
	writeJSON(w, map[string]string{"url": s.oidc.AuthURL(state, nonce, verifier)})
}

// handleOIDCCallback is the IdP redirect target (no bearer). It validates state,
// exchanges the code, verifies the ID token, maps roles, JIT-provisions the
// account, and bounces back into the SPA with a freshly minted session token in
// the URL fragment (stripped client-side). A user with no recognized role is
// denied — no account is created.
func (s *server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		http.Error(w, "SSO is not configured", http.StatusServiceUnavailable)
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		redirectSSOError(w, r, e)
		return
	}
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	// Login-CSRF defense: the callback must come from the same browser that
	// started the flow, proven by the binding cookie matching the state param.
	// Without this, an attacker's callback URL replayed in a victim's browser
	// would log the victim into the attacker's account. Clear the cookie either
	// way — it is single-use.
	bound := s.stateCookieMatches(r, state)
	s.clearStateCookie(w)
	if !bound {
		redirectSSOError(w, r, "invalid_request")
		return
	}
	flow := s.takeFlow(state)
	if code == "" || flow == nil {
		redirectSSOError(w, r, "invalid_request")
		return
	}
	ctx := r.Context()
	rawID, err := s.oidc.Exchange(ctx, code, flow.verifier)
	if err != nil {
		log.Printf("oidc: code exchange failed: %v", err)
		redirectSSOError(w, r, "exchange_failed")
		return
	}
	id, err := s.oidc.VerifyIDToken(ctx, rawID, flow.nonce)
	if err != nil {
		log.Printf("oidc: id token verification failed: %v", err)
		redirectSSOError(w, r, "invalid_token")
		return
	}
	roles := s.oidc.Roles(id)
	if len(roles) == 0 {
		// Authenticated, but no group maps to a Conway role: deny, and log who
		// tried and what groups they presented so an admin can fix the mapping.
		log.Printf("oidc: access denied for %s (sub %s) — no recognized role from groups %v", id.Email, id.Subject, id.Groups)
		redirectSSOError(w, r, "no_role")
		return
	}
	username := ssoUsername(id)
	s.mu.Lock()
	u := s.store.UpsertSSO(username, ssoDisplay(id), roles)
	tok := s.store.Token(u)
	err = s.store.Save()
	s.mu.Unlock()
	if err != nil {
		log.Printf("oidc: could not persist account %s: %v", username, err)
		redirectSSOError(w, r, "server_error")
		return
	}
	log.Printf("oidc: signed in %s as %v", username, roles)
	// Deliver the token in the fragment so it never reaches the server logs or
	// Referer headers; auth.js reads it, stores it, and strips it.
	http.Redirect(w, r, "/#sso="+url.QueryEscape(tok), http.StatusFound)
}

// ssoUsername derives the JIT account key: the email claim (needs the "email"
// scope; human-readable in the admin panel) when present, else the immutable
// "sso:<sub>". See docs/sso-oidc.md "How the JIT account username is derived".
//
// Known limitation (accepted for EA): keying on the mutable email means an IdP
// email change orphans the old account and its data. Keying on the immutable
// sub is the post-EA fix.
func ssoUsername(id *oidc.Identity) string {
	if e := strings.TrimSpace(strings.ToLower(id.Email)); e != "" {
		return e
	}
	return "sso:" + id.Subject
}

func ssoDisplay(id *oidc.Identity) string {
	if n := strings.TrimSpace(id.Name); n != "" {
		return n
	}
	return ssoUsername(id)
}

func redirectSSOError(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, "/?sso_error="+url.QueryEscape(reason), http.StatusFound)
}

// --- state-binding cookie (login-CSRF defense) ---------------------------

// oidcCookieSecure marks the binding cookie Secure when Conway is served over
// HTTPS (inferred from the configured redirect URI), so it isn't sent in the
// clear. Left off for plain-HTTP local dev so the flow still works there.
func (s *server) oidcCookieSecure() bool {
	return s.oidc != nil && strings.HasPrefix(s.oidc.Config().RedirectURI, "https://")
}

func (s *server) stateCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     oidcStateCookie,
		Value:    value,
		Path:     "/api/oidc",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.oidcCookieSecure(),
		SameSite: http.SameSiteLaxMode,
	}
}

func (s *server) setStateCookie(w http.ResponseWriter, state string) {
	http.SetCookie(w, s.stateCookie(state, int(oidcFlowTTL/time.Second)))
}

// clearStateCookie expires the binding cookie (single-use, whatever the outcome).
func (s *server) clearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, s.stateCookie("", -1))
}

// stateCookieMatches reports whether the request carries a binding cookie equal
// to the callback's state param — the same browser that started the flow.
func (s *server) stateCookieMatches(r *http.Request, state string) bool {
	c, err := r.Cookie(oidcStateCookie)
	return err == nil && c.Value != "" && state != "" && c.Value == state
}

// --- flow store (state -> pending login), with lazy expiry ----------------

func (s *server) putFlow(state string, f *oidcFlow) {
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	if s.oidcFlows == nil {
		s.oidcFlows = map[string]*oidcFlow{}
	}
	// opportunistically evict expired flows so the map can't grow unbounded
	now := time.Now()
	for k, v := range s.oidcFlows {
		if now.After(v.expires) {
			delete(s.oidcFlows, k)
		}
	}
	s.oidcFlows[state] = f
}

// takeFlow returns and removes the flow for state (single-use), or nil if absent
// or expired.
func (s *server) takeFlow(state string) *oidcFlow {
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	f := s.oidcFlows[state]
	delete(s.oidcFlows, state)
	if f == nil || time.Now().After(f.expires) {
		return nil
	}
	return f
}
