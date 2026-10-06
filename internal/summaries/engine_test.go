package summaries

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"ballast/internal/captions"
	"ballast/internal/modelfetch"
)

func TestAvailability(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		if ok, reason, _ := Availability(); ok || reason != ReasonNeedsCaptions {
			t.Fatalf("Availability() = %v, %q; want %q off Apple silicon", ok, reason, ReasonNeedsCaptions)
		}
		return
	}
	bin := func(name string) string {
		p := filepath.Join(t.TempDir(), name)
		os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755)
		return p
	}
	t.Setenv(captions.EnvWhisperCLI, bin("whisper-cli"))
	orig := Model
	t.Cleanup(func() { Model = orig })

	Model = modelfetch.Spec{}
	if ok, reason, _ := Availability(); ok || reason != ReasonModelNotChosen {
		t.Fatalf("with no model pinned: %v, %q", ok, reason)
	}
	Model = modelfetch.Spec{FileName: "m.gguf", Size: 1}
	t.Setenv(EnvLlamaServer, filepath.Join(t.TempDir(), "missing"))
	if ok, reason, _ := Availability(); ok || reason != ReasonEngineMissing {
		t.Fatalf("with no engine: %v, %q", ok, reason)
	}
	server := bin("llama-server")
	t.Setenv(EnvLlamaServer, server)
	if ok, _, p := Availability(); !ok || p != server {
		t.Fatalf("with everything in place: %v, %q", ok, p)
	}
	t.Setenv(captions.EnvWhisperCLI, filepath.Join(t.TempDir(), "missing"))
	if ok, reason, _ := Availability(); ok || reason != ReasonNeedsCaptions {
		t.Fatalf("without captions: %v, %q", ok, reason)
	}
}
