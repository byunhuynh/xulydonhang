package satra

import "testing"

func TestParsePONumber_ExtractsBetweenAsterisks(t *testing.T) {
	text := "Header\n*P-005508192*\nmore text"
	got, ok := ParsePONumber(text)
	if !ok || got != "P-005508192" {
		t.Fatalf("ParsePONumber = (%q, %v), want (%q, true)", got, ok, "P-005508192")
	}
}

func TestParsePONumber_NoMatchReturnsFalse(t *testing.T) {
	if _, ok := ParsePONumber("no po number here"); ok {
		t.Fatal("ParsePONumber = matched, want no match")
	}
}

func TestParseEntryDate_UsesLineBeforeMarker(t *testing.T) {
	text := "some header\n08/13/2026\nNgày đặt hàng: 08/13/2026"
	got, ok := ParseEntryDate(text)
	if !ok || got != "13/08/2026" {
		t.Fatalf("ParseEntryDate = (%q, %v), want (%q, true)", got, ok, "13/08/2026")
	}
}

func TestParseEntryDate_FallsBackToNgayInWhenPlaceholderDate(t *testing.T) {
	// The PDF template literally renders "01/01/0001" when the "Ngày đặt
	// hàng" field is unset — Python detects this exact placeholder string
	// after formatting and retries against "Ngày in:" instead.
	text := "header\n01/01/0001\nNgày đặt hàng: 01/01/0001\nmore\n08/14/2026\nNgày in: 08/14/2026"
	got, ok := ParseEntryDate(text)
	if !ok || got != "14/08/2026" {
		t.Fatalf("ParseEntryDate (fallback) = (%q, %v), want (%q, true)", got, ok, "14/08/2026")
	}
}

func TestParseEntryDate_NoMatchReturnsFalse(t *testing.T) {
	if _, ok := ParseEntryDate("no date marker here"); ok {
		t.Fatal("ParseEntryDate = matched, want no match")
	}
}

func TestParseCancelDate_FindsFirstDateShapedLineInBlock(t *testing.T) {
	text := "Ngày giao hàng:\nKhẩn cấp\n08/20/2026\nĐịa chỉ giao hàng: 123 Đường ABC"
	got, ok := ParseCancelDate(text)
	if !ok || got != "20/08/2026" {
		t.Fatalf("ParseCancelDate = (%q, %v), want (%q, true)", got, ok, "20/08/2026")
	}
}

func TestParseCancelDate_NoMatchReturnsFalse(t *testing.T) {
	if _, ok := ParseCancelDate("no markers here"); ok {
		t.Fatal("ParseCancelDate = matched, want no match")
	}
}

func TestParseShipToAddress_JoinsLinesBetweenMarkers(t *testing.T) {
	text := "Địa chỉ giao hàng:\n123 Nguyễn Huệ\nPhường Bến Nghé\nĐịa chỉ thanh toán:\nsomewhere else"
	got, ok := ParseShipToAddress(text)
	if !ok {
		t.Fatal("ParseShipToAddress: no match, want match")
	}
	want := "123 Nguyễn Huệ Phường Bến Nghé"
	if got != want {
		t.Fatalf("ParseShipToAddress = %q, want %q", got, want)
	}
}

func TestParseShipToAddress_NoMatchReturnsFalse(t *testing.T) {
	if _, ok := ParseShipToAddress("no markers here"); ok {
		t.Fatal("ParseShipToAddress = matched, want no match")
	}
}

func TestExtractProducts_ParsesBarcodeAnchoredBlocks(t *testing.T) {
	// Shape mirrors trichxuatsanpham_satra's expectations: a line with
	// "N D" (STT + something) followed by a 13-digit barcode line, then
	// free-form lines, one of which is "N,000"-shaped (quantity), the
	// NEXT line being the total price, ending before "Tổng cộng".
	text := "1 1\n1234567890123\nSome Product Name\n5,000\n199,000,00\n" +
		"2 1\n9876543210987\nAnother Product\n3,000\n99,000,00\n" +
		"Tổng cộng\nfooter text"
	got := ExtractProducts(text)
	want := []Product{
		{Barcode: "1234567890123", Qty: 5, TotalPrice: 199000},
		{Barcode: "9876543210987", Qty: 3, TotalPrice: 99000},
	}
	if len(got) != len(want) {
		t.Fatalf("ExtractProducts returned %d products, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ExtractProducts()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestExtractProducts_SkipsZeroPriceLines(t *testing.T) {
	text := "1 1\n1234567890123\nFree Sample\n1,000\n0,00\nTổng cộng"
	got := ExtractProducts(text)
	if len(got) != 0 {
		t.Fatalf("ExtractProducts = %+v, want empty (zero-price line must be skipped)", got)
	}
}

func TestExtractProducts_NoBarcodeMatchesReturnsEmpty(t *testing.T) {
	if got := ExtractProducts("no product data here\nTổng cộng"); len(got) != 0 {
		t.Fatalf("ExtractProducts = %+v, want empty", got)
	}
}
