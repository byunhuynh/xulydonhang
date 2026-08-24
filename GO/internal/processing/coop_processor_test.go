package processing

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/xuri/excelize/v2"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

type failAfterFirstStreamPricingSource struct {
	index *pricing.Index
	fail  bool
}

func (s *failAfterFirstStreamPricingSource) FetchIndex(string) (*pricing.Index, error) {
	if s.fail {
		return nil, errors.New("pricing source switched off after first streamed row")
	}
	return s.index, nil
}

// TestRealProcessorStreamsEachCompletedSegment proves that a completed
// segment is reported before processing the next one. The callback makes the
// second page's pricing fetch fail; that failure is possible only when the
// first page was emitted immediately, rather than after the whole file.
func TestRealProcessorStreamsEachCompletedSegment(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá"},
		{"1", "1234567", "Nước giặt", "141.272"},
	}
	priceIndex := pricing.ParseIndex(priceCsv)
	fixturePath := filepath.Join(t.TempDir(), "multi-page-coop.pdf")
	if err := api.MergeCreateFile([]string{"testdata/sample_coop_order.pdf", "testdata/sample_coop_order.pdf"}, fixturePath, false, nil); err != nil {
		t.Fatalf("build two-page fixture from sample Coop PDF: %v", err)
	}

	baseline := &RealProcessor{
		Store: store, Pricing: &fixturePricingSource{index: priceIndex}, ExcelPath: copyTestWorkbookForProcessor(t),
	}
	baselineRows, err := baseline.Process(context.Background(), fixturePath, 1)
	if err != nil {
		t.Fatalf("baseline Process returned error: %v", err)
	}
	if len(baselineRows) != 2 {
		t.Fatalf("baseline Process returned %d rows, want 2 for the two-page fixture: %+v", len(baselineRows), baselineRows)
	}
	for _, row := range baselineRows {
		if row.StatusKind == StatusKindFailed {
			t.Fatalf("baseline row unexpectedly failed: %+v", row)
		}
	}

	pricingSource := &failAfterFirstStreamPricingSource{index: priceIndex}
	rp := &RealProcessor{
		Store: store, Pricing: pricingSource, ExcelPath: copyTestWorkbookForProcessor(t),
	}
	var emitted []OrderRow
	var callbackLengths []int
	rows, err := rp.ProcessStreaming(context.Background(), fixturePath, 1, func(row OrderRow) {
		emitted = append(emitted, row)
		callbackLengths = append(callbackLengths, len(emitted))
		if len(emitted) == 1 {
			pricingSource.fail = true
		}
	})
	if err != nil {
		t.Fatalf("ProcessStreaming returned error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("ProcessStreaming returned %d rows, want 2: %+v", len(rows), rows)
	}
	if rows[1].StatusKind != StatusKindFailed {
		t.Fatalf("second returned row StatusKind = %q, want %q after first row streams", rows[1].StatusKind, StatusKindFailed)
	}
	if len(emitted) != len(rows) {
		t.Fatalf("emitted %d rows, want %d returned rows", len(emitted), len(rows))
	}
	for i, row := range rows {
		if callbackLengths[i] != i+1 {
			t.Errorf("callback length after segment %d = %d, want %d", i+1, callbackLengths[i], i+1)
		}
		if emitted[i].StatusKind != row.StatusKind {
			t.Errorf("emitted row %d StatusKind = %q, want returned row's %q", i, emitted[i].StatusKind, row.StatusKind)
		}
		if emitted[i].ResultKey != row.ResultKey {
			t.Errorf("emitted row %d ResultKey = %q, want returned row's %q", i, emitted[i].ResultKey, row.ResultKey)
		}
		wantKey := orderResultKey(SourceIDForPath(fixturePath), fmt.Sprintf("page:%d:segment:1", i+1), row.PO)
		if row.ResultKey != wantKey {
			t.Errorf("returned row %d ResultKey = %q, want %q", i, row.ResultKey, wantKey)
		}
	}
	if emitted[1].StatusKind != StatusKindFailed {
		t.Fatalf("second emitted row StatusKind = %q, want %q", emitted[1].StatusKind, StatusKindFailed)
	}
}

func TestRealProcessor_ResultIdentityDistinguishesDuplicateFailedSegments(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	fixturePath := filepath.Join(t.TempDir(), "duplicate-failures.pdf")
	if err := api.MergeCreateFile([]string{"testdata/sample_coop_order.pdf", "testdata/sample_coop_order.pdf"}, fixturePath, false, nil); err != nil {
		t.Fatalf("build two-page fixture: %v", err)
	}

	rp := &RealProcessor{
		Store: store,
		Pricing: &failAfterFirstStreamPricingSource{
			index: pricing.ParseIndex(nil),
			fail:  true,
		},
		ExcelPath: copyTestWorkbookForProcessor(t),
	}
	rows, err := rp.Process(context.Background(), fixturePath, 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("Process returned %d rows, want two failed physical pages: %+v", len(rows), rows)
	}
	if rows[0].StatusKind != StatusKindFailed || rows[1].StatusKind != StatusKindFailed {
		t.Fatalf("rows = %+v, want two failed rows", rows)
	}
	if rows[0].Page != "1/1" || rows[1].Page != "1/1" || rows[0].PO != "" || rows[1].PO != "" {
		t.Fatalf("fixture no longer reproduces duplicate display identity: %+v", rows)
	}
	if rows[0].SourceID == "" || rows[0].SourceID != rows[1].SourceID {
		t.Fatalf("SourceID values = %q, %q; want one stable non-empty source", rows[0].SourceID, rows[1].SourceID)
	}
	if rows[0].ResultKey == rows[1].ResultKey {
		t.Fatalf("duplicate failed physical pages collapsed to ResultKey %q", rows[0].ResultKey)
	}
}

