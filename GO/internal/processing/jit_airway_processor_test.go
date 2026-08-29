package processing

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/xuri/excelize/v2"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func newTwoPageJITProcessor(t *testing.T, excelPath string) (*RealProcessor, string) {
	t.Helper()
	store, err := productdata.Load(filepath.Join("..", "..", "..", "data.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(t.TempDir(), "air_waybill_WH6_HTLA_24082026.pdf")
	sourcePath := filepath.Join("..", "..", "..", "đơn hàng", "air_waybill_WH6_HTLA_24082026.pdf")
	if err := api.MergeCreateFile([]string{sourcePath, sourcePath}, filePath, false, nil); err != nil {
		t.Fatalf("build two-page JIT fixture: %v", err)
	}
	return &RealProcessor{
		Store: store,
		Pricing: &fixturePricingSource{index: pricing.ParseIndex([][]string{
			{"", "Mã hàng", "", "Giá"},
			{"", "TP30671", "", "24537"},
		})},
		ExcelPath: excelPath,
	}, filePath
}

func TestJITStreamingBatchesWorkbookRowsAndFinalizesAbsoluteRows(t *testing.T) {
	excelPath := copyTestWorkbookForProcessor(t)
	rp, filePath := newTwoPageJITProcessor(t, excelPath)

	originalWrite := writeJITOrderRows
	writeCalls := 0
	writtenProductRows := 0
	writeJITOrderRows = func(path string, rows []excelwriter.Row, description string) (int, error) {
		writeCalls++
		writtenProductRows += len(rows)
		return originalWrite(path, rows, description)
	}
	t.Cleanup(func() { writeJITOrderRows = originalWrite })

	var events []OrderRow
	rows, err := rp.ProcessStreaming(context.Background(), filePath, func(row OrderRow) {
		events = append(events, row)
	})
	if err != nil {
		t.Fatalf("ProcessStreaming returned error: %v", err)
	}
	if writeCalls != 1 || writtenProductRows != 2 {
		t.Fatalf("JIT workbook writes = %d calls / %d product rows, want one call containing both product rows", writeCalls, writtenProductRows)
	}
	if len(rows) != 2 {
		t.Fatalf("returned %d rows, want two final page rows: %+v", len(rows), rows)
	}
	if len(events) != 4 {
		t.Fatalf("emitted %d events, want processing/processing/done/done: %+v", len(events), events)
	}

	sourceID := SourceIDForPath(filePath)
	wantKeys := []string{
		orderResultKey(sourceID, "page:1", "2608246E2455ST"),
		orderResultKey(sourceID, "page:2", "2608246E2455ST"),
		orderResultKey(sourceID, "page:1", "2608246E2455ST"),
		orderResultKey(sourceID, "page:2", "2608246E2455ST"),
	}
	wantStatuses := []string{StatusKindProcessing, StatusKindProcessing, StatusKindDone, StatusKindDone}
	for i := range events {
		if events[i].ResultKey != wantKeys[i] || events[i].StatusKind != wantStatuses[i] {
			t.Errorf("event %d = key %q status %q, want key %q status %q", i, events[i].ResultKey, events[i].StatusKind, wantKeys[i], wantStatuses[i])
		}
		if events[i].SourceID != sourceID {
			t.Errorf("event %d SourceID = %q, want stable source %q across provisional/final updates", i, events[i].SourceID, sourceID)
		}
	}
	for i, row := range rows {
		wantExcelRow := 9 + i
		if len(row.ExcelRows) != 1 || row.ExcelRows[0] != wantExcelRow {
			t.Errorf("returned page %d ExcelRows = %v, want [%d]", i+1, row.ExcelRows, wantExcelRow)
		}
		if row.ResultKey != wantKeys[i] || row.StatusKind != StatusKindDone {
			t.Errorf("returned page %d = key %q status %q, want key %q status done", i+1, row.ResultKey, row.StatusKind, wantKeys[i])
		}
		if row.SourceID != sourceID {
			t.Errorf("returned page %d SourceID = %q, want %q", i+1, row.SourceID, sourceID)
		}
	}

	book, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for _, cell := range []string{"Q9", "Q10"} {
		got, cellErr := book.GetCellValue("Don dat hang", cell)
		if cellErr != nil {
			t.Fatal(cellErr)
		}
		if got != "TP30671" {
			t.Errorf("%s = %q, want TP30671 from the combined write", cell, got)
		}
	}
}

func TestJITCombinedWriteFailureFinalizesEveryProvisionalKey(t *testing.T) {
	missingWorkbook := filepath.Join(t.TempDir(), "missing", "dondathang.xlsx")
	rp, filePath := newTwoPageJITProcessor(t, missingWorkbook)

	originalWrite := writeJITOrderRows
	writeCalls := 0
	writeJITOrderRows = func(path string, rows []excelwriter.Row, description string) (int, error) {
		writeCalls++
		return originalWrite(path, rows, description)
	}
	t.Cleanup(func() { writeJITOrderRows = originalWrite })

	var events []OrderRow
	rows, err := rp.ProcessStreaming(context.Background(), filePath, func(row OrderRow) {
		events = append(events, row)
	})
	if err != nil {
		t.Fatalf("ProcessStreaming returned error: %v", err)
	}
	if writeCalls != 1 {
		t.Fatalf("JIT workbook writes = %d, want one failed combined write", writeCalls)
	}
	if len(rows) != 2 || len(events) != 4 {
		t.Fatalf("returned %d rows and emitted %d events, want 2 final rows and 4 events: rows=%+v events=%+v", len(rows), len(events), rows, events)
	}

	sourceID := SourceIDForPath(filePath)
	wantKeys := []string{
		orderResultKey(sourceID, "page:1", "2608246E2455ST"),
		orderResultKey(sourceID, "page:2", "2608246E2455ST"),
	}
	for i, key := range wantKeys {
		if events[i].ResultKey != key || events[i].StatusKind != StatusKindProcessing {
			t.Errorf("provisional event %d = key %q status %q, want key %q status processing", i, events[i].ResultKey, events[i].StatusKind, key)
		}
		finalEvent := events[i+2]
		if finalEvent.ResultKey != key || finalEvent.StatusKind != StatusKindFailed {
			t.Errorf("final event %d = key %q status %q, want key %q status failed", i, finalEvent.ResultKey, finalEvent.StatusKind, key)
		}
		if rows[i].ResultKey != key || rows[i].StatusKind != StatusKindFailed {
			t.Errorf("returned row %d = key %q status %q, want key %q status failed", i, rows[i].ResultKey, rows[i].StatusKind, key)
		}
		if len(rows[i].ExcelRows) != 0 {
			t.Errorf("failed row %d ExcelRows = %v, want none", i, rows[i].ExcelRows)
		}
	}
}

func TestParseJITAirWaybillFilename(t *testing.T) {
	warehouse, date, ok := parseJITAirWaybillFilename(`C:\orders\air_waybill_WH6_HN_24082026.pdf`)
	if !ok || warehouse != "WH6_HN" || date != "24/08/2026" {
		t.Fatalf("got (%q, %q, %v), want (%q, %q, true)", warehouse, date, ok, "WH6_HN", "24/08/2026")
	}
}

func TestParseJITAirWaybillFilenameRejectsOtherFiles(t *testing.T) {
	if _, _, ok := parseJITAirWaybillFilename("package_list_WH6_HN_24082026.pdf"); ok {
		t.Fatal("package_list file matched air-waybill parser")
	}
}

func TestParseJITAirWaybillFilenameAcceptsWindowsCopySuffix(t *testing.T) {
	warehouse, date, ok := parseJITAirWaybillFilename("air_waybill_WH6_HTLA_24082026 (1).pdf")
	if !ok || warehouse != "WH6_HTLA" || date != "24/08/2026" {
		t.Fatalf("got (%q, %q, %v), want copied air-waybill filename to be accepted", warehouse, date, ok)
	}
}

func TestParseJITAirWaybillFilenameAcceptsShortName(t *testing.T) {
	now := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name          string
		wantWarehouse string
		wantDate      string
	}{
		{"WH6_HN_2908.pdf", "WH6_HN", "29/08/2026"},
		{`C:\orders\WH6_HTLA_0109.pdf`, "WH6_HTLA", "01/09/2026"},
		{"WH6_HN_2908 (1).pdf", "WH6_HN", "29/08/2026"},
		{"WH6_HN_29082026.pdf", "WH6_HN", "29/08/2026"},
		{"air_waybill_WH6_HN_2908.pdf", "WH6_HN", "29/08/2026"},
	}
	for _, tc := range cases {
		warehouse, date, ok := parseJITAirWaybillFilenameAt(tc.name, now)
		if !ok || warehouse != tc.wantWarehouse || date != tc.wantDate {
			t.Errorf("%s: got (%q, %q, %v), want (%q, %q, true)", tc.name, warehouse, date, ok, tc.wantWarehouse, tc.wantDate)
		}
	}
}

