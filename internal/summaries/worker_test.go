package summaries

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ballast/internal/captions"
	"ballast/internal/drive"
	"ballast/internal/events"
	"ballast/internal/heavywork"
	"ballast/internal/modelfetch"
	"ballast/internal/storage"
)

// fakeEngine scripts what the "model" answers.
type fakeEngine struct {
	mu        sync.Mutex
	starts    int
	singles   int
	notes     int
	combines  int
	startErr  []error // returned by successive starts
	answerErr []error // returned by successive Single/Combine calls
	share     string
	block     chan struct{} // if set, Single waits on it (or ctx)
}

func (f *fakeEngine) engine(ctx context.Context, bin, model string) (Summarizer, func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	if len(f.startErr) > 0 {
		err := f.startErr[0]
		f.startErr = f.startErr[1:]
		if err != nil {
			return nil, nil, err
		}
	}
	return f, func() {}, nil
}

func (f *fakeEngine) answer() (*Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.answerErr) > 0 {
		err := f.answerErr[0]
		f.answerErr = f.answerErr[1:]
		if err != nil {
			return nil, err
		}
	}
	share := f.share
	if share == "" {
		share = "*Aksum*\nThe forgotten empire."
	}
	return &Summary{
		Overview:   "A talk about trade.",
		MainPoints: []MainPoint{{Title: "Trade", Detail: "It ruled the Red Sea."}},
		Quotes:     []Quote{{Text: "Money just moved.", StartSeconds: 10, SourceText: "Money just moved"}},
		Title:      "Aksum", Description: "The empire.", ShareMessage: share,
	}, nil
}

func (f *fakeEngine) Single(ctx context.Context, _ string) (*Summary, error) {
	f.mu.Lock()
	f.singles++
	block := f.block
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.answer()
}

func (f *fakeEngine) Notes(_ context.Context, _ string, i, _ int) (*PartNotes, error) {
	f.mu.Lock()
	f.notes++
	f.mu.Unlock()
	return &PartNotes{Points: []MainPoint{{Title: fmt.Sprintf("part %d", i)}}}, nil
}

func (f *fakeEngine) Combine(context.Context, []PartNotes, string) (*Summary, error) {
	f.mu.Lock()
	f.combines++
	f.mu.Unlock()
	return f.answer()
}

type fakeDocs struct {
	mu      sync.Mutex
	created []string
	html    []string
	err     error
}

func (d *fakeDocs) Find(context.Context, int64, string) (*drive.UploadResult, error) {
	return nil, d.err
}
func (d *fakeDocs) ChooseName(_ context.Context, _ string, base string) (string, error) {
	return base + " — Summary", d.err
}
func (d *fakeDocs) Create(_ context.Context, _ int64, _ string, name string, r io.Reader) (*drive.UploadResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return nil, d.err
	}
	b, _ := io.ReadAll(r)
	d.created = append(d.created, name)
	d.html = append(d.html, string(b))
	return &drive.UploadResult{FileID: "doc-" + name, WebViewLink: "https://docs/" + name}, nil
}

type harness struct {
	t        *testing.T
	db       *storage.DB
	w        *Worker
	eng      *fakeEngine
	docs     *fakeDocs
	videoDir string
	busy     bool
	consents int
	fetches  int
	mu       sync.Mutex
	events   []events.SummaryJob
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db, err := storage.OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := &harness{t: t, db: db, eng: &fakeEngine{}, docs: &fakeDocs{}, videoDir: t.TempDir()}
	spec := modelfetch.Spec{FileName: "summary-model.gguf", Size: 4}
	h.w = NewWorker(Deps{
		DB: db, DataDir: t.TempDir(), DownloadsDir: t.TempDir(), Model: spec,
		FetchModel: func(_ context.Context, _ *http.Client, dir string, s modelfetch.Spec, _ func(int64, int64)) error {
			h.fetches++
			os.MkdirAll(dir, 0o700)
			return os.WriteFile(modelfetch.Path(dir, s), []byte("fake"), 0o644)
		},
		Availability:      func() (bool, string, string) { return true, "", "/bin/true" },
		Engine:            h.eng.engine,
		Uploader:          func(context.Context) (Uploader, error) { return h.docs, nil },
		CaptionsBusy:      func() bool { return h.busy },
		Emit:              func(e events.SummaryJob) { h.mu.Lock(); h.events = append(h.events, e); h.mu.Unlock() },
		EmitConsentNeeded: func(int64) { h.consents++ },
		RetryDelay:        func(int) time.Duration { return 0 },
	})
	return h
}

