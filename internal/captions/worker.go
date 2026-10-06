package captions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ballast/internal/drive"
	"ballast/internal/events"
	"ballast/internal/heavywork"
	"ballast/internal/logging"
	"ballast/internal/modelfetch"
	"ballast/internal/storage"
)

// Uploader is the slice of Drive the worker needs to put a caption file
// next to its video (internal/drive/captionfile.go).
type Uploader interface {
	Find(ctx context.Context, uploadID int64, folderID string) (*drive.UploadResult, error)
	ChooseName(ctx context.Context, folderID, base, ext string) (string, error)
	Upload(ctx context.Context, uploadID int64, folderID, name string, content io.Reader) (*drive.UploadResult, error)
}

// Deps is everything the worker reaches outside itself, so tests can swap
// each piece for a fake. Zero-valued function fields get the real ones.
type Deps struct {
	DB           *storage.DB
	DataDir      string // app data folder: models/ and captions/<job id>/ live here
	DownloadsDir string // fallback for the local caption copy (FR-022)
	HTTPClient   *http.Client
	Model        modelfetch.Spec

	FetchModel   func(ctx context.Context, client *http.Client, dir string, spec modelfetch.Spec, progress func(done, total int64)) error
	Availability func() (ok bool, reason, binPath string)
	Extract      func(ctx context.Context, videoPath, outWAV string) error
	Uploader     func(ctx context.Context) (Uploader, error) // errors while signed out
	FreeSpace    func(dir string) (uint64, error)

	Emit              func(events.CaptionJob)
	EmitConsentNeeded func(modelSizeBytes int64)
	// OnTranscriptReady gets the merged transcript as soon as it exists;
	// the file is deleted when the job ends, so a subscriber that needs it
	// (Feature 006) must copy it before returning.
	OnTranscriptReady func(uploadID int64, srtPath string)
	// OnNoTranscript is called once when a job ends without a transcript.
	OnNoTranscript func(uploadID int64, reason string)

	RetryDelay func(attempt int) time.Duration
}

// Notes shown to the user (FR-007).
const (
	NoteNoSpeech       = "No speech found in this video"
	NoteTurnedOff      = "Captions were turned off"
	NoteNoDiskForModel = "Not enough free disk space to download the speech model"
	NoteUnreadable     = "Couldn't read the audio from this video"
	NoteNoDiskForAudio = "Not enough free disk space to make captions"
)

// minFreeForAudio is the free space required before extracting audio. The
// extracted audio is about 115 MB an hour (16 kHz mono 16-bit), so 1 GiB
// covers any realistic recording with room to spare (research.md §3).
const minFreeForAudio = 1 << 30

// Worker turns caption jobs into caption files, one job at a time
// (spec Assumptions), separately from every upload goroutine.
type Worker struct {
	deps Deps
	step sync.Mutex // held while one job advances one phase
	wake chan struct{}

	mu           sync.Mutex
	running      map[int64]context.CancelFunc // upload id → the step in progress
	retryAt      map[int64]time.Time
	attempts     map[int64]int
	lastProgress map[int64]time.Time
	consentAsked bool
}

// NewWorker fills in the real implementations for any unset Deps field.
func NewWorker(deps Deps) *Worker {
	if deps.HTTPClient == nil {
		deps.HTTPClient = http.DefaultClient
	}
	if deps.FetchModel == nil {
		deps.FetchModel = modelfetch.Fetch
	}
	if deps.Availability == nil {
		deps.Availability = Availability
	}
	if deps.Extract == nil {
		deps.Extract = ExtractAudio
	}
	if deps.FreeSpace == nil {
		deps.FreeSpace = modelfetch.AvailableBytes
	}
	if deps.RetryDelay == nil {
		deps.RetryDelay = defaultRetryDelay
	}
	for _, f := range []*func(int64, string){&deps.OnTranscriptReady, &deps.OnNoTranscript} {
		if *f == nil {
			*f = func(int64, string) {}
		}
	}
	if deps.Emit == nil {
		deps.Emit = func(events.CaptionJob) {}
	}
	if deps.EmitConsentNeeded == nil {
		deps.EmitConsentNeeded = func(int64) {}
	}
	return &Worker{
		deps:         deps,
		wake:         make(chan struct{}, 1),
		running:      map[int64]context.CancelFunc{},
		retryAt:      map[int64]time.Time{},
		attempts:     map[int64]int{},
		lastProgress: map[int64]time.Time{},
	}
}

