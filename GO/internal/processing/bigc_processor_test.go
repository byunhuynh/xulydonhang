package processing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/xuri/excelize/v2"

	"order-processor/internal/pdfpage"
	"order-processor/internal/processing/bigc"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func bigcTestWorkbookRowCount(t *testing.T, path string) int {
	t.Helper()
	book, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open workbook %s: %v", path, err)
	}
	defer book.Close()
	rows, err := book.GetRows("Don dat hang")
	if err != nil {
		t.Fatalf("read Don dat hang rows: %v", err)
	}
	return len(rows)
}

func bigcStreamingFixture(t *testing.T) string {
	t.Helper()
	source := "testdata/sample_bigc_order.pdf"
	page0, cleanup0, err := pdfpage.ExtractPage(source, 1)
	if err != nil {
		t.Fatalf("extract BigC page 1: %v", err)
	}
	t.Cleanup(cleanup0)
	store1, cleanup1, err := pdfpage.ExtractPage(source, 2)
	if err != nil {
		t.Fatalf("extract BigC page 2: %v", err)
	}
	t.Cleanup(cleanup1)
	store2, cleanup2, err := pdfpage.ExtractPage(source, 3)
	if err != nil {
		t.Fatalf("extract BigC page 3: %v", err)
	}
	t.Cleanup(cleanup2)

	fixturePath := filepath.Join(t.TempDir(), "bigc-streaming-mixed-pages.pdf")
	inputs := []string{page0, store1, "testdata/not_a_coop_file.pdf", store2}
	if err := api.MergeCreateFile(inputs, fixturePath, false, nil); err != nil {
		t.Fatalf("build mixed BigC fixture: %v", err)
	}
	return fixturePath
}

func newBigCStreamingProcessor(t *testing.T, excelPath string) *RealProcessor {
	t.Helper()
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("load product fixture: %v", err)
	}
	return &RealProcessor{
		Store: store, Pricing: &fixturePricingSource{index: pricing.ParseIndex(nil)}, ExcelPath: excelPath,
	}
}

