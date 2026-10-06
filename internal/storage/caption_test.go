package storage

import (
	"errors"
	"testing"
)

func newTestUpload(t *testing.T, db *DB) *Upload {
	t.Helper()
	u, err := db.CreateUpload("/tmp/sermon.mp4", 1024, testMtime, "folder", "Sermons")
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	return u
}

func TestCaptionJobCreateAndOnePerUpload(t *testing.T) {
	db := newTestDB(t)
	u := newTestUpload(t, db)

	j, err := db.CreateCaptionJob(u.ID, "en", CaptionWaiting, PhaseAwaitingConsent, "")
	if err != nil {
		t.Fatalf("CreateCaptionJob: %v", err)
	}
	if j.Status != CaptionWaiting || j.Phase != PhaseAwaitingConsent || j.Language != "en" {
		t.Fatalf("created job = %+v", j)
	}
	got, err := db.GetCaptionJobByUpload(u.ID)
	if err != nil || got.ID != j.ID {
		t.Fatalf("GetCaptionJobByUpload = %+v, %v", got, err)
	}
	if _, err := db.CreateCaptionJob(u.ID, "en", CaptionWaiting, PhaseAwaitingConsent, ""); err == nil {
		t.Fatal("second CreateCaptionJob for the same upload succeeded")
	}
	if _, err := db.GetCaptionJobByUpload(u.ID + 99); !errors.Is(err, ErrCaptionJobNotFound) {
		t.Fatalf("GetCaptionJobByUpload(missing) err = %v, want ErrCaptionJobNotFound", err)
	}
}

func TestCaptionJobCreateValidation(t *testing.T) {
	db := newTestDB(t)
	u := newTestUpload(t, db)
	if _, err := db.CreateCaptionJob(u.ID, "en", CaptionFailed, PhaseNone, ""); err == nil {
		t.Error("failed job with no note was accepted")
	}
	if _, err := db.CreateCaptionJob(u.ID, "en", CaptionInProgress, PhaseNone, ""); err == nil {
		t.Error("in-progress job with no phase was accepted")
	}
	if _, err := db.CreateCaptionJob(u.ID, "fr", CaptionInProgress, PhaseExtractingAudio, ""); err == nil {
		t.Error("unsupported language was accepted")
	}
	j, err := db.CreateCaptionJob(u.ID, "en", CaptionFailed, PhaseNone, "Captions aren't supported for .mkv files yet")
	if err != nil {
		t.Fatalf("CreateCaptionJob(failed with note): %v", err)
	}
	if j.EndedAt == nil {
		t.Error("a job created already failed has no ended_at")
	}
}

func TestCaptionJobHappyPathTransitions(t *testing.T) {
	db := newTestDB(t)
	u := newTestUpload(t, db)
	j, _ := db.CreateCaptionJob(u.ID, "auto", CaptionInProgress, PhaseDownloadingModel, "")

	steps := []CaptionPhase{PhaseExtractingAudio, PhaseTranscribing}
	for _, p := range steps {
		if err := db.SetCaptionPhase(j.ID, CaptionInProgress, p); err != nil {
			t.Fatalf("SetCaptionPhase(%s): %v", p, err)
		}
	}
	if err := db.SetCaptionProgress(j.ID, 42); err != nil {
		t.Fatalf("SetCaptionProgress: %v", err)
	}
	if err := db.SetCaptionPieces(j.ID, 1_500_000, 3); err != nil {
		t.Fatalf("SetCaptionPieces: %v", err)
	}
	if err := db.SetCaptionPiecesDone(j.ID, 3); err != nil {
		t.Fatalf("SetCaptionPiecesDone: %v", err)
	}
	if err := db.SetCaptionPiecesDone(j.ID, 4); err == nil {
		t.Error("pieces_done > piece_count was accepted")
	}
	if err := db.SetCaptionLocalCopy(j.ID, "/Users/me/Downloads/sermon.srt"); err != nil {
		t.Fatalf("SetCaptionLocalCopy: %v", err)
	}
	for _, p := range []CaptionPhase{PhaseWaitingForVideo, PhaseUploadingCaptions} {
		if err := db.SetCaptionPhase(j.ID, CaptionInProgress, p); err != nil {
			t.Fatalf("SetCaptionPhase(%s): %v", p, err)
		}
	}
	if err := db.SetCaptionDone(j.ID, "fid", "", "sermon.srt", ""); err == nil {
		t.Error("done with a file id but no link was accepted")
	}
	if err := db.SetCaptionDone(j.ID, "fid", "https://drive/x", "sermon.srt", ""); err != nil {
		t.Fatalf("SetCaptionDone: %v", err)
	}

	got, _ := db.GetCaptionJob(j.ID)
	if got.Status != CaptionDone || got.Phase != PhaseNone || got.EndedAt == nil {
		t.Fatalf("done job = %+v", got)
	}
	if got.DriveFileLink == nil || *got.DriveFileLink != "https://drive/x" || got.LocalCopyPath == nil {
		t.Fatalf("done job lost its file references: %+v", got)
	}
	if got.PieceCount == nil || *got.PieceCount != 3 || got.PiecesDone != 3 || got.Language != "auto" {
		t.Fatalf("done job lost its checkpoint: %+v", got)
	}

	// An ended job can't be moved again.
	if err := db.SetCaptionPhase(j.ID, CaptionInProgress, PhaseTranscribing); !errors.Is(err, ErrCaptionJobEnded) {
		t.Fatalf("SetCaptionPhase on a done job err = %v, want ErrCaptionJobEnded", err)
	}
	if err := db.SetCaptionCancelled(j.ID, ""); !errors.Is(err, ErrCaptionJobEnded) {
		t.Fatalf("SetCaptionCancelled on a done job err = %v, want ErrCaptionJobEnded", err)
	}
}

