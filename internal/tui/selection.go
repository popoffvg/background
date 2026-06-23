package tui

import "strings"

// selectedText returns the inclusive range of lines [a..b] joined by "\n".
// a and b are clamped into [0, len(lines)-1] and reordered if a > b. An empty
// lines slice yields "".
func selectedText(lines []string, a, b int) string {
	if len(lines) == 0 {
		return ""
	}
	if a > b {
		a, b = b, a
	}
	if a < 0 {
		a = 0
	}
	if b > len(lines)-1 {
		b = len(lines) - 1
	}
	return strings.Join(lines[a:b+1], "\n")
}

// splitLines splits content into lines without a trailing empty element for a
// final newline (so line indices match what the user sees).
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	s := strings.TrimSuffix(content, "\n")
	return strings.Split(s, "\n")
}

// decorateLines applies a background to lines in [from..to] (inclusive) and a
// distinct background to the cursor line. It re-injects the line background SGR
// after any ANSI reset within the line so search-match highlighting stays
// visible while the line background persists. lineBG/cursorBG are SGR prefixes
// (e.g. "\x1b[48;5;236m"); reset is "\x1b[0m".
//
// from/to define the visual-selection band; pass from>to (e.g. from=1,to=0) to
// disable the band. cursor is the cursor line index, or -1 to disable.
func decorateLines(lines []string, from, to, cursor int, lineBG, cursorBG, reset string) []string {
	out := make([]string, len(lines))
	for i, ln := range lines {
		var bg string
		switch {
		case cursor == i && cursorBG != "":
			bg = cursorBG
		case from <= to && i >= from && i <= to && lineBG != "":
			bg = lineBG
		}
		if bg == "" {
			out[i] = ln
			continue
		}
		// Persist the background across any embedded resets (from search markers).
		body := strings.ReplaceAll(ln, reset, reset+bg)
		out[i] = bg + body + reset
	}
	return out
}
