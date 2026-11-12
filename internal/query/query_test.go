package query

import (
	"reflect"
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	text := "i-0abc web-prod-1 running us-east-1a"
	has := func(term string) bool { return strings.Contains(text, term) }
	for q, want := range map[string]bool{
		"": true, "web": true, "db": false, "db|web": true, "web running": true, "web&stopped": false,
		"web !canary": true, "web !running": false, "db|running !stop": true, "!web": false,
		"db cache|stopped": false, "web api|running": true, "web api|stopped": false, "web cache|running": true, "web | db": true, "web & running": true, "( web )": true, "web&db|running": true, "web&(db|running)": true, "web&(db|cache)": false, "(db|cache) web": false,
		"!(db|cache)": true, "!(db|web)": false, "web,db": true, "! web": false, "(web)": true,
		// precedence: "!" now binds to its own term only
		"db|web !canary": true, "db|web !running": false, // = db | (web AND NOT running) "(db|web) !running": false,
	} {
		if got := Parse(q).Match(has); got != want {
			t.Errorf("Match(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestForgivingWhileTyping(t *testing.T) {
	has := func(term string) bool { return term == "web" }
	for q, want := range map[string]bool{
		"(web":         true, // missing ")" is closed at the end
		"web)":         true, // stray ")" is ignored
		"web&":         true,
		"|web|":        true,
		"&web,|api":    true,
		"(":            true,
		")":            true,
		" | , & ! ( )": true,
		"web !":        true,
		"!":            true,
		"api|":         false,
		"web&(api":     false,
		"((web))":      true,
		"a!b":          false, // "!" inside a term is literal
	} {
		if got := Parse(q).Match(has); got != want {
			t.Errorf("Match(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestEmpty(t *testing.T) {
	for _, q := range []string{"", " ", " | , & ! ", "()", "!", "( )"} {
		if !Parse(q).Empty() {
			t.Errorf("%q should be empty", q)
		}
	}
	if Parse("!a").Empty() || Parse("a").Empty() {
		t.Error("real terms are not empty")
	}
}

func TestServerTerms(t *testing.T) {
	for q, want := range map[string][]string{
		"":                nil,
		"web":             {"web"},
		"web|api":         {"web", "api"},
		"web api|db":      {"web"}, // web AND (api|db): any one conjunct will do
		"web|api !canary": {"web", "api"},
		"web&(api|batch)": {"web"}, // the tightest conjunct
		"(a|b|c) d":       {"d"},   // 1 term beats 3
		"!web":            nil,     // nothing to narrow by
		"web !canary":     {"web"},
		"a|!b":            nil, // one alternative is unbounded
		"a|a|b":           {"a", "b"},
	} {
		if got := Parse(q).ServerTerms(); !reflect.DeepEqual(got, want) {
			t.Errorf("ServerTerms(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestExactOnServer(t *testing.T) {
	for q, want := range map[string]bool{
		"": true, "a": true, "a|b": true, "a,b": true,
		"a b": false, "a !b": false, "!a": false, "a&(b|c)": false, "(a|b)": true,
	} {
		if got := Parse(q).ExactOnServer(); got != want {
			t.Errorf("ExactOnServer(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestBackslashEscapes(t *testing.T) {
	text := "a|b x&y (p) a b !hot back\\slash"
	has := func(term string) bool { return strings.Contains(text, term) }
	for q, want := range map[string]bool{
		`a\|b`:        true,  // the text "a|b", not an OR
		`a\|zzz`:      false, // no OR: "a|zzz" is absent
		`x\&y`:        true,
		`\(p\)`:       true,
		`a\ b`:        true, // an escaped space stays inside the term
		`\!hot`:       true, // "!hot" as text
		`!\!hot`:      false,
		`back\\slash`: true, // \\ is one backslash
		`zzz\|a`:      false,
		`zzz|a`:       true, // unescaped, it is an OR
		`a\`:          true, // a trailing backslash is ignored
		`\,`:          false,
	} {
		if got := Parse(q).Match(has); got != want {
			t.Errorf("Match(%q) = %v, want %v", q, got, want)
		}
	}
	if got := Parse(`a\|b c`).ServerTerms(); !reflect.DeepEqual(got, []string{"a|b"}) {
		t.Errorf("escaped terms reach the server literally: %v", got)
	}
}