func TestCaptionJobDoneWithoutFile(t *testing.T) {
	db := newTestDB(t)
	u := newTestUpload(t, db)
	j, _ := db.CreateCaptionJob(u.ID, "en", CaptionInProgress, PhaseTranscribing, "")
	if err := db.SetCaptionDone(j.ID, "", "", "", "No speech found in this video"); err != nil {
		t.Fatalf("SetCaptionDone(no speech): %v", err)
	}
	got, _ := db.GetCaptionJob(j.ID)
	if got.DriveFileID != nil || got.Note == nil || *got.Note != "No speech found in this video" {
		t.Fatalf("no-speech job = %+v", got)
	}
}

func TestCaptionJobFailAndCancel(t *testing.T) {
	db := newTestDB(t)
	u := newTestUpload(t, db)
	j, _ := db.CreateCaptionJob(u.ID, "en", CaptionInProgress, PhaseExtractingAudio, "")
	if err := db.SetCaptionFailed(j.ID, ""); err == nil {
		t.Error("failed with no note was accepted")
	}
	if err := db.SetCaptionFailed(j.ID, "This video has no audio track"); err != nil {
		t.Fatalf("SetCaptionFailed: %v", err)
	}

	u2, _ := db.CreateUpload("/tmp/b.mp4", 1, testMtime, "folder", "F")
	j2, _ := db.CreateCaptionJob(u2.ID, "en", CaptionInProgress, PhaseWaitingForVideo, "")
	if err := db.SetCaptionLocalCopy(j2.ID, "/tmp/b.srt"); err != nil {
		t.Fatalf("SetCaptionLocalCopy: %v", err)
	}
	if err := db.SetCaptionCancelled(j2.ID, ""); err != nil {
		t.Fatalf("SetCaptionCancelled: %v", err)
	}
	got, _ := db.GetCaptionJob(j2.ID)
	if got.Status != CaptionCancelled || got.LocalCopyPath == nil {
		t.Fatalf("cancelled job should keep local_copy_path: %+v", got)
	}
}

func TestListActiveCaptionJobs(t *testing.T) {
	db := newTestDB(t)
	a, _ := db.CreateCaptionJob(newTestUpload(t, db).ID, "en", CaptionWaiting, PhaseAwaitingConsent, "")
	b, _ := db.CreateCaptionJob(newTestUpload(t, db).ID, "en", CaptionInProgress, PhaseTranscribing, "")
	c, _ := db.CreateCaptionJob(newTestUpload(t, db).ID, "en", CaptionInProgress, PhaseTranscribing, "")
	if err := db.SetCaptionFailed(c.ID, "boom"); err != nil {
		t.Fatal(err)
	}
	jobs, err := db.ListActiveCaptionJobs()
	if err != nil {
		t.Fatalf("ListActiveCaptionJobs: %v", err)
	}
	if len(jobs) != 2 || jobs[0].ID != a.ID || jobs[1].ID != b.ID {
		t.Fatalf("ListActiveCaptionJobs = %+v, want jobs %d and %d in order", jobs, a.ID, b.ID)
	}
}

func TestDeleteUploadDeletesCaptionJob(t *testing.T) {
	db := newTestDB(t)
	u := newTestUpload(t, db)
	if err := db.SetUploadInProgress(u.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUploadFailed(u.ID, "x"); err != nil {
		t.Fatal(err)
	}
	j, _ := db.CreateCaptionJob(u.ID, "en", CaptionFailed, PhaseNone, "x")
	if err := db.DeleteUpload(u.ID); err != nil {
		t.Fatalf("DeleteUpload: %v", err)
	}
	if _, err := db.GetCaptionJob(j.ID); !errors.Is(err, ErrCaptionJobNotFound) {
		t.Fatalf("caption job still there after its upload was deleted: err = %v", err)
	}
}
