// GO/internal/driveupload/upload.go

// Package driveupload uploads a processed order's source file to Google
// Drive via the same Google Apps Script endpoints the old Python app
// used (xulydonhang.py's ProcessHandler.upload_file_to_drive) — fire-
// and-forget: the file is read and base64-encoded synchronously (the
// caller's source file may be deleted right after this call returns),
// but the actual network POST + retry runs in a background goroutine
// so a slow/failing upload never blocks order processing. Returns a
// constructed "view" URL immediately, before the upload is even
// attempted.
package driveupload

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// scriptURL/viewURLBase are the SAME Google Apps Script endpoints the
// old Python app used (xulydonhang.py:3093-3094) — reused deliberately
// so uploads keep landing in the same shared Drive location, per the
// project owner's explicit choice (not a placeholder to fill in).
//
// Deliberately `var`, not `const`: Upload calls scriptURL directly
// (not as a parameter), so a test needs to temporarily repoint it at
// an httptest.Server to avoid ever making a real network call during
// `go test`. A const would make that impossible without changing
// Upload's signature just for tests. Tests that override this MUST
// restore the original value (e.g. via t.Cleanup) and must NOT run in
// parallel with each other while doing so, since it's shared mutable
// package state.
var scriptURL = "https://script.google.com/macros/s/AKfycbx2ZJhdxEAZq_79ibt3g5UeqccNqLT2ScOtRldnwlgRQB2JdquUPSnSebMQoYESNSv2/exec"
const viewURLBase = "https://script.google.com/macros/s/AKfycby9fc3IaX1-EwIb26g34WLs8TbQXNkdxeqpVSYSWddwwxRAFaz9kjsS9yFhypezIaF2/exec?po="

// Metadata is embedded into the uploaded file's name on Drive, in this
// exact order — mirrors upload_file_to_drive's name_parts exactly.
type Metadata struct {
	Vendor       string
	EntryDate    string // any of dateInputLayouts' formats, or "" / a not-found sentinel
	CustomerCode string
	CancelDate   string // same format rules as EntryDate
	OutputName   string // typically the PO/order number
}

var sanitizePattern = regexp.MustCompile(`[\\/:*?"<>|\[\]]+`)

// sanitize mirrors Python's _sanitize: strip characters that would
// break a filename or the bracket-delimited convention itself, empty
// (or now-empty-after-stripping) becomes "NA" so every bracket always
// has content and field position never shifts.
func sanitize(value string) string {
	if value == "" {
		return "NA"
	}
	cleaned := strings.TrimSpace(sanitizePattern.ReplaceAllString(value, ""))
	if cleaned == "" {
		return "NA"
	}
	return cleaned
}

// dateInputLayouts mirrors Python's _format_date try-list
// (xulydonhang.py:3118), plus one layout Python never needed: Go date
// fields already arrive as strings (never a native date/time value the
// way Python's could), so there is no separate "isinstance(datetime)"
// branch to port.
var dateInputLayouts = []string{
	"02/01/2006",
	"02-01-2006",
	"02/01/06",
	"02-01-06",
	"2006-01-02",
	"2006/01/02",
	// JMart's own entry/cancel date regex allows 1-2 digit day/month
	// with no zero-padding (internal/processing/jmart/extract.go:8,
	// `\d{1,2}/\d{1,2}/\d{4}`) — verified directly against that file
	// before writing this plan, not assumed. "02/01/2006" alone would
	// fail to parse a real single-digit day or month (e.g. "5/3/2026");
	// Go's non-padded day/month layout tokens ("2"/"1") cover both 1-
	// and 2-digit input, so this one extra layout is enough — no
	// vendor-specific function needed (see Task 11, JMart).
	"2/1/2006",
}

// formatDate mirrors Python's _format_date: try each layout in turn,
// "NA" if none match (covers empty strings and any vendor's
// not-found sentinel, e.g. Coop's "Không tìm thấy"/"Không hợp lệ" -
// neither matches any real date layout, so both correctly fall
// through to "NA", matching Python's behavior for the same inputs).
func formatDate(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "NA"
	}
	for _, layout := range dateInputLayouts {
		if t, err := time.Parse(layout, trimmed); err == nil {
			return t.Format("02-01-2006")
		}
	}
	return "NA"
}

// BuildFilename mirrors upload_file_to_drive's name_parts/filename
// construction exactly (xulydonhang.py:3125-3132).
func BuildFilename(m Metadata) string {
	parts := []string{
		sanitize(m.Vendor),
		formatDate(m.EntryDate),
		sanitize(m.CustomerCode),
		formatDate(m.CancelDate),
		sanitize(m.OutputName),
	}
	var b strings.Builder
	for _, p := range parts {
		b.WriteString("[")
		b.WriteString(p)
		b.WriteString("]")
	}
	return b.String()
}

type uploadPayload struct {
	Filename string `json:"filename"`
	Ext      string `json:"ext"`
	Mime     string `json:"mime"`
	FileB64  string `json:"file_b64"`
}

// Upload reads path synchronously, builds the Drive filename and a
// constructed view URL, starts the real network upload (with retry) in
// a background goroutine, and returns the view URL immediately -
// mirroring upload_file_to_drive's fire-and-forget contract exactly.
// onResult (may be nil) is called exactly once when the background
// goroutine finishes, ok=true on the first successful POST, ok=false
// with the last error after all retries are exhausted - this is the
// ONE deliberate behavior difference from the Python original (which
// only printed to a hidden console): the caller can route this to a
// real, visible log line.
func Upload(client *http.Client, path string, m Metadata, onResult func(ok bool, err error)) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("driveupload: read %s: %w", path, err)
	}

	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	filename := BuildFilename(m)
	viewURL := viewURLBase + url.QueryEscape(filename)

	payload := uploadPayload{
		Filename: filename,
		Ext:      ext,
		Mime:     mimeType,
		FileB64:  base64.StdEncoding.EncodeToString(data),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("driveupload: encode payload: %w", err)
	}

	// Snapshot scriptURL synchronously, here in Upload's synchronous
	// portion, rather than letting the background goroutine (or its
	// retries) read the package var fresh each time. scriptURL is
	// mutable shared state that tests repoint at an httptest.Server for
	// the duration of a single test; Upload's own goroutine can outlive
	// that test (this function is fire-and-forget by design, and at
	// least one caller never waits for the background upload to
	// finish). Without this snapshot, a late retry could read a
	// *different* test's scriptURL override, silently sending its
	// request to the wrong server.
	targetURL := scriptURL

	go func() {
		const maxRetries = 3
		var lastErr error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			if err := postOnce(client, targetURL, body); err != nil {
				lastErr = err
				if attempt < maxRetries {
					time.Sleep(time.Duration(2*attempt) * time.Second)
				}
				continue
			}
			if onResult != nil {
				onResult(true, nil)
			}
			return
		}
		if onResult != nil {
			onResult(false, lastErr)
		}
	}()

	return viewURL, nil
}

func postOnce(client *http.Client, targetURL string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// NewHTTPClient matches this project's other HTTP clients
// (pricing.NewHTTPSource, productdata.NewHTTPClient, applock.Check) -
// a bare-timeout client, no cookies/retries at the transport level
// (retry is handled explicitly in Upload's goroutine, since a 30s
// per-attempt timeout needs to coexist with 3 attempts + backoff, not
// a single client-wide timeout covering all of them).
func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}
