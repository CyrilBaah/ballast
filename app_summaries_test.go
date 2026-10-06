package main

import (
	"context"
	"net/http"
	"testing"

	"ballast/internal/events"
	"ballast/internal/modelfetch"
	"ballast/internal/summaries"
)

func newSummariesTestApp(t *testing.T) *App {
	t.Helper()
	a := newCaptionsTestApp(t)
	available := func() (bool, string, string) { return true, "", "/bin/true" }
	a.summariesAvailability = available
	a.summaries = summaries.NewWorker(summaries.Deps{
		DB:           a.db,
		DataDir:      t.TempDir(),
		Model:        modelfetch.Spec{FileName: "m.gguf", Size: 1},
		Availability: available,
		Emit:         func(events.SummaryJob) {},
		// Never download a real model from a test.
		FetchModel: func(context.Context, *http.Client, string, modelfetch.Spec, func(int64, int64)) error { return nil },
	})
	return a
}

func TestSummaryJobCreatedWithCaptionJob(t *testing.T) {
	a := newSummariesTestApp(t)
	id := newVideoUpload(t, a, "talk.mp4")
	a.enqueueCaptions(id)
	if j := a.summaryJobFor(id); j == nil || j.Phase != "waiting_for_captions" {
		t.Fatalf("summary job = %+v; want one waiting for captions (on by default)", j)
	}
	mkv := newVideoUpload(t, a, "talk.mkv")
	a.enqueueCaptions(mkv)
	if j := a.summaryJobFor(mkv); j != nil {
		t.Fatal("a video that can't be captioned got a summary job")
	}
}

func TestSummariesOffCreatesNoJob(t *testing.T) {
	a := newSummariesTestApp(t)
	if _, err := a.SummariesSetEnabled(false); err != nil {
		t.Fatal(err)
	}
	id := newVideoUpload(t, a, "talk.mp4")
	a.enqueueCaptions(id)
	if j := a.summaryJobFor(id); j != nil {
		t.Fatal("summary job created with summaries off")
	}
	if a.captionJobFor(id) == nil {
		t.Fatal("turning summaries off must not affect captions")
	}
}

func TestSummariesDeclineAndReenable(t *testing.T) {
	a := newSummariesTestApp(t)
	if _, err := a.SummariesAnswerModelDownload(false); err != nil {
		t.Fatal(err)
	}
	if s, _ := a.SummariesGetSettings(); s.Enabled || s.ModelConsent != "declined" {
		t.Fatalf("after declining: %+v", s)
	}
	s, err := a.SummariesSetEnabled(true)
	if err != nil || !s.Enabled || s.ModelConsent != "accepted" {
		t.Fatalf("re-enabling = %+v, %v", s, err)
	}
}

func TestSummariesRejectedWhenUnavailable(t *testing.T) {
	a := newSummariesTestApp(t)
	a.summariesAvailability = func() (bool, string, string) { return false, summaries.ReasonModelNotChosen, "" }
	s, _ := a.SummariesGetSettings()
	if s.Available || s.UnavailableReason != summaries.ReasonModelNotChosen {
		t.Fatalf("settings = %+v", s)
	}
	if _, err := a.SummariesSetEnabled(true); err == nil {
		t.Fatal("SummariesSetEnabled succeeded while unavailable")
	}
}

func TestCancelCaptionsAlsoCancelsSummary(t *testing.T) {
	a := newSummariesTestApp(t)
	id := newVideoUpload(t, a, "talk.mp4")
	a.enqueueCaptions(id)
	a.cancelCaptions(id)
	if j := a.summaryJobFor(id); j == nil || j.Status != "cancelled" {
		t.Fatalf("summary job after cancel = %+v", j)
	}
}
