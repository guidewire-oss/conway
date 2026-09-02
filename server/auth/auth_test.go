package auth

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("PasswordRoundTrip", func() {
	It("behaves", func() {
		salt := "abc123"
		h := HashPassword("hunter2", salt)
		if !VerifyPassword("hunter2", salt, h) {
			Fail("correct password should verify")
		}
		if VerifyPassword("wrong", salt, h) {
			Fail("wrong password must not verify")
		}
	})
})

var _ = Describe("SaltChangesHash", func() {
	It("behaves", func() {
		if HashPassword("pw", "salt1") == HashPassword("pw", "salt2") {
			Fail("different salts must yield different hashes")
		}
	})
})

var _ = Describe("TokenRoundTrip", func() {
	It("behaves", func() {
		secret := []byte("server-secret")
		tok := SignToken(secret, "team1", []string{"player"}, 1000)
		c, err := ParseToken(secret, tok, 900)
		if err != nil {
			Fail(fmt.Sprintf("valid token should parse: %v", err))
		}
		if c.Sub != "team1" || !c.Has("player") {
			Fail(fmt.Sprintf("claims mismatch: %+v", c))
		}
	})
})

var _ = Describe("TokenTamperFails", func() {
	It("behaves", func() {
		secret := []byte("s")
		tok := SignToken(secret, "team1", []string{"player"}, 1000)
		if _, err := ParseToken(secret, tok+"x", 900); err == nil {
			Fail("tampered token must fail")
		}
		if _, err := ParseToken([]byte("other"), tok, 900); err == nil {
			Fail("wrong secret must fail")
		}
	})
})

var _ = Describe("TokenExpiry", func() {
	It("behaves", func() {
		secret := []byte("s")
		tok := SignToken(secret, "team1", []string{"player"}, 1000)
		if _, err := ParseToken(secret, tok, 1001); err == nil {
			Fail("expired token must fail")
		}
	})
})

var _ = Describe("StoreLifecycle", func() {
	It("behaves", func() {
		now := int64(1000)
		s := NewStore([]byte("secret"))
		s.NowUnix = func() int64 { return now }

		u, pw := s.CreateUser("Team 1", []string{"player"}, 48)
		if u.Username == "" || pw == "" {
			Fail("create should return username and generated password")
		}
		if _, err := s.Authenticate(u.Username, pw); err != nil {
			Fail(fmt.Sprintf("fresh account should authenticate: %v", err))
		}
		if _, err := s.Authenticate(u.Username, "nope"); err == nil {
			Fail("bad password must fail")
		}

		// expire it
		now += int64(49 * 3600)
		if _, err := s.Authenticate(u.Username, pw); err == nil {
			Fail("expired account must fail to authenticate")
		}

		// extend revives it
		s.Extend(u.Username, 24)
		if _, err := s.Authenticate(u.Username, pw); err != nil {
			Fail(fmt.Sprintf("extended account should authenticate: %v", err))
		}

		// delete
		s.Delete(u.Username)
		if _, err := s.Authenticate(u.Username, pw); err == nil {
			Fail("deleted account must fail")
		}
	})
})

var _ = Describe("StorePersistence", func() {
	It("behaves", func() {
		dir := GinkgoT().TempDir()
		path := dir + "/store.json"
		s1 := NewStore([]byte("secret"))
		s1.Path = path
		u, pw := s1.CreateUser("Team A", []string{"player"}, 48)
		if err := s1.Save(); err != nil {
			Fail(fmt.Sprintf("save: %v", err))
		}

		s2 := NewStore(nil)
		s2.Path = path
		if err := s2.Load(); err != nil {
			Fail(fmt.Sprintf("load: %v", err))
		}
		if _, err := s2.Authenticate(u.Username, pw); err != nil {
			Fail(fmt.Sprintf("account should survive reload: %v", err))
		}
		if string(s2.Secret) != "secret" {
			Fail("secret should persist")
		}
	})
})

var _ = Describe("SignTokenGameRoundTrip", func() {
	It("behaves", func() {
		secret := []byte("server-secret")
		tok := SignTokenGame(secret, "Team Rocket", []string{"player"}, "game-123", 1000)
		c, err := ParseToken(secret, tok, 900)
		if err != nil {
			Fail(fmt.Sprintf("join token should parse: %v", err))
		}
		if c.Sub != "Team Rocket" || c.GameID != "game-123" || !c.Has("player") {
			Fail(fmt.Sprintf("join claims mismatch: %+v", c))
		}
		// a plain token carries no game scope
		c2, _ := ParseToken(secret, SignToken(secret, "u", []string{"admin"}, 1000), 900)
		if c2.GameID != "" {
			Fail(fmt.Sprintf("plain token should have no game id: %q", c2.GameID))
		}
	})
})

var _ = Describe("UpsertSSOCreatesAndUpdates", func() {
	It("behaves", func() {
		now := int64(1000)
		s := NewStore([]byte("secret"))
		s.NowUnix = func() int64 { return now }

		// first login: creates a passwordless account with the mapped roles
		u := s.UpsertSSO("dana@acme.com", "Dana Ops", []string{"facilitator"})
		if u.Username != "dana@acme.com" || u.Display != "Dana Ops" {
			Fail(fmt.Sprintf("unexpected account: %+v", u))
		}
		if !u.Has("facilitator") || u.Has("admin") {
			Fail(fmt.Sprintf("roles not set from IdP: %v", u.Roles))
		}
		if u.Hash != "" || u.Salt != "" {
			Fail("SSO account must have no password hash/salt")
		}
		if u.SSO != true {
			Fail("SSO account must be flagged")
		}

		// second login with changed roles: same row, roles re-synced from IdP
		u2 := s.UpsertSSO("dana@acme.com", "Dana O", []string{"manager", "admin"})
		if len(s.Users) != 1 {
			Fail(fmt.Sprintf("re-login must not duplicate accounts: %d", len(s.Users)))
		}
		if !u2.Has("admin") || !u2.Has("manager") || u2.Has("facilitator") {
			Fail(fmt.Sprintf("roles not re-synced: %v", u2.Roles))
		}
		if u2.CreatedAt != u.CreatedAt {
			Fail("createdAt should be preserved across re-login")
		}
	})
})

var _ = Describe("SSOAccountRejectsPasswordLogin", func() {
	It("behaves", func() {
		s := NewStore([]byte("secret"))
		s.NowUnix = func() int64 { return 1000 }
		s.UpsertSSO("dana@acme.com", "Dana", []string{"admin"})
		// empty password / any password must never authenticate an SSO account
		if _, err := s.Authenticate("dana@acme.com", ""); err == nil {
			Fail("SSO account must reject empty-password login")
		}
		if _, err := s.Authenticate("dana@acme.com", "guess"); err == nil {
			Fail("SSO account must reject password login")
		}
	})
})
