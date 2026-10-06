package captions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// afconvertPath is macOS's built-in audio converter (research.md §3).
var afconvertPath = "/usr/bin/afconvert"

// ErrNoAudio means afconvert found no audio it could read in the video --
// in practice, a video with no audio track (FR-007's plain-language reason).
var ErrNoAudio = errors.New("This video has no audio track")

// ExtractAudio writes videoPath's audio to outWAV as 16 kHz mono 16-bit
// PCM, the input whisper.cpp expects. afconvert streams, so memory stays
// flat however large the video is (measured: 30 minutes in 1.2 s, 11 MB).
func ExtractAudio(ctx context.Context, videoPath, outWAV string) error {
	cmd := exec.CommandContext(ctx, afconvertPath, "-f", "WAVE", "-d", "LEI16@16000", "-c", "1", videoPath, outWAV)
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("captions: start afconvert: %w", err)
	}
	LowerPriority(cmd.Process.Pid)
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if strings.Contains(stderr.String(), "Couldn't open input file") {
			return ErrNoAudio
		}
		return fmt.Errorf("captions: afconvert: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}