func TestRealProcessor_SourceIdentityDistinguishesSameBasenamePaths(t *testing.T) {
	left := filepath.Join(t.TempDir(), "same.pdf")
	right := filepath.Join(t.TempDir(), "same.pdf")
	rp := &RealProcessor{}

	leftRows, err := rp.Process(context.Background(), left, 1)
	if err != nil {
		t.Fatalf("left Process returned error: %v", err)
	}
	rightRows, err := rp.Process(context.Background(), right, 1)
	if err != nil {
		t.Fatalf("right Process returned error: %v", err)
	}
	if len(leftRows) != 1 || len(rightRows) != 1 {
		t.Fatalf("left/right rows = %d/%d, want one file-level failure each", len(leftRows), len(rightRows))
	}
	leftRow, rightRow := leftRows[0], rightRows[0]
	if leftRow.FileName != "same.pdf" || rightRow.FileName != "same.pdf" {
		t.Fatalf("display filenames changed: %q, %q", leftRow.FileName, rightRow.FileName)
	}
	if leftRow.SourceID == "" || rightRow.SourceID == "" || leftRow.SourceID == rightRow.SourceID {
		t.Fatalf("SourceID values = %q, %q; want distinct non-empty source identities", leftRow.SourceID, rightRow.SourceID)
	}
	if leftRow.ResultKey == rightRow.ResultKey {
		t.Fatalf("same-basename sources collapsed to ResultKey %q", leftRow.ResultKey)
	}
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
// a bug a review found in the plan's original reference code for
// coop_processor.go's promo/bonus-row logic, re-traced against
// xulydonhang.py:1085,1174-1176,1201,1211: the FIRST promo item (index 0)
// split by "|" writes its note/bundle-SKU onto the MAIN PRODUCT ROW (not
// its own bonus row); every later item (index > 0) writes onto its own
// bonus row instead.
//
// This uses the real sample PDF's actual first product barcode (cleaned to
// "3564270") together with two internal SKUs already present in the
// productdata test fixture (SP0001, SP0002) as the promo's bonus-item
// mentions. Price is "33726" (the real extracted invoice price for this
// barcode, confirmed by direct probe), not an arbitrary value: the note/
// bundle-SKU/bonus-row logic under test now only runs once matched is
// true (see processCoopSegment's own "Gated on matched" comment), so this
// test needs a genuinely matching price to exercise it. The companion
// mismatch scenario (khuyenmai still populated, but no note/bonus row at
// all) is covered separately by
// TestRealProcessor_PromoContentSetButNoBonusRowOnMismatch below.
func TestRealProcessor_PromoBonusRowFieldsMatchPythonRowTarget(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	const promoValue = "cm Tang SP0001 {Bó Kèm - Che Barcode}|Tang SP0002 {Combo 2}"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "3564270", "Nước giặt", "33726", promoValue},
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

	if got := cell(mainRow, colPromoContent); got != promoValue {
		t.Errorf("main row PromoContent = %q, want %q", got, promoValue)
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

// TestRealProcessor_PromoContentSetButNoBonusRowOnMismatch regression-tests
// the companion half of the bug
// TestRealProcessor_PromoBonusRowFieldsMatchPythonRowTarget covers: khuyenmai
// (PromoContent) is set on every examined promo candidate, not only on a
// price match, so it must still be populated when the product row ends up
// a price mismatch (xulydonhang.py:1085) — but, per this Go port's
// deliberate divergence from Python (see processCoopSegment's own "Gated
// on matched" comment), the note/bundle-SKU/bonus-row machinery must NOT
// run at all on a genuine mismatch, since a gift is only ever part of the
// SAME CTKM that's confirmed to explain the invoice price.
//
// Uses the same real sample PDF/barcode/promo as the sibling test above,
// but with a price ("500000") engineered to NOT match the real invoice
// price (33726), so matched stays false throughout.
func TestRealProcessor_PromoContentSetButNoBonusRowOnMismatch(t *testing.T) {
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

	const colSKU, colPromoNote, colPromoBundleSku, colPromoContent = 16, 40, 41, 42
	cell := func(row []string, idx int) string {
		if idx < len(row) {
			return row[idx]
		}
		return ""
	}

	var mainRow []string
	for _, row := range sheetRows {
		switch cell(row, colSKU) {
		case "3564270":
			mainRow = row
		case "SP0001", "SP0002":
			t.Errorf("found a %s bonus row, want none: a genuine price mismatch must not build a gift row", cell(row, colSKU))
		}
	}
	if mainRow == nil {
		t.Fatalf("missing expected main row")
	}

	if got := cell(mainRow, colPromoContent); got != promoValue {
		t.Errorf("main row PromoContent = %q, want %q (khuyenmai must be set even on mismatch)", got, promoValue)
	}
	if got := cell(mainRow, colPromoNote); got != "" {
		t.Errorf("main row PromoNote = %q, want empty (no confirmed CTKM on a genuine mismatch)", got)
	}
	if got := cell(mainRow, colPromoBundleSku); got != "" {
		t.Errorf("main row PromoBundleSku = %q, want empty (no confirmed CTKM on a genuine mismatch)", got)
	}
}
