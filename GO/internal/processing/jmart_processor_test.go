package processing

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleJMartFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	pricingSource := &fixturePricingSource{index: pricing.ParseIndex([][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
	})}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_jmart_order.pdf")
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].System != "JMart" {
		t.Fatalf("System = %q, want %q", rows[0].System, "JMart")
	}
	if rows[0].StatusKind == StatusKindFailed {
		t.Fatalf("Process produced a Failed row: %+v", rows[0])
	}
	if rows[0].MaKhachHang != jmartCustomerCode {
		t.Fatalf("MaKhachHang = %q, want the hardcoded constant %q", rows[0].MaKhachHang, jmartCustomerCode)
	}
	if rows[0].PO != "DH01010844" {
		t.Fatalf("PO = %q, want %q", rows[0].PO, "DH01010844")
	}
	// SkuLog: one diagnostic line per real product (3), populated end-to-
	// end through the real extraction -> price-match -> formatSkuLogLine
	// pipeline, not just the isolated formatter unit tests. This
	// pricingSource has no real price data (FindPrice returns "" for
	// every barcode), so every real product's invoice price legitimately
	// fails to match a 0 system price — confirming the mismatch-message
	// branch fires on real extracted data, with the real barcode present.
	if len(rows[0].SkuLog) != 3 {
		t.Fatalf("len(SkuLog) = %d, want 3 (one per real product); got %v", len(rows[0].SkuLog), rows[0].SkuLog)
	}
	for i, line := range rows[0].SkuLog {
		if !strings.Contains(line, "SAI GIÁ") {
			t.Errorf("SkuLog[%d] = %q, want it to contain %q (no real price data in this test's pricingSource)", i, line, "SAI GIÁ")
		}
	}
	if !strings.Contains(rows[0].SkuLog[0], "8936156730886") {
		t.Errorf("SkuLog[0] = %q, want it to contain the real first product's barcode %q", rows[0].SkuLog[0], "8936156730886")
	}
	// PriceMismatchCount: same "saigia" count already reflected in
	// Status's text, now also exposed as its own typed field. All 3 real
	// products mismatch here (same reasoning as the SkuLog check above),
	// so StatusKind must be "warning" (not "done") and the count must be
	// exactly 3.
	if rows[0].StatusKind != StatusKindWarning {
		t.Errorf("StatusKind = %q, want %q (all 3 real products mismatch with this test's empty pricing data)", rows[0].StatusKind, StatusKindWarning)
	}
	if rows[0].PriceMismatchCount != 3 {
		t.Errorf("PriceMismatchCount = %d, want 3", rows[0].PriceMismatchCount)
	}

	// PriceMismatchDetails: same 3 real mismatches, now as structured
	// per-SKU detail — verify not just the computed values but that
	// ExcelRow genuinely points at the real cell excelwriter flagged
	// (comment + non-default style), by reopening the written workbook
	// directly rather than trusting the arithmetic alone.
	if len(rows[0].PriceMismatchDetails) != 3 {
		t.Fatalf("len(PriceMismatchDetails) = %d, want 3", len(rows[0].PriceMismatchDetails))
	}
	firstDetail := rows[0].PriceMismatchDetails[0]
	if firstDetail.SKU != "8936156730886" {
		t.Errorf("PriceMismatchDetails[0].SKU = %q, want %q", firstDetail.SKU, "8936156730886")
	}
	if firstDetail.SystemPrice != 0 {
		t.Errorf("PriceMismatchDetails[0].SystemPrice = %v, want 0 (this test's pricingSource has no real price data)", firstDetail.SystemPrice)
	}
	if firstDetail.InvoicePrice <= 0 {
		t.Errorf("PriceMismatchDetails[0].InvoicePrice = %v, want a real positive extracted price", firstDetail.InvoicePrice)
	}

	fVerify, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening workbook for ExcelRow verification: %v", err)
	}
	defer fVerify.Close()
	verifyCell := fmt.Sprintf("Y%d", firstDetail.ExcelRow)
	styleID, err := fVerify.GetCellStyle("Don dat hang", verifyCell)
	if err != nil {
		t.Fatalf("GetCellStyle(%s): %v", verifyCell, err)
	}
	if styleID == 0 {
		t.Errorf("ExcelRow=%d (cell %s) has default style, want the red-fill mismatch style — ExcelRow doesn't point at the real flagged cell", firstDetail.ExcelRow, verifyCell)
	}
	comments, err := fVerify.GetComments("Don dat hang")
	if err != nil {
		t.Fatalf("GetComments: %v", err)
	}
	foundComment := false
	for _, c := range comments {
		if c.Cell == verifyCell {
			foundComment = true
		}
	}
	if !foundComment {
		t.Errorf("no mismatch comment found at %s — ExcelRow=%d doesn't point at the real flagged cell", verifyCell, firstDetail.ExcelRow)
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
	// 1 header + 3 products = 4 new rows (the real sample has 3
	// products); no promo bonus row expected since the synthetic
	// pricing source above has no real promo data.
	if len(sheetRows) != 8+4 {
		t.Fatalf("total rows = %d, want %d (8 template + 1 header + 3 products)", len(sheetRows), 8+4)
	}
}

