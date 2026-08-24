package processing

import (
	"os"
	"path/filepath"
	"testing"

	"order-processor/internal/processing/vendor"
)

func TestPdfOpen_TrimsGarbageAfterEOF(t *testing.T) {
	// Synthetic malformed PDF: a minimal valid single-page PDF (enough
	// structure for pdf.NewReader to succeed on the trimmed view) with
	// 120 bytes of non-whitespace garbage appended after its real
	// "%%EOF" — the same shape confirmed on all 17 real Emart PDFs
	// (đơn hàng/08-2026/*.PDF), which this fix was written to handle.
	// NOTE: includes a real xref table (with accurate byte offsets) unlike
	// a naive minimal PDF skeleton — pdf.NewReader requires an actual
	// cross-reference table to resolve objects; without one it fails with
	// "cross-reference table not found" regardless of the trailing-garbage
	// fix under test here. Verified this exact literal opens cleanly via
	// pdf.NewReader with no trailing garbage appended (i.e. the fixture
	// itself is a valid PDF), before appending garbage to exercise the
	// fallback path.
	minimalPDF := "%PDF-1.4\n" +
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n" +
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n" +
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj\n" +
		"xref\n0 4\n" +
		"0000000000 65535 f \n" +
		"0000000009 00000 n \n" +
		"0000000058 00000 n \n" +
		"0000000115 00000 n \n" +
		"trailer\n<< /Size 4 /Root 1 0 R >>\n" +
		"startxref\n186\n%%EOF\n"
	garbage := make([]byte, 120)
	for i := range garbage {
		garbage[i] = byte('A' + (i % 26))
	}

	path := filepath.Join(t.TempDir(), "garbage_after_eof.pdf")
	if err := os.WriteFile(path, append([]byte(minimalPDF), garbage...), 0o644); err != nil {
		t.Fatalf("failed writing synthetic PDF: %v", err)
	}

	f, r, err := pdfOpen(path)
	if err != nil {
		t.Fatalf("pdfOpen returned error for a PDF with trailing garbage after %%%%EOF: %v", err)
	}
	defer f.Close()
	if r == nil {
		t.Fatal("pdfOpen returned a nil reader with no error")
	}
	if r.NumPage() != 1 {
		t.Errorf("NumPage() = %d, want 1", r.NumPage())
	}
}

func TestPdfOpen_StillFailsOnGenuinelyMalformedFile(t *testing.T) {
	// A file with no "%%EOF" anywhere near its end at all — not the
	// trailing-garbage-after-a-real-EOF pattern this fix targets, so it
	// must still fail cleanly, not be silently "fixed" into something
	// that looks valid.
	path := filepath.Join(t.TempDir(), "not_a_pdf.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4\nthis is not a real PDF body at all, no xref, no EOF marker\n"), 0o644); err != nil {
		t.Fatalf("failed writing synthetic file: %v", err)
	}

	_, _, err := pdfOpen(path)
	if err == nil {
		t.Fatal("pdfOpen returned no error for a file with no EOF marker at all, want an error")
	}
}

func TestPdfOpen_OpensRealEmartPDFWithTrailingGarbage(t *testing.T) {
	// The real, confirmed case this fix was written for: a genuine
	// production PDF, not a synthetic one. Reads from this repo's own
	// stable, git-tracked emart/testdata/realpdfs/ directory (Task 5)
	// rather than the live đơn hàng/ tree every other vendor's tests
	// still depend on — that live folder was demonstrated mid-plan to be
	// an unstable dependency (reorganized by a live, concurrently-running
	// production instance of this same application), which is exactly
	// why this test now points here instead. Still skips gracefully if
	// even this stable path is somehow absent (e.g. a sparse checkout).
	path := filepath.Join("emart", "testdata", "realpdfs", "4501866956.pdf")
	if _, statErr := os.Stat(path); statErr != nil {
		t.Skipf("real sample PDF not found at %s: %v", path, statErr)
	}
	f, r, err := pdfOpen(path)
	if err != nil {
		t.Fatalf("pdfOpen returned error for real Emart PDF 4501866956.pdf: %v", err)
	}
	defer f.Close()
	if r.NumPage() != 1 {
		t.Errorf("NumPage() = %d, want 1", r.NumPage())
	}
}

func TestStripInlineImageData_RemovesPayloadKeepsLength(t *testing.T) {
	original := []byte("q 1 0 0 1 0 0 cm BI /W 2 /H 2 ID \x00\x01/weird\x02bytes\x03 EI Q")
	stripped := stripInlineImageData(append([]byte(nil), original...))

	if len(stripped) != len(original) {
		t.Fatalf("len(stripped) = %d, want %d (length must be preserved)", len(stripped), len(original))
	}
	// Everything up to and including "ID " must be untouched, and " EI Q" at
	// the end must be untouched — only the payload bytes in between change.
	prefix := []byte("q 1 0 0 1 0 0 cm BI /W 2 /H 2 ID ")
	if string(stripped[:len(prefix)]) != string(prefix) {
		t.Errorf("prefix changed: got %q, want %q", stripped[:len(prefix)], prefix)
	}
	suffix := []byte("EI Q")
	if string(stripped[len(stripped)-len(suffix):]) != string(suffix) {
		t.Errorf("suffix changed: got %q, want %q", stripped[len(stripped)-len(suffix):], suffix)
	}
	payload := stripped[len(prefix) : len(stripped)-len(suffix)-1] // -1 excludes the space before "EI"
	for i, b := range payload {
		if b != ' ' {
			t.Fatalf("payload byte %d = %q, want a space (stripped)", i, b)
		}
	}
}

