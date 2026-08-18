package processing

import (
	"os"
	"path/filepath"
	"testing"
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
	// production PDF, not a synthetic one. Skips gracefully if the repo
	// layout this test expects isn't present (e.g. a CI checkout without
	// the đơn hàng folder), matching this project's established pattern
	// for tests that depend on real, large data files outside the repo's
	// own tracked test fixtures.
	path := filepath.Join("..", "..", "..", "đơn hàng", "08-2026", "4501866956.PDF")
	if _, statErr := os.Stat(path); statErr != nil {
		t.Skipf("real sample PDF not found at %s: %v", path, statErr)
	}
	f, r, err := pdfOpen(path)
	if err != nil {
		t.Fatalf("pdfOpen returned error for real Emart PDF 4501866956.PDF: %v", err)
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
	// The real, confirmed case this fix was written for. All 17 real
	// Emart PDFs embed an inline image in their content stream; 15 of
	// them previously failed page.GetPlainText() outright. Skips
	// gracefully if the repo layout this test expects isn't present,
	// matching this project's established pattern for tests depending on
	// real, large data files outside the repo's own tracked fixtures.
	dir := filepath.Join("..", "..", "..", "đơn hàng", "08-2026")
	names := []string{
		"4501866956.PDF", "4501866958.PDF", "4501873464.PDF", "4501873471.PDF",
		"4501873478.PDF", "4501875697.PDF", "4501875698.PDF", "4501875699.PDF",
		"4501878295.PDF", "4501880037.PDF", "4501880038.PDF", "4501880119.PDF",
		"4501880122.PDF", "4501880895.PDF", "4501880904.PDF", "4501880907.PDF",
		"4501881986.PDF",
	}
	if _, statErr := os.Stat(filepath.Join(dir, names[0])); statErr != nil {
		t.Skipf("real Emart PDF corpus not found at %s: %v", dir, statErr)
	}
	for _, name := range names {
		path := filepath.Join(dir, name)
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
