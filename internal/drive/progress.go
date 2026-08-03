package drive

import (
	"io"
	"time"
)

// progressEmitThrottle bounds how often countingReader invokes onEmit during an active transfer.
const progressEmitThrottle = time.Second

// countingReader wraps an io.Reader, tracking cumulative bytes read and
// invoking onEmit with the running total. Emits are throttled to at most
// once per throttle duration, except the final read (io.EOF), which always emits.
type countingReader struct {
	src      io.Reader
	throttle time.Duration
	onEmit   func(bytesRead int64)

	total    int64
	lastEmit time.Time
	// now is overridable in tests; defaults to time.Now.
	now func() time.Time
}

func newCountingReader(src io.Reader, throttle time.Duration, onEmit func(bytesRead int64)) *countingReader {
	return &countingReader{
		src:      src,
		throttle: throttle,
		onEmit:   onEmit,
		now:      time.Now,
	}
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.src.Read(p)
	c.total += int64(n)

	now := c.now()
	if err == io.EOF || now.Sub(c.lastEmit) >= c.throttle {
		c.lastEmit = now
		c.onEmit(c.total)
	}
	return n, err
}
