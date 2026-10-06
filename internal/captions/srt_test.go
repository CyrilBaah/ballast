package captions

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

const pieceSRT = `1
00:00:00,000 --> 00:00:04,500
Today is the first Sunday of October.

2
00:00:04,500 --> 00:00:09,000
 Romans chapter 8 and verse 28.

3
00:00:09,000 --> 00:00:10,000
[BLANK_AUDIO]
`

func TestParseSRT(t *testing.T) {
	cues, err := ParseSRT(strings.NewReader(pieceSRT))
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if len(cues) != 3 {
		t.Fatalf("got %d cues, want 3", len(cues))
	}
	if cues[1].Start != 4500*time.Millisecond || cues[1].End != 9*time.Second || cues[1].Text != "Romans chapter 8 and verse 28." {
		t.Fatalf("cue 2 = %+v", cues[1])
	}
}

func TestParseSRTRejectsGarbage(t *testing.T) {
	if _, err := ParseSRT(strings.NewReader("1\nnot a timestamp\nhello\n")); err == nil {
		t.Fatal("ParseSRT accepted a cue with no timestamp line")
	}
}

func TestCleanDropsNonSpeechAndShifts(t *testing.T) {
	cues, _ := ParseSRT(strings.NewReader(pieceSRT))
	got := Clean(Shift(cues, 10*time.Minute))
	if len(got) != 2 {
		t.Fatalf("Clean kept %d cues, want the 2 spoken ones: %+v", len(got), got)
	}
	if got[0].Start != 10*time.Minute || got[1].End != 10*time.Minute+9*time.Second {
		t.Fatalf("shifted cues = %+v", got)
	}
}

func TestCleanCollapsesRepeatedCues(t *testing.T) {
	// The real 43-minute sermon looped one line for minutes (research.md §5).
	var cues []Cue
	for i := 0; i < 6; i++ {
		cues = append(cues, Cue{Start: time.Duration(i) * 2 * time.Second, End: time.Duration(i+1) * 2 * time.Second, Text: "I wonder what will happen to you?"})
	}
	cues = append(cues, Cue{Start: 12 * time.Second, End: 14 * time.Second, Text: "Joseph became a prime minister."})
	got := Clean(cues)
	if len(got) != 2 {
		t.Fatalf("Clean = %+v, want the loop collapsed to one cue plus the next line", got)
	}
	if got[0].Start != 0 || got[0].End != 12*time.Second {
		t.Fatalf("collapsed cue spans %v–%v, want 0s–12s", got[0].Start, got[0].End)
	}
}

func TestCleanKeepsTwoGenuineRepeats(t *testing.T) {
	cues := []Cue{
		{0, time.Second, "Amen."},
		{time.Second, 2 * time.Second, "Amen."},
		{2 * time.Second, 3 * time.Second, "Hallelujah."},
	}
	if got := Clean(cues); len(got) != 3 {
		t.Fatalf("Clean = %+v; two identical cues in a row are normal speech and must stay", got)
	}
}

func TestCleanCollapsesPhraseLoopInsideOneCue(t *testing.T) {
	text := "They were going around, " + strings.Repeat("because they told us about it, ", 8) + "and then they left."
	got := Clean([]Cue{{0, 30 * time.Second, text}})
	if n := strings.Count(got[0].Text, "because they told us about it"); n != 1 {
		t.Fatalf("phrase appears %d times after Clean, want 1: %q", n, got[0].Text)
	}
	if !strings.HasPrefix(got[0].Text, "They were going around,") || !strings.HasSuffix(got[0].Text, "and then they left.") {
		t.Fatalf("Clean lost the surrounding words: %q", got[0].Text)
	}
}

func TestWriteSRTRenumbersAndFormats(t *testing.T) {
	cues := []Cue{
		{0, 1500 * time.Millisecond, "First."},
		{time.Hour + 2*time.Minute + 3*time.Second + 45*time.Millisecond, time.Hour + 2*time.Minute + 5*time.Second, "Second."},
	}
	var buf bytes.Buffer
	if err := WriteSRT(&buf, cues); err != nil {
		t.Fatal(err)
	}
	want := "1\n00:00:00,000 --> 00:00:01,500\nFirst.\n\n2\n01:02:03,045 --> 01:02:05,000\nSecond.\n\n"
	if buf.String() != want {
		t.Fatalf("WriteSRT =\n%q\nwant\n%q", buf.String(), want)
	}
	back, err := ParseSRT(&buf)
	if err != nil || len(back) != 2 || back[1].Start != cues[1].Start {
		t.Fatalf("round trip = %+v, %v", back, err)
	}
}

func TestHasSpeech(t *testing.T) {
	if HasSpeech(nil) {
		t.Error("HasSpeech(nil) = true")
	}
	if !HasSpeech([]Cue{{0, time.Second, "Hello."}}) {
		t.Error("HasSpeech with a spoken cue = false")
	}
}
