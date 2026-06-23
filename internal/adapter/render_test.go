package adapter_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/popoffvg/background/internal/adapter"
)

func renderString(t *testing.T, name, input string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := adapter.GetRenderer(name).Render(strings.NewReader(input), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestClaudeRenderTextAndThinking(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}]}}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"let me think"}]}}` + "\n"
	out := renderString(t, "claude-stream-json", input)
	for _, want := range []string{"Hello world", "let me think", "\x1b[2;3m", "💭"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestClaudeRenderResult(t *testing.T) {
	out := renderString(t, "claude-stream-json", `{"type":"result","duration_ms":1234,"total_cost_usd":0.05}`+"\n")
	for _, want := range []string{"done", "1234ms", "$0.05"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestClaudeToolUse(t *testing.T) {
	out := renderString(t, "claude-stream-json", `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"ls -la"}}]}}`+"\n")
	for _, want := range []string{"▸ Bash:", "ls -la"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestOpencodeRender(t *testing.T) {
	input := `{"type":"reasoning","part":{"text":"thinking hard"}}` + "\n" +
		`{"type":"text","part":{"text":"final answer"}}` + "\n" +
		`{"type":"step_finish","part":{"tokens":{"input":10,"output":20},"cost":0.01}}` + "\n"
	out := renderString(t, "opencode-json", input)
	if !strings.Contains(out, "\x1b[2;3m") || !strings.Contains(out, "thinking hard") {
		t.Fatalf("reasoning not dimmed:\n%s", out)
	}
	if !strings.Contains(out, "final answer") {
		t.Fatalf("missing final answer:\n%s", out)
	}
	for _, want := range []string{"done", "in 10 out 20"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRendererMissingFields(t *testing.T) {
	if err := adapter.GetRenderer("claude-stream-json").Render(strings.NewReader(`{"type":"assistant"}`+"\n"+`{"type":"result"}`+"\n"), &bytes.Buffer{}); err != nil {
		t.Fatalf("render: %v", err)
	}
}

func TestRawRenderer(t *testing.T) {
	out := renderString(t, "raw", "anything\n")
	if out != "anything\n" {
		t.Fatalf("raw = %q, want %q", out, "anything\n")
	}
}

func TestGarbledLine(t *testing.T) {
	out := renderString(t, "claude-stream-json", "not json at all\n")
	if !strings.Contains(out, "not json at all") {
		t.Fatalf("output missing garbled line:\n%s", out)
	}
}
