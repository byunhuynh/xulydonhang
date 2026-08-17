package processing

import (
	"context"
	"testing"

	"github.com/xuri/excelize/v2"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

// TestWinmartRegionInfo covers winmartRegionInfo's 3 branches directly —
// none of these require a RealProcessor or real PDF, they're a pure
// function of customerCode. TestRealProcessor_ProcessesRealSampleWinmartFile
// only ever exercises the `default` branch (via the "Không xác định"
// fallback customer code), so the "MB"-prefix and "MN_MT_WIN1326"
// exact-match branches otherwise have zero coverage.
func TestWinmartRegionInfo(t *testing.T) {
	cases := []struct {
		name                                    string
		customerCode                            string
		wantRegion, wantStatCode, wantWarehouse string
	}{
		{
			name:          "MN_MT_WIN1326 exact match overrides MB/else",
			customerCode:  "MN_MT_WIN1326",
			wantRegion:    "MT_MN",
			wantStatCode:  "DN",
			wantWarehouse: "TP_DN_1",
		},
		{
			name:          "MB-prefixed code",
			customerCode:  "MB12345",
			wantRegion:    "MT_MB",
			wantStatCode:  "HN",
			wantWarehouse: "TP_HN_12",
		},
		{
			name:          "plain non-MB, non-special code (default branch)",
			customerCode:  "MN_MT_SOMEOTHER",
			wantRegion:    "MT_MN",
			wantStatCode:  "LA",
			wantWarehouse: "LA_KHO2026",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRegion, gotStatCode, gotWarehouse := winmartRegionInfo(tc.customerCode)
			if gotRegion != tc.wantRegion || gotStatCode != tc.wantStatCode || gotWarehouse != tc.wantWarehouse {
				t.Errorf("winmartRegionInfo(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.customerCode, gotRegion, gotStatCode, gotWarehouse,
					tc.wantRegion, tc.wantStatCode, tc.wantWarehouse)
			}
		})
	}
}

// TestRealProcessor_WinmartNoBraceBonusRowUsesGiaoRoiNote regression-tests
// the same bug already fixed for Lotte (see
// TestRealProcessor_LotteNoBraceBonusRowUsesGiaoRoiNote): processWinmartSegment
// reaches the shared buildPromoBonusRow helper whenever a matched Winmart
// promo yields a bonus SKU (an "X+1" match with a known bonus SKU
// mention), but that helper's no-{...}-brace fallback is Coop's own
// default ("KM Bó Kèm - Che Barcode", which also makes it write an AP
// bundle SKU on both rows). write_to_dondathang_winmart's own no-brace
// branch (xulydonhang.py:4477-4485's "else: sheet[f'AO{current_row}'] =
// 'KM Giao Rời - Không Che Barcode'") never writes AP at all — this is
// the exact text winmart_processor.go's call site now overrides to.
//
// sample_winmart_order.pdf's first real product is barcode
// "8936156731203" with OU Qty 4 and Total Price 649088 (confirmed by
// direct extraction during planning), so its per-unit invoice price is
// 649088/4 = 162272. Setting that as the real price with a "2+1 SP0002"
// promo (an "X+1" match mentioning SP0002, a known internal SKU already
// present in the productdata test fixture — see TestFindSkusMentioned)
// and NO {...} braces triggers the exact no-brace bonus-row path.
func TestRealProcessor_WinmartNoBraceBonusRowUsesGiaoRoiNote(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	const promoValue = "2+1 SP0002"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156731203", "Nước giặt", "162272", promoValue},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_winmart_order.pdf", 1); err != nil {
		t.Fatalf("Process returned error: %v", err)
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening written workbook: %v", err)
	}
	defer f.Close()
	sheetRows, err := f.GetRows("Don dat hang")
	if err != nil {
		t.Fatalf("failed reading Don dat hang rows: %v", err)
	}

	// Column indices (0-based) matching excelwriter's Q/AO/AP layout,
	// same as lotte_processor_test.go's equivalent regression test.
	const colSKU, colPromoNote, colPromoBundleSku = 16, 40, 41
	cell := func(row []string, idx int) string {
		if idx < len(row) {
			return row[idx]
		}
		return ""
	}

	var mainRow, bonusRow []string
	for _, row := range sheetRows {
		switch cell(row, colSKU) {
		case "8936156731203":
			mainRow = row
		case "SP0002":
			bonusRow = row
		}
	}
	if mainRow == nil || bonusRow == nil {
		t.Fatalf("missing expected rows: main=%v bonus=%v", mainRow, bonusRow)
	}

	if got := cell(mainRow, colPromoNote); got != "KM Giao Rời - Không Che Barcode" {
		t.Errorf("main row PromoNote (AO) = %q, want %q (Winmart's own no-brace fallback, not Coop's)", got, "KM Giao Rời - Không Che Barcode")
	}
	if got := cell(mainRow, colPromoBundleSku); got != "" {
		t.Errorf("main row PromoBundleSku (AP) = %q, want empty (Winmart's no-brace branch never writes AP)", got)
	}
	if got := cell(bonusRow, colPromoNote); got != "" {
		t.Errorf("bonus row PromoNote (AO) = %q, want empty (Winmart's no-brace branch never touches the bonus row's own AO)", got)
	}
	if got := cell(bonusRow, colPromoBundleSku); got != "" {
		t.Errorf("bonus row PromoBundleSku (AP) = %q, want empty (Winmart's no-brace branch never writes AP)", got)
	}
}

