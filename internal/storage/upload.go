package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// UploadState is one of the four states an Upload row can be in
// (data-model.md).
type UploadState string

const (
	UploadPending    UploadState = "pending"
	UploadInProgress UploadState = "in_progress"
	UploadSucceeded  UploadState = "succeeded"
	UploadFailed     UploadState = "failed"
)

// Upload is the in-memory representation of one Upload row.
type Upload struct {
	ID             int64
	LocalPath      string
	LocalSizeBytes int64
	DriveFolderID  string
	Status         UploadState
	BytesSent      int64
	DriveFileID    *string
	DriveFileLink  *string
	FailureReason  *string
	StartedAt      time.Time
	EndedAt        *time.Time
}

// ErrUploadNotFound is returned when no Upload row exists with the given
// ID.
var ErrUploadNotFound = errors.New("storage: upload not found")

// ErrUploadAlreadyInProgress is returned by SetUploadInProgress when
// another Upload row is already in_progress. data-model.md: "Exactly one
// Upload may be in_progress at a time (this feature has no concurrency)."
var ErrUploadAlreadyInProgress = errors.New("storage: another upload is already in progress")

// CreateUpload inserts a new Upload row in the pending state
// (contracts/wails-bindings.md's Upload.Start: "creates an Upload row
// (pending -> in_progress)").
func (d *DB) CreateUpload(localPath string, localSizeBytes int64, driveFolderID string) (*Upload, error) {
	now := time.Now()
	res, err := d.conn.Exec(`
		INSERT INTO upload (local_path, local_size_bytes, drive_folder_id, status, bytes_sent, started_at)
		VALUES (?, ?, ?, ?, 0, ?)
	`, localPath, localSizeBytes, driveFolderID, string(UploadPending), formatTime(now))
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
		DriveFolderID:  driveFolderID,
		Status:         UploadPending,
		StartedAt:      now,
	}, nil
}

// SetUploadInProgress transitions an Upload from pending to in_progress.
// Rejects with ErrUploadAlreadyInProgress if another upload is already
// in_progress (data-model.md's no-concurrency validation rule).
func (d *DB) SetUploadInProgress(id int64) error {
	var inProgressCount int
	row := d.conn.QueryRow(`SELECT COUNT(*) FROM upload WHERE status = ? AND id != ?`, string(UploadInProgress), id)
	if err := row.Scan(&inProgressCount); err != nil {
		return fmt.Errorf("storage: check concurrent uploads: %w", err)
	}
	if inProgressCount > 0 {
		return ErrUploadAlreadyInProgress
	}

	res, err := d.conn.Exec(`UPDATE upload SET status = ? WHERE id = ?`, string(UploadInProgress), id)
	if err != nil {
		return fmt.Errorf("storage: set upload in_progress: %w", err)
	}
	return requireRowsAffected(res)
}

// UpdateUploadProgress records the latest bytes-sent count for an
// in-progress upload (FR-007's progress indicator).
func (d *DB) UpdateUploadProgress(id int64, bytesSent int64) error {
	res, err := d.conn.Exec(`UPDATE upload SET bytes_sent = ? WHERE id = ?`, bytesSent, id)
	if err != nil {
		return fmt.Errorf("storage: update upload progress: %w", err)
	}
	return requireRowsAffected(res)
}

// SetUploadSucceeded transitions an Upload to succeeded. Both driveFileID
// and driveFileLink are required (data-model.md: "A succeeded row MUST
// have both drive_file_id and drive_file_link populated").
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

// SetUploadFailed transitions an Upload to failed. reason is required
// (data-model.md: "A failed row MUST have failure_reason populated").
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

// GetUpload returns a single Upload row by ID.
func (d *DB) GetUpload(id int64) (*Upload, error) {
	row := d.conn.QueryRow(`
		SELECT local_path, local_size_bytes, drive_folder_id, status, bytes_sent,
			drive_file_id, drive_file_link, failure_reason, started_at, ended_at
		FROM upload WHERE id = ?
	`, id)

	var u Upload
	var status string
	var startedAt string
	var endedAt sql.NullString
	u.ID = id
	err := row.Scan(
		&u.LocalPath, &u.LocalSizeBytes, &u.DriveFolderID, &status, &u.BytesSent,
		&u.DriveFileID, &u.DriveFileLink, &u.FailureReason, &startedAt, &endedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUploadNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get upload: %w", err)
	}
	u.Status = UploadState(status)
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
