package captions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ballast/internal/modelfetch"
	"ballast/internal/storage"
)

// installFakeModel puts the harness's fake model in place, as if it had
// been downloaded before a restart.
func (h *harness) installFakeModel() {
	h.t.Helper()
	dir := h.w.modelDir()
	os.MkdirAll(dir, 0o700)
	if err := os.WriteFile(modelfetch.Path(dir, h.w.deps.Model), []byte("fake"), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func TestRestartResumesAtSavedPiece(t *testing.T) {
	t.Setenv("FAKEWHISPER_MODE", "ok")
	h := newHarness(t)
	h.installFakeModel()
	u := h.newUpload("Sermon.mp4")

	// A job a previous run left two pieces into a three-piece transcription.
	j, err := h.db.CreateCaptionJob(u.ID, "en", storage.CaptionInProgress, storage.PhaseTranscribing, "")
	if err != nil {
		t.Fatal(err)
	}
	dir := h.w.workDir(j.ID)
	os.MkdirAll(dir, 0o700)
	if err := copyFile(writeTestWAV(t, 21*time.Minute), filepath.Join(dir, "audio.wav")); err != nil {
		t.Fatal(err)
	}
	h.db.SetCaptionPieces(j.ID, (21 * time.Minute).Milliseconds(), 3)
	writeFile(t, piecePath(dir, 0), "1\n00:00:00,000 --> 00:00:05,000\nsaved from piece zero\n\n")
	writeFile(t, piecePath(dir, 1), "1\n00:10:00,000 --> 00:10:05,000\nsaved from piece one\n\n")
	h.db.SetCaptionPiecesDone(j.ID, 2)

	h.w.RunUntilIdle(context.Background())

	got := h.job(u.ID)
	if got.Phase != storage.PhaseWaitingForVideo || got.PiecesDone != 3 {
		t.Fatalf("resumed job = %s/%s with %d pieces done", got.Status, got.Phase, got.PiecesDone)
	}
	b, _ := os.ReadFile(*got.LocalCopyPath)
	text := string(b)
	for _, want := range []string{"saved from piece zero", "saved from piece one", "line 1"} {
		if !strings.Contains(text, want) {
			t.Errorf("transcript is missing %q — pieces 0 and 1 must be reused, only piece 2 redone:\n%s", want, text)
		}
	}
	if strings.Count(text, "line 1") != 1 {
		t.Errorf("more than one piece was re-transcribed:\n%s", text)
	}
}

func TestRestartWithMissingAudioExtractsAgain(t *testing.T) {
	t.Setenv("FAKEWHISPER_MODE", "ok")
	h := newHarness(t)
	h.installFakeModel()
	u := h.newUpload("Sermon.mp4")
	j, _ := h.db.CreateCaptionJob(u.ID, "en", storage.CaptionInProgress, storage.PhaseTranscribing, "")
	h.db.SetCaptionPieces(j.ID, 60_000, 1)

	h.w.RunUntilIdle(context.Background())

	if got := h.job(u.ID); got.Phase != storage.PhaseWaitingForVideo {
		t.Fatalf("job = %s/%s; with its audio gone it should have re-extracted and finished", got.Status, got.Phase)
	}
}

func TestCleanupOrphans(t *testing.T) {
	h := newHarness(t)
	active, _ := h.db.CreateCaptionJob(h.newUpload("a.mp4").ID, "en", storage.CaptionInProgress, storage.PhaseTranscribing, "")
	ended, _ := h.db.CreateCaptionJob(h.newUpload("b.mp4").ID, "en", storage.CaptionFailed, storage.PhaseNone, "boom")
	for _, d := range []string{h.w.workDir(active.ID), h.w.workDir(ended.ID), h.w.workDir(999)} {
		os.MkdirAll(d, 0o700)
	}

	h.w.CleanupOrphans()

	if _, err := os.Stat(h.w.workDir(active.ID)); err != nil {
		t.Error("an active job's work folder was removed")
	}
	for _, id := range []int64{ended.ID, 999} {
		if _, err := os.Stat(h.w.workDir(id)); !os.IsNotExist(err) {
			t.Errorf("work folder for job %d was kept", id)
		}
	}
}

func TestNotEnoughDiskForAudio(t *testing.T) {
	h := newHarness(t)
	h.installFakeModel()
	h.w.deps.FreeSpace = func(string) (uint64, error) { return 10 << 20, nil }
	u := h.runToWaiting("a.mp4")
	if j := h.job(u.ID); j.Status != storage.CaptionFailed || *j.Note != NoteNoDiskForAudio {
		t.Fatalf("job = %+v; want failed with %q", j, NoteNoDiskForAudio)
	}
}

func TestVideoMovedBeforeCaptioning(t *testing.T) {
	h := newHarness(t)
	h.installFakeModel()
	h.db.SetCaptionModelConsent(storage.ConsentAccepted)
	u := h.newUpload("moved.mp4")
	h.w.Enqueue(u.ID)
	os.Remove(u.LocalPath)
	h.w.RunUntilIdle(context.Background())
	if j := h.job(u.ID); j.Status != storage.CaptionFailed || *j.Note != NoteVideoMissing {
		t.Fatalf("job = %+v; a moved video must not be reported as having no audio", j)
	}
}
