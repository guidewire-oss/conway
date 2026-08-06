package oidc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// --- test signing harness ------------------------------------------------

type signer struct {
	key *rsa.PrivateKey
	kid string
}

func newSigner(t *testing.T) *signer {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	return &signer{key: k, kid: "test-kid-1"}
}

func b64json(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// sign builds a compact RS256 JWT from the given claims map.
func (s *signer) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	hdr := b64json(t, map[string]any{"alg": "RS256", "typ": "JWT", "kid": s.kid})
	pay := b64json(t, claims)
	signed := sha256.Sum256([]byte(hdr + "." + pay))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, signed[:])
	if err != nil {
		t.Fatal(err)
	}
	return hdr + "." + pay + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// jwks returns a kid->key map as the verifier expects (skips the HTTP fetch).
func (s *signer) keys() map[string]*rsa.PublicKey {
	return map[string]*rsa.PublicKey{s.kid: &s.key.PublicKey}
}

// testProvider wires a Provider with an injected key set (no network).
func testProvider(s *signer, cfg Config) *Provider {
	p := &Provider{cfg: cfg, disc: discovery{Issuer: cfg.Issuer}, now: time.Now, keysTTL: time.Hour}
	p.fetchKey = func(context.Context) (map[string]*rsa.PublicKey, error) { return s.keys(), nil }
	return p
}

func baseClaims(iss, aud, nonce string, exp int64) map[string]any {
	return map[string]any{
		"iss": iss, "sub": "00u123", "aud": aud, "exp": exp, "nonce": nonce,
		"email": "dana@acme.com", "name": "Dana Ops",
		"groups": []string{"conway-facilitators", "everyone"},
	}
}

// --- JWKS / signature ----------------------------------------------------

func TestParseJWKSRoundTrip(t *testing.T) {
	s := newSigner(t)
	// Build a JWKS JSON the way a provider would, then parse it back.
	full := make([]byte, 8)
	binary.BigEndian.PutUint64(full, uint64(s.key.PublicKey.E))
	eb := bytes.TrimLeft(full, "\x00") // minimal big-endian exponent bytes
	doc := map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": s.kid, "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(s.key.PublicKey.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(eb),
	}}}
	raw, _ := json.Marshal(doc)
	keys, err := parseJWKS(raw)
	if err != nil {
		t.Fatalf("parseJWKS: %v", err)
	}
	got := keys[s.kid]
	if got == nil || got.N.Cmp(s.key.PublicKey.N) != 0 || got.E != s.key.PublicKey.E {
		t.Fatalf("reconstructed key mismatch: %+v", got)
	}
}

func TestVerifySignatureTamperFails(t *testing.T) {
	s := newSigner(t)
	tok := s.sign(t, map[string]any{"sub": "x"})
	if _, err := verifySignature(tok, s.keys()); err != nil {
		t.Fatalf("valid token should verify: %v", err)
	}
	if _, err := verifySignature(tok+"x", s.keys()); err == nil {
		t.Fatal("tampered signature must fail")
	}
	other := newSigner(t)
	if _, err := verifySignature(tok, other.keys()); err == nil {
		t.Fatal("wrong key must fail")
	}
}

// --- ID token verification ----------------------------------------------

func TestVerifyIDTokenHappyPath(t *testing.T) {
	s := newSigner(t)
	cfg := Config{Issuer: "https://acme.okta.com", ClientID: "client-abc"}
	p := testProvider(s, cfg)
	tok := s.sign(t, baseClaims(cfg.Issuer, cfg.ClientID, "nonce-1", time.Now().Add(time.Hour).Unix()))
	id, err := p.VerifyIDToken(context.Background(), tok, "nonce-1")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if id.Subject != "00u123" || id.Email != "dana@acme.com" {
		t.Fatalf("identity mismatch: %+v", id)
	}
	if len(id.Groups) != 2 || id.Groups[0] != "conway-facilitators" {
		t.Fatalf("groups mismatch: %v", id.Groups)
	}
}

func TestVerifyIDTokenRejections(t *testing.T) {
	s := newSigner(t)
	cfg := Config{Issuer: "https://acme.okta.com", ClientID: "client-abc"}
	p := testProvider(s, cfg)
	now := time.Now()
	cases := []struct {
		name   string
		claims map[string]any
		nonce  string
	}{
		{"bad issuer", baseClaims("https://evil.com", cfg.ClientID, "n", now.Add(time.Hour).Unix()), "n"},
		{"bad audience", baseClaims(cfg.Issuer, "someone-else", "n", now.Add(time.Hour).Unix()), "n"},
		{"expired", baseClaims(cfg.Issuer, cfg.ClientID, "n", now.Add(-2*time.Hour).Unix()), "n"},
		{"nonce mismatch", baseClaims(cfg.Issuer, cfg.ClientID, "n", now.Add(time.Hour).Unix()), "different"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := s.sign(t, tc.claims)
			if _, err := p.VerifyIDToken(context.Background(), tok, tc.nonce); err == nil {
				t.Fatalf("%s: expected rejection, got nil", tc.name)
			}
		})
	}
}

