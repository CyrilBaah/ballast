package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SummaryStatus is a SummaryJob's overall state (Feature 006 data-model.md).
type SummaryStatus string

const (
	SummaryWaiting    SummaryStatus = "waiting"
	SummaryInProgress SummaryStatus = "in_progress"
	SummaryDone       SummaryStatus = "done"
	SummaryFailed     SummaryStatus = "failed"
	SummaryCancelled  SummaryStatus = "cancelled"
)

// SummaryPhase is where a waiting or in-progress SummaryJob is;
// SummaryPhaseNone (NULL) once it has ended.
type SummaryPhase string

const (
	SummaryPhaseNone               SummaryPhase = ""
	SummaryPhaseWaitingForCaptions SummaryPhase = "waiting_for_captions"
	SummaryPhaseAwaitingConsent    SummaryPhase = "awaiting_consent"
	SummaryPhaseDownloadingModel   SummaryPhase = "downloading_model"
	SummaryPhaseWaitingForEngine   SummaryPhase = "waiting_for_engine"
	SummaryPhaseSummarising        SummaryPhase = "summarising"
	SummaryPhaseWaitingForVideo    SummaryPhase = "waiting_for_video"
	SummaryPhaseUploadingSummary   SummaryPhase = "uploading_summary"
)

var (
	ErrSummaryJobNotFound = errors.New("storage: summary job not found")
	ErrSummaryJobEnded    = errors.New("storage: summary job has already ended")
)

// SummaryJob is one row of summary_job.
type SummaryJob struct {
	ID               int64
	UploadID         int64
	Status           SummaryStatus
	Phase            SummaryPhase
	Engine           string
	Model            string
	PartCount        *int
	PartsDone        int
	ProgressPercent  int
	Attempts         int
	UnverifiedQuotes int
	LocalCopyPath    *string
	DriveFileID      *string
	DriveFileLink    *string
	DriveFileName    *string
	Note             *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	EndedAt          *time.Time
}

func (s SummaryStatus) ended() bool {
	return s == SummaryDone || s == SummaryFailed || s == SummaryCancelled
}

