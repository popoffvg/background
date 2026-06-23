package runner

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
)

// DefaultMaxLogBytes caps how many bytes a single task may write to out.log.
const DefaultMaxLogBytes int64 = 50 * 1024 * 1024 // 50 MB

// maxLogBytes returns the per-task log cap, honoring BG_MAX_LOG_BYTES (0 or
// unset = default).
func maxLogBytes() int64 {
	if v := os.Getenv("BG_MAX_LOG_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return DefaultMaxLogBytes
}

// cappedWriter wraps an io.Writer and stops writing once a byte cap is reached,
// emitting a single truncation marker. It is safe for concurrent use, so the
// child's stdout/stderr and the renderer goroutine can share one cap (the cap is
// a total across all of them). It never seeks or rewrites — fine for an append
// fd held by the supervisor.
type cappedWriter struct {
	mu       sync.Mutex
	w        io.Writer
	cap      int64
	written  int64
	finished bool // marker already emitted
}

func newCappedWriter(w io.Writer, capBytes int64) *cappedWriter {
	return &cappedWriter{w: w, cap: capBytes}
}

// Write writes up to the remaining budget, then emits the marker once and
// discards everything after. It reports len(p) written to the caller so the
// child process never sees a short write / EPIPE-style error.
func (c *cappedWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.finished {
		return len(p), nil
	}

	remaining := c.cap - c.written
	if int64(len(p)) <= remaining {
		n, err := c.w.Write(p)
		c.written += int64(n)
		if err != nil {
			return n, err
		}
		return len(p), nil
	}

	// Write what fits, then the marker, then stop.
	if remaining > 0 {
		n, err := c.w.Write(p[:remaining])
		c.written += int64(n)
		if err != nil {
			return len(p), err
		}
	}
	marker := fmt.Sprintf("\n— log truncated at %dMB —\n", c.cap/(1024*1024))
	_, _ = io.WriteString(c.w, marker)
	c.finished = true
	return len(p), nil
}
