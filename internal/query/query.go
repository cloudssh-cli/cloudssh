// Package query is the one small boolean grammar used by both the search box
// (matching rows in memory) and filters (matching on the server and checked
// again on the returned items).
//
//	web&api         web AND api
//	web|api         web OR api
//	web,api         web OR api               (a comma is OR too)
//	web&api|db      (web AND api) OR db      (& binds tighter than |)
//	web|api !canary (web OR api) AND NOT canary
//	web&(api|db)    web AND (api OR db)
//	!(a|b)          neither a nor b
//
// A space separates whole conditions, like an invisible pair of parentheses
// around each: the pieces between spaces are evaluated on their own and then
// ANDed. Inside one piece, & binds tighter than |. Spaces next to an operator
// ("web | api") do not count as separators.
//
// A query with no usable term matches everything. Parsing never fails: while
// someone is still typing, a missing ")" is closed at the end, a stray ")" or
// operator is ignored.
package query

import "strings"

type kind uint8

const (
	kTerm kind = iota
	kAnd
	kOr
	kNot
)

type node struct {
	kind kind
	term string  // kTerm
	kids []*node // kAnd, kOr (>=2 once simplified), kNot (exactly 1)
}

// Expr is a parsed query.
type Expr struct{ root *node }

// Parse reads s.
func Parse(s string) Expr {
	p := &parser{in: squeeze(tokenize(s))}
	return Expr{root: p.parseSeq(0)}
}

// tokenize splits s into terms and the operators ( ) | & !. A comma is "|",
// whitespace separates (and means AND). "!" only counts at the start of a term.
func tokenize(s string) []string {
	var toks []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch r {
		case ' ', '\t':
			flush()
			toks = append(toks, " ")
		case '(', ')', '|', '&':
			flush()
			toks = append(toks, string(r))
		case ',':
			flush()
			toks = append(toks, "|")
		case '!':
			if cur.Len() == 0 {
				toks = append(toks, "!")
			} else {
				cur.WriteRune(r)
			}
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return toks
}

type parser struct {
	in  []string
	pos int
}

func (p *parser) peek() string {
	if p.pos < len(p.in) {
		return p.in[p.pos]
	}
	return ""
}

// squeeze drops spaces that only sit next to an operator or a bracket, so
// "web | api" and "( web )" read like "web|api" and "(web)". The spaces left
// separate whole conditions.
func squeeze(in []string) []string {
	opens := func(t string) bool { return t == "|" || t == "&" || t == "(" || t == "!" }
	closes := func(t string) bool { return t == "|" || t == "&" || t == ")" }
	var out []string
	for i, t := range in {
		if t == " " {
			if len(out) == 0 || opens(out[len(out)-1]) || i+1 >= len(in) || closes(in[i+1]) || in[i+1] == " " {
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

// parseSeq: space-separated conditions, ANDed. Stops at ")" inside brackets.
func (p *parser) parseSeq(depth int) *node {
	var parts []*node
	for p.pos < len(p.in) {
		switch p.peek() {
		case " ":
			p.pos++
			continue
		case ")":
			if depth > 0 {
				return join(kAnd, parts)
			}
			p.pos++ // stray ")": ignore
			continue
		}
		if n := p.parseOr(depth); n != nil {
			parts = append(parts, n)
		}
	}
	return join(kAnd, parts)
}

// parseOr: tight-and ( "|" tight-and )*
func (p *parser) parseOr(depth int) *node {
	var alts []*node
	for {
		if n := p.parseTight(depth); n != nil {
			alts = append(alts, n)
		}
		if p.peek() != "|" {
			break
		}
		p.pos++
	}
	return join(kOr, alts)
}

// parseTight: unary ( "&" unary )*
func (p *parser) parseTight(depth int) *node {
	var parts []*node
	for {
		if n := p.parseUnary(depth); n != nil {
			parts = append(parts, n)
		}
		if p.peek() != "&" {
			break
		}
		p.pos++
	}
	return join(kAnd, parts)
}

func (p *parser) parseUnary(depth int) *node {
	switch t := p.peek(); t {
	case "", " ", "|", "&", ")":
		return nil // nothing here; the caller deals with the token
	case "!":
		p.pos++
		if n := p.parseUnary(depth); n != nil {
			return &node{kind: kNot, kids: []*node{n}}
		}
		return nil
	case "(":
		p.pos++
		n := p.parseSeq(depth + 1)
		if p.peek() == ")" {
			p.pos++
		}
		return n
	default:
		p.pos++
		return &node{kind: kTerm, term: t}
	}
}

// join builds an and/or node, dropping the wrapper for 0 or 1 children.
func join(k kind, kids []*node) *node {
	switch len(kids) {
	case 0:
		return nil
	case 1:
		return kids[0]
	}
	return &node{kind: k, kids: kids}
}

// Empty reports whether the query has no usable term.
func (e Expr) Empty() bool { return e.root == nil }

// Match evaluates the query; has reports whether the item contains a term.
func (e Expr) Match(has func(term string) bool) bool {
	if e.root == nil {
		return true
	}
	return e.root.eval(has)
}

func (n *node) eval(has func(string) bool) bool {
	switch n.kind {
	case kTerm:
		return has(n.term)
	case kNot:
		return !n.kids[0].eval(has)
	case kAnd:
		for _, k := range n.kids {
			if !k.eval(has) {
				return false
			}
		}
		return true
	default: // kOr
		for _, k := range n.kids {
			if k.eval(has) {
				return true
			}
		}
		return false
	}
}

// ServerTerms is a list of terms such that every matching item contains at
// least one of them: a superset a server can apply as "any of these" before
// Match narrows it down. Nil when the query cannot be narrowed that way
// (empty, or it starts from a NOT).
func (e Expr) ServerTerms() []string {
	if e.root == nil {
		return nil
	}
	terms, ok := e.root.superset()
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	out := terms[:0:0]
	for _, t := range terms {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// superset returns terms of which every match contains one; ok=false when no
// such list exists (anything could match).
func (n *node) superset() ([]string, bool) {
	switch n.kind {
	case kTerm:
		return []string{n.term}, true
	case kNot:
		return nil, false
	case kAnd: // any one conjunct is enough; take the tightest
		var best []string
		found := false
		for _, k := range n.kids {
			if s, ok := k.superset(); ok && (!found || len(s) < len(best)) {
				best, found = s, true
			}
		}
		return best, found
	default: // kOr: every alternative must be bounded
		var all []string
		for _, k := range n.kids {
			s, ok := k.superset()
			if !ok {
				return nil, false
			}
			all = append(all, s...)
		}
		return all, true
	}
}

// ExactOnServer reports whether ServerTerms alone gives the exact answer, so
// no second check is needed: the query is a plain OR of single terms.
func (e Expr) ExactOnServer() bool {
	if e.root == nil {
		return true
	}
	switch e.root.kind {
	case kTerm:
		return true
	case kOr:
		for _, k := range e.root.kids {
			if k.kind != kTerm {
				return false
			}
		}
		return true
	}
	return false
}
