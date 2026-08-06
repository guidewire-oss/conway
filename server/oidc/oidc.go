// Package oidc implements an OpenID Connect Authorization Code + PKCE login for
// Conway, using stdlib only (net/http + the RS256 verifier in jwt.go). Login is
// delegated to an org IdP (Okta, Google, Entra ID, Auth0); the server exchanges
// the code, verifies the ID token, and reads a groups claim to derive Conway
// roles. It mirrors the existing Jira 3LO client's shape (server/jira/oauth.go).
package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
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

func (c *Config) scopes() []string {
	if len(c.Scopes) > 0 {
		return c.Scopes
	}
	return []string{"openid", "profile", "email", "groups"}
}

func (c *Config) groupsClaim() string {
	if c.GroupsClaim != "" {
		return c.GroupsClaim
	}
	return "groups"
}

// discovery is the subset of the OIDC discovery document we use.
type discovery struct {
	Issuer        string `json:"issuer"`
	AuthEndpoint  string `json:"authorization_endpoint"`
	TokenEndpoint string `json:"token_endpoint"`
	JWKSURI       string `json:"jwks_uri"`
}

// Provider is a Config resolved against its discovery document, with a cached
// JWKS key set. Build one with NewProvider; it is safe for concurrent use.
type Provider struct {
	cfg  Config
	disc discovery
	hc   *http.Client
	now  func() time.Time

	mu       sync.Mutex
	keys     map[string]*rsa.PublicKey
	keysAt   time.Time
	keysTTL  time.Duration
	fetchKey func(ctx context.Context) (map[string]*rsa.PublicKey, error) // overridable in tests
}

// NewProvider fetches the discovery document for cfg.Issuer and returns a ready
// Provider. Network failures here are surfaced so the server can log and run
// with OIDC disabled rather than crash.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	hc := &http.Client{Timeout: 15 * time.Second}
	well := strings.TrimRight(cfg.Issuer, "/") + "/.well-known/openid-configuration"
	var d discovery
	if err := getJSON(ctx, hc, well, &d); err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	if d.AuthEndpoint == "" || d.TokenEndpoint == "" || d.JWKSURI == "" {
		return nil, errors.New("oidc discovery missing required endpoints")
	}
	p := &Provider{cfg: cfg, disc: d, hc: hc, now: time.Now, keysTTL: time.Hour}
	p.fetchKey = p.fetchJWKS
	return p, nil
}

// Config returns the provider's configuration (read-only use by handlers).
func (p *Provider) Config() Config { return p.cfg }

// --- PKCE + login URL ----------------------------------------------------

// PKCE holds a verifier and its S256 challenge for one login attempt.
type PKCE struct {
	Verifier  string
	Challenge string
}

// NewPKCE generates a fresh PKCE pair (RFC 7636, S256).
func NewPKCE() (PKCE, error) {
	v, err := randB64(32)
	if err != nil {
		return PKCE{}, err
	}
	sum := sha256.Sum256([]byte(v))
	return PKCE{Verifier: v, Challenge: base64.RawURLEncoding.EncodeToString(sum[:])}, nil
}

// NewState returns a random opaque value for CSRF-binding the login flow.
func NewState() (string, error) { return randB64(24) }

// AuthURL builds the provider authorize URL for the browser to navigate to.
func (p *Provider) AuthURL(state, nonce, challenge string) string {
	v := url.Values{}
	v.Set("response_type", "code")
	v.Set("client_id", p.cfg.ClientID)
	v.Set("redirect_uri", p.cfg.RedirectURI)
	v.Set("scope", strings.Join(p.cfg.scopes(), " "))
	v.Set("state", state)
	v.Set("nonce", nonce)
	v.Set("code_challenge", challenge)
	v.Set("code_challenge_method", "S256")
	return p.disc.AuthEndpoint + "?" + v.Encode()
}

// --- code exchange -------------------------------------------------------

