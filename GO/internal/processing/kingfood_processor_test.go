package processing

import (
	"context"
	"path/filepath"
	"strings"
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
	if rows[0].MaKhachHang != "Không xác định" {
		t.Fatalf("MaKhachHang = %q, want %q because the test customer sheet has no matching Kingfood address", rows[0].MaKhachHang, "Không xác định")
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
			name:          "southern code",
			customerCode:  "MN_MT_KFMSL",
			wantRegion:    "MT_MN",
			wantStatCode:  "LA",
			wantWarehouse: "LA_KHO2026",
		},
		{
			name:          "northern code",
			customerCode:  "MB_MT_KFMSL",
			wantRegion:    "MT_MB",
			wantStatCode:  "HN",
			wantWarehouse: "TP_HN_12",
		},
		{
			name:          "legacy JMart exact-match branch",
			customerCode:  "MN_MT_JM0001",
			wantRegion:    "MT_MN",
			wantStatCode:  "LA",
			wantWarehouse: "LA_TP",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRegion, gotStatCode, gotWarehouse := kingfoodRegionInfo(tc.customerCode, nil)
			if gotRegion != tc.wantRegion || gotStatCode != tc.wantStatCode || gotWarehouse != tc.wantWarehouse {
				t.Errorf("kingfoodRegionInfo(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.customerCode, gotRegion, gotStatCode, gotWarehouse,
					tc.wantRegion, tc.wantStatCode, tc.wantWarehouse)
			}
		})
	}
}

func TestRealProcessor_KingfoodUsesDeliveryAddressAndCustomerSheetLookup(t *testing.T) {
	store, err := kingfoodStoreWithCustomerAddress(t, "MB_MT_KFMSL", "Số 10, Phố Huế, Hà Nội")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex([][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156732620", "Viên giặt xả", "52195", ""},
	})}
	processor := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}

	pages, _, err := extractPageTexts("testdata/sample_kingfood_order.pdf")
	if err != nil {
		t.Fatalf("extractPageTexts failed: %v", err)
	}
	const address = "Số 10, Phố Huế, Hà Nội"
	text := strings.Replace(
		pages[0],
		"Địa\tchỉ\tgiao\nhàng:\nSố\t324,\tđường\tĐT743A,\tPhường\tĐông\tHoà,\tThành\tphố\tHồ\tChí\tMinh\n",
		"Địa\tchỉ\tgiao\nhàng:\n"+strings.ReplaceAll(address, " ", "\t")+"\n",
		1,
	)
	if text == pages[0] {
		t.Fatal("failed to replace the sample delivery-place field")
	}

	row, err := processor.processKingfoodSegment("testdata/sample_kingfood_order.pdf", 1, text, "1/1")
	if err != nil {
		t.Fatalf("processKingfoodSegment returned error: %v", err)
	}
	if row.MaKhachHang != "MB_MT_KFMSL" {
		t.Errorf("MaKhachHang = %q, want %q from the MaKH address lookup", row.MaKhachHang, "MB_MT_KFMSL")
	}
	if row.ShipTo != address {
		t.Errorf("ShipTo = %q, want %q", row.ShipTo, address)
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
	const colShipTo = 4
	if got := sheetRows[8][colShipTo]; got != address {
		t.Errorf("header row ShipTo = %q, want extracted address %q", got, address)
	}
	const colCustomerCode = 6
	if got := sheetRows[8][colCustomerCode]; got != "MB_MT_KFMSL" {
		t.Errorf("header row customer code = %q, want mapped code %q", got, "MB_MT_KFMSL")
	}
}

