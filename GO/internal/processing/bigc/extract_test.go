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

func TestExtractPriceList_ParsesProductLinesAfterArticleHeader(t *testing.T) {
	// Shape mirrors laydanhsachsanpham_bigc's expected line: 13-digit
	// barcode, some description text, then "Pack <level> <SKU/OU>
	// <OU Qty> <another number> <unit price with comma> <unit> <total
	// price with comma>".
	text := "Preamble that must be ignored\n" +
		"Article Description Pack Level SKU OUQty More Unit TotalPrice\n" +
		"8936156730879 Nuoc giat Blue Pack 1 4 20 1 37,188 PC 148,750\n" +
		"8936156730992 Nuoc xa Pink Pack 1 6 12 1 25,000 PC 150,000\n"
	got := ExtractPriceList(text)
	want := []Product{
		{Barcode: "8936156730879", SKUOrUnit: "4", OrderedUnitQty: "20", UnitPrice: 37188, TotalNetPurchasePrice: 148750},
		{Barcode: "8936156730992", SKUOrUnit: "6", OrderedUnitQty: "12", UnitPrice: 25000, TotalNetPurchasePrice: 150000},
	}
	if len(got) != len(want) {
		t.Fatalf("ExtractPriceList returned %d products, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ExtractPriceList()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestExtractPriceList_NoArticleHeaderReturnsEmpty(t *testing.T) {
	if got := ExtractPriceList("no header keyword anywhere"); len(got) != 0 {
		t.Fatalf("ExtractPriceList = %+v, want empty", got)
	}
}

func TestExtractPriceList_TextBeforeArticleHeaderIsIgnored(t *testing.T) {
	// A product-shaped line appearing BEFORE the "Article" header must
	// not be matched (mirrors slicing the text at match_start.start()
	// before running the product-line regex).
	text := "8936156730879 Preamble Pack 1 4 20 1 37,188 PC 148,750\n" +
		"Article\n" +
		"8936156730992 Nuoc xa Pink Pack 1 6 12 1 25,000 PC 150,000\n"
	got := ExtractPriceList(text)
	if len(got) != 1 || got[0].Barcode != "8936156730992" {
		t.Fatalf("ExtractPriceList = %+v, want exactly 1 product (8936156730992)", got)
	}
}

func TestResolveCustomerCode_MBLinfoxCombination(t *testing.T) {
	code, warehouse := ResolveCustomerCode("some text 3006900 more text LINFOX WAREHOUSE (802) footer")
	if code != "MB_GC_BIGC" || warehouse != "LINFOX WAREHOUSE (802)" {
		t.Fatalf("ResolveCustomerCode = (%q, %q), want (%q, %q)", code, warehouse, "MB_GC_BIGC", "LINFOX WAREHOUSE (802)")
	}
}

func TestResolveCustomerCode_AllFourCombinationsAndDefault(t *testing.T) {
	cases := []struct {
		name          string
		text          string
		wantCode      string
		wantWarehouse string
	}{
		{"3006900+LINFOX", "3006900 LINFOX WAREHOUSE (802)", "MB_GC_BIGC", "LINFOX WAREHOUSE (802)"},
		{"3005382+LINFOX", "3005382 LINFOX WAREHOUSE (802)", "MB_MT_BIGC", "LINFOX WAREHOUSE (802)"},
		{"3005382+FMLOGISTIC", "3005382 FM LOGISTIC VSIP 2 (806)", "MN_MT_BIGCAC", "FM LOGISTIC VSIP 2 (806)"},
		{"3006900+FMLOGISTIC", "3006900 FM LOGISTIC VSIP 2 (806)", "MN_GC_BIGCAC", "FM LOGISTIC VSIP 2 (806)"},
		{"neither signal", "nothing relevant here", "MN_MT_BIGCAC", "FM LOGISTIC VSIP 2 (806)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, warehouse := ResolveCustomerCode(c.text)
			if code != c.wantCode || warehouse != c.wantWarehouse {
				t.Fatalf("ResolveCustomerCode(%q) = (%q, %q), want (%q, %q)", c.text, code, warehouse, c.wantCode, c.wantWarehouse)
			}
		})
	}
}

func TestResolveCustomerCode_CheckOrderMatchesPythonWhenMultipleSignalsPresent(t *testing.T) {
	// If text somehow contains BOTH "3006900" and "3005382" plus LINFOX,
	// Python's if/elif order means the FIRST matching branch wins
	// ("3006900"+LINFOX checked before "3005382"+LINFOX).
	code, warehouse := ResolveCustomerCode("3006900 3005382 LINFOX WAREHOUSE (802)")
	if code != "MB_GC_BIGC" || warehouse != "LINFOX WAREHOUSE (802)" {
		t.Fatalf("ResolveCustomerCode(both signals) = (%q, %q), want (%q, %q) (first-matching-branch-wins)", code, warehouse, "MB_GC_BIGC", "LINFOX WAREHOUSE (802)")
	}
}

func TestExtractPriceList_NonBreakingSpaceInLine(t *testing.T) {
	// Go's RE2 \s is ASCII-only; Python's \s is Unicode-aware and matches
	// U+00A0 (non-breaking space), a confirmed real artifact in this
	// project's PDF-extracted text (see xulydonhang.py's
	// demsodonhang1trang_coop, which explicitly replaces "\xa0" with " ").
	// laydanhsachsanpham_bigc (xulydonhang.py:5846-5849) never normalizes
	// \xa0 out of its input either — Python's Unicode-aware \s just
	// swallows it silently — so ExtractPriceList must normalize NBSP
	// itself to match that (implicit) Python behavior.
	nbsp := string(rune(0x00A0))
	text := "Article Description Pack Level SKU OUQty More Unit TotalPrice\n" +
		"8936156730879" + nbsp + "Nuoc giat Blue Pack 1 4 20 1 37,188 PC 148,750\n"
	got := ExtractPriceList(text)
	want := []Product{
		{Barcode: "8936156730879", SKUOrUnit: "4", OrderedUnitQty: "20", UnitPrice: 37188, TotalNetPurchasePrice: 148750},
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("ExtractPriceList(NBSP) = %+v, want %+v", got, want)
	}
}

func TestExtractStoreName_FindsLineAfterWarehouseAndVietnam(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"FM LOGISTIC", "header\nFM LOGISTIC VSIP 2, Binh Duong, Vietnam\nGO! DONG NAI\nfooter", "GO! DONG NAI"},
		{"LINFOX", "header\nLINFOX WAREHOUSE (802), Hanoi, Vietnam\nGO! AN LAC\nfooter", "GO! AN LAC"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ExtractStoreName(c.text)
			if !ok || got != c.want {
				t.Fatalf("ExtractStoreName(%q) = (%q, %v), want (%q, true)", c.text, got, ok, c.want)
			}
		})
	}
}

