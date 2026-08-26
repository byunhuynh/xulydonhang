package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/processing"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/tmdt"
	"order-processor/internal/tmdt/lookup"
)

func TestInspectTMDTFiles(t *testing.T) {
	dir := t.TempDir()

	wb := excelize.NewFile()
	for _, s := range []string{lookup.SheetMisa, lookup.SheetDataShop, "Haravan"} {
		if _, err := wb.NewSheet(s); err != nil {
			t.Fatalf("NewSheet: %v", err)
		}
	}
	wb.DeleteSheet("Sheet1")
	tmdtPath := filepath.Join(dir, "XUẤT HÀNG HN-LA MỚI.xlsx")
	if err := wb.SaveAs(tmdtPath); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	wb.Close()

	other := excelize.NewFile()
	otherPath := filepath.Join(dir, "don-vendor.xlsx")
	if err := other.SaveAs(otherPath); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	other.Close()

	app := &App{}
	got, err := app.InspectTMDTFiles([]string{otherPath, tmdtPath})
	if err != nil {
		t.Fatalf("InspectTMDTFiles: %v", err)
	}
	if len(got) != 1 || got[0] != tmdtPath {
		t.Errorf("InspectTMDTFiles = %v, muốn chỉ %q", got, tmdtPath)
	}
}

func TestParseTMDTRangeRejectsBadInput(t *testing.T) {
	today := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)

	if _, _, err := parseTMDTRange(TMDTDateRange{From: "2026-08-18", To: "2026-08-24"}, today); err != nil {
		t.Errorf("khoảng 7 ngày hợp lệ bị từ chối: %v", err)
	}
	if _, _, err := parseTMDTRange(TMDTDateRange{From: "2026-08-17", To: "2026-08-24"}, today); err == nil {
		t.Errorf("khoảng 8 ngày phải bị từ chối")
	}
	if _, _, err := parseTMDTRange(TMDTDateRange{From: "2026-08-25", To: "2026-08-25"}, today); err == nil {
		t.Errorf("ngày hôm nay phải bị từ chối")
	}
	if _, _, err := parseTMDTRange(TMDTDateRange{From: "24/08/2026", To: "2026-08-24"}, today); err == nil {
		t.Errorf("định dạng sai phải bị từ chối")
	}
	if _, _, err := parseTMDTRange(TMDTDateRange{From: "2026-08-24", To: "2026-08-20"}, today); err == nil {
		t.Errorf("from sau to phải bị từ chối")
	}

	// Biên giờ: from ở 00:00:00+07, to ở 23:59:59+07.
	from, to, err := parseTMDTRange(TMDTDateRange{From: "2026-08-22", To: "2026-08-23"}, today)
	if err != nil {
		t.Fatalf("parseTMDTRange: %v", err)
	}
	if got := from.Format(time.RFC3339); got != "2026-08-22T00:00:00+07:00" {
		t.Errorf("from = %s", got)
	}
	if got := to.Format(time.RFC3339); got != "2026-08-23T23:59:59+07:00" {
		t.Errorf("to = %s", got)
	}
}

func TestWaitForTMDTResolutionCancel(t *testing.T) {
	app := &App{tmdtResolve: make(chan tmdtResolution, 1)}
	// Bật cờ TRƯỚC khi chạy goroutine: nếu để waitForTMDTResolution bật thì
	// CancelTMDTMissing có thể chạy trước và bị chính cờ đó từ chối — test
	// sẽ chập chờn. Bản thật cũng bật cờ trước khi phát tmdt:missing, đúng
	// vì lý do này.
	app.tmdtWaiting.Store(true)
	go func() {
		if err := app.CancelTMDTMissing(); err != nil {
			t.Errorf("CancelTMDTMissing: %v", err)
		}
	}()
	res, ok := app.waitForTMDTResolution(200 * time.Millisecond)
	if !ok {
		t.Fatalf("waitForTMDTResolution báo hết giờ dù đã có phản hồi")
	}
	if !res.cancel {
		t.Errorf("muốn cancel = true")
	}
}

func TestWaitForTMDTResolutionTimeout(t *testing.T) {
	app := &App{tmdtResolve: make(chan tmdtResolution, 1)}
	start := time.Now()
	if _, ok := app.waitForTMDTResolution(50 * time.Millisecond); ok {
		t.Errorf("muốn hết giờ khi không ai trả lời")
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Errorf("trả về sớm hơn hạn giờ")
	}
}

