package oidc

import "strings"

// RoleMap maps external IdP group names to Conway roles. It is the RBAC bridge:
// the IdP owns group membership; this map decides what each group means inside
// Conway ({admin, facilitator, manager} — never "player", which is the code-join
// team path, not an SSO identity).
type RoleMap map[string]string

// knownRoles are the Conway roles an SSO login may grant. "player" is excluded
// deliberately: teams join a game by code and are never provisioned via SSO.
var knownRoles = map[string]bool{"admin": true, "facilitator": true, "manager": true}

// ParseRoleMap parses the CONWAY_OIDC_ROLE_MAP env format:
//
//	group=role,group=role,...
//
// e.g. "conway-admins=admin,conway-facilitators=facilitator". Group names are
// matched case-insensitively — the casing an admin types here rarely matches
// the IdP's exact group casing, and groups differing only by case are never
// intentional — so keys are stored lower-cased (and looked up lower-cased in
// Map). Roles are lower-cased and validated against knownRoles. Unknown roles
// and malformed pairs are skipped so one typo can't silently grant
// nothing-or-everything.
func ParseRoleMap(s string) RoleMap {
	m := RoleMap{}
	for pair := range strings.SplitSeq(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		eq := strings.LastIndex(pair, "=")
		if eq <= 0 {
			continue
		}
		group := strings.ToLower(strings.TrimSpace(pair[:eq]))
		role := strings.ToLower(strings.TrimSpace(pair[eq+1:]))
		if group == "" || !knownRoles[role] {
			continue
		}
		m[group] = role
	}
	return m
}

// Map resolves a set of IdP groups to the deduped, ordered Conway roles they
// grant. The result is empty when no group matches — the caller treats that as
// "deny" (no recognized role).
func (m RoleMap) Map(groups []string) []string {
	seen := map[string]bool{}
	// Fixed precedence so the stored roles slice is deterministic regardless of
	// the order groups arrive in the token.
	order := []string{"admin", "facilitator", "manager"}
	granted := map[string]bool{}
	for _, g := range groups {
		if role, ok := m[strings.ToLower(strings.TrimSpace(g))]; ok {
			granted[role] = true
		}
	}
	var out []string
	for _, role := range order {
		if granted[role] && !seen[role] {
			out = append(out, role)
			seen[role] = true
		}
	}
	return out
}