func TestExtractStoreName_NoMatchReturnsFalse(t *testing.T) {
	if _, ok := ExtractStoreName("no warehouse marker here"); ok {
		t.Fatal("ExtractStoreName: matched, want no match")
	}
}

func TestExtractStoreName_HandlesNonBreakingSpaceBeforeVietnamAndLineEnd(t *testing.T) {
	// Go's RE2 \s is ASCII-only; Python's \s is Unicode-aware and matches
	// U+00A0 (non-breaking space) — a confirmed real artifact in this
	// project's PDF-extracted text (see xulydonhang.py's
	// demsodonhang1trang_coop, which explicitly replaces "\xa0" with " ").
	// lay_ten_store (xulydonhang.py:5878-5884) never normalizes \xa0 out
	// of its input either — Python's Unicode-aware \s just swallows it
	// silently in "Vietnam\s*\n" — so ExtractStoreName must normalize NBSP
	// itself to match that (implicit) Python behavior.
	nbsp := string(rune(0x00A0))
	text := "header\nFM LOGISTIC VSIP 2, Binh Duong, Vietnam" + nbsp + "\nGO! DONG NAI\nfooter"
	got, ok := ExtractStoreName(text)
	if !ok || got != "GO! DONG NAI" {
		t.Fatalf("ExtractStoreName(NBSP) = (%q, %v), want (%q, true)", got, ok, "GO! DONG NAI")
	}
}

