package ui

import "testing"

func TestMatchSearch(t *testing.T) {
	text := "i-0abc\x00web-prod-1\x00running\x00us-east-1a"
	for query, want := range map[string]bool{
		"":                 true,
		"web":              true,
		"db":               false,
		"db|web":           true, // or
		"db|cache":         false,
		"web running":      true, // and
		"web stopped":      false,
		"!canary":          true, // not
		"!web":             false,
		"web !canary":      true,
		"web !running":     false,
		"db|web !running":  false,
		"db|running !stop": true,
		"|":                true, // no usable term matches everything
		"!":                true,
		" | ":              true,
		"web|":             true,
	} {
		if got := matchSearch(query, text); got != want {
			t.Errorf("matchSearch(%q) = %v, want %v", query, got, want)
		}
	}
}

func TestSearchBoxUnderstandsOrAndNot(t *testing.T) {
	m := loaded(t, threeRows()) // alpha, beta, gamma
	m, _ = send(m, key("/"))
	m = typeKeys(m, "alpha|gamma")
	if len(m.cur().view) != 2 {
		t.Fatalf("a|b should keep both: %d", len(m.cur().view))
	}
	m, _ = send(m, key("esc"))
	m, _ = send(m, key("/"))
	m = typeKeys(m, "!beta")
	if len(m.cur().view) != 2 {
		t.Fatalf("!beta should drop one row: %d", len(m.cur().view))
	}
}
