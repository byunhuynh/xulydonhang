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
