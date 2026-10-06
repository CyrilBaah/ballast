package summaries

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func srtOf(cues ...string) string {
	var b strings.Builder
	for i, c := range cues {
		start := time.Duration(i) * 10 * time.Second
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, stamp(start), stamp(start+10*time.Second), c)
	}
	return b.String()
}

func stamp(d time.Duration) string {
	ms := d.Milliseconds()
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3_600_000, ms/60_000%60, ms/1000%60, ms%1000)
}

func TestFromSRTAndText(t *testing.T) {
	tr, err := FromSRT(strings.NewReader(srtOf("Welcome everyone.", "Turn to Romans 8.")))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := tr.Text(), "[00:00] Welcome everyone.\n[00:10] Turn to Romans 8.\n"; got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
	long := Transcript{Lines: []Line{{At: time.Hour + 2*time.Minute + 3*time.Second, Text: "Late."}}}
	if got := long.Text(); got != "[1:02:03] Late.\n" {
		t.Fatalf("Text() past an hour = %q", got)
	}
}

func TestPartsShortTranscriptIsOnePass(t *testing.T) {
	tr, _ := FromSRT(strings.NewReader(srtOf("a", "b", "c")))
	if parts := tr.Parts(); len(parts) != 1 || len(parts[0].Lines) != 3 {
		t.Fatalf("Parts() = %d parts, want 1 with all lines", len(parts))
	}
}

func TestPartsSplitsLongTranscriptOnCueBoundaries(t *testing.T) {
	// 90 minutes of 10-second cues, each long enough that the whole thing
	// is well over the single-pass budget.
	var lines []Line
	words := strings.Repeat("word ", 30)
	for at := time.Duration(0); at < 90*time.Minute; at += 10 * time.Second {
		lines = append(lines, Line{At: at, Text: words})
	}
	tr := Transcript{Lines: lines}
	if EstimateTokens(tr.Text()) <= SinglePassTokens {
		t.Fatal("test transcript isn't long enough")
	}
	parts := tr.Parts()
	if len(parts) != 5 { // 90 min / ~20 min
		t.Fatalf("Parts() = %d parts, want 5", len(parts))
	}
	total := 0
	for i, p := range parts {
		total += len(p.Lines)
		if i > 0 && p.Lines[0].At < parts[i-1].Lines[len(parts[i-1].Lines)-1].At {
			t.Fatalf("part %d starts before part %d ends", i, i-1)
		}
	}
	if total != len(lines) {
		t.Fatalf("parts hold %d lines, want all %d", total, len(lines))
	}
}

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens(strings.Repeat("abcd", 100)); got != 100 {
		t.Fatalf("EstimateTokens(400 chars) = %d, want 100", got)
	}
}
