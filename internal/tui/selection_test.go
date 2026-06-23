package tui

import "testing"

func TestSelectedTextSingleLine(t *testing.T) {
	lines := []string{"a", "b", "c"}
	if got := selectedText(lines, 1, 1); got != "b" {
		t.Errorf("got %q want %q", got, "b")
	}
}

func TestSelectedTextRange(t *testing.T) {
	lines := []string{"a", "b", "c", "d"}
	if got := selectedText(lines, 1, 2); got != "b\nc" {
		t.Errorf("got %q want %q", got, "b\nc")
	}
}

func TestSelectedTextReversed(t *testing.T) {
	lines := []string{"a", "b", "c", "d"}
	if got := selectedText(lines, 3, 1); got != "b\nc\nd" {
		t.Errorf("got %q want %q", got, "b\nc\nd")
	}
}

func TestSelectedTextClampHigh(t *testing.T) {
	lines := []string{"a", "b"}
	if got := selectedText(lines, 0, 99); got != "a\nb" {
		t.Errorf("got %q want %q", got, "a\nb")
	}
}

func TestSelectedTextClampLow(t *testing.T) {
	lines := []string{"a", "b"}
	if got := selectedText(lines, -5, 0); got != "a" {
		t.Errorf("got %q want %q", got, "a")
	}
}

func TestSelectedTextEmpty(t *testing.T) {
	if got := selectedText(nil, 0, 3); got != "" {
		t.Errorf("got %q want empty", got)
	}
}

func TestSplitLines(t *testing.T) {
	if got := splitLines("a\nb\n"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got %v want [a b]", got)
	}
	if got := splitLines(""); got != nil {
		t.Errorf("got %v want nil", got)
	}
}

func TestDecorateLinesCursor(t *testing.T) {
	lines := []string{"one", "two", "three"}
	out := decorateLines(lines, 1, 0, 1, "<S>", "<C>", "[R]")
	// band disabled (from>to); only cursor line 1 decorated
	if out[0] != "one" {
		t.Errorf("line0 = %q want unchanged", out[0])
	}
	if out[1] != "<C>two[R]" {
		t.Errorf("line1 = %q want cursor-decorated", out[1])
	}
	if out[2] != "three" {
		t.Errorf("line2 = %q want unchanged", out[2])
	}
}

func TestDecorateLinesSelectionBand(t *testing.T) {
	lines := []string{"a", "b", "c", "d"}
	// selection 1..2, cursor at 2 (cursor takes precedence on its line)
	out := decorateLines(lines, 1, 2, 2, "<S>", "<C>", "[R]")
	if out[0] != "a" {
		t.Errorf("line0 = %q want unchanged", out[0])
	}
	if out[1] != "<S>b[R]" {
		t.Errorf("line1 = %q want selection", out[1])
	}
	if out[2] != "<C>c[R]" {
		t.Errorf("line2 = %q want cursor", out[2])
	}
	if out[3] != "d" {
		t.Errorf("line3 = %q want unchanged", out[3])
	}
}

func TestDecorateLinesReinjectsBackgroundAfterReset(t *testing.T) {
	// a line containing an embedded reset (from a search marker) must keep the
	// line background after the reset.
	lines := []string{"pre[R]match[R]post"}
	out := decorateLines(lines, 1, 0, 0, "<S>", "<C>", "[R]")
	want := "<C>pre[R]<C>match[R]<C>post[R]"
	if out[0] != want {
		t.Errorf("got %q want %q", out[0], want)
	}
}