func (h *harness) newUpload(name string) *storage.Upload {
	h.t.Helper()
	p := filepath.Join(h.videoDir, name)
	os.WriteFile(p, []byte("v"), 0o644)
	u, err := h.db.CreateUpload(p, 1, time.Now(), "folder-1", "F")
	if err != nil {
		h.t.Fatal(err)
	}
	return u
}

// transcriptReady hands over a short transcript the way captions does.
func (h *harness) transcriptReady(u *storage.Upload, lines ...string) {
	h.t.Helper()
	if len(lines) == 0 {
		lines = []string{"Welcome.", "Money just moved."}
	}
	var b strings.Builder
	for i, l := range lines {
		fmt.Fprintf(&b, "%d\n00:00:%02d,000 --> 00:00:%02d,000\n%s\n\n", i+1, i*10, i*10+9, l)
	}
	p := filepath.Join(h.t.TempDir(), "transcript.srt")
	os.WriteFile(p, []byte(b.String()), 0o644)
	h.w.TranscriptReady(u.ID, p)
	os.Remove(p) // captions deletes its copy; the job must have its own
}

func (h *harness) succeed(u *storage.Upload) {
	h.db.SetUploadInProgress(u.ID)
	h.db.SetUploadSucceeded(u.ID, "vid", "https://drive/vid")
	h.w.VideoSucceeded(u.ID)
}

func (h *harness) job(u *storage.Upload) *storage.SummaryJob {
	h.t.Helper()
	j, err := h.db.GetSummaryJobByUpload(u.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	return j
}

func (h *harness) phases(uploadID int64) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, e := range h.events {
		if e.UploadID != uploadID {
			continue
		}
		p := e.Status + "/" + e.Phase
		if len(out) == 0 || out[len(out)-1] != p {
			out = append(out, p)
		}
	}
	return out
}

func (h *harness) ready(name string) *storage.Upload {
	h.t.Helper()
	h.db.SetSummaryModelConsent(storage.ConsentAccepted)
	u := h.newUpload(name)
	if _, err := h.w.Enqueue(u.ID); err != nil {
		h.t.Fatal(err)
	}
	h.transcriptReady(u)
	h.w.RunUntilIdle(context.Background())
	return u
}

// --- US1 ---

func TestSummaryHappyPath(t *testing.T) {
	h := newHarness(t)
	u := h.newUpload("Aksum.mp4")
	if _, err := h.w.Enqueue(u.ID); err != nil {
		t.Fatal(err)
	}
	h.transcriptReady(u)
	h.w.RunUntilIdle(context.Background())
	if h.consents != 1 || h.job(u).Phase != storage.SummaryPhaseAwaitingConsent {
		t.Fatalf("consent prompts = %d, phase = %s", h.consents, h.job(u).Phase)
	}
	h.w.AnswerConsent(true)
	h.w.RunUntilIdle(context.Background())

	j := h.job(u)
	if j.Phase != storage.SummaryPhaseWaitingForVideo || j.LocalCopyPath == nil {
		t.Fatalf("before the video lands: %+v", j)
	}
	if want := filepath.Join(h.videoDir, "Aksum — Summary.md"); *j.LocalCopyPath != want {
		t.Fatalf("local copy at %s, want %s", *j.LocalCopyPath, want)
	}
	if len(h.docs.created) != 0 {
		t.Fatal("summary uploaded before the video arrived")
	}
	h.succeed(u)
	h.w.RunUntilIdle(context.Background())

	j = h.job(u)
	if j.Status != storage.SummaryDone || len(h.docs.created) != 1 || h.docs.created[0] != "Aksum — Summary" {
		t.Fatalf("after the video lands: %+v, docs %v", j, h.docs.created)
	}
	if !strings.Contains(h.docs.html[0], "Money just moved.") {
		t.Fatal("the verified quote is missing from the Google Doc")
	}
	want := []string{
		"waiting/waiting_for_captions", "waiting/awaiting_consent", "waiting/downloading_model",
		"in_progress/downloading_model", "waiting/waiting_for_engine", "in_progress/summarising",
		"in_progress/waiting_for_video", "in_progress/uploading_summary", "done/",
	}
	if got := h.phases(u.ID); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("phases = %v\nwant     %v", got, want)
	}
	if _, err := os.Stat(h.w.workDir(j.ID)); !os.IsNotExist(err) {
		t.Fatal("work folder left after done")
	}
}

