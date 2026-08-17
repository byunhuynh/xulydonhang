package processing

import (
	"context"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleBigcFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	// Empty price index on purpose: this file's real barcodes aren't in
	// the small test fixture, so products are expected to come back as
	// price mismatches (Warning), not Done.
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
		if row.StatusKind != StatusKindWarning {
			t.Fatalf("rows[%d].StatusKind = %v, want %v (empty price index -> every product mismatches)", i, row.StatusKind, StatusKindWarning)
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

	rows, err := rp.processBigcDocument("synthetic_bigc_isolation.pdf", []string{page0, page1, page2, page3})
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
