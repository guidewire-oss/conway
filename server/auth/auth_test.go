package auth

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	salt := "abc123"
	h := HashPassword("hunter2", salt)
	if !VerifyPassword("hunter2", salt, h) {
		t.Fatal("correct password should verify")
	}
	if VerifyPassword("wrong", salt, h) {
		t.Fatal("wrong password must not verify")
	}
}

func TestSaltChangesHash(t *testing.T) {
	if HashPassword("pw", "salt1") == HashPassword("pw", "salt2") {
		t.Fatal("different salts must yield different hashes")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	secret := []byte("server-secret")
	tok := SignToken(secret, "team1", []string{"player"}, 1000)
	c, err := ParseToken(secret, tok, 900)
	if err != nil {
		t.Fatalf("valid token should parse: %v", err)
	}
	if c.Sub != "team1" || !c.Has("player") {
		t.Fatalf("claims mismatch: %+v", c)
	}
}

func TestTokenTamperFails(t *testing.T) {
	secret := []byte("s")
	tok := SignToken(secret, "team1", []string{"player"}, 1000)
	if _, err := ParseToken(secret, tok+"x", 900); err == nil {
		t.Fatal("tampered token must fail")
	}
	if _, err := ParseToken([]byte("other"), tok, 900); err == nil {
		t.Fatal("wrong secret must fail")
	}
}

func TestTokenExpiry(t *testing.T) {
	secret := []byte("s")
	tok := SignToken(secret, "team1", []string{"player"}, 1000)
	if _, err := ParseToken(secret, tok, 1001); err == nil {
		t.Fatal("expired token must fail")
	}
}

func TestStoreLifecycle(t *testing.T) {
	now := int64(1000)
	s := NewStore([]byte("secret"))
	s.NowUnix = func() int64 { return now }

	u, pw := s.CreateUser("Team 1", []string{"player"}, 48)
	if u.Username == "" || pw == "" {
		t.Fatal("create should return username and generated password")
	}
	if _, err := s.Authenticate(u.Username, pw); err != nil {
		t.Fatalf("fresh account should authenticate: %v", err)
	}
	if _, err := s.Authenticate(u.Username, "nope"); err == nil {
		t.Fatal("bad password must fail")
	}

	// expire it
	now += int64(49 * 3600)
	if _, err := s.Authenticate(u.Username, pw); err == nil {
		t.Fatal("expired account must fail to authenticate")
	}

	// extend revives it
	s.Extend(u.Username, 24)
	if _, err := s.Authenticate(u.Username, pw); err != nil {
		t.Fatalf("extended account should authenticate: %v", err)
	}

	// delete
	s.Delete(u.Username)
	if _, err := s.Authenticate(u.Username, pw); err == nil {
		t.Fatal("deleted account must fail")
	}
}

func TestStorePersistence(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/store.json"
	s1 := NewStore([]byte("secret"))
	s1.Path = path
	u, pw := s1.CreateUser("Team A", []string{"player"}, 48)
	if err := s1.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	s2 := NewStore(nil)
	s2.Path = path
	if err := s2.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := s2.Authenticate(u.Username, pw); err != nil {
		t.Fatalf("account should survive reload: %v", err)
	}
	if string(s2.Secret) != "secret" {
		t.Fatal("secret should persist")
	}
}

func TestSignTokenGameRoundTrip(t *testing.T) {
	secret := []byte("server-secret")
	tok := SignTokenGame(secret, "Team Rocket", []string{"player"}, "game-123", 1000)
	c, err := ParseToken(secret, tok, 900)
	if err != nil {
		t.Fatalf("join token should parse: %v", err)
	}
	if c.Sub != "Team Rocket" || c.GameID != "game-123" || !c.Has("player") {
		t.Fatalf("join claims mismatch: %+v", c)
	}
	// a plain token carries no game scope
	c2, _ := ParseToken(secret, SignToken(secret, "u", []string{"admin"}, 1000), 900)
	if c2.GameID != "" {
		t.Fatalf("plain token should have no game id: %q", c2.GameID)
	}
}

func TestUpsertSSOCreatesAndUpdates(t *testing.T) {
	now := int64(1000)
	s := NewStore([]byte("secret"))
	s.NowUnix = func() int64 { return now }

	// first login: creates a passwordless account with the mapped roles
	u := s.UpsertSSO("dana@acme.com", "Dana Ops", []string{"facilitator"})
	if u.Username != "dana@acme.com" || u.Display != "Dana Ops" {
		t.Fatalf("unexpected account: %+v", u)
	}
	if !u.Has("facilitator") || u.Has("admin") {
		t.Fatalf("roles not set from IdP: %v", u.Roles)
	}
	if u.Hash != "" || u.Salt != "" {
		t.Fatal("SSO account must have no password hash/salt")
	}
	if u.SSO != true {
		t.Fatal("SSO account must be flagged")
	}

	// second login with changed roles: same row, roles re-synced from IdP
	u2 := s.UpsertSSO("dana@acme.com", "Dana O", []string{"manager", "admin"})
	if len(s.Users) != 1 {
		t.Fatalf("re-login must not duplicate accounts: %d", len(s.Users))
	}
	if !u2.Has("admin") || !u2.Has("manager") || u2.Has("facilitator") {
		t.Fatalf("roles not re-synced: %v", u2.Roles)
	}
	if u2.CreatedAt != u.CreatedAt {
		t.Fatal("createdAt should be preserved across re-login")
	}
}

func TestSSOAccountRejectsPasswordLogin(t *testing.T) {
	s := NewStore([]byte("secret"))
	s.NowUnix = func() int64 { return 1000 }
	s.UpsertSSO("dana@acme.com", "Dana", []string{"admin"})
	// empty password / any password must never authenticate an SSO account
	if _, err := s.Authenticate("dana@acme.com", ""); err == nil {
		t.Fatal("SSO account must reject empty-password login")
	}
	if _, err := s.Authenticate("dana@acme.com", "guess"); err == nil {
		t.Fatal("SSO account must reject password login")
	}
}
