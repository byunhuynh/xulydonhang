package processing

import (
	"bytes"
	"fmt"
	"os"

	"github.com/ledongthuc/pdf"
)

// pdfOpen wraps pdf.Open with a panic recovery boundary. Most of
// ledongthuc/pdf's malformed-input handling (e.g. inside Page.Content)
// already recovers its own internal panics and turns them into errors,
// but the reader-construction path (NewReaderEncrypted -> readXref)
// does not: a PDF whose trailer's startxref offset doesn't actually
// point at a cross-reference table (seen in the wild on at least one
// real archived Coop PDF whose startxref pointed exactly at EOF instead
// of ~340 bytes earlier, where the real "xref" keyword was) panics all
// the way out of pdf.Open instead of returning an error. A single
// malformed PDF file must never be able to crash the whole process —
// convert that panic into a regular error here, same shape as any other
// pdf.Open failure, so callers (extractPageTexts, and ultimately
// RealProcessor.Process) handle it via their existing "không đọc được
// PDF" / Failed-row error path instead of taking down the app.
func pdfOpen(path string) (f *os.File, r *pdf.Reader, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			if f != nil {
				f.Close()
			}
			f, r = nil, nil
			err = fmt.Errorf("malformed PDF (panic while reading): %v", rec)
		}
	}()

	f, err = os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	fi, statErr := f.Stat()
	if statErr != nil {
		f.Close()
		return nil, nil, statErr
	}
	size := fi.Size()

	r, err = pdf.NewReader(f, size)
	if err == nil {
		return f, r, nil
	}

	// Fallback: some real PDFs have extra, garbled bytes appended AFTER
	// their real, valid "%%EOF" marker — the vendored ledongthuc/pdf
	// library's own NewReaderEncrypted only looks at the file's last 100
	// bytes and requires them (after trimming trailing whitespace) to end
	// in "%%EOF" exactly (read.go:139-148 in the vendored module), so it
	// rejects these outright with "not a PDF file: missing %%EOF" even
	// though the file is otherwise well-formed. Confirmed on all 17 real
	// Emart PDFs in đơn hàng/08-2026/ during Emart plan Task 3's real-PDF
	// verification — the real Python system's PyMuPDF-based reader opens
	// every one of them without issue; only this Go library's stricter
	// tail check rejects them. Scan backward from the file's end for the
	// LAST real "%%EOF" occurrence and re-open with a truncated LOGICAL
	// size that ends immediately after it — this doesn't touch the file
	// on disk, only how much of it this read treats as "the file", so
	// the library's own tail check now sees a clean, well-formed ending.
	if trimmedSize, found := trimTrailingGarbageAfterEOF(f, size); found {
		if r2, err2 := pdf.NewReader(f, trimmedSize); err2 == nil {
			return f, r2, nil
		}
	}

	f.Close()
	return nil, nil, err
}

// eofScanWindow bounds how far back from the file's end
// trimTrailingGarbageAfterEOF looks for a "%%EOF" marker — generous
// enough to cover every trailing-garbage case seen on real PDFs so far
// (confirmed up to ~150 bytes on Emart's real corpus) with a wide safety
// margin, small enough to stay cheap.
const eofScanWindow = 4096

// trimTrailingGarbageAfterEOF scans the last eofScanWindow bytes of f for
// the LAST occurrence of the literal "%%EOF" marker and returns the
// logical file size that ends immediately after it (so
// pdf.NewReader's own last-100-byte check, which requires the file's
// real tail to end in "%%EOF", succeeds against that adjusted size).
// Returns found=false if no "%%EOF" appears in that window at all (a
// genuinely different kind of malformed file, not this specific
// trailing-garbage pattern — the original error is returned unchanged in
// that case).
func trimTrailingGarbageAfterEOF(f *os.File, size int64) (trimmedSize int64, found bool) {
	window := int64(eofScanWindow)
	if window > size {
		window = size
	}
	buf := make([]byte, window)
	if _, err := f.ReadAt(buf, size-window); err != nil {
		return 0, false
	}
	idx := bytes.LastIndex(buf, []byte("%%EOF"))
	if idx < 0 {
		return 0, false
	}
	return size - window + int64(idx) + int64(len("%%EOF")), true
}
