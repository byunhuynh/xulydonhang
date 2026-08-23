package processing

import (
	"context"
	"math"
	"strconv"
	"testing"

	"github.com/xuri/excelize/v2"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleLotteFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	// Empty price index on purpose: this file's real barcodes
	// (8936156730244/8936156730329) aren't in the small test fixture, so
	// both products are expected to come back as price mismatches
	// (Warning), not Done — that's still a fully exercised, deterministic
	// code path.
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(nil)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_lotte_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Process returned %d rows, want 1: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.System != "Lotte" {
		t.Fatalf("row.System = %q, want %q", row.System, "Lotte")
	}
	if row.PO != "260727-01013-00057" {
		t.Fatalf("row.PO = %q, want %q", row.PO, "260727-01013-00057")
	}
	if row.MaKhachHang != "Không xác định" {
		t.Fatalf("row.MaKhachHang = %q, want %q (test fixture's data.xlsx has no store 1013)", row.MaKhachHang, "Không xác định")
	}
	if row.StatusKind != StatusKindWarning {
		t.Fatalf("row.StatusKind = %q, want %q (both products should price-mismatch against the empty index)", row.StatusKind, StatusKindWarning)
	}
}

// TestRealProcessor_LotteNoBraceBonusRowUsesGiaoRoiNote regression-tests a
// bug a Task 9 review found: processLotteSegment reaches buildPromoBonusRow
// (shared with Coop) whenever a Lotte promo yields a bonus SKU (via an
// "X+1" match or a known-SKU mention), but that helper's no-{...}-brace
// fallback is Coop's own default ("KM Bó Kèm - Che Barcode", which also
// makes it write an AP bundle-SKU on both rows — xulydonhang.py:1198's
// "... or 'KM Bó Kèm - Che Barcode'"). Lotte's write_to_dondathang_lotte
// has a different no-brace branch (xulydonhang.py:2204-2217: "else:
// sheet[f'AO{current_row}'] = 'KM Giao Rời - Không Che Barcode'") that
// never writes AP at all. None of the 60 real golden fixtures exercise
// this path (every non-null AO in that set originates from a real {...}
// brace), so it needed its own synthetic regression test: a promo value
// with an "X+1" match and a known bonus SKU mention but NO {...} braces.
func TestRealProcessor_LotteNoBraceBonusRowUsesGiaoRoiNote(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	// No {...} braces here on purpose — sample_lotte_order.pdf's first
	// real product barcode is "8936156730244" (per
	// TestRealProcessor_ProcessesRealSampleLotteFile's comment); SP0002
	// is a known internal SKU already present in the productdata test
	// fixture (see TestFindSkusMentioned), so it's mentioned directly
	// rather than needing a mapped barcode. Price is "65296" (the real
	// extracted invoice price for this barcode/PO, confirmed by direct
	// probe) rather than an arbitrary value: the bonus row now only
	// builds once matched is true (see processLotteSegment's own "Gated
	// on matched" comment), so the price must actually match the
	// invoice for this regression test to still exercise that path.
	const promoValue = "2+1 SP0002"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156730244", "Nước giặt", "65296", promoValue},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_lotte_order.pdf", 1); err != nil {
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
	// same as TestRealProcessor_PromoBonusRowFieldsMatchPythonRowTarget.
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
		case "8936156730244":
			mainRow = row
		case "SP0002":
			bonusRow = row
		}
	}
	if mainRow == nil || bonusRow == nil {
		t.Fatalf("missing expected rows: main=%v bonus=%v", mainRow, bonusRow)
	}

	if got := cell(mainRow, colPromoNote); got != "KM Giao Rời - Không Che Barcode" {
		t.Errorf("main row PromoNote (AO) = %q, want %q (Lotte's own no-brace fallback, not Coop's)", got, "KM Giao Rời - Không Che Barcode")
	}
	if got := cell(mainRow, colPromoBundleSku); got != "" {
		t.Errorf("main row PromoBundleSku (AP) = %q, want empty (Lotte's no-brace branch never writes AP)", got)
	}
	if got := cell(bonusRow, colPromoNote); got != "" {
		t.Errorf("bonus row PromoNote (AO) = %q, want empty (Lotte's no-brace branch never touches the bonus row's own AO)", got)
	}
	if got := cell(bonusRow, colPromoBundleSku); got != "" {
		t.Errorf("bonus row PromoBundleSku (AP) = %q, want empty (Lotte's no-brace branch never writes AP)", got)
	}
}