func TestStripInlineImageData_NoOpWhenNoInlineImage(t *testing.T) {
	original := []byte("q 1 0 0 1 0 0 cm BT /F1 12 Tf (Hello) Tj ET Q")
	stripped := stripInlineImageData(append([]byte(nil), original...))
	if string(stripped) != string(original) {
		t.Errorf("stripInlineImageData changed a stream with no inline image:\ngot  %q\nwant %q", stripped, original)
	}
}

func TestPdfOpen_ExtractsTextFromRealEmartPDFsWithInlineImages(t *testing.T) {
	// The real, confirmed case this fix was written for: real Emart PDFs
	// embed an inline image in their content stream, which previously
	// made page.GetPlainText() fail outright. Reads every PDF present in
	// this repo's own stable, git-tracked emart/testdata/realpdfs/
	// directory (Task 5) — as of this writing that's 9 of the original
	// 17 real Emart PDFs found (the other 8 were still pending in a live
	// production instance's own processing queue when Task 5 ran); using
	// a glob instead of a hardcoded filename list means this test picks
	// up the remaining 8 automatically whenever they're added later, with
	// no test change required.
	dir := filepath.Join("emart", "testdata", "realpdfs")
	paths, err := filepath.Glob(filepath.Join(dir, "*.pdf"))
	if err != nil {
		t.Fatalf("failed globbing %s: %v", dir, err)
	}
	if len(paths) == 0 {
		t.Skipf("no real Emart PDFs found under %s", dir)
	}
	for _, path := range paths {
		name := filepath.Base(path)
		f, r, err := pdfOpen(path)
		if err != nil {
			t.Errorf("%s: pdfOpen returned error: %v", name, err)
			continue
		}
		page := r.Page(1)
		text, err := page.GetPlainText(nil)
		f.Close()
		if err != nil {
			t.Errorf("%s: page.GetPlainText returned error: %v", name, err)
			continue
		}
		if len(text) == 0 {
			t.Errorf("%s: page.GetPlainText returned empty text", name)
		}
	}
}

func TestPdfOpen_RepairsStartxrefPointingPastTheRealXrefTable(t *testing.T) {
	// The real archived Coop PDF this repair exists for: its trailer says
	// "startxref 9482", which is the file's own total length — 342 bytes
	// PAST the "xref" keyword that actually starts its cross-reference
	// table (offset 9140). The vendored library seeks straight to 9482,
	// reads EOF and panics out of pdf.NewReader, so before this repair
	// every attempt to process this file produced a Failed row with
	// "malformed PDF (panic while reading)". The table itself, the
	// trailer and every object in the file are intact — only that one
	// offset is wrong.
	path := filepath.Join("coop", "testdata", "realpdfs", "103229379-00.pdf")
	if _, statErr := os.Stat(path); statErr != nil {
		t.Skipf("real sample PDF not found at %s: %v", path, statErr)
	}
	f, r, err := pdfOpen(path)
	if err != nil {
		t.Fatalf("pdfOpen returned error for a PDF whose startxref points past its xref table: %v", err)
	}
	defer f.Close()
	if r == nil {
		t.Fatal("pdfOpen returned a nil reader with no error")
	}
	if r.NumPage() < 1 {
		t.Errorf("NumPage() = %d, want at least 1", r.NumPage())
	}
}

func TestPdfOpen_RecoversTextFromPlaceholderStreamLengths(t *testing.T) {
	// Same real file as above, one layer deeper: every content stream in
	// it declares "/Length 9 0 R" while object 9 is the placeholder 0 its
	// generator never patched, so a rebuilt cross-reference table alone
	// still yields a page whose text is empty — enough to open, not
	// enough to recognise the vendor. Measuring each stream's real extent
	// up to "endstream" is what makes this file processable at all.
	path := filepath.Join("coop", "testdata", "realpdfs", "103229379-00.pdf")
	if _, statErr := os.Stat(path); statErr != nil {
		t.Skipf("real sample PDF not found at %s: %v", path, statErr)
	}
	pages, _, err := extractPageTexts(path)
	if err != nil {
		t.Fatalf("extractPageTexts: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("no pages extracted")
	}
	if got := vendor.Identify(pages[0]); got != "Coop" {
		t.Fatalf("vendor.Identify = %q, want %q (extracted %d chars)", got, "Coop", len(pages[0]))
	}
}