func TestVerifyIDTokenAudienceArray(t *testing.T) {
	s := newSigner(t)
	cfg := Config{Issuer: "https://acme.okta.com", ClientID: "client-abc"}
	p := testProvider(s, cfg)
	claims := baseClaims(cfg.Issuer, "", "n", time.Now().Add(time.Hour).Unix())
	claims["aud"] = []string{"other", "client-abc"} // array form
	tok := s.sign(t, claims)
	if _, err := p.VerifyIDToken(context.Background(), tok, "n"); err != nil {
		t.Fatalf("array audience containing clientID should pass: %v", err)
	}
}

func TestConfiguredGroupsClaim(t *testing.T) {
	s := newSigner(t)
	cfg := Config{Issuer: "https://acme.okta.com", ClientID: "client-abc", GroupsClaim: "roles"}
	p := testProvider(s, cfg)
	claims := baseClaims(cfg.Issuer, cfg.ClientID, "n", time.Now().Add(time.Hour).Unix())
	delete(claims, "groups")
	claims["roles"] = []string{"conway-admins"}
	tok := s.sign(t, claims)
	id, err := p.VerifyIDToken(context.Background(), tok, "n")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(id.Groups) != 1 || id.Groups[0] != "conway-admins" {
		t.Fatalf("custom claim not read: %v", id.Groups)
	}
}

// --- PKCE + auth URL -----------------------------------------------------

func TestPKCEChallengeIsS256OfVerifier(t *testing.T) {
	pk, err := NewPKCE()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(pk.Verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if pk.Challenge != want {
		t.Fatalf("challenge is not S256(verifier)")
	}
}

func TestAuthURLIncludesPKCEAndNonce(t *testing.T) {
	p := &Provider{cfg: Config{ClientID: "cid", RedirectURI: "https://c/cb"}, disc: discovery{AuthEndpoint: "https://idp/authorize"}}
	u := p.AuthURL("state-1", "nonce-1", "chal-1")
	for _, want := range []string{"response_type=code", "client_id=cid", "state=state-1", "nonce=nonce-1", "code_challenge=chal-1", "code_challenge_method=S256"} {
		if !strings.Contains(u, want) {
			t.Fatalf("auth url missing %q: %s", want, u)
		}
	}
}

// --- code exchange (PKCE vs. secret) ------------------------------------

// exchangeProvider wires a Provider whose token endpoint is a test server that
// records the posted form, so we can assert exactly what went on the wire.
func exchangeProvider(t *testing.T, cfg Config) (*Provider, *url.Values) {
	t.Helper()
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		got = r.Form
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id_token":"header.payload.sig","token_type":"Bearer"}`))
	}))
	t.Cleanup(srv.Close)
	p := &Provider{cfg: cfg, disc: discovery{TokenEndpoint: srv.URL}, hc: srv.Client(), now: time.Now, keysTTL: time.Hour}
	return p, &got
}

func TestExchangePKCEOnlyOmitsSecret(t *testing.T) {
	p, form := exchangeProvider(t, Config{ClientID: "cid"}) // no ClientSecret
	if _, err := p.Exchange(context.Background(), "the-code", "the-verifier"); err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if form.Get("grant_type") != "authorization_code" || form.Get("code") != "the-code" {
		t.Fatalf("unexpected grant: %v", form.Encode())
	}
	if form.Get("code_verifier") != "the-verifier" {
		t.Fatal("PKCE code_verifier must be sent")
	}
	if _, ok := (*form)["client_secret"]; ok {
		t.Fatalf("public client must NOT send client_secret: %v", form.Encode())
	}
}

func TestExchangeWithSecretStillSendsPKCE(t *testing.T) {
	p, form := exchangeProvider(t, Config{ClientID: "cid", ClientSecret: "shh"})
	if _, err := p.Exchange(context.Background(), "c", "v"); err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if form.Get("code_verifier") != "v" {
		t.Fatal("PKCE must be sent even for a confidential client")
	}
	if form.Get("client_secret") != "shh" {
		t.Fatal("confidential client should send client_secret when configured")
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
