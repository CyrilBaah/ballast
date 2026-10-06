package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ballast/internal/captions"
	"ballast/internal/events"
	"ballast/internal/modelfetch"
	"ballast/internal/storage"
)

func newCaptionsTestApp(t *testing.T) *App {
	t.Helper()
	db, err := storage.OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	available := func() (bool, string, string) { return true, "", "/bin/true" }
	a := NewApp()
	a.ctx = context.Background()
	a.db = db
	a.captionsAvailability = available
	a.captions = captions.NewWorker(captions.Deps{
		DB:           db,
		DataDir:      t.TempDir(),
		Model:        captions.SpeechModel,
		Availability: available,
		Emit:         func(events.CaptionJob) {},
		// Never download the real 1.6 GB model from a test.
		FetchModel: func(context.Context, *http.Client, string, modelfetch.Spec, func(int64, int64)) error {
			return nil
		},
	})
	return a
}

func newVideoUpload(t *testing.T, a *App, name string) int64 {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	os.WriteFile(p, []byte("v"), 0o644)
	u, err := a.db.CreateUpload(p, 1, time.Now(), "folder", "F")
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestCaptionsSetEnabledControlsNewJobs(t *testing.T) {
	a := newCaptionsTestApp(t)
	if _, err := a.CaptionsSetEnabled(false); err != nil {
		t.Fatalf("CaptionsSetEnabled(false): %v", err)
	}
	off := newVideoUpload(t, a, "off.mp4")
	a.enqueueCaptions(off)
	if j := a.captionJobFor(off); j != nil {
		t.Fatalf("a caption job was created with captions off: %+v", j)
	}

	settings, err := a.CaptionsSetEnabled(true)
	if err != nil || !settings.Enabled {
		t.Fatalf("CaptionsSetEnabled(true) = %+v, %v", settings, err)
	}
	on := newVideoUpload(t, a, "on.mp4")
	a.enqueueCaptions(on)
	if j := a.captionJobFor(on); j == nil {
		t.Fatal("no caption job with captions on")
	}
}

func TestCaptionsSetEnabledAfterDeclineAcceptsDownload(t *testing.T) {
	a := newCaptionsTestApp(t)
	if _, err := a.CaptionsAnswerModelDownload(false); err != nil {
		t.Fatal(err)
	}
	if s, _ := a.CaptionsGetSettings(); s.Enabled || s.ModelConsent != "declined" {
		t.Fatalf("after declining: %+v", s)
	}
	s, err := a.CaptionsSetEnabled(true)
	if err != nil || !s.Enabled || s.ModelConsent != "accepted" {
		t.Fatalf("turning captions back on = %+v, %v; want enabled with consent accepted", s, err)
	}
}

func TestCaptionsDeclineCancelsWaitingJobs(t *testing.T) {
	a := newCaptionsTestApp(t)
	id := newVideoUpload(t, a, "a.mp4")
	a.enqueueCaptions(id)
	if j := a.captionJobFor(id); j == nil || j.Phase != "awaiting_consent" {
		t.Fatalf("job before answering = %+v", j)
	}
	a.CaptionsAnswerModelDownload(false)
	if j := a.captionJobFor(id); j.Status != "cancelled" || j.Note != "Captions were turned off" {
		t.Fatalf("job after declining = %+v", j)
	}
}

func TestCaptionsSetLanguage(t *testing.T) {
	a := newCaptionsTestApp(t)
	if _, err := a.CaptionsSetLanguage("fr"); err == nil {
		t.Fatal("CaptionsSetLanguage(fr) succeeded")
	}
	a.db.SetCaptionModelConsent(storage.ConsentAccepted)
	first := newVideoUpload(t, a, "first.mp4")
	a.enqueueCaptions(first)
	if s, err := a.CaptionsSetLanguage("auto"); err != nil || s.Language != "auto" {
		t.Fatalf("CaptionsSetLanguage(auto) = %+v, %v", s, err)
	}
	second := newVideoUpload(t, a, "second.mp4")
	a.enqueueCaptions(second)
	if a.captionJobFor(first).Language != "en" || a.captionJobFor(second).Language != "auto" {
		t.Fatal("a job's language must be fixed when it is created")
	}
}

func TestCaptionsSettingsRejectedWhenUnavailable(t *testing.T) {
	a := newCaptionsTestApp(t)
	a.captionsAvailability = func() (bool, string, string) { return false, captions.ReasonUnsupportedSystem, "" }
	s, err := a.CaptionsGetSettings()
	if err != nil || s.Available || s.UnavailableReason != captions.ReasonUnsupportedSystem {
		t.Fatalf("CaptionsGetSettings = %+v, %v", s, err)
	}
	if _, err := a.CaptionsSetEnabled(true); err == nil {
		t.Fatal("CaptionsSetEnabled succeeded where captions aren't available")
	}
	if _, err := a.CaptionsSetLanguage("auto"); err == nil {
		t.Fatal("CaptionsSetLanguage succeeded where captions aren't available")
	}
}