func defaultRetryDelay(attempt int) time.Duration {
	d := 5 * time.Second << attempt
	if d <= 0 || d > 10*time.Minute {
		return 10 * time.Minute
	}
	return d
}

// ToEvent is the frontend's view of a job (contracts/wails-bindings.md).
func ToEvent(j *storage.CaptionJob) events.CaptionJob {
	e := events.CaptionJob{
		UploadID:        j.UploadID,
		Status:          string(j.Status),
		Phase:           string(j.Phase),
		ProgressPercent: j.ProgressPercent,
		Language:        j.Language,
	}
	for dst, src := range map[*string]*string{&e.DriveFileName: j.DriveFileName, &e.DriveFileLink: j.DriveFileLink, &e.LocalCopyPath: j.LocalCopyPath, &e.Note: j.Note} {
		if src != nil {
			*dst = *src
		}
	}
	return e
}

// Enqueue creates uploadID's caption job if it should have one: a video,
// with captions on and available on this machine. A video in a format
// Ballast can't read gets a job that has already failed with the reason;
// anything else gets none (FR-010). It never touches the upload.
func (w *Worker) Enqueue(uploadID int64) (*storage.CaptionJob, error) {
	db := w.deps.DB
	u, err := db.GetUpload(uploadID)
	if err != nil {
		return nil, err
	}
	captionable, ext := Classify(u.LocalPath)
	if !captionable && ext == "" {
		return nil, nil
	}
	if on, err := db.CaptionsEnabled(); err != nil || !on {
		return nil, err
	}
	if ok, _, _ := w.deps.Availability(); !ok {
		return nil, nil
	}
	lang, err := db.CaptionLanguage()
	if err != nil {
		return nil, err
	}

	if ext != "" {
		note := UnsupportedNote(ext)
		j, err := db.CreateCaptionJob(uploadID, lang, storage.CaptionFailed, storage.PhaseNone, note)
		if err != nil {
			return nil, err
		}
		w.emit(j.ID)
		w.deps.OnNoTranscript(uploadID, note)
		return j, nil
	}

	status, phase := storage.CaptionWaiting, storage.PhaseAwaitingConsent
	if consent, err := db.CaptionModelConsent(); err != nil {
		return nil, err
	} else if consent == storage.ConsentAccepted {
		status, phase = storage.CaptionInProgress, w.firstWorkPhase()
	}
	j, err := db.CreateCaptionJob(uploadID, lang, status, phase, "")
	if err != nil {
		return nil, err
	}
	w.emit(j.ID)
	w.Wake()
	return j, nil
}

func (w *Worker) firstWorkPhase() storage.CaptionPhase {
	if modelfetch.Present(w.modelDir(), w.deps.Model) {
		return storage.PhaseExtractingAudio
	}
	return storage.PhaseDownloadingModel
}

// VideoSucceeded tells the worker an upload has landed in Drive, so its
// caption file can follow.
func (w *Worker) VideoSucceeded(uploadID int64) { w.Wake() }

// Wake nudges Run to look for work now.
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// AnswerConsent records the user's answer to the one-time speech-model
// download prompt. Declining turns captions off and ends every job waiting
// on the answer; the uploads themselves are never touched (FR-019).
func (w *Worker) AnswerConsent(accept bool) error {
	db := w.deps.DB
	consent := storage.ConsentDeclined
	if accept {
		consent = storage.ConsentAccepted
	}
	if err := db.SetCaptionModelConsent(consent); err != nil {
		return err
	}
	w.mu.Lock()
	w.consentAsked = false
	w.mu.Unlock()
	if !accept {
		if err := db.SetCaptionsEnabled(false); err != nil {
			return err
		}
		w.step.Lock()
		defer w.step.Unlock()
		jobs, err := db.ListActiveCaptionJobs()
		if err != nil {
			return err
		}
		for _, j := range jobs {
			if j.Phase == storage.PhaseAwaitingConsent {
				w.endJob(j, func() error { return db.SetCaptionCancelled(j.ID, NoteTurnedOff) }, NoteTurnedOff)
			}
		}
		return nil
	}
	w.Wake()
	return nil
}