// CreateSummaryJob adds uploadID's summary job, waiting for its captions.
func (d *DB) CreateSummaryJob(uploadID int64, model string) (*SummaryJob, error) {
	now := formatTime(time.Now())
	res, err := d.conn.Exec(`
		INSERT INTO summary_job (upload_id, status, phase, engine, model, created_at, updated_at)
		VALUES (?, ?, ?, 'local', ?, ?, ?)
	`, uploadID, string(SummaryWaiting), string(SummaryPhaseWaitingForCaptions), model, now, now)
	if err != nil {
		return nil, fmt.Errorf("storage: create summary job: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("storage: get new summary job id: %w", err)
	}
	return d.GetSummaryJob(id)
}

const summaryJobColumns = `id, upload_id, status, phase, engine, model, part_count, parts_done,
	progress_percent, attempts, unverified_quotes, local_copy_path, drive_file_id, drive_file_link,
	drive_file_name, note, created_at, updated_at, ended_at`

// GetSummaryJob returns the summary job with the given id.
func (d *DB) GetSummaryJob(id int64) (*SummaryJob, error) {
	return scanSummaryJob(d.conn.QueryRow(`SELECT `+summaryJobColumns+` FROM summary_job WHERE id = ?`, id))
}

// GetSummaryJobByUpload returns uploadID's summary job, or ErrSummaryJobNotFound.
func (d *DB) GetSummaryJobByUpload(uploadID int64) (*SummaryJob, error) {
	return scanSummaryJob(d.conn.QueryRow(`SELECT `+summaryJobColumns+` FROM summary_job WHERE upload_id = ?`, uploadID))
}

// ListActiveSummaryJobs returns waiting and in-progress jobs, oldest first.
func (d *DB) ListActiveSummaryJobs() ([]*SummaryJob, error) {
	rows, err := d.conn.Query(`SELECT `+summaryJobColumns+` FROM summary_job WHERE status IN (?, ?) ORDER BY id`,
		string(SummaryWaiting), string(SummaryInProgress))
	if err != nil {
		return nil, fmt.Errorf("storage: list active summary jobs: %w", err)
	}
	defer rows.Close()
	var jobs []*SummaryJob
	for rows.Next() {
		j, err := scanSummaryJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// SetSummaryPhase moves a not-yet-ended job to status/phase, resetting progress.
func (d *DB) SetSummaryPhase(id int64, status SummaryStatus, phase SummaryPhase) error {
	if status.ended() || phase == SummaryPhaseNone {
		return fmt.Errorf("storage: SetSummaryPhase needs a waiting/in-progress status and a phase")
	}
	return d.updateActiveSummaryJob(id, `status = ?, phase = ?, progress_percent = 0`, string(status), string(phase))
}

// SetSummaryProgress records progress (0-100) within the current phase.
func (d *DB) SetSummaryProgress(id int64, percent int) error {
	return d.updateActiveSummaryJob(id, `progress_percent = ?`, percent)
}

// SetSummaryParts records how many transcript parts the job has.
func (d *DB) SetSummaryParts(id int64, partCount int) error {
	return d.updateActiveSummaryJob(id, `part_count = ?, parts_done = 0`, partCount)
}

// SetSummaryPartsDone checkpoints how many parts' notes are saved.
func (d *DB) SetSummaryPartsDone(id int64, n int) error {
	j, err := d.GetSummaryJob(id)
	if err != nil {
		return err
	}
	if j.PartCount == nil || n < 0 || n > *j.PartCount {
		return fmt.Errorf("storage: parts_done %d is out of range", n)
	}
	return d.updateActiveSummaryJob(id, `parts_done = ?`, n)
}

// AddSummaryAttempt counts one more summarise attempt and returns the total.
func (d *DB) AddSummaryAttempt(id int64) (int, error) {
	if err := d.updateActiveSummaryJob(id, `attempts = attempts + 1`); err != nil {
		return 0, err
	}
	j, err := d.GetSummaryJob(id)
	if err != nil {
		return 0, err
	}
	return j.Attempts, nil
}

// SetSummaryUnverifiedQuotes records how many quotes the transcript check dropped.
func (d *DB) SetSummaryUnverifiedQuotes(id int64, n int) error {
	return d.updateActiveSummaryJob(id, `unverified_quotes = ?`, n)
}

// SetSummaryLocalCopy records where the summary was saved on this computer.
func (d *DB) SetSummaryLocalCopy(id int64, path string) error {
	return d.updateActiveSummaryJob(id, `local_copy_path = ?`, path)
}

// SetSummaryDone ends a job with its Google Doc.
func (d *DB) SetSummaryDone(id int64, driveFileID, driveFileLink, driveFileName string) error {
	if driveFileID == "" || driveFileLink == "" {
		return fmt.Errorf("storage: a done summary needs both its Drive file id and link")
	}
	return d.updateActiveSummaryJob(id,
		`status = ?, phase = NULL, drive_file_id = ?, drive_file_link = ?, drive_file_name = ?, ended_at = ?`,
		string(SummaryDone), driveFileID, driveFileLink, nullString(driveFileName), formatTime(time.Now()))
}

// SetSummaryFailed ends a job with a plain-language reason (FR-009).
func (d *DB) SetSummaryFailed(id int64, note string) error {
	if note == "" {
		return fmt.Errorf("storage: a failed summary job needs a note")
	}
	return d.updateActiveSummaryJob(id, `status = ?, phase = NULL, note = ?, ended_at = ?`,
		string(SummaryFailed), note, formatTime(time.Now()))
}

// SetSummaryCancelled ends a job; a local copy already saved is kept.
func (d *DB) SetSummaryCancelled(id int64, note string) error {
	return d.updateActiveSummaryJob(id, `status = ?, phase = NULL, note = ?, ended_at = ?`,
		string(SummaryCancelled), nullString(note), formatTime(time.Now()))
}

// ResetSummaryForRetry puts a failed job back to wait for the engine with
// its attempts cleared ("Try again", FR-009). Only failed jobs qualify.
func (d *DB) ResetSummaryForRetry(id int64) error {
	res, err := d.conn.Exec(`
		UPDATE summary_job
		SET status = ?, phase = ?, attempts = 0, note = NULL, ended_at = NULL, progress_percent = 0, updated_at = ?
		WHERE id = ? AND status = ?
	`, string(SummaryWaiting), string(SummaryPhaseWaitingForEngine), formatTime(time.Now()), id, string(SummaryFailed))
	if err != nil {
		return fmt.Errorf("storage: reset summary job: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("storage: only a failed summary can be tried again")
	}
	return nil
}

func (d *DB) updateActiveSummaryJob(id int64, set string, args ...any) error {
	args = append(args, formatTime(time.Now()), id, string(SummaryWaiting), string(SummaryInProgress))
	res, err := d.conn.Exec(`UPDATE summary_job SET `+set+`, updated_at = ? WHERE id = ? AND status IN (?, ?)`, args...)
	if err != nil {
		return fmt.Errorf("storage: update summary job: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: check rows affected: %w", err)
	}
	if n == 0 {
		if _, err := d.GetSummaryJob(id); err != nil {
			return err
		}
		return ErrSummaryJobEnded
	}
	return nil
}

func scanSummaryJob(row rowScanner) (*SummaryJob, error) {
	var (
		j                                      SummaryJob
		status, created, updated               string
		phase, local, fileID, link, name, note sql.NullString
		ended                                  sql.NullString
		parts                                  sql.NullInt64
	)
	err := row.Scan(&j.ID, &j.UploadID, &status, &phase, &j.Engine, &j.Model, &parts, &j.PartsDone,
		&j.ProgressPercent, &j.Attempts, &j.UnverifiedQuotes, &local, &fileID, &link, &name, &note,
		&created, &updated, &ended)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSummaryJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: scan summary job: %w", err)
	}
	j.Status = SummaryStatus(status)
	j.Phase = SummaryPhase(phase.String)
	if parts.Valid {
		n := int(parts.Int64)
		j.PartCount = &n
	}
	j.LocalCopyPath, j.DriveFileID, j.DriveFileLink, j.DriveFileName, j.Note =
		stringPtr(local), stringPtr(fileID), stringPtr(link), stringPtr(name), stringPtr(note)
	if j.CreatedAt, err = parseTime(created); err != nil {
		return nil, fmt.Errorf("storage: parse summary job created_at: %w", err)
	}
	if j.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, fmt.Errorf("storage: parse summary job updated_at: %w", err)
	}
	if ended.Valid {
		t, err := parseTime(ended.String)
		if err != nil {
			return nil, fmt.Errorf("storage: parse summary job ended_at: %w", err)
		}
		j.EndedAt = &t
	}
	return &j, nil
}
