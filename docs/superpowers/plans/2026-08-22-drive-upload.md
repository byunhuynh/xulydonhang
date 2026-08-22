# Port Upload Đơn Hàng lên Google Drive (Python → Go) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `xulydonhang.py`'s real, production `upload_file_to_drive` feature (fire-and-forget upload of every processed order's source file to Google Drive, via 2 existing Apps Script endpoints) to the Go/Wails rewrite — currently completely missing there.

**Architecture:** A new, dependency-free `internal/driveupload` package owns the HTTP/retry/filename-convention mechanics (mirrors the Python function's contract exactly: read+encode synchronously, POST+retry in a background goroutine, return a constructed view URL immediately). `RealProcessor` gains 2 fields (`DriveClient`, `LogFunc`) wired once in `app.go`. Each of the 9 vendor processors calls `driveupload.Upload` right after its own successful Excel write, storing the result in a new `OrderRow.DriveURL` field that flows straight to the frontend's existing results table as a new clickable column — no new Wails events needed, since fire-and-forget means the URL is ready the instant the row itself is built.

**Tech Stack:** Go (stdlib `net/http`, `encoding/json`, `encoding/base64`, `mime`), React/TypeScript (existing `ResultTable.tsx`, Wails `BrowserOpenURL` runtime).

**Spec:** `docs/superpowers/specs/2026-08-22-drive-upload-design.md`

## Global Constraints

- Upload the ORIGINAL source file (`filePath`) whole — never cut/split PDF pages, for ANY vendor, even when one source file bundles multiple orders/pages. Multiple orders from the same source file share the same `DriveURL`.
- A synchronous file-read failure inside `driveupload.Upload` (`uploadErr`) must NEVER fail the enclosing `OrderRow`/order — the order's Excel row has already been written successfully by that point; only log a warning via `LogFunc` and leave `DriveURL` empty. Never `return OrderRow{}, uploadErr`.
- Reuse the exact 2 Google Apps Script endpoints already in the spec (`scriptURL`, `viewURLBase`) — never point at a different endpoint.
- Every vendor task must use the ACTUAL local variable names already verified per-vendor in the spec's table (section "6. Áp dụng cho 8 vendor còn lại") — never invent or guess a variable name.
- `dateInputLayouts` (in `driveupload/upload.go`) is the ONE place date-format handling lives — if a vendor's date format needs a new layout, add it there, never write a vendor-specific date-parsing function inside a `*_processor.go` file.
- No test may ever call the real `scriptURL`/Drive production endpoint. Every HTTP-calling test must override the package-level `scriptURL` var to point at an `httptest.Server`, and restore the original value via `t.Cleanup`. Tests that do this must not run with `t.Parallel()`.
- After every task that touches Go code: `go build ./...`, `go vet ./...`, `go test ./...` must be clean. Known, unrelated, pre-existing baseline: exactly 2 Coop golden-fixture failures (`103229379-00.pdf`, `103346096-00.pdf` — a corrupt source PDF and a font/CID-decode issue) already failing before this plan and unrelated to it. Any OTHER failure is a real regression from this plan's own work.
- Frontend verification for Task 3 must use a REAL running `wails dev` session when possible, not just `tsc --noEmit` — per this project's own established convention for UI changes. **Known sandbox limitation from a prior session**: the Go child process spawned by `wails dev` was previously observed unable to reach the internet (Google Sheets fetch timed out) in this exact sandboxed tool environment, even though `curl` via the Bash tool worked fine for the identical URL. If this recurs, the task's report must say so explicitly ("could not verify live — sandbox network restriction, see task report") rather than claiming full verification that didn't actually happen. Fall back to isolated component testing (temporarily swap `main.tsx`'s render to mount just the changed component, screenshot via Playwright, then revert — the exact technique already used successfully for `LockOverlay` earlier this session) if the full `wails dev` path is blocked.

---

### Task 1: Package `GO/internal/driveupload/`

