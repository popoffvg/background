package adapter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	dimStart = "\x1b[2;3m"
	dimEnd   = "\x1b[0m"
)

// Renderer turns a tool's structured stdout stream into pretty text.
type Renderer interface {
	Render(r io.Reader, w io.Writer) error
}

var registry = map[string]Renderer{
	"claude-stream-json": claudeStreamJSON{},
	"opencode-json":      opencodeJSON{},
	"raw":                rawRenderer{},
}

// GetRenderer returns the named renderer, falling back to raw.
func GetRenderer(name string) Renderer {
	if r, ok := registry[name]; ok {
		return r
	}
	return rawRenderer{}
}

func dim(s string) string {
	return dimStart + s + dimEnd
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// rawRenderer copies bytes through unchanged.
type rawRenderer struct{}

func (rawRenderer) Render(r io.Reader, w io.Writer) error {
	_, err := io.Copy(w, r)
	return err
}

func newScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	return sc
}

// ---- claude-stream-json ----

type claudeStreamJSON struct{}

func (claudeStreamJSON) Render(r io.Reader, w io.Writer) error {
	sc := newScanner(r)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			// garbled line: print raw
			fmt.Fprintln(w, string(line))
			continue
		}
		renderClaudeObj(obj, w)
	}
	return sc.Err()
}

func renderClaudeObj(obj map[string]any, w io.Writer) {
	typ, _ := obj["type"].(string)
	switch typ {
	case "system":
		if sid, ok := obj["session_id"].(string); ok && sid != "" {
			fmt.Fprintln(w, dim("  session "+sid))
		}
	case "assistant":
		msg, _ := obj["message"].(map[string]any)
		content, _ := msg["content"].([]any)
		for _, c := range content {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			switch cm["type"] {
			case "text":
				if t, ok := cm["text"].(string); ok {
					fmt.Fprintln(w, t)
				}
			case "thinking":
				if t, ok := cm["thinking"].(string); ok {
					for _, ln := range strings.Split(strings.TrimRight(t, "\n"), "\n") {
						fmt.Fprintln(w, dim("  💭 "+ln))
					}
				}
			case "tool_use":
				name, _ := cm["name"].(string)
				input, _ := cm["input"].(map[string]any)
				preview := toolPreview(name, input)
				fmt.Fprintln(w, "  ▸ "+name+": "+preview)
			}
		}
	case "user":
		msg, _ := obj["message"].(map[string]any)
		content, _ := msg["content"].([]any)
		for _, c := range content {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			if cm["type"] == "tool_result" {
				res := stringifyContent(cm["content"])
				if res != "" {
					fmt.Fprintln(w, dim("  ◂ "+truncate(res, 200)))
				}
			}
		}
	case "result":
		var b strings.Builder
		b.WriteString("  ── done")
		if d, ok := obj["duration_ms"].(float64); ok {
			b.WriteString(fmt.Sprintf(" · %dms", int(d)))
		}
		if cost, ok := obj["total_cost_usd"].(float64); ok {
			b.WriteString(fmt.Sprintf(" · $%.4f", cost))
		}
		fmt.Fprintln(w, dim(b.String()))
	}
}

func toolPreview(name string, input map[string]any) string {
	if input == nil {
		return ""
	}
	if name == "Bash" {
		if cmd, ok := input["command"].(string); ok {
			return truncate(cmd, 80)
		}
	}
	b, _ := json.Marshal(input)
	return truncate(string(b), 80)
}

func stringifyContent(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var parts []string
		for _, e := range t {
			if em, ok := e.(map[string]any); ok {
				if s, ok := em["text"].(string); ok {
					parts = append(parts, s)
				}
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// ---- opencode-json ----

type opencodeJSON struct{}

func (opencodeJSON) Render(r io.Reader, w io.Writer) error {
	sc := newScanner(r)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			fmt.Fprintln(w, string(line))
			continue
		}
		renderOpencodeObj(obj, w)
	}
	return sc.Err()
}

func renderOpencodeObj(obj map[string]any, w io.Writer) {
	typ, _ := obj["type"].(string)
	part, _ := obj["part"].(map[string]any)
	switch typ {
	case "step_start":
		return
	case "reasoning":
		if part != nil {
			if t, ok := part["text"].(string); ok {
				for _, ln := range strings.Split(strings.TrimRight(t, "\n"), "\n") {
					fmt.Fprintln(w, dim("  💭 "+ln))
				}
			}
		}
	case "text":
		if part != nil {
			if t, ok := part["text"].(string); ok {
				fmt.Fprintln(w, t)
			}
		}
	case "tool":
		if part != nil {
			tool, _ := part["tool"].(string)
			if tool != "" {
				line := "  ▸ " + tool
				if state, ok := part["state"].(map[string]any); ok {
					if title, ok := state["title"].(string); ok && title != "" {
						line += ": " + truncate(title, 80)
					} else if status, ok := state["status"].(string); ok && status != "" {
						line += " (" + status + ")"
					}
				}
				fmt.Fprintln(w, line)
			}
		}
	case "step_finish":
		var b strings.Builder
		b.WriteString("  ── done")
		if part != nil {
			if tokens, ok := part["tokens"].(map[string]any); ok {
				in, _ := tokens["input"].(float64)
				out, _ := tokens["output"].(float64)
				b.WriteString(fmt.Sprintf(" · in %d out %d", int(in), int(out)))
			}
			if cost, ok := part["cost"].(float64); ok {
				b.WriteString(fmt.Sprintf(" · $%.4f", cost))
			}
		}
		fmt.Fprintln(w, dim(b.String()))
	}
}
