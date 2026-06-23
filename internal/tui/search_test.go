package tui

import "testing"

func TestFindMatchesBasic(t *testing.T) {
	got := findMatches("foo bar foo", "foo")
	if len(got) != 2 {
		t.Fatalf("len = %d want 2 (%v)", len(got), got)
	}
	if got[0].Start != 0 || got[1].Start != 8 {
		t.Fatalf("starts = %d,%d want 0,8", got[0].Start, got[1].Start)
	}
}

func TestFindMatchesCaseInsensitive(t *testing.T) {
	got := findMatches("Hello HELLO hello", "hello")
	if len(got) != 3 {
		t.Fatalf("len = %d want 3", len(got))
	}
}

func TestFindMatchesEmpty(t *testing.T) {
	if got := findMatches("abc", ""); got != nil {
		t.Fatalf("got %v want nil", got)
	}
}

func TestFindMatchesNone(t *testing.T) {
	if got := findMatches("abc", "z"); len(got) != 0 {
		t.Fatalf("got %v want none", got)
	}
}

func TestHighlightMatches(t *testing.T) {
	c := "axbxc"
	got := highlightMatches(c, findMatches(c, "x"), "[", "]", -1, "", "")
	if got != "a[x]b[x]c" {
		t.Fatalf("got %q want a[x]b[x]c", got)
	}
}

func TestHighlightMatchesCurrent(t *testing.T) {
	c := "axbxc"
	got := highlightMatches(c, findMatches(c, "x"), "[", "]", 1, "{", "}")
	if got != "a[x]b{x}c" {
		t.Fatalf("got %q want a[x]b{x}c", got)
	}
}

func TestLineOf(t *testing.T) {
	if got := lineOf("a\nb\nc", 4); got != 2 {
		t.Fatalf("got %d want 2", got)
	}
}