func TestSummaryWaitsForCaptionsEngineToFinish(t *testing.T) {
	h := newHarness(t)
	h.busy = true
	u := h.ready("a.mp4")
	if h.job(u).Phase != storage.SummaryPhaseWaitingForEngine || h.eng.starts != 0 {
		t.Fatalf("summary started while captions were busy: %s, %d starts", h.job(u).Phase, h.eng.starts)
	}
	h.busy = false
	h.w.RunUntilIdle(context.Background())
	if h.job(u).Phase != storage.SummaryPhaseWaitingForVideo {
		t.Fatalf("phase = %s", h.job(u).Phase)
	}
}

func TestSummaryHoldsHeavyworkLock(t *testing.T) {
	h := newHarness(t)
	release, _ := heavywork.Acquire(context.Background(), "captions")
	h.db.SetSummaryModelConsent(storage.ConsentAccepted)
	u := h.newUpload("a.mp4")
	h.w.Enqueue(u.ID)
	h.transcriptReady(u)
	done := make(chan struct{})
	go func() { h.w.RunUntilIdle(context.Background()); close(done) }()
	time.Sleep(100 * time.Millisecond)
	if h.eng.starts != 0 {
		t.Fatal("the summary model started while the speech model held the lock")
	}
	release()
	<-done
	if h.job(u).Phase != storage.SummaryPhaseWaitingForVideo {
		t.Fatalf("phase = %s", h.job(u).Phase)
	}
}

func TestEnqueueRespectsSetting(t *testing.T) {
	h := newHarness(t)
	h.db.SetSummariesEnabled(false)
	if j, _ := h.w.Enqueue(h.newUpload("a.mp4").ID); j != nil {
		t.Fatal("job created with summaries off")
	}
}

func TestLongTranscriptUsesPartsAndCombine(t *testing.T) {
	h := newHarness(t)
	h.db.SetSummaryModelConsent(storage.ConsentAccepted)
	u := h.newUpload("long.mp4")
	h.w.Enqueue(u.ID)
	var b strings.Builder
	words := strings.Repeat("word ", 30)
	n := 0
	for at := 0; at < 90*60; at += 10 {
		n++
		fmt.Fprintf(&b, "%d\n%02d:%02d:%02d,000 --> %02d:%02d:%02d,000\n%s\n\n", n, at/3600, at/60%60, at%60, (at+9)/3600, (at+9)/60%60, (at+9)%60, words)
	}
	p := filepath.Join(t.TempDir(), "t.srt")
	os.WriteFile(p, []byte(b.String()), 0o644)
	h.w.TranscriptReady(u.ID, p)
	h.w.RunUntilIdle(context.Background())
	if h.eng.notes != 5 || h.eng.combines != 1 || h.eng.singles != 0 {
		t.Fatalf("notes %d, combines %d, singles %d; want 5, 1, 0", h.eng.notes, h.eng.combines, h.eng.singles)
	}
	if j := h.job(u); j.PartsDone != 5 || j.Phase != storage.SummaryPhaseWaitingForVideo {
		t.Fatalf("job = %+v", j)
	}
}

// --- US2 ---

func TestUnusableAnswersRetryThenFail(t *testing.T) {
	h := newHarness(t)
	h.eng.answerErr = []error{ErrUnusable, ErrUnusable, ErrUnusable}
	u := h.ready("a.mp4")
	j := h.job(u)
	if j.Status != storage.SummaryFailed || *j.Note != NoteUnusable || h.eng.singles != 3 {
		t.Fatalf("job = %+v after %d tries", j, h.eng.singles)
	}
	if _, err := os.Stat(filepath.Join(h.w.workDir(j.ID), "transcript.srt")); err != nil {
		t.Fatal("a failed summary lost its transcript; Try again needs it")
	}
}

func TestUnusableThenGoodAnswer(t *testing.T) {
	h := newHarness(t)
	h.eng.answerErr = []error{ErrUnusable, nil}
	u := h.ready("a.mp4")
	if j := h.job(u); j.Phase != storage.SummaryPhaseWaitingForVideo || j.Attempts != 1 {
		t.Fatalf("job = %+v", j)
	}
}

func TestShareMessageAboutTranscriptCountsAsUnusable(t *testing.T) {
	h := newHarness(t)
	h.eng.share = "The transcript was unclear."
	u := h.ready("a.mp4")
	if j := h.job(u); j.Status != storage.SummaryFailed || *j.Note != NoteUnusable {
		t.Fatalf("job = %+v", j)
	}
}

