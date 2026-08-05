package drive

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
)

// fakeResumableServer simulates Google Drive's resumable-upload protocol
// for Go unit tests (research.md §6), reusing the hijack+close technique
// mock_e2e.go already uses at the Playwright/E2E level to simulate a
// dropped connection mid-chunk. It supports exactly one session at a time,
// which matches this feature's one-upload-at-a-time constraint (FR-013).
type fakeResumableServer struct {
	*httptest.Server

	mu           sync.Mutex
	session      *fakeSession
	outcome      string
	parentExists bool
	// wireBytes counts every byte sent in a chunk PUT body, regardless of
	// whether it was ultimately acknowledged -- lets a test assert "zero
	// re-transmission" by checking this never exceeds the unsent remainder.
	wireBytes int64
	// chunkSizes records the byte length of every chunk PUT this server
	// accepted (in send order), letting a test assert the exact
	// growth/shrink sequence chunk-size adaptation produced (research.md §6).
	chunkSizes []int64
	// failOnAttempt, if non-nil, names 0-indexed chunk-PUT attempt numbers
	// (counting every attempt, including ones that fail -- not just
	// accepted ones) to fail with a simulated dropped connection,
	// independent of and evaluated before the sticky outcome switch. This
	// gives chunk-size adaptation tests an exact, race-free way to fail a
	// specific attempt, unlike toggling setOutcome from a separate
	// goroutine racing against however fast retries happen to run.
	failOnAttempt map[int]bool
	chunkAttempts int
}

// failAttempts arranges for the given 0-indexed chunk-PUT attempt numbers
// to each fail once with a simulated dropped connection, deterministically
// by attempt count rather than by timing.
func (s *fakeResumableServer) failAttempts(indices ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := make(map[int]bool, len(indices))
	for _, i := range indices {
		m[i] = true
	}
	s.failOnAttempt = m
}

type fakeSession struct {
	total    int64
	received int64
	content  []byte
}

func newFakeResumableServer() *fakeResumableServer {
	s := &fakeResumableServer{parentExists: true}
	mux := http.NewServeMux()
	mux.HandleFunc("/upload/drive/v3/files", s.handleInitiate)
	mux.HandleFunc("/upload/drive/v3/files/session", s.handleSession)
	s.Server = httptest.NewServer(mux)
	return s
}

// setOutcome controls what the next chunk PUT / offset query does.
// "" (or "approve") behaves as a normal successful accept.
func (s *fakeResumableServer) setOutcome(o string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcome = o
}

// setParentExists controls whether session initiation succeeds or reports
// a missing-destination-folder 404 (research.md §4).
func (s *fakeResumableServer) setParentExists(exists bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.parentExists = exists
}

// bytesReceived returns how many bytes of the current session Drive has
// acknowledged so far.
func (s *fakeResumableServer) bytesReceived() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return 0
	}
	return s.session.received
}

// receivedContent returns a copy of the bytes acknowledged so far, for
// byte-identical comparisons against the source file.
func (s *fakeResumableServer) receivedContent() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return nil
	}
	out := make([]byte, len(s.session.content))
	copy(out, s.session.content)
	return out
}

func (s *fakeResumableServer) totalWireBytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.wireBytes
}

// acceptedChunkSizes returns the byte length of every chunk PUT this
// server has accepted so far, in send order.
func (s *fakeResumableServer) acceptedChunkSizes() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int64, len(s.chunkSizes))
	copy(out, s.chunkSizes)
	return out
}

func writeDriveError(w http.ResponseWriter, status int, reason, location, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    status,
			"message": message,
			"errors": []map[string]string{
				{"reason": reason, "location": location, "message": message},
			},
		},
	})
}

func (s *fakeResumableServer) handleInitiate(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	parentExists := s.parentExists
	s.mu.Unlock()

	if !parentExists {
		writeDriveError(w, http.StatusNotFound, "notFound", "parents", "the specified parent was not found")
		return
	}

	totalStr := r.Header.Get("X-Upload-Content-Length")
	total, _ := strconv.ParseInt(totalStr, 10, 64)

	s.mu.Lock()
	s.session = &fakeSession{total: total}
	s.mu.Unlock()

	w.Header().Set("Location", s.Server.URL+"/upload/drive/v3/files/session")
	w.WriteHeader(http.StatusOK)
}