// TestTMDTResolveRejectedWhenNobodyWaiting khoá cái van chống phản hồi lạc:
// người dùng bấm "Xong" khi không có nhánh nào đang chờ thì phản hồi KHÔNG
// được nằm lại trong channel, vì file TMĐT kế tiếp sẽ ăn ngay nó và bỏ qua
// bước khai mã mà không ai bấm gì.
func TestTMDTResolveRejectedWhenNobodyWaiting(t *testing.T) {
	app := &App{tmdtResolve: make(chan tmdtResolution, 1)}
	if err := app.ResolveTMDTMissing(nil); err == nil {
		t.Errorf("ResolveTMDTMissing phải lỗi khi không có ai chờ")
	}
	if err := app.CancelTMDTMissing(); err == nil {
		t.Errorf("CancelTMDTMissing phải lỗi khi không có ai chờ")
	}
	if len(app.tmdtResolve) != 0 {
		t.Errorf("channel còn %d phản hồi lạc, muốn 0", len(app.tmdtResolve))
	}
}

func TestSummaryRowsGroupByShopAndDate(t *testing.T) {
	rows := summaryTMDTRows("XUẤT HÀNG HN-LA MỚI.xlsx", []summaryKeyCount{
		{shop: "Blue HN", date: "23/08/2026", channel: "TikTok", misa: "MB_TMDT_00001", shipTo: "HN", orders: 3, lines: 5},
		{shop: "Blue HN", date: "22/08/2026", channel: "TikTok", misa: "MB_TMDT_00001", shipTo: "HN", orders: 1, lines: 1, hasNA: true},
	})
	if len(rows) != 2 {
		t.Fatalf("có %d dòng tóm tắt, muốn 2", len(rows))
	}
	if rows[0].System != "TMĐT-TikTok" {
		t.Errorf("System = %q, muốn TMĐT-TikTok", rows[0].System)
	}
	if rows[0].PO != "Blue HN · 23/08/2026" {
		t.Errorf("PO = %q", rows[0].PO)
	}
	if rows[0].StatusKind != "done" {
		t.Errorf("StatusKind = %q, muốn done", rows[0].StatusKind)
	}
	if rows[1].StatusKind != "warning" {
		t.Errorf("nhóm còn #N/A phải là warning, được %q", rows[1].StatusKind)
	}
}

// TestGroupTMDTSummaryShopNameWithDash: tên shop do người dùng đặt trên sàn
// có thể chứa " - " ("Blue - Chính hãng"), đúng chuỗi dùng để ghép cột Diễn
// giải. Tách từ ĐẦU chuỗi sẽ cắt mất nửa tên; phải neo từ CUỐI vì ba phần
// cuối (mã đơn, "Ngày đổ ...", kho) luôn cố định.
func TestGroupTMDTSummaryShopNameWithDash(t *testing.T) {
	res := tmdt.Result{OrderRows: []excelwriter.TMDTRow{
		{
			EntryDate:    "23/08/2026",
			OrderNumber:  "ĐĐHTMĐT-TikTok-585694438276170905",
			ShipTo:       "HN",
			CustomerCode: "MB_TMDT_00001",
			Description:  "TMĐT-TikTok - Blue - Chính hãng - 585694438276170905 - Ngày đổ 23/08/2026 - HN",
			SKU:          "TP10127",
			Note:         "585694438276170905",
		},
	}}
	groups := groupTMDTSummary(res)
	if len(groups) != 1 {
		t.Fatalf("có %d nhóm, muốn 1", len(groups))
	}
	if groups[0].shop != "Blue - Chính hãng" {
		t.Errorf("shop = %q, muốn %q", groups[0].shop, "Blue - Chính hãng")
	}
	if groups[0].channel != "TikTok" {
		t.Errorf("channel = %q", groups[0].channel)
	}
	if groups[0].orders != 1 || groups[0].lines != 1 {
		t.Errorf("orders/lines = %d/%d, muốn 1/1", groups[0].orders, groups[0].lines)
	}
	if groups[0].hasNA {
		t.Errorf("không có #N/A nào mà báo hasNA")
	}
}