// Cancel ends uploadID's caption job because its upload was cancelled or
// failed: any running helper process is killed, nothing is uploaded to
// Drive, the work folder is deleted, and a local copy already saved next
// to the video is kept (FR-011, FR-022).
//
// The job is marked cancelled first and its running step (if any) killed
// second; a step re-reads its job right after registering, so a cancel
// that lands between two steps stops the next one before it starts.
func (w *Worker) Cancel(uploadID int64) {
	db := w.deps.DB
	j, err := db.GetCaptionJobByUpload(uploadID)
	if err != nil || (j.Status != storage.CaptionWaiting && j.Status != storage.CaptionInProgress) {
		return
	}
	if err := db.SetCaptionCancelled(j.ID, ""); err != nil {
		return
	}
	w.mu.Lock()
	if cancel := w.running[uploadID]; cancel != nil {
		cancel()
	}
	w.mu.Unlock()

	// Wait for a killed step to unwind before deleting its files.
	w.step.Lock()
	defer w.step.Unlock()
	w.afterEnd(j, "The video upload was cancelled")
}

// Run processes jobs until ctx ends, sleeping when there's nothing to do.
func (w *Worker) Run(ctx context.Context) {
	w.CleanupOrphans()
	for ctx.Err() == nil {
		if w.RunOnce(ctx) {
			continue
		}
		wait := 30 * time.Second
		w.mu.Lock()
		for _, at := range w.retryAt {
			if d := time.Until(at); d < wait {
				wait = d
			}
		}
		w.mu.Unlock()
		if wait < 0 {
			wait = 0
		}
		select {
		case <-ctx.Done():
		case <-w.wake:
		case <-time.After(wait):
		}
	}
}

// RunUntilIdle processes jobs until none can advance right now.
func (w *Worker) RunUntilIdle(ctx context.Context) {
	for w.RunOnce(ctx) {
	}
}

// RunOnce advances the first job that can advance by one phase, and
// reports whether any did. Only one job advances at a time, across every
// caller.
func (w *Worker) RunOnce(ctx context.Context) bool {
	w.step.Lock()
	defer w.step.Unlock()

	jobs, err := w.deps.DB.ListActiveCaptionJobs()
	if err != nil {
		logging.Warn("could not list caption jobs", "error", err)
		return false
	}
	w.askConsentIfNeeded(jobs)
	for _, j := range jobs {
		w.mu.Lock()
		at := w.retryAt[j.UploadID]
		w.mu.Unlock()
		if time.Now().Before(at) {
			continue
		}
		if w.advance(ctx, j) {
			return true
		}
	}
	return false
}

func (w *Worker) askConsentIfNeeded(jobs []*storage.CaptionJob) {
	consent, err := w.deps.DB.CaptionModelConsent()
	if err != nil || consent != storage.ConsentUnasked {
		return
	}
	for _, j := range jobs {
		if j.Phase == storage.PhaseAwaitingConsent {
			w.mu.Lock()
			asked := w.consentAsked
			w.consentAsked = true
			w.mu.Unlock()
			if !asked {
				w.deps.EmitConsentNeeded(w.deps.Model.Size)
			}
			return
		}
	}
}

// advance moves job j forward by one phase if it can, under a context the
// job's Cancel can stop.
func (w *Worker) advance(parent context.Context, j *storage.CaptionJob) bool {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	w.mu.Lock()
	w.running[j.UploadID] = cancel
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		delete(w.running, j.UploadID)
		w.mu.Unlock()
	}()
	if cur, err := w.deps.DB.GetCaptionJob(j.ID); err != nil ||
		(cur.Status != storage.CaptionWaiting && cur.Status != storage.CaptionInProgress) {
		return false // cancelled since the job list was read
	}

	switch j.Phase {
	case storage.PhaseAwaitingConsent:
		switch consent, _ := w.deps.DB.CaptionModelConsent(); consent {
		case storage.ConsentAccepted:
			return w.setPhase(j, storage.PhaseDownloadingModel)
		case storage.ConsentDeclined:
			return w.endJob(j, func() error { return w.deps.DB.SetCaptionCancelled(j.ID, NoteTurnedOff) }, NoteTurnedOff)
		}
		return false
	case storage.PhaseDownloadingModel:
		return w.downloadModel(ctx, j)
	case storage.PhaseExtractingAudio:
		return w.extract(ctx, j)
	case storage.PhaseTranscribing:
		return w.transcribe(ctx, j)
	case storage.PhaseWaitingForVideo:
		u, err := w.deps.DB.GetUpload(j.UploadID)
		if err != nil {
			return false
		}
		switch u.Status {
		case storage.UploadSucceeded:
			return w.setPhase(j, storage.PhaseUploadingCaptions)
		case storage.UploadFailed, storage.UploadCancelled:
			return w.endJob(j, func() error { return w.deps.DB.SetCaptionCancelled(j.ID, "") }, "")
		}
		return false
	case storage.PhaseUploadingCaptions:
		return w.upload(ctx, j)
	}
	return false
}

