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
