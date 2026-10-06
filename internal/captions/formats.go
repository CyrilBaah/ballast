package captions

import (
	"path/filepath"
	"strings"
)

// captionableExts are the video formats macOS's afconvert can read the
// audio from (research.md §3, §10).
var captionableExts = map[string]bool{"mp4": true, "mov": true, "m4v": true}

// unsupportedVideoExts are videos that still upload but whose audio Ballast
// can't read yet; their caption job ends straight away with a reason.
var unsupportedVideoExts = map[string]bool{
	"mkv": true, "avi": true, "webm": true, "wmv": true, "flv": true,
	"mpg": true, "mpeg": true, "3gp": true,
}

// Classify reports whether path is a video Ballast can caption, or, for a
// video it can't, its extension (lower-case, no dot). Anything else --
// not a video -- returns false, "" and gets no caption job (FR-010).
func Classify(path string) (captionable bool, unsupportedExt string) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	if captionableExts[ext] {
		return true, ""
	}
	if unsupportedVideoExts[ext] {
		return false, ext
	}
	return false, ""
}

// UnsupportedNote is the caption job's note for a video format Ballast
// can't read the audio from.
func UnsupportedNote(ext string) string {
	return "Captions aren't supported for ." + ext + " files yet"
}