func (w *Worker) downloadModel(ctx context.Context, j *storage.CaptionJob) bool {
	if modelfetch.Present(w.modelDir(), w.deps.Model) {
		return w.setPhase(j, storage.PhaseExtractingAudio)
	}
	err := w.deps.FetchModel(ctx, w.deps.HTTPClient, w.modelDir(), w.deps.Model, func(done, total int64) {
		if total > 0 {
			w.setProgress(j, int(done*100/total))
		}
	})
	switch {
	case err == nil:
		return w.setPhase(j, storage.PhaseExtractingAudio)
	case ctx.Err() != nil:
		return false
	case errors.Is(err, modelfetch.ErrNotEnoughSpace):
		return w.fail(j, NoteNoDiskForModel)
	default:
		logging.Warn("speech model download failed; will retry", "uploadId", j.UploadID, "error", err)
		w.scheduleRetry(j)
		return false
	}
}

func (w *Worker) extract(ctx context.Context, j *storage.CaptionJob) bool {
	u, err := w.deps.DB.GetUpload(j.UploadID)
	if err != nil {
		return false
	}
	dir := w.workDir(j.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return w.fail(j, NoteUnreadable)
	}
	if free, err := w.deps.FreeSpace(dir); err == nil && free < minFreeForAudio {
		return w.fail(j, NoteNoDiskForAudio)
	}
	audio := filepath.Join(dir, "audio.wav")
	if err := w.deps.Extract(ctx, u.LocalPath, audio); err != nil {
		if ctx.Err() != nil {
			return false
		}
		if errors.Is(err, ErrNoAudio) {
			return w.fail(j, ErrNoAudio.Error())
		}
		logging.Warn("audio extraction failed", "uploadId", j.UploadID, "error", err)
		return w.fail(j, NoteUnreadable)
	}
	wav, err := OpenWAV(audio)
	if err != nil {
		return w.fail(j, NoteUnreadable)
	}
	cuts, err := SplitPoints(wav, PieceLength, CutSearchWindow)
	if err != nil {
		return w.fail(j, NoteUnreadable)
	}
	removeGlob(filepath.Join(dir, "piece-*"))
	if err := w.deps.DB.SetCaptionPieces(j.ID, wav.Duration().Milliseconds(), len(Pieces(wav, cuts))); err != nil {
		return false
	}
	return w.setPhase(j, storage.PhaseTranscribing)
}

