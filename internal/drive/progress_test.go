package drive

import (
	"bytes"
	"io"
	"testing"
	"time"
)

func TestCountingReaderThrottlesEmitsWithinWindow(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 10)
	src := bytes.NewReader(data)

	var emits []int64
	fakeNow := time.Unix(0, 0)
	cr := newCountingReader(src, time.Second, func(bytesRead int64) {
		emits = append(emits, bytesRead)
	})
	cr.now = func() time.Time { return fakeNow }

	buf := make([]byte, 1)
	// Fake clock never advances, so only the first read should emit.
	for i := 0; i < 5; i++ {
		if _, err := cr.Read(buf); err != nil && err != io.EOF {
			t.Fatalf("Read #%d: %v", i, err)
		}
	}

	if len(emits) != 1 {
		t.Fatalf("got %d emits within a single throttle window, want 1: %v", len(emits), emits)
	}
	if emits[0] != 1 {
		t.Fatalf("first emit reported %d bytes, want 1", emits[0])
	}
}

func TestCountingReaderEmitsAgainAfterThrottleWindowElapses(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 10)
	src := bytes.NewReader(data)

	var emits []int64
	fakeNow := time.Unix(0, 0)
	cr := newCountingReader(src, time.Second, func(bytesRead int64) {
		emits = append(emits, bytesRead)
	})
	cr.now = func() time.Time { return fakeNow }

	buf := make([]byte, 1)
	_, _ = cr.Read(buf) // emit #1 at total=1

	fakeNow = fakeNow.Add(2 * time.Second) // advance past the throttle window
	_, _ = cr.Read(buf)                    // emit #2 at total=2

	if len(emits) != 2 {
		t.Fatalf("got %d emits, want 2 (one per throttle window): %v", len(emits), emits)
	}
	if emits[0] != 1 || emits[1] != 2 {
		t.Fatalf("emits = %v, want [1 2] (non-decreasing cumulative bytes)", emits)
	}
}

func TestCountingReaderAlwaysEmitsOnFinalRead(t *testing.T) {
	data := []byte("hello")
	src := bytes.NewReader(data)

	var emits []int64
	fakeNow := time.Unix(0, 0)
	cr := newCountingReader(src, time.Hour, func(bytesRead int64) { // huge throttle window
		emits = append(emits, bytesRead)
	})
	cr.now = func() time.Time { return fakeNow }

	// Drain fully; the final io.EOF read must still emit despite the huge throttle window.
	buf := make([]byte, len(data))
	for {
		_, err := cr.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
	}

	if len(emits) == 0 {
		t.Fatal("expected at least one emit (the final one) even within one throttle window")
	}
	last := emits[len(emits)-1]
	if last != int64(len(data)) {
		t.Fatalf("final emit = %d, want %d (total bytes)", last, len(data))
	}
}

func TestCountingReaderBytesAreNonDecreasing(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 100)
	src := bytes.NewReader(data)

	var emits []int64
	fakeNow := time.Unix(0, 0)
	cr := newCountingReader(src, time.Millisecond, func(bytesRead int64) {
		emits = append(emits, bytesRead)
	})
	cr.now = func() time.Time {
		fakeNow = fakeNow.Add(time.Millisecond)
		return fakeNow
	}

	buf := make([]byte, 10)
	for {
		_, err := cr.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
	}

	prev := int64(0)
	for _, v := range emits {
		if v < prev {
			t.Fatalf("emits went backwards: %v", emits)
		}
		prev = v
	}
	if prev != int64(len(data)) {
		t.Fatalf("last emit = %d, want %d", prev, len(data))
	}
}
