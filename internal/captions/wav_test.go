package captions

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTestWAV writes a 16 kHz mono 16-bit WAV of the given length that is
// loud everywhere except the given silent spans, with an afconvert-style
// FLLR padding chunk before the data (afconvert really writes one).
func writeTestWAV(t *testing.T, length time.Duration, silences ...[2]time.Duration) string {
	t.Helper()
	const rate = 16000
	n := int(length.Seconds() * rate)
	samples := make([]byte, n*2)
	for i := 0; i < n; i++ {
		v := int16(8000)
		if i%2 == 1 {
			v = -8000
		}
		at := time.Duration(i) * time.Second / rate
		for _, s := range silences {
			if at >= s[0] && at < s[1] {
				v = 0
			}
		}
		binary.LittleEndian.PutUint16(samples[i*2:], uint16(v))
	}

	var b []byte
	le := binary.LittleEndian
	b = append(b, "RIFF"...)
	b = le.AppendUint32(b, 0) // patched below
	b = append(b, "WAVE"...)
	b = append(b, "fmt "...)
	b = le.AppendUint32(b, 16)
	b = le.AppendUint16(b, 1)
	b = le.AppendUint16(b, 1)
	b = le.AppendUint32(b, rate)
	b = le.AppendUint32(b, rate*2)
	b = le.AppendUint16(b, 2)
	b = le.AppendUint16(b, 16)
	b = append(b, "FLLR"...)
	b = le.AppendUint32(b, 12)
	b = append(b, make([]byte, 12)...)
	b = append(b, "data"...)
	b = le.AppendUint32(b, uint32(len(samples)))
	b = append(b, samples...)
	le.PutUint32(b[4:], uint32(len(b)-8))

	path := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenWAVSkipsPaddingChunks(t *testing.T) {
	path := writeTestWAV(t, 3*time.Second)
	w, err := OpenWAV(path)
	if err != nil {
		t.Fatalf("OpenWAV: %v", err)
	}
	if w.SampleRate != 16000 || w.Channels != 1 || w.BitsPerSample != 16 {
		t.Fatalf("format = %d Hz, %d ch, %d bit", w.SampleRate, w.Channels, w.BitsPerSample)
	}
	if w.Duration() != 3*time.Second {
		t.Fatalf("Duration() = %v, want 3s", w.Duration())
	}
}

func TestOpenWAVRejectsNonWAV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.wav")
	os.WriteFile(path, []byte("not a wav file at all, definitely not"), 0o644)
	if _, err := OpenWAV(path); err == nil {
		t.Fatal("OpenWAV accepted a non-WAV file")
	}
}

func TestSplitPointsLandInSilence(t *testing.T) {
	// 150 s of audio, 60 s pieces, ±5 s search: the cuts should land in
	// the short silences placed near (not at) the 60 s and 120 s marks.
	sil1 := [2]time.Duration{62 * time.Second, 62*time.Second + 400*time.Millisecond}
	sil2 := [2]time.Duration{117 * time.Second, 117*time.Second + 400*time.Millisecond}
	w, err := OpenWAV(writeTestWAV(t, 150*time.Second, sil1, sil2))
	if err != nil {
		t.Fatal(err)
	}
	cuts, err := SplitPoints(w, 60*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("SplitPoints: %v", err)
	}
	if len(cuts) != 2 {
		t.Fatalf("cuts = %v, want 2", cuts)
	}
	for i, s := range [][2]time.Duration{sil1, sil2} {
		if cuts[i] < s[0] || cuts[i] >= s[1] {
			t.Errorf("cut %d at %v, want inside the silence %v–%v", i, cuts[i], s[0], s[1])
		}
	}
}

func TestSplitPointsShortAudioHasNoCuts(t *testing.T) {
	w, _ := OpenWAV(writeTestWAV(t, 30*time.Second))
	cuts, err := SplitPoints(w, 60*time.Second, 5*time.Second)
	if err != nil || len(cuts) != 0 {
		t.Fatalf("SplitPoints on 30 s = %v, %v; want no cuts", cuts, err)
	}
}

func TestWritePiecesCoverTheWholeFileExactly(t *testing.T) {
	sil := [2]time.Duration{61 * time.Second, 61*time.Second + 300*time.Millisecond}
	w, _ := OpenWAV(writeTestWAV(t, 130*time.Second, sil))
	cuts, _ := SplitPoints(w, 60*time.Second, 5*time.Second)
	pieces := Pieces(w, cuts)

	var total time.Duration
	for i, p := range pieces {
		if i > 0 && p.Start != pieces[i-1].Start+pieces[i-1].Length {
			t.Errorf("piece %d starts at %v, previous ended at %v", i, p.Start, pieces[i-1].Start+pieces[i-1].Length)
		}
		out := filepath.Join(t.TempDir(), "piece.wav")
		if err := WritePiece(w, p, out); err != nil {
			t.Fatalf("WritePiece %d: %v", i, err)
		}
		pw, err := OpenWAV(out)
		if err != nil {
			t.Fatalf("piece %d is not a valid WAV: %v", i, err)
		}
		if pw.Duration() != p.Length {
			t.Errorf("piece %d lasts %v, want %v", i, pw.Duration(), p.Length)
		}
		total += p.Length
	}
	if pieces[0].Start != 0 || total != w.Duration() {
		t.Fatalf("pieces cover %v from %v, want all %v from 0", total, pieces[0].Start, w.Duration())
	}
}

func TestDefaultPieceSettings(t *testing.T) {
	if PieceLength != 10*time.Minute || CutSearchWindow != 5*time.Second {
		t.Fatalf("PieceLength = %v, CutSearchWindow = %v; research.md §4 says 10 min and ±5 s", PieceLength, CutSearchWindow)
	}
}

func TestOpenWAVAcceptsExtensibleFormat(t *testing.T) {
	// The header afconvert really wrote for Aksum.mp4 (44.1 kHz mono AAC in):
	// WAVE_FORMAT_EXTENSIBLE with the PCM sub-format GUID.
	le := binary.LittleEndian
	var b []byte
	b = append(b, "RIFF"...)
	b = le.AppendUint32(b, 0)
	b = append(b, "WAVEfmt "...)
	b = le.AppendUint32(b, 40)
	b = le.AppendUint16(b, 0xFFFE)
	b = le.AppendUint16(b, 1)
	b = le.AppendUint32(b, 16000)
	b = le.AppendUint32(b, 32000)
	b = le.AppendUint16(b, 2)
	b = le.AppendUint16(b, 16)
	b = le.AppendUint16(b, 22) // cbSize
	b = le.AppendUint16(b, 16) // valid bits
	b = le.AppendUint32(b, 4)  // channel mask
	b = append(b, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71)
	b = append(b, "data"...)
	b = le.AppendUint32(b, 32000)
	b = append(b, make([]byte, 32000)...)
	path := filepath.Join(t.TempDir(), "ext.wav")
	os.WriteFile(path, b, 0o644)

	w, err := OpenWAV(path)
	if err != nil {
		t.Fatalf("OpenWAV(extensible PCM): %v", err)
	}
	if w.Duration() != time.Second {
		t.Fatalf("Duration() = %v, want 1s", w.Duration())
	}
}
