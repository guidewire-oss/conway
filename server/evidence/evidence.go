// Package evidence defines durable capture configuration and identity policies.
package evidence

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

type Team struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}
type Config struct {
	Name           string          `json:"name"`
	Site           string          `json:"site"`
	Projects       []string        `json:"projects"`
	RosterID       string          `json:"rosterId"`
	Roster         json.RawMessage `json:"roster"`
	WipMode        string          `json:"wipMode"`
	PodField       string          `json:"podField"`
	IntervalHours  int             `json:"intervalHours"`
	FreshnessHours int             `json:"freshnessHours"`
	Enabled        bool            `json:"enabled"`
	Teams          []Team          `json:"teams"`
}

// per specs/026-reliable-evidence-foundation.md:34
func Freshness(last int64, hours int, now int64) string {
	if last == 0 {
		return "No evidence"
	}
	if now-last >= int64(hours)*3600 {
		return "Stale"
	}
	return "Fresh"
}
func ValidateSite(site string) error {
	u, err := url.Parse(site)
	if err != nil {
		return fmt.Errorf("enter a Jira Cloud site URL")
	}
	if u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.atlassian\.net$`).MatchString(u.Host) {
		return fmt.Errorf("use an HTTPS Jira Cloud site such as https://atlas.atlassian.net, without a path or port")
	}
	return nil
}
func normal(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
func ValidateTeams(teams []Team) error {
	ids := map[string]bool{}
	aliases := map[string]string{}
	if len(teams) == 0 || len(teams) > 1000 {
		return fmt.Errorf("provide between 1 and 1000 teams")
	}
	for _, t := range teams {
		if t.ID == "" || ids[t.ID] || strings.TrimSpace(t.Name) == "" || len(t.Name) > 200 || len(t.Aliases) == 0 || len(t.Aliases) > 50 {
			return fmt.Errorf("teams need unique IDs, names and aliases")
		}
		ids[t.ID] = true
		for _, a := range t.Aliases {
			k := normal(a)
			if k == "" || len(a) > 200 {
				return fmt.Errorf("team aliases must be nonempty and at most 200 characters")
			}
			if owner, ok := aliases[k]; ok && owner != t.ID {
				return fmt.Errorf("team alias %q belongs to more than one team", a)
			}
			aliases[k] = t.ID
		}
	}
	return nil
}
func TeamID(teams []Team, name string) string {
	for _, t := range teams {
		for _, a := range t.Aliases {
			if normal(a) == normal(name) {
				return t.ID
			}
		}
	}
	return ""
}
func Identity(source, kind, provider string) string {
	sum := sha256.Sum256([]byte(source + "\x00" + kind + "\x00" + provider))
	return hex.EncodeToString(sum[:16])
}
func Validate(c Config) error {
	if strings.TrimSpace(c.Name) == "" || len(c.Name) > 120 {
		return fmt.Errorf("name the source using at most 120 characters")
	}
	if err := ValidateSite(c.Site); err != nil {
		return err
	}
	if len(c.Projects) == 0 || len(c.Projects) > 100 {
		return fmt.Errorf("choose 1 to 100 project keys")
	}
	seen := map[string]bool{}
	for _, p := range c.Projects {
		if !regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,99}$`).MatchString(p) || seen[p] {
			return fmt.Errorf("project keys must be unique uppercase Jira keys")
		}
		seen[p] = true
	}
	if c.RosterID == "" {
		return fmt.Errorf("choose a roster")
	}
	if c.WipMode != "leaf" && c.WipMode != "epic_or_parentless" {
		return fmt.Errorf("choose a supported counting policy")
	}
	if c.PodField != "" && !regexp.MustCompile(`^customfield_[0-9]+$`).MatchString(c.PodField) {
		return fmt.Errorf("team field must be a Jira customfield ID")
	}
	if c.IntervalHours != 0 && c.IntervalHours != 6 && c.IntervalHours != 24 && c.IntervalHours != 168 {
		return fmt.Errorf("choose manual, six-hour, daily or weekly captures")
	}
	if c.FreshnessHours < 1 || c.FreshnessHours > 720 {
		return fmt.Errorf("freshness must be 1 to 720 hours")
	}
	return ValidateTeams(c.Teams)
}

// per specs/026-reliable-evidence-foundation.md:138
func aead(secret []byte) (cipher.AEAD, error) {
	if len(secret) == 0 {
		return nil, fmt.Errorf("server secret unavailable")
	}
	key := sha256.Sum256(append([]byte("conway-evidence-credentials-v1\x00"), secret...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func Seal(secret []byte, id string, plain []byte) ([]byte, error) {
	a, err := aead(secret)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, plain, []byte(id)), nil
}
func Open(secret []byte, id string, sealed []byte) ([]byte, error) {
	a, err := aead(secret)
	if err != nil {
		return nil, err
	}
	n := a.NonceSize()
	if len(sealed) < n {
		return nil, fmt.Errorf("invalid saved credentials")
	}
	return a.Open(nil, sealed[:n], sealed[n:], []byte(id))
}
