package summaries

import (
	"strings"
	"testing"
)

func sampleResult() Result {
	return Result{
		Summary: Summary{
			Overview:     "A talk about Aksum & its <coins>.",
			MainPoints:   []MainPoint{{Title: "Trade", Detail: "It controlled the Red Sea."}},
			Quotes:       []Quote{{Text: "Money just moved.", StartSeconds: 11*60 + 7}},
			Title:        "Aksum",
			Description:  "The forgotten empire.",
			ShareMessage: "*Aksum*\nThe forgotten empire.",
		},
	}
}

func TestRenderLeavesOutEmptyOptionalSections(t *testing.T) {
	md := Markdown("Aksum.mp4", sampleResult())
	for _, want := range []string{"## Overview", "## Main points", "## Key quotes", "11:07", "## Suggested title", "## Suggested description", "## Share message"} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown is missing %q", want)
		}
	}
	for _, absent := range []string{"References", "Key takeaways", "Check before sharing"} {
		if strings.Contains(md, absent) {
			t.Errorf("Markdown has an empty %q section (FR-002b)", absent)
		}
	}
}

func TestRenderOptionalSectionsAndCheckNote(t *testing.T) {
	r := sampleResult()
	r.Summary.Actions = []string{"Pray daily"}
	r.Summary.References = []Reference{{Kind: "bible", Reference: "Hebrews 11:24–26", HeardAs: "Hebrews chapter 24, verse 26"}}
	r.CheckBeforeSharing = r.Summary.References
	md := Markdown("Sermon.mp4", r)
	for _, want := range []string{"## Key takeaways / actions", "- Pray daily", "## References", "Hebrews 11:24–26", "## Check before sharing", `heard "Hebrews chapter 24, verse 26"`} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown is missing %q", want)
		}
	}
	share := md[strings.Index(md, "## Share message"):]
	if i := strings.Index(md, "## Check before sharing"); i > strings.Index(md, "## Share message") {
		share = md[strings.Index(md, "## Share message"):i]
	}
	if strings.Contains(share, "heard") {
		t.Fatal("the check-before-sharing note leaked into the share message")
	}
}

func TestHTMLEscapesModelText(t *testing.T) {
	html := HTML("Aksum.mp4", sampleResult())
	if strings.Contains(html, "<coins>") || !strings.Contains(html, "&lt;coins&gt;") {
		t.Fatal("model text wasn't escaped in the Google Doc HTML")
	}
	if !strings.Contains(html, "<h2>Overview</h2>") {
		t.Fatal("HTML has no Overview heading")
	}
}

func TestNoQuotesNote(t *testing.T) {
	r := sampleResult()
	r.Summary.Quotes = nil
	r.NoQuotesNote = NoteNoQuotes
	if md := Markdown("x.mp4", r); !strings.Contains(md, NoteNoQuotes) {
		t.Fatal("the no-quotes note is missing")
	}
}
