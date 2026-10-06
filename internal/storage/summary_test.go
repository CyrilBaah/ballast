package storage

import (
	"errors"
	"testing"
)

func TestSummaryJobLifecycle(t *testing.T) {
	db := newTestDB(t)
	u := newTestUpload(t, db)

	j, err := db.CreateSummaryJob(u.ID, "model.gguf")
	if err != nil {
		t.Fatalf("CreateSummaryJob: %v", err)
	}
	if j.Status != SummaryWaiting || j.Phase != SummaryPhaseWaitingForCaptions || j.Engine != "local" || j.Model != "model.gguf" {
		t.Fatalf("new job = %+v", j)
	}
	if _, err := db.CreateSummaryJob(u.ID, "model.gguf"); err == nil {
		t.Fatal("a second summary job for the same upload was accepted")
	}
	if got, err := db.GetSummaryJobByUpload(u.ID); err != nil || got.ID != j.ID {
		t.Fatalf("GetSummaryJobByUpload = %+v, %v", got, err)
	}
	if _, err := db.GetSummaryJobByUpload(u.ID + 9); !errors.Is(err, ErrSummaryJobNotFound) {
		t.Fatalf("missing job err = %v", err)
	}

	for _, p := range []SummaryPhase{SummaryPhaseAwaitingConsent, SummaryPhaseDownloadingModel, SummaryPhaseWaitingForEngine} {
		if err := db.SetSummaryPhase(j.ID, SummaryWaiting, p); err != nil {
			t.Fatalf("SetSummaryPhase(%s): %v", p, err)
		}
	}
	if err := db.SetSummaryPhase(j.ID, SummaryInProgress, SummaryPhaseSummarising); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSummaryParts(j.ID, 3); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSummaryPartsDone(j.ID, 4); err == nil {
		t.Error("parts_done > part_count was accepted")
	}
	if err := db.SetSummaryPartsDone(j.ID, 3); err != nil {
		t.Fatal(err)
	}
	if n, err := db.AddSummaryAttempt(j.ID); err != nil || n != 1 {
		t.Fatalf("AddSummaryAttempt = %d, %v", n, err)
	}
	if err := db.SetSummaryUnverifiedQuotes(j.ID, 2); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSummaryLocalCopy(j.ID, "/v/Sermon — Summary.md"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSummaryDone(j.ID, "doc", "", "Sermon — Summary"); err == nil {
		t.Error("done with an id but no link was accepted")
	}
	if err := db.SetSummaryDone(j.ID, "doc", "https://docs/x", "Sermon — Summary"); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetSummaryJob(j.ID)
	if got.Status != SummaryDone || got.Phase != SummaryPhaseNone || got.EndedAt == nil || got.Attempts != 1 || got.UnverifiedQuotes != 2 || got.PartsDone != 3 {
		t.Fatalf("done job = %+v", got)
	}
	if err := db.SetSummaryFailed(j.ID, "x"); !errors.Is(err, ErrSummaryJobEnded) {
		t.Fatalf("SetSummaryFailed on a done job err = %v", err)
	}
}

func TestSummaryJobFailRetryCancel(t *testing.T) {
	db := newTestDB(t)
	j, _ := db.CreateSummaryJob(newTestUpload(t, db).ID, "m")
	if err := db.SetSummaryFailed(j.ID, ""); err == nil {
		t.Error("failed with no note accepted")
	}
	db.AddSummaryAttempt(j.ID)
	if err := db.SetSummaryFailed(j.ID, "The summary model couldn't produce a usable summary"); err != nil {
		t.Fatal(err)
	}
	if err := db.ResetSummaryForRetry(j.ID); err != nil {
		t.Fatalf("ResetSummaryForRetry: %v", err)
	}
	got, _ := db.GetSummaryJob(j.ID)
	if got.Status != SummaryWaiting || got.Phase != SummaryPhaseWaitingForEngine || got.Attempts != 0 || got.Note != nil || got.EndedAt != nil {
		t.Fatalf("retried job = %+v", got)
	}
	if err := db.SetSummaryCancelled(j.ID, "No speech to summarise"); err != nil {
		t.Fatal(err)
	}
	if err := db.ResetSummaryForRetry(j.ID); err == nil {
		t.Error("a cancelled job was reset for retry; only failed ones can be")
	}
}

func TestListActiveSummaryJobsAndCascade(t *testing.T) {
	db := newTestDB(t)
	a, _ := db.CreateSummaryJob(newTestUpload(t, db).ID, "m")
	b, _ := db.CreateSummaryJob(newTestUpload(t, db).ID, "m")
	db.SetSummaryCancelled(b.ID, "")
	jobs, err := db.ListActiveSummaryJobs()
	if err != nil || len(jobs) != 1 || jobs[0].ID != a.ID {
		t.Fatalf("ListActiveSummaryJobs = %+v, %v", jobs, err)
	}

	u := newTestUpload(t, db)
	db.SetUploadInProgress(u.ID)
	db.SetUploadFailed(u.ID, "x")
	c, _ := db.CreateSummaryJob(u.ID, "m")
	db.SetSummaryCancelled(c.ID, "")
	if err := db.DeleteUpload(u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetSummaryJob(c.ID); !errors.Is(err, ErrSummaryJobNotFound) {
		t.Fatalf("summary job survived its upload's deletion: %v", err)
	}
}

func TestSummarySettings(t *testing.T) {
	db := newTestDB(t)
	if on, _ := db.SummariesEnabled(); !on {
		t.Error("summaries should be on by default (FR-012)")
	}
	if c, _ := db.SummaryModelConsent(); c != ConsentUnasked {
		t.Errorf("summary consent default = %q", c)
	}
	db.SetSummariesEnabled(false)
	db.SetSummaryModelConsent(ConsentDeclined)
	if on, _ := db.SummariesEnabled(); on {
		t.Error("SetSummariesEnabled(false) didn't stick")
	}
	if c, _ := db.SummaryModelConsent(); c != ConsentDeclined {
		t.Errorf("summary consent = %q", c)
	}
}
