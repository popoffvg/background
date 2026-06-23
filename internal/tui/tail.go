package tui

import (
	"bytes"
	"io"
	"os"
	"strings"
)

const (
	tailSeedBytes  = 64 * 1024  // initial seed: last 64KB
	maxRetainBytes = 512 * 1024 // cap retained content
	maxRetainLines = 8000
)

// logTail tracks incremental reads of a single log file.
type logTail struct {
	path    string
	offset  int64
	content string
}

// newLogTail seeds from the tail (~64KB) of path and sets offset to EOF.
func newLogTail(path string) *logTail {
	t := &logTail{path: path}
	t.seed()
	return t
}

func (t *logTail) seed() {
	f, err := os.Open(t.path)
	if err != nil {
		t.content = ""
		t.offset = 0
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return
	}
	size := info.Size()
	start := int64(0)
	if size > tailSeedBytes {
		start = size - tailSeedBytes
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return
	}
	data, _ := io.ReadAll(f)
	// If we seeked into the middle of a line, drop the partial first line.
	if start > 0 {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	t.content = string(data)
	t.offset = size
	t.trim()
}

// poll reads any bytes appended since the last read. Returns true if content
// changed. Handles truncation/rotation by resetting when size < offset.
func (t *logTail) poll() bool {
	info, err := os.Stat(t.path)
	if err != nil {
		return false
	}
	size := info.Size()
	if size < t.offset {
		// truncated/rotated: reload from scratch
		t.offset = 0
		t.content = ""
		t.seedFromZero()
		return true
	}
	if size == t.offset {
		return false
	}
	f, err := os.Open(t.path)
	if err != nil {
		return false
	}
	defer f.Close()
	if _, err := f.Seek(t.offset, io.SeekStart); err != nil {
		return false
	}
	delta, _ := io.ReadAll(f)
	if len(delta) == 0 {
		return false
	}
	t.content += string(delta)
	t.offset = size
	t.trim()
	return true
}

// seedFromZero reloads whole file (used after truncation; file is small now).
func (t *logTail) seedFromZero() {
	data, err := os.ReadFile(t.path)
	if err != nil {
		return
	}
	t.content = string(data)
	t.offset = int64(len(data))
	t.trim()
}

// trim caps retained content to bounded size/lines, dropping from the front.
func (t *logTail) trim() {
	if len(t.content) > maxRetainBytes {
		t.content = t.content[len(t.content)-maxRetainBytes:]
		if i := strings.IndexByte(t.content, '\n'); i >= 0 {
			t.content = t.content[i+1:]
		}
	}
	lines := strings.Count(t.content, "\n")
	if lines > maxRetainLines {
		// drop oldest lines
		idx := 0
		drop := lines - maxRetainLines
		for drop > 0 {
			j := strings.IndexByte(t.content[idx:], '\n')
			if j < 0 {
				break
			}
			idx += j + 1
			drop--
		}
		t.content = t.content[idx:]
	}
}
