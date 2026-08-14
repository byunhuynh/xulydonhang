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
