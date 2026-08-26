package processing

import (
	"bytes"
	"fmt"
	"os"
	"strconv"

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
//
// The returned *os.File is no longer the pdf.Reader's actual data
// source (that's now always an in-memory byte slice — see the
// inline-image-stripping and %%EOF-trimming steps below): it exists
// only so callers can defer f.Close() on the open file handle. Do not
// assume closing or otherwise touching this *os.File affects anything
// the returned *pdf.Reader still needs.
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
	normalizePDFHeader(raw)

	r, err = tryNewReader(raw)
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
	trimmed := raw
	if trimmedLen, found := trimTrailingGarbageAfterEOFBytes(raw); found {
		trimmed = raw[:trimmedLen]
		if r2, err2 := tryNewReader(trimmed); err2 == nil {
			return f, r2, nil
		}
	}

	// Fallback: the trailer's startxref offset does not actually point at
	// the cross-reference table. Confirmed on a real archived Coop PDF
	// (coop/testdata/realpdfs/103229379-00.pdf, PO 103229379): its
	// startxref says 9482, which is the file's own total length, while
	// the "xref" keyword that starts its real table sits 342 bytes
	// earlier at 9140 — every object, the table and the trailer are
	// intact, only that one number is wrong. The vendored library seeks
	// straight to the declared offset and panics on EOF instead of
	// looking for the table, so before this fallback the file was
	// unreadable and produced a Failed row. Rebuilding the offset is
	// exactly what lenient real-world readers do for this shape.
	//
	// A file whose recorded entries are themselves wrong needs more than
	// a corrected offset, so rebuildXref (pdfxrefrepair.go) follows as a
	// last resort — the real Coop file above turns out to need both.
	// Every repaired reader is additionally required to resolve its own
	// page tree before being handed back: a rebuilt table that merely
	// parses, but whose entries still point at the wrong bytes, would
	// otherwise panic later inside the caller instead of failing here.
	repairs := []func([]byte) ([]byte, bool){repairStartxrefOffset, rebuildXref}
	for _, candidate := range [][]byte{raw, trimmed} {
		for _, repair := range repairs {
			repaired, ok := repair(candidate)
			if !ok {
				continue
			}
			r3, err3 := tryNewReader(repaired)
			if err3 != nil || !readerHasPages(r3) {
				continue
			}
			return f, r3, nil
		}
	}

	f.Close()
	return nil, nil, err
}

// normalizePDFHeader repairs, IN PLACE, a "%PDF-1.x" header whose
// version number is followed by a space instead of the CR or LF the
// vendored library demands. That library's NewReaderEncrypted checks
// byte 8 explicitly (read.go:136 requires it to be '\r' or '\n')
// and rejects anything else outright with "not a PDF file: invalid
// header" — before parsing a single object, so no later fallback in
// pdfOpen ever gets a chance to run.
//
// Confirmed on every real archived Maxidi delivery note (all four
// samples available when this was written): they begin with the ten
// bytes "%PDF-1.2 \n" — a Crystal Reports quirk. PyMuPDF, which the
// original Python app used, opens them without complaint; only this
// library's stricter byte-8 check rejects them, and it rejects 100% of
// this vendor's files.
//
// Writes a newline over the offending space rather than deleting it:
// every xref entry in a PDF is an absolute byte offset into the file,
// so removing even one byte would invalidate the whole table. A
// same-length substitution keeps every offset exactly where it was.
//
// Deliberately narrow — only a literal space is replaced, and only when
// the first eight bytes already spell a valid "%PDF-1.x" version. A
// header that is malformed in any OTHER way still reaches the library
// and is still rejected there, rather than being silently "repaired"
// into something that merely looks valid.
func normalizePDFHeader(data []byte) {
	if len(data) < 9 {
		return
	}
	if !bytes.HasPrefix(data, []byte("%PDF-1.")) {
		return
	}
	if data[7] < '0' || data[7] > '7' {
		return
	}
	if data[8] == ' ' {
		data[8] = '\n'
	}
}

// readerHasPages reports whether r can actually resolve its page tree,
// with a panic boundary because the vendored library panics rather than
// returning an error when an xref entry points at bytes that are not the
// object it promised. Used to validate a REPAIRED reader before pdfOpen
// hands it back — a repair that produces a reader which only fails later
// is worse than no repair at all, since the caller's own error path
// never gets the chance to report the file as unreadable.
func readerHasPages(r *pdf.Reader) (ok bool) {
	if r == nil {
		return false
	}
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return r.NumPage() > 0
}