// TestRealProcessor_WinmartInvoiceLevelPromoBonusRow covers the
// invoice-level ("Hóa Đơn") promo bonus row processWinmartSegment builds
// inline (xulydonhang.py:4521-4562) — deliberately NOT via the shared
// buildInvoiceBonusRow helper, because Winmart's version writes only the
// FIRST matched SKU to column Q (xulydonhang.py:4537, kiemtra[0]), not a
// comma-joined list of every matched SKU. This asserts exactly that
// divergence, following the pattern of
// TestRealProcessor_BigcInvoiceLevelPromoBonusRow (bigc_processor_test.go).
//
// sample_winmart_order.pdf has two real products: barcode
// "8936156731203" (OU Qty 4, Total Price 649088 -> per-unit 162272) and
// "8936156732767" (OU Qty 12, Total Price 859200 -> per-unit 71600).
// Pricing both at their exact per-unit invoice price with NO promo
// column value means both match cleanly with no per-item bonus row (so
// the only bonus row present is the invoice-level one), giving a known
// totalValue of 162272*4 + 71600*12 = 1508288. A "Hóa Đơn" row mentioning
// both SP0001 and SP0002 (in that order) with a 100000 money amount then
// yields floor(1508288/100000) = 15 expected bonus units, attributed only
// to SP0001 (the first mentioned SKU).
func TestRealProcessor_WinmartInvoiceLevelPromoBonusRow(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156731203", "Nước giặt", "162272", ""},
		{"2", "8936156732767", "Nước rửa chén", "71600", ""},
		{"3", "Hóa Đơn", "", "0", "100000 SP0001 SP0002"},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_winmart_order.pdf", 1); err != nil {
		t.Fatalf("Process returned error: %v", err)
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening written workbook: %v", err)
	}
	defer f.Close()
	sheetRows, err := f.GetRows("Don dat hang")
	if err != nil {
		t.Fatalf("failed reading Don dat hang rows: %v", err)
	}

	const colSKU, colIsPromoItem, colQty, colPromoNote = 16, 20, 23, 40
	cell := func(row []string, idx int) string {
		if idx < len(row) {
			return row[idx]
		}
		return ""
	}

	var bonusRow []string
	for _, row := range sheetRows {
		if cell(row, colSKU) == "SP0001" {
			bonusRow = row
			break
		}
	}
	if bonusRow == nil {
		t.Fatalf("no row with SKU (Q) = %q found; sheet rows: %+v", "SP0001", sheetRows)
	}

	if got := cell(bonusRow, colSKU); got != "SP0001" {
		t.Errorf("invoice bonus row SKU (Q) = %q, want %q (kiemtra[0] — the FIRST mentioned SKU only, not a joined list)", got, "SP0001")
	}
	if got := cell(bonusRow, colIsPromoItem); got != "Có" {
		t.Errorf("invoice bonus row IsPromoItem (U) = %q, want %q", got, "Có")
	}
	if got := cell(bonusRow, colQty); got != "15" {
		t.Errorf("invoice bonus row Qty (X) = %q, want %q (floor(totalValue=1508288 / amount=100000))", got, "15")
	}
	if got := cell(bonusRow, colPromoNote); got != "KM Bó Kèm - Che Barcode" {
		t.Errorf("invoice bonus row PromoNote (AO) = %q, want %q (the invoice-level block's own fallback, unaffected by this plan's per-item fix)", got, "KM Bó Kèm - Che Barcode")
	}

	// Make sure no OTHER row also carries SKU "SP0002" as its own bonus
	// row — the whole point of this test is that only the first mentioned
	// SKU gets a bonus row, not a joined list nor a second row per SKU.
	for _, row := range sheetRows {
		if cell(row, colSKU) == "SP0002" {
			t.Errorf("found a row with SKU (Q) = %q, want none (only the first mentioned SKU, SP0001, should get an invoice bonus row)", "SP0002")
		}
	}
}

