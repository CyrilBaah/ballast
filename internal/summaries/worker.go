package summaries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"ballast/internal/captions"
	"ballast/internal/drive"
	"ballast/internal/events"
	"ballast/internal/heavywork"
	"ballast/internal/logging"
	"ballast/internal/modelfetch"
	"ballast/internal/storage"
)

// Uploader is the slice of Drive the worker needs (internal/drive/summarydoc.go).
type Uploader interface {
	Find(ctx context.Context, uploadID int64, folderID string) (*drive.UploadResult, error)
	ChooseName(ctx context.Context, folderID, base string) (string, error)
	Create(ctx context.Context, uploadID int64, folderID, name string, html io.Reader) (*drive.UploadResult, error)
}

// Engine starts a Summarizer for one job and returns a func that stops it.
type Engine func(ctx context.Context, binPath, modelPath string) (Summarizer, func(), error)

// LocalEngine runs llama-server for the job's lifetime.
func LocalEngine(ctx context.Context, binPath, modelPath string) (Summarizer, func(), error) {
	s, err := StartServer(ctx, binPath, modelPath)
	if err != nil {
		return nil, nil, err
	}
	return Local{Server: s}, s.Stop, nil
}

// Deps is everything the worker reaches outside itself; zero-valued
// function fields get the real ones.
type Deps struct {
	DB           *storage.DB
	DataDir      string
	DownloadsDir string
	HTTPClient   *http.Client
	Model        modelfetch.Spec

	FetchModel   func(ctx context.Context, client *http.Client, dir string, spec modelfetch.Spec, progress func(done, total int64)) error
	Availability func() (ok bool, reason, binPath string)
	Engine       Engine
	Uploader     func(ctx context.Context) (Uploader, error)
	// CaptionsBusy reports whether the speech model is working or about
	// to; captions always go first (research.md §7).
	CaptionsBusy func() bool

	Emit              func(events.SummaryJob)
	EmitConsentNeeded func(modelSizeBytes int64)
	RetryDelay        func(attempt int) time.Duration
}

// MaxAttempts caps automatic retries of an unusable answer (FR-010).
const MaxAttempts = 3

// Notes shown to the user (FR-009).
const (
	NoteUnusable       = "The summary model couldn't produce a usable summary"
	NoteLoadFailed     = "The summary model couldn't be loaded"
	NoteNoMemory       = "Not enough free memory to write the summary — close other apps and try again"
	NoteNoDisk         = "Not enough free disk space to download the summary model"
	NoteTurnedOff      = "Summaries were turned off"
	NoteNoSpeech       = "No speech to summarise"
	NoteNoTranscript   = "No transcript to summarise"
	NoteTranscriptGone = "The transcript is no longer available — the video needs captions again"
)

// Worker writes summaries one at a time, separately from uploads and
// captions.
type Worker struct {
	deps    Deps
	step    sync.Mutex
	fetchMu sync.Mutex
	wake    chan struct{}

	mu           sync.Mutex
	running      map[int64]context.CancelFunc
	retryAt      map[int64]time.Time
	attempts     map[int64]int
	reloaded     map[int64]bool // the model was re-downloaded once after failing to load
	lastProgress map[int64]time.Time
	consentAsked bool
}

// NewWorker fills in the real implementations for unset Deps fields.
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
	if deps.Engine == nil {
		deps.Engine = LocalEngine
	}
	if deps.CaptionsBusy == nil {
		deps.CaptionsBusy = func() bool { return false }
	}
	if deps.Emit == nil {
		deps.Emit = func(events.SummaryJob) {}
	}
	if deps.EmitConsentNeeded == nil {
		deps.EmitConsentNeeded = func(int64) {}
	}
	if deps.RetryDelay == nil {
		deps.RetryDelay = func(attempt int) time.Duration {
			d := 5 * time.Second << attempt
			if d <= 0 || d > 10*time.Minute {
				return 10 * time.Minute
			}
			return d
		}
	}
	return &Worker{
		deps:         deps,
		wake:         make(chan struct{}, 1),
		running:      map[int64]context.CancelFunc{},
		retryAt:      map[int64]time.Time{},
		attempts:     map[int64]int{},
		reloaded:     map[int64]bool{},
		lastProgress: map[int64]time.Time{},
	}
}

