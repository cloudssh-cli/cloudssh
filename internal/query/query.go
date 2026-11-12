// Package query is the one small boolean grammar used by both the search box
// (matching rows in memory) and filters (matching on the server and checked
// again on the returned items).
//
//	web api         web AND api              (a space is AND)
//	web&api         web AND api
//	web|api         web OR api
//	web,api         web OR api               (a comma is OR too)
//	web&(api|db)    web AND (api OR db)      (parentheses group)
//	web api|db      (web AND api) OR db      (AND binds tighter than OR)
//	!canary         NOT canary
//	!(a|b)          neither a nor b
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
	p := &parser{in: tokenize(s)}
	return Expr{root: p.parseOr(0)}
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

// parseOr: and ( "|" and )* ; stops at an unmatched ")".
func (p *parser) parseOr(depth int) *node {
	var alts []*node
	for {
		if n := p.parseAnd(depth); n != nil {
			alts = append(alts, n)
		}
		if p.peek() == "|" {
			p.pos++
			continue
		}
		if p.peek() == ")" && depth == 0 {
			p.pos++ // stray ")": ignore and carry on
			continue
		}
		break
	}
	return join(kOr, alts)
}

// parseAnd: unary ( ("&" | space) unary )*
func (p *parser) parseAnd(depth int) *node {
	var parts []*node
	for p.pos < len(p.in) {
		t := p.peek()
		if t == "|" || (t == ")" && depth > 0) || t == ")" {
			break
		}
		if t == " " || t == "&" {
			p.pos++
			continue
		}
		if n := p.parseUnary(depth); n != nil {
			parts = append(parts, n)
		}
	}
	return join(kAnd, parts)
}

func (p *parser) parseUnary(depth int) *node {
	t := p.peek()
	p.pos++
	switch t {
	case "!":
		for p.peek() == " " {
			p.pos++ // "! web" still means "!web"
		}
		if p.pos >= len(p.in) || strings.ContainsAny(p.peek(), "|&)") && len(p.peek()) == 1 {
			return nil
		}
		if n := p.parseUnary(depth); n != nil {
			return &node{kind: kNot, kids: []*node{n}}
		}
		return nil
	case "(":
		n := p.parseOr(depth + 1)
		if p.peek() == ")" {
			p.pos++
		}
		return n
	default:
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