func kingfoodStoreWithCustomerAddress(t *testing.T, code, address string) (*productdata.Store, error) {
	t.Helper()
	sourcePath := "productdata/testdata/data.xlsx"
	tempPath := filepath.Join(t.TempDir(), "productdata.xlsx")
	copyFile(t, sourcePath, tempPath)

	f, err := excelize.OpenFile(tempPath)
	if err != nil {
		return nil, err
	}
	rows, err := f.GetRows("MaKH")
	if err != nil {
		f.Close()
		return nil, err
	}
	rowNumber := len(rows) + 1
	for column, value := range []string{"Kingfood", "KFM-MB", code, address} {
		cell, err := excelize.CoordinatesToCellName(column+1, rowNumber)
		if err != nil {
			f.Close()
			return nil, err
		}
		if err := f.SetCellValue("MaKH", cell, value); err != nil {
			f.Close()
			return nil, err
		}
	}
	if err := f.Save(); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return productdata.Load(tempPath)
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

func TestRealProcessor_KingfoodUsesInvoicePriceWhenSystemPriceIsMissing(t *testing.T) {
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
	if rows[0].PriceMismatchCount != 1 {
		t.Fatalf("PriceMismatchCount = %d, want 1 when the system price is missing", rows[0].PriceMismatchCount)
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

	const colSKU, colUnitPrice = 16, 24
	for _, row := range sheetRows {
		if colSKU < len(row) && row[colSKU] == "8936156732620" {
			if got, want := row[colUnitPrice], "52195.073"; got != want {
				t.Errorf("cột Y (đơn giá) = %q, want PO price %q when system price is unavailable", got, want)
			}
			return
		}
	}
	t.Fatal("không tìm thấy dòng sản phẩm trong sheet")
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
// a literal "52195.073" would parse to 52195073, not 52195.073 — and
// closeEnough's tolerance (an absolute 1đ) comfortably covers the 0.073
// gap between realPrice=52195 and the real invoicePrice=52195.073, so
// this test exercises the matched-price path rather than
// promoGiftOnMismatchRule's.
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

// TestRealProcessor_KingfoodWritesPOPriceWhenPricesMatch pins the
// end-to-end wiring of appliedUnitPrice on a NON-Satra vendor: khi giá
// khớp thì cột Y phải mang đơn giá của PO, không phải giá hệ thống.
//
// Dùng đúng sản phẩm thật của sample_kingfood_order.pdf (barcode
// 8936156732620, đơn giá PO "52.195,073" -> 52195.073). Bảng giá khai
// 52195 chẵn, lệch 0.073đ — nằm trong cửa sổ 1đ của closeEnough nên coi
// là đúng giá, và Y phải là 52195.073 (số của PO), không phải 52195.
//
// Đây là chỗ duy nhất khẳng định trực tiếp hành vi mới trên một nhà
// cung cấp không phải Satra: golden test chỉ CHẤP NHẬN chênh lệch này
// (xem isPOPriceOnMatch) chứ không tự khẳng định chiều của nó.
func TestRealProcessor_KingfoodWritesPOPriceWhenPricesMatch(t *testing.T) {
	const poPrice = "52195.073"

	cases := []struct {
		name        string
		sheetPrice  string
		wantY       string
		wantFlagged bool
	}{
		{
			name:       "khớp giá -> ghi giá PO",
			sheetPrice: "52195", // lệch 0.073đ so với PO
			wantY:      poPrice,
		},
		{
			name:        "lệch quá 1đ -> vẫn ghi giá hệ thống và tô đỏ",
			sheetPrice:  "52190", // lệch 5.073đ so với PO
			wantY:       "52190",
			wantFlagged: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := productdata.Load("productdata/testdata/data.xlsx")
			if err != nil {
				t.Fatalf("Load productdata failed: %v", err)
			}
			excelPath := copyTestWorkbookForProcessor(t)

			priceCsv := [][]string{
				{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
				{"1", "8936156732620", "Viên giặt xả", tc.sheetPrice, ""},
			}
			rp := &RealProcessor{
				Store:     store,
				Pricing:   &fixturePricingSource{index: pricing.ParseIndex(priceCsv)},
				ExcelPath: excelPath,
			}
			row, err := rp.Process(context.Background(), "testdata/sample_kingfood_order.pdf")
			if err != nil {
				t.Fatalf("Process returned error: %v", err)
			}
			if got := row[0].PriceMismatchCount > 0; got != tc.wantFlagged {
				t.Errorf("PriceMismatchCount > 0 = %v, want %v (log: %v)", got, tc.wantFlagged, row[0].SkuLog)
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

			const colSKU, colUnitPrice = 16, 24
			var productRow []string
			for _, r := range sheetRows {
				if colSKU < len(r) && r[colSKU] == "8936156732620" {
					productRow = r
					break
				}
			}
			if productRow == nil {
				t.Fatalf("không tìm thấy dòng sản phẩm trong sheet")
			}
			if got := productRow[colUnitPrice]; got != tc.wantY {
				t.Errorf("cột Y (đơn giá) = %q, want %q", got, tc.wantY)
			}
		})
	}
}