// ToEvent is the frontend's view of a job.
func ToEvent(j *storage.SummaryJob) events.SummaryJob {
	e := events.SummaryJob{
		UploadID:        j.UploadID,
		Status:          string(j.Status),
		Phase:           string(j.Phase),
		ProgressPercent: j.ProgressPercent,
		CanRetry:        j.Status == storage.SummaryFailed,
	}
	for dst, src := range map[*string]*string{&e.DriveFileName: j.DriveFileName, &e.DriveFileLink: j.DriveFileLink, &e.LocalCopyPath: j.LocalCopyPath, &e.Note: j.Note} {
		if src != nil {
			*dst = *src
		}
	}
	return e
}

// Enqueue creates uploadID's summary job, waiting for its captions, if
// summaries are on and can run here. The caller only calls it for videos
// that got a caption job.
func (w *Worker) Enqueue(uploadID int64) (*storage.SummaryJob, error) {
	if on, err := w.deps.DB.SummariesEnabled(); err != nil || !on {
		return nil, err
	}
	if ok, _, _ := w.deps.Availability(); !ok {
		return nil, nil
	}
	j, err := w.deps.DB.CreateSummaryJob(uploadID, w.deps.Model.FileName)
	if err != nil {
		return nil, err
	}
	w.emit(j.ID)
	return j, nil
}

// TranscriptReady takes uploadID's finished transcript. It copies it into
// the job's own folder straight away -- captions deletes its copy when it
// finishes -- and moves the job on (Feature 005's OnTranscriptReady).
func (w *Worker) TranscriptReady(uploadID int64, srtPath string) {
	db := w.deps.DB
	j, err := db.GetSummaryJobByUpload(uploadID)
	if err != nil || j.Phase != storage.SummaryPhaseWaitingForCaptions {
		return
	}
	dir := w.workDir(j.ID)
	if err := os.MkdirAll(dir, 0o700); err == nil {
		err = copyFile(srtPath, filepath.Join(dir, "transcript.srt"))
	}
	if err != nil {
		logging.Warn("could not keep the transcript for its summary", "uploadId", uploadID, "error", err)
		w.endJob(j, func() error { return db.SetSummaryFailed(j.ID, NoteTranscriptGone) }, false)
		return
	}
	consent, _ := db.SummaryModelConsent()
	switch consent {
	case storage.ConsentAccepted:
		w.setPhase(j, storage.SummaryWaiting, w.afterConsentPhase())
	case storage.ConsentDeclined:
		w.endJob(j, func() error { return db.SetSummaryCancelled(j.ID, NoteTurnedOff) }, true)
	default:
		w.setPhase(j, storage.SummaryWaiting, storage.SummaryPhaseAwaitingConsent)
	}
	w.Wake()
}

// NoTranscript ends uploadID's summary job because its captions ended
// without a transcript (Feature 005's OnNoTranscript).
func (w *Worker) NoTranscript(uploadID int64, reason string) {
	j, err := w.deps.DB.GetSummaryJobByUpload(uploadID)
	if err != nil || (j.Status != storage.SummaryWaiting && j.Status != storage.SummaryInProgress) {
		return
	}
	note := NoteNoTranscript
	if reason == captions.NoteNoSpeech {
		note = NoteNoSpeech
	}
	w.endJob(j, func() error { return w.deps.DB.SetSummaryCancelled(j.ID, note) }, true)
}

func (w *Worker) afterConsentPhase() storage.SummaryPhase {
	if modelfetch.Present(w.modelDir(), w.deps.Model) {
		return storage.SummaryPhaseWaitingForEngine
	}
	return storage.SummaryPhaseDownloadingModel
}

// VideoSucceeded lets uploadID's summary follow its video into Drive.
func (w *Worker) VideoSucceeded(uploadID int64) { w.Wake() }

// Wake nudges Run to look for work now.
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// AnswerConsent records the answer to the one-time summary-model prompt.
// Declining turns summaries off and ends every job waiting on it (FR-013).
func (w *Worker) AnswerConsent(accept bool) error {
	db := w.deps.DB
	consent := storage.ConsentDeclined
	if accept {
		consent = storage.ConsentAccepted
	}
	if err := db.SetSummaryModelConsent(consent); err != nil {
		return err
	}
	w.mu.Lock()
	w.consentAsked = false
	w.mu.Unlock()
	if accept {
		w.Wake()
		return nil
	}
	if err := db.SetSummariesEnabled(false); err != nil {
		return err
	}
	w.step.Lock()
	defer w.step.Unlock()
	jobs, err := db.ListActiveSummaryJobs()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.Phase == storage.SummaryPhaseAwaitingConsent {
			w.endJob(j, func() error { return db.SetSummaryCancelled(j.ID, NoteTurnedOff) }, true)
		}
	}
	return nil
}