**Files:**
- Create: `GO/internal/driveupload/upload.go`
- Create: `GO/internal/driveupload/upload_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks (first task).
- Produces: `driveupload.Metadata{Vendor, EntryDate, CustomerCode, CancelDate, OutputName string}`, `driveupload.BuildFilename(m Metadata) string`, `driveupload.Upload(client *http.Client, path string, m Metadata, onResult func(ok bool, err error)) (string, error)`, `driveupload.NewHTTPClient() *http.Client` — Task 2 and every vendor task (4-12) call these exact names/signatures.

- [ ] **Step 1: Write `upload.go`**

```go
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

	go func() {
		const maxRetries = 3
		var lastErr error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			if err := postOnce(client, body); err != nil {
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

func postOnce(client *http.Client, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, scriptURL, bytes.NewReader(body))
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
```

- [ ] **Step 2: Write `upload_test.go`**

```go
// GO/internal/driveupload/upload_test.go
package driveupload

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestSanitize_ReplacesEmptyWithNA(t *testing.T) {
	if got := sanitize(""); got != "NA" {
		t.Errorf("sanitize(\"\") = %q, want \"NA\"", got)
	}
}

func TestSanitize_StripsForbiddenCharacters(t *testing.T) {
	got := sanitize(`a\b/c:d*e?f"g<h>i|j[k]l`)
	want := "abcdefghijkl"
	if got != want {
		t.Errorf("sanitize(...) = %q, want %q", got, want)
	}
}

func TestSanitize_AllForbiddenCharactersBecomesNA(t *testing.T) {
	if got := sanitize(`\/:*?"<>|[]`); got != "NA" {
		t.Errorf("sanitize(all-forbidden) = %q, want \"NA\"", got)
	}
}

func TestFormatDate_ParsesAllListedLayouts(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"15/03/2026", "15-03-2026"},
		{"15-03-2026", "15-03-2026"},
		{"15/03/26", "15-03-2026"},
		{"15-03-26", "15-03-2026"},
		{"2026-03-15", "15-03-2026"},
		{"2026/03/15", "15-03-2026"},
		{"5/3/2026", "05-03-2026"},
	}
	for _, c := range cases {
		if got := formatDate(c.input); got != c.want {
			t.Errorf("formatDate(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestFormatDate_UnrecognizedInputReturnsNA(t *testing.T) {
	cases := []string{"", "Không tìm thấy", "Không hợp lệ", "not a date", "32/13/2026"}
	for _, c := range cases {
		if got := formatDate(c); got != "NA" {
			t.Errorf("formatDate(%q) = %q, want \"NA\"", c, got)
		}
	}
}

func TestBuildFilename_MatchesPythonBracketConvention(t *testing.T) {
	got := BuildFilename(Metadata{
		Vendor:       "COOP",
		EntryDate:    "15/03/2026",
		CustomerCode: "MNCOOP001",
		CancelDate:   "",
		OutputName:   "103229379-00",
	})
	want := "[COOP][15-03-2026][MNCOOP001][NA][103229379-00]"
	if got != want {
		t.Errorf("BuildFilename(...) = %q, want %q", got, want)
	}
}

// withTestServer points scriptURL at server.URL for the duration of the
// test, restoring the real value afterward via t.Cleanup. Must not be
// used from a t.Parallel() test — scriptURL is shared package state.
func withTestServer(t *testing.T, server *httptest.Server) {
	t.Helper()
	original := scriptURL
	scriptURL = server.URL
	t.Cleanup(func() { scriptURL = original })
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTempFile: %v", err)
	}
	return path
}

func TestUpload_ReturnsURLImmediatelyWithoutWaitingForNetwork(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // block until the test explicitly lets the response go
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	withTestServer(t, server)

	path := writeTempFile(t, "order.pdf", "fake pdf content")

	done := make(chan struct{})
	go func() {
		_, err := Upload(&http.Client{Timeout: 5 * time.Second}, path, Metadata{Vendor: "COOP", OutputName: "PO1"}, nil)
		if err != nil {
			t.Errorf("Upload returned error: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
		// Upload returned before the server's handler was released — correct.
	case <-time.After(2 * time.Second):
		t.Fatal("Upload did not return promptly; appears to be waiting on the network response")
	}
	close(release)
}

func TestUpload_RetriesOnFailureThenCallsOnResult(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	withTestServer(t, server)

	path := writeTempFile(t, "order.pdf", "fake pdf content")

	resultCh := make(chan bool, 1)
	_, err := Upload(&http.Client{Timeout: 5 * time.Second}, path, Metadata{Vendor: "COOP", OutputName: "PO1"}, func(ok bool, err error) {
		resultCh <- ok
	})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}

	select {
	case ok := <-resultCh:
		if !ok {
			t.Error("onResult called with ok=false, want true after eventual success")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("onResult was never called")
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("server received %d requests, want exactly 3", got)
	}
}

func TestUpload_AllRetriesFailedCallsOnResultFalse(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	withTestServer(t, server)

	path := writeTempFile(t, "order.pdf", "fake pdf content")

	resultCh := make(chan bool, 1)
	_, err := Upload(&http.Client{Timeout: 5 * time.Second}, path, Metadata{Vendor: "COOP", OutputName: "PO1"}, func(ok bool, err error) {
		if err == nil {
			t.Error("onResult called with a nil error on final failure, want a non-nil error")
		}
		resultCh <- ok
	})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}

	select {
	case ok := <-resultCh:
		if ok {
			t.Error("onResult called with ok=true, want false after all retries fail")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("onResult was never called")
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("server received %d requests, want exactly 3 (maxRetries)", got)
	}
}

func TestUpload_FileNotFoundReturnsErrorSynchronously(t *testing.T) {
	var serverHit int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&serverHit, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	withTestServer(t, server)

	nonexistent := filepath.Join(t.TempDir(), "does-not-exist.pdf")
	url, err := Upload(&http.Client{}, nonexistent, Metadata{Vendor: "COOP", OutputName: "PO1"}, nil)
	if err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
	if url != "" {
		t.Errorf("expected an empty URL on error, got %q", url)
	}

	// Give any (incorrectly) spawned goroutine a moment to fire, then
	// confirm the server was never actually hit.
	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(&serverHit); got != 0 {
		t.Errorf("server was hit %d times after a synchronous read failure, want 0 (no goroutine should have been spawned)", got)
	}
}

func TestUpload_SendsCorrectPayloadFields(t *testing.T) {
	type received struct {
		Filename string `json:"filename"`
		Ext      string `json:"ext"`
		Mime     string `json:"mime"`
		FileB64  string `json:"file_b64"`
	}
	resultCh := make(chan received, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var got received
		json.Unmarshal(body, &got)
		resultCh <- got
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	withTestServer(t, server)

	path := writeTempFile(t, "order.pdf", "fake pdf content")
	_, err := Upload(&http.Client{Timeout: 5 * time.Second}, path, Metadata{
		Vendor: "COOP", EntryDate: "15/03/2026", CustomerCode: "MNCOOP001", OutputName: "PO1",
	}, nil)
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}

	select {
	case got := <-resultCh:
		if got.Filename != "[COOP][15-03-2026][MNCOOP001][NA][PO1]" {
			t.Errorf("payload filename = %q, want the bracketed convention", got.Filename)
		}
		if got.Ext != "pdf" {
			t.Errorf("payload ext = %q, want \"pdf\" (no leading dot)", got.Ext)
		}
		if got.FileB64 == "" {
			t.Error("payload file_b64 is empty, want the base64-encoded file content")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server never received a request")
	}
}
```

- [ ] **Step 3: Run tests, verify all pass**

Run: `cd GO && go test ./internal/driveupload/... -v`
Expected: all tests PASS. If any fails, fix `upload.go` (not the test) unless the test itself is wrong per this plan's literal spec.

- [ ] **Step 4: `go vet`**

Run: `cd GO && go vet ./internal/driveupload/...`
Expected: no output (clean).

- [ ] **Step 5: Commit**

```bash
cd GO && git add internal/driveupload/upload.go internal/driveupload/upload_test.go
git commit -m "feat(go): add internal/driveupload package (port upload_file_to_drive from Python)"
```

---

### Task 2: `RealProcessor` fields + `app.go` wiring

**Files:**
- Modify: `GO/internal/processing/coop_processor.go` (where `RealProcessor` is defined)
- Modify: `GO/app.go`
- Modify: `GO/internal/processing/types.go` (add `OrderRow.DriveURL`)

**Interfaces:**
- Consumes: `driveupload.NewHTTPClient() *http.Client` (Task 1).
- Produces: `RealProcessor.DriveClient *http.Client`, `RealProcessor.LogFunc func(string)`, `OrderRow.DriveURL string` (json tag `driveUrl`) — every vendor task (4-12) reads `p.DriveClient`/`p.LogFunc` and sets `DriveURL` on the `OrderRow` it returns.

This task is merged (struct change + wiring in the same task) because changing `RealProcessor`'s fields without also wiring `app.go`'s `NewApp()` to populate them would leave the build red between commits — same reasoning already applied to earlier merged tasks in this project (see the settings-editor plan's Task 2).

- [ ] **Step 1: Add `DriveURL` to `OrderRow`**

Open `GO/internal/processing/types.go`. Find the `OrderRow` struct (starts `type OrderRow struct {`). Insert a new field right after `StatusKind string \`json:"statusKind"\`` and before the blank line + `PriceMismatchCount` block:

```go
	// DriveURL is the constructed "view" link from driveupload.Upload -
	// populated the moment a row is built (fire-and-forget: the real
	// upload may still be in progress or even fail in the background,
	// this URL is a best-effort placeholder from the start). Empty
	// string if the row's file was never uploaded (e.g. a Failed row
	// with no successfully-written Excel data to link to).
	DriveURL string `json:"driveUrl"`
```

Do not remove or reorder any other field/comment already in the struct.

- [ ] **Step 2: Add fields to `RealProcessor`**

Open `GO/internal/processing/coop_processor.go`. Add `"net/http"` to the import block (alphabetical order among the existing imports: `"context"`, `"fmt"`, `"math"`, `"net/http"`, `"path/filepath"`, `"strconv"`, `"strings"`).

Find:
```go
type RealProcessor struct {
	Store     *productdata.Store
	Pricing   PricingSource
	ExcelPath string
}
```

Replace with:
```go
type RealProcessor struct {
	Store       *productdata.Store
	Pricing     PricingSource
	ExcelPath   string
	DriveClient *http.Client // driveupload.NewHTTPClient() in production
	LogFunc     func(string) // optional (nil-safe) - routes background upload results to "process:log"
}
```

- [ ] **Step 3: Verify it fails to build (RealProcessor's new fields aren't wired yet, but that alone shouldn't break anything — this step is a sanity check, not a real red-state)**

Run: `cd GO && go build ./...`
Expected: PASS (adding unset-by-default struct fields doesn't break existing callers) — this step exists to confirm nothing yet depends on `DriveClient`/`LogFunc` being non-nil.

- [ ] **Step 4: Wire `app.go`**

Open `GO/app.go`. Add `"order-processor/internal/driveupload"` to the import block (alphabetical among the existing `order-processor/internal/...` imports: `applock`, `appsettings`, `config`, `driveupload`, `fileset`, `processing`, `processing/excelwriter`, `processing/pricing`, `processing/productdata`).

Find the block in `NewApp()` that currently reads:
```go
	return &App{
		cfg:              config.NewStore(configFileName),
		appSettingsStore: appSettingsStore,
		processor: &processing.RealProcessor{
			Store:     store,
			Pricing:   pricing.NewHTTPSource(settings.Gid),
			ExcelPath: excelPath,
		},
		orderDir:  orderFolderName,
		excelPath: excelPath,
	}, nil
```

Replace with:
```go
	processor := &processing.RealProcessor{
		Store:       store,
		Pricing:     pricing.NewHTTPSource(settings.Gid),
		ExcelPath:   excelPath,
		DriveClient: driveupload.NewHTTPClient(),
	}

	app := &App{
		cfg:              config.NewStore(configFileName),
		appSettingsStore: appSettingsStore,
		processor:        processor,
		orderDir:         orderFolderName,
		excelPath:        excelPath,
	}

	processor.LogFunc = func(msg string) {
		if app.emitter != nil {
			app.emitter.Emit("process:log", msg)
		}
	}

	return app, nil
```

**Why the `app.emitter != nil` check**: `NewApp()` runs BEFORE `startup(ctx)` — `app.emitter` isn't set yet when `NewApp()` returns. `LogFunc` is a closure called LATER (only once a vendor processor's background upload goroutine finishes, always after `startup()` has already run, since order processing can only start after the UI is up) — but the nil-check guards the astronomically unlikely case of a background upload finishing before `startup()` runs, avoiding a nil-pointer panic.

- [ ] **Step 5: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: clean build/vet. `go test ./...` matches the known baseline (only the 2 pre-existing Coop fixture failures, see Global Constraints).

- [ ] **Step 6: Commit**

```bash
cd GO && git add internal/processing/types.go internal/processing/coop_processor.go app.go
git commit -m "feat(go): wire driveupload into RealProcessor + app.go, add OrderRow.DriveURL"
```

---

### Task 3: Frontend — `types.ts` + `ResultTable.tsx`

**Files:**
- Modify: `GO/frontend/src/types.ts`
- Modify: `GO/frontend/src/components/ResultTable.tsx`

**Interfaces:**
- Consumes: `OrderRow.driveUrl` field (matches Task 2's Go `json:"driveUrl"` tag exactly — this task does not depend on Task 2 being done to compile, but the field will only ever carry a real value once Task 2 AND at least one vendor task have shipped).
- Produces: nothing further tasks depend on (this is the terminal UI surface for `driveUrl`).

- [ ] **Step 1: Add `driveUrl` to `types.ts`'s `OrderRow`**

Open `GO/frontend/src/types.ts`. Find the `OrderRow` interface. Add `driveUrl: string` as a new field — exact position doesn't matter for TypeScript, but for readability add it right after `statusKind`:

```typescript
export interface OrderRow {
  fileName: string
  page: string
  system: string
  maKhachHang: string
  po: string
  donGia: string
  status: string
  statusKind: string
  driveUrl: string
  priceMismatchCount: number
  priceMismatchDetails: PriceMismatchDetail[]
}
```

(Keep every other field in the interface exactly as it already is — this shows the full shape only so the insertion point is unambiguous; do not remove/rename anything else already there.)

- [ ] **Step 2: Add the column + rendering to `ResultTable.tsx`**

Open `GO/frontend/src/components/ResultTable.tsx`.

Add 2 imports at the top, alongside the existing `react-icons/fa6` import and the existing `wailsjs/go/main/App` import:
```typescript
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { FaUpRightFromSquare } from 'react-icons/fa6'
```

Find the `columns` array:
```typescript
const columns: { key: Exclude<keyof OrderRow, 'priceMismatchDetails'>; label: string }[] = [
  { key: 'fileName', label: 'Tên file' },
  { key: 'page', label: 'Trang' },
  { key: 'system', label: 'Hệ thống' },
  { key: 'maKhachHang', label: 'Mã khách hàng' },
  { key: 'po', label: 'PO' },
  { key: 'donGia', label: 'Đơn giá' },
  { key: 'priceMismatchCount', label: 'Đối soát giá' },
  { key: 'status', label: 'Trạng thái' },
]
```

Replace with (adds one entry at the end):
```typescript
const columns: { key: Exclude<keyof OrderRow, 'priceMismatchDetails'>; label: string }[] = [
  { key: 'fileName', label: 'Tên file' },
  { key: 'page', label: 'Trang' },
  { key: 'system', label: 'Hệ thống' },
  { key: 'maKhachHang', label: 'Mã khách hàng' },
  { key: 'po', label: 'PO' },
  { key: 'donGia', label: 'Đơn giá' },
  { key: 'priceMismatchCount', label: 'Đối soát giá' },
  { key: 'status', label: 'Trạng thái' },
  { key: 'driveUrl', label: 'File Drive' },
]
```

Find the render chain inside the `<td>` (the `c.key === 'status' ? (...) : c.key === 'priceMismatchCount' ? (...) : c.key === 'donGia' ? (...) : ( row[c.key] )` ternary chain). Insert a new branch for `driveUrl` right before the final `: (` fallback branch (i.e. right after the `c.key === 'donGia'` branch's closing `)`):

```tsx
                          ) : c.key === 'donGia' ? (
                            <span className="font-semibold text-accent">{formatMoney(row[c.key])}</span>
                          ) : c.key === 'driveUrl' ? (
                            row.driveUrl ? (
                              <button
                                type="button"
                                onClick={(e) => {
                                  e.stopPropagation()
                                  BrowserOpenURL(row.driveUrl)
                                }}
                                className="inline-flex items-center gap-1.5 rounded-full border border-border px-2.5 py-0.5 font-sans font-semibold text-accent transition-colors hover:border-accent"
                              >
                                <FaUpRightFromSquare size={9} /> Mở file
                              </button>
                            ) : (
                              <span className="text-muted">—</span>
                            )
                          ) : (
                            row[c.key]
                          )}
```

(This replaces the existing `) : ( row[c.key] )}` tail of the ternary chain with the version above that adds the new `driveUrl` branch before the final fallback — every other branch in the chain, `status`/`priceMismatchCount`/`donGia`, stays exactly as it already is.)

The `e.stopPropagation()` inside the button's `onClick` is required: the parent `<td>` already has its own `onClick={() => handleCopy(cellKey, copyValue)}` (copy-to-clipboard) — without stopping propagation, clicking "Mở file" would also trigger a copy, matching the same pattern the price-mismatch "Dùng giá PO"/"Dùng giá hệ thống" buttons already use elsewhere in this same file.

- [ ] **Step 3: Type-check**

Run: `cd GO/frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Verify visually**

Per this plan's Global Constraints, try `wails dev` first. If the sandbox network limitation blocks a real end-to-end order-processing test, fall back to isolated component testing:
1. Temporarily edit `GO/frontend/src/main.tsx` to render `<ResultTable />` directly (bypassing `<App />`), with `useAppStore.getState().appendRow({ fileName: 'test.pdf', page: '1/1', system: 'Coop', maKhachHang: 'MNCOOP001', po: '123', donGia: '100000', status: '✅ Hoàn thành', statusKind: 'done', driveUrl: 'https://example.com/view?po=123', priceMismatchCount: 0, priceMismatchDetails: [] })` called once before the render (or via a small inline script) to seed one visible row.
2. Run a plain `npm run dev` (not `wails dev`, since this doesn't need the Go backend) inside `GO/frontend/`, navigate via Playwright, screenshot, confirm the "File Drive" column renders a working "Mở file" button.
3. Revert `main.tsx` back to its original content afterward (confirm `git diff GO/frontend/src/main.tsx` shows no changes before moving on).

Record in the task report exactly which path (real `wails dev` end-to-end, or the isolated fallback) was actually used — do not claim end-to-end verification if only the isolated fallback ran.

- [ ] **Step 5: Commit**

```bash
cd GO && git add frontend/src/types.ts frontend/src/components/ResultTable.tsx
git commit -m "feat(frontend): add File Drive column to ResultTable"
```

---

### Task 4: Coop integration (worked example — do this one first among vendors)

**Files:**
- Modify: `GO/internal/processing/coop_processor.go`

**Interfaces:**
- Consumes: `driveupload.Upload`/`driveupload.Metadata` (Task 1), `RealProcessor.DriveClient`/`.LogFunc` (Task 2), `OrderRow.DriveURL` (Task 2).
- Produces: nothing further tasks depend on (each vendor task is independent of the others).

- [ ] **Step 1: Add the import**

Add `"order-processor/internal/driveupload"` to `coop_processor.go`'s import block.

- [ ] **Step 2: Insert the upload call in `processSegment`**

Find, in `processSegment` (currently around line 413-432):
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: system, MaKhachHang: customerCode,
		PO: info.PONumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

Replace with:
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
		Vendor:       "COOP",
		EntryDate:    entryDate,
		CustomerCode: customerCode,
		CancelDate:   cancelDate,
		OutputName:   info.PONumber,
	}, func(ok bool, err error) {
		if p.LogFunc == nil {
			return
		}
		if ok {
			p.LogFunc(fmt.Sprintf("✅ Đã upload file lên Drive: %s", filepath.Base(filePath)))
		} else {
			p.LogFunc(fmt.Sprintf("❌ Upload Drive thất bại (%s): %v", filepath.Base(filePath), err))
		}
	})
	if uploadErr != nil && p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không đọc được file để upload Drive: %v", uploadErr))
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: system, MaKhachHang: customerCode,
		PO: info.PONumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		DriveURL: driveURL,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

Note: `uploadErr` (a synchronous file-read failure) must NOT cause `return OrderRow{}, uploadErr` — the Excel row is already written by this point. Only log it; `DriveURL` simply stays empty in that case.

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: clean build/vet. `go test ./...` matches the known baseline (2 pre-existing Coop fixture failures only — this task's own change does NOT touch fixture-comparison logic, so the golden-fixture tests exercising Coop's happy path should stay green; `DriveURL` is a new field the golden-fixture comparison does not check against a frozen Python value, since Python never wrote this anywhere comparable).

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/coop_processor.go
git commit -m "feat(go): upload Coop orders to Drive after processing"
```

---

### Task 5: Lotte integration

**Files:**
- Modify: `GO/internal/processing/lotte_processor.go`

**Interfaces:**
- Consumes: same as Task 4.

- [ ] **Step 1: Add the import**

Add `"order-processor/internal/driveupload"` to `lotte_processor.go`'s import block.

- [ ] **Step 2: Insert the upload call in `processLotteSegment`**

Find (currently around line 234-254):
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Lotte", MaKhachHang: customerCode,
		PO: info.PONumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

Replace with:
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
		Vendor:       "LOTTE",
		EntryDate:    info.EntryDate,
		CustomerCode: customerCode,
		CancelDate:   cancelDate,
		OutputName:   info.PONumber,
	}, func(ok bool, err error) {
		if p.LogFunc == nil {
			return
		}
		if ok {
			p.LogFunc(fmt.Sprintf("✅ Đã upload file lên Drive: %s", filepath.Base(filePath)))
		} else {
			p.LogFunc(fmt.Sprintf("❌ Upload Drive thất bại (%s): %v", filepath.Base(filePath), err))
		}
	})
	if uploadErr != nil && p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không đọc được file để upload Drive: %v", uploadErr))
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Lotte", MaKhachHang: customerCode,
		PO: info.PONumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		DriveURL: driveURL,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

**Known behavior, not a bug**: `cancelDate` here (from `lotte.ExtractCancelDate`) can be a multi-line string joined with `"\n"` when several lines in the source PDF match the date-line pattern. `formatDate` (Task 1) will not parse that shape and will correctly fall back to `"NA"` in the Drive filename — this is the intended fallback behavior, not something to special-case or "fix" in this task.

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: clean, matches known baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/lotte_processor.go
git commit -m "feat(go): upload Lotte orders to Drive after processing"
```

---

### Task 6: Satra integration

**Files:**
- Modify: `GO/internal/processing/satra_processor.go`

**Interfaces:**
- Consumes: same as Task 4.

- [ ] **Step 1: Add the import**

Add `"order-processor/internal/driveupload"` to `satra_processor.go`'s import block.

- [ ] **Step 2: Insert the upload call in `processSatraSegment`**

Find (currently around line 273-293):
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", noteText, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Satra", MaKhachHang: customerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

Replace with:
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", noteText, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
		Vendor:       "SATRA",
		EntryDate:    entryDate,
		CustomerCode: customerCode,
		CancelDate:   cancelDate,
		OutputName:   poNumber,
	}, func(ok bool, err error) {
		if p.LogFunc == nil {
			return
		}
		if ok {
			p.LogFunc(fmt.Sprintf("✅ Đã upload file lên Drive: %s", filepath.Base(filePath)))
		} else {
			p.LogFunc(fmt.Sprintf("❌ Upload Drive thất bại (%s): %v", filepath.Base(filePath), err))
		}
	})
	if uploadErr != nil && p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không đọc được file để upload Drive: %v", uploadErr))
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Satra", MaKhachHang: customerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		DriveURL: driveURL,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: clean, matches known baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/satra_processor.go
git commit -m "feat(go): upload Satra orders to Drive after processing"
```

---

### Task 7: Emart integration

**Files:**
- Modify: `GO/internal/processing/emart_processor.go`

**Interfaces:**
- Consumes: same as Task 4.

- [ ] **Step 1: Add the import**

Add `"order-processor/internal/driveupload"` to `emart_processor.go`'s import block.

- [ ] **Step 2: Insert the upload call in `processEmartSegment`**

Find (currently around line 301-324):
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	// Python's own status logic (xulydonhang.py:9367) flags a warning
	// when saigia>0 OR the store couldn't be resolved (tenstore
	// falls back to "Không xác định") — mirrored here via storeName=="".
	if saigia > 0 || storeName == "" {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Emart", MaKhachHang: emartCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

Replace with:
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
		Vendor:       "EMART",
		EntryDate:    entryDate,
		CustomerCode: emartCustomerCode,
		CancelDate:   cancelDate,
		OutputName:   poNumber,
	}, func(ok bool, err error) {
		if p.LogFunc == nil {
			return
		}
		if ok {
			p.LogFunc(fmt.Sprintf("✅ Đã upload file lên Drive: %s", filepath.Base(filePath)))
		} else {
			p.LogFunc(fmt.Sprintf("❌ Upload Drive thất bại (%s): %v", filepath.Base(filePath), err))
		}
	})
	if uploadErr != nil && p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không đọc được file để upload Drive: %v", uploadErr))
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	// Python's own status logic (xulydonhang.py:9367) flags a warning
	// when saigia>0 OR the store couldn't be resolved (tenstore
	// falls back to "Không xác định") — mirrored here via storeName=="".
	if saigia > 0 || storeName == "" {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Emart", MaKhachHang: emartCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		DriveURL: driveURL,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

Note: `emartCustomerCode` is a package-level constant (Emart has exactly one fixed customer), not a local variable — used directly, same as the existing code already does for `MaKhachHang`.

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: clean, matches known baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/emart_processor.go
git commit -m "feat(go): upload Emart orders to Drive after processing"
```

---

### Task 8: Kingfood integration

**Files:**
- Modify: `GO/internal/processing/kingfood_processor.go`

**Interfaces:**
- Consumes: same as Task 4.

- [ ] **Step 1: Add the import**

Add `"order-processor/internal/driveupload"` to `kingfood_processor.go`'s import block.

- [ ] **Step 2: Insert the upload call in `processKingfoodSegment`**

Find (currently around line 279-299):
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Kingfood", MaKhachHang: kingfoodCustomerCode,
```

(the return statement continues on the next lines with `PO:`/etc. — leave those untouched, only the block above the `return OrderRow{` line and the addition of `DriveURL` inside it are shown here for brevity; see Step 2's full replacement below which repeats the complete statement.)

Replace with:
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
		Vendor:       "KINGFOOD",
		EntryDate:    entryDate,
		CustomerCode: kingfoodCustomerCode,
		CancelDate:   cancelDate,
		OutputName:   poNumber,
	}, func(ok bool, err error) {
		if p.LogFunc == nil {
			return
		}
		if ok {
			p.LogFunc(fmt.Sprintf("✅ Đã upload file lên Drive: %s", filepath.Base(filePath)))
		} else {
			p.LogFunc(fmt.Sprintf("❌ Upload Drive thất bại (%s): %v", filepath.Base(filePath), err))
		}
	})
	if uploadErr != nil && p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không đọc được file để upload Drive: %v", uploadErr))
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Kingfood", MaKhachHang: kingfoodCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		DriveURL: driveURL,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

(The final `return OrderRow{...}` statement's remaining fields — `PO`, `DonGia`, `Status`, `StatusKind`, `SkuLog`, `PriceMismatchCount`, `PriceMismatchDetails` — are unchanged from the existing code; only `DriveURL: driveURL,` is newly inserted into that literal, exactly as shown above.)

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: clean, matches known baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/kingfood_processor.go
git commit -m "feat(go): upload Kingfood orders to Drive after processing"
```

---

### Task 9: Winmart integration

**Files:**
- Modify: `GO/internal/processing/winmart_processor.go`

**Interfaces:**
- Consumes: same as Task 4.

- [ ] **Step 1: Add the import**

Add `"order-processor/internal/driveupload"` to `winmart_processor.go`'s import block.

- [ ] **Step 2: Insert the upload call in `processWinmartSegment`**

Find (currently around line 326-347):
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Winmart", MaKhachHang: customerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

Replace with:
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
		Vendor:       "WINMART",
		EntryDate:    entryDate,
		CustomerCode: customerCode,
		CancelDate:   cancelDate,
		OutputName:   poNumber,
	}, func(ok bool, err error) {
		if p.LogFunc == nil {
			return
		}
		if ok {
			p.LogFunc(fmt.Sprintf("✅ Đã upload file lên Drive: %s", filepath.Base(filePath)))
		} else {
			p.LogFunc(fmt.Sprintf("❌ Upload Drive thất bại (%s): %v", filepath.Base(filePath), err))
		}
	})
	if uploadErr != nil && p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không đọc được file để upload Drive: %v", uploadErr))
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Winmart", MaKhachHang: customerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		DriveURL: driveURL,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: clean, matches known baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/winmart_processor.go
git commit -m "feat(go): upload Winmart orders to Drive after processing"
```

---

### Task 10: FujiMart integration

**Files:**
- Modify: `GO/internal/processing/fujimart_processor.go`

**Interfaces:**
- Consumes: same as Task 4.

- [ ] **Step 1: Add the import**

Add `"order-processor/internal/driveupload"` to `fujimart_processor.go`'s import block.

- [ ] **Step 2: Insert the upload call in `processFujimartSegment`**

Find (currently around line 248-267, the tail of the function — the exact `return OrderRow{...}` fields after `DonGia`/`Status`/`StatusKind` are `SkuLog`/`PriceMismatchCount`/`PriceMismatchDetails`, same shape as every other vendor above):
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "FujiMart", MaKhachHang: fujimartCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

Replace with:
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
		Vendor:       "FUJIMART",
		EntryDate:    entryDate,
		CustomerCode: fujimartCustomerCode,
		CancelDate:   cancelDate,
		OutputName:   poNumber,
	}, func(ok bool, err error) {
		if p.LogFunc == nil {
			return
		}
		if ok {
			p.LogFunc(fmt.Sprintf("✅ Đã upload file lên Drive: %s", filepath.Base(filePath)))
		} else {
			p.LogFunc(fmt.Sprintf("❌ Upload Drive thất bại (%s): %v", filepath.Base(filePath), err))
		}
	})
	if uploadErr != nil && p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không đọc được file để upload Drive: %v", uploadErr))
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "FujiMart", MaKhachHang: fujimartCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		DriveURL: driveURL,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: clean, matches known baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/fujimart_processor.go
git commit -m "feat(go): upload FujiMart orders to Drive after processing"
```

---

### Task 11: JMart integration

**Files:**
- Modify: `GO/internal/processing/jmart_processor.go`

**Interfaces:**
- Consumes: same as Task 4. Also relies on Task 1's `"2/1/2006"` layout (JMart's date format is the one that needs it — see Global Constraints).

- [ ] **Step 1: Add the import**

Add `"order-processor/internal/driveupload"` to `jmart_processor.go`'s import block.

- [ ] **Step 2: Insert the upload call in `processJMartSegment`**

Find (currently around line 235-254):
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "JMart", MaKhachHang: jmartCustomerCode,
```

(as with Task 8/Kingfood, the return statement's remaining fields continue unchanged on subsequent lines — full replacement below.)

Replace with:
```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
		Vendor:       "JMART",
		EntryDate:    entryDate,
		CustomerCode: jmartCustomerCode,
		CancelDate:   cancelDate,
		OutputName:   poNumber,
	}, func(ok bool, err error) {
		if p.LogFunc == nil {
			return
		}
		if ok {
			p.LogFunc(fmt.Sprintf("✅ Đã upload file lên Drive: %s", filepath.Base(filePath)))
		} else {
			p.LogFunc(fmt.Sprintf("❌ Upload Drive thất bại (%s): %v", filepath.Base(filePath), err))
		}
	})
	if uploadErr != nil && p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không đọc được file để upload Drive: %v", uploadErr))
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "JMart", MaKhachHang: jmartCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		DriveURL: driveURL,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: clean, matches known baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/jmart_processor.go
git commit -m "feat(go): upload JMart orders to Drive after processing"
```

---

### Task 12: BigC integration (architecturally different — one upload per whole file, not per store page)

**Files:**
- Modify: `GO/internal/processing/bigc_processor.go`

**Interfaces:**
- Consumes: same as Task 4.

BigC's `processBigcDocument` processes an entire multi-page file in one call (multiple store pages), accumulating every successful store page's rows into one `allRows` slice and calling `excelwriter.WriteOrderRows` exactly ONCE for the whole file — AFTER each store page's own `OrderRow` has already been appended to the `orderRows` slice inside the per-page loop. Since the source file (`filePath`) is the same for every store page in the document, and this plan's Global Constraints already commit to "upload the whole file, never split pages", `driveupload.Upload` is called exactly ONCE here (not once per store page), with the resulting URL backfilled onto every `OrderRow` already built — the same pattern the existing code already uses to backfill `PriceMismatchDetails[].ExcelRow` at this exact point.

- [ ] **Step 1: Add the import**

Add `"order-processor/internal/driveupload"` to `bigc_processor.go`'s import block.

- [ ] **Step 2: Insert the single upload call + backfill in `processBigcDocument`**

Find (currently around line 155-166):
```go
	if len(allRows) > 0 {
		headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
		startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, allRows, headerDescription)
		if err != nil {
			return nil, err
		}
		for i := range orderRows {
			for j := range orderRows[i].PriceMismatchDetails {
				orderRows[i].PriceMismatchDetails[j].ExcelRow += startRow
			}
		}
	}

	return orderRows, nil
}
```

Replace with:
```go
	if len(allRows) > 0 {
		headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
		startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, allRows, headerDescription)
		if err != nil {
			return nil, err
		}
		for i := range orderRows {
			for j := range orderRows[i].PriceMismatchDetails {
				orderRows[i].PriceMismatchDetails[j].ExcelRow += startRow
			}
		}

		driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
			Vendor:       "BIGC",
			EntryDate:    entryDate,
			CustomerCode: customerCode,
			CancelDate:   cancelDate,
			OutputName:   poNumber,
		}, func(ok bool, err error) {
			if p.LogFunc == nil {
				return
			}
			if ok {
				p.LogFunc(fmt.Sprintf("✅ Đã upload file lên Drive: %s", filepath.Base(filePath)))
			} else {
				p.LogFunc(fmt.Sprintf("❌ Upload Drive thất bại (%s): %v", filepath.Base(filePath), err))
			}
		})
		if uploadErr != nil && p.LogFunc != nil {
			p.LogFunc(fmt.Sprintf("⚠️ Không đọc được file để upload Drive: %v", uploadErr))
		}
		for i := range orderRows {
			orderRows[i].DriveURL = driveURL
		}
	}

	return orderRows, nil
}
```

`entryDate`, `cancelDate`, `customerCode`, `poNumber` are all already in scope at this point in `processBigcDocument` (declared earlier in the function, from `bigc.ParseOrderInfo(pageTexts[0])` and `bigc.ResolveCustomerCode(pageTexts[0])`) — no new variables need to be introduced, only read.

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: clean, matches known baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/bigc_processor.go
git commit -m "feat(go): upload BigC orders to Drive after processing (one upload per file)"
```

---

## Final Verification

After all 12 tasks:
- [ ] `cd GO && go build ./... && go vet ./... && go test ./...` — clean, matches known baseline exactly (2 pre-existing Coop fixture failures, zero new regressions).
- [ ] `cd GO/frontend && npx tsc --noEmit` — clean.
- [ ] `cd GO && wails build` — builds a production exe successfully.
- [ ] Grep confirms all 9 vendor processor files import and call `driveupload.Upload` exactly once each (BigC: once per whole document, not per store page — see Task 12).
