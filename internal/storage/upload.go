package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// UploadState is one of the states an Upload row can be in (data-model.md).
type UploadState string

const (
	UploadPending              UploadState = "pending"
	UploadInProgress           UploadState = "in_progress"
	UploadPaused               UploadState = "paused"
	UploadAwaitingConfirmation UploadState = "awaiting_confirmation"
	UploadCancelled            UploadState = "cancelled"
	UploadSucceeded            UploadState = "succeeded"
	UploadFailed               UploadState = "failed"
)

// Reason values for awaiting_confirmation_reason (data-model.md).
const (
	AwaitingConfirmationSessionExpired = "session_expired"
	AwaitingConfirmationFileChanged    = "file_changed"
)

// nonTerminalStatuses are the statuses that count toward FR-013's
// single-active-upload constraint and are eligible for crash recovery.
var nonTerminalStatuses = []UploadState{UploadInProgress, UploadPaused, UploadAwaitingConfirmation}

// Upload is the in-memory representation of one Upload row.
type Upload struct {
	ID                         int64
	LocalPath                  string
	LocalSizeBytes             int64
	LocalMtime                 time.Time
	DriveFolderID              string
	Status                     UploadState
	BytesSent                  int64
	SessionURI                 *string
	ContentHashState           []byte
	AwaitingConfirmationReason *string
	DriveFileID                *string
	DriveFileLink              *string
	FailureReason              *string
	StartedAt                  time.Time
	EndedAt                    *time.Time
}

// ErrUploadNotFound is returned when no Upload row exists with the given ID.
var ErrUploadNotFound = errors.New("storage: upload not found")

// ErrUploadAlreadyInProgress is returned by SetUploadInProgress when
// another Upload row is already in_progress/paused/awaiting_confirmation;
// only one upload can be non-terminal at a time (FR-013).
var ErrUploadAlreadyInProgress = errors.New("storage: another upload is already in progress")

// ErrUploadNotAwaitingConfirmation is returned by ResetUploadForRestart
// when the target upload isn't currently awaiting_confirmation.
var ErrUploadNotAwaitingConfirmation = errors.New("storage: upload is not awaiting confirmation")

// ErrUploadNotCancellable is returned by SetUploadCancelled when the
// target upload isn't currently paused or awaiting_confirmation (FR-014).
var ErrUploadNotCancellable = errors.New("storage: upload is not in a cancellable state")

