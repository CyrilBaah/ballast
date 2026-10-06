package captions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// fakeWhisperBin is the fake speech engine (testdata/fakewhisper), built
// once for the whole test run.
var fakeWhisperBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakewhisper")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fakeWhisperBin = filepath.Join(dir, "whisper-cli")
	if out, err := exec.Command("go", "build", "-o", fakeWhisperBin, "./testdata/fakewhisper").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building fake whisper-cli: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
