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
