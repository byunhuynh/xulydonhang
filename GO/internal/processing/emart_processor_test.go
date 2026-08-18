package processing

import (
	"context"
	"testing"

	"github.com/xuri/excelize/v2"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleEmartFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	pricingSource := &fixturePricingSource{index: pricing.ParseIndex([][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
	})}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_emart_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].System != "Emart" {
		t.Fatalf("System = %q, want %q", rows[0].System, "Emart")
	}
	if rows[0].StatusKind == StatusKindFailed {
		t.Fatalf("Process produced a Failed row: %+v", rows[0])
	}
	if rows[0].MaKhachHang != emartCustomerCode {
		t.Fatalf("MaKhachHang = %q, want the hardcoded constant %q", rows[0].MaKhachHang, emartCustomerCode)
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
	if len(sheetRows) <= 8 {
		t.Fatalf("expected rows written beyond the 8-row template header, got %d total rows", len(sheetRows))
	}
}

func TestEmartRegionInfo(t *testing.T) {
	cases := []struct {
		name                                     string
		customerCode                             string
		wantRegion, wantStatCode, wantWarehouse string
	}{
		{
			name:          "MB-prefixed code",
			customerCode:  "MB12345",
			wantRegion:    "MT_MB",
			wantStatCode:  "HN",
			wantWarehouse: "TP_HN_12",
		},
		{
			name:          "the real, always-used hardcoded constant (default branch)",
			customerCode:  emartCustomerCode,
			wantRegion:    "MT_MN",
			wantStatCode:  "LA",
			wantWarehouse: "LA_KHO2026",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRegion, gotStatCode, gotWarehouse := emartRegionInfo(tc.customerCode)
			if gotRegion != tc.wantRegion || gotStatCode != tc.wantStatCode || gotWarehouse != tc.wantWarehouse {
				t.Errorf("emartRegionInfo(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.customerCode, gotRegion, gotStatCode, gotWarehouse,
					tc.wantRegion, tc.wantStatCode, tc.wantWarehouse)
			}
		})
	}
}

func TestEmartStoreNames(t *testing.T) {
	cases := []struct {
		name           string
		storeName      string
		wantShortCode  string
		wantFullName   string
	}{
		{"EMART GO VAP -> PVT", "EMART GO VAP", "PVT", "SIÊU THỊ EMART PHAN VĂN TRỊ"},
		{"EMART PHI -> PHI", "EMART PHI", "PHI", "SIÊU THỊ EMART PHAN HUY ÍCH"},
		{"EMART SALA -> SALA", "EMART SALA", "SALA", "SIÊU THỊ EMART SALA"},
		{"unrecognized store: short code falls back to raw text, no full name", "EMART SOMEWHERE ELSE", "EMART SOMEWHERE ELSE", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotShort, gotFull := emartStoreNames(tc.storeName)
			if gotShort != tc.wantShortCode {
				t.Errorf("shortCode = %q, want %q", gotShort, tc.wantShortCode)
			}
			if gotFull != tc.wantFullName {
				t.Errorf("fullName = %q, want %q", gotFull, tc.wantFullName)
			}
		})
	}
}

// TestRealProcessor_EmartNoBraceBonusRowUsesKMRoiNote regression-tests
// Emart's own no-{...}-brace fallback text — the spec's explicitly
// required test for this block (docs/superpowers/specs/2026-08-18-
// emart-real-processor-design.md's Testing section). Uses
// sample_emart_order.pdf's real first product (barcode 8809174900138,
// OU Qty 48, per-unit price 26950 — confirmed by direct extraction of
// đơn hàng/08-2026/4501866956.PDF during planning) with a "2+1 SP0002"
// promo (an "X+1" match mentioning SP0002, a known internal SKU already
// present in the productdata test fixture — see TestFindSkusMentioned)
// and NO {...} braces, triggering the exact no-brace bonus-row path at
// i==0.
func TestRealProcessor_EmartNoBraceBonusRowUsesKMRoiNote(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	const promoValue = "2+1 SP0002"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8809174900138", "Chai thả toilet", "26950", promoValue},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_emart_order.pdf", 1); err != nil {
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
		case "8809174900138":
			mainRow = row
		case "SP0002":
			bonusRow = row
		}
	}
	if mainRow == nil || bonusRow == nil {
		t.Fatalf("missing expected rows: main=%v bonus=%v", mainRow, bonusRow)
	}

	if got := cell(mainRow, colPromoNote); got != "KM Rời - Không Che Barcode" {
		t.Errorf("main row PromoNote (AO) = %q, want %q (Emart's own no-brace fallback, not Coop's or Winmart's)", got, "KM Rời - Không Che Barcode")
	}
	if got := cell(mainRow, colPromoBundleSku); got != "" {
		t.Errorf("main row PromoBundleSku (AP) = %q, want empty (Emart's no-brace branch never writes AP)", got)
	}
	if got := cell(bonusRow, colPromoNote); got != "" {
		t.Errorf("bonus row PromoNote (AO) = %q, want empty (Emart's no-brace branch never touches the bonus row's own AO at i==0)", got)
	}
	if got := cell(bonusRow, colPromoBundleSku); got != "" {
		t.Errorf("bonus row PromoBundleSku (AP) = %q, want empty", got)
	}
}

