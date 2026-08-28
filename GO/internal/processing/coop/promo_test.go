package coop

import "testing"

func TestSplitPromoText(t *testing.T) {
	text := "cm Mua 2 tặng 1 (cf Mua 3 tặng 1)"
	if got := SplitPromoText(text, "COOPMART"); got != "cm Mua 2 tặng 1" {
		t.Fatalf("SplitPromoText(COOPMART) = %q, want %q", got, "cm Mua 2 tặng 1")
	}
	if got := SplitPromoText(text, "COOPFOOD"); got != "cf Mua 3 tặng 1" {
		t.Fatalf("SplitPromoText(COOPFOOD) = %q, want %q", got, "cf Mua 3 tặng 1")
	}
	if got := SplitPromoText(text, "OTHER"); got != text {
		t.Fatalf("SplitPromoText(OTHER) = %q, want unchanged", got)
	}
}

func TestSplitPromoText_NoCfMeansCmTakesWholeText(t *testing.T) {
	text := "cm Giảm 10% toàn bộ đơn hàng"
	if got := SplitPromoText(text, "COOPMART"); got != text {
		t.Fatalf("SplitPromoText(no cf) = %q, want unchanged %q", got, text)
	}
}

func TestExtractDiscount(t *testing.T) {
	if got := ExtractDiscount("Giảm 15% cho mã này"); got != 15 {
		t.Fatalf("ExtractDiscount = %v, want 15", got)
	}
	if got := ExtractDiscount("Giảm 12.5%"); got != 12.5 {
		t.Fatalf("ExtractDiscount = %v, want 12.5", got)
	}
	if got := ExtractDiscount("Không có giảm giá"); got != 0 {
		t.Fatalf("ExtractDiscount(none) = %v, want 0", got)
	}
}

func TestExtractBraceContent(t *testing.T) {
	if got := ExtractBraceContent("Mua 2 tặng 1 {KM Bó Kèm}"); got != "KM Bó Kèm" {
		t.Fatalf("ExtractBraceContent = %q, want %q", got, "KM Bó Kèm")
	}
	if got := ExtractBraceContent("không có ngoặc"); got != "" {
		t.Fatalf("ExtractBraceContent(none) = %q, want empty", got)
	}
}

func TestExtractMoneyAmount(t *testing.T) {
	cases := []struct {
		text string
		want int
		ok   bool
	}{
		{"Tặng quà khi mua trên 199k", 199000, true},
		{"Tặng quà khi mua trên 199 K", 199000, true},
		{"Tặng quà khi mua trên 150000 đồng", 150000, true},
		{"không có số tiền hợp lệ", 0, false},
	}
	for _, c := range cases {
		got, ok := ExtractMoneyAmount(c.text)
		if ok != c.ok || got != c.want {
			t.Fatalf("ExtractMoneyAmount(%q) = (%d, %v), want (%d, %v)", c.text, got, ok, c.want, c.ok)
		}
	}
}

func TestLastFourDigits(t *testing.T) {
	if got := LastFourDigits("SP0001234_extra"); got != "1234" {
		t.Fatalf("LastFourDigits = %q, want %q", got, "1234")
	}
	if got := LastFourDigits("ab"); got != "ab" {
		t.Fatalf("LastFourDigits(short) = %q, want %q", got, "ab")
	}
}

func TestFormatWeightKg(t *testing.T) {
	// Python's format_weight_kg does f"{round(value, 2)} kg" — and
	// Python's default float-to-str always shows a fractional part
	// (round(500, 2) is a float 500.0, str is "500.0", never "500").
	// Verified against real golden fixtures (Task 13), e.g.
	// "COOPFOOD PO103204622-00 (Tổng trọng lượng: 70.0 kg)".
	if got := FormatWeightKg(500); got != "500.0 kg" {
		t.Fatalf("FormatWeightKg(500) = %q, want %q", got, "500.0 kg")
	}
	if got := FormatWeightKg(1500); got != "1.5 tấn" {
		t.Fatalf("FormatWeightKg(1500) = %q, want %q", got, "1.5 tấn")
	}
	if got := FormatWeightKg(20.16); got != "20.16 kg" {
		t.Fatalf("FormatWeightKg(20.16) = %q, want %q", got, "20.16 kg")
	}
}

// --- Coopmart / Coopfood system scoping -------------------------------
//
// The Coop promo sheet encodes which of the two systems a CTKM belongs to
// at two levels, both taken from the real sheet snapshot in
// testdata/fixtures/_frozen_pricing.json:
//
//   - column level: the campaign name carries a standalone "CF"/"CM"
//     token, e.g. "CTKM CF", "CNMS 11+12 CM", "CNMS 14+15 Dành riêng CF";
//   - cell level: the promo text itself starts with "CF ..." / "CM ...",
//     e.g. "CF 1+1 | CF 2+1 tặng NLS 1L TP30565 {KM Giao Rời - Che Barcode}".
//
// A CTKM carrying no marker at all applies to BOTH systems, unchanged.

