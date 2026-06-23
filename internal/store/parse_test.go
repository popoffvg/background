package store_test

import (
	"testing"
	"time"

	"github.com/popoffvg/background/internal/store"
)

func TestParseDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"14d", 14 * 24 * time.Hour},
		{"72h", 72 * time.Hour},
		{"30m", 30 * time.Minute},
		{"1.5d", 36 * time.Hour},
	}
	for _, c := range cases {
		got, err := store.ParseDuration(c.in)
		if err != nil {
			t.Errorf("ParseDuration(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if _, err := store.ParseDuration("nope"); err == nil {
		t.Errorf("ParseDuration(nope) should error")
	}
}

func TestParseBytes(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"500", 500},
		{"500B", 500},
		{"1K", 1024},
		{"1KB", 1024},
		{"2MB", 2 * 1024 * 1024},
		{"500MB", 500 * 1024 * 1024},
		{"1GB", 1024 * 1024 * 1024},
	}
	for _, c := range cases {
		got, err := store.ParseBytes(c.in)
		if err != nil {
			t.Errorf("ParseBytes(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseBytes(%q) = %d, want %d", c.in, got, c.want)
		}
	}
	if _, err := store.ParseBytes("xMB"); err == nil {
		t.Errorf("ParseBytes(xMB) should error")
	}
}