func (s *fakeResumableServer) handleSession(w http.ResponseWriter, r *http.Request) {
	contentRange := r.Header.Get("Content-Range")
	isOffsetQuery := strings.HasPrefix(contentRange, "bytes */")

	if !isOffsetQuery {
		s.mu.Lock()
		idx := s.chunkAttempts
		s.chunkAttempts++
		shouldFail := s.failOnAttempt[idx]
		s.mu.Unlock()

		if shouldFail {
			hj, ok := w.(http.Hijacker)
			if !ok {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			conn, _, err := hj.Hijack()
			if err == nil {
				conn.Close()
			}
			return
		}
	}

	s.mu.Lock()
	outcome := s.outcome
	s.mu.Unlock()

	switch outcome {
	case "network-fail":
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		conn, _, err := hj.Hijack()
		if err == nil {
			conn.Close()
		}
		return
	case "429":
		writeDriveError(w, http.StatusTooManyRequests, "rateLimitExceeded", "", "rate limit exceeded")
		return
	case "500":
		writeDriveError(w, http.StatusInternalServerError, "backendError", "", "internal error")
		return
	case "403-quota":
		writeDriveError(w, http.StatusForbidden, "storageQuotaExceeded", "", "storage quota exceeded")
		return
	case "404-session":
		writeDriveError(w, http.StatusNotFound, "notFound", "", "session not found")
		return
	case "410-session":
		writeDriveError(w, http.StatusGone, "gone", "", "session gone")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session == nil {
		writeDriveError(w, http.StatusNotFound, "notFound", "", "no such session")
		return
	}

	if isOffsetQuery {
		s.respondCurrentOffsetLocked(w)
		return
	}

	// Chunk send: parse "bytes start-end/total".
	spec := strings.TrimPrefix(contentRange, "bytes ")
	rangePart, totalPart, _ := strings.Cut(spec, "/")
	startPart, endPart, _ := strings.Cut(rangePart, "-")
	start, _ := strconv.ParseInt(startPart, 10, 64)
	end, _ := strconv.ParseInt(endPart, 10, 64)
	total, _ := strconv.ParseInt(totalPart, 10, 64)

	body := make([]byte, r.ContentLength)
	n, _ := readFull(r, body)
	body = body[:n]
	s.wireBytes += int64(n)

	if start != s.session.received {
		// Out-of-order/duplicate chunk -- report the true current offset
		// rather than accepting it, mirroring Drive's real behavior.
		s.respondCurrentOffsetLocked(w)
		return
	}

	s.session.content = append(s.session.content, body...)
	s.session.received = end + 1
	s.session.total = total
	s.chunkSizes = append(s.chunkSizes, int64(n))

	if s.session.received >= total {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "fake-file-id",
			"webViewLink": "https://drive.google.com/file/d/fake-file-id/view",
		})
		return
	}

	w.Header().Set("Range", "bytes=0-"+strconv.FormatInt(s.session.received-1, 10))
	w.WriteHeader(resumeIncomplete)
}

// respondCurrentOffsetLocked must be called with s.mu held.
func (s *fakeResumableServer) respondCurrentOffsetLocked(w http.ResponseWriter) {
	if s.session.received >= s.session.total && s.session.total > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "fake-file-id",
			"webViewLink": "https://drive.google.com/file/d/fake-file-id/view",
		})
		return
	}
	if s.session.received > 0 {
		w.Header().Set("Range", "bytes=0-"+strconv.FormatInt(s.session.received-1, 10))
	}
	w.WriteHeader(resumeIncomplete)
}

func readFull(r *http.Request, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Body.Read(buf[total:])
		total += n
		if err != nil {
			if total == len(buf) {
				return total, nil
			}
			return total, err
		}
	}
	return total, nil
}
