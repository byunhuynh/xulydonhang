package pricing

import "testing"

func csvRows() [][]string {
	return [][]string{
		{"STT", "Mã hàng", "Tên hàng", "Giá"},
		{"1", "1234567", "Nước giặt", "141.272"},
		{"2", "7654321", "Chai tay", "50.000"},
	}
}

func TestParseIndex_FindPrice(t *testing.T) {
	idx := ParseIndex(csvRows())

	price, ok := idx.FindPrice("1234567")
	if !ok {
		t.Fatal("FindPrice(1234567) = not found, want found")
	}
	if price != "141272" {
		t.Fatalf("FindPrice(1234567) = %q, want %q", price, "141272")
	}

	if _, ok := idx.FindPrice("0000000"); ok {
		t.Fatal("FindPrice(0000000) = found, want not found")
	}
}

func TestParseIndex_FindPriceRequiresExactWhitespaceStrippedQuery(t *testing.T) {
	idx := ParseIndex(csvRows())

	// The Python original strips whitespace only from the query SKU,
	// never from the stored CSV column value — preserved here.
	price, ok := idx.FindPrice("  1234567  ")
	if !ok || price != "141272" {
		t.Fatalf("FindPrice(with whitespace) = (%q, %v), want (%q, true)", price, ok, "141272")
	}
}

func promotionCsvRows() [][]string {
	return [][]string{
		{"Mã hàng", "1/1-15/1", "16/1-31/1"},
		{"1234567", "", "Mua 2 tặng 1 (cf mua 2 tặng 1)"},
	}
}

func TestParseIndex_FindPromotionsWithinDateRange(t *testing.T) {
	idx := ParseIndex(promotionCsvRows())

	promos := idx.FindPromotions("1234567", "20/01/2026")
	if len(promos) != 1 {
		t.Fatalf("FindPromotions = %d promos, want 1: %+v", len(promos), promos)
	}
	if promos[0].Column != "16/1-31/1" {
		t.Fatalf("promo column = %q, want %q", promos[0].Column, "16/1-31/1")
	}

	none := idx.FindPromotions("1234567", "05/01/2026")
	if len(none) != 0 {
		t.Fatalf("FindPromotions outside range = %d promos, want 0", len(none))
	}
}
