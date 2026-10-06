package summaries

import (
	"testing"
	"time"
)

func sermonTranscript() Transcript {
	return Transcript{Lines: []Line{
		{At: 0, Text: "Turn with me to the book of Romans chapter 8 and verse number 28."},
		{At: 40 * time.Second, Text: "Paul says, as often as you come before the Lord."},
		{At: 33 * time.Minute, Text: "Even in prison. The Lord was with him. Amen."},
		{At: 40 * time.Minute, Text: "Hebrews chapter 24, verse 26 says Moses refused to be known as the son of Pharaoh."},
		{At: 42 * time.Minute, Text: "We must continue to trust in God no matter the difficulties."},
	}}
}

func TestVerifyKeepsTracedQuotesAndDropsInventedOnes(t *testing.T) {
	s := &Summary{Quotes: []Quote{
		{Text: "Even in prison, the Lord was with him.", StartSeconds: 33*60 + 5, SourceText: "Even in prison. The Lord was with him."},
		{Text: "Trust God, whatever comes.", StartSeconds: 42 * 60, SourceText: "We must continue to trust in God no matter the difficulties"},
		{Text: "God will make you rich.", StartSeconds: 42 * 60, SourceText: "God will make you rich"},                   // never said
		{Text: "Even in prison…", StartSeconds: 10 * 60, SourceText: "Even in prison. The Lord was with him."}, // said, but not near 10:00
	}}
	r := Verify(s, sermonTranscript())
	if len(r.Summary.Quotes) != 2 || r.UnverifiedQuotes != 2 {
		t.Fatalf("kept %d quotes, dropped %d; want 2 and 2: %+v", len(r.Summary.Quotes), r.UnverifiedQuotes, r.Summary.Quotes)
	}
}

func TestVerifyNoQuotesLeft(t *testing.T) {
	r := Verify(&Summary{Quotes: []Quote{{Text: "x", StartSeconds: 0, SourceText: "nothing like this was said"}}}, sermonTranscript())
	if len(r.Summary.Quotes) != 0 || r.NoQuotesNote != NoteNoQuotes {
		t.Fatalf("result = %+v", r)
	}
}

func TestVerifyReferences(t *testing.T) {
	s := &Summary{References: []Reference{
		{Kind: "bible", Reference: "Romans 8:28", HeardAs: "Romans chapter 8 and verse 28", Certain: true},
		{Kind: "bible", Reference: "Hebrews 11:24–26", HeardAs: "Hebrews chapter 24, verse 26", Certain: false},
		{Kind: "bible", Reference: "John 3:16", HeardAs: "John 3:16", Certain: true},                             // never said
		{Kind: "bible", Reference: "Hezekiah 4:2", HeardAs: "Romans chapter 8 and verse 28", Certain: true},      // not a book
		{Kind: "book", Reference: "The Periplus", HeardAs: "Paul says as often as you come before the Lord", Certain: true},
	}}
	r := Verify(s, sermonTranscript())
	got := map[string]bool{}
	for _, ref := range r.Summary.References {
		got[ref.Reference] = true
	}
	if !got["Romans 8:28"] || !got["Hebrews 11:24–26"] || got["John 3:16"] {
		t.Fatalf("references = %+v", r.Summary.References)
	}
	check := map[string]bool{}
	for _, ref := range r.CheckBeforeSharing {
		check[ref.Reference] = true
	}
	if !check["Hebrews 11:24–26"] || !check["Hezekiah 4:2"] || check["Romans 8:28"] {
		t.Fatalf("check before sharing = %+v", r.CheckBeforeSharing)
	}
}

func TestIsBibleBook(t *testing.T) {
	for _, ok := range []string{"Romans 8:28", "1 Peter 2:9–10", "Psalm 23", "Song of Solomon 2:4", "Genesis 41:38-45", "1 Corinthians 11:26"} {
		if !isBibleBook(ok) {
			t.Errorf("isBibleBook(%q) = false", ok)
		}
	}
	for _, bad := range []string{"Hezekiah 4:2", "Gospel of Thomas 1", ""} {
		if isBibleBook(bad) {
			t.Errorf("isBibleBook(%q) = true", bad)
		}
	}
}
