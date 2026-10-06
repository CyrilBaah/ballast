package captions

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ballast/internal/drive"
	"ballast/internal/events"
	"ballast/internal/modelfetch"
	"ballast/internal/storage"
)

// fakeUploader stands in for Drive.
type fakeUploader struct {
	mu       sync.Mutex
	existing map[int64]*drive.UploadResult // already-uploaded caption files, by upload id
	names    []string                      // names already in the folder
	uploaded []string                      // names created
	content  []string
	err      error
}

func (f *fakeUploader) Find(_ context.Context, uploadID int64, _ string) (*drive.UploadResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.existing[uploadID], nil
}

func (f *fakeUploader) ChooseName(_ context.Context, _ string, base, ext string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return FreeName(base, ext, func(n string) bool {
		for _, x := range append(f.names, f.uploaded...) {
			if x == n {
				return true
			}
		}
		return false
	}), nil
}

func (f *fakeUploader) Upload(_ context.Context, _ int64, _ string, name string, r io.Reader) (*drive.UploadResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	b, _ := io.ReadAll(r)
	f.uploaded = append(f.uploaded, name)
	f.content = append(f.content, string(b))
	return &drive.UploadResult{FileID: "id-" + name, WebViewLink: "https://drive/" + name}, nil
}

type harness struct {
	t         *testing.T
	db        *storage.DB
	w         *Worker
	up        *fakeUploader
	videoDir  string
	mu        sync.Mutex
	events    []events.CaptionJob
	consents  int
	ready     []int64
	noScript  []string
	extractOf time.Duration
	extractFn func(ctx context.Context, video, out string) error
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db, err := storage.OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := &harness{t: t, db: db, up: &fakeUploader{}, videoDir: t.TempDir(), extractOf: 25 * time.Second}
	dataDir := t.TempDir()
	modelSpec := modelfetch.Spec{FileName: "fake-model.bin", Size: 4}
	h.w = NewWorker(Deps{
		DB:           db,
		DataDir:      dataDir,
		DownloadsDir: t.TempDir(),
		Model:        modelSpec,
		FetchModel: func(_ context.Context, _ *http.Client, dir string, spec modelfetch.Spec, progress func(int64, int64)) error {
			if progress != nil {
				progress(spec.Size, spec.Size)
			}
			os.MkdirAll(dir, 0o700)
			return os.WriteFile(modelfetch.Path(dir, spec), []byte("fake"), 0o644)
		},
		Availability: func() (bool, string, string) { return true, "", fakeWhisperBin },
		Extract: func(ctx context.Context, video, out string) error {
			if h.extractFn != nil {
				return h.extractFn(ctx, video, out)
			}
			return copyFile(writeTestWAV(t, h.extractOf), out)
		},
		Uploader: func(context.Context) (Uploader, error) { return h.up, nil },
		Emit: func(j events.CaptionJob) {
			h.mu.Lock()
			h.events = append(h.events, j)
			h.mu.Unlock()
		},
		EmitConsentNeeded: func(int64) { h.mu.Lock(); h.consents++; h.mu.Unlock() },
		OnTranscriptReady: func(id int64, _ string) { h.mu.Lock(); h.ready = append(h.ready, id); h.mu.Unlock() },
		OnNoTranscript:    func(_ int64, reason string) { h.mu.Lock(); h.noScript = append(h.noScript, reason); h.mu.Unlock() },
		RetryDelay:        func(int) time.Duration { return 0 },
	})
	return h
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// newUpload creates an in-progress upload of a video file named name.
func (h *harness) newUpload(name string) *storage.Upload {
	h.t.Helper()
	path := filepath.Join(h.videoDir, name)
	os.WriteFile(path, []byte("video"), 0o644)
	u, err := h.db.CreateUpload(path, 5, time.Now(), "folder-1", "Sermons")
	if err != nil {
		h.t.Fatal(err)
	}
	return u
}

func (h *harness) succeedUpload(id int64) {
	h.t.Helper()
	if err := h.db.SetUploadInProgress(id); err != nil {
		h.t.Fatal(err)
	}
	if err := h.db.SetUploadSucceeded(id, "vid", "https://drive/vid"); err != nil {
		h.t.Fatal(err)
	}
	h.w.VideoSucceeded(id)
}

func (h *harness) job(uploadID int64) *storage.CaptionJob {
	h.t.Helper()
	j, err := h.db.GetCaptionJobByUpload(uploadID)
	if err != nil {
		h.t.Fatalf("GetCaptionJobByUpload(%d): %v", uploadID, err)
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

func TestWorkerHappyPath(t *testing.T) {
	t.Setenv("FAKEWHISPER_MODE", "ok")
	h := newHarness(t)
	h.db.SetCaptionModelConsent(storage.ConsentAccepted)
	u := h.newUpload("Sermon.mp4")

	if _, err := h.w.Enqueue(u.ID); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	h.w.RunUntilIdle(context.Background())

	j := h.job(u.ID)
	if j.Status != storage.CaptionInProgress || j.Phase != storage.PhaseWaitingForVideo {
		t.Fatalf("before the video lands: job = %s/%s, want in_progress/waiting_for_video", j.Status, j.Phase)
	}
	if j.LocalCopyPath == nil || filepath.Dir(*j.LocalCopyPath) != h.videoDir {
		t.Fatalf("local copy = %v, want next to the video in %s", j.LocalCopyPath, h.videoDir)
	}
	if len(h.up.uploaded) != 0 {
		t.Fatal("caption file uploaded before the video was confirmed in Drive")
	}
	if len(h.ready) != 1 || h.ready[0] != u.ID {
		t.Fatalf("OnTranscriptReady calls = %v", h.ready)
	}

	h.succeedUpload(u.ID)
	h.w.RunUntilIdle(context.Background())

	j = h.job(u.ID)
	if j.Status != storage.CaptionDone || j.DriveFileName == nil || *j.DriveFileName != "Sermon.srt" {
		t.Fatalf("after the video lands: job = %+v", j)
	}
	if len(h.up.uploaded) != 1 || !strings.Contains(h.up.content[0], "line 1") {
		t.Fatalf("uploaded = %v", h.up.uploaded)
	}
	want := []string{
		"in_progress/downloading_model", "in_progress/extracting_audio", "in_progress/transcribing",
		"in_progress/waiting_for_video", "in_progress/uploading_captions", "done/",
	}
	if got := h.phases(u.ID); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("phases = %v\nwant     %v", got, want)
	}
	if _, err := os.Stat(h.w.workDir(j.ID)); !os.IsNotExist(err) {
		t.Fatal("work folder left behind after the job finished")
	}
}

func TestWorkerAsksForConsentOnce(t *testing.T) {
	t.Setenv("FAKEWHISPER_MODE", "ok")
	h := newHarness(t)
	a, b := h.newUpload("A.mp4"), h.newUpload("B.mov")
	h.w.Enqueue(a.ID)
	h.w.Enqueue(b.ID)
	h.w.RunUntilIdle(context.Background())

	if h.consents != 1 {
		t.Fatalf("consent prompts = %d, want exactly 1", h.consents)
	}
	if j := h.job(a.ID); j.Status != storage.CaptionWaiting || j.Phase != storage.PhaseAwaitingConsent {
		t.Fatalf("job before consent = %s/%s", j.Status, j.Phase)
	}

	if err := h.w.AnswerConsent(true); err != nil {
		t.Fatal(err)
	}
	h.w.RunUntilIdle(context.Background())
	for _, u := range []*storage.Upload{a, b} {
		if j := h.job(u.ID); j.Phase != storage.PhaseWaitingForVideo {
			t.Fatalf("upload %d after consent = %s/%s, want waiting_for_video", u.ID, j.Status, j.Phase)
		}
	}
}

func TestWorkerDeclinedConsentTurnsCaptionsOff(t *testing.T) {
	h := newHarness(t)
	u := h.newUpload("A.mp4")
	h.w.Enqueue(u.ID)
	h.w.RunUntilIdle(context.Background())
	if err := h.w.AnswerConsent(false); err != nil {
		t.Fatal(err)
	}
	j := h.job(u.ID)
	if j.Status != storage.CaptionCancelled || j.Note == nil || *j.Note != "Captions were turned off" {
		t.Fatalf("job after declining = %+v", j)
	}
	if on, _ := h.db.CaptionsEnabled(); on {
		t.Fatal("declining the download left captions on")
	}
}

func TestEnqueueRules(t *testing.T) {
	h := newHarness(t)
	h.db.SetCaptionModelConsent(storage.ConsentAccepted)

	pdf := h.newUpload("notes.pdf")
	if j, err := h.w.Enqueue(pdf.ID); err != nil || j != nil {
		t.Fatalf("Enqueue(pdf) = %v, %v; want no job", j, err)
	}

	mkv := h.newUpload("talk.mkv")
	j, err := h.w.Enqueue(mkv.ID)
	if err != nil || j == nil || j.Status != storage.CaptionFailed || *j.Note != "Captions aren't supported for .mkv files yet" {
		t.Fatalf("Enqueue(mkv) = %+v, %v", j, err)
	}

	h.db.SetCaptionLanguage("auto")
	mp4 := h.newUpload("talk.mp4")
	j, _ = h.w.Enqueue(mp4.ID)
	if j == nil || j.Language != "auto" {
		t.Fatalf("Enqueue(mp4) job = %+v, want language copied from settings", j)
	}

	h.db.SetCaptionsEnabled(false)
	off := h.newUpload("later.mp4")
	if j, _ := h.w.Enqueue(off.ID); j != nil {
		t.Fatal("Enqueue created a job with captions turned off")
	}
}

func TestEnqueueWhenUnavailable(t *testing.T) {
	h := newHarness(t)
	h.w.deps.Availability = func() (bool, string, string) { return false, ReasonUnsupportedSystem, "" }
	if j, err := h.w.Enqueue(h.newUpload("a.mp4").ID); err != nil || j != nil {
		t.Fatalf("Enqueue on an unsupported system = %v, %v; want no job", j, err)
	}
}

func TestWorkerRunsJobsOneAtATime(t *testing.T) {
	t.Setenv("FAKEWHISPER_MODE", "ok")
	h := newHarness(t)
	h.db.SetCaptionModelConsent(storage.ConsentAccepted)

	var mu sync.Mutex
	active, maxActive := 0, 0
	h.extractFn = func(ctx context.Context, video, out string) error {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		return copyFile(writeTestWAV(t, 5*time.Second), out)
	}
	for _, n := range []string{"a.mp4", "b.mp4", "c.mp4"} {
		h.w.Enqueue(h.newUpload(n).ID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.w.Run(ctx)
	go h.w.Run(ctx) // even two loops must not process two jobs at once

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		jobs, _ := h.db.ListActiveCaptionJobs()
		waiting := 0
		for _, j := range jobs {
			if j.Phase == storage.PhaseWaitingForVideo {
				waiting++
			}
		}
		if waiting == 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if maxActive != 1 {
		t.Fatalf("up to %d jobs extracted at once, want 1", maxActive)
	}
}

func TestSecondUploadOfSameNameGetsNumberedCaption(t *testing.T) {
	t.Setenv("FAKEWHISPER_MODE", "ok")
	h := newHarness(t)
	h.db.SetCaptionModelConsent(storage.ConsentAccepted)
	h.up.names = []string{"Sermon.srt"}
	u := h.newUpload("Sermon.mp4")
	h.w.Enqueue(u.ID)
	h.w.RunUntilIdle(context.Background())
	h.succeedUpload(u.ID)
	h.w.RunUntilIdle(context.Background())
	if j := h.job(u.ID); j.DriveFileName == nil || *j.DriveFileName != "Sermon (2).srt" {
		t.Fatalf("drive name = %v, want Sermon (2).srt", j.DriveFileName)
	}
}

func TestWorkerAdoptsAlreadyUploadedCaption(t *testing.T) {
	t.Setenv("FAKEWHISPER_MODE", "ok")
	h := newHarness(t)
	h.db.SetCaptionModelConsent(storage.ConsentAccepted)
	u := h.newUpload("Sermon.mp4")
	h.up.existing = map[int64]*drive.UploadResult{u.ID: {FileID: "earlier", WebViewLink: "https://drive/earlier"}}
	h.w.Enqueue(u.ID)
	h.w.RunUntilIdle(context.Background())
	h.succeedUpload(u.ID)
	h.w.RunUntilIdle(context.Background())
	j := h.job(u.ID)
	if j.Status != storage.CaptionDone || *j.DriveFileLink != "https://drive/earlier" || len(h.up.uploaded) != 0 {
		t.Fatalf("job = %+v, uploads = %v; want the earlier file adopted, nothing re-uploaded", j, h.up.uploaded)
	}
}