// TestRealProcessor_LotteInvoiceBonusRowFromFrozenPricing regression-tests
// the invoice-level bonus-row path in processLotteSegment (buildInvoiceBonusRow,
// reached via priceIndex.FindInvoicePromotion(info.EntryDate) — see
// lotte_processor.go's call site right after the products loop). None of
// the 60 real Lotte golden fixtures exercise this path: every one of them
// is dated in a range the frozen pricing sheet's "Hóa Đơn" row has no
// matching promo column for (confirmed by inspecting
// lotte/testdata/fixtures/_frozen_pricing.json during the final
// whole-branch review), so it had zero test coverage even though it's the
// same class of "Coop-authored helper reused on the Lotte path" as the
// no-brace bonus-row case above. buildInvoiceBonusRow is shared verbatim
// with Coop and was verified correct for Lotte against
// xulydonhang.py:2251-2298 during that review — this test proves it, it
// doesn't just assert a no-op.
//
// The synthetic price CSV gives sample_lotte_order.pdf's two real product
// barcodes (8936156730244, 8936156730329 — see
// TestRealProcessor_ProcessesRealSampleLotteFile) fixed prices with no
// promo column, and a "Hóa Đơn" row with a promo active across the whole
// year (so it covers the sample PDF's entry date, 27/07/2026) mentioning
// SP0001, a known internal SKU already present in the productdata test
// fixture (see TestFindSkusMentioned), with a bare 5-digit money amount
// ExtractMoneyAmount recognizes. Rather than hardcoding an expected Qty
// (which would require knowing the PDF's exact box quantities), this
// reads back the actual total order value RealProcessor computed (OrderRow
// .DonGia, which lotte_processor.go sets to fmt.Sprintf("%.0f", totalValue)
// right before the invoice-bonus check runs) and derives the expected
// floor(totalValue/amount) from that, matching buildInvoiceBonusRow's own
// bonusQty formula (coop_processor.go: math.Floor(totalValue/float64(amount))).
func TestRealProcessor_LotteInvoiceBonusRowFromFrozenPricing(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	const invoicePromoValue = "50000 SP0001"
	const invoicePromoAmount = 50000.0
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156730244", "Nước giặt", "100000", ""},
		{"2", "8936156730329", "Nước xả", "200000", ""},
		{"3", "Hóa Đơn", "", "0", invoicePromoValue},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_lotte_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Process returned %d rows, want 1: %+v", len(rows), rows)
	}
	totalValue, err := strconv.ParseFloat(rows[0].DonGia, 64)
	if err != nil {
		t.Fatalf("failed parsing DonGia %q as float: %v", rows[0].DonGia, err)
	}
	expectedQty := math.Floor(totalValue / invoicePromoAmount)
	if expectedQty <= 0 {
		t.Fatalf("expectedQty = %v (from totalValue %v / amount %v), want > 0 for this test to exercise a real bonus row", expectedQty, totalValue, invoicePromoAmount)
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

	// Column indices (0-based), same layout as
	// TestRealProcessor_LotteNoBraceBonusRowUsesGiaoRoiNote: Q=SKU (16),
	// U=IsPromoItem (20), X=Qty (23), Z=Thành tiền formula (25).
	const colSKU, colIsPromoItem, colQty, colZFormula = 16, 20, 23, 25
	cell := func(row []string, idx int) string {
		if idx < len(row) {
			return row[idx]
		}
		return ""
	}

	var bonusRow []string
	var bonusRowNum int
	for i, row := range sheetRows {
		if cell(row, colSKU) == "SP0001" {
			bonusRow = row
			bonusRowNum = i + 1 // excelize rows are 1-indexed
			break
		}
	}
	if bonusRow == nil {
		t.Fatalf("no row with SKU (Q) = %q found; sheet rows: %+v", "SP0001", sheetRows)
	}

	if got := cell(bonusRow, colIsPromoItem); got != "Có" {
		t.Errorf("bonus row IsPromoItem (U) = %q, want %q", got, "Có")
	}
	gotQtyStr := cell(bonusRow, colQty)
	gotQty, err := strconv.ParseFloat(gotQtyStr, 64)
	if err != nil {
		t.Fatalf("failed parsing Qty (X) %q as float: %v", gotQtyStr, err)
	}
	if gotQty != expectedQty {
		t.Errorf("bonus row Qty (X) = %v, want %v (floor(totalValue=%v / amount=%v))", gotQty, expectedQty, totalValue, invoicePromoAmount)
	}

	gotFormula, err := f.GetCellFormula("Don dat hang", "Z"+strconv.Itoa(bonusRowNum))
	if err != nil {
		t.Fatalf("failed reading Z formula: %v", err)
	}
	if gotFormula != "" {
		t.Errorf("bonus row Z formula = %q, want no formula (UseZFormula: false -> literal 0)", gotFormula)
	}
	gotZ := cell(bonusRow, 25)
	if gotZ != "0" {
		t.Errorf("bonus row Z literal value = %q, want %q", gotZ, "0")
	}
}