// tryNewReader is pdf.NewReader with its own panic boundary, so one
// failed attempt can be followed by another repair attempt instead of
// unwinding all the way out of pdfOpen. The reader-construction path
// (NewReaderEncrypted -> readXref) panics rather than returning an error
// whenever the startxref offset does not land on a readable token — see
// pdfOpen's own doc comment.
func tryNewReader(data []byte) (r *pdf.Reader, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			r = nil
			err = fmt.Errorf("malformed PDF (panic while reading): %v", rec)
		}
	}()
	return pdf.NewReader(bytes.NewReader(data), int64(len(data)))
}

// repairStartxrefOffset returns a copy of data with a corrected trailer
// appended when the existing trailer's startxref offset does not point
// at a cross-reference section but a real "xref" table exists elsewhere
// in the file. Returns ok=false whenever the declared offset already
// looks right, no real table can be located, or the file has no trailing
// startxref at all — so a genuinely structureless file still fails
// instead of being silently "repaired" into something that looks valid.
//
// The original bytes are never rewritten in place: every xref entry in
// the file is a byte offset into it, so shifting anything would
// invalidate the very table being recovered. Appending a fresh
// "startxref <offset> %%EOF" block after the existing one is enough
// because the library reads the LAST startxref inside the file's final
// 100 bytes.
func repairStartxrefOffset(data []byte) ([]byte, bool) {
	window := eofScanWindow
	if window > len(data) {
		window = len(data)
	}
	start := len(data) - window
	idx := bytes.LastIndex(data[start:], []byte("startxref"))
	if idx < 0 {
		return nil, false
	}
	declared, declaredOK := parseOffsetAfter(data[start+idx+len("startxref"):])
	if declaredOK && startsXrefSection(data, declared) {
		return nil, false
	}
	actual, found := lastXrefTableOffset(data)
	if !found || (declaredOK && actual == declared) {
		return nil, false
	}
	repaired := make([]byte, 0, len(data)+32)
	repaired = append(repaired, data...)
	repaired = append(repaired, []byte(fmt.Sprintf("\r\nstartxref\r\n%d\r\n%%%%EOF\r\n", actual))...)
	return repaired, true
}

// parseOffsetAt reads the first decimal integer in data, skipping any
// leading PDF whitespace, and also reports the index just past its last
// digit so a caller can keep reading the tokens after it.
func parseOffsetAt(data []byte) (value int, next int, ok bool) {
	i := 0
	for i < len(data) && isPDFWhitespace(data[i]) {
		i++
	}
	digits := i
	for i < len(data) && data[i] >= '0' && data[i] <= '9' {
		i++
	}
	if i == digits {
		return 0, 0, false
	}
	n, err := strconv.Atoi(string(data[digits:i]))
	if err != nil {
		return 0, 0, false
	}
	return n, i, true
}

// parseOffsetAfter reads the first decimal integer in data, skipping any
// leading PDF whitespace.
func parseOffsetAfter(data []byte) (int, bool) {
	value, _, ok := parseOffsetAt(data)
	return value, ok
}

// startsXrefSection reports whether offset lands on something a
// cross-reference section can legally start with: the "xref" keyword of
// a classic table, or the object header of an xref stream ("12 0 obj").
func startsXrefSection(data []byte, offset int) bool {
	if offset < 0 || offset >= len(data) {
		return false
	}
	i := offset
	for i < len(data) && isPDFWhitespace(data[i]) {
		i++
	}
	if i >= len(data) {
		return false
	}
	if findPDFKeyword(data[i:min(i+4+1, len(data))], 0, "xref") == 0 {
		return true
	}
	return data[i] >= '0' && data[i] <= '9'
}

// lastXrefTableOffset returns the offset of the last standalone "xref"
// keyword that is actually followed by a subsection header ("0 13"), so
// a stray "xref" token inside a stream or a string cannot be mistaken
// for the table.
func lastXrefTableOffset(data []byte) (int, bool) {
	offset, found := 0, false
	for pos := 0; ; {
		idx := findPDFKeyword(data, pos, "xref")
		if idx < 0 {
			break
		}
		pos = idx + len("xref")
		// A real table's keyword is followed by a subsection header —
		// two integers, "<first object number> <count>". Anything else
		// is a coincidental token, not a table to point the trailer at.
		if _, next, ok := parseOffsetAt(data[pos:]); ok {
			if _, _, ok2 := parseOffsetAt(data[pos+next:]); ok2 {
				offset, found = idx, true
			}
		}
	}
	return offset, found
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