// TestWinmartZeroPriceSkip directly covers winmartZeroPriceSkip's
// len(rows) threshold with synthetic rows — no PDF or regex parsing
// needed, since the function is a pure transform on []excelwriter.Row.
// This is the dedicated test for both branches of the zero-price "giao
// rời" skip (this plan's design decision, see winmartZeroPriceSkip's own
// doc comment): the skip-cleanly branch (no same-order row to mark) and
// the mark-a-prior-row branch, including the len(rows) == 2 case (marks
// the order's own header row) that the real 12-fixture golden corpus
// never happens to exercise.
func TestWinmartZeroPriceSkip(t *testing.T) {
	t.Run("no rows: skips cleanly, no panic", func(t *testing.T) {
		rows := []excelwriter.Row{}
		if got := winmartZeroPriceSkip(rows); got != false {
			t.Errorf("winmartZeroPriceSkip(empty) = %v, want false", got)
		}
	})

	t.Run("only header row: skips cleanly, no cross-order reach", func(t *testing.T) {
		rows := []excelwriter.Row{
			{ProductName: "header"},
		}
		if got := winmartZeroPriceSkip(rows); got != false {
			t.Errorf("winmartZeroPriceSkip(len 1) = %v, want false", got)
		}
		if rows[0].PromoNote != "" {
			t.Errorf("header row PromoNote = %q, want untouched (empty) — marking it here would be a cross-order reach", rows[0].PromoNote)
		}
	})

	t.Run("header + one product, no bonus row yet: marks the header row itself", func(t *testing.T) {
		rows := []excelwriter.Row{
			{ProductName: "header"},
			{SKU: "PROD1", PromoBundleSku: "shouldbeCleared"},
		}
		if got := winmartZeroPriceSkip(rows); got != true {
			t.Errorf("winmartZeroPriceSkip(len 2) = %v, want true", got)
		}
		if rows[0].PromoNote != "KM Giao Rời - Không Che" {
			t.Errorf("header row (rows[0]) PromoNote = %q, want %q", rows[0].PromoNote, "KM Giao Rời - Không Che")
		}
		if rows[0].PromoBundleSku != "" {
			t.Errorf("header row (rows[0]) PromoBundleSku = %q, want empty", rows[0].PromoBundleSku)
		}
		if rows[1].PromoBundleSku != "" {
			t.Errorf("product row (rows[1]) PromoBundleSku = %q, want cleared to empty", rows[1].PromoBundleSku)
		}
	})

	t.Run("header + 2+ products: marks the last two accumulated rows, not the header", func(t *testing.T) {
		rows := []excelwriter.Row{
			{ProductName: "header"},
			{SKU: "PROD1"},
			{SKU: "PROD2", PromoBundleSku: "shouldbeCleared"},
		}
		if got := winmartZeroPriceSkip(rows); got != true {
			t.Errorf("winmartZeroPriceSkip(len 3) = %v, want true", got)
		}
		if rows[0].PromoNote != "" {
			t.Errorf("header row (rows[0]) PromoNote = %q, want untouched (empty) — only the last 2 rows should be marked", rows[0].PromoNote)
		}
		if rows[1].PromoNote != "KM Giao Rời - Không Che" {
			t.Errorf("rows[1] PromoNote = %q, want %q", rows[1].PromoNote, "KM Giao Rời - Không Che")
		}
		if rows[2].PromoBundleSku != "" {
			t.Errorf("rows[2] PromoBundleSku = %q, want cleared to empty", rows[2].PromoBundleSku)
		}
	})
}
