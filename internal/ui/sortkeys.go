package ui

import (
	"strings"
	"unicode"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// reservedKeys are shift+letter bindings with a fixed meaning.
var reservedKeys = map[string]bool{"S": true, "F": true, "R": true, "G": true}

// assignSortKeys gives each column a k9s-style shift+letter sort key. Word
// initials are tried first, then the remaining letters of the title; reserved
// and already-used letters are skipped. Columns that run out get "".
func assignSortKeys(cols []provider.Column) []string {
	used := map[string]bool{}
	keys := make([]string, len(cols))
	for i, c := range cols {
		var cands []rune
		for _, w := range strings.Fields(c.Title) {
			cands = append(cands, []rune(w)[0])
		}
		cands = append(cands, []rune(c.Title)...)
		for _, r := range cands {
			k := strings.ToUpper(string(r))
			if !unicode.IsLetter(r) || len(k) != 1 || reservedKeys[k] || used[k] {
				continue
			}
			keys[i], used[k] = k, true
			break
		}
	}
	return keys
}
