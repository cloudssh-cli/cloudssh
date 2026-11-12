package query

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for in, want := range map[string]Expr{
		"":            {},
		"web":         {Any: [][]string{{"web"}}},
		"web api":     {Any: [][]string{{"web", "api"}}},
		"web&api":     {Any: [][]string{{"web", "api"}}},
		"web|api":     {Any: [][]string{{"web"}, {"api"}}},
		"web,api":     {Any: [][]string{{"web"}, {"api"}}},
		"web api|db":  {Any: [][]string{{"web", "api"}, {"db"}}},
		"web !canary": {Any: [][]string{{"web"}}, Not: []string{"canary"}},
		"!a|b !c":     {Any: [][]string{{"b"}}, Not: []string{"a", "c"}},
		" | , & ! ":   {},
		"&web,|api":   {Any: [][]string{{"web"}, {"api"}}}, // stray operators are ignored
	} {
		if got := Parse(in); !reflect.DeepEqual(got, want) {
			t.Errorf("Parse(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func TestMatchAndServerTerms(t *testing.T) {
	text := "i-0abc web-prod-1 running us-east-1a"
	has := func(term string) bool { return strings.Contains(text, term) }
	for q, want := range map[string]bool{
		"": true, "web": true, "db": false, "db|web": true, "web running": true, "web&stopped": false,
		"web !canary": true, "web !running": false, "db|running !stop": true, "!web": false, "db cache|stopped": false,
	} {
		if got := Parse(q).Match(has); got != want {
			t.Errorf("Match(%q) = %v, want %v", q, got, want)
		}
	}
	e := Parse("web api|db !canary")
	if got := e.ServerTerms(); !reflect.DeepEqual(got, []string{"web", "db"}) {
		t.Errorf("ServerTerms: %v", got)
	}
	if e.ExactOnServer() || !Parse("a|b").ExactOnServer() || Parse("a b").ExactOnServer() || Parse("a !b").ExactOnServer() {
		t.Error("ExactOnServer is true only for a plain OR of single terms")
	}
	if !Parse("!a").Empty() == false || !Parse("").Empty() {
		t.Error("Empty")
	}
}
