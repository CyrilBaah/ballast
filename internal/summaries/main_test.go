package summaries

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// fakeLlamaBin is the fake llama-server (testdata/fakellama), built once.
var fakeLlamaBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakellama")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fakeLlamaBin = filepath.Join(dir, "llama-server")
	if out, err := exec.Command("go", "build", "-o", fakeLlamaBin, "./testdata/fakellama").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building fake llama-server: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