func TestBigCStreamingEmitsProvisionalStoresBeforeCombinedWriteAndFinalizesSameKeys(t *testing.T) {
	excelPath := copyTestWorkbookForProcessor(t)
	initialRows := bigcTestWorkbookRowCount(t, excelPath)
	rp := newBigCStreamingProcessor(t, excelPath)
	filePath := bigcStreamingFixture(t)

	var events []OrderRow
	rows, err := rp.ProcessStreaming(context.Background(), filePath, 1, func(row OrderRow) {
		if row.StatusKind == StatusKindProcessing {
			if got := bigcTestWorkbookRowCount(t, excelPath); got != initialRows {
				t.Errorf("workbook already has %d rows during provisional callback, want unchanged %d before combined write", got, initialRows)
			}
		}
		events = append(events, row)
	})
	if err != nil {
		t.Fatalf("ProcessStreaming returned error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("returned %d rows, want one final row per store page: %+v", len(rows), rows)
	}
	if len(events) != 5 {
		t.Fatalf("emitted %d events, want processing/failed/processing/done/done: %+v", len(events), events)
	}

	base := filepath.Base(filePath)
	po := "2631057733376"
	firstKey := orderResultKey(base, "2/4", po)
	failedKey := orderResultKey(base, "3/4", "")
	lastKey := orderResultKey(base, "4/4", po)
	wantKeys := []string{firstKey, failedKey, lastKey, firstKey, lastKey}
	wantKinds := []string{StatusKindProcessing, StatusKindFailed, StatusKindProcessing, StatusKindDone, StatusKindDone}
	for i := range events {
		if events[i].ResultKey != wantKeys[i] || events[i].StatusKind != wantKinds[i] {
			t.Errorf("event %d = key %q status %q, want key %q status %q", i, events[i].ResultKey, events[i].StatusKind, wantKeys[i], wantKinds[i])
		}
	}

	if rows[0].ResultKey != firstKey || rows[0].StatusKind != StatusKindDone {
		t.Errorf("first returned store = key %q status %q, want %q done", rows[0].ResultKey, rows[0].StatusKind, firstKey)
	}
	if rows[1].ResultKey != failedKey || rows[1].StatusKind != StatusKindFailed {
		t.Errorf("parse-failed returned page = key %q status %q, want %q failed", rows[1].ResultKey, rows[1].StatusKind, failedKey)
	}
	if rows[2].ResultKey != lastKey || rows[2].StatusKind != StatusKindDone {
		t.Errorf("last returned store = key %q status %q, want %q done", rows[2].ResultKey, rows[2].StatusKind, lastKey)
	}
	if len(rows[0].ExcelRows) != 1 || rows[0].ExcelRows[0] != initialRows+1 {
		t.Errorf("first store ExcelRows = %v, want [%d] (absolute combined-write header row)", rows[0].ExcelRows, initialRows+1)
	}
	if len(rows[1].ExcelRows) != 0 || len(rows[2].ExcelRows) != 0 {
		t.Errorf("rows without written workbook lines got ExcelRows: failed=%v last-empty-store=%v", rows[1].ExcelRows, rows[2].ExcelRows)
	}
}

func TestBigCStreamingCombinedWriteFailureFinalizesEveryProvisionalKey(t *testing.T) {
	rp := newBigCStreamingProcessor(t, filepath.Join(t.TempDir(), "missing", "dondathang.xlsx"))
	filePath := "testdata/sample_bigc_order.pdf"

	var events []OrderRow
	rows, err := rp.ProcessStreaming(context.Background(), filePath, 1, func(row OrderRow) {
		events = append(events, row)
	})
	if err != nil {
		t.Fatalf("ProcessStreaming returned error: %v", err)
	}
	if len(rows) != 19 || len(events) != 38 {
		t.Fatalf("returned %d rows and emitted %d events, want 19 final rows and 38 lifecycle events", len(rows), len(events))
	}
	for i := 0; i < 19; i++ {
		provisional := events[i]
		final := events[i+19]
		if provisional.StatusKind != StatusKindProcessing {
			t.Errorf("provisional event %d status = %q, want processing", i, provisional.StatusKind)
		}
		if final.ResultKey != provisional.ResultKey || final.StatusKind != StatusKindFailed {
			t.Errorf("final event %d = key %q status %q, want same key %q failed", i, final.ResultKey, final.StatusKind, provisional.ResultKey)
		}
		if rows[i].ResultKey != provisional.ResultKey || rows[i].StatusKind != StatusKindFailed {
			t.Errorf("returned row %d = key %q status %q, want same key %q failed", i, rows[i].ResultKey, rows[i].StatusKind, provisional.ResultKey)
		}
		if len(rows[i].ExcelRows) != 0 {
			t.Errorf("failed returned row %d ExcelRows = %v, want none", i, rows[i].ExcelRows)
		}
	}
}

func TestBigCLatestPDFSmoke(t *testing.T) {
	const pdfName = "806_SOUTHDC_Q06_3005382_2634058273095.pdf"
	filePath := filepath.Join("..", "..", "..", "đơn hàng", pdfName)
	if _, err := os.Stat(filePath); err != nil {
		t.Skipf("latest BigC smoke PDF is not available: %v", err)
	}

	store, err := productdata.Load("../../../data.xlsx")
	if err != nil {
		t.Fatalf("load production product fixture: %v", err)
	}
	excelPath := filepath.Join(t.TempDir(), "dondathang.xlsx")
	copyFile(t, "excelwriter/testdata/dondathang.xlsx", excelPath)
	rp := &RealProcessor{Store: store, Pricing: loadFrozenBigcPricingSource(t), ExcelPath: excelPath}

	var events []OrderRow
	rows, err := rp.ProcessStreaming(context.Background(), filePath, 1, func(row OrderRow) {
		events = append(events, row)
	})
	if err != nil {
		t.Fatalf("ProcessStreaming latest BigC PDF: %v", err)
	}
	if len(rows) != 23 {
		t.Fatalf("latest BigC PDF returned %d store rows, want 23: %+v", len(rows), rows)
	}
	if len(events) != 46 {
		t.Fatalf("latest BigC PDF emitted %d lifecycle events, want 46 processing/final events", len(events))
	}

	seen := make(map[string]bool, len(rows))
	for i, row := range rows {
		if row.StatusKind == StatusKindFailed {
			t.Errorf("returned store row %d failed: %+v", i, row)
		}
		if seen[row.ResultKey] {
			t.Errorf("duplicate final ResultKey %q at returned store row %d", row.ResultKey, i)
		}
		seen[row.ResultKey] = true

		provisional := events[i]
		final := events[i+23]
		if provisional.ResultKey != row.ResultKey || provisional.StatusKind != StatusKindProcessing {
			t.Errorf("store %d provisional = key %q status %q, want key %q processing", i, provisional.ResultKey, provisional.StatusKind, row.ResultKey)
		}
		if final.ResultKey != row.ResultKey || final.StatusKind != row.StatusKind {
			t.Errorf("store %d final event = key %q status %q, want returned key %q status %q", i, final.ResultKey, final.StatusKind, row.ResultKey, row.StatusKind)
		}
	}
	t.Logf("smoke PDF %s: stores=%d uniqueKeys=%d failed=0 workbook=%s", filePath, len(rows), len(seen), excelPath)
}

func TestRealProcessor_ProcessesRealSampleBigcFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	// Empty price index on purpose. This file's real barcodes also aren't
	// in the small test fixture's product database (only synthetic
	// SP0001/SP0002/BigC-test entries) — with the xulydonhang.py:4606-4607
	// "skip item if product name not found" guard (bigc_processor.go),
	// every real item on every page is therefore filtered out before it
	// can ever reach price matching, leaving each page with zero saigia
	// and Done status rather than Warning. Real-data correctness
	// (including genuine price mismatches) is covered exhaustively by
	// TestRealProcessor_MatchesGoldenFixtures_BigC, which loads the real
	// production data.xlsx; this test only exercises PDF text extraction
	// end-to-end.
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(nil)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_bigc_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	// 19 store pages -> 19 OrderRows (one per store page, matching
	// Python's per-page-call saigia/tongtien scoping — see this plan's
	// Task 6 design notes on why this is "1 row per store", not "1 row
	// per file", despite all stores sharing one PO and one combined
	// Excel write).
	if len(rows) != 19 {
		t.Fatalf("Process returned %d rows, want 19: %+v", len(rows), rows)
	}
	for i, row := range rows {
		if row.System != "BigC" {
			t.Fatalf("rows[%d].System = %q, want %q", i, row.System, "BigC")
		}
		if row.PO != "2631057733376" {
			t.Fatalf("rows[%d].PO = %q, want %q", i, row.PO, "2631057733376")
		}
		if row.StatusKind != StatusKindDone {
			t.Fatalf("rows[%d].StatusKind = %v, want %v (test fixture's product database has none of this file's real barcodes -> every item skipped by the not-found guard -> no mismatches possible)", i, row.StatusKind, StatusKindDone)
		}
	}
}

