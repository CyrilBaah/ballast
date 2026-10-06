package captions

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAvailability(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		ok, reason, _ := Availability()
		if ok || reason != ReasonUnsupportedSystem {
			t.Fatalf("Availability() on %s/%s = %v, %q; want unavailable with %q", runtime.GOOS, runtime.GOARCH, ok, reason, ReasonUnsupportedSystem)
		}
		return
	}

	bin := filepath.Join(t.TempDir(), "whisper-cli")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvWhisperCLI, bin)
	ok, reason, path := Availability()
	if !ok || path != bin || reason != "" {
		t.Fatalf("Availability() with %s set = %v, %q, %q; want available at %s", EnvWhisperCLI, ok, reason, path, bin)
	}

	t.Setenv(EnvWhisperCLI, filepath.Join(t.TempDir(), "missing"))
	ok, reason, _ = Availability()
	// With the override pointing nowhere, only a binary next to the test
	// executable could satisfy it, and there is none.
	if ok || reason != ReasonEngineMissing {
		t.Fatalf("Availability() with a missing binary = %v, %q; want unavailable with %q", ok, reason, ReasonEngineMissing)
	}
}