// Tên rút gọn không mang năm: file "3112" xử lý sang đầu tháng 1 phải rơi vào
// năm trước, "0101" xử lý cuối tháng 12 phải rơi vào năm sau.
func TestParseJITAirWaybillFilenameShortNamePicksNearestYear(t *testing.T) {
	if _, date, ok := parseJITAirWaybillFilenameAt("WH6_HN_3112.pdf", time.Date(2027, 1, 2, 9, 0, 0, 0, time.UTC)); !ok || date != "31/12/2026" {
		t.Errorf("got (%q, %v), want (%q, true)", date, ok, "31/12/2026")
	}
	if _, date, ok := parseJITAirWaybillFilenameAt("WH6_HN_0101.pdf", time.Date(2026, 12, 30, 9, 0, 0, 0, time.UTC)); !ok || date != "01/01/2027" {
		t.Errorf("got (%q, %v), want (%q, true)", date, ok, "01/01/2027")
	}
	// 29/02 chỉ hợp lệ ở năm nhuận: 2028 chứ không phải 2026/2027.
	if _, date, ok := parseJITAirWaybillFilenameAt("WH6_HN_2902.pdf", time.Date(2027, 6, 1, 9, 0, 0, 0, time.UTC)); !ok || date != "29/02/2028" {
		t.Errorf("got (%q, %v), want (%q, true)", date, ok, "29/02/2028")
	}
}

