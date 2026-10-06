package summaries

import (
	"errors"
	"regexp"
	"strings"
)

// ErrUnusable means the model's answer can't be used as it is; the worker
// retries a limited number of times (FR-010).
var ErrUnusable = errors.New("summaries: the model's answer couldn't be used")

var (
	// Emoji and their joiners/selectors: symbol-other characters, the
	// keycap combining mark, variation selectors and the zero-width joiner.
	emoji = regexp.MustCompile(`[\p{So}\x{20E3}\x{FE0E}\x{FE0F}\x{200D}\x{1F3FB}-\x{1F3FF}]`)
	// Timestamps like [04:28], (1:02:03) or "at 12:03" -- but not verse
	// references like 8:28, which have no brackets or "at".
	timestamp  = regexp.MustCompile(`\s*(\[\d{1,2}:\d{2}(:\d{2})?\]|\((at\s+)?\d{1,2}:\d{2}(:\d{2})?\)|\bat\s+\d{1,2}:\d{2}:\d{2}\b)`)
	blankLines = regexp.MustCompile(`\n{3,}`)
)

// CleanShareMessage enforces the share message's rules in code, since a
// small model can drift from the prompt (FR-002a, research.md §12): emoji
// and timestamps are removed, and a message that talks about the
// transcript (or is empty) is unusable.
func CleanShareMessage(s string) (string, error) {
	s = emoji.ReplaceAllString(s, "")
	s = timestamp.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(strings.Join(strings.Fields(l), " "))
	}
	s = strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
	if s == "" || strings.Contains(strings.ToLower(s), "transcript") {
		return "", ErrUnusable
	}
	return s, nil
}