// TestRealProcessor_EmartInvoiceLevelPromoBonusRow covers the
// invoice-level ("Hóa Đơn") promo bonus row — Q gets only the FIRST
// mentioned SKU, not a joined list — the spec's other explicitly
// required test. Uses all 7 of sample_emart_order.pdf's real products at
// their exact real per-unit price (confirmed against
// 4501866956.PDF: 48*26950 + 24*26950 + 20*97258 + 40*97258 + 24*40000 +
// 8*73545 + 8*73545 = 9,912,600 — which matches the real PDF's own
// printed "Total Amount(without VAT) : 9.912.600" line exactly), so
// totalValue is a known, real, independently-confirmed constant with no
// price-mismatch noise. A "Hóa Đơn" row mentioning both SP0001 and
// SP0002 (in that order) with a 100000 money amount yields
// floor(9912600/100000) = 99 expected bonus units, attributed only to
// SP0001 (the first mentioned SKU).
func TestRealProcessor_EmartInvoiceLevelPromoBonusRow(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8809174900138", "Chai thả toilet ngàn hoa", "26950", ""},
		{"2", "8809174900213", "Chai thả toilet hoa đào", "26950", ""},
		{"3", "8936156730404", "Nước giặt hương thảo mộc", "97258", ""},
		{"4", "8936156730398", "Nước giặt hương nước hoa", "97258", ""},
		{"5", "8936156730459", "Nước rửa chén đậu xanh", "40000", ""},
		{"6", "8936156731630", "Nước rửa chén chanh", "73545", ""},
		{"7", "8936156731647", "Nước rửa chén không mùi", "73545", ""},
		{"8", "Hóa Đơn", "", "0", "100000 SP0001 SP0002"},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_emart_order.pdf", 1); err != nil {
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

	if got := cell(bonusRow, colIsPromoItem); got != "Có" {
		t.Errorf("invoice bonus row IsPromoItem (U) = %q, want %q", got, "Có")
	}
	if got := cell(bonusRow, colQty); got != "99" {
		t.Errorf("invoice bonus row Qty (X) = %q, want %q (floor(totalValue=9912600 / amount=100000))", got, "99")
	}
	if got := cell(bonusRow, colPromoNote); got != "KM Bó Kèm - Che Barcode" {
		t.Errorf("invoice bonus row PromoNote (AO) = %q, want %q (the invoice-level block's own fallback — unlike the per-item block, this one is NOT overridden for Emart)", got, "KM Bó Kèm - Che Barcode")
	}

	for _, row := range sheetRows {
		if cell(row, colSKU) == "SP0002" {
			t.Errorf("found a row with SKU (Q) = %q, want none (only the first mentioned SKU, SP0001, should get an invoice bonus row)", "SP0002")
		}
	}
}

