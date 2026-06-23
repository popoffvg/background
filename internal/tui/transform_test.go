package tui

import (
	"reflect"
	"testing"
)

func TestStripCommandBuffer(t *testing.T) {
	in := "# comment\n\n  # indented comment\nwc -l \"$FILE\"\n\n"
	if got := stripCommandBuffer(in); got != `wc -l "$FILE"` {
		t.Fatalf("got %q", got)
	}
}

func TestStripCommandBufferMultiline(t *testing.T) {
	in := "# c\nline one\nline two\n"
	if got := stripCommandBuffer(in); got != "line one\nline two" {
		t.Fatalf("got %q", got)
	}
}

func TestStripCommandBufferEmpty(t *testing.T) {
	in := "# only\n\n  # comments\n"
	if got := stripCommandBuffer(in); got != "" {
		t.Fatalf("got %q want empty", got)
	}
}

func TestEditorArgvDefault(t *testing.T) {
	if got := editorArgv(""); !reflect.DeepEqual(got, []string{"vi"}) {
		t.Fatalf("got %v want [vi]", got)
	}
}

func TestEditorArgvWithFlags(t *testing.T) {
	if got := editorArgv("code -w"); !reflect.DeepEqual(got, []string{"code", "-w"}) {
		t.Fatalf("got %v want [code -w]", got)
	}
}
