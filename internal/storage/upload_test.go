package storage

import (
	"path/filepath"
	"testing"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUploadStartsPendingThenMovesInProgress(t *testing.T) {
	db := newTestDB(t)

	u, err := db.CreateUpload("/tmp/file.txt", 1024, "root")
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	if u.Status != UploadPending {
		t.Fatalf("initial status = %q, want %q", u.Status, UploadPending)
	}

	if err := db.SetUploadInProgress(u.ID); err != nil {
		t.Fatalf("SetUploadInProgress: %v", err)
	}
	got, err := db.GetUpload(u.ID)
	if err != nil {
		t.Fatalf("GetUpload: %v", err)
	}
	if got.Status != UploadInProgress {
		t.Fatalf("status after SetUploadInProgress = %q, want %q", got.Status, UploadInProgress)
	}
}

func TestUploadProgressUpdatesBytesSent(t *testing.T) {
	db := newTestDB(t)
	u, err := db.CreateUpload("/tmp/file.txt", 1024, "root")
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	if err := db.SetUploadInProgress(u.ID); err != nil {
		t.Fatalf("SetUploadInProgress: %v", err)
	}
	if err := db.UpdateUploadProgress(u.ID, 512); err != nil {
		t.Fatalf("UpdateUploadProgress: %v", err)
	}
	got, err := db.GetUpload(u.ID)
	if err != nil {
		t.Fatalf("GetUpload: %v", err)
	}
	if got.BytesSent != 512 {
		t.Fatalf("BytesSent = %d, want 512", got.BytesSent)
	}
}

func TestUploadSucceededRequiresFileIDAndLink(t *testing.T) {
	db := newTestDB(t)
	u, err := db.CreateUpload("/tmp/file.txt", 1024, "root")
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	_ = db.SetUploadInProgress(u.ID)

	if err := db.SetUploadSucceeded(u.ID, "", ""); err == nil {
		t.Fatal("expected SetUploadSucceeded to reject empty driveFileID/driveFileLink")
	}

	if err := db.SetUploadSucceeded(u.ID, "drive-file-id", "https://drive.google.com/file/d/xyz"); err != nil {
		t.Fatalf("SetUploadSucceeded: %v", err)
	}
	got, err := db.GetUpload(u.ID)
	if err != nil {
		t.Fatalf("GetUpload: %v", err)
	}
	if got.Status != UploadSucceeded {
		t.Fatalf("status = %q, want %q", got.Status, UploadSucceeded)
	}
	if got.DriveFileID == nil || *got.DriveFileID != "drive-file-id" {
		t.Fatalf("DriveFileID = %v, want drive-file-id", got.DriveFileID)
	}
	if got.DriveFileLink == nil || *got.DriveFileLink != "https://drive.google.com/file/d/xyz" {
		t.Fatalf("DriveFileLink = %v", got.DriveFileLink)
	}
	if got.EndedAt == nil {
		t.Fatal("EndedAt must be set on a terminal status")
	}
}

func TestUploadFailedRequiresReason(t *testing.T) {
	db := newTestDB(t)
	u, err := db.CreateUpload("/tmp/file.txt", 1024, "root")
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	_ = db.SetUploadInProgress(u.ID)

	if err := db.SetUploadFailed(u.ID, ""); err == nil {
		t.Fatal("expected SetUploadFailed to reject an empty reason")
	}

	if err := db.SetUploadFailed(u.ID, "network connection lost"); err != nil {
		t.Fatalf("SetUploadFailed: %v", err)
	}
	got, err := db.GetUpload(u.ID)
	if err != nil {
		t.Fatalf("GetUpload: %v", err)
	}
	if got.Status != UploadFailed {
		t.Fatalf("status = %q, want %q", got.Status, UploadFailed)
	}
	if got.FailureReason == nil || *got.FailureReason != "network connection lost" {
		t.Fatalf("FailureReason = %v", got.FailureReason)
	}
	if got.EndedAt == nil {
		t.Fatal("EndedAt must be set on a terminal status")
	}
}

// TestOnlyOneUploadInProgressAtATime verifies a second upload can't be moved to in_progress while one is already running.
func TestOnlyOneUploadInProgressAtATime(t *testing.T) {
	db := newTestDB(t)

	first, err := db.CreateUpload("/tmp/a.txt", 10, "root")
	if err != nil {
		t.Fatalf("CreateUpload #1: %v", err)
	}
	if err := db.SetUploadInProgress(first.ID); err != nil {
		t.Fatalf("SetUploadInProgress #1: %v", err)
	}

	second, err := db.CreateUpload("/tmp/b.txt", 20, "root")
	if err != nil {
		t.Fatalf("CreateUpload #2: %v", err)
	}
	if err := db.SetUploadInProgress(second.ID); err == nil {
		t.Fatal("expected SetUploadInProgress to reject a second concurrent in_progress upload")
	}
}
