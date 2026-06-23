package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogTailIncremental(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	if err := os.WriteFile(path, []byte("line1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tail := newLogTail(path)
	if tail.content != "line1\n" {
		t.Fatalf("seed content = %q", tail.content)
	}
	// append
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("line2\n")
	f.Close()
	if !tail.poll() {
		t.Fatal("poll should report change")
	}
	if tail.content != "line1\nline2\n" {
		t.Fatalf("after append content = %q", tail.content)
	}
	// no change
	if tail.poll() {
		t.Fatal("poll should report no change")
	}
}

func TestLogTailTruncationResets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	os.WriteFile(path, []byte("aaaa\nbbbb\n"), 0o644)
	tail := newLogTail(path)
	if tail.offset != 10 {
		t.Fatalf("offset = %d want 10", tail.offset)
	}
	// truncate to smaller
	os.WriteFile(path, []byte("x\n"), 0o644)
	if !tail.poll() {
		t.Fatal("poll should report change after truncation")
	}
	if tail.content != "x\n" {
		t.Fatalf("after truncation content = %q", tail.content)
	}
	if tail.offset != 2 {
		t.Fatalf("offset after truncation = %d want 2", tail.offset)
	}
}

func TestLogTailSeedFromTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	// write > 64KB so seed drops the front
	big := make([]byte, 0, 70*1024)
	for i := 0; i < 70*1024/6; i++ {
		big = append(big, []byte("xxxxx\n")...)
	}
	os.WriteFile(path, big, 0o644)
	tail := newLogTail(path)
	if int64(len(tail.content)) >= int64(len(big)) {
		t.Fatalf("seed should be truncated to tail; got %d of %d", len(tail.content), len(big))
	}
	if tail.offset != int64(len(big)) {
		t.Fatalf("offset = %d want %d", tail.offset, len(big))
	}
}
