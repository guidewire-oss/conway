package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"conway/server/auth"
	"conway/server/oidc"
)

// mockIdP is a minimal OIDC provider (discovery + JWKS + token) that signs ID
// tokens with a generated RSA key — enough to drive Conway's real OIDC handlers
// end to end without a browser or a live IdP.
type mockIdP struct {
	srv     *httptest.Server
	key     *rsa.PrivateKey // key advertised in JWKS
	signKey *rsa.PrivateKey // key the /token endpoint signs with (=key, unless forging)
	kid     string
	issuer  string
	// what the next /token call should embed in the id_token:
	nextClaims func() map[string]any
}

func newMockIDP() *mockIdP {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		Fail(err.Error())
	}
	m := &mockIdP{key: key, signKey: key, kid: "mock-kid"}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 m.issuer,
			"authorization_endpoint": m.issuer + "/authorize",
			"token_endpoint":         m.issuer + "/token",
			"jwks_uri":               m.issuer + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		// minimal big-endian exponent bytes
		full := make([]byte, 8)
		binary.BigEndian.PutUint64(full, uint64(key.PublicKey.E))
		exp := full
		for len(exp) > 1 && exp[0] == 0 {
			exp = exp[1:]
		}
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": m.kid, "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(exp),
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("code") == "" {
			http.Error(w, "bad token request", 400)
			return
		}
		idTok := m.sign(m.nextClaims())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id_token": idTok, "token_type": "Bearer", "access_token": "at"})
	})
	m.srv = httptest.NewServer(mux)
	m.issuer = m.srv.URL
	return m
}

func (m *mockIdP) close() { m.srv.Close() }

func (m *mockIdP) sign(claims map[string]any) string {
	enc := func(v any) string { b, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(b) }
	hdr := enc(map[string]any{"alg": "RS256", "typ": "JWT", "kid": m.kid})
	pay := enc(claims)
	sum := sha256.Sum256([]byte(hdr + "." + pay))
	sig, err := rsa.SignPKCS1v15(rand.Reader, m.signKey, crypto.SHA256, sum[:])
	if err != nil {
		Fail(err.Error())
	}
	return hdr + "." + pay + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// newOIDCServer wires a server with a real OIDC provider pointed at the mock IdP
// and a file-backed store (no Postgres needed).
func newOIDCServer(m *mockIdP, roleMap string) *server {
	cfg := oidc.Config{
		Issuer:      m.issuer,
		ClientID:    "conway-client",
		RedirectURI: "http://conway.test/api/oidc/callback",
		RoleMap:     oidc.ParseRoleMap(roleMap),
	}
	p, err := oidc.NewProvider(context.Background(), cfg)
	if err != nil {
		Fail(fmt.Sprintf("provider: %v", err))
	}
	st := auth.NewStore([]byte("test-secret"))
	st.Path = GinkgoT().TempDir() + "/store.json"
	return &server{store: st, oidc: p, oidcFlows: map[string]*oidcFlow{}}
}

// startFlow calls /api/oidc/start and returns the state plus the browser-binding
// cookie the server set (nil if none) — the callback must present that cookie.
func startFlow(s *server) (string, *http.Cookie) {
	rec := httptest.NewRecorder()
	s.handleOIDCStart(rec, httptest.NewRequest("GET", "/api/oidc/start", nil))
	if rec.Code != 200 {
		Fail(fmt.Sprintf("start status %d", rec.Code))
	}
	var body struct{ URL string }
	json.Unmarshal(rec.Body.Bytes(), &body)
	u, _ := url.Parse(body.URL)
	state := u.Query().Get("state")
	if state == "" || u.Query().Get("code_challenge_method") != "S256" {
		Fail(fmt.Sprintf("authorize url missing state/PKCE: %s", body.URL))
	}
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == oidcStateCookie {
			cookie = c
		}
	}
	return state, cookie
}

// doCallback drives /api/oidc/callback with the given state and (optional)
// binding cookie, returning the recorder.
func doCallback(s *server, state string, cookie *http.Cookie) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/oidc/callback?code=xyz&state="+state, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	s.handleOIDCCallback(rec, req)
	return rec
}

