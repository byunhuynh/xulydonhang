package bigc

import "testing"

func TestParseOrderInfo_ExtractsPOAndEntryDate(t *testing.T) {
	// PO number is a 13+ digit number immediately followed by a
	// DD/MM/YY-shaped date; cancel date here comes from the region after
	// the LAST "Total Net Purchase Price" occurrence.
	text := "Header\n2631057733376 31/07/26\nsome content\nTotal Net Purchase Price\n04/08/26\nfooter"
	po, entry, cancel, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo: no match, want match")
	}
	if po != "2631057733376" {
		t.Fatalf("po = %q, want %q", po, "2631057733376")
	}
	if entry != "31/07/2026" {
		t.Fatalf("entry = %q, want %q", entry, "31/07/2026")
	}
	if cancel != "04/08/2026" {
		t.Fatalf("cancel = %q, want %q", cancel, "04/08/2026")
	}
}

func TestParseOrderInfo_UsesLastTotalNetPurchasePriceOccurrence(t *testing.T) {
	text := "2631057733376 31/07/26\n" +
		"Total Net Purchase Price\n01/01/26 (this one must be ignored)\n" +
		"more text\n" +
		"Total Net Purchase Price\n04/08/26 (this is the real one)"
	_, _, cancel, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo: no match, want match")
	}
	if cancel != "04/08/2026" {
		t.Fatalf("cancel = %q, want %q (must use the LAST occurrence)", cancel, "04/08/2026")
	}
}

func TestParseOrderInfo_FallsBackToEntryDatePlus5DaysWhenNoCancelDateFound(t *testing.T) {
	// No "Total Net Purchase Price" marker at all -> fallback fires.
	text := "2631057733376 31/07/26\nno marker anywhere in this text"
	po, entry, cancel, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo: no match, want match")
	}
	if po != "2631057733376" || entry != "31/07/2026" {
		t.Fatalf("po/entry = %q/%q, want %q/%q", po, entry, "2631057733376", "31/07/2026")
	}
	if cancel != "05/08/2026" {
		t.Fatalf("cancel (fallback) = %q, want %q (entry + 5 days)", cancel, "05/08/2026")
	}
}

func TestParseOrderInfo_NoMatchReturnsFalse(t *testing.T) {
	_, _, _, ok := ParseOrderInfo("no PO-shaped number and date pair here")
	if ok {
		t.Fatal("ParseOrderInfo: matched, want no match")
	}
}

func TestParseOrderInfo_WhitespaceIsCollapsedBeforeMatching(t *testing.T) {
	// Python collapses all whitespace runs to a single space before
	// matching (re.sub(r"\s+", " ", text)) — confirm the Go port does
	// the same, so a PO number split across a line break still matches.
	text := "2631057733376\n\n   31/07/26\nTotal Net Purchase Price\n\n04/08/26"
	po, entry, cancel, ok := ParseOrderInfo(text)
	if !ok || po != "2631057733376" || entry != "31/07/2026" || cancel != "04/08/2026" {
		t.Fatalf("ParseOrderInfo(whitespace-heavy) = (%q, %q, %q, %v), want (%q, %q, %q, true)",
			po, entry, cancel, ok, "2631057733376", "31/07/2026", "04/08/2026")
	}
}

func TestParseOrderInfo_HandlesNonBreakingSpaceBetweenPOAndDate(t *testing.T) {
	// Python's re module treats \s as Unicode-aware and matches non-breaking space (U+00A0).
	// Go's RE2 engine treats \s as ASCII-only, so ParseOrderInfo must normalize U+00A0
	// to regular space before regex processing. This test verifies the normalization works
	// with real non-breaking space characters from PDF extraction.
	// This is a confirmed artifact in this codebase's PDF-extracted text.

	// Use literal non-breaking space character (U+00A0 / \xa0) between PO number and date
	text := "Header\n2631057733376" + string(rune(0x00A0)) + "31/07/26\nsome content\nTotal Net Purchase Price\n04/08/26\nfooter"
	po, entry, cancel, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo with NBSP: no match, want match")
	}
	if po != "2631057733376" {
		t.Fatalf("po = %q, want %q", po, "2631057733376")
	}
	if entry != "31/07/2026" {
		t.Fatalf("entry = %q, want %q", entry, "31/07/2026")
	}
	if cancel != "04/08/2026" {
		t.Fatalf("cancel = %q, want %q", cancel, "04/08/2026")
	}
}
