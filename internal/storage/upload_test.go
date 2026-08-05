package storage

import (
	"path/filepath"
	"testing"
	"time"
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

var testMtime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestUploadStartsPendingThenMovesInProgress(t *testing.T) {
	db := newTestDB(t)

	u, err := db.CreateUpload("/tmp/file.txt", 1024, testMtime, "root", "Test Folder")
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
	if !got.LocalMtime.Equal(testMtime) {
		t.Fatalf("LocalMtime = %v, want %v", got.LocalMtime, testMtime)
	}
}

func TestUploadProgressUpdatesBytesSentSessionURIAndHashState(t *testing.T) {
	db := newTestDB(t)
	u, err := db.CreateUpload("/tmp/file.txt", 1024, testMtime, "root", "Test Folder")
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	if err := db.SetUploadInProgress(u.ID); err != nil {
		t.Fatalf("SetUploadInProgress: %v", err)
	}
	hashState := []byte{1, 2, 3}
	if err := db.UpdateUploadProgress(u.ID, 512, "https://example.test/session/abc", hashState, 16*1024*1024, 1); err != nil {
		t.Fatalf("UpdateUploadProgress: %v", err)
	}
	got, err := db.GetUpload(u.ID)
	if err != nil {
		t.Fatalf("GetUpload: %v", err)
	}
	if got.BytesSent != 512 {
		t.Fatalf("BytesSent = %d, want 512", got.BytesSent)
	}
	if got.SessionURI == nil || *got.SessionURI != "https://example.test/session/abc" {
		t.Fatalf("SessionURI = %v, want the persisted session URI", got.SessionURI)
	}
	if string(got.ContentHashState) != string(hashState) {
		t.Fatalf("ContentHashState = %v, want %v", got.ContentHashState, hashState)
	}
	if got.ChunkSizeBytes != 16*1024*1024 {
		t.Fatalf("ChunkSizeBytes = %d, want %d", got.ChunkSizeBytes, 16*1024*1024)
	}
	if got.ConsecutiveChunkSuccesses != 1 {
		t.Fatalf("ConsecutiveChunkSuccesses = %d, want 1", got.ConsecutiveChunkSuccesses)
	}
}

func TestCreateUploadDefaultsToBaselineChunkSize(t *testing.T) {
	db := newTestDB(t)
	u, err := db.CreateUpload("/tmp/file.txt", 1024, testMtime, "root", "Test Folder")
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	if u.ChunkSizeBytes != baselineChunkSizeBytes {
		t.Fatalf("ChunkSizeBytes = %d, want baseline %d", u.ChunkSizeBytes, baselineChunkSizeBytes)
	}
	if u.ConsecutiveChunkSuccesses != 0 {
		t.Fatalf("ConsecutiveChunkSuccesses = %d, want 0", u.ConsecutiveChunkSuccesses)
	}

	got, err := db.GetUpload(u.ID)
	if err != nil {
		t.Fatalf("GetUpload: %v", err)
	}
	if got.ChunkSizeBytes != baselineChunkSizeBytes {
		t.Fatalf("persisted ChunkSizeBytes = %d, want baseline %d", got.ChunkSizeBytes, baselineChunkSizeBytes)
	}
}

func TestUploadSucceededRequiresFileIDAndLink(t *testing.T) {
	db := newTestDB(t)
	u, err := db.CreateUpload("/tmp/file.txt", 1024, testMtime, "root", "Test Folder")
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
	u, err := db.CreateUpload("/tmp/file.txt", 1024, testMtime, "root", "Test Folder")
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

	first, err := db.CreateUpload("/tmp/a.txt", 10, testMtime, "root", "Test Folder")
	if err != nil {
		t.Fatalf("CreateUpload #1: %v", err)
	}
	if err := db.SetUploadInProgress(first.ID); err != nil {
		t.Fatalf("SetUploadInProgress #1: %v", err)
	}

	second, err := db.CreateUpload("/tmp/b.txt", 20, testMtime, "root", "Test Folder")
	if err != nil {
		t.Fatalf("CreateUpload #2: %v", err)
	}
	if err := db.SetUploadInProgress(second.ID); err == nil {
		t.Fatal("expected SetUploadInProgress to reject a second concurrent in_progress upload")
	}
}

// TestSingleActiveUploadSpansPausedAndAwaitingConfirmation verifies FR-013's
// single-active-upload constraint blocks a new upload while the existing
// one is merely paused or awaiting_confirmation, not just in_progress.
func TestSingleActiveUploadSpansPausedAndAwaitingConfirmation(t *testing.T) {
	db := newTestDB(t)

	first, err := db.CreateUpload("/tmp/a.txt", 10, testMtime, "root", "Test Folder")
	if err != nil {
		t.Fatalf("CreateUpload #1: %v", err)
	}
	if err := db.SetUploadInProgress(first.ID); err != nil {
		t.Fatalf("SetUploadInProgress #1: %v", err)
	}
	if err := db.SetUploadPaused(first.ID); err != nil {
		t.Fatalf("SetUploadPaused: %v", err)
	}

	second, err := db.CreateUpload("/tmp/b.txt", 20, testMtime, "root", "Test Folder")
	if err != nil {
		t.Fatalf("CreateUpload #2: %v", err)
	}
	if err := db.SetUploadInProgress(second.ID); err == nil {
		t.Fatal("expected SetUploadInProgress to reject a concurrent upload while the first is paused")
	}

	if err := db.SetUploadAwaitingConfirmation(first.ID, AwaitingConfirmationSessionExpired); err != nil {
		t.Fatalf("SetUploadAwaitingConfirmation: %v", err)
	}
	if err := db.SetUploadInProgress(second.ID); err == nil {
		t.Fatal("expected SetUploadInProgress to reject a concurrent upload while the first is awaiting_confirmation")
	}
}

func TestSetUploadAwaitingConfirmationRejectsInvalidReason(t *testing.T) {
	db := newTestDB(t)
	u, err := db.CreateUpload("/tmp/file.txt", 1024, testMtime, "root", "Test Folder")
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	if err := db.SetUploadAwaitingConfirmation(u.ID, "not_a_real_reason"); err == nil {
		t.Fatal("expected SetUploadAwaitingConfirmation to reject an unrecognized reason")
	}
}

func TestResetUploadForRestartClearsSessionAndRefreshesBaseline(t *testing.T) {
	db := newTestDB(t)
	u, err := db.CreateUpload("/tmp/file.txt", 1024, testMtime, "root", "Test Folder")
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	_ = db.SetUploadInProgress(u.ID)
	_ = db.UpdateUploadProgress(u.ID, 512, "https://example.test/session/abc", []byte{9, 9}, 16*1024*1024, 1)
	if err := db.SetUploadAwaitingConfirmation(u.ID, AwaitingConfirmationFileChanged); err != nil {
		t.Fatalf("SetUploadAwaitingConfirmation: %v", err)
	}

	newMtime := testMtime.Add(time.Hour)
	if err := db.ResetUploadForRestart(u.ID, 2048, newMtime); err != nil {
		t.Fatalf("ResetUploadForRestart: %v", err)
	}

	got, err := db.GetUpload(u.ID)
	if err != nil {
		t.Fatalf("GetUpload: %v", err)
	}
	if got.Status != UploadInProgress {
		t.Fatalf("status = %q, want %q", got.Status, UploadInProgress)
	}
	if got.BytesSent != 0 {
		t.Fatalf("BytesSent = %d, want 0", got.BytesSent)
	}
	if got.SessionURI != nil {
		t.Fatalf("SessionURI = %v, want nil", got.SessionURI)
	}
	if got.ContentHashState != nil {
		t.Fatalf("ContentHashState = %v, want nil", got.ContentHashState)
	}
	if got.AwaitingConfirmationReason != nil {
		t.Fatalf("AwaitingConfirmationReason = %v, want nil", got.AwaitingConfirmationReason)
	}
	if got.LocalSizeBytes != 2048 {
		t.Fatalf("LocalSizeBytes = %d, want 2048", got.LocalSizeBytes)
	}
	if !got.LocalMtime.Equal(newMtime) {
		t.Fatalf("LocalMtime = %v, want %v", got.LocalMtime, newMtime)
	}
}

// TestResetUploadForRestartPreservesChunkSizeState covers User Story 3
// (FR-009, research.md §5): a byte-0 restart clears the session/offset/
// hash checkpoint but must leave the earned chunk-size state untouched,
// regardless of which awaiting_confirmation reason triggered it.
func TestResetUploadForRestartPreservesChunkSizeState(t *testing.T) {
	for _, reason := range []string{AwaitingConfirmationSessionExpired, AwaitingConfirmationFileChanged} {
		t.Run(reason, func(t *testing.T) {
			db := newTestDB(t)
			u, err := db.CreateUpload("/tmp/file.txt", 1024, testMtime, "root", "Test Folder")
			if err != nil {
				t.Fatalf("CreateUpload: %v", err)
			}
			_ = db.SetUploadInProgress(u.ID)
			if err := db.UpdateUploadProgress(u.ID, 512, "https://example.test/session/abc", []byte{9, 9}, 32*1024*1024, 2); err != nil {
				t.Fatalf("UpdateUploadProgress: %v", err)
			}
			if err := db.SetUploadAwaitingConfirmation(u.ID, reason); err != nil {
				t.Fatalf("SetUploadAwaitingConfirmation: %v", err)
			}

			if err := db.ResetUploadForRestart(u.ID, 2048, testMtime.Add(time.Hour)); err != nil {
				t.Fatalf("ResetUploadForRestart: %v", err)
			}

			got, err := db.GetUpload(u.ID)
			if err != nil {
				t.Fatalf("GetUpload: %v", err)
			}
			if got.BytesSent != 0 {
				t.Fatalf("BytesSent = %d, want reset to 0", got.BytesSent)
			}
			if got.SessionURI != nil {
				t.Fatalf("SessionURI = %v, want reset to nil", got.SessionURI)
			}
			if got.ContentHashState != nil {
				t.Fatalf("ContentHashState = %v, want reset to nil", got.ContentHashState)
			}
			if got.AwaitingConfirmationReason != nil {
				t.Fatalf("AwaitingConfirmationReason = %v, want reset to nil", got.AwaitingConfirmationReason)
			}
			if got.ChunkSizeBytes != 32*1024*1024 {
				t.Fatalf("ChunkSizeBytes = %d, want preserved at %d, not reset to baseline", got.ChunkSizeBytes, 32*1024*1024)
			}
			if got.ConsecutiveChunkSuccesses != 2 {
				t.Fatalf("ConsecutiveChunkSuccesses = %d, want preserved at 2", got.ConsecutiveChunkSuccesses)
			}
		})
	}
}

func TestResetUploadForRestartRejectsWhenNotAwaitingConfirmation(t *testing.T) {
	db := newTestDB(t)
	u, err := db.CreateUpload("/tmp/file.txt", 1024, testMtime, "root", "Test Folder")
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	_ = db.SetUploadInProgress(u.ID)

	if err := db.ResetUploadForRestart(u.ID, 2048, testMtime); err == nil {
		t.Fatal("expected ResetUploadForRestart to reject an upload that isn't awaiting_confirmation")
	}
}

func TestSetUploadCancelledOnlyFromPausedOrAwaitingConfirmation(t *testing.T) {
	db := newTestDB(t)
	u, err := db.CreateUpload("/tmp/file.txt", 1024, testMtime, "root", "Test Folder")
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	if err := db.SetUploadCancelled(u.ID); err == nil {
		t.Fatal("expected SetUploadCancelled to reject a pending (non-paused/awaiting_confirmation) upload")
	}

	_ = db.SetUploadInProgress(u.ID)
	if err := db.SetUploadCancelled(u.ID); err == nil {
		t.Fatal("expected SetUploadCancelled to reject an in_progress upload")
	}

	_ = db.SetUploadPaused(u.ID)
	if err := db.SetUploadCancelled(u.ID); err != nil {
		t.Fatalf("SetUploadCancelled from paused: %v", err)
	}
	got, err := db.GetUpload(u.ID)
	if err != nil {
		t.Fatalf("GetUpload: %v", err)
	}
	if got.Status != UploadCancelled {
		t.Fatalf("status = %q, want %q", got.Status, UploadCancelled)
	}
	if got.EndedAt == nil {
		t.Fatal("EndedAt must be set on cancellation")
	}

	// A cancelled upload frees FR-013's single-active-upload slot.
	second, err := db.CreateUpload("/tmp/b.txt", 20, testMtime, "root", "Test Folder")
	if err != nil {
		t.Fatalf("CreateUpload #2: %v", err)
	}
	if err := db.SetUploadInProgress(second.ID); err != nil {
		t.Fatalf("SetUploadInProgress #2 after cancellation: %v", err)
	}
}

func TestGetRecoverableUploadNormalizesStaleInProgress(t *testing.T) {
	db := newTestDB(t)
	u, err := db.CreateUpload("/tmp/file.txt", 1024, testMtime, "root", "Test Folder")
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	if err := db.SetUploadInProgress(u.ID); err != nil {
		t.Fatalf("SetUploadInProgress: %v", err)
	}
	_ = db.UpdateUploadProgress(u.ID, 256, "https://example.test/session/abc", []byte{1}, 8*1024*1024, 2)

	// Simulates a process restart against the same DB file: the row is
	// still in_progress since the process died before persisting a paused
	// transition.
	got, err := db.GetRecoverableUpload()
	if err != nil {
		t.Fatalf("GetRecoverableUpload: %v", err)
	}
	if got == nil {
		t.Fatal("expected a recoverable upload, got nil")
	}
	if got.ID != u.ID {
		t.Fatalf("recovered upload ID = %d, want %d", got.ID, u.ID)
	}
	if got.Status != UploadPaused {
		t.Fatalf("recovered status = %q, want normalized %q", got.Status, UploadPaused)
	}
	if got.BytesSent != 256 {
		t.Fatalf("BytesSent = %d, want 256 (checkpoint preserved)", got.BytesSent)
	}

	persisted, err := db.GetUpload(u.ID)
	if err != nil {
		t.Fatalf("GetUpload: %v", err)
	}
	if persisted.Status != UploadPaused {
		t.Fatalf("persisted status = %q, want %q", persisted.Status, UploadPaused)
	}
}

// TestGetRecoverableUploadSurvivesProcessRestart simulates User Story 2's
// crash/restart scenario at the storage layer: closes the DB connection
// entirely (as if the process had died) and reopens a brand-new *DB
// against the same SQLite file, confirming the interrupted upload's
// checkpoint (bytes_sent, session_uri, content_hash_state) survives and is
// detected as recoverable by the new instance.
func TestGetRecoverableUploadSurvivesProcessRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "restart-test.db")

	db1, err := OpenAt(dbPath)
	if err != nil {
		t.Fatalf("OpenAt (first process): %v", err)
	}
	u, err := db1.CreateUpload("/tmp/file.txt", 4096, testMtime, "root", "Test Folder")
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	if err := db1.SetUploadInProgress(u.ID); err != nil {
		t.Fatalf("SetUploadInProgress: %v", err)
	}
	hashState := []byte{5, 6, 7, 8}
	if err := db1.UpdateUploadProgress(u.ID, 2048, "https://example.test/session/xyz", hashState, 32*1024*1024, 2); err != nil {
		t.Fatalf("UpdateUploadProgress: %v", err)
	}
	// The process dies before it can persist a paused transition -- close
	// the connection without ever calling SetUploadPaused.
	if err := db1.Close(); err != nil {
		t.Fatalf("Close (simulating process death): %v", err)
	}

	db2, err := OpenAt(dbPath)
	if err != nil {
		t.Fatalf("OpenAt (second process, same file): %v", err)
	}
	t.Cleanup(func() { db2.Close() })

	got, err := db2.GetRecoverableUpload()
	if err != nil {
		t.Fatalf("GetRecoverableUpload (second process): %v", err)
	}
	if got == nil {
		t.Fatal("expected the interrupted upload to be recoverable after a restart")
	}
	if got.ID != u.ID {
		t.Fatalf("recovered upload ID = %d, want %d", got.ID, u.ID)
	}
	if got.Status != UploadPaused {
		t.Fatalf("recovered status = %q, want normalized %q", got.Status, UploadPaused)
	}
	if got.BytesSent != 2048 {
		t.Fatalf("BytesSent = %d, want 2048 (no progress lost)", got.BytesSent)
	}
	if got.SessionURI == nil || *got.SessionURI != "https://example.test/session/xyz" {
		t.Fatalf("SessionURI = %v, want the persisted session URI", got.SessionURI)
	}
	if string(got.ContentHashState) != string(hashState) {
		t.Fatalf("ContentHashState = %v, want %v", got.ContentHashState, hashState)
	}
}

