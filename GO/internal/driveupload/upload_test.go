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