var _ = Describe("OIDCLoginEndToEnd", func() {
	It("behaves", func() {
		m := newMockIDP()
		defer m.close()
		s := newOIDCServer(m, "conway-facilitators=facilitator,conway-admins=admin")

		state, cookie := startFlow(s)
		// The IdP echoes the flow's nonce back in the id_token.
		nonce := s.oidcFlows[state].nonce
		m.nextClaims = func() map[string]any {
			return map[string]any{
				"iss": m.issuer, "sub": "00u-abc", "aud": "conway-client",
				"exp": time.Now().Add(time.Hour).Unix(), "nonce": nonce,
				"email": "Dana@Acme.com", "name": "Dana Ops",
				"groups": []string{"conway-facilitators", "everyone"},
			}
		}

		rec := doCallback(s, state, cookie)

		if rec.Code != http.StatusFound {
			Fail(fmt.Sprintf("callback status %d, body %s", rec.Code, rec.Body.String()))
		}
		loc := rec.Header().Get("Location")
		if !strings.HasPrefix(loc, "/#sso=") {
			Fail(fmt.Sprintf("expected token in fragment, got redirect %q", loc))
		}
		rawTok, _ := url.QueryUnescape(strings.TrimPrefix(loc, "/#sso="))
		claims, err := auth.ParseToken(s.store.Secret, rawTok, time.Now().Unix())
		if err != nil {
			Fail(fmt.Sprintf("minted token should parse: %v", err))
		}
		if claims.Sub != "dana@acme.com" || !claims.Has("facilitator") || claims.Has("admin") {
			Fail(fmt.Sprintf("token claims mismatch: %+v", claims))
		}
		// JIT account exists, passwordless, correct roles.
		u := s.store.Users["dana@acme.com"]
		if u == nil || !u.SSO || !u.Has("facilitator") || u.Hash != "" {
			Fail(fmt.Sprintf("JIT account wrong: %+v", u))
		}
		// The login flow is single-use: replaying the same state (even with the same
		// binding cookie) fails because the flow was consumed.
		rec2 := doCallback(s, state, cookie)
		if got := rec2.Header().Get("Location"); !strings.Contains(got, "sso_error") {
			Fail(fmt.Sprintf("state replay should fail, got %q", got))
		}
	})
})

var _ = Describe("OIDCLoginDeniedNoRole", func() {
	It("behaves", func() {
		m := newMockIDP()
		defer m.close()
		s := newOIDCServer(m, "conway-admins=admin")

		state, cookie := startFlow(s)
		nonce := s.oidcFlows[state].nonce
		m.nextClaims = func() map[string]any {
			return map[string]any{
				"iss": m.issuer, "sub": "00u-nobody", "aud": "conway-client",
				"exp": time.Now().Add(time.Hour).Unix(), "nonce": nonce,
				"email": "nobody@acme.com", "groups": []string{"everyone"},
			}
		}
		rec := doCallback(s, state, cookie)

		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, "sso_error=no_role") {
			Fail(fmt.Sprintf("expected no_role denial, got %q (body %s)", loc, rec.Body.String()))
		}
		if len(s.store.Users) != 0 {
			Fail(fmt.Sprintf("denied login must not provision an account, got %d users", len(s.store.Users)))
		}
	})
})

var _ = Describe("OIDCCallbackRejectsForgedToken", func() {
	It("behaves", func() {
		m := newMockIDP()
		defer m.close()
		s := newOIDCServer(m, "conway-admins=admin")
		state, cookie := startFlow(s)
		// Sign the id_token with a foreign key while JWKS still advertises the real
		// one — RS256 verification against the published key must fail.
		forger, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			Fail(err.Error())
		}
		m.signKey = forger
		nonce := s.oidcFlows[state].nonce
		m.nextClaims = func() map[string]any {
			return map[string]any{
				"iss": m.issuer, "sub": "x", "aud": "conway-client",
				"exp": time.Now().Add(time.Hour).Unix(), "nonce": nonce,
				"email": "attacker@acme.com", "groups": []string{"conway-admins"},
			}
		}
		rec := doCallback(s, state, cookie)
		if !strings.Contains(rec.Header().Get("Location"), "sso_error") {
			Fail(fmt.Sprintf("forged-token login must be rejected, got %q", rec.Header().Get("Location")))
		}
		if len(s.store.Users) != 0 {
			Fail("forged login must not provision an account")
		}
	})
})

// Login-CSRF / session-swap: a callback carrying a valid state but NOT the
// browser-binding cookie (the attacker's callback URL replayed in a victim's
// browser) must be rejected and provision nothing — even though the code would
// otherwise exchange and verify cleanly.
var _ = Describe("OIDCCallbackRejectsUnboundState", func() {
	It("behaves", func() {
		m := newMockIDP()
		defer m.close()
		s := newOIDCServer(m, "conway-admins=admin")
		state, _ := startFlow(s)
		nonce := s.oidcFlows[state].nonce
		m.nextClaims = func() map[string]any {
			return map[string]any{
				"iss": m.issuer, "sub": "00u-attacker", "aud": "conway-client",
				"exp": time.Now().Add(time.Hour).Unix(), "nonce": nonce,
				"email": "attacker@acme.com", "groups": []string{"conway-admins"},
			}
		}
		// No binding cookie — mimics a victim's browser opening the attacker's URL.
		rec := doCallback(s, state, nil)
		if !strings.Contains(rec.Header().Get("Location"), "sso_error") {
			Fail(fmt.Sprintf("unbound state must be rejected, got %q", rec.Header().Get("Location")))
		}
		if len(s.store.Users) != 0 {
			Fail(fmt.Sprintf("login-CSRF attempt must not provision an account, got %d", len(s.store.Users)))
		}
		// A wrong cookie value must also fail.
		rec2 := doCallback(s, state, &http.Cookie{Name: oidcStateCookie, Value: "not-the-state"})
		if !strings.Contains(rec2.Header().Get("Location"), "sso_error") {
			Fail(fmt.Sprintf("mismatched binding cookie must be rejected, got %q", rec2.Header().Get("Location")))
		}
	})
})
