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

	// Read the whole file once and strip any inline-image binary payload
	// (BI...ID...EI construct — see stripInlineImageData's own doc
	// comment) before the vendored library ever tokenizes anything. This
	// runs unconditionally, not just as an error fallback: the failure
	// this fixes surfaces later, per-page, inside page.GetPlainText()
	// (pdfextract.go's extractPageText) — not inside pdf.NewReader
	// itself, which already succeeds for these files today — so there is
	// no "try first, retry on error" moment to hook at this layer. The
	// scan is cheap and a guaranteed no-op for any file with no "BI"
	// keyword in it at all (every already-shipped vendor's PDFs today).
	raw := make([]byte, size)
	if _, readErr := f.ReadAt(raw, 0); readErr != nil {
		f.Close()
		return nil, nil, readErr
	}
	raw = stripInlineImageData(raw)

	r, err = pdf.NewReader(bytes.NewReader(raw), int64(len(raw)))
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
	// Adapted to work against the in-memory (possibly now inline-image-
	// stripped) copy rather than the raw *os.File, so both fixes compose
	// correctly for a file that needs both.
	if trimmedLen, found := trimTrailingGarbageAfterEOFBytes(raw); found {
		if r2, err2 := pdf.NewReader(bytes.NewReader(raw[:trimmedLen]), int64(trimmedLen)); err2 == nil {
			return f, r2, nil
		}
	}

	f.Close()
	return nil, nil, err
}

// stripInlineImageData replaces every "BI ... ID<ws><data>EI"
// inline-image construct's raw <data> payload with ASCII space bytes, in
// place, leaving every other byte (and the total length) unchanged — so
// any xref byte offset elsewhere in the file remains valid against the
// modified copy. Real PDFs from Emart's PO-generation system embed
// inline raster images directly in the page content stream (confirmed:
// "BI /W 544 /H 150 /BPC 1 /IM true /F [/A85 /Fl] ID <binary> EI" on
// đơn hàng/08-2026/4501866956.PDF's content stream, object 39, which
// like all 17 real Emart PDFs' content streams has NO outer /Filter —
// its bytes in the file are the literal, uncompressed operator text)
// rather than as XObjects. The vendored github.com/ledongthuc/pdf
// library's generic content-stream tokenizer (shared by both
// page.GetPlainText() and page.Content(), both built on the same
// exported pdf.Interpret) has no special handling for this construct: it
// tries to tokenize the raw/encoded binary payload as ordinary PDF
// syntax and fails, with a different surface error depending on which
// byte sequence it chokes on first ("malformed name", "unexpected
// delimiter", "unexpected keyword" — all confirmed on real Emart PDFs,
// all the same root cause). Since inline image data can never contain a
// text-showing operator (it's pure raster pixel data per the PDF spec),
// removing it entirely is lossless for TEXT extraction.
//
// This is a heuristic, whitespace/delimiter-bounded scan for "EI"
// (rather than relying on an inline image's declared /L length key,
// which isn't present in the confirmed real cases) — the same approach
// most lenient real-world PDF readers use for this construct. Confirmed
// safe against all 17 real Emart PDFs (each has 1-2 BI occurrences, all
// correctly found and stripped — see this task's own report for the
// full per-file verification).
func stripInlineImageData(data []byte) []byte {
	pos := 0
	for {
		biIdx := findPDFKeyword(data, pos, "BI")
		if biIdx < 0 {
			break
		}
		idIdx := findPDFKeyword(data, biIdx+2, "ID")
		if idIdx < 0 {
			pos = biIdx + 2
			continue
		}
		dataStart := idIdx + 2
		if dataStart < len(data) && isPDFWhitespace(data[dataStart]) {
			dataStart++
		}
		eiIdx := findPDFKeyword(data, dataStart, "EI")
		if eiIdx < 0 {
			pos = dataStart
			continue
		}
		for i := dataStart; i < eiIdx; i++ {
			data[i] = ' '
		}
		pos = eiIdx + 2
	}
	return data
}

// findPDFKeyword returns the index of the next occurrence of keyword in
// data at or after start, requiring it to be bounded by PDF whitespace
// or a PDF delimiter character on both sides (so it matches a real,
// standalone operator/keyword token, not a coincidental byte substring
// inside unrelated binary data) — returns -1 if none found.
func findPDFKeyword(data []byte, start int, keyword string) int {
	kw := []byte(keyword)
	for i := start; i+len(kw) <= len(data); i++ {
		if !bytes.Equal(data[i:i+len(kw)], kw) {
			continue
		}
		beforeOK := i == 0 || isPDFWhitespace(data[i-1]) || isPDFDelimiter(data[i-1])
		afterIdx := i + len(kw)
		afterOK := afterIdx == len(data) || isPDFWhitespace(data[afterIdx]) || isPDFDelimiter(data[afterIdx])
		if beforeOK && afterOK {
			return i
		}
	}
	return -1
}

func isPDFWhitespace(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', '\f', 0:
		return true
	}
	return false
}

func isPDFDelimiter(b byte) bool {
	switch b {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

// eofScanWindow bounds how far back from the file's end
// trimTrailingGarbageAfterEOFBytes looks for a "%%EOF" marker — generous
// enough to cover every trailing-garbage case seen on real PDFs so far
// (confirmed up to ~150 bytes on Emart's real corpus) with a wide safety
// margin, small enough to stay cheap.
const eofScanWindow = 4096

// trimTrailingGarbageAfterEOFBytes is Task 3b's trailing-%%EOF-garbage
// fix (see that task's own history for the full root-cause writeup),
// adapted to operate on an in-memory byte slice instead of an *os.File
// — needed here because this fix's own inline-image stripping already
// requires materializing the whole file in memory first, and both fixes
// must compose for a file that needs both. Deliberately NOT reusing
// Task 3b's original trimTrailingGarbageAfterEOF(f *os.File, ...)
// function as-is (different parameter type) — this is a small, separate
// byte-slice-native variant that pdfOpen now calls exclusively, since
// the primary pdf.NewReader attempt always runs against the in-memory
// (already inline-image-stripped) copy, never the raw *os.File.
func trimTrailingGarbageAfterEOFBytes(data []byte) (trimmedLen int, found bool) {
	window := eofScanWindow
	if window > len(data) {
		window = len(data)
	}
	start := len(data) - window
	idx := bytes.LastIndex(data[start:], []byte("%%EOF"))
	if idx < 0 {
		return 0, false
	}
	return start + idx + len("%%EOF"), true
}
