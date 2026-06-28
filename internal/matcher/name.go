package matcher

import "strings"

// NormalizeName collapses a component-ish name to a comparable key —
// lowercase, letters/digits only — so "Button", "_Button", "Save CTA Button"
// (no, that one shouldn't match) but "Button" vs "1st Avatar..." vs "_Button"
// all reduce predictably for an exact-equality check.
func NormalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// MatchByName looks for an unambiguous exact match between target's
// normalized name and the candidates' normalized Story.Component names.
// ok is false when target is empty, nothing matches, or more than one
// distinct component normalizes to the same key (ambiguous — the caller
// should fall back to vision rather than guess).
func MatchByName(target string, candidates []CatalogItem) (CatalogItem, bool) {
	norm := NormalizeName(target)
	if norm == "" {
		return CatalogItem{}, false
	}

	seen := map[string]bool{}
	var found CatalogItem
	matches := 0
	for _, c := range candidates {
		if seen[c.Story.Component] {
			continue
		}
		seen[c.Story.Component] = true
		if NormalizeName(c.Story.Component) == norm {
			found = c
			matches++
		}
	}
	return found, matches == 1
}
