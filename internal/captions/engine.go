package captions

import (
	"os"
	"path/filepath"
)

// EnvWhisperCLI points at a whisper-cli binary to use instead of the one
// bundled next to Ballast's executable -- for `wails dev` and tests
// (research.md §8).
const EnvWhisperCLI = "BALLAST_WHISPER_CLI"

// Reasons captions can be unavailable (FR-017).
const (
	ReasonUnsupportedSystem = "Captions aren't available on this system yet"
	ReasonEngineMissing     = "The speech engine is missing from this copy of Ballast"
)

// Availability reports whether captions can run on this machine and, if
// so, where the speech engine is. On anything but Apple-silicon macOS it
// is always unavailable (engine_other.go).
func Availability() (ok bool, reason string, binPath string) {
	if !platformSupported {
		return false, ReasonUnsupportedSystem, ""
	}
	if p := os.Getenv(EnvWhisperCLI); p != "" {
		if isExecutable(p) {
			return true, "", p
		}
	} else if exe, err := os.Executable(); err == nil {
		if p := filepath.Join(filepath.Dir(exe), "whisper-cli"); isExecutable(p) {
			return true, "", p
		}
	}
	return false, ReasonEngineMissing, ""
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}