func TestParseJITAirWaybillFilenameRejectsShortNameLookalikes(t *testing.T) {
	now := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	for _, name := range []string{
		"package_list_WH6_HN_2908.pdf",
		"BAOCAO_KHO_2908.pdf",
		"don_hang_2026.pdf",
		"WH6_HN_3213.pdf",
	} {
		if _, _, ok := parseJITAirWaybillFilenameAt(name, now); ok {
			t.Errorf("%s matched air-waybill parser, want no match", name)
		}
	}
}

func TestParseJITAirWaybillPageHandlesGoPDFSpacing(t *testing.T) {
	text := `
Mã v ậ n đ ơ n: SPXVN065281192958
Mã đ ơ n hàng: 26082225AS95HD
N ộ i dung hàng (T ổ ng SL s ả n ph ẩ m: 2)
1. [T op V alue] Chai th ả b ồ n c ầ u Smile Mom CH34 màu tím,
Ng ẫu nhiên, S
L: 2
`
	tracking, po, products, err := parseJITAirWaybillPage(text)
	if err != nil {
		t.Fatal(err)
	}
	if tracking != "SPXVN065281192958" {
		t.Fatalf("tracking = %q, want %q", tracking, "SPXVN065281192958")
	}
	if po != "26082225AS95HD" {
		t.Fatalf("PO = %q, want %q", po, "26082225AS95HD")
	}
	if len(products) != 1 || products[0].Barcode != "[TopValue]ChaithảbồncầuSmileMomCH34màutím,Ngẫunhiên" || products[0].Qty != 2 {
		t.Fatalf("products = %#v, want one full product key containing CH34 x2", products)
	}
}