// TestShopFromDescriptionKhongPanic: Diễn giải lạ (rỗng, thiếu dấu phân
// cách) chỉ được trả về chuỗi rỗng, KHÔNG panic — nó chạy trên dữ liệu do
// người dùng gõ, và một panic ở đây giết cả batch.
func TestShopFromDescriptionKhongPanic(t *testing.T) {
	for _, desc := range []string{"", "TMĐT-TikTok", "a - b", " -  - "} {
		_ = shopFromDescription(desc)
	}
}

// TestNoComponentLogsPhanBietMucDo là hàng rào chống báo động giả: 6 dòng
// "quà tặng" cố ý không khai mã thành phẩm xuất hiện ở HẦU HẾT lần chạy
// thật, nên chúng phải là thông tin (ℹ️). Chỉ SLTP không đọc được — lỗi dữ
// liệu thật — mới được mang ⚠️.
func TestNoComponentLogsPhanBietMucDo(t *testing.T) {
	lines := noComponentLogLines(map[string]int{
		tmdt.KhongKhaiThanhPham + "sku:QT200K": 12,
		tmdt.SLTPKhongDocDuoc + "sku:SP000450": 3,
	})
	if len(lines) != 2 {
		t.Fatalf("có %d dòng log, muốn 2:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	var info, warn string
	for _, l := range lines {
		if strings.HasPrefix(l, "⚠️") {
			warn = l
		} else {
			info = l
		}
	}
	if info == "" || strings.Contains(info, "⚠️") {
		t.Errorf("dòng quà tặng phải là thông tin, không cảnh báo: %q", info)
	}
	if !strings.Contains(info, "12") {
		t.Errorf("dòng thông tin phải nói số dòng bị bỏ: %q", info)
	}
	if warn == "" {
		t.Errorf("SLTP không đọc được phải mang ⚠️, được: %v", lines)
	}
	if !strings.Contains(warn, "SP000450") || !strings.Contains(warn, "3") {
		t.Errorf("cảnh báo phải chỉ rõ mã và số dòng: %q", warn)
	}
}

// TestMissingShopLogLines: khoá tmdt.ShopKhongTen là NHÃN đọc được, không
// phải tên shop — đặt nó vào 'Shop "..."' sẽ ra câu vô nghĩa.
func TestMissingShopLogLines(t *testing.T) {
	lines := missingShopLogLines(map[string]int{
		"Shop Lạ":         4,
		tmdt.ShopKhongTen: 2,
	})
	if len(lines) != 2 {
		t.Fatalf("có %d dòng, muốn 2: %v", len(lines), lines)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, `"Shop Lạ"`) {
		t.Errorf("thiếu tên shop trong log: %s", joined)
	}
	for _, l := range lines {
		if strings.Contains(l, `"`+tmdt.ShopKhongTen+`"`) {
			t.Errorf("nhãn %q bị bọc như tên shop: %q", tmdt.ShopKhongTen, l)
		}
	}
}

// TestProcessTMDTFileKhongCoKhoangNgay: file TMĐT lọt vào batch mà không có
// khoảng ngày là TÌNH HUỐNG NGƯỜI DÙNG THẤY (bản frontend cũ, hoặc gọi qua
// bindings), không phải panic. Phải trả lỗi đọc được và không chạm tới bất
// cứ thứ gì khác — app ở đây còn chưa có appSettingsStore, nếu hàm đi tiếp
// thì nil pointer sẽ panic và test đổ.
func TestProcessTMDTFileKhongCoKhoangNgay(t *testing.T) {
	app := &App{}
	emitter := &fakeEmitter{}
	rows, err := app.processTMDTFile(emitter, "XUẤT HÀNG HN-LA MỚI.xlsx", TMDTDateRange{}, func(processing.OrderRow) {})
	if err == nil {
		t.Fatalf("muốn lỗi khi chưa chọn khoảng ngày")
	}
	if rows != nil {
		t.Errorf("rows = %v, muốn nil", rows)
	}
	if !strings.Contains(err.Error(), "khoảng thời gian") {
		t.Errorf("lỗi phải nói về khoảng thời gian, được: %v", err)
	}
	if len(emitter.events) == 0 {
		t.Errorf("phải ghi log cho người dùng thấy lý do")
	}
}