func TestLoadFailureRedownloadsOnce(t *testing.T) {
	h := newHarness(t)
	h.eng.startErr = []error{ErrLoadFailed, nil}
	u := h.ready("a.mp4")
	if j := h.job(u); j.Phase != storage.SummaryPhaseWaitingForVideo || h.fetches != 2 {
		t.Fatalf("job = %+v, fetches = %d; want one re-download then success", j, h.fetches)
	}
	h2 := newHarness(t)
	h2.eng.startErr = []error{ErrLoadFailed, ErrLoadFailed}
	u2 := h2.ready("b.mp4")
	if j := h2.job(u2); j.Status != storage.SummaryFailed || *j.Note != NoteLoadFailed {
		t.Fatalf("job = %+v; want failure after one re-download", j)
	}
}

func TestOutOfMemoryFails(t *testing.T) {
	h := newHarness(t)
	h.eng.startErr = []error{ErrOutOfMemory}
	u := h.ready("a.mp4")
	if j := h.job(u); j.Status != storage.SummaryFailed || *j.Note != NoteNoMemory {
		t.Fatalf("job = %+v", j)
	}
}

func TestNoTranscriptCancels(t *testing.T) {
	h := newHarness(t)
	u := h.newUpload("a.mp4")
	h.w.Enqueue(u.ID)
	h.w.NoTranscript(u.ID, captions.NoteNoSpeech)
	if j := h.job(u); j.Status != storage.SummaryCancelled || *j.Note != NoteNoSpeech {
		t.Fatalf("job = %+v", j)
	}
}

func TestCancelStopsEngineAndKeepsLocalCopy(t *testing.T) {
	h := newHarness(t)
	h.eng.block = make(chan struct{})
	h.db.SetSummaryModelConsent(storage.ConsentAccepted)
	u := h.newUpload("a.mp4")
	h.w.Enqueue(u.ID)
	h.transcriptReady(u)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go h.w.Run(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for h.job(u).Phase != storage.SummaryPhaseSummarising {
		if time.Now().After(deadline) {
			t.Fatal("never started summarising")
		}
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	h.w.Cancel(u.ID)
	if time.Since(start) > 2*time.Second {
		t.Fatal("Cancel didn't stop the engine promptly")
	}
	if j := h.job(u); j.Status != storage.SummaryCancelled || len(h.docs.created) != 0 {
		t.Fatalf("job = %+v", j)
	}

	// Cancelling after the local copy exists keeps it.
	h2 := newHarness(t)
	u2 := h2.ready("b.mp4")
	local := *h2.job(u2).LocalCopyPath
	h2.w.Cancel(u2.ID)
	if _, err := os.Stat(local); err != nil {
		t.Fatalf("local summary removed on cancel: %v", err)
	}
	if len(h2.docs.created) != 0 {
		t.Fatal("summary uploaded for a cancelled video")
	}
}

func TestFailedUploadCancelsSummary(t *testing.T) {
	h := newHarness(t)
	u := h.ready("a.mp4")
	h.db.SetUploadInProgress(u.ID)
	h.db.SetUploadFailed(u.ID, "x")
	h.w.RunUntilIdle(context.Background())
	if j := h.job(u); j.Status != storage.SummaryCancelled {
		t.Fatalf("job = %s", j.Status)
	}
}

func TestDriveErrorsRetry(t *testing.T) {
	h := newHarness(t)
	u := h.ready("a.mp4")
	h.docs.err = errors.New("drive: 503")
	h.succeed(u)
	h.w.RunUntilIdle(context.Background())
	if j := h.job(u); j.Phase != storage.SummaryPhaseUploadingSummary {
		t.Fatalf("job = %s/%s", j.Status, j.Phase)
	}
	h.docs.err = nil
	h.w.RunUntilIdle(context.Background())
	if j := h.job(u); j.Status != storage.SummaryDone {
		t.Fatalf("job = %s", j.Status)
	}
}

func TestTryAgainReusesTranscript(t *testing.T) {
	h := newHarness(t)
	h.eng.startErr = []error{ErrOutOfMemory}
	u := h.ready("a.mp4")
	if h.job(u).Status != storage.SummaryFailed {
		t.Fatal("setup: expected a failure")
	}
	if err := h.w.Retry(u.ID); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	h.w.RunUntilIdle(context.Background())
	if j := h.job(u); j.Phase != storage.SummaryPhaseWaitingForVideo {
		t.Fatalf("after Try again: %s/%s", j.Status, j.Phase)
	}
}

func TestRestartResumesFromSavedParts(t *testing.T) {
	h := newHarness(t)
	h.db.SetSummaryModelConsent(storage.ConsentAccepted)
	u := h.newUpload("long.mp4")
	j, _ := h.db.CreateSummaryJob(u.ID, "m")
	dir := h.w.workDir(j.ID)
	os.MkdirAll(dir, 0o700)
	var b strings.Builder
	words := strings.Repeat("word ", 30)
	n := 0
	for at := 0; at < 90*60; at += 10 {
		n++
		fmt.Fprintf(&b, "%d\n%02d:%02d:%02d,000 --> %02d:%02d:%02d,000\n%s\n\n", n, at/3600, at/60%60, at%60, (at+9)/3600, (at+9)/60%60, (at+9)%60, words)
	}
	os.WriteFile(filepath.Join(dir, "transcript.srt"), []byte(b.String()), 0o600)
	for i := 0; i < 3; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("part-%03d.json", i)), []byte(`{"points":[{"title":"saved","detail":""}],"actions":[],"references":[],"quotes":[]}`), 0o600)
	}
	h.db.SetSummaryPhase(j.ID, storage.SummaryInProgress, storage.SummaryPhaseSummarising)
	h.db.SetSummaryParts(j.ID, 5)
	h.db.SetSummaryPartsDone(j.ID, 3)
	os.MkdirAll(h.w.modelDir(), 0o700)
	os.WriteFile(modelfetch.Path(h.w.modelDir(), h.w.deps.Model), []byte("fake"), 0o644)

	h.w.RunUntilIdle(context.Background())
	if h.eng.notes != 2 {
		t.Fatalf("re-took notes on %d parts, want only the 2 not yet saved", h.eng.notes)
	}
}

