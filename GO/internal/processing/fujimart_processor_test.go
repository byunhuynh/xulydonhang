package processing

import (
	"context"
	"testing"

	"github.com/xuri/excelize/v2"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleFujiMartFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	pricingSource := &fixturePricingSource{index: pricing.ParseIndex([][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
	})}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_fujimart_order.pdf")
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].System != "FujiMart" {
		t.Fatalf("System = %q, want %q", rows[0].System, "FujiMart")
	}
	if rows[0].StatusKind == StatusKindFailed {
		t.Fatalf("Process produced a Failed row: %+v", rows[0])
	}
	if rows[0].MaKhachHang != fujimartCustomerCode {
		t.Fatalf("MaKhachHang = %q, want the hardcoded constant %q", rows[0].MaKhachHang, fujimartCustomerCode)
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
	// 1 header + 5 products = 6 new rows, none should have a promo bonus
	// row since the synthetic pricing source above has no real promo data.
	if len(sheetRows) != 8+6 {
		t.Fatalf("total rows = %d, want %d (8 template + 1 header + 5 products)", len(sheetRows), 8+6)
	}
}

func TestFujimartRegionInfo(t *testing.T) {
	cases := []struct {
		name                                    string
		customerCode                            string
		wantRegion, wantStatCode, wantWarehouse string
	}{
		{
			name:          "the real, always-used hardcoded constant (MB branch)",
			customerCode:  fujimartCustomerCode,
			wantRegion:    "MT_MB",
			wantStatCode:  "HN",
			wantWarehouse: "TP_HN_12",
		},
		{
			name:          "non-MB code (unreachable with real input today, still tested)",
			customerCode:  "MN_SOMETHING",
			wantRegion:    "MT_MN",
			wantStatCode:  "LA",
			wantWarehouse: "LA_KHO2026",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRegion, gotStatCode, gotWarehouse := fujimartRegionInfo(tc.customerCode, nil)
			if gotRegion != tc.wantRegion || gotStatCode != tc.wantStatCode || gotWarehouse != tc.wantWarehouse {
				t.Errorf("fujimartRegionInfo(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.customerCode, gotRegion, gotStatCode, gotWarehouse,
					tc.wantRegion, tc.wantStatCode, tc.wantWarehouse)
			}
		})
	}
}

// TestRealProcessor_FujimartNoBraceBonusRowUsesKMBoKemKhongCheBarcode
// regression-tests FujiMart's own no-{...}-brace fallback text
// ("KM Bó Kèm - Không Che Barcode", xulydonhang.py:2973) — the shared
// buildPromoBonusRow's default fallback ("KM Bó Kèm - Che Barcode")
// must be overridden at this call site. Unlike Winmart's/Emart's
// equivalent fix, FujiMart's own fallback STILL writes AP (both texts
// contain "bó kèm", so buildPromoBonusRow's own bundle detection is
// unaffected by the text override) — this test explicitly confirms AP
// stays populated, not cleared.
//
// Uses sample_fujimart_order.pdf's real first product (barcode
// 8936156730879, OU Qty 12.0, Total Price 1,695,264 -> per-unit
// giahoadon = 1695264/12 = 141272 — confirmed by direct extraction
// during planning) with a "2+1 SP0002" promo (an "X+1" match mentioning
// SP0002, a known internal SKU already present in the productdata test
// fixture — see TestFindSkusMentioned) and NO {...} braces.
func TestRealProcessor_FujimartNoBraceBonusRowUsesKMBoKemKhongCheBarcode(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	const promoValue = "2+1 SP0002"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156730879", "Nước giặt", "141272", promoValue},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_fujimart_order.pdf"); err != nil {
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
		case "8936156730879":
			mainRow = row
		case "SP0002":
			bonusRow = row
		}
	}
	if mainRow == nil || bonusRow == nil {
		t.Fatalf("missing expected rows: main=%v bonus=%v", mainRow, bonusRow)
	}

	if got := cell(mainRow, colPromoNote); got != "KM Bó Kèm - Không Che Barcode" {
		t.Errorf("main row PromoNote (AO) = %q, want %q (FujiMart's own no-brace fallback)", got, "KM Bó Kèm - Không Che Barcode")
	}
	if got := cell(mainRow, colPromoBundleSku); got == "" {
		t.Errorf("main row PromoBundleSku (AP) = %q, want NON-empty (FujiMart's no-brace branch DOES write AP, unlike Winmart/Emart)", got)
	}
	if got := cell(bonusRow, colPromoNote); got != "" {
		t.Errorf("bonus row PromoNote (AO) = %q, want empty (Python only ever writes AO onto the main row for FujiMart, never the bonus row)", got)
	}
}

// TestRealProcessor_FujimartInvoiceLevelPromoBonusRow covers the
// invoice-level ("Hóa Đơn") promo bonus row (xulydonhang.py:3010-3047)
// — Q gets only the FIRST mentioned SKU, not a joined list, same
// divergence already handled for Winmart/Emart.
//
// Uses all 5 of sample_fujimart_order.pdf's real products at their exact
// real per-unit price (confirmed against 103001302608001342.pdf:
// 12*141272 + 12*141272 + 12*40903 + 12*40903 + 12*37706 = 4,824,672,
// which matches the real PDF's own printed pre-VAT total exactly), so
// totalValue is a known, real, independently-confirmed constant.
func TestRealProcessor_FujimartInvoiceLevelPromoBonusRow(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156730879", "SP1", "141272", ""},
		{"2", "8936156730886", "SP2", "141272", ""},
		{"3", "8936156730473", "SP3", "40903", ""},
		{"4", "8936156730466", "SP4", "40903", ""},
		{"5", "8809174900138", "SP5", "37706", ""},
		{"6", "Hóa Đơn", "", "0", "100000 SP0001 SP0002"},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_fujimart_order.pdf"); err != nil {
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

	const colSKU, colIsPromoItem, colQty = 16, 20, 23
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
	if got := cell(bonusRow, colQty); got != "48" {
		t.Errorf("invoice bonus row Qty (X) = %q, want %q (floor(totalValue=4824672 / amount=100000))", got, "48")
	}

	for _, row := range sheetRows {
		if cell(row, colSKU) == "SP0002" {
			t.Errorf("found a row with SKU (Q) = %q, want none (only the first mentioned SKU, SP0001, should get an invoice bonus row)", "SP0002")
		}
	}
}