func TestParseJITAirWaybillPageRejectsMissingPO(t *testing.T) {
	_, _, _, err := parseJITAirWaybillPage("Mã vận đơn: SPXVN065281192958\n1. [Top Value] product CH34, SL: 1")
	if err == nil {
		t.Fatal("missing PO returned nil error")
	}
}

func TestParseJITAirWaybillPageKeepsProductNameWhenNoCHCodeExists(t *testing.T) {
	text := "Mã vận đơn: SPXVN066220307978\nMã đơn hàng: 2608222E3UC9YR\nNội dung hàng\n1. [Top Value] Bột thông cống BLUE Đột phá gói 100g, SL: 2"
	_, _, products, err := parseJITAirWaybillPage(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 1 || products[0].Barcode != "[TopValue]BộtthôngcốngBLUEĐộtphágói100g" || products[0].Qty != 2 {
		t.Fatalf("products = %#v, want normalized product-name lookup key x2", products)
	}
}

func TestParseJITAirWaybillPageRejectsMissingTrackingNumber(t *testing.T) {
	_, _, _, err := parseJITAirWaybillPage("Mã đơn hàng: 2608222E3UC9YR\n1. [Top Value] product CH34, SL: 1")
	if err == nil {
		t.Fatal("missing tracking number returned nil error")
	}
}

func TestParseJITAirWaybillPageAcceptsNonSPXTrackingNumber(t *testing.T) {
	text := "Mã vận đơn: GY8ANXT7\nMã đơn hàng: 2608222X2WW9KT\n1. [Top Value] product CH34, SL: 1"
	tracking, _, _, err := parseJITAirWaybillPage(text)
	if err != nil {
		t.Fatal(err)
	}
	if tracking != "GY8ANXT7" {
		t.Fatalf("tracking = %q, want GY8ANXT7", tracking)
	}
}

func TestParseJITAirWaybillPageKeepsAdjacentMultiProductsSeparate(t *testing.T) {
	text := "Mã vận đơn: SPXVN065281192958\nMã đơn hàng: 260823EXAMPLE1\nNội dung hàng (Tổng SL sản phẩm: 3)\n" +
		"1. [Top Value] Nước xả vải Blue CH14, SL: 1" +
		"2. [Top Value] Túi nước xả vải Blue 3.2L, SL: 1" +
		"3. [Top Value] Chai thả bồn cầu CH34, SL: 1"
	_, _, products, err := parseJITAirWaybillPage(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 3 {
		t.Fatalf("got %d products (%#v), want 3", len(products), products)
	}
	for i, product := range products {
		if product.Qty != 1 {
			t.Errorf("product %d qty = %v, want 1", i+1, product.Qty)
		}
	}
}

func TestJITRegionInfoRoutesNorthernAndSouthernWarehouses(t *testing.T) {
	cases := []struct {
		shipTo, wantRegion, wantStat, wantWarehouse string
	}{
		{"WH6_HN", "TMĐT_MB", "HN", "TP_HN_12"},
		{"WH6_HTLA", "TMĐT_MB", "HN", "TP_HN_12"},
		{"WH6_HCM", "TMĐT_MN", "LA", "LA_KHOTMDT"},
	}
	for _, tc := range cases {
		region, stat, warehouse := jitRegionInfo(tc.shipTo)
		if region != tc.wantRegion || stat != tc.wantStat || warehouse != tc.wantWarehouse {
			t.Errorf("jitRegionInfo(%q) = (%q,%q,%q), want (%q,%q,%q)", tc.shipTo, region, stat, warehouse, tc.wantRegion, tc.wantStat, tc.wantWarehouse)
		}
	}
}

func TestRealProcessorProcessesJITAirWaybillByFilename(t *testing.T) {
	store, err := productdata.Load(filepath.Join("..", "..", "..", "data.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	excelPath := copyTestWorkbookForProcessor(t)
	var logs []string
	rp := &RealProcessor{
		Store: store,
		Pricing: &fixturePricingSource{index: pricing.ParseIndex([][]string{
			{"", "Mã hàng", "", "Giá"},
			{"", "TP30671", "", "24537"},
		})},
		ExcelPath: excelPath,
		LogFunc:   func(line string) { logs = append(logs, line) },
	}
	path := filepath.Join("..", "..", "..", "đơn hàng", "air_waybill_WH6_HTLA_24082026.pdf")
	rows, err := rp.Process(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	row := rows[0]
	if row.System != "JIT-CHOICE" || row.MaKhachHang != "MN_JIT_01512" || row.PO != "2608246E2455ST" || row.MaVanDon != "SPXVN066031037238" || row.ShipTo != "WH6_HTLA" || row.EntryDate != "24/08/2026" {
		t.Fatalf("unexpected result row: %#v", row)
	}
	if row.StatusKind != StatusKindDone {
		t.Fatalf("status = %q (%q), want done", row.Status, row.StatusKind)
	}

	book, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for cell, want := range map[string]string{
		"A9": "24/08/2026", "C9": "Chưa thực hiện", "D9": "24/08/2026",
		"E9": "WH6_HTLA", "G9": "MN_JIT_01512", "Q9": "TP30671",
		"V9": "TP_HN_12", "X9": "1", "Y9": "24537", "AE9": "8",
		"AJ9": "TMĐT_MB", "AM9": "HN", "AO9": "2608246E2455ST - SPXVN066031037238",
		"AT9": "0.5", "AU9": "1", "AV9": "15",
	} {
		got, cellErr := book.GetCellValue("Don dat hang", cell)
		if cellErr != nil {
			t.Fatal(cellErr)
		}
		if got != want {
			t.Errorf("%s = %q, want %q", cell, got, want)
		}
	}
	productName, err := book.GetCellValue("Don dat hang", "S9")
	if err != nil {
		t.Fatal(err)
	}
	if productName == "" {
		t.Error("S9 is blank, want mapped JIT product name")
	}
	orderNumber, err := book.GetCellValue("Don dat hang", "B9")
	if err != nil {
		t.Fatal(err)
	}
	if len(orderNumber) < len("ĐĐHJIT-") || orderNumber[:len("ĐĐHJIT-")] != "ĐĐHJIT-" {
		t.Errorf("B9 = %q, want JIT order-number prefix", orderNumber)
	}
	joinedLogs := strings.Join(logs, "\n")
	for _, want := range []string{"🚀 JIT [1/1]", "PO: 2608246E2455ST", "MVĐ: SPXVN066031037238", "✅ TP30671", "SL: 1", "Giá: 24537"} {
		if !strings.Contains(joinedLogs, want) {
			t.Errorf("JIT logs = %q, want to contain %q", joinedLogs, want)
		}
	}
}

func TestRealProcessorDoesNotWriteJITOrderWhenPriceIsMissing(t *testing.T) {
	store, err := productdata.Load(filepath.Join("..", "..", "..", "data.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	excelPath := copyTestWorkbookForProcessor(t)
	rp := &RealProcessor{
		Store:     store,
		Pricing:   &fixturePricingSource{index: pricing.ParseIndex([][]string{{"", "Mã hàng", "", "Giá"}})},
		ExcelPath: excelPath,
	}
	path := filepath.Join("..", "..", "..", "đơn hàng", "air_waybill_WH6_HTLA_24082026.pdf")
	rows, err := rp.Process(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].StatusKind != StatusKindFailed {
		t.Fatalf("rows = %#v, want one failed result", rows)
	}
	if !strings.Contains(strings.ToLower(rows[0].Status), "giá") {
		t.Fatalf("status = %q, want a missing-price failure", rows[0].Status)
	}

	book, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	got, err := book.GetCellValue("Don dat hang", "A9")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("A9 = %q, want blank because an order with a missing price must not be written", got)
	}
}
