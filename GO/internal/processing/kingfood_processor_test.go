package processing

import (
	"context"
	"testing"

	"github.com/xuri/excelize/v2"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleKingfoodFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	pricingSource := &fixturePricingSource{index: pricing.ParseIndex([][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
	})}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_kingfood_order.pdf")
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].System != "Kingfood" {
		t.Fatalf("System = %q, want %q", rows[0].System, "Kingfood")
	}
	if rows[0].StatusKind == StatusKindFailed {
		t.Fatalf("Process produced a Failed row: %+v", rows[0])
	}
	if rows[0].MaKhachHang != kingfoodCustomerCode {
		t.Fatalf("MaKhachHang = %q, want the hardcoded constant %q", rows[0].MaKhachHang, kingfoodCustomerCode)
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
	// 1 header + 1 product = 2 new rows (the real sample has 1 product);
	// no promo bonus row expected since the synthetic pricing source
	// above has no real promo data.
	if len(sheetRows) != 8+2 {
		t.Fatalf("total rows = %d, want %d (8 template + 1 header + 1 product)", len(sheetRows), 8+2)
	}
}

func TestKingfoodRegionInfo(t *testing.T) {
	cases := []struct {
		name                                    string
		customerCode                            string
		wantRegion, wantStatCode, wantWarehouse string
	}{
		{
			name:          "the real, always-used hardcoded constant (else branch)",
			customerCode:  kingfoodCustomerCode,
			wantRegion:    "MT_MN",
			wantStatCode:  "LA",
			wantWarehouse: "LA_KHO2026",
		},
		{
			name:          "MB branch (unreachable with real input today, still tested)",
			customerCode:  "MB_SOMETHING",
			wantRegion:    "MT_MB",
			wantStatCode:  "HN",
			wantWarehouse: "TP_HN_12",
		},
		{
			name:          "MN_MT_JM0001 exact-match branch (unreachable with real input today, still tested)",
			customerCode:  "MN_MT_JM0001",
			wantRegion:    "MT_MN",
			wantStatCode:  "LA",
			wantWarehouse: "LA_TP",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRegion, gotStatCode, gotWarehouse := kingfoodRegionInfo(tc.customerCode)
			if gotRegion != tc.wantRegion || gotStatCode != tc.wantStatCode || gotWarehouse != tc.wantWarehouse {
				t.Errorf("kingfoodRegionInfo(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.customerCode, gotRegion, gotStatCode, gotWarehouse,
					tc.wantRegion, tc.wantStatCode, tc.wantWarehouse)
			}
		})
	}
}

func TestParseKingfoodPrice(t *testing.T) {
	cases := []struct {
		input string
		want  float64
	}{
		{"52.195,073", 52195.073},
		{"1.252.682", 1252682},
		{"85.000", 85000},
		{"0", 0},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got := parseKingfoodPrice(c.input)
			if got != c.want {
				t.Errorf("parseKingfoodPrice(%q) = %v, want %v", c.input, got, c.want)
			}
		})
	}
}

// TestRealProcessor_KingfoodNoBraceBonusRowDoesNotWriteAP regression-tests
// Kingfood's own no-{...}-brace fallback text ("KM Giao Rời - Không Che
// Barcode", xulydonhang.py:4096) — the shared buildPromoBonusRow's
// default fallback ("KM Bó Kèm - Che Barcode") must be overridden at
// this call site, AND (unlike FujiMart, like Winmart/Emart) column AP
// must be cleared, not left at buildPromoBonusRow's own default.
//
// Uses sample_kingfood_order.pdf's real single product (barcode
// 8936156732620, OU Qty 300, price "52.195,073" -> parseKingfoodPrice =
// 52195.073 — confirmed by direct extraction during planning) with a
// "2+1 SP0002" promo (an "X+1" match mentioning SP0002, a known internal
// SKU already present in the productdata test fixture) and NO {...}
// braces. The price CSV's own "Giá" column is "52195" (a bare integer,
// not "52195.073"): pricing.Index.ParseIndex strips every "." from that
// column as a Vietnamese thousands separator (never a decimal point), so
// a literal "52195.073" would parse to 52195073, not 52195.073 — this
// bonus row now only builds once matched is true (see the per-item promo
// bonus row's own "Gated on matched" comment in kingfood_processor.go),
// and closeEnough's tolerance (relTol 1e-4) comfortably covers the 0.073
// gap between realPrice=52195 and the real invoicePrice=52195.073.
func TestRealProcessor_KingfoodNoBraceBonusRowDoesNotWriteAP(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	const promoValue = "2+1 SP0002"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156732620", "Viên giặt xả", "52195", promoValue},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_kingfood_order.pdf"); err != nil {
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
		case "8936156732620":
			mainRow = row
		case "SP0002":
			bonusRow = row
		}
	}
	if mainRow == nil || bonusRow == nil {
		t.Fatalf("missing expected rows: main=%v bonus=%v", mainRow, bonusRow)
	}

	if got := cell(mainRow, colPromoNote); got != "KM Giao Rời - Không Che Barcode" {
		t.Errorf("main row PromoNote (AO) = %q, want %q (Kingfood's own no-brace fallback)", got, "KM Giao Rời - Không Che Barcode")
	}
	if got := cell(mainRow, colPromoBundleSku); got != "" {
		t.Errorf("main row PromoBundleSku (AP) = %q, want empty (Kingfood's no-brace branch does NOT write AP, matching Winmart/Emart, unlike FujiMart)", got)
	}
	if got := cell(bonusRow, colPromoBundleSku); got != "" {
		t.Errorf("bonus row PromoBundleSku (AP) = %q, want empty", got)
	}
}

// TestRealProcessor_KingfoodInvoiceLevelPromoBonusRow covers the
// invoice-level ("Hóa Đơn") promo bonus row (xulydonhang.py:4131-4177)
// — Q gets only the FIRST mentioned SKU, not a joined list, same
// divergence already handled for Winmart/Emart/FujiMart.
func TestRealProcessor_KingfoodInvoiceLevelPromoBonusRow(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156732620", "Viên giặt xả", "52195.073", ""},
		{"2", "Hóa Đơn", "", "0", "100000 SP0001 SP0002"},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_kingfood_order.pdf"); err != nil {
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
	// wantQty: pricing.ParseIndex strips "." from the Giá column (matching
	// Python's own find_price_by_sku's period-stripped/thousands-separator
	// convention, shared by every vendor's pricing sheet, since VND has no
	// fractional currency) BEFORE parseNumericField parses it — so the
	// fixture's "52195.073" price cell is read back as the plain integer
	// 52195073, not the decimal 52195.073. floor(300 (OU Qty) * 52195073
	// (unit price after "."-stripping) / 100000) = 156585, not the naively
	// decimal-preserving 156 (flagged in task-4-report.md as a brief-test
	// discrepancy — this is the correct value given ParseIndex's existing,
	// verified-correct, unmodified behavior; not a production code change).
	wantQty := "156585"
	if got := cell(bonusRow, colQty); got != wantQty {
		t.Errorf("invoice bonus row Qty (X) = %q, want %q (floor(totalValue / amount))", got, wantQty)
	}

	for _, row := range sheetRows {
		if cell(row, colSKU) == "SP0002" {
			t.Errorf("found a row with SKU (Q) = %q, want none (only the first mentioned SKU, SP0001, should get an invoice bonus row)", "SP0002")
		}
	}
}