func TestRestartWithSavedSummarySkipsEngine(t *testing.T) {
	h := newHarness(t)
	u := h.newUpload("a.mp4")
	j, _ := h.db.CreateSummaryJob(u.ID, "m")
	dir := h.w.workDir(j.ID)
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "summary.json"), []byte(`{"Summary":{"overview":"o","main_points":[{"title":"t","detail":""}],"actions":[],"references":[],"quotes":[],"title":"T","description":"d","share_message":"s"}}`), 0o600)
	h.db.SetSummaryPhase(j.ID, storage.SummaryInProgress, storage.SummaryPhaseSummarising)
	h.w.RunUntilIdle(context.Background())
	if jj := h.job(u); jj.Phase != storage.SummaryPhaseWaitingForVideo || h.eng.starts != 0 || jj.LocalCopyPath == nil {
		t.Fatalf("job = %+v, engine starts %d", jj, h.eng.starts)
	}
}

func TestCleanupOrphansKeepsFailedJobs(t *testing.T) {
	h := newHarness(t)
	failed, _ := h.db.CreateSummaryJob(h.newUpload("a.mp4").ID, "m")
	h.db.SetSummaryFailed(failed.ID, "x")
	done, _ := h.db.CreateSummaryJob(h.newUpload("b.mp4").ID, "m")
	h.db.SetSummaryCancelled(done.ID, "")
	for _, id := range []int64{failed.ID, done.ID, 999} {
		os.MkdirAll(h.w.workDir(id), 0o700)
	}
	h.w.CleanupOrphans()
	if _, err := os.Stat(h.w.workDir(failed.ID)); err != nil {
		t.Error("a failed job's folder was removed; Try again needs it")
	}
	for _, id := range []int64{done.ID, 999} {
		if _, err := os.Stat(h.w.workDir(id)); !os.IsNotExist(err) {
			t.Errorf("folder %d kept", id)
		}
	}
}

// --- US4 ---

func TestLocalSummaryBeforeVideoFinishes(t *testing.T) {
	h := newHarness(t)
	u := h.ready("Sermon.mp4")
	j := h.job(u)
	if j.LocalCopyPath == nil {
		t.Fatal("no local summary while the video is still uploading")
	}
	b, _ := os.ReadFile(*j.LocalCopyPath)
	if !strings.Contains(string(b), "# Sermon — Summary") || !strings.Contains(string(b), "## Share message") {
		t.Fatalf("local summary = %q", b)
	}
	if len(h.docs.created) != 0 {
		t.Fatal("summary in Drive before the video")
	}
}
