package summaries

import (
	"regexp"
	"strings"
	"time"
)

// NoteNoQuotes replaces the quotes section when none survive the check.
const NoteNoQuotes = "No quotes could be verified"

// quoteWindow and minMatch: a quote counts as traced when at least 80% of
// its source words appear, in order, in what was said within ±30 s of its
// stated time (research.md §5).
const (
	quoteWindow = 30 * time.Second
	minMatch    = 0.8
)

// Result is a summary after the honesty checks.
type Result struct {
	Summary            Summary
	CheckBeforeSharing []Reference // guessed or doubtful references, never shared as-is
	UnverifiedQuotes   int
	NoQuotesNote       string
}

// Verify applies the checks that keep a summary honest whatever model
// wrote it (FR-003, FR-003a, SC-002, SC-010): quotes that don't trace back
// to the transcript are dropped; references whose words were never said
// are dropped; doubtful ones -- uncertain, or a Bible "book" that isn't
// one -- are listed for the user to check before sharing.
func Verify(s *Summary, t Transcript) Result {
	r := Result{Summary: *s}
	all := words(t.Text())

	r.Summary.Quotes = nil
	for _, q := range s.Quotes {
		if quoteTraces(q, t) {
			r.Summary.Quotes = append(r.Summary.Quotes, q)
		} else {
			r.UnverifiedQuotes++
		}
	}
	if len(s.Quotes) > 0 && len(r.Summary.Quotes) == 0 {
		r.NoQuotesNote = NoteNoQuotes
	}

	r.Summary.References = nil
	for _, ref := range s.References {
		if !saidSomewhere(words(ref.HeardAs), all) {
			continue
		}
		if ref.Kind == "bible" && !isBibleBook(ref.Reference) {
			ref.Certain = false
		}
		r.Summary.References = append(r.Summary.References, ref)
		if !ref.Certain {
			r.CheckBeforeSharing = append(r.CheckBeforeSharing, ref)
		}
	}
	return r
}

func quoteTraces(q Quote, t Transcript) bool {
	at := time.Duration(q.StartSeconds * float64(time.Second))
	var near []string
	for _, l := range t.Lines {
		if l.At >= at-quoteWindow && l.At <= at+quoteWindow {
			near = append(near, words(l.Text)...)
		}
	}
	return inOrderShare(words(q.SourceText), near) >= minMatch
}

// saidSomewhere reports whether phrase was said anywhere: 80% of its
// words in order within a stretch at most three times its length.
func saidSomewhere(phrase, all []string) bool {
	if len(phrase) == 0 {
		return false
	}
	span := 3*len(phrase) + 2
	for i := range all {
		if all[i] != phrase[0] && (len(phrase) < 2 || all[i] != phrase[1]) {
			continue
		}
		end := i + span
		if end > len(all) {
			end = len(all)
		}
		if inOrderShare(phrase, all[i:end]) >= minMatch {
			return true
		}
	}
	return false
}

// inOrderShare is the fraction of want's words found in order in have.
func inOrderShare(want, have []string) float64 {
	if len(want) == 0 {
		return 0
	}
	i := 0
	for _, w := range have {
		if i < len(want) && w == want[i] {
			i++
		}
	}
	// Allow a skipped word: greedy in-order matching can stall on one
	// mis-transcribed word, so also count matches after skipping it.
	if i < len(want) {
		best := i
		for skip := 0; skip < len(want); skip++ {
			n, j := 0, 0
			for _, w := range have {
				for j < len(want) && j == skip {
					j++
				}
				if j < len(want) && w == want[j] {
					n++
					j++
				}
			}
			if n > best {
				best = n
			}
		}
		i = best
	}
	return float64(i) / float64(len(want))
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

func words(s string) []string {
	return strings.Fields(nonWord.ReplaceAllString(strings.ToLower(s), " "))
}

var bibleBooks = func() map[string]bool {
	m := map[string]bool{}
	for _, b := range strings.Split("genesis exodus leviticus numbers deuteronomy joshua judges ruth 1samuel 2samuel 1kings 2kings 1chronicles 2chronicles ezra nehemiah esther job psalm psalms proverbs ecclesiastes songofsolomon songofsongs song isaiah jeremiah lamentations ezekiel daniel hosea joel amos obadiah jonah micah nahum habakkuk zephaniah haggai zechariah malachi matthew mark luke john acts romans 1corinthians 2corinthians galatians ephesians philippians colossians 1thessalonians 2thessalonians 1timothy 2timothy titus philemon hebrews james 1peter 2peter 1john 2john 3john jude revelation revelations", " ") {
		m[b] = true
	}
	return m
}()

var leadingBook = regexp.MustCompile(`^\s*((?:[123]\s*)?[A-Za-z]+(?:\s+of\s+[A-Za-z]+)?)`)

// isBibleBook reports whether ref starts with one of the 66 books' names.
func isBibleBook(ref string) bool {
	m := leadingBook.FindStringSubmatch(ref)
	if m == nil {
		return false
	}
	key := strings.ToLower(strings.Join(strings.Fields(m[1]), ""))
	return bibleBooks[key]
}