// TestRealProcessor_ProcessesBigcDocument_IsolatesPerPageErrors exercises
// processBigcDocument's per-page error-isolation design directly (Task 6's
// headline architectural risk): a synthetic 4-page document (page 0 +
// 3 store pages) where the MIDDLE store page is deliberately malformed.
// It asserts the malformed page produces exactly one Failed OrderRow
// while the pages BEFORE and AFTER it both succeed, and that both
// successful pages' item rows landed in the single combined
// excelwriter.WriteOrderRows call despite the middle page's failure —
// proving a mid-file error neither aborts later pages nor drops earlier
// pages' already-accumulated rows from the eventual write.
func TestRealProcessor_ProcessesBigcDocument_IsolatesPerPageErrors(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(nil)}
	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}

	// existingRowCount captures how many rows the freshly copied template
	// workbook already has, so the assertions below can locate exactly
	// the rows this test's single WriteOrderRows call appends, without
	// hardcoding the template's header size.
	existingRowCount := func() int {
		f, err := excelize.OpenFile(excelPath)
		if err != nil {
			t.Fatalf("OpenFile failed: %v", err)
		}
		defer f.Close()
		rows, err := f.GetRows("Don dat hang")
		if err != nil {
			t.Fatalf("GetRows failed: %v", err)
		}
		return len(rows)
	}()

	// page0 is a minimal but fully valid page-0 text: a "<PO><entry
	// date>" pair (bigc.ParseOrderInfo's poEntryDatePattern), a "Total
	// Net Purchase Price" marker followed by a cancel date, and an
	// "Article"-prefixed price-list line (bigc.ExtractPriceList) for
	// barcode 8934563112223. It carries none of the MB/LINFOX/FM
	// LOGISTIC customer-code markers ResolveCustomerCode looks for, so
	// customer-code resolution falls through to its default branch —
	// irrelevant to what this test checks.
	page0 := "2631099999999 15/08/26\n" +
		"Total Net Purchase Price\n" +
		"20/08/26\n" +
		"Article\n" +
		"8934563112223 Product One Master Pack 1 6 1 1 15000 PCS 15000\n"

	// page1 (store page 1, FIRST, valid): satisfies both
	// bigc.ExtractStoreName (the "FM LOGISTIC VSIP 2 ... Vietnam
	// \n<name>\n" shape) and bigc.ExtractStoreItems (the
	// "<barcode>\n<desc>\nPack\n<n>\n<n>\n<n>" shape).
	page1 := "FM LOGISTIC VSIP 2 (806)\n" +
		"Some Address Line\n" +
		"Vietnam\n" +
		"Store One\n" +
		"8934563112223\n" +
		"Product One Description\n" +
		"Pack\n" +
		"1\n" +
		"6\n" +
		"1\n"

	// page2 (store page 2, MIDDLE, deliberately malformed): carries none
	// of the "FM LOGISTIC VSIP 2" / "LINFOX WAREHOUSE (802)" markers
	// bigc.ExtractStoreName requires, so it returns ok=false and
	// processBigcStorePage fails fast with "không tách được tên store"
	// before ever reaching item extraction — this is the isolation path
	// this test exists to exercise.
	page2 := "This page has no store markers at all.\n" +
		"Just some unrelated filler text.\n"

	// page3 (store page 3, LAST, valid): same shape as page1 with a
	// different store name/barcode, proving pages AFTER the failed page
	// still get processed, not just pages before it.
	page3 := "FM LOGISTIC VSIP 2 (806)\n" +
		"Some Address Line\n" +
		"Vietnam\n" +
		"Store Three\n" +
		"8934563112230\n" +
		"Product Three Description\n" +
		"Pack\n" +
		"1\n" +
		"6\n" +
		"1\n"

	rows, err := rp.processBigcDocument("synthetic_bigc_isolation.pdf", []string{page0, page1, page2, page3}, nil)
	if err != nil {
		t.Fatalf("processBigcDocument returned error: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("processBigcDocument returned %d rows, want 3 (one per store page): %+v", len(rows), rows)
	}

	// rows[0] (store 1, page 2/4) and rows[2] (store 3, page 4/4) must
	// both have succeeded; rows[1] (the malformed middle page, 3/4) must
	// be the one and only Failed row.
	if rows[0].StatusKind == StatusKindFailed {
		t.Errorf("rows[0] (first store page) unexpectedly Failed: %+v", rows[0])
	}
	if rows[1].StatusKind != StatusKindFailed {
		t.Errorf("rows[1] (malformed middle store page) StatusKind = %v, want Failed: %+v", rows[1].StatusKind, rows[1])
	}
	if rows[2].StatusKind == StatusKindFailed {
		t.Errorf("rows[2] (last store page) unexpectedly Failed — pages after a failure must still be processed: %+v", rows[2])
	}
	for i, row := range rows {
		if row.System != "BigC" {
			t.Errorf("rows[%d].System = %q, want %q", i, row.System, "BigC")
		}
	}

	// Confirm the successful pages' rows actually landed in the ONE
	// combined Excel write despite the middle page's failure: the
	// written sheet must carry both store1's and store3's item barcodes
	// in column Q (SKU).
	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("reopening workbook failed: %v", err)
	}
	defer f.Close()
	sheetRows, err := f.GetRows("Don dat hang")
	if err != nil {
		t.Fatalf("GetRows failed: %v", err)
	}

	// This test's single processBigcDocument call is the only write
	// against this freshly copied workbook, so every row after the
	// template's original rows belongs to it.
	if len(sheetRows) <= existingRowCount {
		t.Fatalf("sheet has %d rows, want more than the template's original %d — expected new rows from the successful store pages", len(sheetRows), existingRowCount)
	}
	newRows := sheetRows[existingRowCount:]
	var skuValues []string
	for _, r := range newRows {
		if len(r) >= 17 { // column Q is the 17th (1-indexed) column
			skuValues = append(skuValues, r[16])
		} else {
			skuValues = append(skuValues, "")
		}
	}

	hasSKU := func(sku string) bool {
		for _, v := range skuValues {
			if v == sku {
				return true
			}
		}
		return false
	}

	if !hasSKU("8934563112223") {
		t.Errorf("written sheet is missing store 1's item SKU 8934563112223; new rows' SKUs: %v", skuValues)
	}
	if !hasSKU("8934563112230") {
		t.Errorf("written sheet is missing store 3's item SKU 8934563112230 — a mid-file failure must not block later successful pages' rows from landing in the combined write; new rows' SKUs: %v", skuValues)
	}
}

