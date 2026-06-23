package tui

import (
	"errors"
	"reflect"
	"testing"
)

func TestClipboardCommandDarwin(t *testing.T) {
	got := clipboardCommand("darwin", func(string) (string, error) { return "/bin/pbcopy", nil })
	if !reflect.DeepEqual(got, []string{"pbcopy"}) {
		t.Fatalf("got %v want [pbcopy]", got)
	}
}

func TestClipboardCommandDarwinMissing(t *testing.T) {
	got := clipboardCommand("darwin", func(string) (string, error) { return "", errors.New("nope") })
	if got != nil {
		t.Fatalf("got %v want nil", got)
	}
}

func TestClipboardCommandLinuxFirstAvailable(t *testing.T) {
	look := func(name string) (string, error) {
		switch name {
		case "xclip":
			return "/usr/bin/xclip", nil
		default:
			return "", errors.New("nope")
		}
	}
	got := clipboardCommand("linux", look)
	want := []string{"xclip", "-selection", "clipboard"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestClipboardCommandLinuxNone(t *testing.T) {
	got := clipboardCommand("linux", func(string) (string, error) { return "", errors.New("nope") })
	if got != nil {
		t.Fatalf("got %v want nil", got)
	}
}
