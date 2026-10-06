package captions

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// Transcription runs over ~10-minute pieces so memory stays flat whatever
// the video's length and a restart loses at most one piece (research.md
// §4). Each cut moves to the quietest 100 ms within ±CutSearchWindow of the
// mark so it falls between words. Both are hypotheses to confirm against a
// real sermon (Constitution III).
const (
	PieceLength     = 10 * time.Minute
	CutSearchWindow = 5 * time.Second
	quietWindow     = 100 * time.Millisecond
)

// WAV is a PCM WAV file's format and where its samples are.
type WAV struct {
	Path          string
	SampleRate    int
	Channels      int
	BitsPerSample int
	dataOffset    int64
	dataSize      int64
}

// Piece is one span of a WAV to transcribe on its own.
type Piece struct {
	Start  time.Duration
	Length time.Duration
}

var errNotWAV = errors.New("captions: not a 16-bit PCM WAV file")

// OpenWAV reads path's chunk headers -- skipping any chunk that isn't
// "fmt " or "data", such as the FLLR padding afconvert writes -- and
// checks it is 16-bit PCM.
func OpenWAV(path string) (*WAV, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("captions: open audio: %w", err)
	}
	defer f.Close()

	hdr := make([]byte, 12)
	if _, err := io.ReadFull(f, hdr); err != nil || string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WAVE" {
		return nil, errNotWAV
	}
	w := &WAV{Path: path}
	var offset int64 = 12
	var sawFmt bool
	for {
		ch := make([]byte, 8)
		if _, err := io.ReadFull(f, ch); err != nil {
			return nil, errNotWAV
		}
		id, size := string(ch[0:4]), int64(binary.LittleEndian.Uint32(ch[4:8]))
		offset += 8
		switch id {
		case "fmt ":
			body := make([]byte, size)
			if _, err := io.ReadFull(f, body); err != nil || size < 16 {
				return nil, errNotWAV
			}
			if !isPCM(body) {
				return nil, errNotWAV
			}
			w.Channels = int(binary.LittleEndian.Uint16(body[2:4]))
			w.SampleRate = int(binary.LittleEndian.Uint32(body[4:8]))
			w.BitsPerSample = int(binary.LittleEndian.Uint16(body[14:16]))
			sawFmt = true
		case "data":
			if !sawFmt || w.BitsPerSample != 16 || w.Channels < 1 || w.SampleRate < 1 {
				return nil, errNotWAV
			}
			w.dataOffset, w.dataSize = offset, size
			return w, nil
		default:
			if _, err := f.Seek(size, io.SeekCurrent); err != nil {
				return nil, errNotWAV
			}
		}
		offset += size
		if size%2 == 1 { // chunks are word-aligned
			f.Seek(1, io.SeekCurrent)
			offset++
		}
	}
}

// isPCM reports whether a fmt chunk describes integer PCM: format 1, or
// WAVE_FORMAT_EXTENSIBLE (0xFFFE) whose sub-format GUID starts with 1 --
// which afconvert writes for some inputs (measured on a 44.1 kHz mono AAC
// video).
func isPCM(fmtBody []byte) bool {
	switch binary.LittleEndian.Uint16(fmtBody[0:2]) {
	case 1:
		return true
	case 0xFFFE:
		return len(fmtBody) >= 26 && binary.LittleEndian.Uint16(fmtBody[24:26]) == 1
	}
	return false
}

func (w *WAV) frameBytes() int64 { return int64(w.Channels) * 2 }

func (w *WAV) frames() int64 { return w.dataSize / w.frameBytes() }

// Duration is the audio's length.
func (w *WAV) Duration() time.Duration {
	return w.toDuration(w.frames())
}

func (w *WAV) toFrame(d time.Duration) int64 {
	return int64(d) * int64(w.SampleRate) / int64(time.Second)
}

func (w *WAV) toDuration(frame int64) time.Duration {
	return time.Duration(frame * int64(time.Second) / int64(w.SampleRate))
}