type tokenResponse struct {
	IDToken     string `json:"id_token"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// Exchange trades an authorization code (with its PKCE verifier) for tokens and
// returns the raw ID token. The client secret is sent when present (confidential
// client); PKCE covers public clients that omit it.
func (p *Provider) Exchange(ctx context.Context, code, verifier string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", p.cfg.RedirectURI)
	form.Set("client_id", p.cfg.ClientID)
	form.Set("code_verifier", verifier)
	if p.cfg.ClientSecret != "" {
		form.Set("client_secret", p.cfg.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.disc.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := p.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("token endpoint: bad response (status %d)", resp.StatusCode)
	}
	if tr.Error != "" {
		return "", fmt.Errorf("token endpoint error: %s %s", tr.Error, tr.ErrorDesc)
	}
	if resp.StatusCode/100 != 2 || tr.IDToken == "" {
		return "", fmt.Errorf("token endpoint: no id_token (status %d)", resp.StatusCode)
	}
	return tr.IDToken, nil
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

// idClaims are the registered + standard claims we validate/read.
type idClaims struct {
	Iss   string          `json:"iss"`
	Sub   string          `json:"sub"`
	Aud   json.RawMessage `json:"aud"` // string or []string
	Exp   int64           `json:"exp"`
	Nonce string          `json:"nonce"`
	Email string          `json:"email"`
	Name  string          `json:"name"`
}

// VerifyIDToken checks the ID token's signature (against the cached JWKS) and
// its iss / aud / exp / nonce, then extracts identity + the configured groups
// claim. A nonzero clock skew of 60s is tolerated on exp.
func (p *Provider) VerifyIDToken(ctx context.Context, rawIDToken, wantNonce string) (*Identity, error) {
	keys, err := p.keySet(ctx)
	if err != nil {
		return nil, err
	}
	claimBytes, err := verifySignature(rawIDToken, keys)
	if err != nil {
		// One retry with a forced JWKS refresh: providers rotate signing keys,
		// and a cached set can miss a brand-new kid.
		if keys2, ferr := p.refreshKeys(ctx); ferr == nil {
			claimBytes, err = verifySignature(rawIDToken, keys2)
		}
		if err != nil {
			return nil, err
		}
	}
	var c idClaims
	if err := json.Unmarshal(claimBytes, &c); err != nil {
		return nil, fmt.Errorf("parse id token claims: %w", err)
	}
	if c.Iss != p.disc.Issuer {
		return nil, fmt.Errorf("id token issuer mismatch")
	}
	if !audienceContains(c.Aud, p.cfg.ClientID) {
		return nil, errors.New("id token audience mismatch")
	}
	if p.now().Unix() >= c.Exp+60 {
		return nil, errors.New("id token expired")
	}
	if wantNonce != "" && c.Nonce != wantNonce {
		return nil, errors.New("id token nonce mismatch")
	}
	groups, err := extractGroups(claimBytes, p.cfg.groupsClaim())
	if err != nil {
		return nil, err
	}
	return &Identity{Subject: c.Sub, Email: c.Email, Name: c.Name, Groups: groups}, nil
}

// Roles maps an identity's groups to Conway roles via the configured RoleMap.
func (p *Provider) Roles(id *Identity) []string { return p.cfg.RoleMap.Map(id.Groups) }

// audienceContains reports whether the aud claim (string or array) includes want.
func audienceContains(raw json.RawMessage, want string) bool {
	if len(raw) == 0 {
		return false
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return one == want
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return slices.Contains(many, want)
	}
	return false
}

// extractGroups pulls a string-array claim by name from the raw claims JSON.
// A missing claim yields an empty slice (not an error) — the caller decides
// whether an empty role set means "deny".
func extractGroups(claimBytes []byte, name string) ([]string, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(claimBytes, &m); err != nil {
		return nil, err
	}
	raw, ok := m[name]
	if !ok {
		return nil, nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	// Some IdPs emit a single group as a bare string.
	var one string
	if err := json.Unmarshal(raw, &one); err == nil && one != "" {
		return []string{one}, nil
	}
	return nil, nil
}

// --- JWKS caching --------------------------------------------------------

func (p *Provider) keySet(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	p.mu.Lock()
	fresh := p.keys != nil && p.now().Sub(p.keysAt) < p.keysTTL
	keys := p.keys
	p.mu.Unlock()
	if fresh {
		return keys, nil
	}
	return p.refreshKeys(ctx)
}

func (p *Provider) refreshKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	keys, err := p.fetchKey(ctx)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.keys, p.keysAt = keys, p.now()
	p.mu.Unlock()
	return keys, nil
}

func (p *Provider) fetchJWKS(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.disc.JWKSURI, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("jwks endpoint status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return parseJWKS(body)
}

// --- small helpers -------------------------------------------------------

func randB64(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func getJSON(ctx context.Context, hc *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}
