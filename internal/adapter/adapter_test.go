package adapter_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/popoffvg/background/internal/adapter"
)

func TestMatchClaude(t *testing.T) {
	a := adapter.Match("claude", []string{"-p", "hi"})
	if a == nil {
		t.Fatal("expected claude adapter, got nil")
	}
	if a.Name != "claude" {
		t.Fatalf("name = %q, want claude", a.Name)
	}
	found := false
	for _, in := range a.Inject {
		if strings.Contains(in, "stream-json") {
			found = true
		}
	}
	if !found {
		t.Fatalf("inject %v missing stream-json", a.Inject)
	}
}

func TestMatchNoArgs(t *testing.T) {
	if a := adapter.Match("claude", []string{"hi"}); a != nil {
		t.Fatalf("expected nil, got %+v", a)
	}
}

func TestMatchUnknownBin(t *testing.T) {
	if a := adapter.Match("ls", []string{"-p"}); a != nil {
		t.Fatalf("expected nil, got %+v", a)
	}
}

func TestMatchBasePath(t *testing.T) {
	a := adapter.Match("/usr/local/bin/claude", []string{"--print"})
	if a == nil || a.Name != "claude" {
		t.Fatalf("expected claude adapter, got %+v", a)
	}
}

func TestInjectionShape(t *testing.T) {
	a := adapter.Match("claude", []string{"-p"})
	if a == nil {
		t.Fatal("expected claude adapter")
	}
	want := []string{"--output-format", "stream-json", "--verbose"}
	if !reflect.DeepEqual(a.Inject, want) {
		t.Fatalf("inject = %v, want %v", a.Inject, want)
	}
}