// Prefetch downloads the summary model in the background, for when the
// user turns summaries on in Settings (FR-013).
func (w *Worker) Prefetch(ctx context.Context) {
	go func() {
		w.fetchMu.Lock()
		defer w.fetchMu.Unlock()
		if w.deps.Model.FileName == "" || modelfetch.Present(w.modelDir(), w.deps.Model) {
			return
		}
		if err := w.deps.FetchModel(ctx, w.deps.HTTPClient, w.modelDir(), w.deps.Model, nil); err != nil {
			logging.Warn("background summary model download failed; the first summary will retry it", "error", err)
		}
		w.Wake()
	}()
}

// Cancel ends uploadID's summary job because its upload was cancelled or
// failed: the engine is stopped, nothing goes to Drive, and a local copy
// already saved is kept (FR-016). Marked first, killed second, as in
// captions, so a cancel between two steps can't be missed.
func (w *Worker) Cancel(uploadID int64) {
	db := w.deps.DB
	j, err := db.GetSummaryJobByUpload(uploadID)
	if err != nil || (j.Status != storage.SummaryWaiting && j.Status != storage.SummaryInProgress) {
		return
	}
	if err := db.SetSummaryCancelled(j.ID, ""); err != nil {
		return
	}
	w.mu.Lock()
	if cancel := w.running[uploadID]; cancel != nil {
		cancel()
	}
	w.mu.Unlock()
	w.step.Lock()
	defer w.step.Unlock()
	w.afterEnd(j, true)
}

// Retry puts a failed summary back in line, reusing its transcript
// ("Try again", FR-009).
func (w *Worker) Retry(uploadID int64) error {
	j, err := w.deps.DB.GetSummaryJobByUpload(uploadID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(w.workDir(j.ID), "transcript.srt")); err != nil {
		return errors.New(NoteTranscriptGone)
	}
	if err := w.deps.DB.ResetSummaryForRetry(j.ID); err != nil {
		return err
	}
	w.mu.Lock()
	delete(w.attempts, uploadID)
	delete(w.retryAt, uploadID)
	delete(w.reloaded, uploadID)
	w.mu.Unlock()
	w.emit(j.ID)
	w.Wake()
	return nil
}

// Run processes jobs until ctx ends.
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

