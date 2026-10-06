package captions

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// Cue is one caption: a span of time and what was said in it.
type Cue struct {
	Start time.Duration
	End   time.Duration
	Text  string
}

// minRepeatRun is how many identical cues (or phrases within a cue) in a
// row count as the model looping rather than the speaker repeating
// themselves (research.md §5) -- "Amen. Amen." is speech; six identical
// lines in a row was the measured loop.
const minRepeatRun = 3

var timingLine = regexp.MustCompile(`^(\d{2}):(\d{2}):(\d{2})[,.](\d{3}) --> (\d{2}):(\d{2}):(\d{2})[,.](\d{3})`)

// nonSpeech matches cues that are only a bracketed marker such as
// [BLANK_AUDIO] or (music).
var nonSpeech = regexp.MustCompile(`^\s*(\[[^\]]*\]|\([^)]*\))\s*$`)

// ParseSRT reads SubRip cues.
func ParseSRT(r io.Reader) ([]Cue, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var cues []Cue
	var cur *Cue
	expectTiming := false
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\uFEFF"))
		switch {
		case text == "":
			if cur != nil {
				cues = append(cues, *cur)
				cur = nil
			}
			expectTiming = false
		case cur == nil && !expectTiming:
			expectTiming = true // the cue number
		case expectTiming:
			m := timingLine.FindStringSubmatch(text)
			if m == nil {
				return nil, fmt.Errorf("captions: SRT line %d: expected a timing line, got %q", line, text)
			}
			cur = &Cue{Start: stampOf(m[1:5]), End: stampOf(m[5:9])}
			expectTiming = false
		default:
			if cur.Text != "" {
				cur.Text += " "
			}
			cur.Text += text
		}
	}
	if cur != nil {
		cues = append(cues, *cur)
	}
	if expectTiming {
		return nil, fmt.Errorf("captions: SRT ends after a cue number with no timing line")
	}
	return cues, sc.Err()
}

func stampOf(p []string) time.Duration {
	var h, m, s, ms int
	fmt.Sscanf(p[0]+" "+p[1]+" "+p[2]+" "+p[3], "%d %d %d %d", &h, &m, &s, &ms)
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second + time.Duration(ms)*time.Millisecond
}

// Shift moves every cue later by offset -- a piece's start within the video.
func Shift(cues []Cue, offset time.Duration) []Cue {
	out := make([]Cue, len(cues))
	for i, c := range cues {
		out[i] = Cue{Start: c.Start + offset, End: c.End + offset, Text: c.Text}
	}
	return out
}

// Clean drops cues that aren't speech, collapses a phrase looped within
// one cue, and collapses runs of minRepeatRun or more identical cues into
// one spanning the whole run (research.md §5).
func Clean(cues []Cue) []Cue {
	var kept []Cue
	for _, c := range cues {
		c.Text = collapsePhraseLoop(strings.TrimSpace(c.Text))
		if c.Text == "" || nonSpeech.MatchString(c.Text) {
			continue
		}
		kept = append(kept, c)
	}

	var out []Cue
	for i := 0; i < len(kept); {
		j := i + 1
		for j < len(kept) && normalize(kept[j].Text) == normalize(kept[i].Text) {
			j++
		}
		if j-i >= minRepeatRun {
			out = append(out, Cue{Start: kept[i].Start, End: kept[j-1].End, Text: kept[i].Text})
		} else {
			out = append(out, kept[i:j]...)
		}
		i = j
	}
	return out
}

var clauseEnd = regexp.MustCompile(`[,.;!?]\s+`)

// collapsePhraseLoop keeps one copy of a clause repeated minRepeatRun or
// more times in a row within text.
func collapsePhraseLoop(text string) string {
	// Split into clauses, keeping each clause's trailing punctuation.
	var clauses []string
	last := 0
	for _, m := range clauseEnd.FindAllStringIndex(text, -1) {
		clauses = append(clauses, text[last:m[1]])
		last = m[1]
	}
	if last < len(text) {
		clauses = append(clauses, text[last:])
	}

	var out []string
	for i := 0; i < len(clauses); {
		j := i + 1
		for j < len(clauses) && normalize(clauses[j]) == normalize(clauses[i]) {
			j++
		}
		if j-i >= minRepeatRun {
			out = append(out, clauses[i])
		} else {
			out = append(out, clauses[i:j]...)
		}
		i = j
	}
	return strings.TrimSpace(strings.Join(out, ""))
}

var notWordChars = regexp.MustCompile(`[^a-z0-9 ]+`)

func normalize(s string) string {
	return strings.Join(strings.Fields(notWordChars.ReplaceAllString(strings.ToLower(s), " ")), " ")
}

// HasSpeech reports whether any cue has spoken text.
func HasSpeech(cues []Cue) bool {
	return len(Clean(cues)) > 0
}

// WriteSRT writes cues as SubRip, numbered from 1.
func WriteSRT(w io.Writer, cues []Cue) error {
	bw := bufio.NewWriter(w)
	for i, c := range cues {
		fmt.Fprintf(bw, "%d\n%s --> %s\n%s\n\n", i+1, srtStamp(c.Start), srtStamp(c.End), c.Text)
	}
	return bw.Flush()
}

func srtStamp(d time.Duration) string {
	ms := d.Milliseconds()
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3_600_000, ms/60_000%60, ms/1000%60, ms%1000)
}
