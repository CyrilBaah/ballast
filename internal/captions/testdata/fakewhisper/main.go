// Command fakewhisper stands in for whisper-cli in tests. It accepts the
// flags transcribe.go passes, prints whisper-style progress lines, and
// writes a deterministic .srt sized to the input WAV's duration.
// FAKEWHISPER_MODE picks the behaviour: ok (default), empty, repeat,
// crash, or hang.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	model := flag.String("m", "", "model path")
	input := flag.String("f", "", "input wav")
	flag.String("l", "en", "language")
	srt := flag.Bool("osrt", false, "write srt")
	out := flag.String("of", "", "output path without extension")
	flag.Bool("pp", false, "print progress")
	flag.Bool("sns", false, "suppress non-speech")
	flag.Int("mc", -1, "max context")
	flag.Int("t", 4, "threads")
	flag.Parse()

	if *model == "" || *input == "" || !*srt || *out == "" {
		fmt.Fprintln(os.Stderr, "fakewhisper: missing -m, -f, -osrt or -of")
		os.Exit(2)
	}

	mode := os.Getenv("FAKEWHISPER_MODE")
	switch mode {
	case "crash":
		fmt.Fprintln(os.Stderr, "whisper_init: failed to load model")
		os.Exit(3)
	case "hang":
		time.Sleep(time.Hour)
	}

	secs, err := wavSeconds(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakewhisper:", err)
		os.Exit(4)
	}
	for p := 25; p <= 100; p += 25 {
		fmt.Fprintf(os.Stderr, "whisper_print_progress_callback: progress = %3d%%\n", p)
	}

	f, err := os.Create(*out + ".srt")
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakewhisper:", err)
		os.Exit(5)
	}
	defer f.Close()
	if mode == "empty" {
		return
	}
	// One 10-second cue per 10 seconds of audio; "repeat" makes the first
	// five cues identical, the looping the real model was measured doing.
	n := 0
	for start := 0; start+10 <= secs || (n == 0 && secs > 0); start += 10 {
		n++
		text := fmt.Sprintf("line %d", n)
		if mode == "repeat" && n <= 5 {
			text = "I wonder what will happen to you?"
		}
		end := start + 10
		if end > secs {
			end = secs
		}
		fmt.Fprintf(f, "%d\n%s --> %s\n%s\n\n", n, stamp(start), stamp(end), text)
		if start+10 > secs {
			break
		}
	}
}

func stamp(s int) string {
	return fmt.Sprintf("%02d:%02d:%02d,000", s/3600, s/60%60, s%60)
}

// wavSeconds reads a canonical 44-byte-header PCM WAV and returns its
// whole-second duration.
func wavSeconds(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	hdr := make([]byte, 44)
	if _, err := f.Read(hdr); err != nil {
		return 0, err
	}
	byteRate := binary.LittleEndian.Uint32(hdr[28:32])
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if byteRate == 0 {
		return 0, fmt.Errorf("bad wav header")
	}
	return int((info.Size() - 44) / int64(byteRate)), nil
}