// RunOnce advances the first job that can by one phase.
func (w *Worker) RunOnce(ctx context.Context) bool {
	w.step.Lock()
	defer w.step.Unlock()
	jobs, err := w.deps.DB.ListActiveSummaryJobs()
	if err != nil {
		logging.Warn("could not list summary jobs", "error", err)
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

func (w *Worker) askConsentIfNeeded(jobs []*storage.SummaryJob) {
	if c, err := w.deps.DB.SummaryModelConsent(); err != nil || c != storage.ConsentUnasked {
		return
	}
	for _, j := range jobs {
		if j.Phase == storage.SummaryPhaseAwaitingConsent {
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

func (w *Worker) advance(parent context.Context, j *storage.SummaryJob) bool {
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
	if cur, err := w.deps.DB.GetSummaryJob(j.ID); err != nil ||
		(cur.Status != storage.SummaryWaiting && cur.Status != storage.SummaryInProgress) {
		return false
	}

	db := w.deps.DB
	switch j.Phase {
	case storage.SummaryPhaseAwaitingConsent:
		switch c, _ := db.SummaryModelConsent(); c {
		case storage.ConsentAccepted:
			return w.setPhase(j, storage.SummaryWaiting, w.afterConsentPhase())
		case storage.ConsentDeclined:
			return w.endJob(j, func() error { return db.SetSummaryCancelled(j.ID, NoteTurnedOff) }, true)
		}
		return false
	case storage.SummaryPhaseDownloadingModel:
		return w.downloadModel(ctx, j)
	case storage.SummaryPhaseWaitingForEngine, storage.SummaryPhaseSummarising:
		return w.summarise(ctx, j)
	case storage.SummaryPhaseWaitingForVideo:
		u, err := db.GetUpload(j.UploadID)
		if err != nil {
			return false
		}
		switch u.Status {
		case storage.UploadSucceeded:
			return w.setPhase(j, storage.SummaryInProgress, storage.SummaryPhaseUploadingSummary)
		case storage.UploadFailed, storage.UploadCancelled:
			return w.endJob(j, func() error { return db.SetSummaryCancelled(j.ID, "") }, true)
		}
		return false
	case storage.SummaryPhaseUploadingSummary:
		return w.upload(ctx, j)
	}
	return false
}

func (w *Worker) downloadModel(ctx context.Context, j *storage.SummaryJob) bool {
	if modelfetch.Present(w.modelDir(), w.deps.Model) {
		return w.setPhase(j, storage.SummaryWaiting, storage.SummaryPhaseWaitingForEngine)
	}
	if w.deps.Model.FileName == "" {
		return w.fail(j, ReasonModelNotChosen)
	}
	w.setPhase(j, storage.SummaryInProgress, storage.SummaryPhaseDownloadingModel)
	w.fetchMu.Lock()
	err := w.deps.FetchModel(ctx, w.deps.HTTPClient, w.modelDir(), w.deps.Model, func(done, total int64) {
		if total > 0 {
			w.setProgress(j, int(done*100/total))
		}
	})
	w.fetchMu.Unlock()
	switch {
	case err == nil:
		return w.setPhase(j, storage.SummaryWaiting, storage.SummaryPhaseWaitingForEngine)
	case ctx.Err() != nil:
		return false
	case errors.Is(err, modelfetch.ErrNotEnoughSpace):
		return w.fail(j, NoteNoDisk)
	default:
		logging.Warn("summary model download failed; will retry", "uploadId", j.UploadID, "error", err)
		w.scheduleRetry(j)
		return false
	}
}

// summarise runs the engine over the transcript and saves the checked
// summary and its local copy (FR-001..FR-005, FR-007).
func (w *Worker) summarise(ctx context.Context, j *storage.SummaryJob) bool {
	db := w.deps.DB
	dir := w.workDir(j.ID)
	resultPath := filepath.Join(dir, "summary.json")
	if _, err := os.Stat(resultPath); err == nil {
		// Done before a restart: just make sure the local copy exists.
		return w.finishSummary(j, resultPath)
	}
	if w.deps.CaptionsBusy() {
		return false // captions go first
	}
	ok, reason, bin := w.deps.Availability()
	if !ok {
		return w.fail(j, reason)
	}
	f, err := os.Open(filepath.Join(dir, "transcript.srt"))
	if err != nil {
		return w.fail(j, NoteTranscriptGone)
	}
	transcript, err := FromSRT(f)
	f.Close()
	if err != nil {
		return w.fail(j, NoteTranscriptGone)
	}

	release, err := heavywork.Acquire(ctx, "summaries")
	if err != nil {
		return false
	}
	defer release()
	if !w.setPhase(j, storage.SummaryInProgress, storage.SummaryPhaseSummarising) {
		return false
	}
	engine, stop, err := w.deps.Engine(ctx, bin, modelfetch.Path(w.modelDir(), w.deps.Model))
	if err != nil {
		return w.engineFailed(ctx, j, err)
	}
	defer stop()

	parts := transcript.Parts()
	var sum *Summary
	if len(parts) == 1 {
		sum, err = engine.Single(ctx, transcript.Text())
	} else {
		sum, err = w.summariseParts(ctx, j, engine, parts, transcript.Duration())
	}
	if err != nil {
		return w.engineFailed(ctx, j, err)
	}

	result := Verify(sum, transcript)
	share, err := CleanShareMessage(result.Summary.ShareMessage)
	if err != nil {
		return w.engineFailed(ctx, j, err)
	}
	result.Summary.ShareMessage = share
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil || os.WriteFile(resultPath, b, 0o600) != nil {
		return w.fail(j, NoteUnusable)
	}
	db.SetSummaryUnverifiedQuotes(j.ID, result.UnverifiedQuotes)
	return w.finishSummary(j, resultPath)
}

// summariseParts takes notes part by part, saving each so a restart
// resumes at the next one, then combines them (research.md §4).
func (w *Worker) summariseParts(ctx context.Context, j *storage.SummaryJob, engine Summarizer, parts []Transcript, length time.Duration) (*Summary, error) {
	db := w.deps.DB
	dir := w.workDir(j.ID)
	cur, err := db.GetSummaryJob(j.ID)
	if err != nil {
		return nil, err
	}
	if cur.PartCount == nil || *cur.PartCount != len(parts) {
		if err := db.SetSummaryParts(j.ID, len(parts)); err != nil {
			return nil, err
		}
		cur.PartsDone = 0
	}
	notes := make([]PartNotes, len(parts))
	for i := range parts {
		path := filepath.Join(dir, fmt.Sprintf("part-%03d.json", i))
		if i < cur.PartsDone {
			if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &notes[i]) == nil {
				continue
			}
		}
		n, err := engine.Notes(ctx, parts[i].Text(), i, len(parts))
		if err != nil {
			return nil, err
		}
		notes[i] = *n
		b, _ := json.Marshal(n)
		if err := os.WriteFile(path, b, 0o600); err != nil {
			return nil, err
		}
		if err := db.SetSummaryPartsDone(j.ID, i+1); err != nil {
			return nil, err
		}
		w.setProgress(j, (i+1)*100/(len(parts)+1))
	}
	return engine.Combine(ctx, notes, humanLength(length))
}

func humanLength(d time.Duration) string {
	if d >= time.Hour {
		return fmt.Sprintf("%d hours %d minutes", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%d minutes", int(d.Minutes())+1)
}

// engineFailed applies research.md §8's table.
func (w *Worker) engineFailed(ctx context.Context, j *storage.SummaryJob, err error) bool {
	db := w.deps.DB
	switch {
	case ctx.Err() != nil:
		return false
	case errors.Is(err, ErrOutOfMemory):
		return w.fail(j, NoteNoMemory)
	case errors.Is(err, ErrLoadFailed):
		w.mu.Lock()
		again := !w.reloaded[j.UploadID]
		w.reloaded[j.UploadID] = true
		w.mu.Unlock()
		logging.Warn("summary model failed to load", "uploadId", j.UploadID, "redownloading", again, "error", err)
		if again {
			os.Remove(modelfetch.Path(w.modelDir(), w.deps.Model))
			return w.setPhase(j, storage.SummaryWaiting, storage.SummaryPhaseDownloadingModel)
		}
		return w.fail(j, NoteLoadFailed)
	case errors.Is(err, ErrUnusable):
		n, aerr := db.AddSummaryAttempt(j.ID)
		if aerr != nil {
			return false
		}
		logging.Warn("summary answer unusable", "uploadId", j.UploadID, "attempt", n, "error", err)
		if n >= MaxAttempts {
			return w.fail(j, NoteUnusable)
		}
		return w.setPhase(j, storage.SummaryWaiting, storage.SummaryPhaseWaitingForEngine)
	default:
		logging.Warn("summary engine error; will retry", "uploadId", j.UploadID, "error", err)
		w.setPhase(j, storage.SummaryWaiting, storage.SummaryPhaseWaitingForEngine)
		w.scheduleRetry(j)
		return false
	}
}

// finishSummary saves the local Markdown copy next to the video (if not
// already) and waits for the video (FR-007).
func (w *Worker) finishSummary(j *storage.SummaryJob, resultPath string) bool {
	db := w.deps.DB
	cur, err := db.GetSummaryJob(j.ID)
	if err != nil {
		return false
	}
	if cur.LocalCopyPath == nil {
		u, err := db.GetUpload(j.UploadID)
		if err != nil {
			return false
		}
		r, err := readResult(resultPath)
		if err != nil {
			os.Remove(resultPath)
			return w.setPhase(j, storage.SummaryWaiting, storage.SummaryPhaseWaitingForEngine)
		}
		md := filepath.Join(w.workDir(j.ID), "summary.md")
		if err := os.WriteFile(md, []byte(Markdown(u.LocalPath, r)), 0o600); err == nil {
			if saved, err := captions.SaveBesideVideo(u.LocalPath, md, w.deps.DownloadsDir, " — Summary", ".md"); err != nil {
				logging.Warn("could not save the local summary copy", "uploadId", j.UploadID, "error", err)
			} else if err := db.SetSummaryLocalCopy(j.ID, saved); err != nil {
				return false
			}
		}
	}
	return w.setPhase(j, storage.SummaryInProgress, storage.SummaryPhaseWaitingForVideo)
}

func (w *Worker) upload(ctx context.Context, j *storage.SummaryJob) bool {
	db := w.deps.DB
	u, err := db.GetUpload(j.UploadID)
	if err != nil {
		return false
	}
	r, err := readResult(filepath.Join(w.workDir(j.ID), "summary.json"))
	if err != nil {
		return w.setPhase(j, storage.SummaryWaiting, storage.SummaryPhaseWaitingForEngine)
	}
	up, err := w.deps.Uploader(ctx)
	if err != nil {
		w.scheduleRetry(j)
		return false
	}
	if existing, err := up.Find(ctx, j.UploadID, u.DriveFolderID); err != nil {
		w.scheduleRetry(j)
		return false
	} else if existing != nil {
		return w.endJob(j, func() error { return db.SetSummaryDone(j.ID, existing.FileID, existing.WebViewLink, "") }, true)
	}
	base := strings.TrimSuffix(filepath.Base(u.LocalPath), filepath.Ext(u.LocalPath))
	name, err := up.ChooseName(ctx, u.DriveFolderID, base)
	if err != nil {
		w.scheduleRetry(j)
		return false
	}
	res, err := up.Create(ctx, j.UploadID, u.DriveFolderID, name, strings.NewReader(HTML(u.LocalPath, r)))
	if err != nil {
		logging.Warn("summary doc upload failed; will retry", "uploadId", j.UploadID, "error", err)
		w.scheduleRetry(j)
		return false
	}
	return w.endJob(j, func() error { return db.SetSummaryDone(j.ID, res.FileID, res.WebViewLink, name) }, true)
}

func readResult(path string) (Result, error) {
	var r Result
	b, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(b, &r)
}

func (w *Worker) setPhase(j *storage.SummaryJob, status storage.SummaryStatus, phase storage.SummaryPhase) bool {
	if err := w.deps.DB.SetSummaryPhase(j.ID, status, phase); err != nil {
		return false
	}
	w.mu.Lock()
	delete(w.retryAt, j.UploadID)
	w.mu.Unlock()
	w.emit(j.ID)
	return true
}

func (w *Worker) setProgress(j *storage.SummaryJob, percent int) {
	w.mu.Lock()
	due := time.Since(w.lastProgress[j.UploadID]) >= time.Second || percent >= 100
	if due {
		w.lastProgress[j.UploadID] = time.Now()
	}
	w.mu.Unlock()
	if due && w.deps.DB.SetSummaryProgress(j.ID, percent) == nil {
		w.emit(j.ID)
	}
}

// fail ends j with a reason; its folder (and transcript) is kept for Try again.
func (w *Worker) fail(j *storage.SummaryJob, note string) bool {
	return w.endJob(j, func() error { return w.deps.DB.SetSummaryFailed(j.ID, note) }, false)
}

// endJob applies the final transition and its aftermath. removeFiles is
// false for failures, which keep the transcript so "Try again" can reuse it.
func (w *Worker) endJob(j *storage.SummaryJob, transition func() error, removeFiles bool) bool {
	if err := transition(); err != nil {
		return false
	}
	w.afterEnd(j, removeFiles)
	return true
}

func (w *Worker) afterEnd(j *storage.SummaryJob, removeFiles bool) {
	if removeFiles {
		os.RemoveAll(w.workDir(j.ID))
	}
	w.mu.Lock()
	delete(w.attempts, j.UploadID)
	delete(w.retryAt, j.UploadID)
	delete(w.lastProgress, j.UploadID)
	w.mu.Unlock()
	w.emit(j.ID)
}

func (w *Worker) scheduleRetry(j *storage.SummaryJob) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := w.attempts[j.UploadID]
	w.attempts[j.UploadID] = n + 1
	w.retryAt[j.UploadID] = time.Now().Add(w.deps.RetryDelay(n))
}

func (w *Worker) emit(jobID int64) {
	if j, err := w.deps.DB.GetSummaryJob(jobID); err == nil {
		w.deps.Emit(ToEvent(j))
	}
}

// CleanupOrphans deletes work folders of jobs that are done, cancelled or
// gone. Failed jobs keep theirs, so Try again can reuse the transcript.
func (w *Worker) CleanupOrphans() {
	entries, err := os.ReadDir(filepath.Join(w.deps.DataDir, "summaries"))
	if err != nil {
		return
	}
	for _, e := range entries {
		id, err := strconv.ParseInt(e.Name(), 10, 64)
		if err != nil {
			continue
		}
		j, err := w.deps.DB.GetSummaryJob(id)
		if errors.Is(err, storage.ErrSummaryJobNotFound) || (err == nil && (j.Status == storage.SummaryDone || j.Status == storage.SummaryCancelled)) {
			os.RemoveAll(filepath.Join(w.deps.DataDir, "summaries", e.Name()))
		}
	}
}

func (w *Worker) modelDir() string { return filepath.Join(w.deps.DataDir, "models") }

func (w *Worker) workDir(jobID int64) string {
	return filepath.Join(w.deps.DataDir, "summaries", strconv.FormatInt(jobID, 10))
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o600)
}
