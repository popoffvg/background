package runner

import (
	"bytes"
	"strings"
	"testing"
)

func TestCappedWriterTruncates(t *testing.T) {
	var buf bytes.Buffer
	cap := int64(100)
	cw := newCappedWriter(&buf, cap)

	// Write 1KB in 10-byte chunks.
	chunk := bytes.Repeat([]byte("x"), 10)
	for i := 0; i < 100; i++ {
		n, err := cw.Write(chunk)
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
		if n != len(chunk) {
			t.Fatalf("Write returned %d, want %d (caller must see full write)", n, len(chunk))
		}
	}

	out := buf.String()
	// Output ≈ cap bytes of data + one marker.
	if int64(strings.Count(out, "x")) != cap {
		t.Errorf("kept %d data bytes, want %d", strings.Count(out, "x"), cap)
	}
	if c := strings.Count(out, "log truncated"); c != 1 {
		t.Errorf("marker appears %d times, want exactly 1", c)
	}
	if !strings.Contains(out, "— log truncated at 0MB —") {
		// cap=100 bytes → 0 MB; marker still present with the MB figure.
		t.Errorf("marker text missing/unexpected: %q", out)
	}

	// Further writes are no-ops (no second marker, no growth).
	before := buf.Len()
	n, err := cw.Write([]byte("more data"))
	if err != nil || n != len("more data") {
		t.Fatalf("post-cap Write: n=%d err=%v", n, err)
	}
	if buf.Len() != before {
		t.Errorf("buffer grew after cap: before=%d after=%d", before, buf.Len())
	}
	if c := strings.Count(buf.String(), "log truncated"); c != 1 {
		t.Errorf("marker appears %d times after extra writes, want 1", c)
	}
}

func TestCappedWriterUnderCap(t *testing.T) {
	var buf bytes.Buffer
	cw := newCappedWriter(&buf, 1000)
	data := bytes.Repeat([]byte("y"), 500)
	if _, err := cw.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if buf.String() != string(data) {
		t.Errorf("under-cap write altered output")
	}
	if strings.Contains(buf.String(), "truncated") {
		t.Errorf("marker emitted under cap")
	}
}
