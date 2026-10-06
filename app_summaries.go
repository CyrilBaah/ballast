package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"ballast/internal/drive"
	"ballast/internal/events"
	"ballast/internal/logging"
	"ballast/internal/modelfetch"
	"ballast/internal/storage"
	"ballast/internal/summaries"

	drivev3 "google.golang.org/api/drive/v3"
)

// SummarySettingsDTO is Summaries.GetSettings' result (Feature 006
// contracts/wails-bindings.md).
type SummarySettingsDTO struct {
	Enabled           bool   `json:"enabled"`
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailableReason,omitempty"`
	ModelConsent      string `json:"modelConsent"`
	ModelDownloaded   bool   `json:"modelDownloaded"`
	ModelSizeBytes    int64  `json:"modelSizeBytes"`
}

// startSummaries (re)starts the summaries worker against the current DB,
// beside captions.
func (a *App) startSummaries() {
	if a.summariesStop != nil {
		a.summariesStop()
		a.summariesStop = nil
	}
	if a.db == nil {
		return
	}
	dataDir, err := storage.AppDataDir()
	if err != nil {
		logging.Warn("summaries disabled: no app data folder", "error", err)
		return
	}
	downloads := ""
	if home, err := os.UserHomeDir(); err == nil {
		downloads = filepath.Join(home, "Downloads")
	}
	a.summaries = summaries.NewWorker(summaries.Deps{
		DB:           a.db,
		DataDir:      dataDir,
		DownloadsDir: downloads,
		Model:        summaries.Model,
		Availability: a.summariesAvailability,
		Uploader: func(ctx context.Context) (summaries.Uploader, error) {
			svc, err := a.driveService(ctx)
			if err != nil {
				return nil, err
			}
			return driveSummaryUploader{svc: svc}, nil
		},
		CaptionsBusy:      a.captionsBusy,
		Emit:              func(j events.SummaryJob) { events.EmitSummariesUpdated(a.ctx, j) },
		EmitConsentNeeded: func(size int64) { events.EmitSummariesConsentNeeded(a.ctx, size) },
	})
	ctx, cancel := context.WithCancel(a.ctx)
	a.summariesStop = cancel
	go a.summaries.Run(ctx)
}

// captionsBusy reports whether the speech model is at work or about to
// be, so the summary model waits its turn (research.md §7).
func (a *App) captionsBusy() bool {
	if a.db == nil {
		return false
	}
	jobs, err := a.db.ListActiveCaptionJobs()
	if err != nil {
		return false
	}
	for _, j := range jobs {
		if j.Phase == storage.PhaseExtractingAudio || j.Phase == storage.PhaseTranscribing {
			return true
		}
	}
	return false
}

// summaryJobFor returns uploadID's summary job for the frontend, or nil.
func (a *App) summaryJobFor(uploadID int64) *events.SummaryJob {
	if a.db == nil {
		return nil
	}
	j, err := a.db.GetSummaryJobByUpload(uploadID)
	if err != nil {
		return nil
	}
	e := summaries.ToEvent(j)
	return &e
}

// SummariesGetSettings returns the summary settings and whether summaries
// can run here.
func (a *App) SummariesGetSettings() (SummarySettingsDTO, error) {
	if a.db == nil {
		return SummarySettingsDTO{}, fmt.Errorf("summaries: local database is unavailable")
	}
	enabled, err := a.db.SummariesEnabled()
	if err != nil {
		return SummarySettingsDTO{}, err
	}
	consent, err := a.db.SummaryModelConsent()
	if err != nil {
		return SummarySettingsDTO{}, err
	}
	ok, reason, _ := a.summariesAvailability()
	downloaded := false
	if dir, err := storage.AppDataDir(); err == nil && summaries.Model.FileName != "" {
		downloaded = modelfetch.Present(filepath.Join(dir, "models"), summaries.Model)
	}
	return SummarySettingsDTO{
		Enabled:           enabled,
		Available:         ok,
		UnavailableReason: reason,
		ModelConsent:      string(consent),
		ModelDownloaded:   downloaded,
		ModelSizeBytes:    summaries.Model.Size,
	}, nil
}

// SummariesSetEnabled turns summaries on or off for videos captioned
// afterwards; turning them on counts as agreeing to the model download,
// which starts in the background (FR-012, FR-013).
func (a *App) SummariesSetEnabled(enabled bool) (SummarySettingsDTO, error) {
	if a.db == nil {
		return SummarySettingsDTO{}, fmt.Errorf("summaries: local database is unavailable")
	}
	if ok, reason, _ := a.summariesAvailability(); !ok {
		return SummarySettingsDTO{}, fmt.Errorf("summaries: %s", reason)
	}
	if err := a.db.SetSummariesEnabled(enabled); err != nil {
		return SummarySettingsDTO{}, err
	}
	if enabled {
		if err := a.db.SetSummaryModelConsent(storage.ConsentAccepted); err != nil {
			return SummarySettingsDTO{}, err
		}
		if a.summaries != nil && a.ctx != nil {
			a.summaries.Prefetch(a.ctx)
		}
	}
	return a.SummariesGetSettings()
}

// SummariesAnswerModelDownload records the answer to the one-time
// summary-model download prompt (FR-013).
func (a *App) SummariesAnswerModelDownload(accept bool) (SummarySettingsDTO, error) {
	if a.summaries == nil {
		return SummarySettingsDTO{}, fmt.Errorf("summaries: not running")
	}
	if err := a.summaries.AnswerConsent(accept); err != nil {
		return SummarySettingsDTO{}, err
	}
	return a.SummariesGetSettings()
}

// SummariesGetJob returns uploadID's summary state, or null.
func (a *App) SummariesGetJob(uploadID int64) (*events.SummaryJob, error) {
	return a.summaryJobFor(uploadID), nil
}

// SummariesRetry is "Try again" on a failed summary (FR-009).
func (a *App) SummariesRetry(uploadID int64) (*events.SummaryJob, error) {
	if a.summaries == nil {
		return nil, fmt.Errorf("summaries: not running")
	}
	if err := a.summaries.Retry(uploadID); err != nil {
		return nil, err
	}
	return a.summaryJobFor(uploadID), nil
}

// SummariesShowLocalCopy reveals the summary saved next to the video in Finder.
func (a *App) SummariesShowLocalCopy(uploadID int64) error {
	j := a.summaryJobFor(uploadID)
	if j == nil || j.LocalCopyPath == "" {
		return fmt.Errorf("summaries: this video has no summary on this Mac")
	}
	if _, err := os.Stat(j.LocalCopyPath); err != nil {
		return fmt.Errorf("summaries: the summary has been moved or deleted")
	}
	return exec.Command("open", "-R", j.LocalCopyPath).Start()
}

// driveSummaryUploader adapts internal/drive's summary calls.
type driveSummaryUploader struct{ svc *drivev3.Service }

func (d driveSummaryUploader) Find(ctx context.Context, uploadID int64, folderID string) (*drive.UploadResult, error) {
	return drive.FindSummaryDoc(ctx, d.svc, uploadID, folderID)
}

func (d driveSummaryUploader) ChooseName(ctx context.Context, folderID, base string) (string, error) {
	return drive.ChooseSummaryName(ctx, d.svc, folderID, base)
}

func (d driveSummaryUploader) Create(ctx context.Context, uploadID int64, folderID, name string, html io.Reader) (*drive.UploadResult, error) {
	return drive.CreateSummaryDoc(ctx, d.svc, uploadID, folderID, name, html)
}
