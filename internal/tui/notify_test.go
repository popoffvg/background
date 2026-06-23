package tui

import (
	"strings"
	"testing"
)

func TestGhosttyNotifyShape(t *testing.T) {
	got := ghosttyNotify("bg", "npm build — exited(0)")
	want := "\x1b]777;notify;bg;npm build — exited(0)\x07"
	if got != want {
		t.Errorf("ghosttyNotify = %q, want %q", got, want)
	}
}

func TestGhosttyNotifySanitizes(t *testing.T) {
	// title/body containing ESC, BEL, ';' and a control char must not break the
	// sequence: exactly the three structural ';' separators remain.
	got := ghosttyNotify("ti;tle\x1b", "bo\x07dy;\x01here")
	// Structural separators: after 777, after notify, between title and body = 3.
	if c := strings.Count(got, ";"); c != 3 {
		t.Errorf("expected 3 structural ';', got %d in %q", c, got)
	}
	if strings.ContainsAny(got[len("\x1b]"):len(got)-1], "\x07") {
		t.Errorf("body BEL not stripped: %q", got)
	}
	if strings.Contains(got[2:len(got)-1], "\x01") {
		t.Errorf("control char not stripped: %q", got)
	}
	// the injected ESC in the title must be gone (only the leading OSC ESC remains)
	if strings.Count(got, "\x1b") != 1 {
		t.Errorf("expected 1 ESC (the OSC introducer), got %d", strings.Count(got, "\x1b"))
	}
}

func TestGhosttyNotifyTruncatesBody(t *testing.T) {
	body := strings.Repeat("x", 500)
	got := ghosttyNotify("bg", body)
	// extract body between the last ';' and the BEL
	inner := strings.TrimSuffix(strings.TrimPrefix(got, "\x1b]777;notify;bg;"), "\x07")
	if len([]rune(inner)) != 200 {
		t.Errorf("body length = %d, want 200", len([]rune(inner)))
	}
}

func TestShouldNotify(t *testing.T) {
	cases := []struct {
		prev, next string
		want       bool
	}{
		{"running", "ok", true},
		{"running", "fail", true},
		{"running", "running", false},
		{"running", "unknown", false},
		{"ok", "ok", false},
		{"", "ok", false},        // first-seen handled elsewhere; not a transition
		{"unknown", "ok", false}, // wasn't running
	}
	for _, c := range cases {
		if got := shouldNotify(c.prev, c.next); got != c.want {
			t.Errorf("shouldNotify(%q,%q) = %v, want %v", c.prev, c.next, got, c.want)
		}
	}
}

func TestNotifyEnabled(t *testing.T) {
	t.Setenv("BG_NOTIFY", "")
	if !notifyEnabled() {
		t.Errorf("default should be enabled")
	}
	t.Setenv("BG_NOTIFY", "0")
	if notifyEnabled() {
		t.Errorf("BG_NOTIFY=0 should disable")
	}
	t.Setenv("BG_NOTIFY", "false")
	if notifyEnabled() {
		t.Errorf("BG_NOTIFY=false should disable")
	}
	t.Setenv("BG_NOTIFY", "1")
	if !notifyEnabled() {
		t.Errorf("BG_NOTIFY=1 should remain enabled")
	}
}

func TestNotifyForced(t *testing.T) {
	t.Setenv("BG_NOTIFY", "1")
	if !notifyForced() {
		t.Errorf("BG_NOTIFY=1 should force")
	}
	t.Setenv("BG_NOTIFY", "true")
	if !notifyForced() {
		t.Errorf("BG_NOTIFY=true should force")
	}
	t.Setenv("BG_NOTIFY", "")
	if notifyForced() {
		t.Errorf("unset should not force")
	}
}

func TestIsGhostty(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("GHOSTTY_RESOURCES_DIR", "")
	t.Setenv("GHOSTTY_BIN_DIR", "")
	if isGhostty() {
		t.Errorf("no ghostty env should be false")
	}
	t.Setenv("TERM_PROGRAM", "ghostty")
	if !isGhostty() {
		t.Errorf("TERM_PROGRAM=ghostty should be true")
	}
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("GHOSTTY_RESOURCES_DIR", "/x")
	if !isGhostty() {
		t.Errorf("GHOSTTY_RESOURCES_DIR present should be true")
	}
}

func TestNotifyAllowed(t *testing.T) {
	// disabled overrides everything
	t.Setenv("BG_NOTIFY", "0")
	t.Setenv("TERM_PROGRAM", "ghostty")
	if notifyAllowed() {
		t.Errorf("disabled should never allow")
	}
	// forced even without ghostty
	t.Setenv("BG_NOTIFY", "1")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("GHOSTTY_RESOURCES_DIR", "")
	t.Setenv("GHOSTTY_BIN_DIR", "")
	if !notifyAllowed() {
		t.Errorf("forced should allow without ghostty")
	}
	// default-on + ghostty detected
	t.Setenv("BG_NOTIFY", "")
	t.Setenv("TERM_PROGRAM", "ghostty")
	if !notifyAllowed() {
		t.Errorf("default+ghostty should allow")
	}
	// default-on but not ghostty and not forced → no
	t.Setenv("TERM_PROGRAM", "")
	if notifyAllowed() {
		t.Errorf("default+non-ghostty should not allow")
	}
}
