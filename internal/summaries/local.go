package summaries

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// RequestTimeout caps one request; a slower answer counts as an unusable
// attempt (research.md §8).
var RequestTimeout = 20 * time.Minute

// Local is the free, on-Mac Summarizer: a running llama-server.
type Local struct {
	Server *Server
	Client *http.Client
}

// Single implements Summarizer.
func (l Local) Single(ctx context.Context, transcript string) (*Summary, error) {
	var s Summary
	if err := l.chat(ctx, SystemSingle, transcript, "summary", SummarySchema(), &s); err != nil {
		return nil, err
	}
	return &s, usable(&s)
}

// Notes implements Summarizer.
func (l Local) Notes(ctx context.Context, part string, index, of int) (*PartNotes, error) {
	var n PartNotes
	user := fmt.Sprintf("Part %d of %d:\n\n%s", index+1, of, part)
	if err := l.chat(ctx, SystemNotes, user, "part_notes", PartNotesSchema(), &n); err != nil {
		return nil, err
	}
	return &n, nil
}

// Combine implements Summarizer.
func (l Local) Combine(ctx context.Context, notes []PartNotes, length string) (*Summary, error) {
	b, err := json.Marshal(notes)
	if err != nil {
		return nil, err
	}
	var s Summary
	user := fmt.Sprintf("The video is about %s long. Notes on its %d parts, in order:\n\n%s", length, len(notes), b)
	if err := l.chat(ctx, SystemCombine, user, "summary", SummarySchema(), &s); err != nil {
		return nil, err
	}
	return &s, usable(&s)
}

// usable rejects a schema-valid but empty answer.
func usable(s *Summary) error {
	if strings.TrimSpace(s.Overview) == "" || len(s.MainPoints) == 0 || strings.TrimSpace(s.ShareMessage) == "" || strings.TrimSpace(s.Title) == "" {
		return ErrUnusable
	}
	return nil
}

func (l Local) chat(ctx context.Context, system, user, name string, schema map[string]any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()
	body, err := json.Marshal(map[string]any{
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.2,
		"max_tokens":  4096,
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": name, "strict": true, "schema": schema},
		},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.Server.URL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := l.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if l.Server.Exited() {
			return l.Server.exitError()
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: it took too long", ErrUnusable)
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: server answered %d", ErrUnusable, resp.StatusCode)
	}
	var cr struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil || len(cr.Choices) == 0 {
		return fmt.Errorf("%w: no answer", ErrUnusable)
	}
	if err := json.Unmarshal([]byte(cr.Choices[0].Message.Content), out); err != nil {
		return fmt.Errorf("%w: %v", ErrUnusable, err)
	}
	return nil
}
