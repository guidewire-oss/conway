// Package oidc adapts an OpenID Connect provider to Conway's login: it wraps
// coreos/go-oidc (discovery, JWKS, ID-token verification) and golang.org/x/oauth2
// (authorization-code exchange with PKCE), and adds the only Conway-specific
// piece — mapping an IdP groups claim to Conway roles (see roles.go).
//
// The verification-critical work (RS256/JWKS signature checks, iss/aud/exp,
// discovery issuer validation, PKCE) is delegated to those vetted libraries
// rather than hand-rolled. Login is delegated to an org IdP (Okta, Google,
// Entra ID, Auth0); the callback handler (server/oidchandlers.go) still owns the
// state/nonce binding and JIT provisioning.
package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Config is the server-side OIDC configuration, built from CONWAY_OIDC_* env.
type Config struct {
	Issuer   string // e.g. https://acme.okta.com  (or .../oauth2/default)
	ClientID string
	// ClientSecret is OPTIONAL. The default and recommended setup is a public
	// client authenticated by PKCE alone (no secret to store or rotate) — the
	// current guidance for browser-driven web apps. Set a secret only when your
	// provider or policy mandates a confidential client; PKCE is sent either way.
	ClientSecret string
	RedirectURI  string   // <public-url>/api/oidc/callback
	Scopes       []string // defaults to openid profile email groups
	GroupsClaim  string   // claim holding group membership; default "groups"
	RoleMap      RoleMap  // external group -> Conway role
}

// Enabled reports whether OIDC is configured enough to attempt a login.
func (c *Config) Enabled() bool {
	return c != nil && c.Issuer != "" && c.ClientID != "" && c.RedirectURI != ""
}

// scopes returns the requested OAuth scopes: the operator's CONWAY_OIDC_SCOPES
// list, or a sensible default. "openid" is mandatory for OIDC (no id_token
// without it), so it is always forced in and placed first even when a custom
// list omits it; the result is de-duped. The default requests "groups" as a
// scope — some IdPs release the groups claim only behind a scope, others via a
// claim mapping and reject an unknown scope; override the list per your IdP.
func (c *Config) scopes() []string {
	raw := c.Scopes
	if len(raw) == 0 {
		raw = []string{gooidc.ScopeOpenID, "profile", "email", "groups"}
	}
	out := []string{gooidc.ScopeOpenID}
	seen := map[string]bool{gooidc.ScopeOpenID: true}
	for _, s := range raw {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func (c *Config) groupsClaim() string {
	if c.GroupsClaim != "" {
		return c.GroupsClaim
	}
	return "groups"
}

// Provider is a Config resolved against its discovery document, with an OAuth2
// client and an ID-token verifier. Safe for concurrent use.
type Provider struct {
	cfg      Config
	oauth    oauth2.Config
	verifier *gooidc.IDTokenVerifier
}

// NewProvider runs OIDC discovery for cfg.Issuer (which also validates that the
// document's issuer matches) and returns a ready Provider. Network/validation
// failures are surfaced so the server can log and run with OIDC disabled rather
// than crash.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	p, err := gooidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	oauthCfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret, // empty => public client (PKCE only)
		RedirectURL:  cfg.RedirectURI,
		Endpoint:     p.Endpoint(),
		Scopes:       cfg.scopes(),
	}
	return &Provider{
		cfg:      cfg,
		oauth:    oauthCfg,
		verifier: p.Verifier(&gooidc.Config{ClientID: cfg.ClientID}),
	}, nil
}

// Config returns the provider's configuration (read-only use by handlers).
func (p *Provider) Config() Config { return p.cfg }

// --- login URL + PKCE ----------------------------------------------------

// NewState returns a random opaque value for the CSRF state and the nonce.
func NewState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewVerifier returns a fresh PKCE code verifier (RFC 7636).
func NewVerifier() string { return oauth2.GenerateVerifier() }

// AuthURL builds the provider authorize URL for the browser to navigate to,
// carrying the CSRF state, the nonce, and the S256 PKCE challenge derived from
// verifier.
func (p *Provider) AuthURL(state, nonce, verifier string) string {
	return p.oauth.AuthCodeURL(state, gooidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
}

// --- code exchange -------------------------------------------------------

// Exchange trades an authorization code (with its PKCE verifier) for tokens and
// returns the raw ID token.
func (p *Provider) Exchange(ctx context.Context, code, verifier string) (string, error) {
	tok, err := p.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return "", err
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok || raw == "" {
		return "", errors.New("token response contained no id_token")
	}
	return raw, nil
}

// --- ID token verification + claims -------------------------------------

// Identity is the validated result of a login: who the user is and the groups
// the IdP asserted for them.
type Identity struct {
	Subject string
	Email   string
	Name    string
	Groups  []string
}

// VerifyIDToken verifies the ID token (signature via JWKS, iss/aud/exp — all in
// go-oidc), checks the nonce, and extracts identity plus the configured groups
// claim.
func (p *Provider) VerifyIDToken(ctx context.Context, rawIDToken, wantNonce string) (*Identity, error) {
	tok, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, err
	}
	if wantNonce != "" && tok.Nonce != wantNonce {
		return nil, errors.New("id token nonce mismatch")
	}
	var std struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := tok.Claims(&std); err != nil {
		return nil, fmt.Errorf("parse id token claims: %w", err)
	}
	var all map[string]json.RawMessage
	if err := tok.Claims(&all); err != nil {
		return nil, fmt.Errorf("parse id token claims: %w", err)
	}
	return &Identity{
		Subject: tok.Subject,
		Email:   std.Email,
		Name:    std.Name,
		Groups:  extractGroups(all, p.cfg.groupsClaim()),
	}, nil
}

// Roles maps an identity's groups to Conway roles via the configured RoleMap.
func (p *Provider) Roles(id *Identity) []string { return p.cfg.RoleMap.Map(id.Groups) }

// extractGroups pulls a string-array claim by name from the decoded claims. A
// missing claim yields nil (not an error) — the caller decides whether an empty
// role set means "deny". A bare-string claim (some IdPs emit one group as a
// string) is accepted too.
func extractGroups(all map[string]json.RawMessage, name string) []string {
	raw, ok := all[name]
	if !ok {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil && one != "" {
		return []string{one}
	}
	return nil
}
