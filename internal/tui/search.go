package tui

import "strings"

// matchPos is a byte-offset span [Start,End) of a match in the content.
type matchPos struct {
	Start int
	End   int
}

// findMatches returns all case-insensitive substring matches of query in
// content, as byte spans, in order. Empty query returns nil.
func findMatches(content, query string) []matchPos {
	if query == "" {
		return nil
	}
	lc := strings.ToLower(content)
	lq := strings.ToLower(query)
	var out []matchPos
	from := 0
	for {
		i := strings.Index(lc[from:], lq)
		if i < 0 {
			break
		}
		start := from + i
		out = append(out, matchPos{Start: start, End: start + len(lq)})
		from = start + len(lq)
		if from > len(lc) {
			break
		}
	}
	return out
}

// lineOf returns the zero-based line number containing byte offset off.
func lineOf(content string, off int) int {
	if off > len(content) {
		off = len(content)
	}
	return strings.Count(content[:off], "\n")
}

// highlightMatches wraps each match span in content with pre/post markers.
// The match at curIdx uses curPre/curPost instead, so the active match is
// styled distinctly. Pass curIdx < 0 to style all matches the same.
func highlightMatches(content string, matches []matchPos, pre, post string, curIdx int, curPre, curPost string) string {
	if len(matches) == 0 {
		return content
	}
	var b strings.Builder
	last := 0
	for i, m := range matches {
		if m.Start < last || m.End > len(content) {
			continue
		}
		p, q := pre, post
		if i == curIdx {
			p, q = curPre, curPost
		}
		b.WriteString(content[last:m.Start])
		b.WriteString(p)
		b.WriteString(content[m.Start:m.End])
		b.WriteString(q)
		last = m.End
	}
	b.WriteString(content[last:])
	return b.String()
}
