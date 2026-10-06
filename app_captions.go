package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"ballast/internal/captions"
	"ballast/internal/drive"
	"ballast/internal/events"
	"ballast/internal/logging"
	"ballast/internal/modelfetch"
	"ballast/internal/storage"

	drivev3 "google.golang.org/api/drive/v3"
)

// CaptionSettingsDTO is Captions.GetSettings' result (Feature 005
// contracts/wails-bindings.md).
type CaptionSettingsDTO struct {
	Enabled           bool   `json:"enabled"`
	Language          string `json:"language"`
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailableReason,omitempty"`
	ModelConsent      string `json:"modelConsent"`
	ModelDownloaded   bool   `json:"modelDownloaded"`
	ModelSizeBytes    int64  `json:"modelSizeBytes"`
}

// startCaptions (re)starts the captions worker against the current DB.
// Called at startup and again by DebugRestart, which swaps the DB handle.
func (a *App) startCaptions() {
	if a.captionsStop != nil {
		a.captionsStop()
		a.captionsStop = nil
	}
	if a.db == nil {
		return
	}
	dataDir, err := storage.AppDataDir()
	if err != nil {
		logging.Warn("captions disabled: no app data folder", "error", err)
		return
	}
	downloads := ""
	if home, err := os.UserHomeDir(); err == nil {
		downloads = filepath.Join(home, "Downloads")
	}
	a.captions = captions.NewWorker(captions.Deps{
		DB:           a.db,
		DataDir:      dataDir,
		DownloadsDir: downloads,
		Model:        captions.SpeechModel,
		Availability: a.captionsAvailability,
		Uploader: func(ctx context.Context) (captions.Uploader, error) {
			svc, err := a.driveService(ctx)
			if err != nil {
				return nil, err
			}
			return driveCaptionUploader{svc: svc}, nil
		},
		Emit:              func(j events.CaptionJob) { events.EmitCaptionsUpdated(a.ctx, j) },
		EmitConsentNeeded: func(size int64) { events.EmitCaptionsConsentNeeded(a.ctx, size) },
	})
	ctx, cancel := context.WithCancel(a.ctx)
	a.captionsStop = cancel
	go a.captions.Run(ctx)
}

// enqueueCaptions creates uploadID's caption job if it should have one.
// It only logs on error: captions must never get in the way of the
// upload (FR-006).
func (a *App) enqueueCaptions(uploadID int64) {
	if a.captions == nil {
		return
	}
	if _, err := a.captions.Enqueue(uploadID); err != nil {
		logging.Warn("could not create caption job", "uploadId", uploadID, "error", err)
	}
}

// captionsVideoSucceeded lets uploadID's caption file follow its video.
func (a *App) captionsVideoSucceeded(uploadID int64) {
	if a.captions != nil {
		a.captions.VideoSucceeded(uploadID)
	}
}

// cancelCaptions stops uploadID's captioning because the upload was
// cancelled, deleted, or failed (FR-011). The upload's own handling is
// already done by the time this runs.
func (a *App) cancelCaptions(uploadID int64) {
	if a.captions != nil {
		a.captions.Cancel(uploadID)
	}
}

// captionJobFor returns uploadID's caption job for the frontend, or nil.
func (a *App) captionJobFor(uploadID int64) *events.CaptionJob {
	if a.db == nil {
		return nil
	}
	j, err := a.db.GetCaptionJobByUpload(uploadID)
	if err != nil {
		return nil
	}
	e := captions.ToEvent(j)
	return &e
}

// CaptionsGetSettings returns the caption settings and whether captions
// can run on this machine (FR-017).
func (a *App) CaptionsGetSettings() (CaptionSettingsDTO, error) {
	if a.db == nil {
		return CaptionSettingsDTO{}, fmt.Errorf("captions: local database is unavailable")
	}
	enabled, err := a.db.CaptionsEnabled()
	if err != nil {
		return CaptionSettingsDTO{}, err
	}
	lang, err := a.db.CaptionLanguage()
	if err != nil {
		return CaptionSettingsDTO{}, err
	}
	consent, err := a.db.CaptionModelConsent()
	if err != nil {
		return CaptionSettingsDTO{}, err
	}
	ok, reason, _ := a.captionsAvailability()
	downloaded := false
	if dir, err := storage.AppDataDir(); err == nil {
		downloaded = modelfetch.Present(filepath.Join(dir, "models"), captions.SpeechModel)
	}
	return CaptionSettingsDTO{
		Enabled:           enabled,
		Language:          lang,
		Available:         ok,
		UnavailableReason: reason,
		ModelConsent:      string(consent),
		ModelDownloaded:   downloaded,
		ModelSizeBytes:    captions.SpeechModel.Size,
	}, nil
}

