package summaries

import (
	"fmt"
	"html"
	"path/filepath"
	"strings"
	"time"
)

// section is one heading and its paragraphs or list items.
type section struct {
	title string
	paras []string
	items []string
	pre   string // kept as-is (the share message)
}

// sections lays out a summary (FR-002, FR-002b, FR-003a). Optional
// sections with nothing in them are left out; "Check before sharing"
// comes after the share message and never inside it.
func sections(r Result) []section {
	s := r.Summary
	out := []section{{title: "Overview", paras: []string{s.Overview}}}

	points := make([]string, 0, len(s.MainPoints))
	for _, p := range s.MainPoints {
		if p.Detail != "" {
			points = append(points, p.Title+" — "+p.Detail)
		} else {
			points = append(points, p.Title)
		}
	}
	out = append(out, section{title: "Main points", items: points})

	if len(s.Actions) > 0 {
		out = append(out, section{title: "Key takeaways / actions", items: s.Actions})
	}
	if len(s.References) > 0 {
		refs := make([]string, 0, len(s.References))
		for _, ref := range s.References {
			refs = append(refs, ref.Reference)
		}
		out = append(out, section{title: "References", items: refs})
	}
	if len(s.Quotes) > 0 {
		qs := make([]string, 0, len(s.Quotes))
		for _, q := range s.Quotes {
			qs = append(qs, fmt.Sprintf("%s — “%s”", Clock(time.Duration(q.StartSeconds*float64(time.Second))), q.Text))
		}
		out = append(out, section{title: "Key quotes", items: qs})
	} else if r.NoQuotesNote != "" {
		out = append(out, section{title: "Key quotes", paras: []string{r.NoQuotesNote}})
	}
	out = append(out,
		section{title: "Suggested title", paras: []string{s.Title}},
		section{title: "Suggested description", paras: []string{s.Description}},
		section{title: "Share message", pre: s.ShareMessage},
	)
	if len(r.CheckBeforeSharing) > 0 {
		var checks []string
		for _, ref := range r.CheckBeforeSharing {
			checks = append(checks, fmt.Sprintf("%s — heard \"%s\"", ref.Reference, ref.HeardAs))
		}
		out = append(out, section{title: "Check before sharing", paras: []string{"These were unclear in the recording. Please confirm them before sharing:"}, items: checks})
	}
	return out
}

func docTitle(videoPath string) string {
	return strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath)) + " — Summary"
}

// Markdown is the local copy saved next to the video (FR-007).
func Markdown(videoPath string, r Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", docTitle(videoPath))
	for _, s := range sections(r) {
		fmt.Fprintf(&b, "\n## %s\n\n", s.title)
		for _, p := range s.paras {
			fmt.Fprintf(&b, "%s\n\n", p)
		}
		for _, it := range s.items {
			fmt.Fprintf(&b, "- %s\n", it)
		}
		if s.pre != "" {
			fmt.Fprintf(&b, "```\n%s\n```\n", s.pre)
		}
	}
	return b.String()
}

// HTML is uploaded to Drive and converted into a Google Doc (research.md §11).
func HTML(videoPath string, r Result) string {
	e := html.EscapeString
	var b strings.Builder
	fmt.Fprintf(&b, "<!doctype html><html><head><meta charset=\"utf-8\"><title>%s</title></head><body>\n<h1>%s</h1>\n", e(docTitle(videoPath)), e(docTitle(videoPath)))
	for _, s := range sections(r) {
		fmt.Fprintf(&b, "<h2>%s</h2>\n", e(s.title))
		for _, p := range s.paras {
			fmt.Fprintf(&b, "<p>%s</p>\n", e(p))
		}
		if len(s.items) > 0 {
			b.WriteString("<ul>\n")
			for _, it := range s.items {
				fmt.Fprintf(&b, "<li>%s</li>\n", e(it))
			}
			b.WriteString("</ul>\n")
		}
		if s.pre != "" {
			for _, line := range strings.Split(s.pre, "\n") {
				fmt.Fprintf(&b, "<p>%s</p>\n", e(line))
			}
		}
	}
	b.WriteString("</body></html>\n")
	return b.String()
}
