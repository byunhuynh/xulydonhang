package kingfood

import "testing"

func TestParseOrderInfo_ExtractsRealSampleFields(t *testing.T) {
	// Text shape mirrors this repo's OWN extractPageTexts output against
	// a real sample Kingfood PDF (confirmed during planning by running
	// the actual Go PDF pipeline directly, then cross-checked against
	// PyMuPDF's output on the SAME file) — including the tab characters
	// Go's extraction inserts between words in multi-word labels, where
	// PyMuPDF inserts plain spaces. \t below is a literal tab character.
	text := "\n" +
		"Page\t1\t/\t2\n" +
		"PO\tNumber:\n" +
		"PO1002601888\n" +
		"Nơi\tgiao:\n" +
		"KHO\tSEEDLOG\n" +
		"Ngày\tGiao\tHàng\tDự\tKiến:\n" +
		"05-08-2026\n" +
		"Ngày\tGiao\tHàng\tNCC\tXác\n" +
		"Nhận:\n" +
		"05-08-2026\n" +
		"Ngày\tĐặt\tHàng:\n" +
		"03-08-2026\n" +
		"Quá\tcảnh:\n"

	poNumber, entryDate, cancelDate, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if poNumber != "PO1002601888" {
		t.Errorf("poNumber = %q, want %q", poNumber, "PO1002601888")
	}
	if entryDate != "03/08/2026" {
		t.Errorf("entryDate = %q, want %q", entryDate, "03/08/2026")
	}
	if cancelDate != "05/08/2026" {
		t.Errorf("cancelDate = %q, want %q", cancelDate, "05/08/2026")
	}
}

func TestParseOrderInfo_MissingPONumberMarkerFailsCleanly(t *testing.T) {
	// No "PO Number:" marker anywhere -> poNumber resolves empty ->
	// ok=false. Mirrors Python's real crash risk here (a downstream
	// datetime.strptime on an unresolved/garbage date string would raise
	// ValueError, uncaught) with a clean failure instead, per this
	// codebase's established policy — Kingfood has NO cross-validate/
	// fallback logic to backfill a missing date (unlike FujiMart/
	// Winmart/Emart), so a single missing marker is unrecoverable.
	_, _, _, ok := ParseOrderInfo("nothing relevant here\nno markers at all\n")
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for text with no markers, want false")
	}
}

func TestParseOrderInfo_MalformedDateFailsCleanly(t *testing.T) {
	// A date that doesn't match dd-mm-yyyy should fail cleanly rather
	// than reproducing Python's real datetime.strptime crash.
	text := "PO\tNumber:\n" +
		"PO1002601888\n" +
		"Ngày\tGiao\tHàng\tNCC\tXác\n" +
		"Nhận:\n" +
		"not-a-date\n" +
		"Ngày\tĐặt\tHàng:\n" +
		"03-08-2026\n"
	_, _, _, ok := ParseOrderInfo(text)
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for a malformed cancelDate, want false")
	}
}

func TestNormalizeTabs_ReplacesTabsWithSpaces(t *testing.T) {
	got := normalizeTabs("PO\tNumber:\nKHO\tSEEDLOG")
	want := "PO Number:\nKHO SEEDLOG"
	if got != want {
		t.Errorf("normalizeTabs(...) = %q, want %q", got, want)
	}
}