// TestRealProcessor_EmartInvoiceBonusRowSkipsCleanlyWhenNoSkuMentioned
// covers the guard Python's real code lacks (xulydonhang.py:5290,
// unconditional kiemtra[0] indexing with no length check — a latent
// IndexError crash risk if the "Hóa Đơn" promo string mentions no known
// SKU). This port mirrors buildInvoiceBonusRow's own len(skus)==0 guard
// instead: Process must complete without error/a Failed row, and no
// invoice-level bonus row gets added.
func TestRealProcessor_EmartInvoiceBonusRowSkipsCleanlyWhenNoSkuMentioned(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8809174900138", "Chai thả toilet", "26950", ""},
		{"2", "Hóa Đơn", "", "0", "100000 KHONGCOSKUNAODUOCNHACDEN"},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_emart_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v (should skip the invoice bonus row cleanly, not fail the whole order)", err)
	}
	if len(rows) == 0 || rows[0].StatusKind == StatusKindFailed {
		t.Fatalf("Process produced a Failed row: %+v", rows)
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
	const colSKU = 16
	for _, row := range sheetRows {
		if len(row) > colSKU && row[colSKU] == "KHONGCOSKUNAODUOCNHACDEN" {
			t.Errorf("found a bonus row for an unmatched promo string, want none")
		}
	}
}

// TestRealProcessor_EmartMultiCTKMSecondPartGetsKMRoiNote covers the i>0
// branch of the multi-CTKM "|"-split loop — the exact branch the Task 4
// Critical AO-placement fix's gate (`if i != 0`) must still allow
// through, not just correctly withhold at i==0 (already covered by
// TestRealProcessor_EmartNoBraceBonusRowUsesKMRoiNote). Flagged as an
// untested gap by this plan's final whole-branch review: none of the 9
// currently-available real fixtures contain a "|"-split promo.
//
// Uses sample_emart_order.pdf's real first product (barcode
// 8809174900138, OU Qty 48, per-unit price 26950 — confirmed by direct
// extraction of đơn hàng/08-2026/4501866956.PDF during planning) with a
// two-part no-brace promo "2+1 SP0002|1+1 SP0001" (both parts are "X+1"
// matches mentioning known internal SKUs already present in the
// productdata test fixture — see TestFindSkusMentioned). Both parts lack
// {...} braces, so both trigger Emart's own no-brace fallback path — the
// first (i==0) writes the fallback onto the MAIN row only; the second
// (i==1) must write it onto ITS OWN bonus row.
func TestRealProcessor_EmartMultiCTKMSecondPartGetsKMRoiNote(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	const promoValue = "2+1 SP0002|1+1 SP0001"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8809174900138", "Chai thả toilet", "26950", promoValue},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_emart_order.pdf", 1); err != nil {
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

	const colSKU, colPromoNote, colPromoBundleSku = 16, 40, 41
	cell := func(row []string, idx int) string {
		if idx < len(row) {
			return row[idx]
		}
		return ""
	}

	var mainRow, firstBonusRow, secondBonusRow []string
	for _, row := range sheetRows {
		switch cell(row, colSKU) {
		case "8809174900138":
			mainRow = row
		case "SP0002":
			firstBonusRow = row
		case "SP0001":
			secondBonusRow = row
		}
	}
	if mainRow == nil || firstBonusRow == nil || secondBonusRow == nil {
		t.Fatalf("missing expected rows: main=%v first(SP0002)=%v second(SP0001)=%v", mainRow, firstBonusRow, secondBonusRow)
	}

	// i==0 (SP0002's bonus row): AO stays empty, main row carries the fallback.
	if got := cell(mainRow, colPromoNote); got != "KM Rời - Không Che Barcode" {
		t.Errorf("main row PromoNote (AO) = %q, want %q", got, "KM Rời - Không Che Barcode")
	}
	if got := cell(firstBonusRow, colPromoNote); got != "" {
		t.Errorf("i==0 bonus row (SP0002) PromoNote (AO) = %q, want empty", got)
	}

	// i>0 (SP0001's bonus row): this IS where the fallback note must
	// land, per xulydonhang.py:5230 (the "if i > 0:" branch) — the exact
	// branch the Task 4 Critical fix's `if i != 0` gate must still allow
	// through.
	if got := cell(secondBonusRow, colPromoNote); got != "KM Rời - Không Che Barcode" {
		t.Errorf("i>0 bonus row (SP0001) PromoNote (AO) = %q, want %q (this is the branch the Task 4 AO fix must still fire for)", got, "KM Rời - Không Che Barcode")
	}
	if got := cell(secondBonusRow, colPromoBundleSku); got != "" {
		t.Errorf("i>0 bonus row (SP0001) PromoBundleSku (AP) = %q, want empty (no-brace fallback never writes AP)", got)
	}
}
