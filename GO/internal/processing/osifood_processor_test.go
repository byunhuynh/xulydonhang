package processing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/misapush"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

const osifoodRealOrder = "DH-HA-THANH-LA1-Binh-Loi-12.10.pdf"

// osifoodPricingRows is an OSIFOOD price tab holding the campaign the real
// order was placed for: 16–31/10, four days AFTER the order date.
func osifoodPricingRows() [][]string {
	return [][]string{
		{"STT", "Mã hàng", "TÊN SẢN PHẨM", "ĐƠN GIÁ", "16/10-31/10"},
		{"1", "TP30244_02", "Nước giặt xả Blue Thảo Mộc túi 2.1L", "110.501", "Mua 1 tặng 1 (1+1) {KM Quấn Kèm - Che Barcode}"},
		{"2", "TP31647", "Nước rửa chén Blue Không Mùi túi 3.2L", "80.900", "Giảm 30%"},
		{"3", "TP31630", "Nước rửa chén Blue hương Chanh túi 3.2L", "80.900", "Giảm 30%"},
	}
}

// osifoodTestProcessor builds a processor over the shared data.xlsx
// fixture plus the rows the live sheets carry for this vendor, added to a
// temporary copy so the shared fixture itself stays untouched: one MaKH
// row per store, and the three products with their barcodes.
func osifoodTestProcessor(t *testing.T, pricingRows [][]string) (*RealProcessor, string) {
	t.Helper()
	data, err := os.ReadFile("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("failed reading productdata fixture: %v", err)
	}
	dataPath := filepath.Join(t.TempDir(), "data.xlsx")
	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		t.Fatalf("failed writing temp productdata: %v", err)
	}
	f, err := excelize.OpenFile(dataPath)
	if err != nil {
		t.Fatalf("failed opening temp productdata: %v", err)
	}
	appendRow := func(sheet string, values ...interface{}) {
		rows, err := f.GetRows(sheet)
		if err != nil {
			t.Fatalf("GetRows(%s): %v", sheet, err)
		}
		cell, _ := excelize.CoordinatesToCellName(1, len(rows)+1)
		if err := f.SetSheetRow(sheet, cell, &values); err != nil {
			t.Fatalf("SetSheetRow(%s): %v", sheet, err)
		}
	}
	appendRow("MaKH", "Osifood", "MN_MT_OF68014", "MN_MT_OF68014", "127 Bình Lợi, Phường 13, Bình Thạnh")
	// Two of the five stores that ordered through the campaign workbook;
	// the other three have no code, as in the live sheet today.
	appendRow("MaKH", "Osifood", "MN_MT_OF79001", "MN_MT_OF79001", "288 Phan Văn Trị, Bình Thạnh")
	appendRow("MaKH", "Osifood", "MN_MT_OF79003", "MN_MT_OF79003", "CC Sky 09, Thủ Đức")
	// Barcode in column K, where the live SanPham sheet keeps it.
	appendRow("SanPham", "TP30244_02", "Nước Giặt xả Blue Đậm đặc Hương Thảo Mộc túi 2.1L", 2.3, 8, nil, nil, nil, nil, nil, nil, "8936156730244")
	appendRow("SanPham", "TP31647", "Nước rửa chén Blue Không Mùi túi 3.2L (QC*4)", 3.48, 4, nil, nil, nil, nil, nil, nil, "8936156731647")
	appendRow("SanPham", "TP31630", "Nước rửa chén Blue hương Chanh túi 3.2L (QC*4)", 3.48, 4, nil, nil, nil, nil, nil, nil, "8936156731630")
	appendRow("SanPham", "TP30329_02", "Nước giặt xả Blue Đậm đặc Hương Nước Hoa túi 2.1L", 2.3, 8, nil, nil, nil, nil, nil, nil, "8936156730329")
	appendRow("SanPham", "TP30879_02", "Nước giặt xả Blue Đậm đặc Hương Thảo Mộc túi 3.6L", 3.8, 4, nil, nil, nil, nil, nil, nil, "8936156730879")
	appendRow("SanPham", "TP30886_02", "Nước giặt xả Blue Đậm đặc Hương Nước Hoa túi 3.6L", 3.8, 4, nil, nil, nil, nil, nil, nil, "8936156730886")
	if err := f.Save(); err != nil {
		t.Fatalf("failed saving temp productdata: %v", err)
	}
	f.Close()

	store, err := productdata.Load(dataPath)
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)
	return &RealProcessor{
		Store:     store,
		Pricing:   &fixturePricingSource{index: pricing.ParseIndex(pricingRows)},
		ExcelPath: excelPath,
	}, excelPath
}

