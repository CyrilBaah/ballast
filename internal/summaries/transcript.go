package summaries

import (
	"fmt"
	"io"
	"strings"
	"time"

	"ballast/internal/captions"
)

// SinglePassTokens is the largest transcript summarised in one request;
// above it the transcript is split into PartLength parts and combined
// (research.md §4). The 43-minute sermon measured ~7,000 tokens.
const (
	SinglePassTokens = 9000
	PartLength       = 20 * time.Minute
)

// Line is one caption cue's text and when it was said.
type Line struct {
	At   time.Duration
	Text string
}

// Transcript is what gets summarised.
type Transcript struct {
	Lines []Line
}

// FromSRT reads Feature 005's caption file.
func FromSRT(r io.Reader) (Transcript, error) {
	cues, err := captions.ParseSRT(r)
	if err != nil {
		return Transcript{}, err
	}
	t := Transcript{Lines: make([]Line, 0, len(cues))}
	for _, c := range cues {
		if s := strings.TrimSpace(c.Text); s != "" {
			t.Lines = append(t.Lines, Line{At: c.Start, Text: s})
		}
	}
	return t, nil
}

// Text renders one line per cue as "[mm:ss] text" ("[h:mm:ss]" past an
// hour) -- compact, and the timestamps are what make quotes checkable
// (research.md §10).
func (t Transcript) Text() string {
	var b strings.Builder
	for _, l := range t.Lines {
		fmt.Fprintf(&b, "[%s] %s\n", Clock(l.At), l.Text)
	}
	return b.String()
}

// Clock formats d as m:ss under an hour (zero-padded minutes) or h:mm:ss.
func Clock(d time.Duration) string {
	s := int(d.Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

// EstimateTokens is a rough token count (about 4 characters a token).
func EstimateTokens(s string) int { return len(s) / 4 }

// Parts returns the transcript itself if it fits one pass, or consecutive
// ~PartLength parts split between cues.
func (t Transcript) Parts() []Transcript {
	if len(t.Lines) == 0 || EstimateTokens(t.Text()) <= SinglePassTokens {
		return []Transcript{t}
	}
	var parts []Transcript
	var cur []Line
	boundary := PartLength
	for _, l := range t.Lines {
		if l.At >= boundary && len(cur) > 0 {
			parts = append(parts, Transcript{Lines: cur})
			cur = nil
			for l.At >= boundary {
				boundary += PartLength
			}
		}
		cur = append(cur, l)
	}
	if len(cur) > 0 {
		parts = append(parts, Transcript{Lines: cur})
	}
	return parts
}

// Duration is when the last line starts -- close enough to the video's
// length to tell the model how long it was.
func (t Transcript) Duration() time.Duration {
	if len(t.Lines) == 0 {
		return 0
	}
	return t.Lines[len(t.Lines)-1].At
}
