package main

import (
	"context"
	"testing"

	"ballast/internal/drive"
)

// TestStartUploadRefusesASecondTransfer checks that an upload with a live
// transfer goroutine is never handed a second one -- two goroutines on
// the same row each finish their own Drive session, leaving duplicate
// files.
func TestStartUploadRefusesASecondTransfer(t *testing.T) {
	a := NewApp()
	a.ctx = context.Background()
	live := &uploadRun{cancel: func() {}}
	a.running[7] = live

	if a.startUpload(7, nil, "/tmp/x", "folder", 1, drive.IdentityBaseline{}, drive.ResumeState{}) {
		t.Fatal("startUpload started a second transfer for an upload that already has one")
	}
	if a.running[7] != live {
		t.Fatal("startUpload replaced the live transfer")
	}
}

func TestUploadRunBookkeeping(t *testing.T) {
	a := NewApp()
	cancelled := false
	run := &uploadRun{cancel: func() { cancelled = true }}
	a.running[3] = run

	if a.uploadIsRunning(3, run) {
		t.Error("a run should not count itself as another live transfer")
	}
	if !a.uploadIsRunning(3, nil) {
		t.Error("uploadIsRunning(nil) should see the live run")
	}

	// A run that has handed its upload over to a newer one must not
	// remove the newer one on its way out.
	newer := &uploadRun{cancel: func() {}}
	a.running[3] = newer
	a.finishRun(3, run)
	if a.running[3] != newer {
		t.Error("finishRun removed a newer run it no longer owned")
	}
	if !cancelled {
		t.Error("finishRun should cancel the finished run's context")
	}

	a.stopUpload(3)
	if a.uploadIsRunning(3, nil) {
		t.Error("stopUpload left the upload marked as running")
	}
}
