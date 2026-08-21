package processing

import (
	"strings"
	"testing"
)

func TestFormatSkuLogLine_MatchedNoPromo(t *testing.T) {
	got := formatSkuLogLine("8936156730886", "Cà phê G7 3in1", true, 133806, 133806, "", "")
	want := "8936156730886 Cà phê G7 3in1 — Đúng giá"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatSkuLogLine_MatchedWithPromo(t *testing.T) {
	got := formatSkuLogLine("8936156730886", "Cà phê G7 3in1", true, 133806, 133806, "Mua 1 tặng 1", "1/1-31/12")
	want := "8936156730886 Cà phê G7 3in1 — Đúng giá, KM: Mua 1 tặng 1 (áp dụng 1/1-31/12)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatSkuLogLine_MatchedWithPromoButNoDateRange(t *testing.T) {
	// promoDateRange can legitimately be "" (e.g. a caller that hasn't
	// threaded pricing.Promotion.Column through yet) — must not print a
	// dangling "(áp dụng )".
	got := formatSkuLogLine("SP0001", "", true, 1000, 1000, "Mua 1 tặng 1", "")
	want := "SP0001 — Đúng giá, KM: Mua 1 tặng 1"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatSkuLogLine_MismatchNoPromo(t *testing.T) {
	got := formatSkuLogLine("SP0005", "Sữa tươi", false, 133806, 120000, "", "")
	want := "SP0005 Sữa tươi — ⚠️ SAI GIÁ! Giá đúng: 120000, Giá trên PO: 133806"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatSkuLogLine_MismatchWithPromo(t *testing.T) {
	got := formatSkuLogLine("SP0005", "Sữa tươi", false, 133806, 120000, "Giảm 10%", "15/8-20/8")
	want := "SP0005 Sữa tươi — ⚠️ SAI GIÁ! Giá đúng: 120000, Giá trên PO: 133806, đã thử KM: Giảm 10% (áp dụng 15/8-20/8)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatSkuLogLine_NoProductNameFallsBackToSkuOnly(t *testing.T) {
	got := formatSkuLogLine("SP0009", "", true, 1000, 1000, "", "")
	want := "SP0009 — Đúng giá"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTruncatePromoText_ShortTextPassesThroughUnchanged(t *testing.T) {
	got := truncatePromoText("Mua 1 tặng 1")
	if got != "Mua 1 tặng 1" {
		t.Fatalf("got %q, want unchanged", got)
	}
}

func TestTruncatePromoText_CollapsesMultilineToOneLine(t *testing.T) {
	got := truncatePromoText("KM Bó Kèm\nChi tiết: xem hóa đơn")
	want := "KM Bó Kèm Chi tiết: xem hóa đơn"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTruncatePromoText_LongTextTruncatesAtRuneBoundaryNotByteBoundary(t *testing.T) {
	// 70 repetitions of "ệ" (a 3-byte UTF-8 rune) — a byte-based
	// truncation to 60 bytes would land mid-character and corrupt the
	// string; a rune-based truncation to 60 runes must not.
	long := strings.Repeat("ệ", 70)
	got := truncatePromoText(long)
	wantRunes := strings.Repeat("ệ", skuLogPromoMaxLen) + "..."
	if got != wantRunes {
		t.Fatalf("got %q (valid utf8: %v), want %q", got, isValidUTF8(got), wantRunes)
	}
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}
