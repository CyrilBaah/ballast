package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

// resumeIncomplete is Drive's resumable-upload status code for "chunk
// accepted, more bytes expected" -- numerically the same as
// http.StatusPermanentRedirect, but Drive's use of it here is
// protocol-specific, not an HTTP redirect to follow.
const resumeIncomplete = 308

// ProdAPIBase is Drive's real upload host, used unless overridden (the
// E2E-mocked dev build points this at an in-process fake server instead --
// mock_e2e.go).
const ProdAPIBase = "https://www.googleapis.com"

// SessionResult is what a chunk send or offset query reports back.
type SessionResult struct {
	// Done is true once Drive has acknowledged the final byte and returned
	// the created file's metadata.
	Done bool
	// Offset is the number of bytes Drive has acknowledged so far. Only
	// meaningful when !Done.
	Offset      int64
	FileID      string
	WebViewLink string
}

// DriveError is a classified Drive API error response, carrying enough of
// the JSON error body for retry.go's classification logic to distinguish,
// e.g., an expired session from a missing destination folder.
type DriveError struct {
	StatusCode int
	Reason     string // errors[0].reason, e.g. "storageQuotaExceeded", "notFound"
	Location   string // errors[0].location, e.g. "parents"
	Message    string
}

func (e *DriveError) Error() string {
	return fmt.Sprintf("drive: %d %s: %s", e.StatusCode, e.Reason, e.Message)
}

type driveErrorBody struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Errors  []struct {
			Reason   string `json:"reason"`
			Location string `json:"location"`
			Message  string `json:"message"`
		} `json:"errors"`
	} `json:"error"`
}

func parseDriveError(resp *http.Response) *DriveError {
	de := &DriveError{StatusCode: resp.StatusCode}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var parsed driveErrorBody
	if json.Unmarshal(body, &parsed) == nil && len(parsed.Error.Errors) > 0 {
		de.Reason = parsed.Error.Errors[0].Reason
		de.Location = parsed.Error.Errors[0].Location
		de.Message = parsed.Error.Errors[0].Message
	} else {
		de.Message = strings.TrimSpace(string(body))
	}
	return de
}

// InitiateSession starts a new Drive resumable-upload session for a file
// named fileName, totalBytes long, inside driveFolderID, returning the
// session URI from the response's Location header (research.md §1). The
// fields= parameter set here is what Drive honors for the *finalize*
// response too (the completed-upload body SendChunk/QueryOffset parse) --
// without it, Drive's default partial response omits webViewLink, which
// storage.SetUploadSucceeded requires as non-empty.
//
// The file is tagged with uploadID as a private app property, so that
// FindLandedUpload can later tell whether a session Ballast lost track of
// actually finished on Drive's side before anything restarts it from zero.
func InitiateSession(ctx context.Context, client *http.Client, apiBase string, uploadID int64, fileName, driveFolderID string, totalBytes int64) (string, *DriveError, error) {
	if apiBase == "" {
		apiBase = ProdAPIBase
	}
	meta := map[string]any{
		"name":          filepath.Base(fileName),
		"parents":       []string{driveFolderID},
		"appProperties": map[string]string{UploadIDAppProperty: strconv.FormatInt(uploadID, 10)},
	}
	body, err := json.Marshal(meta)
	if err != nil {
		return "", nil, fmt.Errorf("drive: encode session-initiate metadata: %w", err)
	}

	url := strings.TrimRight(apiBase, "/") + "/upload/drive/v3/files?uploadType=resumable&fields=id,webViewLink"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", nil, fmt.Errorf("drive: build session-initiate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(totalBytes, 10))

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", parseDriveError(resp), nil
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", nil, fmt.Errorf("drive: session-initiate response missing Location header")
	}
	return loc, nil, nil
}

// UploadIDAppProperty is the private Drive app property every uploaded
// file carries, holding the local Upload row's ID (see InitiateSession).
const UploadIDAppProperty = "ballastUploadId"

// SendChunk PUTs the bytes [start, start+len(chunk)) of a totalBytes-long
// file to an already-initiated session (research.md §1). Chunks MUST be
// sent strictly in order, one at a time (Constitution Principle II) --
// this function sends exactly one chunk and returns.
func SendChunk(ctx context.Context, client *http.Client, sessionURI string, chunk []byte, start, totalBytes int64) (*SessionResult, *DriveError, error) {
	end := start + int64(len(chunk)) - 1
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, sessionURI, bytes.NewReader(chunk))
	if err != nil {
		return nil, nil, fmt.Errorf("drive: build chunk request: %w", err)
	}
	req.ContentLength = int64(len(chunk))
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalBytes))

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	return parseSessionResponse(resp)
}

