// Package auth provides stdlib-only password hashing, signed expiring tokens,
// and a JSON-file-backed account store for the Conway offsite game.
//
// Security posture: PBKDF2-HMAC-SHA256 password hashing and HMAC-SHA256 signed
// tokens — sound primitives for an internal, short-lived offsite tool. The
// store is a plain JSON file (contains hashes + the signing secret); keep it
// off version control and readable only by the host user.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const pbkdf2Iter = 100_000

// ---- password hashing (PBKDF2-HMAC-SHA256, stdlib only) -----------------

func pbkdf2(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hLen := prf.Size()
	numBlocks := (keyLen + hLen - 1) / hLen
	var out []byte
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		prf.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := prf.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)
		for n := 1; n < iter; n++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)
			for i := range t {
				t[i] ^= u[i]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

// HashPassword returns a hex-encoded PBKDF2 hash of pw with the given salt.
func HashPassword(pw, salt string) string {
	return hex.EncodeToString(pbkdf2([]byte(pw), []byte(salt), pbkdf2Iter, 32))
}

// VerifyPassword checks pw against a stored hash in constant time.
func VerifyPassword(pw, salt, hashHex string) bool {
	want, err := hex.DecodeString(hashHex)
	if err != nil {
		return false
	}
	got := pbkdf2([]byte(pw), []byte(salt), pbkdf2Iter, 32)
	return subtle.ConstantTimeCompare(want, got) == 1
}

// ---- tokens (compact HMAC-signed, base64url) ----------------------------

type Claims struct {
	Sub    string   `json:"sub"`
	Roles  []string `json:"roles"`
	GameID string   `json:"gid,omitempty"` // set on join tokens: scopes a team to one game
	Exp    int64    `json:"exp"`
}

// Has reports whether the token carries an exact role (superuser logic — admin
// passing every gate — is applied at the permission check, not here).
func (c Claims) Has(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// CanTest reports whether the token may bypass a game's round-gating to play
// freely: true system admins, and "tester" tokens minted for a facilitator to
// trial-run one specific game of their own (see gamesadmin.go's "test" action).
// A tester token carries no other admin privilege — it only satisfies this check.
func (c Claims) CanTest() bool {
	return c.Has("admin") || c.Has("tester")
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
func sign(secret, payload []byte) string {
	m := hmac.New(sha256.New, secret)
	m.Write(payload)
	return b64(m.Sum(nil))
}

// SignToken builds payload.signature where payload is base64url(JSON claims).
func SignToken(secret []byte, sub string, roles []string, expUnix int64) string {
	return SignTokenGame(secret, sub, roles, "", expUnix)
}

// SignTokenGame is SignToken with a game scope (for join tokens).
func SignTokenGame(secret []byte, sub string, roles []string, gameID string, expUnix int64) string {
	p, _ := json.Marshal(Claims{Sub: sub, Roles: roles, GameID: gameID, Exp: expUnix})
	pe := b64(p)
	return pe + "." + sign(secret, []byte(pe))
}

// ParseToken validates signature and expiry (against nowUnix) and returns claims.
func ParseToken(secret []byte, tok string, nowUnix int64) (Claims, error) {
	var c Claims
	parts := strings.SplitN(tok, ".", 2)
	if len(parts) != 2 {
		return c, errors.New("malformed token")
	}
	if subtle.ConstantTimeCompare([]byte(parts[1]), []byte(sign(secret, []byte(parts[0])))) != 1 {
		return c, errors.New("bad signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	if nowUnix >= c.Exp {
		return c, errors.New("token expired")
	}
	return c, nil
}

// ---- account store ------------------------------------------------------

type User struct {
	Username  string   `json:"username"`
	Display   string   `json:"display"`
	Roles     []string `json:"roles"` // any of: admin, facilitator, manager, player
	Salt      string   `json:"salt"`
	Hash      string   `json:"hash"`
	ExpiresAt int64    `json:"expiresAt"` // unix seconds; 0 = never (admin, SSO)
	CreatedAt int64    `json:"createdAt"`
	SSO       bool     `json:"sso,omitempty"` // provisioned via OIDC; has no password
}

// Has reports exact role membership (admin-superuser logic lives at the gate).
func (u *User) Has(role string) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

type Store struct {
	Secret  []byte           `json:"secret"`
	Users   map[string]*User `json:"users"`
	Path    string           `json:"-"`
	NowUnix func() int64     `json:"-"`
	backend Backend          // when set, Save/Load go here instead of the file
}

// Backend persists the store somewhere other than a JSON file (e.g. Postgres).
// Implemented in the db package so this package stays dependency-free.
type Backend interface {
	Save(*Store) error
	Load(*Store) error
}

// SetBackend routes Save/Load to b (e.g. Postgres) instead of the filesystem.
func (s *Store) SetBackend(b Backend) { s.backend = b }

func NewStore(secret []byte) *Store {
	return &Store{
		Secret:  secret,
		Users:   map[string]*User{},
		NowUnix: func() int64 { return time.Now().Unix() },
	}
}

func randString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}

// slugify a display name into a stable username.
func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "user"
	}
	return out
}

// CreateUser mints an account with a generated password, returning (user, plaintextPassword).
// expiryHours <= 0 means never-expires (use for admin).
func (s *Store) CreateUser(display string, roles []string, expiryHours int) (*User, string) {
	now := s.NowUnix()
	base := slug(display)
	username := base
	for i := 2; s.Users[username] != nil; i++ {
		username = fmt.Sprintf("%s%d", base, i)
	}
	pw := randString(10)
	salt := randString(16)
	exp := int64(0)
	if expiryHours > 0 {
		exp = now + int64(expiryHours)*3600
	}
	if len(roles) == 0 {
		roles = []string{"player"}
	}
	u := &User{
		Username: username, Display: display, Roles: roles,
		Salt: salt, Hash: HashPassword(pw, salt),
		ExpiresAt: exp, CreatedAt: now,
	}
	s.Users[username] = u
	return u, pw
}

// UpsertSSO creates or updates a passwordless account for an OIDC-authenticated
// user. Roles are (re)synced from the IdP on every login so group changes take
// effect immediately; a pre-existing account's CreatedAt is preserved. The
// account never expires (ExpiresAt 0) — access is really gated at each login by
// the IdP, and the minted session token still carries its own 12h cap.
func (s *Store) UpsertSSO(username, display string, roles []string) *User {
	now := s.NowUnix()
	u := s.Users[username]
	if u == nil {
		u = &User{Username: username, CreatedAt: now}
		s.Users[username] = u
	}
	u.Display = display
	u.Roles = roles
	u.Salt = ""
	u.Hash = ""
	u.ExpiresAt = 0
	u.SSO = true
	return u
}

// SetAdmin creates or replaces the admin account with a known password.
func (s *Store) SetAdmin(password string) *User {
	salt := randString(16)
	u := &User{
		Username: "admin", Display: "Admin", Roles: []string{"admin"},
		Salt: salt, Hash: HashPassword(password, salt),
		ExpiresAt: 0, CreatedAt: s.NowUnix(),
	}
	s.Users["admin"] = u
	return u
}

var ErrAuth = errors.New("invalid credentials or expired account")

// Authenticate verifies password and expiry; returns the user on success.
func (s *Store) Authenticate(username, pw string) (*User, error) {
	u := s.Users[username]
	// SSO accounts carry no password material; reject them from this path
	// outright so an empty stored hash can never be matched.
	if u == nil || u.SSO || u.Hash == "" || !VerifyPassword(pw, u.Salt, u.Hash) {
		return nil, ErrAuth
	}
	if u.ExpiresAt != 0 && s.NowUnix() >= u.ExpiresAt {
		return nil, ErrAuth
	}
	return u, nil
}

// Token issues a signed token whose expiry matches the account's (capped 12h
// for admin so a leaked admin token doesn't live forever).
func (s *Store) Token(u *User) string {
	exp := u.ExpiresAt
	now := s.NowUnix()
	if exp == 0 || exp > now+12*3600 {
		exp = now + 12*3600
	}
	return SignToken(s.Secret, u.Username, u.Roles, exp)
}

func (s *Store) Extend(username string, hours int) bool {
	u := s.Users[username]
	if u == nil {
		return false
	}
	base := s.NowUnix()
	if u.ExpiresAt > base {
		base = u.ExpiresAt
	}
	u.ExpiresAt = base + int64(hours)*3600
	return true
}

func (s *Store) Delete(username string) { delete(s.Users, username) }

func (s *Store) Expired(u *User) bool {
	return u.ExpiresAt != 0 && s.NowUnix() >= u.ExpiresAt
}

func (s *Store) Save() error {
	if s.backend != nil {
		return s.backend.Save(s)
	}
	if s.Path == "" {
		return errors.New("no store path")
	}
	// G117: this file *is* the credential store — serialising Secret is the
	// purpose, and the write below is 0600. Not a leak into logs or a response.
	// #nosec G117 -- standalone gosec reads this form; golangci-lint reads the
	// trailing //nolint. Below the CI severity threshold today, annotated anyway
	// so lowering that threshold does not surface a finding with no explanation.
	b, err := json.MarshalIndent(s, "", " ") //nolint:gosec // G117: intentional, see above
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, b, 0o600)
}

func (s *Store) Load() error {
	if s.backend != nil {
		return s.backend.Load(s)
	}
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return err
	}
	if s.Users == nil {
		s.Users = map[string]*User{}
	}
	if s.NowUnix == nil {
		s.NowUnix = func() int64 { return time.Now().Unix() }
	}
	return nil
}