func TestJMartUsesKingfoodRegionInfoDirectly(t *testing.T) {
	// Confirms processJMartSegment calls the EXISTING, unmodified
	// kingfoodRegionInfo helper — not a JMart-specific copy — by
	// checking that kingfoodRegionInfo's own MN_MT_JM0001 branch (which
	// exists in kingfood_processor.go specifically for JMart's sake,
	// per that file's own doc comment) produces exactly what JMart's
	// real hardcoded customer code needs.
	region, statCode, warehouse := kingfoodRegionInfo(jmartCustomerCode, nil)
	if region != "MT_MN" || statCode != "LA" || warehouse != "LA_TP" {
		t.Errorf("kingfoodRegionInfo(%q) = (%q, %q, %q), want (\"MT_MN\", \"LA\", \"LA_TP\")",
			jmartCustomerCode, region, statCode, warehouse)
	}
}

// TestRealProcessor_JMartNoBraceBonusRowDoesNotWriteAP regression-tests
// that JMart's promo-fallback logic matches Kingfood's exactly (since
// both are produced by the same real Python function,
// write_to_dondathang_kingfood) — the no-{...}-brace fallback text
// ("KM Giao Rời - Không Che Barcode") must NOT write column AP.
//
// Uses sample_jmart_order.pdf's real first product (barcode
// 8936156730886, OU Qty 8, price "133806.000" — confirmed by direct
// extraction during planning) with a "2+1 SP0002" promo and NO {...}
// braces.
func TestRealProcessor_JMartNoBraceBonusRowDoesNotWriteAP(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	const promoValue = "2+1 SP0002"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156730886", "Nước giặt xả", "133806", promoValue},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_jmart_order.pdf"); err != nil {
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
		case "8936156730886":
			mainRow = row
		case "SP0002":
			bonusRow = row
		}
	}
	if mainRow == nil || bonusRow == nil {
		t.Fatalf("missing expected rows: main=%v bonus=%v", mainRow, bonusRow)
	}

	if got := cell(mainRow, colPromoNote); got != "KM Giao Rời - Không Che Barcode" {
		t.Errorf("main row PromoNote (AO) = %q, want %q (JMart's own no-brace fallback, via shared write_to_dondathang_kingfood)", got, "KM Giao Rời - Không Che Barcode")
	}
	if got := cell(mainRow, colPromoBundleSku); got != "" {
		t.Errorf("main row PromoBundleSku (AP) = %q, want empty (no-brace branch does NOT write AP)", got)
	}
	if got := cell(bonusRow, colPromoBundleSku); got != "" {
		t.Errorf("bonus row PromoBundleSku (AP) = %q, want empty", got)
	}
}