// TestRealProcessor_BigcInvoiceLevelPromoBonusRow covers the invoice-level
// ("Hóa Đơn") promo bonus row that write_to_dondathang_bigc adds once per
// store page (xulydonhang.py:4810-4848) — a real feature gap this Go port
// initially missed entirely (see bigc_processor.go's block right after
// the per-item loop in processBigcStorePage). None of the 29 real BigC
// golden fixtures exercise this path (no "Hóa Đơn" row in the real
// pricing sheet on any fixture's entry date), so it needs a synthetic
// priceIndex built directly via pricing.ParseIndex, following the same
// pattern as TestRealProcessor_LotteInvoiceBonusRowFromFrozenPricing
// (lotte_processor_test.go).
//
// productdata/testdata/data.xlsx carries two rows added specifically for
// BigC tests, keyed directly by 13-digit barcode (no sku_mapping
// indirection needed): "8934563112223" -> BigC Test Product A (weight
// 2.5kg) and "8934563112230" -> BigC Test Product B (weight 1.5kg) — the
// same two barcodes TestRealProcessor_ProcessesBigcDocument_IsolatesPerPageErrors
// already used before this fixture change (that test's hasSKU assertions
// still pass unmodified, since a barcode with no sku_mapping entry
// resolves to itself).
func TestRealProcessor_BigcInvoiceLevelPromoBonusRow(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}

	// "Hóa Đơn" row active all year, mentioning SP0001 (a known internal
	// SKU already present in the fixture, see TestFindSkusMentioned) with
	// a bare 5-digit money amount ExtractMoneyAmount recognizes as 20000.
	// The one real product row (8934563112223) gets a matching real price
	// of 1000, so its 100 ordered units sum to a known tongtien of
	// 100000 -> floor(100000/20000) = 5 expected bonus units.
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8934563112223", "Test Product", "1000", ""},
		{"2", "Hóa Đơn", "", "0", "20000 SP0001"},
	}
	priceIndex := pricing.ParseIndex(priceCsv)
	priceList := []bigc.Product{{Barcode: "8934563112223", UnitPrice: 1000}}

	storeText := "FM LOGISTIC VSIP 2 (806)\n" +
		"Some Address Line\n" +
		"Vietnam\n" +
		"Store One\n" +
		"8934563112223\n" +
		"Product One Description\n" +
		"Pack\n" +
		"1\n" +
		"100\n" +
		"1\n"

	rp := &RealProcessor{Store: store}
	result := rp.processBigcStorePage(storeText, priceList, priceIndex,
		"ĐĐHBIGC-2631099999999", "15/08/2026", "20/08/2026",
		"MN_MT_BIGCAC", "FM LOGISTIC VSIP 2 (806)", "BIGC PO2631099999999",
		"LA_KHO2026", "MT_MN", "LA", false)

	// Expect exactly 2 rows: the one real product row (no per-item promo,
	// so no per-item bonus row), plus the invoice-level bonus row.
	if len(result.rows) != 2 {
		t.Fatalf("processBigcStorePage returned %d rows, want 2 (product row + invoice bonus row): %+v", len(result.rows), result.rows)
	}
	bonusRow := result.rows[1]
	if bonusRow.SKU != "SP0001" {
		t.Errorf("invoice bonus row SKU (Q) = %q, want %q (kiemtra[0] — the FIRST mentioned SKU only, not a joined list)", bonusRow.SKU, "SP0001")
	}
	if bonusRow.Qty != 5 {
		t.Errorf("invoice bonus row Qty (X) = %v, want 5 (floor(tongtien=100000 / amount=20000))", bonusRow.Qty)
	}
	if bonusRow.PromoContent != "20000 SP0001" {
		t.Errorf("invoice bonus row PromoContent (AQ) = %q, want %q", bonusRow.PromoContent, "20000 SP0001")
	}
	if bonusRow.PromoNote != "KM Bó Kèm - Che Barcode" {
		t.Errorf("invoice bonus row PromoNote (AO) = %q, want %q (same default fallback as Coop/Satra, NOT this file's per-item \"KM Rời - Không Che Barcode\" fallback)", bonusRow.PromoNote, "KM Bó Kèm - Che Barcode")
	}
	if !bonusRow.IsPromoItem {
		t.Errorf("invoice bonus row IsPromoItem (U) = false, want true")
	}
	if !bonusRow.NoCaseCount {
		t.Errorf("invoice bonus row NoCaseCount = false, want true (BigC never writes AU on any row)")
	}
	if bonusRow.UseZFormula {
		t.Errorf("invoice bonus row UseZFormula = true, want false (Y=0, Z=0, matching xulydonhang.py:4829-4830)")
	}
	wantWeight := 3.6 * 5 // SP0001's WeightKg * bonusQty
	if bonusRow.LineWeightKg != wantWeight {
		t.Errorf("invoice bonus row LineWeightKg (AT) = %v, want %v", bonusRow.LineWeightKg, wantWeight)
	}
}