func TestSplitPromoText_CfOnlyCellDoesNotApplyToCoopmart(t *testing.T) {
	// Verbatim from the real sheet (column "CTKM CF").
	text := "CF 1+1 | CF 2+1 tặng NLS 1L TP30565  {KM Giao Rời - Che Barcode}"
	if got := SplitPromoText(text, "COOPFOOD"); got != text {
		t.Errorf("SplitPromoText(COOPFOOD) = %q, want the whole cell %q", got, text)
	}
	if got := SplitPromoText(text, "COOPMART"); got != "" {
		t.Errorf("SplitPromoText(COOPMART) = %q, want empty (a CF-only CTKM must not reach Coopmart)", got)
	}
}

func TestSplitPromoText_CmOnlyCellDoesNotApplyToCoopfood(t *testing.T) {
	text := "CM Giảm 40%"
	if got := SplitPromoText(text, "COOPMART"); got != text {
		t.Errorf("SplitPromoText(COOPMART) = %q, want the whole cell %q", got, text)
	}
	if got := SplitPromoText(text, "COOPFOOD"); got != "" {
		t.Errorf("SplitPromoText(COOPFOOD) = %q, want empty (a CM-only CTKM must not reach Coopfood)", got)
	}
}

func TestSplitPromoText_NoMarkerAppliesToBothSystems(t *testing.T) {
	text := "Mua 2 tặng 1 {KM Giao Rời}"
	if got := SplitPromoText(text, "COOPMART"); got != text {
		t.Errorf("SplitPromoText(COOPMART) = %q, want unchanged %q", got, text)
	}
	if got := SplitPromoText(text, "COOPFOOD"); got != text {
		t.Errorf("SplitPromoText(COOPFOOD) = %q, want unchanged %q", got, text)
	}
}

func TestSplitPromoText_MarkerMustBeAStandaloneToken(t *testing.T) {
	// "20cm" must not read as a Coopmart marker, or this CTKM would stop
	// applying to Coopfood.
	text := "Giảm 10% chai cao 20cm"
	if got := SplitPromoText(text, "COOPFOOD"); got != text {
		t.Errorf("SplitPromoText(COOPFOOD) = %q, want unchanged %q", got, text)
	}
	if got := SplitPromoText(text, "COOPMART"); got != text {
		t.Errorf("SplitPromoText(COOPMART) = %q, want unchanged %q", got, text)
	}
}

func TestColumnSystem(t *testing.T) {
	// Column names verbatim from the real sheet snapshot, newline already
	// normalised to a space by pricing.normalizeHeader.
	cases := []struct {
		column string
		want   string
	}{
		{"CTKM CF 17/07 30/09", "COOPFOOD"},
		{"CNMS CF 16/02-25/03", "COOPFOOD"},
		{"CNMS 14+15 Dành riêng CF 26/03-08/04", "COOPFOOD"},
		{"CNMS 11+12 CM 16/02-25/03", "COOPMART"},
		{"CTKM 02/07-05/08", ""},
		{"CNMS 40+41 10/09-07/10", ""},
		{"CTKM NLS + Viên Tẩy 06/08-23/09", ""},
		{"SĐBS 11/06-22/07", ""},
	}
	for _, c := range cases {
		if got := ColumnSystem(c.column); got != c.want {
			t.Errorf("ColumnSystem(%q) = %q, want %q", c.column, got, c.want)
		}
	}
}

func TestPromoForSystem_ColumnScopeWinsOverCellText(t *testing.T) {
	// A CF-only column excludes Coopmart even though the cell itself
	// carries no marker at all.
	if _, ok := PromoForSystem("CTKM CF 17/07-30/09", "Mua 2 tặng 1", "COOPMART"); ok {
		t.Error("PromoForSystem(CF column, COOPMART) applied, want skipped")
	}
	value, ok := PromoForSystem("CTKM CF 17/07-30/09", "Mua 2 tặng 1", "COOPFOOD")
	if !ok || value != "Mua 2 tặng 1" {
		t.Errorf("PromoForSystem(CF column, COOPFOOD) = (%q, %v), want (%q, true)", value, ok, "Mua 2 tặng 1")
	}
}

func TestPromoForSystem_UnmarkedColumnDefersToCellText(t *testing.T) {
	const cell = "CM Giảm 40% | CF giảm 30% Tặng NRC 2,1L TP30473"
	value, ok := PromoForSystem("CNMS 14+15 10/03-08/04", cell, "COOPFOOD")
	if !ok || value != "CF giảm 30% Tặng NRC 2,1L TP30473" {
		t.Errorf("PromoForSystem(COOPFOOD) = (%q, %v), want the CF half", value, ok)
	}
	value, ok = PromoForSystem("CNMS 14+15 10/03-08/04", cell, "COOPMART")
	if !ok || value != "CM Giảm 40% |" {
		t.Errorf("PromoForSystem(COOPMART) = (%q, %v), want the CM half", value, ok)
	}
}

func TestPromoForSystem_CellMarkerExcludesTheOtherSystem(t *testing.T) {
	const cell = "CF 1+1 | CF 2+1 tặng NLS 1L TP30565  {KM Giao Rời - Che Barcode}"
	if _, ok := PromoForSystem("CNMS 33+34 21/07-19/08", cell, "COOPMART"); ok {
		t.Error("PromoForSystem(CF-only cell, COOPMART) applied, want skipped")
	}
	value, ok := PromoForSystem("CNMS 33+34 21/07-19/08", cell, "COOPFOOD")
	if !ok || value != cell {
		t.Errorf("PromoForSystem(CF-only cell, COOPFOOD) = (%q, %v), want the whole cell", value, ok)
	}
}