// CreateUpload inserts a new Upload row in the pending state, capturing
// localMtime alongside localSizeBytes as the source-file-identity check's
// cheap-check baseline (data-model.md, research.md §5).
func (d *DB) CreateUpload(localPath string, localSizeBytes int64, localMtime time.Time, driveFolderID string) (*Upload, error) {
	now := time.Now()
	res, err := d.conn.Exec(`
		INSERT INTO upload (local_path, local_size_bytes, local_mtime, drive_folder_id, status, bytes_sent, started_at)
		VALUES (?, ?, ?, ?, ?, 0, ?)
	`, localPath, localSizeBytes, formatTime(localMtime), driveFolderID, string(UploadPending), formatTime(now))
	if err != nil {
		return nil, fmt.Errorf("storage: create upload: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("storage: get new upload id: %w", err)
	}
	return &Upload{
		ID:             id,
		LocalPath:      localPath,
		LocalSizeBytes: localSizeBytes,
		LocalMtime:     localMtime,
		DriveFolderID:  driveFolderID,
		Status:         UploadPending,
		StartedAt:      now,
	}, nil
}

// SetUploadInProgress transitions an Upload from pending to in_progress.
// Rejects with ErrUploadAlreadyInProgress if another upload is already
// in_progress, paused, or awaiting_confirmation -- FR-013's
// single-active-upload constraint spans all three non-terminal states.
func (d *DB) SetUploadInProgress(id int64) error {
	placeholders := make([]any, 0, len(nonTerminalStatuses)+1)
	placeholders = append(placeholders, id)
	inClause := ""
	for i, s := range nonTerminalStatuses {
		if i > 0 {
			inClause += ", "
		}
		inClause += "?"
		placeholders = append(placeholders, string(s))
	}

	var conflictCount int
	row := d.conn.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM upload WHERE id != ? AND status IN (%s)`, inClause), placeholders...)
	if err := row.Scan(&conflictCount); err != nil {
		return fmt.Errorf("storage: check concurrent uploads: %w", err)
	}
	if conflictCount > 0 {
		return ErrUploadAlreadyInProgress
	}

	res, err := d.conn.Exec(`UPDATE upload SET status = ? WHERE id = ?`, string(UploadInProgress), id)
	if err != nil {
		return fmt.Errorf("storage: set upload in_progress: %w", err)
	}
	return requireRowsAffected(res)
}

// SetUploadResumed transitions a paused (or still in_progress) Upload back
// to in_progress after a retry succeeds.
func (d *DB) SetUploadResumed(id int64) error {
	res, err := d.conn.Exec(`UPDATE upload SET status = ? WHERE id = ?`, string(UploadInProgress), id)
	if err != nil {
		return fmt.Errorf("storage: set upload resumed: %w", err)
	}
	return requireRowsAffected(res)
}

// SetUploadPaused transitions an Upload to paused when a retryable error
// is hit (research.md §4). Paused is not a failure state (FR-003, FR-007).
func (d *DB) SetUploadPaused(id int64) error {
	res, err := d.conn.Exec(`UPDATE upload SET status = ? WHERE id = ?`, string(UploadPaused), id)
	if err != nil {
		return fmt.Errorf("storage: set upload paused: %w", err)
	}
	return requireRowsAffected(res)
}

// SetUploadAwaitingConfirmation transitions an Upload to
// awaiting_confirmation with the given reason (must be one of the
// AwaitingConfirmation* constants) when a terminal-but-recoverable
// condition is detected (research.md §4).
func (d *DB) SetUploadAwaitingConfirmation(id int64, reason string) error {
	if reason != AwaitingConfirmationSessionExpired && reason != AwaitingConfirmationFileChanged {
		return fmt.Errorf("storage: SetUploadAwaitingConfirmation: invalid reason %q", reason)
	}
	res, err := d.conn.Exec(`
		UPDATE upload SET status = ?, awaiting_confirmation_reason = ? WHERE id = ?
	`, string(UploadAwaitingConfirmation), reason, id)
	if err != nil {
		return fmt.Errorf("storage: set upload awaiting_confirmation: %w", err)
	}
	return requireRowsAffected(res)
}

// UpdateUploadProgress records the latest Drive-acknowledged bytes-sent
// offset together with the session URI and checkpointed content-hash
// state, atomically -- these three fields must always move together
// (data-model.md's correctness invariant).
func (d *DB) UpdateUploadProgress(id int64, bytesSent int64, sessionURI string, contentHashState []byte) error {
	res, err := d.conn.Exec(`
		UPDATE upload SET bytes_sent = ?, session_uri = ?, content_hash_state = ? WHERE id = ?
	`, bytesSent, sessionURI, contentHashState, id)
	if err != nil {
		return fmt.Errorf("storage: update upload progress: %w", err)
	}
	return requireRowsAffected(res)
}

// SetUploadSucceeded transitions an Upload to succeeded. Both driveFileID and driveFileLink are required.
func (d *DB) SetUploadSucceeded(id int64, driveFileID, driveFileLink string) error {
	if driveFileID == "" || driveFileLink == "" {
		return fmt.Errorf("storage: SetUploadSucceeded requires a non-empty driveFileID and driveFileLink")
	}
	res, err := d.conn.Exec(`
		UPDATE upload
		SET status = ?, drive_file_id = ?, drive_file_link = ?, ended_at = ?
		WHERE id = ?
	`, string(UploadSucceeded), driveFileID, driveFileLink, formatTime(time.Now()), id)
	if err != nil {
		return fmt.Errorf("storage: set upload succeeded: %w", err)
	}
	return requireRowsAffected(res)
}

// SetUploadFailed transitions an Upload to failed. reason is required.
func (d *DB) SetUploadFailed(id int64, reason string) error {
	if reason == "" {
		return fmt.Errorf("storage: SetUploadFailed requires a non-empty reason")
	}
	res, err := d.conn.Exec(`
		UPDATE upload
		SET status = ?, failure_reason = ?, ended_at = ?
		WHERE id = ?
	`, string(UploadFailed), reason, formatTime(time.Now()), id)
	if err != nil {
		return fmt.Errorf("storage: set upload failed: %w", err)
	}
	return requireRowsAffected(res)
}

// SetUploadCancelled transitions a paused or awaiting_confirmation Upload
// directly to the terminal cancelled state (FR-014). Rejects with
// ErrUploadNotCancellable for any other status.
func (d *DB) SetUploadCancelled(id int64) error {
	res, err := d.conn.Exec(`
		UPDATE upload
		SET status = ?, ended_at = ?
		WHERE id = ? AND status IN (?, ?)
	`, string(UploadCancelled), formatTime(time.Now()), id, string(UploadPaused), string(UploadAwaitingConfirmation))
	if err != nil {
		return fmt.Errorf("storage: set upload cancelled: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: check rows affected: %w", err)
	}
	if n == 0 {
		if _, getErr := d.GetUpload(id); getErr != nil {
			return getErr
		}
		return ErrUploadNotCancellable
	}
	return nil
}

// ResetUploadForRestart clears session_uri/bytes_sent/content_hash_state
// and refreshes the local_size_bytes/local_mtime baseline to
// newSize/newMtime (a fresh os.Stat taken at confirmation time, since a
// changed or re-checked file is exactly what led here), then transitions
// the row back to in_progress -- restarting the same logical upload from
// byte 0 against a brand-new Drive session (data-model.md). Only valid
// when the upload is currently awaiting_confirmation (FR-010).
func (d *DB) ResetUploadForRestart(id int64, newSize int64, newMtime time.Time) error {
	res, err := d.conn.Exec(`
		UPDATE upload
		SET status = ?, session_uri = NULL, bytes_sent = 0, content_hash_state = NULL,
			awaiting_confirmation_reason = NULL, local_size_bytes = ?, local_mtime = ?
		WHERE id = ? AND status = ?
	`, string(UploadInProgress), newSize, formatTime(newMtime), id, string(UploadAwaitingConfirmation))
	if err != nil {
		return fmt.Errorf("storage: reset upload for restart: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: check rows affected: %w", err)
	}
	if n == 0 {
		if _, getErr := d.GetUpload(id); getErr != nil {
			return getErr
		}
		return ErrUploadNotAwaitingConfirmation
	}
	return nil
}

// GetRecoverableUpload returns the single non-terminal Upload left over
// from a previous run, if any (research.md §7) -- nil if there is none. A
// row still in_progress when the app starts (the process died before it
// could persist a paused transition) is normalized to paused first, since
// bytes_sent/content_hash_state/session_uri are exactly as trustworthy
// either way (data-model.md's startup-normalization rule).
func (d *DB) GetRecoverableUpload() (*Upload, error) {
	var id int64
	row := d.conn.QueryRow(`
		SELECT id FROM upload WHERE status IN (?, ?, ?) LIMIT 1
	`, string(UploadInProgress), string(UploadPaused), string(UploadAwaitingConfirmation))
	if err := row.Scan(&id); errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("storage: find recoverable upload: %w", err)
	}

	u, err := d.GetUpload(id)
	if err != nil {
		return nil, err
	}
	if u.Status == UploadInProgress {
		if err := d.SetUploadPaused(id); err != nil {
			return nil, fmt.Errorf("storage: normalize stale in_progress upload: %w", err)
		}
		u.Status = UploadPaused
	}
	return u, nil
}

// GetUpload returns a single Upload row by ID.
func (d *DB) GetUpload(id int64) (*Upload, error) {
	row := d.conn.QueryRow(`
		SELECT local_path, local_size_bytes, local_mtime, drive_folder_id, status, bytes_sent,
			session_uri, content_hash_state, awaiting_confirmation_reason,
			drive_file_id, drive_file_link, failure_reason, started_at, ended_at
		FROM upload WHERE id = ?
	`, id)

	var u Upload
	var status string
	var localMtime, startedAt string
	var sessionURI, awaitingReason, driveFileID, driveFileLink, failureReason sql.NullString
	var contentHashState []byte
	var endedAt sql.NullString
	u.ID = id
	err := row.Scan(
		&u.LocalPath, &u.LocalSizeBytes, &localMtime, &u.DriveFolderID, &status, &u.BytesSent,
		&sessionURI, &contentHashState, &awaitingReason,
		&driveFileID, &driveFileLink, &failureReason, &startedAt, &endedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUploadNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get upload: %w", err)
	}
	u.Status = UploadState(status)
	u.LocalMtime, err = parseTime(localMtime)
	if err != nil {
		return nil, fmt.Errorf("storage: parse local_mtime: %w", err)
	}
	u.StartedAt, err = parseTime(startedAt)
	if err != nil {
		return nil, fmt.Errorf("storage: parse started_at: %w", err)
	}
	if endedAt.Valid {
		t, err := parseTime(endedAt.String)
		if err != nil {
			return nil, fmt.Errorf("storage: parse ended_at: %w", err)
		}
		u.EndedAt = &t
	}
	if sessionURI.Valid {
		u.SessionURI = &sessionURI.String
	}
	if len(contentHashState) > 0 {
		u.ContentHashState = contentHashState
	}
	if awaitingReason.Valid {
		u.AwaitingConfirmationReason = &awaitingReason.String
	}
	if driveFileID.Valid {
		u.DriveFileID = &driveFileID.String
	}
	if driveFileLink.Valid {
		u.DriveFileLink = &driveFileLink.String
	}
	if failureReason.Valid {
		u.FailureReason = &failureReason.String
	}
	return &u, nil
}

func requireRowsAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: check rows affected: %w", err)
	}
	if n == 0 {
		return ErrUploadNotFound
	}
	return nil
}