// TestRealProcessor_BigcInvoiceLevelPromoBonusRow_SkippedWithoutMatch
// proves the invoice-level bonus row is skipped cleanly (not a crash, not
// a zero-value row) when find_all_promotions_by_sku_and_time("Hóa Đơn",
// ...) returns nothing, matching xulydonhang.py:4812's `if kmhoadon:`
// guard — the ordinary, overwhelmingly common case for all 29 real
// fixtures today.
func TestRealProcessor_BigcInvoiceLevelPromoBonusRow_SkippedWithoutMatch(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	priceIndex := pricing.ParseIndex(nil) // no "Hóa Đơn" row at all
	priceList := []bigc.Product{{Barcode: "8934563112223", UnitPrice: 0}}

	storeText := "FM LOGISTIC VSIP 2 (806)\n" +
		"Some Address Line\n" +
		"Vietnam\n" +
		"Store One\n" +
		"8934563112223\n" +
		"Product One Description\n" +
		"Pack\n" +
		"1\n" +
		"6\n" +
		"1\n"

	rp := &RealProcessor{Store: store}
	result := rp.processBigcStorePage(storeText, priceList, priceIndex,
		"ĐĐHBIGC-2631099999999", "15/08/2026", "20/08/2026",
		"MN_MT_BIGCAC", "FM LOGISTIC VSIP 2 (806)", "BIGC PO2631099999999",
		"LA_KHO2026", "MT_MN", "LA", false)

	if len(result.rows) != 1 {
		t.Fatalf("processBigcStorePage returned %d rows, want 1 (product row only, no invoice bonus row): %+v", len(result.rows), result.rows)
	}
}