// QueryOffset asks Drive how many bytes of totalBytes it has acknowledged
// so far for an already-initiated session, without sending any new data
// (research.md §1) -- used to re-synchronize when a resumed upload isn't
// certain its last-persisted bytes_sent value is still accurate.
func QueryOffset(ctx context.Context, client *http.Client, sessionURI string, totalBytes int64) (*SessionResult, *DriveError, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, sessionURI, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("drive: build offset-query request: %w", err)
	}
	req.ContentLength = 0
	req.Header.Set("Content-Range", fmt.Sprintf("bytes */%d", totalBytes))

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	return parseSessionResponse(resp)
}

func parseSessionResponse(resp *http.Response) (*SessionResult, *DriveError, error) {
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		var file struct {
			ID          string `json:"id"`
			WebViewLink string `json:"webViewLink"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
			return nil, nil, fmt.Errorf("drive: decode completed-upload response: %w", err)
		}
		return &SessionResult{Done: true, FileID: file.ID, WebViewLink: file.WebViewLink}, nil, nil
	case resumeIncomplete:
		offset := int64(0)
		if rng := resp.Header.Get("Range"); rng != "" {
			// Format: "bytes=0-N" -- offset is N+1 (bytes acknowledged so far).
			if _, after, ok := strings.Cut(rng, "-"); ok {
				if n, err := strconv.ParseInt(after, 10, 64); err == nil {
					offset = n + 1
				}
			}
		}
		return &SessionResult{Done: false, Offset: offset}, nil, nil
	default:
		return nil, parseDriveError(resp), nil
	}
}

// ReleaseSession makes a best-effort attempt to tell Drive a resumable
// session is no longer needed (research.md §8) -- its result is always
// ignored by callers, since local cancellation must succeed regardless of
// whether this call does.
func ReleaseSession(ctx context.Context, client *http.Client, sessionURI string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, sessionURI, nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// FetchFileWebViewLink looks up a file's webViewLink by id -- a fallback
// for when a resumable session's completion response didn't carry it,
// which happens for any session initiated before InitiateSession started
// requesting fields=id,webViewLink (that preference is set once at session
// creation and can't be applied retroactively to an already-persisted
// session URI).
func FetchFileWebViewLink(ctx context.Context, client *http.Client, apiBase, fileID string) (string, error) {
	if apiBase == "" {
		apiBase = ProdAPIBase
	}
	reqURL := strings.TrimRight(apiBase, "/") + "/drive/v3/files/" + url.PathEscape(fileID) + "?fields=webViewLink"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("drive: build webViewLink lookup request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", parseDriveError(resp)
	}
	var file struct {
		WebViewLink string `json:"webViewLink"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		return "", fmt.Errorf("drive: decode webViewLink lookup response: %w", err)
	}
	return file.WebViewLink, nil
}

// FindLandedUpload looks for a file in driveFolderID that an earlier
// session of upload uploadID already finished -- tagged with that ID
// (InitiateSession), named fileName, and exactly totalBytes long. It
// returns nil, nil when there is none. A session can complete on Drive's
// side while Ballast never hears back (the final chunk's response is lost,
// or a status query against the finished session comes back 404), and
// restarting from zero at that point uploads a second copy of a file that
// is already there.
func FindLandedUpload(ctx context.Context, client *http.Client, apiBase string, uploadID int64, fileName, driveFolderID string, totalBytes int64) (*UploadResult, error) {
	if apiBase == "" {
		apiBase = ProdAPIBase
	}
	q := fmt.Sprintf("appProperties has { key='%s' and value='%d' } and '%s' in parents and trashed = false",
		UploadIDAppProperty, uploadID, escapeQueryValue(driveFolderID))
	params := url.Values{}
	params.Set("q", q)
	params.Set("fields", "files(id,name,size,webViewLink)")
	params.Set("spaces", "drive")
	reqURL := strings.TrimRight(apiBase, "/") + "/drive/v3/files?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("drive: build landed-upload lookup request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, parseDriveError(resp)
	}
	var list struct {
		Files []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Size        string `json:"size"`
			WebViewLink string `json:"webViewLink"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("drive: decode landed-upload lookup response: %w", err)
	}
	want := strconv.FormatInt(totalBytes, 10)
	for _, f := range list.Files {
		if f.Name == filepath.Base(fileName) && f.Size == want && f.ID != "" {
			return &UploadResult{FileID: f.ID, WebViewLink: f.WebViewLink}, nil
		}
	}
	return nil, nil
}

// escapeQueryValue escapes a value for use inside a single-quoted Drive
// search-query string literal.
func escapeQueryValue(v string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v)
}
