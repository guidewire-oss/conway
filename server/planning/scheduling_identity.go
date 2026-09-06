package planning

import (
	"fmt"
	"strings"
)

func normalizedIdentity(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

// ValidateInitiativeNames rejects ambiguous row identities before map-based
// scheduling. specs/019-scheduling-audit-and-gantt-integrity.md:135
func ValidateInitiativeNames(inits []Initiative) error {
	seen := map[string]bool{}
	for i, it := range inits {
		key := normalizedIdentity(it.Name)
		if key == "" {
			return fmt.Errorf("initiative row %d has a blank name", i+1)
		}
		if seen[key] {
			return fmt.Errorf("duplicate initiative name %q", it.Name)
		}
		seen[key] = true
	}
	return nil
}

func leadIdentity(value string) string {
	key := normalizedIdentity(value)
	switch key {
	case "", "tbd", "none", "n/a", "na", "not required", "unknown", "-":
		return ""
	default:
		return key
	}
}