// TestRealProcessor_BigcSkipsItemWithUnknownProduct covers the
// BigC-specific "skip item if product name not found" guard
// (xulydonhang.py:4606-4607): an item whose (resolved) barcode has no
// entry in the product database must be dropped entirely — no row, no
// weight/tongtien contribution — while leaving every OTHER item on the
// same page unaffected. This exact check exists nowhere else in the
// whole Python file.
func TestRealProcessor_BigcSkipsItemWithUnknownProduct(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	priceIndex := pricing.ParseIndex(nil)

	// Two items on the same store page: 8934563112223 resolves to a real
	// product (BigC Test Product A, added to the test fixture), while
	// 8934563112299 deliberately matches no product at all.
	storeText := "FM LOGISTIC VSIP 2 (806)\n" +
		"Some Address Line\n" +
		"Vietnam\n" +
		"Store One\n" +
		"8934563112223\n" +
		"Known Product Description\n" +
		"Pack\n" +
		"1\n" +
		"6\n" +
		"1\n" +
		"8934563112299\n" +
		"Unknown Product Description\n" +
		"Pack\n" +
		"1\n" +
		"6\n" +
		"1\n"

	rp := &RealProcessor{Store: store}
	result := rp.processBigcStorePage(storeText, nil, priceIndex,
		"ĐĐHBIGC-2631099999999", "15/08/2026", "20/08/2026",
		"MN_MT_BIGCAC", "FM LOGISTIC VSIP 2 (806)", "BIGC PO2631099999999",
		"LA_KHO2026", "MT_MN", "LA", false)

	// Exactly ONE row: the known item. The unknown item contributes no
	// row at all — not a row with a blank/zero product name.
	if len(result.rows) != 1 {
		t.Fatalf("processBigcStorePage returned %d rows, want 1 (unknown-product item must be skipped entirely): %+v", len(result.rows), result.rows)
	}
	if result.rows[0].SKU != "8934563112223" {
		t.Errorf("rows[0].SKU = %q, want %q (the known item)", result.rows[0].SKU, "8934563112223")
	}
	if result.rows[0].ProductName != "BigC Test Product A" {
		t.Errorf("rows[0].ProductName = %q, want %q", result.rows[0].ProductName, "BigC Test Product A")
	}
	// The skipped item's weight (1.5kg * 6 = 9kg) must NOT be counted.
	wantWeight := 2.5 * 6 // only the known item's contribution
	if result.weightKg != wantWeight {
		t.Errorf("weightKg = %v, want %v (unknown item's weight must not be counted)", result.weightKg, wantWeight)
	}
}

