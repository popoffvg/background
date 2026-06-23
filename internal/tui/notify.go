package tui

import (
	"os"
	"strings"
)

// notifyEnabled reports whether finish notifications are allowed. On by default;
// BG_NOTIFY=0 or =false disables.
func notifyEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BG_NOTIFY"))) {
	case "0", "false":
		return false
	}
	return true
}

// notifyForced reports whether BG_NOTIFY explicitly forces emission even when
// Ghostty isn't detected (useful for other OSC-777-aware terminals + tests).
func notifyForced() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BG_NOTIFY"))) {
	case "1", "true":
		return true
	}
	return false
}

// isGhostty reports whether the terminal looks like Ghostty.
func isGhostty() bool {
	if strings.EqualFold(os.Getenv("TERM_PROGRAM"), "ghostty") {
		return true
	}
	for _, k := range []string{"GHOSTTY_RESOURCES_DIR", "GHOSTTY_BIN_DIR"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}

// notifyAllowed is the net gate: emit when enabled AND (Ghostty detected OR
// explicitly forced).
func notifyAllowed() bool {
	return notifyEnabled() && (isGhostty() || notifyForced())
}

// shouldNotify reports whether a category transition warrants a notification:
// only running → finished (ok|fail).
func shouldNotify(prev, next string) bool {
	if prev != "running" {
		return false
	}
	return next == "ok" || next == "fail"
}

// sanitizeOSC strips characters that could break or escape an OSC sequence:
// ESC, BEL, ST, ';' (the field separator), and any other control characters.
func sanitizeOSC(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\x1b', '\x07', ';':
			b.WriteRune(' ')
		default:
			if r < 0x20 || r == 0x7f {
				b.WriteRune(' ')
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// ghosttyNotify builds an OSC 777 desktop-notification escape sequence with a
// BEL terminator. Title and body are sanitized; body is truncated to ~200 runes.
func ghosttyNotify(title, body string) string {
	title = sanitizeOSC(title)
	body = sanitizeOSC(body)
	if r := []rune(body); len(r) > 200 {
		body = string(r[:200])
	}
	return "\x1b]777;notify;" + title + ";" + body + "\x07"
}
