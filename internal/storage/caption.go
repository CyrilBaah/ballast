package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CaptionStatus is a CaptionJob's overall state (Feature 005 data-model.md).
type CaptionStatus string

const (
	CaptionWaiting    CaptionStatus = "waiting"
	CaptionInProgress CaptionStatus = "in_progress"
	CaptionDone       CaptionStatus = "done"
	CaptionFailed     CaptionStatus = "failed"
	CaptionCancelled  CaptionStatus = "cancelled"
)

// CaptionPhase is where a waiting or in-progress CaptionJob is; PhaseNone
// (stored as NULL) once the job has ended.
type CaptionPhase string

const (
	PhaseNone              CaptionPhase = ""
	PhaseAwaitingConsent   CaptionPhase = "awaiting_consent"
	PhaseDownloadingModel  CaptionPhase = "downloading_model"
	PhaseExtractingAudio   CaptionPhase = "extracting_audio"
	PhaseTranscribing      CaptionPhase = "transcribing"
	PhaseWaitingForVideo   CaptionPhase = "waiting_for_video"
	PhaseUploadingCaptions CaptionPhase = "uploading_captions"
)

// ErrCaptionJobNotFound is returned when no CaptionJob matches.
var ErrCaptionJobNotFound = errors.New("storage: caption job not found")

// ErrCaptionJobEnded is returned when a transition targets a job that is
// already done, failed, or cancelled.
var ErrCaptionJobEnded = errors.New("storage: caption job has already ended")

// CaptionJob is one row of caption_job.
type CaptionJob struct {
	ID              int64
	UploadID        int64
	Status          CaptionStatus
	Phase           CaptionPhase
	Language        string
	ProgressPercent int
	AudioDurationMs *int64
	PieceCount      *int
	PiecesDone      int
	DriveFileID     *string
	DriveFileLink   *string
	DriveFileName   *string
	LocalCopyPath   *string
	Note            *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	EndedAt         *time.Time
}

func (s CaptionStatus) ended() bool {
	return s == CaptionDone || s == CaptionFailed || s == CaptionCancelled
}