// TestRealProcessor_ProcessesBigcDocument_PriceMismatchDetailsAcrossStores
// exercises the ONE genuinely different piece of PriceMismatchDetail math
// in the whole price-mismatch-resolution plan: BigC's 3-stage ExcelRow
// offset (store-local index -> cross-store combined-write offset,
// snapshotted BEFORE that store's own rows are appended -> final absolute
// row, added once after WriteOrderRows returns). A prior review traced
// this by hand on paper; this test proves it against a REAL written
// workbook, and specifically includes a FAILED store BETWEEN two
// successful ones (mirroring TestRealProcessor_ProcessesBigcDocument_
// IsolatesPerPageErrors' page shapes) to prove a failed store's zero-row
// contribution doesn't corrupt the offset accumulation for stores after it.
func TestRealProcessor_ProcessesBigcDocument_PriceMismatchDetailsAcrossStores(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)
	// Empty price index -> FindPrice returns "" -> realPrice=0 for every
	// barcode, guaranteeing BOTH real product items below mismatch against
	// their nonzero page-0 invoice prices, with no need to hand-tune a
	// near-miss price.
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(nil)}
	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}

	existingRowCount := func() int {
		f, err := excelize.OpenFile(excelPath)
		if err != nil {
			t.Fatalf("OpenFile failed: %v", err)
		}
		defer f.Close()
		rows, err := f.GetRows("Don dat hang")
		if err != nil {
			t.Fatalf("GetRows failed: %v", err)
		}
		return len(rows)
	}()

	// Page 0: price-list entries for BOTH real test barcodes (see this
	// file's own doc comment on TestRealProcessor_BigcInvoiceLevelPromoBonusRow
	// for where these two fixture barcodes come from), each with a real
	// nonzero invoice price.
	page0 := "2631099999999 15/08/26\n" +
		"Total Net Purchase Price\n" +
		"20/08/26\n" +
		"Article\n" +
		"8934563112223 Product One Master Pack 1 6 1 1 15000 PCS 15000\n" +
		"8934563112230 Product Two Master Pack 1 6 1 1 20000 PCS 20000\n"

	// Store 1 (page 1, FIRST success): header/note row + 1 mismatched
	// item -> local rows [note(0), item(1)], mismatch at local index 1.
	page1 := "FM LOGISTIC VSIP 2 (806)\n" +
		"Some Address Line\n" +
		"Vietnam\n" +
		"Store One\n" +
		"8934563112223\n" +
		"Product One Description\n" +
		"Pack\n" +
		"1\n" +
		"6\n" +
		"1\n"

	// Store 2 (page 2, FAILED — no store-name markers at all, same shape
	// as TestRealProcessor_ProcessesBigcDocument_IsolatesPerPageErrors'
	// malformed page): contributes ZERO rows, must not shift store 3's
	// offset by anything other than store 1's real row count.
	page2 := "This page has no store markers at all.\n" +
		"Just some unrelated filler text.\n"

	// Store 3 (page 3, SECOND success, after a failure in between): no
	// note row (isFirstSuccessful=false, store 1 already got it) -> local
	// rows [item(0)] only, mismatch at local index 0.
	page3 := "FM LOGISTIC VSIP 2 (806)\n" +
		"Some Address Line\n" +
		"Vietnam\n" +
		"Store Three\n" +
		"8934563112230\n" +
		"Product Three Description\n" +
		"Pack\n" +
		"1\n" +
		"6\n" +
		"1\n"

	rows, err := rp.processBigcDocument("synthetic_bigc_mismatch_offsets.pdf", []string{page0, page1, page2, page3}, nil)
	if err != nil {
		t.Fatalf("processBigcDocument returned error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("processBigcDocument returned %d rows, want 3 (store1 success, store2 failed, store3 success): %+v", len(rows), rows)
	}
	if rows[1].StatusKind != StatusKindFailed {
		t.Fatalf("rows[1] (store 2) StatusKind = %v, want Failed", rows[1].StatusKind)
	}

	// Store 1: exactly 1 mismatch, at combined-slice index 1 (right after
	// its own note row at index 0) -> absolute ExcelRow = existingRowCount+2
	// (existingRowCount existing rows, 1-indexed rows start at
	// existingRowCount+1, so index 1 within the write is
	// existingRowCount+1+1).
	if len(rows[0].PriceMismatchDetails) != 1 {
		t.Fatalf("store 1: len(PriceMismatchDetails) = %d, want 1: %+v", len(rows[0].PriceMismatchDetails), rows[0].PriceMismatchDetails)
	}
	store1Detail := rows[0].PriceMismatchDetails[0]
	if store1Detail.SKU != "8934563112223" {
		t.Errorf("store 1 mismatch SKU = %q, want %q", store1Detail.SKU, "8934563112223")
	}
	wantStore1Row := existingRowCount + 2
	if store1Detail.ExcelRow != wantStore1Row {
		t.Errorf("store 1 mismatch ExcelRow = %d, want %d (existingRowCount=%d + local index 1 + 1-indexing)", store1Detail.ExcelRow, wantStore1Row, existingRowCount)
	}

	// Store 3 (rows[2], since rows[1] is the failed store 2): exactly 1
	// mismatch, at combined-slice index 2 (store 1 contributed 2 rows:
	// note+item; the failed store contributed 0; store 3's own item is
	// the 3rd row overall, index 2) -> absolute ExcelRow =
	// existingRowCount+3.
	if len(rows[2].PriceMismatchDetails) != 1 {
		t.Fatalf("store 3: len(PriceMismatchDetails) = %d, want 1: %+v", len(rows[2].PriceMismatchDetails), rows[2].PriceMismatchDetails)
	}
	store3Detail := rows[2].PriceMismatchDetails[0]
	if store3Detail.SKU != "8934563112230" {
		t.Errorf("store 3 mismatch SKU = %q, want %q", store3Detail.SKU, "8934563112230")
	}
	wantStore3Row := existingRowCount + 3
	if store3Detail.ExcelRow != wantStore3Row {
		t.Errorf("store 3 mismatch ExcelRow = %d, want %d — if this is existingRowCount+2 instead, the failed store's zero-row contribution was incorrectly skipped in the offset math; if it's some other value, the cross-store snapshot-before-append ordering is wrong", store3Detail.ExcelRow, wantStore3Row)
	}

	// Reopen the actual written workbook and confirm BOTH ExcelRow values
	// point at REAL flagged cells (comment + non-default style) — not
	// just plausible numbers — and that column Q at each row holds the
	// SKU that mismatch detail claims, proving no cross-store row got
	// attributed to the wrong store.
	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()
	comments, err := f.GetComments("Don dat hang")
	if err != nil {
		t.Fatalf("GetComments: %v", err)
	}
	hasComment := func(cell string) bool {
		for _, c := range comments {
			if c.Cell == cell {
				return true
			}
		}
		return false
	}

	for _, d := range []PriceMismatchDetail{store1Detail, store3Detail} {
		cell := fmt.Sprintf("Y%d", d.ExcelRow)
		styleID, err := f.GetCellStyle("Don dat hang", cell)
		if err != nil {
			t.Fatalf("GetCellStyle(%s): %v", cell, err)
		}
		if styleID == 0 {
			t.Errorf("%s has default style, want the red-fill mismatch style — ExcelRow=%d for SKU %s doesn't point at a real flagged cell", cell, d.ExcelRow, d.SKU)
		}
		if !hasComment(cell) {
			t.Errorf("no mismatch comment at %s — ExcelRow=%d for SKU %s doesn't point at a real flagged cell", cell, d.ExcelRow, d.SKU)
		}
		skuCell := fmt.Sprintf("Q%d", d.ExcelRow)
		gotSKU, _ := f.GetCellValue("Don dat hang", skuCell)
		if gotSKU != d.SKU {
			t.Errorf("%s (SKU column at the row this detail claims) = %q, want %q — this row belongs to a DIFFERENT product than the detail claims", skuCell, gotSKU, d.SKU)
		}
	}
}