func TestGetRecoverableUploadReturnsNilWhenNoneOutstanding(t *testing.T) {
	db := newTestDB(t)
	got, err := db.GetRecoverableUpload()
	if err != nil {
		t.Fatalf("GetRecoverableUpload: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}

	u, err := db.CreateUpload("/tmp/file.txt", 1024, testMtime, "root", "Test Folder")
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	_ = db.SetUploadInProgress(u.ID)
	_ = db.SetUploadSucceeded(u.ID, "id", "https://drive.google.com/file/d/xyz")

	got, err = db.GetRecoverableUpload()
	if err != nil {
		t.Fatalf("GetRecoverableUpload: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for a succeeded upload, got %+v", got)
	}
}

// TestListRecentUploadsOrdersByStartedAtDescending covers data-model.md's
// ListRecentUploads ordering guarantee (most recent first).
func TestListRecentUploadsOrdersByStartedAtDescending(t *testing.T) {
	db := newTestDB(t)

	first, err := db.CreateUpload("/tmp/a.txt", 10, testMtime, "root", "My Drive")
	if err != nil {
		t.Fatalf("CreateUpload first: %v", err)
	}
	second, err := db.CreateUpload("/tmp/b.txt", 20, testMtime, "root", "My Drive")
	if err != nil {
		t.Fatalf("CreateUpload second: %v", err)
	}
	third, err := db.CreateUpload("/tmp/c.txt", 30, testMtime, "root", "My Drive")
	if err != nil {
		t.Fatalf("CreateUpload third: %v", err)
	}

	got, err := db.ListRecentUploads()
	if err != nil {
		t.Fatalf("ListRecentUploads: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	wantOrder := []int64{third.ID, second.ID, first.ID}
	for i, want := range wantOrder {
		if got[i].ID != want {
			t.Fatalf("got[%d].ID = %d, want %d (most-recent-first order)", i, got[i].ID, want)
		}
	}
}

// TestListRecentUploadsCapsAtFiftyRows covers data-model.md's fixed
// 50-row limit (not user-configurable or paginated in this feature's scope).
func TestListRecentUploadsCapsAtFiftyRows(t *testing.T) {
	db := newTestDB(t)

	const total = 55
	for i := 0; i < total; i++ {
		if _, err := db.CreateUpload("/tmp/many.txt", 10, testMtime, "root", "My Drive"); err != nil {
			t.Fatalf("CreateUpload #%d: %v", i, err)
		}
	}

	got, err := db.ListRecentUploads()
	if err != nil {
		t.Fatalf("ListRecentUploads: %v", err)
	}
	if len(got) != recentUploadsLimit {
		t.Fatalf("len(got) = %d, want %d", len(got), recentUploadsLimit)
	}
}

// TestListRecentUploadsDriveFolderNameFallback covers data-model.md's
// UploadListItemDTO note: a null drive_folder_name -- the shape of a row
// created before this migration ran, simulated here with a direct insert
// since CreateUpload always populates the column going forward -- must
// round-trip as nil from ListRecentUploads, not a defaulted string. The
// "My Drive" text fallback itself is applied once, at the DTO boundary
// that owns display formatting (app.go's UploadListRecent), not here.
func TestListRecentUploadsDriveFolderNameFallback(t *testing.T) {
	db := newTestDB(t)

	withName, err := db.CreateUpload("/tmp/a.txt", 10, testMtime, "root", "Team Docs")
	if err != nil {
		t.Fatalf("CreateUpload withName: %v", err)
	}

	res, err := db.conn.Exec(`
		INSERT INTO upload (local_path, local_size_bytes, local_mtime, drive_folder_id, status, bytes_sent, started_at)
		VALUES (?, ?, ?, ?, ?, 0, ?)
	`, "/tmp/pre-migration.txt", 20, formatTime(testMtime), "root", string(UploadPending), formatTime(testMtime))
	if err != nil {
		t.Fatalf("insert pre-migration-shape row: %v", err)
	}
	preMigrationID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	got, err := db.ListRecentUploads()
	if err != nil {
		t.Fatalf("ListRecentUploads: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}

	byID := map[int64]*Upload{got[0].ID: got[0], got[1].ID: got[1]}

	named := byID[withName.ID]
	if named.DriveFolderName == nil || *named.DriveFolderName != "Team Docs" {
		t.Fatalf("named.DriveFolderName = %v, want \"Team Docs\"", named.DriveFolderName)
	}

	preMigration := byID[preMigrationID]
	if preMigration.DriveFolderName != nil {
		t.Fatalf("preMigration.DriveFolderName = %v, want nil", *preMigration.DriveFolderName)
	}
}