// CreateCaptionJob adds the caption job for uploadID. An ended status
// must have no phase, and a failed one must carry a note (data-model.md's
// validation rules); a waiting or in-progress one must have a phase.
func (d *DB) CreateCaptionJob(uploadID int64, language string, status CaptionStatus, phase CaptionPhase, note string) (*CaptionJob, error) {
	if language != "en" && language != "auto" {
		return nil, fmt.Errorf("storage: unsupported caption language %q", language)
	}
	if status.ended() != (phase == PhaseNone) {
		return nil, fmt.Errorf("storage: caption job status %s cannot have phase %q", status, phase)
	}
	if status == CaptionFailed && note == "" {
		return nil, fmt.Errorf("storage: a failed caption job needs a note")
	}
	now := formatTime(time.Now())
	var ended any
	if status.ended() {
		ended = now
	}
	res, err := d.conn.Exec(`
		INSERT INTO caption_job (upload_id, status, phase, language, note, created_at, updated_at, ended_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, uploadID, string(status), nullString(string(phase)), language, nullString(note), now, now, ended)
	if err != nil {
		return nil, fmt.Errorf("storage: create caption job: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("storage: get new caption job id: %w", err)
	}
	return d.GetCaptionJob(id)
}

const captionJobColumns = `id, upload_id, status, phase, language, progress_percent, audio_duration_ms,
	piece_count, pieces_done, drive_file_id, drive_file_link, drive_file_name, local_copy_path, note,
	created_at, updated_at, ended_at`

// GetCaptionJob returns the caption job with the given id.
func (d *DB) GetCaptionJob(id int64) (*CaptionJob, error) {
	return d.scanCaptionJob(d.conn.QueryRow(`SELECT `+captionJobColumns+` FROM caption_job WHERE id = ?`, id))
}

// GetCaptionJobByUpload returns uploadID's caption job, or
// ErrCaptionJobNotFound if it has none.
func (d *DB) GetCaptionJobByUpload(uploadID int64) (*CaptionJob, error) {
	return d.scanCaptionJob(d.conn.QueryRow(`SELECT `+captionJobColumns+` FROM caption_job WHERE upload_id = ?`, uploadID))
}

// ListActiveCaptionJobs returns every waiting or in-progress job, oldest
// first -- the order the worker takes them in.
func (d *DB) ListActiveCaptionJobs() ([]*CaptionJob, error) {
	rows, err := d.conn.Query(`SELECT `+captionJobColumns+` FROM caption_job WHERE status IN (?, ?) ORDER BY id`,
		string(CaptionWaiting), string(CaptionInProgress))
	if err != nil {
		return nil, fmt.Errorf("storage: list active caption jobs: %w", err)
	}
	defer rows.Close()
	var jobs []*CaptionJob
	for rows.Next() {
		j, err := d.scanCaptionJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// SetCaptionPhase moves a not-yet-ended job to status/phase and resets
// its phase progress.
func (d *DB) SetCaptionPhase(id int64, status CaptionStatus, phase CaptionPhase) error {
	if status.ended() || phase == PhaseNone {
		return fmt.Errorf("storage: SetCaptionPhase needs a waiting/in-progress status and a phase")
	}
	return d.updateActiveCaptionJob(id, `status = ?, phase = ?, progress_percent = 0`, string(status), string(phase))
}

// SetCaptionProgress records progress (0-100) within the current phase.
func (d *DB) SetCaptionProgress(id int64, percent int) error {
	return d.updateActiveCaptionJob(id, `progress_percent = ?`, percent)
}

// SetCaptionPieces records the audio length and how many pieces it was split into.
func (d *DB) SetCaptionPieces(id int64, audioDurationMs int64, pieceCount int) error {
	return d.updateActiveCaptionJob(id, `audio_duration_ms = ?, piece_count = ?, pieces_done = 0`, audioDurationMs, pieceCount)
}

// SetCaptionPiecesDone checkpoints how many pieces' transcripts are saved.
func (d *DB) SetCaptionPiecesDone(id int64, n int) error {
	j, err := d.GetCaptionJob(id)
	if err != nil {
		return err
	}
	if j.PieceCount == nil || n < 0 || n > *j.PieceCount {
		return fmt.Errorf("storage: pieces_done %d is out of range", n)
	}
	return d.updateActiveCaptionJob(id, `pieces_done = ?`, n)
}

// SetCaptionLocalCopy records where the caption file was saved on this computer.
func (d *DB) SetCaptionLocalCopy(id int64, path string) error {
	return d.updateActiveCaptionJob(id, `local_copy_path = ?`, path)
}

// SetCaptionDone ends a job successfully. driveFileID and driveFileLink are
// both set (a caption file in Drive) or both empty (no speech, with note).
func (d *DB) SetCaptionDone(id int64, driveFileID, driveFileLink, driveFileName, note string) error {
	if (driveFileID == "") != (driveFileLink == "") {
		return fmt.Errorf("storage: drive file id and link must be set together")
	}
	return d.updateActiveCaptionJob(id,
		`status = ?, phase = NULL, drive_file_id = ?, drive_file_link = ?, drive_file_name = ?, note = ?, ended_at = ?`,
		string(CaptionDone), nullString(driveFileID), nullString(driveFileLink), nullString(driveFileName), nullString(note), formatTime(time.Now()))
}

// SetCaptionFailed ends a job with a plain-language reason (FR-007).
func (d *DB) SetCaptionFailed(id int64, note string) error {
	if note == "" {
		return fmt.Errorf("storage: a failed caption job needs a note")
	}
	return d.updateActiveCaptionJob(id, `status = ?, phase = NULL, note = ?, ended_at = ?`,
		string(CaptionFailed), note, formatTime(time.Now()))
}

// SetCaptionCancelled ends a job because its upload was cancelled, failed,
// or captions were declined. local_copy_path is kept (FR-022).
func (d *DB) SetCaptionCancelled(id int64, note string) error {
	return d.updateActiveCaptionJob(id, `status = ?, phase = NULL, note = ?, ended_at = ?`,
		string(CaptionCancelled), nullString(note), formatTime(time.Now()))
}

// updateActiveCaptionJob applies set to job id only while it is waiting or
// in progress, stamping updated_at.
func (d *DB) updateActiveCaptionJob(id int64, set string, args ...any) error {
	args = append(args, formatTime(time.Now()), id, string(CaptionWaiting), string(CaptionInProgress))
	res, err := d.conn.Exec(`UPDATE caption_job SET `+set+`, updated_at = ? WHERE id = ? AND status IN (?, ?)`, args...)
	if err != nil {
		return fmt.Errorf("storage: update caption job: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: check rows affected: %w", err)
	}
	if n == 0 {
		if _, err := d.GetCaptionJob(id); err != nil {
			return err
		}
		return ErrCaptionJobEnded
	}
	return nil
}

type rowScanner interface{ Scan(dest ...any) error }

func (d *DB) scanCaptionJob(row rowScanner) (*CaptionJob, error) {
	var (
		j                                      CaptionJob
		status, language, created, updated     string
		phase, fileID, link, name, local, note sql.NullString
		ended                                  sql.NullString
		duration                               sql.NullInt64
		pieces                                 sql.NullInt64
	)
	err := row.Scan(&j.ID, &j.UploadID, &status, &phase, &language, &j.ProgressPercent, &duration,
		&pieces, &j.PiecesDone, &fileID, &link, &name, &local, &note, &created, &updated, &ended)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCaptionJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: scan caption job: %w", err)
	}
	j.Status = CaptionStatus(status)
	j.Phase = CaptionPhase(phase.String)
	j.Language = language
	if duration.Valid {
		j.AudioDurationMs = &duration.Int64
	}
	if pieces.Valid {
		n := int(pieces.Int64)
		j.PieceCount = &n
	}
	j.DriveFileID = stringPtr(fileID)
	j.DriveFileLink = stringPtr(link)
	j.DriveFileName = stringPtr(name)
	j.LocalCopyPath = stringPtr(local)
	j.Note = stringPtr(note)
	if j.CreatedAt, err = parseTime(created); err != nil {
		return nil, fmt.Errorf("storage: parse caption job created_at: %w", err)
	}
	if j.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, fmt.Errorf("storage: parse caption job updated_at: %w", err)
	}
	if ended.Valid {
		t, err := parseTime(ended.String)
		if err != nil {
			return nil, fmt.Errorf("storage: parse caption job ended_at: %w", err)
		}
		j.EndedAt = &t
	}
	return &j, nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func stringPtr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}
