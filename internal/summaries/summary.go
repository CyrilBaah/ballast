package summaries

import "context"

// MainPoint is one of the summary's main points.
type MainPoint struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// Reference is something the speaker cited: a Bible verse, a book, a
// study, a named source (FR-002b).
type Reference struct {
	Kind      string `json:"kind"` // bible, book, source, other
	Reference string `json:"reference"`
	HeardAs   string `json:"heard_as"` // the transcript's own words for it
	Certain   bool   `json:"certain"`
}

// Quote is a lightly tidied quote with the transcript words it came from
// (FR-003): Text is shown, SourceText is what the transcript check uses.
type Quote struct {
	Text         string  `json:"text"`
	StartSeconds float64 `json:"start_seconds"`
	SourceText   string  `json:"source_text"`
}

// Summary is the model's answer, in the shape research.md §3 fixes.
type Summary struct {
	Overview     string      `json:"overview"`
	MainPoints   []MainPoint `json:"main_points"`
	Actions      []string    `json:"actions"`
	References   []Reference `json:"references"`
	Quotes       []Quote     `json:"quotes"`
	Title        string      `json:"title"`
	Description  string      `json:"description"`
	ShareMessage string      `json:"share_message"`
}

// PartNotes are the notes for one part of a long transcript, combined
// into a Summary afterwards (research.md §4).
type PartNotes struct {
	Points     []MainPoint `json:"points"`
	Actions    []string    `json:"actions"`
	References []Reference `json:"references"`
	Quotes     []Quote     `json:"quotes"`
}

// Summarizer is the engine behind summaries (FR-019): today the local
// llama-server; an optional cloud engine could implement it later
// without changing anything else.
type Summarizer interface {
	// Single summarises a transcript short enough for one pass.
	Single(ctx context.Context, transcript string) (*Summary, error)
	// Notes takes notes on one part of a long transcript.
	Notes(ctx context.Context, part string, index, of int) (*PartNotes, error)
	// Combine turns every part's notes into one summary.
	Combine(ctx context.Context, notes []PartNotes, length string) (*Summary, error)
}

func str() map[string]any { return map[string]any{"type": "string"} }

func object(props map[string]any) map[string]any {
	required := make([]string, 0, len(props))
	for k := range props {
		required = append(required, k)
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

func array(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}

var (
	mainPointSchema = object(map[string]any{"title": str(), "detail": str()})
	referenceSchema = object(map[string]any{
		"kind":      map[string]any{"type": "string", "enum": []string{"bible", "book", "source", "other"}},
		"reference": str(), "heard_as": str(), "certain": map[string]any{"type": "boolean"},
	})
	quoteSchema = object(map[string]any{"text": str(), "start_seconds": map[string]any{"type": "number"}, "source_text": str()})
)

// SummarySchema is the JSON schema every final answer must match.
func SummarySchema() map[string]any {
	return object(map[string]any{
		"overview":      str(),
		"main_points":   array(mainPointSchema),
		"actions":       array(str()),
		"references":    array(referenceSchema),
		"quotes":        array(quoteSchema),
		"title":         str(),
		"description":   str(),
		"share_message": str(),
	})
}

// PartNotesSchema is the JSON schema for one part's notes.
func PartNotesSchema() map[string]any {
	return object(map[string]any{
		"points":     array(mainPointSchema),
		"actions":    array(str()),
		"references": array(referenceSchema),
		"quotes":     array(quoteSchema),
	})
}