func (w *Worker) transcribe(ctx context.Context, j *storage.CaptionJob) bool {
	dir := w.workDir(j.ID)
	wav, err := OpenWAV(filepath.Join(dir, "audio.wav"))
	if err != nil {
		// The audio is gone (e.g. cleared after a crash): extract it again.
		return w.setPhase(j, storage.PhaseExtractingAudio)
	}
	cuts, err := SplitPoints(wav, PieceLength, CutSearchWindow)
	if err != nil {
		return w.setPhase(j, storage.PhaseExtractingAudio)
	}
	pieces := Pieces(wav, cuts)
	ok, reason, bin := w.deps.Availability()
	if !ok {
		return w.fail(j, reason)
	}
	tr := Transcriber{BinPath: bin, ModelPath: modelfetch.Path(w.modelDir(), w.deps.Model), Threads: 4}

	// Only the speech model itself holds the shared lock, so a summary
	// model never runs alongside it (Feature 006 research.md §7).
	release, err := heavywork.Acquire(ctx, "captions")
	if err != nil {
		return false
	}
	defer release()

	for i := j.PiecesDone; i < len(pieces); i++ {
		pieceWAV := filepath.Join(dir, "piece.wav")
		if err := WritePiece(wav, pieces[i], pieceWAV); err != nil {
			return w.fail(j, NoteUnreadable)
		}
		raw, err := tr.Transcribe(ctx, pieceWAV, j.Language, filepath.Join(dir, "piece-raw"), func(p int) {
			w.setProgress(j, (i*100+p)/len(pieces))
		})
		if err != nil {
			if ctx.Err() != nil {
				return false
			}
			logging.Warn("speech engine failed", "uploadId", j.UploadID, "piece", i, "error", err)
			return w.fail(j, "The speech engine couldn't caption this video")
		}
		cues, err := readSRT(raw)
		if err != nil {
			return w.fail(j, "The speech engine couldn't caption this video")
		}
		if err := writeSRTFile(piecePath(dir, i), Clean(Shift(cues, pieces[i].Start))); err != nil {
			return w.fail(j, NoteUnreadable)
		}
		os.Remove(raw)
		os.Remove(pieceWAV)
		if err := w.deps.DB.SetCaptionPiecesDone(j.ID, i+1); err != nil {
			return false
		}
	}

	var all []Cue
	for i := range pieces {
		cues, err := readSRT(piecePath(dir, i))
		if err != nil {
			return w.setPhase(j, storage.PhaseExtractingAudio)
		}
		all = append(all, cues...)
	}
	all = Clean(all) // a loop can straddle two pieces
	if !HasSpeech(all) {
		return w.endJob(j, func() error { return w.deps.DB.SetCaptionDone(j.ID, "", "", "", NoteNoSpeech) }, NoteNoSpeech)
	}
	transcript := filepath.Join(dir, "transcript.srt")
	if err := writeSRTFile(transcript, all); err != nil {
		return w.fail(j, NoteUnreadable)
	}

	u, err := w.deps.DB.GetUpload(j.UploadID)
	if err != nil {
		return false
	}
	if saved, err := SaveLocalCopy(u.LocalPath, transcript, w.deps.DownloadsDir); err != nil {
		logging.Warn("could not save the local caption copy", "uploadId", j.UploadID, "error", err)
	} else if err := w.deps.DB.SetCaptionLocalCopy(j.ID, saved); err != nil {
		return false
	}
	w.deps.OnTranscriptReady(j.UploadID, transcript)
	return w.setPhase(j, storage.PhaseWaitingForVideo)
}

func (w *Worker) upload(ctx context.Context, j *storage.CaptionJob) bool {
	u, err := w.deps.DB.GetUpload(j.UploadID)
	if err != nil {
		return false
	}
	transcript := filepath.Join(w.workDir(j.ID), "transcript.srt")
	if _, err := os.Stat(transcript); err != nil {
		return w.setPhase(j, storage.PhaseExtractingAudio)
	}
	up, err := w.deps.Uploader(ctx)
	if err != nil {
		// Signed out, most likely: wait and retry without touching the upload.
		w.scheduleRetry(j)
		return false
	}
	if existing, err := up.Find(ctx, j.UploadID, u.DriveFolderID); err != nil {
		w.scheduleRetry(j)
		return false
	} else if existing != nil {
		return w.endJob(j, func() error {
			return w.deps.DB.SetCaptionDone(j.ID, existing.FileID, existing.WebViewLink, "", "")
		}, "")
	}
	base := strings.TrimSuffix(filepath.Base(u.LocalPath), filepath.Ext(u.LocalPath))
	name, err := up.ChooseName(ctx, u.DriveFolderID, base, ".srt")
	if err != nil {
		w.scheduleRetry(j)
		return false
	}
	f, err := os.Open(transcript)
	if err != nil {
		return w.setPhase(j, storage.PhaseExtractingAudio)
	}
	defer f.Close()
	res, err := up.Upload(ctx, j.UploadID, u.DriveFolderID, name, f)
	if err != nil {
		logging.Warn("caption file upload failed; will retry", "uploadId", j.UploadID, "error", err)
		w.scheduleRetry(j)
		return false
	}
	return w.endJob(j, func() error {
		return w.deps.DB.SetCaptionDone(j.ID, res.FileID, res.WebViewLink, name, "")
	}, "")
}

// setPhase moves j to phase (in progress) and tells the frontend.
func (w *Worker) setPhase(j *storage.CaptionJob, phase storage.CaptionPhase) bool {
	if err := w.deps.DB.SetCaptionPhase(j.ID, storage.CaptionInProgress, phase); err != nil {
		return false
	}
	w.mu.Lock()
	delete(w.attempts, j.UploadID)
	delete(w.retryAt, j.UploadID)
	w.mu.Unlock()
	w.emit(j.ID)
	return true
}

