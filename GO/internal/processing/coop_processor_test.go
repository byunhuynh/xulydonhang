package processing

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xuri/excelize/v2"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

type fixturePricingSource struct {
	index *pricing.Index
}

func (f *fixturePricingSource) FetchIndex(sheetKey string) (*pricing.Index, error) {
	return f.index, nil
}

func copyTestWorkbookForProcessor(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/dondathang.xlsx")
	if err != nil {
		t.Fatalf("failed reading test workbook fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "dondathang.xlsx")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed writing temp workbook: %v", err)
	}
	return path
}

func TestRealProcessor_ProcessesRealSampleCoopFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá"},
		{"1", "1234567", "Nước giặt", "141.272"},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_coop_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Process returned %d rows, want 1: %+v", len(rows), rows)
	}
	if rows[0].System != "COOPMART" && rows[0].System != "COOPFOOD" {
		t.Fatalf("System = %q, want COOPMART or COOPFOOD", rows[0].System)
	}
	if rows[0].PO == "" {
		t.Fatal("PO is empty, want a parsed PO number")
	}
}

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

func TestRealProcessor_NonCoopFileProducesFailedRow(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	rp := &RealProcessor{Store: store, Pricing: &fixturePricingSource{index: pricing.ParseIndex(nil)}, ExcelPath: copyTestWorkbookForProcessor(t)}

	// Any file whose text doesn't match Coop's vendor markers — a
	// second copy of the same PDF works for this table-stakes check
	// too, since a text-substitution fixture is simpler to construct
	// than a whole different-vendor PDF; the point under test is the
	// "vendor not recognized" branch of Process, not real BigC parsing.
	rows, err := rp.Process(context.Background(), "testdata/not_a_coop_file.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error (should return a Failed row, not an error): %v", err)
	}
	if len(rows) != 1 || rows[0].StatusKind != StatusKindFailed {
		t.Fatalf("rows = %+v, want exactly 1 Failed row", rows)
	}
}

// TestRealProcessor_PromoBonusRowFieldsMatchPythonRowTarget regression-tests
// two bugs a review found in the plan's original reference code for
// coop_processor.go's promo/bonus-row logic, re-traced against
// xulydonhang.py:1085,1174-1176,1201,1211:
//
//  1. khuyenmai (the promo text used for PromoContent/bonus-SKU splitting)
//     is set on every examined promo candidate, not only on a price match —
//     so it must still be populated when the product row ends up a price
//     mismatch.
//  2. the FIRST promo item (index 0) split by "|" writes its note/bundle-SKU
//     onto the MAIN PRODUCT ROW (not its own bonus row); every later item
//     (index > 0) writes onto its own bonus row instead.
//
// This uses the real sample PDF's actual first product barcode (cleaned to
// "3564270") together with two internal SKUs already present in the
// productdata test fixture (SP0001, SP0002) as the promo's bonus-item
// mentions, and a promo/price fixture engineered to NOT match the invoice
// price — so the "even when mismatched" half of bug 1 is exercised too.
func TestRealProcessor_PromoBonusRowFieldsMatchPythonRowTarget(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	const promoValue = "cm Tang SP0001 {Bó Kèm - Che Barcode}|Tang SP0002 {Combo 2}"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "3564270", "Nước giặt", "500000", promoValue},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_coop_order.pdf", 1); err != nil {
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

	// Column indices (0-based) matching excelwriter's Q/AO/AP/AQ layout.
	const colSKU, colPromoNote, colPromoBundleSku, colPromoContent = 16, 40, 41, 42
	cell := func(row []string, idx int) string {
		if idx < len(row) {
			return row[idx]
		}
		return ""
	}

	var mainRow, bonusRow0, bonusRow1 []string
	for _, row := range sheetRows {
		switch cell(row, colSKU) {
		case "3564270":
			mainRow = row
		case "SP0001":
			bonusRow0 = row
		case "SP0002":
			bonusRow1 = row
		}
	}
	if mainRow == nil || bonusRow0 == nil || bonusRow1 == nil {
		t.Fatalf("missing expected rows: main=%v bonus0=%v bonus1=%v", mainRow, bonusRow0, bonusRow1)
	}

	// Bug 1: PromoContent must be populated on the main row even though
	// this product's price didn't match (500000 real price vs. the PDF's
	// actual invoice unit price).
	if got := cell(mainRow, colPromoContent); got != promoValue {
		t.Errorf("main row PromoContent = %q, want %q (bug 1: khuyenmai must be set even on mismatch)", got, promoValue)
	}

	// Bug 2: item 0's note/bundle-SKU belongs on the MAIN row.
	if got := cell(mainRow, colPromoNote); got != "Bó Kèm - Che Barcode" {
		t.Errorf("main row PromoNote = %q, want %q (item 0's note belongs on the main row)", got, "Bó Kèm - Che Barcode")
	}
	if got := cell(mainRow, colPromoBundleSku); got != "4270_0001_1" {
		t.Errorf("main row PromoBundleSku = %q, want %q", got, "4270_0001_1")
	}

	// Bonus row 0 (SP0001, from item 0): shares the bundle-SKU with the
	// main row, but must NOT also carry the note (that stays on main row).
	if got := cell(bonusRow0, colPromoNote); got != "" {
		t.Errorf("bonus row 0 PromoNote = %q, want empty (item 0's note goes to the main row, not here)", got)
	}
	if got := cell(bonusRow0, colPromoBundleSku); got != "4270_0001_1" {
		t.Errorf("bonus row 0 PromoBundleSku = %q, want %q", got, "4270_0001_1")
	}

	// Bonus row 1 (SP0002, from item 1, index > 0): carries its own note
	// on its own row; not a bundle so no bundle-SKU.
	if got := cell(bonusRow1, colPromoNote); got != "Combo 2" {
		t.Errorf("bonus row 1 PromoNote = %q, want %q (item 1's note belongs on its own row)", got, "Combo 2")
	}
	if got := cell(bonusRow1, colPromoBundleSku); got != "" {
		t.Errorf("bonus row 1 PromoBundleSku = %q, want empty", got)
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
	// rather than needing a mapped barcode.
	const promoValue = "2+1 SP0002"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156730244", "Nước giặt", "500000", promoValue},
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
// coop_processor.go's call site right after the products loop). None of
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
// .DonGia, which coop_processor.go sets to fmt.Sprintf("%.0f", totalValue)
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
