package captions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTranscribeWritesSRTAndReportsProgress(t *testing.T) {
	t.Setenv("FAKEWHISPER_MODE", "ok")
	wav := writeTestWAV(t, 25*time.Second)
	out := filepath.Join(t.TempDir(), "piece-0")
	var last int
	tr := Transcriber{BinPath: fakeWhisperBin, ModelPath: "model.bin"}

	got, err := tr.Transcribe(context.Background(), wav, "en", out, func(p int) { last = p })
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if got != out+".srt" || last != 100 {
		t.Fatalf("Transcribe = %q, last progress %d; want %q and 100", got, last, out+".srt")
	}
	f, _ := os.Open(got)
	defer f.Close()
	cues, err := ParseSRT(f)
	if err != nil || len(cues) == 0 {
		t.Fatalf("output SRT = %v cues, err %v", len(cues), err)
	}
}

func TestTranscribeReportsEngineFailure(t *testing.T) {
	t.Setenv("FAKEWHISPER_MODE", "crash")
	tr := Transcriber{BinPath: fakeWhisperBin, ModelPath: "model.bin"}
	_, err := tr.Transcribe(context.Background(), writeTestWAV(t, 5*time.Second), "en", filepath.Join(t.TempDir(), "p"), nil)
	if err == nil || !strings.Contains(err.Error(), "failed to load model") {
		t.Fatalf("Transcribe err = %v, want the engine's last message", err)
	}
}

func TestTranscribeCancelKillsEngine(t *testing.T) {
	t.Setenv("FAKEWHISPER_MODE", "hang")
	tr := Transcriber{BinPath: fakeWhisperBin, ModelPath: "model.bin"}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		_, err := tr.Transcribe(ctx, writeTestWAV(t, 5*time.Second), "en", filepath.Join(t.TempDir(), "p"), nil)
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("Transcribe err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelling didn't stop the speech engine within 2 s")
	}
}
