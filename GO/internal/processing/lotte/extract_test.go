package lotte

import "testing"

func TestParseOrderInfo_ParsesPageSecondLine(t *testing.T) {
	// Real sample from đơn hàng/08-2026/260727-01013-00057.pdf, page 1:
	// line 0 is "Ord sheet", line 1 is the raw 16-digit PO string.
	text := "Ord sheet\n2607270101300057\nPage\n:\n1 / 1"
	got, err := ParseOrderInfo(text)
	if err != nil {
		t.Fatalf("ParseOrderInfo returned error: %v", err)
	}
	want := OrderInfo{PONumber: "260727-01013-00057", EntryDate: "27/07/2026", StoreCode: "01013"}
	if got != want {
		t.Fatalf("ParseOrderInfo = %+v, want %+v", got, want)
	}
}

func TestParseOrderInfo_TooFewLinesReturnsError(t *testing.T) {
	if _, err := ParseOrderInfo("only one line, no second line at all"); err == nil {
		t.Fatal("expected error for text with fewer than 2 lines, got nil")
	}
}

func TestParseOrderInfo_MalformedSecondLineReturnsError(t *testing.T) {
	if _, err := ParseOrderInfo("header\nnotadigitstring"); err == nil {
		t.Fatal("expected error for a non-numeric second line, got nil")
	}
}

func TestExtractCancelDate_KeepsOnlyDateShapedLines(t *testing.T) {
	// Real shape from 260727-01013-00057.pdf: the line starting with the
	// PO number is followed by a priority label, then the cancel date,
	// then "00:00".
	text := "before\n260727-01013-00057 Khẩn cấp\n30/07/2026\n00:00\nafter"
	got := ExtractCancelDate(text, "260727-01013-00057")
	if got != "30/07/2026" {
		t.Fatalf("ExtractCancelDate = %q, want %q", got, "30/07/2026")
	}
}

func TestExtractCancelDate_NoMarkersReturnsEmpty(t *testing.T) {
	if got := ExtractCancelDate("no markers here", "260727-01013-00057"); got != "" {
		t.Fatalf("ExtractCancelDate = %q, want empty", got)
	}
}

func TestExtractStoreName_ReturnsLastLineBeforePoNumber(t *testing.T) {
	// Real shape from 260727-01013-00057.pdf.
	text := "DOAN TUAN ANH\n0304741634-011\nCONG TY CP TRUNG TAM\nTHUONG MAI LOTTE VIET\nNha trang\n260727-01013-00057\nKhẩn cấp"
	got := ExtractStoreName(text, "260727-01013-00057")
	if got != "Nha trang" {
		t.Fatalf("ExtractStoreName = %q, want %q", got, "Nha trang")
	}
}

func TestExtractStoreName_AdjacentMarkersReturnsAnchorLine(t *testing.T) {
	// When the anchor and the PO-number line are directly adjacent (no
	// lines between them), Python's lines[end_index-1] resolves to the
	// anchor line itself, not to "" — this asserts that exact edge-case
	// behavior, not a simplified "empty" result.
	text := "DOAN TUAN ANH\n260727-01013-00057"
	got := ExtractStoreName(text, "260727-01013-00057")
	if got != "DOAN TUAN ANH" {
		t.Fatalf("ExtractStoreName (adjacent markers) = %q, want %q", got, "DOAN TUAN ANH")
	}
}

func TestExtractStoreName_NoMarkersReturnsEmpty(t *testing.T) {
	if got := ExtractStoreName("no markers here", "260727-01013-00057"); got != "" {
		t.Fatalf("ExtractStoreName = %q, want empty", got)
	}
}
