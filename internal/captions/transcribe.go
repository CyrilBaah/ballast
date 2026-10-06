package captions

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Transcriber runs whisper-cli on one piece of audio at a time.
type Transcriber struct {
	BinPath   string
	ModelPath string
	Threads   int
}

var progressLine = regexp.MustCompile(`progress\s*=\s*(\d+)%`)

// Transcribe writes pieceWAV's captions to outBase+".srt" and returns that
// path. language is "en" or "auto". progress, if non-nil, gets whisper's
// own 0-100 progress for this piece. Cancelling ctx kills the process.
//
// -mc 0 stops whisper carrying text context between its 30-second windows,
// which on a real 43-minute sermon was what let one line repeat for seven
// minutes; -sns drops non-speech tokens (research.md §1, §5).
func (t Transcriber) Transcribe(ctx context.Context, pieceWAV, language, outBase string, progress func(int)) (string, error) {
	threads := t.Threads
	if threads < 1 {
		threads = 4
	}
	cmd := exec.CommandContext(ctx, t.BinPath,
		"-m", t.ModelPath,
		"-f", pieceWAV,
		"-l", language,
		"-osrt", "-of", outBase,
		"-pp", "-sns",
		"-mc", "0",
		"-t", strconv.Itoa(threads),
	)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("captions: whisper-cli stderr: %w", err)
	}
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("captions: start whisper-cli: %w", err)
	}
	lowerPriority(cmd.Process.Pid)

	var lastLine string
	sc := bufio.NewScanner(stderr)
	for sc.Scan() {
		line := sc.Text()
		if m := progressLine.FindStringSubmatch(line); m != nil {
			if p, err := strconv.Atoi(m[1]); err == nil && progress != nil {
				progress(p)
			}
			continue
		}
		if s := strings.TrimSpace(line); s != "" {
			lastLine = s
		}
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("captions: speech engine failed: %s", lastLine)
	}
	return outBase + ".srt", nil
}