func processOsifoodRealOrder(t *testing.T, rp *RealProcessor) OrderRow {
	t.Helper()
	rows, err := rp.Process(context.Background(), filepath.Join("osifood", "testdata", "realpdfs", osifoodRealOrder))
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1: %+v", len(rows), rows)
	}
	return rows[0]
}

func TestRealProcessor_ProcessesRealOsifoodOrder(t *testing.T) {
	rp, excelPath := osifoodTestProcessor(t, osifoodPricingRows())
	row := processOsifoodRealOrder(t, rp)

	if row.StatusKind != StatusKindDone {
		t.Fatalf("StatusKind = %q (%s), want %q\nlog: %s", row.StatusKind, row.Status, StatusKindDone, strings.Join(row.SkuLog, "\n"))
	}
	if row.System != "Osifood" {
		t.Errorf("System = %q, want %q", row.System, "Osifood")
	}
	if row.MaKhachHang != "MN_MT_OF68014" {
		t.Errorf("MaKhachHang = %q, want store 68014's own code from the MaKH sheet", row.MaKhachHang)
	}
	// The order prints no number: store code + order date.
	if row.PO != "68014-121026" {
		t.Errorf("PO = %q, want %q", row.PO, "68014-121026")
	}
	if row.EntryDate != "12/10/2026" || row.CancelDate != "19/10/2026" {
		t.Errorf("EntryDate/CancelDate = %q/%q, want 12/10/2026 and the receiving deadline 19/10/2026", row.EntryDate, row.CancelDate)
	}
	if row.ShipTo != "127 Bình Lợi Phường 13 Quận Bình Thạnh Thành phố Hồ Chí Minh" {
		t.Errorf("ShipTo = %q", row.ShipTo)
	}
	// Exactly the PO's own TOTAL, even though line 1's printed unit price
	// (110,501) is rounded up from the 110,500.5 the total is computed at.
	if row.DonGia != "1337044" {
		t.Errorf("DonGia = %q, want the PO's own total %q", row.DonGia, "1337044")
	}
	if len(row.PromoItems) != 1 || row.PromoItems[0].SKU != "TP30244_02" || row.PromoItems[0].Qty != 8 {
		t.Errorf("PromoItems = %+v, want 8 × TP30244_02 from the buy-1-get-1", row.PromoItems)
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()

	// Row 9 is the note row; 10 the buy-1-get-1 line and 11 its gift; 12
	// and 13 the two 30%-off lines.
	for _, c := range []struct{ cell, want string }{
		{"A9", "12/10/2026"},
		{"B9", "ĐĐHOSIFOOD-68014-121026"},
		{"D9", "19/10/2026"},
		{"G9", "MN_MT_OF68014"},
		{"S9", "OSIFOOD 68014-121026"},
		{"V9", "LA_KHO2026"},
		{"AJ9", "MT_MN"},
		{"AM9", "LA"},
		{"Q10", "TP30244_02"},
		{"X10", "8"},
		{"Y10", "110500.5"},
		{"Q11", "TP30244_02"},
		{"X11", "8"},
		{"Q12", "TP31647"},
		{"X12", "4"},
		// "GIÁ VỐN KM" (56,630), not the list cost (80,900).
		{"Y12", "56630"},
		{"Q13", "TP31630"},
		{"X13", "4"},
		{"Y13", "56630"},
	} {
		got, _ := f.GetCellValue("Don dat hang", c.cell)
		if got != c.want {
			t.Errorf("%s = %q, want %q", c.cell, got, c.want)
		}
	}
}

func TestRealProcessor_FlagsOsifoodPriceThatDisagreesWithThePriceTab(t *testing.T) {
	pricingRows := osifoodPricingRows()
	pricingRows[2][4] = "Giảm 20%" // the PO charges TP31647 at 30% off
	rp, _ := osifoodTestProcessor(t, pricingRows)
	row := processOsifoodRealOrder(t, rp)

	if row.StatusKind != StatusKindWarning || row.PriceMismatchCount != 1 {
		t.Fatalf("StatusKind = %q (%s), PriceMismatchCount = %d, want one price mismatch", row.StatusKind, row.Status, row.PriceMismatchCount)
	}
	d := row.PriceMismatchDetails[0]
	if d.SKU != "TP31647" || d.InvoicePrice != 56630 || d.SystemPrice != 64720 {
		t.Errorf("mismatch = %+v, want TP31647 at PO 56630 vs system 64720", d)
	}
}

func TestOsifoodPromotions_OnlyAPrintedPromoLooksAheadToTheDeadline(t *testing.T) {
	// The receiving-deadline lookup exists for lines the PO itself marks
	// as promotional. A line printed at list cost with no promotion text
	// must be reconciled by order date only, so a campaign that starts
	// before delivery does not hand it a gift nobody ordered.
	index := pricing.ParseIndex(osifoodPricingRows())
	if got := osifoodPromotions(index, "TP30244_02", "12/10/2026", osifoodLine{PromoDate: "19/10/2026"}); len(got) != 0 {
		t.Errorf("promotions for a line with no printed promo = %+v, want none (campaign starts after the order date)", got)
	}
	if got := osifoodPromotions(index, "TP30244_02", "12/10/2026", osifoodLine{StatesPromo: true, PromoDate: "19/10/2026"}); len(got) != 1 {
		t.Errorf("promotions for a line the PO marks as promotional = %+v, want the 16/10-31/10 campaign", got)
	}
}

func TestRealProcessor_FailsOsifoodOrderForAStoreWithNoCustomerCode(t *testing.T) {
	rp, excelPath := osifoodTestProcessor(t, osifoodPricingRows())
	store, err := productdata.Load("productdata/testdata/data.xlsx") // no Osifood rows at all
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	rp.Store = store
	row := processOsifoodRealOrder(t, rp)

	if row.StatusKind != StatusKindFailed || !strings.Contains(row.Status, "68014") {
		t.Fatalf("StatusKind = %q, Status = %q, want a failure naming store 68014", row.StatusKind, row.Status)
	}
	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()
	if got, _ := f.GetCellValue("Don dat hang", "A9"); got != "" {
		t.Errorf("A9 = %q, want nothing written for a failed order", got)
	}
}

func TestRealProcessor_OsifoodOrderIsPushableToMISA(t *testing.T) {
	rp, _ := osifoodTestProcessor(t, osifoodPricingRows())
	row := processOsifoodRealOrder(t, rp)
	if row.StatusKind == StatusKindFailed {
		t.Fatalf("Process produced a Failed row: %+v", row)
	}
	// Note row, three product rows and one gift row.
	if len(row.ExcelRows) != 5 || row.ExcelRows[0] != 9 || row.ExcelRows[4] != 13 {
		t.Errorf("ExcelRows = %v, want rows 9 through 13", row.ExcelRows)
	}
	key := misapush.RouteKey(row.System, row.MaKhachHang, row.ShipTo)
	if branch := misapush.Lookup(misapush.SeedRouting(), key); branch != misapush.BranchHTLA {
		t.Errorf("routing key %q resolves to branch %q, want %q", key, branch, misapush.BranchHTLA)
	}
}

const osifoodRealWorkbook = "CTKM-16-31.10-HA-THANH-LONG-AN-1.xlsx"

// osifoodWorkbookPricingRows is an OSIFOOD price tab for the four products
// of the real campaign workbook, all buy-1-get-1 from 16/10.
func osifoodWorkbookPricingRows() [][]string {
	const gift = "Mua 1 tặng 1 (1+1) {KM Quấn Kèm - Che Barcode}"
	return [][]string{
		{"STT", "Mã hàng", "TÊN SẢN PHẨM", "ĐƠN GIÁ", "16/10-31/10"},
		{"1", "TP30244_02", "Nước giặt xả Blue Thảo Mộc túi 2.1L", "110.501", gift},
		{"2", "TP30329_02", "Nước giặt xả Blue Nước Hoa túi 2.1L", "110.501", gift},
		{"3", "TP30879_02", "Nước giặt xả Blue Thảo Mộc túi 3.6L", "163.540", gift},
		{"4", "TP30886_02", "Nước giặt xả Blue Nước Hoa túi 3.6L", "163.540", gift},
	}
}

func TestRealProcessor_ProcessesRealOsifoodCampaignWorkbook(t *testing.T) {
	rp, excelPath := osifoodTestProcessor(t, osifoodWorkbookPricingRows())

	rows, err := rp.Process(context.Background(), filepath.Join("osifood", "testdata", osifoodRealWorkbook))
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	// One row per store that answered with a quantity; stores that ordered
	// nothing (68014 among them) produce no row at all.
	if len(rows) != 5 {
		t.Fatalf("len(rows) = %d, want 5: %+v", len(rows), rows)
	}

	first := rows[0]
	if first.StatusKind != StatusKindDone {
		t.Fatalf("store 79001: StatusKind = %q (%s), want %q\nlog: %s", first.StatusKind, first.Status, StatusKindDone, strings.Join(first.SkuLog, "\n"))
	}
	if first.System != "Osifood" || first.MaKhachHang != "MN_MT_OF79001" || first.Page != "CH 79001" {
		t.Errorf("store 79001: System/MaKhachHang/Page = %q/%q/%q", first.System, first.MaKhachHang, first.Page)
	}
	// Store code + the day the workbook was saved; no deadline is given,
	// so the delivery date is that same day.
	if first.PO != "79001-101026" || first.EntryDate != "10/10/2026" || first.CancelDate != "10/10/2026" {
		t.Errorf("store 79001: PO/EntryDate/CancelDate = %q/%q/%q", first.PO, first.EntryDate, first.CancelDate)
	}
	if !strings.HasPrefix(first.ShipTo, "288 Phan Văn Trị") {
		t.Errorf("store 79001: ShipTo = %q", first.ShipTo)
	}
	// 4 × 110,500.5 × 2 + 4 × 163,540.3 × 2, the store's own total.
	if first.DonGia != "2192326" {
		t.Errorf("store 79001: DonGia = %q, want %q", first.DonGia, "2192326")
	}
	if len(first.PromoItems) != 4 {
		t.Errorf("store 79001: PromoItems = %+v, want one gift per product", first.PromoItems)
	}

	second := rows[1]
	if second.StatusKind != StatusKindDone || second.MaKhachHang != "MN_MT_OF79003" || second.DonGia != "3076330" {
		t.Errorf("store 79003: StatusKind/MaKhachHang/DonGia = %q/%q/%q (%s)", second.StatusKind, second.MaKhachHang, second.DonGia, second.Status)
	}
	// The second order starts right after the first one's 9 rows (note +
	// 4 products + 4 gifts), which is what MISA push splits on.
	if len(first.ExcelRows) != 9 || first.ExcelRows[0] != 9 || len(second.ExcelRows) != 9 || second.ExcelRows[0] != 18 {
		t.Errorf("ExcelRows = %v then %v, want rows 9–17 then 18–26", first.ExcelRows, second.ExcelRows)
	}

	// A store with no customer code fails on its own row, naming itself,
	// without taking the other stores down.
	for i, code := range []string{"79004", "79005", "79009"} {
		row := rows[2+i]
		if row.StatusKind != StatusKindFailed || !strings.Contains(row.Status, code) || row.Page != "CH "+code {
			t.Errorf("store %s: StatusKind = %q, Status = %q, Page = %q, want a failure naming the store", code, row.StatusKind, row.Status, row.Page)
		}
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()
	for _, c := range []struct{ cell, want string }{
		{"B9", "ĐĐHOSIFOOD-79001-101026"},
		{"G9", "MN_MT_OF79001"},
		{"Q10", "TP30244_02"},
		{"X10", "4"},
		{"Y10", "110500.5"},
		{"Q11", "TP30244_02"}, // its gift
		{"Q16", "TP30886_02"},
		{"Y16", "163540.3"},
		{"B18", "ĐĐHOSIFOOD-79003-101026"},
		{"X19", "8"},
		{"A27", ""}, // nothing written for the three failed stores
	} {
		got, _ := f.GetCellValue("Don dat hang", c.cell)
		if got != c.want {
			t.Errorf("%s = %q, want %q", c.cell, got, c.want)
		}
	}
}
