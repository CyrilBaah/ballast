package summaries

import (
	"os"
	"path/filepath"

	"ballast/internal/captions"
)

// EnvLlamaServer points at a llama-server to use instead of the one
// bundled next to Ballast's executable (research.md §15).
const EnvLlamaServer = "BALLAST_LLAMA_SERVER"

// Reasons summaries can be unavailable.
const (
	ReasonNeedsCaptions  = "Summaries need captions, which aren't available on this system yet"
	ReasonEngineMissing  = "The summary engine is missing from this copy of Ballast"
	ReasonModelNotChosen = "The summary model hasn't been chosen yet"
)

// Availability reports whether summaries can run here and where
// llama-server is. They need captions (their transcript), the engine, and
// a pinned model.
func Availability() (ok bool, reason, binPath string) {
	if ok, _, _ := captions.Availability(); !ok {
		return false, ReasonNeedsCaptions, ""
	}
	if Model.FileName == "" {
		return false, ReasonModelNotChosen, ""
	}
	if p := os.Getenv(EnvLlamaServer); p != "" {
		if isExecutable(p) {
			return true, "", p
		}
	} else if exe, err := os.Executable(); err == nil {
		if p := filepath.Join(filepath.Dir(exe), "llama-server"); isExecutable(p) {
			return true, "", p
		}
	}
	return false, ReasonEngineMissing, ""
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}
