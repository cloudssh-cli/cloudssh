// Package query is the one small boolean grammar used by both the search box
// (matching rows in memory) and filters (matching on the server and checked
// again on the returned items).
//
//	web api      web AND api          (a space is AND)
//	web&api      web AND api
//	web|api      web OR api
//	web,api      web OR api           (a comma is OR too)
//	web api|db   (web AND api) OR db  (AND binds tighter than OR)
//	!canary      NOT canary           (a "!" term excludes everywhere in the query)
//
// A query with no usable term matches everything.
package query

import "strings"

// Expr is a parsed query: Any holds the alternatives (each a list of terms that
// must all match); Not holds exclusions that apply to every alternative.
type Expr struct {
	Any [][]string
	Not []string
}

// Parse reads s. It never fails: odd input just yields fewer terms.
func Parse(s string) Expr {
	var e Expr
	for _, alt := range strings.FieldsFunc(s, func(r rune) bool { return r == '|' || r == ',' }) {
		var terms []string
		for _, tok := range strings.FieldsFunc(alt, func(r rune) bool { return r == ' ' || r == '\t' || r == '&' }) {
			if rest, ok := strings.CutPrefix(tok, "!"); ok {
				if rest != "" {
					e.Not = append(e.Not, rest)
				}
				continue
			}
			terms = append(terms, tok)
		}
		if len(terms) > 0 {
			e.Any = append(e.Any, terms)
		}
	}
	return e
}

// Empty reports whether the query has no usable term.
func (e Expr) Empty() bool { return len(e.Any) == 0 && len(e.Not) == 0 }

// Match evaluates the query; has reports whether the item contains a term.
func (e Expr) Match(has func(term string) bool) bool {
	for _, n := range e.Not {
		if has(n) {
			return false
		}
	}
	if len(e.Any) == 0 {
		return true
	}
	for _, alt := range e.Any {
		all := true
		for _, t := range alt {
			if !has(t) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// ServerTerms is one positive term per alternative: a superset a server can
// apply as "any of these" before Match narrows it down. Empty when the query
// has no positive alternative (nothing to narrow by on the server).
func (e Expr) ServerTerms() []string {
	out := make([]string, 0, len(e.Any))
	for _, alt := range e.Any {
		out = append(out, alt[0])
	}
	return out
}

// ExactOnServer reports whether ServerTerms alone gives the exact answer, so no
// second check is needed: every alternative is a single term and nothing is excluded.
func (e Expr) ExactOnServer() bool {
	if len(e.Not) > 0 {
		return false
	}
	for _, alt := range e.Any {
		if len(alt) != 1 {
			return false
		}
	}
	return true
}