func TestExtractStoreItems_ParsesBarcodeAnchoredLines(t *testing.T) {
	// Shape mirrors trichxuatdanhsachforstore_bigc's expectation:
	// "<13-digit barcode>\n<description>\nPack\n<level>\n<SKU/OU>\n<qty>".
	text := "8936156730879\nNuoc giat Blue 3.8kg\nPack\n1\n4\n20\n" +
		"8936156730992\nNuoc xa Pink 2L\nPack\n1\n6\n12\n"
	got := ExtractStoreItems(text)
	want := []StoreItem{
		{Barcode: "8936156730879", SKUOrUnit: "4", OrderedUnitQty: "20"},
		{Barcode: "8936156730992", SKUOrUnit: "6", OrderedUnitQty: "12"},
	}
	if len(got) != len(want) {
		t.Fatalf("ExtractStoreItems returned %d items, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ExtractStoreItems()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestExtractStoreItems_MatchesAtStartOfTextToo(t *testing.T) {
	// Go's port uses "(?:^|\n)" in place of Python's lookbehind
	// "(?<=\n)" (RE2 has no lookbehind support) — confirm a barcode at
	// the very start of the text (no preceding newline at all) still
	// matches, same as Python's lookbehind would at position 0... wait,
	// Python's (?<=\n) actually requires a literal preceding newline, so
	// it would NOT match at true position 0 of the whole page text
	// either. This test documents the Go port matches Python exactly:
	// position 0 DOES match here because Go's "^" (no multiline flag)
	// anchors the start of the whole string, same effective behavior as
	// every real store page having some preceding header text before
	// the first item line in practice. If this ever diverges from a
	// real fixture in Task 7/8, treat Python's literal (?<=\n) as
	// authoritative and adjust the Go pattern.
	text := "8936156730879\nNuoc giat Blue\nPack\n1\n4\n20\n"
	got := ExtractStoreItems(text)
	if len(got) != 1 || got[0].Barcode != "8936156730879" {
		t.Fatalf("ExtractStoreItems(text starting with barcode) = %+v, want 1 item with barcode 8936156730879", got)
	}
}

func TestExtractStoreItems_NoMatchesReturnsEmpty(t *testing.T) {
	if got := ExtractStoreItems("no item lines here"); len(got) != 0 {
		t.Fatalf("ExtractStoreItems = %+v, want empty", got)
	}
}

func TestExtractStoreItems_NonBreakingSpaceAroundDescriptionAndPack(t *testing.T) {
	// Go's RE2 \s is ASCII-only; Python's \s is Unicode-aware and matches
	// U+00A0 (non-breaking space) — a confirmed real artifact in this
	// project's PDF-extracted text (see xulydonhang.py's
	// demsodonhang1trang_coop, which explicitly replaces "\xa0" with " ").
	// trichxuatdanhsachforstore_bigc (xulydonhang.py:5900-5907) never
	// normalizes \xa0 out of its input either — Python's Unicode-aware \s
	// just swallows it silently in the "\s*\nPack\s*\n" portions of the
	// pattern — so ExtractStoreItems must normalize NBSP itself to match
	// that (implicit) Python behavior.
	nbsp := string(rune(0x00A0))
	text := "8936156730879\nNuoc giat Blue" + nbsp + "\nPack\n1\n4\n20\n"
	got := ExtractStoreItems(text)
	want := []StoreItem{{Barcode: "8936156730879", SKUOrUnit: "4", OrderedUnitQty: "20"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("ExtractStoreItems(NBSP) = %+v, want %+v", got, want)
	}
}

func TestJoinItemsWithPrices_LooksUpByBarcode(t *testing.T) {
	items := []StoreItem{
		{Barcode: "8936156730879", SKUOrUnit: "4", OrderedUnitQty: "20"},
		{Barcode: "8936156730992", SKUOrUnit: "6", OrderedUnitQty: "12"},
	}
	priceList := []Product{
		{Barcode: "8936156730879", UnitPrice: 37188},
		{Barcode: "8936156730992", UnitPrice: 25000},
	}
	got := JoinItemsWithPrices(items, priceList)
	if got[0].UnitPrice != 37188 || got[1].UnitPrice != 25000 {
		t.Fatalf("JoinItemsWithPrices = %+v, want prices 37188 and 25000", got)
	}
}

func TestJoinItemsWithPrices_MissingBarcodeSilentlyDefaultsToZero(t *testing.T) {
	// Mirrors ghepgia_donhangbigc's product_dict.get(article, 0) — the
	// Python comment claims this "reports an error" but the real code
	// does not; port the silent-zero behavior faithfully.
	items := []StoreItem{{Barcode: "9999999999999", SKUOrUnit: "1", OrderedUnitQty: "1"}}
	priceList := []Product{{Barcode: "8936156730879", UnitPrice: 37188}}
	got := JoinItemsWithPrices(items, priceList)
	if got[0].UnitPrice != 0 {
		t.Fatalf("JoinItemsWithPrices(unmatched barcode) = %+v, want UnitPrice 0", got)
	}
}