// SplitPoints returns where to cut w into pieces of about pieceLen, each
// cut moved to the quietest quietWindow within ±window of its mark. The
// result depends only on the file, so a restarted job recomputes the
// same pieces. Audio shorter than pieceLen+window isn't cut.
func SplitPoints(w *WAV, pieceLen, window time.Duration) ([]time.Duration, error) {
	f, err := os.Open(w.Path)
	if err != nil {
		return nil, fmt.Errorf("captions: open audio: %w", err)
	}
	defer f.Close()

	var cuts []time.Duration
	for mark := pieceLen; mark+window < w.Duration(); mark += pieceLen {
		cut, err := quietestPoint(f, w, mark-window, mark+window)
		if err != nil {
			return nil, err
		}
		cuts = append(cuts, cut)
	}
	return cuts, nil
}

// quietestPoint returns the start of the quietest quietWindow in [from, to).
func quietestPoint(f *os.File, w *WAV, from, to time.Duration) (time.Duration, error) {
	start, end := w.toFrame(from), w.toFrame(to)
	buf := make([]byte, (end-start)*w.frameBytes())
	if _, err := f.ReadAt(buf, w.dataOffset+start*w.frameBytes()); err != nil && err != io.EOF {
		return 0, fmt.Errorf("captions: read audio: %w", err)
	}
	// Loudness per frame (first channel), then a sliding sum of squares.
	n := int(end - start)
	win := int(w.toFrame(quietWindow))
	energy := func(i int) int64 {
		v := int64(int16(binary.LittleEndian.Uint16(buf[int64(i)*w.frameBytes():])))
		return v * v
	}
	var sum int64
	for i := 0; i < win && i < n; i++ {
		sum += energy(i)
	}
	best, bestAt := sum, 0
	for i := win; i < n; i++ {
		sum += energy(i) - energy(i-win)
		if sum < best {
			best, bestAt = sum, i-win+1
		}
	}
	return w.toDuration(start + int64(bestAt)), nil
}

// Pieces turns cut points into consecutive pieces covering all of w.
func Pieces(w *WAV, cuts []time.Duration) []Piece {
	var pieces []Piece
	var at int64
	for _, c := range append(cuts, w.Duration()) {
		end := w.toFrame(c)
		if c == w.Duration() {
			end = w.frames()
		}
		pieces = append(pieces, Piece{Start: w.toDuration(at), Length: w.toDuration(end - at)})
		at = end
	}
	return pieces
}

// WritePiece writes piece p of w to out as a canonical WAV, copying with a
// fixed buffer so memory doesn't depend on the piece's size.
func WritePiece(w *WAV, p Piece, out string) error {
	src, err := os.Open(w.Path)
	if err != nil {
		return fmt.Errorf("captions: open audio: %w", err)
	}
	defer src.Close()

	frames := w.toFrame(p.Length)
	size := frames * w.frameBytes()
	dst, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("captions: create piece: %w", err)
	}
	defer dst.Close()

	le := binary.LittleEndian
	hdr := make([]byte, 0, 44)
	hdr = append(hdr, "RIFF"...)
	hdr = le.AppendUint32(hdr, uint32(36+size))
	hdr = append(hdr, "WAVEfmt "...)
	hdr = le.AppendUint32(hdr, 16)
	hdr = le.AppendUint16(hdr, 1)
	hdr = le.AppendUint16(hdr, uint16(w.Channels))
	hdr = le.AppendUint32(hdr, uint32(w.SampleRate))
	hdr = le.AppendUint32(hdr, uint32(int64(w.SampleRate)*w.frameBytes()))
	hdr = le.AppendUint16(hdr, uint16(w.frameBytes()))
	hdr = le.AppendUint16(hdr, 16)
	hdr = append(hdr, "data"...)
	hdr = le.AppendUint32(hdr, uint32(size))
	if _, err := dst.Write(hdr); err != nil {
		return fmt.Errorf("captions: write piece: %w", err)
	}
	section := io.NewSectionReader(src, w.dataOffset+w.toFrame(p.Start)*w.frameBytes(), size)
	if _, err := io.CopyBuffer(dst, section, make([]byte, 1<<20)); err != nil {
		return fmt.Errorf("captions: write piece: %w", err)
	}
	return nil
}