// CaptionsAnswerModelDownload records the answer to the one-time
// speech-model download prompt (FR-019).
func (a *App) CaptionsAnswerModelDownload(accept bool) (CaptionSettingsDTO, error) {
	if a.captions == nil {
		return CaptionSettingsDTO{}, fmt.Errorf("captions: not running")
	}
	if err := a.captions.AnswerConsent(accept); err != nil {
		return CaptionSettingsDTO{}, err
	}
	return a.CaptionsGetSettings()
}

// CaptionsSetEnabled turns automatic captions on or off for uploads
// started afterwards; a job already running finishes (FR-009). Turning
// them on counts as agreeing to the speech-model download, which starts
// in the background.
func (a *App) CaptionsSetEnabled(enabled bool) (CaptionSettingsDTO, error) {
	if err := a.requireCaptionsAvailable(); err != nil {
		return CaptionSettingsDTO{}, err
	}
	if err := a.db.SetCaptionsEnabled(enabled); err != nil {
		return CaptionSettingsDTO{}, err
	}
	if enabled {
		consent, err := a.db.CaptionModelConsent()
		if err != nil {
			return CaptionSettingsDTO{}, err
		}
		if consent != storage.ConsentAccepted {
			if err := a.db.SetCaptionModelConsent(storage.ConsentAccepted); err != nil {
				return CaptionSettingsDTO{}, err
			}
		}
		if a.captions != nil && a.ctx != nil {
			a.captions.Prefetch(a.ctx)
		}
	}
	return a.CaptionsGetSettings()
}

// CaptionsSetLanguage sets the caption language ("en" or "auto") for
// videos captioned afterwards (FR-020).
func (a *App) CaptionsSetLanguage(language string) (CaptionSettingsDTO, error) {
	if err := a.requireCaptionsAvailable(); err != nil {
		return CaptionSettingsDTO{}, err
	}
	if err := a.db.SetCaptionLanguage(language); err != nil {
		return CaptionSettingsDTO{}, err
	}
	return a.CaptionsGetSettings()
}

func (a *App) requireCaptionsAvailable() error {
	if a.db == nil {
		return fmt.Errorf("captions: local database is unavailable")
	}
	if ok, reason, _ := a.captionsAvailability(); !ok {
		return fmt.Errorf("captions: %s", reason)
	}
	return nil
}

// CaptionsGetJob returns uploadID's caption state, or null if it has none.
func (a *App) CaptionsGetJob(uploadID int64) (*events.CaptionJob, error) {
	return a.captionJobFor(uploadID), nil
}

// CaptionsShowLocalCopy reveals the caption file saved next to the
// original video in Finder (FR-022).
func (a *App) CaptionsShowLocalCopy(uploadID int64) error {
	j := a.captionJobFor(uploadID)
	if j == nil || j.LocalCopyPath == "" {
		return fmt.Errorf("captions: this video has no caption file on this Mac")
	}
	if _, err := os.Stat(j.LocalCopyPath); err != nil {
		return fmt.Errorf("captions: the caption file has been moved or deleted")
	}
	return exec.Command("open", "-R", j.LocalCopyPath).Start()
}

// driveCaptionUploader adapts internal/drive's caption-file calls to
// captions.Uploader.
type driveCaptionUploader struct{ svc *drivev3.Service }

func (d driveCaptionUploader) Find(ctx context.Context, uploadID int64, folderID string) (*drive.UploadResult, error) {
	return drive.FindCaptionFile(ctx, d.svc, uploadID, folderID)
}

func (d driveCaptionUploader) ChooseName(ctx context.Context, folderID, base, ext string) (string, error) {
	return drive.ChooseCaptionName(ctx, d.svc, folderID, base, ext)
}

func (d driveCaptionUploader) Upload(ctx context.Context, uploadID int64, folderID, name string, content io.Reader) (*drive.UploadResult, error) {
	return drive.UploadCaptionFile(ctx, d.svc, uploadID, folderID, name, content)
}