// setProgress records progress at most once a second per job.
func (w *Worker) setProgress(j *storage.CaptionJob, percent int) {
	w.mu.Lock()
	last := w.lastProgress[j.UploadID]
	due := time.Since(last) >= time.Second || percent >= 100
	if due {
		w.lastProgress[j.UploadID] = time.Now()
	}
	w.mu.Unlock()
	if !due {
		return
	}
	if err := w.deps.DB.SetCaptionProgress(j.ID, percent); err == nil {
		w.emit(j.ID)
	}
}

// fail ends j with a plain-language note (FR-007).
func (w *Worker) fail(j *storage.CaptionJob, note string) bool {
	return w.endJob(j, func() error { return w.deps.DB.SetCaptionFailed(j.ID, note) }, note)
}

// endJob applies the final transition, deletes the work folder, tells the
// frontend, and -- when the job ended without handing over a transcript
// -- tells OnNoTranscript why.
func (w *Worker) endJob(j *storage.CaptionJob, transition func() error, noTranscriptReason string) bool {
	if err := transition(); err != nil {
		return false
	}
	w.afterEnd(j, noTranscriptReason)
	return true
}

// afterEnd is everything that follows a job's final transition.
func (w *Worker) afterEnd(j *storage.CaptionJob, noTranscriptReason string) {
	os.RemoveAll(w.workDir(j.ID))
	w.mu.Lock()
	delete(w.attempts, j.UploadID)
	delete(w.retryAt, j.UploadID)
	delete(w.lastProgress, j.UploadID)
	w.mu.Unlock()
	w.emit(j.ID)
	ended, err := w.deps.DB.GetCaptionJob(j.ID)
	handedOver := err == nil && ended.Status == storage.CaptionDone && ended.Note == nil
	if !handedOver && j.Phase != storage.PhaseWaitingForVideo && j.Phase != storage.PhaseUploadingCaptions {
		if noTranscriptReason == "" {
			noTranscriptReason = "Captions couldn't be made"
		}
		w.deps.OnNoTranscript(j.UploadID, noTranscriptReason)
	}
}

func (w *Worker) scheduleRetry(j *storage.CaptionJob) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := w.attempts[j.UploadID]
	w.attempts[j.UploadID] = n + 1
	w.retryAt[j.UploadID] = time.Now().Add(w.deps.RetryDelay(n))
}

func (w *Worker) emit(jobID int64) {
	j, err := w.deps.DB.GetCaptionJob(jobID)
	if err != nil {
		return
	}
	w.deps.Emit(ToEvent(j))
}

// CleanupOrphans deletes work folders whose job has ended or no longer
// exists, e.g. after a crash mid-cleanup (FR-013).
func (w *Worker) CleanupOrphans() {
	entries, err := os.ReadDir(filepath.Join(w.deps.DataDir, "captions"))
	if err != nil {
		return
	}
	for _, e := range entries {
		id, err := strconv.ParseInt(e.Name(), 10, 64)
		if err != nil {
			continue
		}
		j, err := w.deps.DB.GetCaptionJob(id)
		if errors.Is(err, storage.ErrCaptionJobNotFound) || (err == nil && j.Status != storage.CaptionWaiting && j.Status != storage.CaptionInProgress) {
			os.RemoveAll(filepath.Join(w.deps.DataDir, "captions", e.Name()))
		}
	}
}

func (w *Worker) modelDir() string { return filepath.Join(w.deps.DataDir, "models") }

func (w *Worker) workDir(jobID int64) string {
	return filepath.Join(w.deps.DataDir, "captions", strconv.FormatInt(jobID, 10))
}

func piecePath(dir string, i int) string {
	return filepath.Join(dir, fmt.Sprintf("piece-%03d.srt", i))
}

func readSRT(path string) ([]Cue, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseSRT(f)
}

func writeSRTFile(path string, cues []Cue) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := WriteSRT(f, cues); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func removeGlob(pattern string) {
	matches, _ := filepath.Glob(pattern)
	sort.Strings(matches)
	for _, m := range matches {
		os.Remove(m)
	}
}
