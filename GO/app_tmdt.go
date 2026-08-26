package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"order-processor/internal/fileset"
	"order-processor/internal/processing"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/tmdt"
	"order-processor/internal/tmdt/haravan"
	"order-processor/internal/tmdt/lookup"
)

// tmdtMissingTimeout là hạn chờ người dùng khai mã thiếu. Nhánh TMĐT dừng
// giữa batch trong khi ĐANG GIỮ a.excelMu — đây là chỗ duy nhất trong app
// làm vậy — nên phải có hạn giờ: người dùng bỏ đi mà không bấm gì thì
// batch vẫn kết thúc chứ không khoá Excel vĩnh viễn.
const tmdtMissingTimeout = 10 * time.Minute

// TMDTDateRange là khoảng ngày (giờ VN) người dùng chọn cho một file TMĐT.
type TMDTDateRange struct {
	From string `json:"from"` // "2026-08-22"
	To   string `json:"to"`   // "2026-08-23", tính hết ngày
}

// TMDTComboEntry là một dòng người dùng vừa khai trong modal sửa mã thiếu
// — đúng hình dạng một dòng cột A..K của sheet "data shop".
type TMDTComboEntry struct {
	Key     string    `json:"key"`
	Product string    `json:"product"`
	Variant string    `json:"variant"`
	Combo   string    `json:"combo"`
	TP      [4]string `json:"tp"`
	SL      [4]string `json:"sl"`
}

type tmdtResolution struct {
	entries []TMDTComboEntry
	cancel  bool
}

// InspectTMDTFiles trả về những đường dẫn trong paths là workbook TMĐT.
// Frontend gọi hàm này khi người dùng bấm "Xử lý" để biết có cần bật modal
// chọn ngày hay không.
func (a *App) InspectTMDTFiles(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range fileset.FilterValid(paths) {
		if tmdt.IsWorkbook(p) {
			out = append(out, p)
		}
	}
	return out, nil
}

// ResolveTMDTMissing nhận các dòng người dùng vừa khai.
func (a *App) ResolveTMDTMissing(entries []TMDTComboEntry) error {
	if !a.tmdtWaiting.Load() {
		return fmt.Errorf("không có yêu cầu khai mã nào đang chờ")
	}
	select {
	case a.tmdtResolve <- tmdtResolution{entries: entries}:
		return nil
	default:
		return fmt.Errorf("đã có phản hồi được gửi trước đó")
	}
}

// CancelTMDTMissing bỏ qua việc khai mã: các dòng thiếu vẫn được ghi với
// #N/A để người dùng thấy, không bị bỏ âm thầm.
func (a *App) CancelTMDTMissing() error {
	if !a.tmdtWaiting.Load() {
		return fmt.Errorf("không có yêu cầu khai mã nào đang chờ")
	}
	select {
	case a.tmdtResolve <- tmdtResolution{cancel: true}:
		return nil
	default:
		return fmt.Errorf("đã có phản hồi được gửi trước đó")
	}
}

// waitForTMDTResolution chờ một trong ba lối ra: người dùng bấm (Resolve
// hoặc Cancel), hết hạn giờ, hoặc app đóng (a.ctx bị huỷ). Hai lối sau đều
// trả về cancel=true để nhánh gọi đi tiếp theo đúng một đường: ghi #N/A.
// Giá trị thứ hai phân biệt "người dùng đã trả lời" với "tự bỏ cuộc", chỉ
// để nói đúng câu trong log.
func (a *App) waitForTMDTResolution(timeout time.Duration) (tmdtResolution, bool) {
	// Store(true) ở đây là để gọi trực tiếp (test) vẫn nhận được phản hồi;
	// bản thật đã bật cờ TRƯỚC khi phát tmdt:missing — xem processTMDTFile.
	a.tmdtWaiting.Store(true)
	defer a.tmdtWaiting.Store(false)

	var done <-chan struct{}
	if a.ctx != nil {
		done = a.ctx.Done()
	}
	select {
	case res := <-a.tmdtResolve:
		return res, true
	case <-time.After(timeout):
		return tmdtResolution{cancel: true}, false
	case <-done:
		return tmdtResolution{cancel: true}, false
	}
}

// parseTMDTRange đổi khoảng ngày của frontend thành mốc giờ VN và KIỂM LẠI
// cả hai ràng buộc. Kiểm lại ở backend chứ không tin frontend: một bản
// frontend cũ hoặc một lời gọi qua bindings có thể gửi khoảng 3 tháng, và
// đó là 90 lần gọi API vô ích cùng một file Excel sai.
func parseTMDTRange(r TMDTDateRange, today time.Time) (from, to time.Time, err error) {
	const layout = "2006-01-02"
	fromDay, err := time.ParseInLocation(layout, strings.TrimSpace(r.From), haravan.VNLocation)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("ngày bắt đầu %q không đúng dạng YYYY-MM-DD", r.From)
	}
	toDay, err := time.ParseInLocation(layout, strings.TrimSpace(r.To), haravan.VNLocation)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("ngày kết thúc %q không đúng dạng YYYY-MM-DD", r.To)
	}
	if toDay.Before(fromDay) {
		return time.Time{}, time.Time{}, fmt.Errorf("ngày bắt đầu phải trước ngày kết thúc")
	}
	yesterday := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, haravan.VNLocation).AddDate(0, 0, -1)
	if toDay.After(yesterday) {
		return time.Time{}, time.Time{}, fmt.Errorf("chỉ lấy được dữ liệu đến hết ngày hôm qua (%s)", yesterday.Format("02/01/2006"))
	}
	if days := int(toDay.Sub(fromDay).Hours()/24) + 1; days > 7 {
		return time.Time{}, time.Time{}, fmt.Errorf("khoảng thời gian tối đa 7 ngày, đang chọn %d ngày", days)
	}
	from = time.Date(fromDay.Year(), fromDay.Month(), fromDay.Day(), 0, 0, 0, 0, haravan.VNLocation)
	to = time.Date(toDay.Year(), toDay.Month(), toDay.Day(), 23, 59, 59, 0, haravan.VNLocation)
	return from, to, nil
}

// summaryKeyCount là số liệu gộp của một nhóm (shop, ngày). Bốn trường
// money/qty/skus/orders tồn tại vì tin Zalo cần chúng: dòng tóm tắt là
// thứ DUY NHẤT frontend nhìn thấy về đơn TMĐT (từng dòng hàng không đổ
// lên bảng), nên số liệu nào tin nhắn cần thì phải đi kèm dòng này.
type summaryKeyCount struct {
	shop    string
	date    string
	channel string
	misa    string
	shipTo  string
	orders  int
	lines   int
	hasNA   bool
	money   float64  // Σ(X×Y) — TRƯỚC VAT, đúng bằng tổng cột Z vừa ghi
	qty     float64  // Σ cột X
	skus    []string // mã thành phẩm KHÁC NHAU, đã bỏ #N/A
	// excelRows là số dòng TUYỆT ĐỐI trong sổ đặt hàng của riêng nhóm này.
	// KHÔNG liên tục: các nhóm ghi đan xen nhau theo thứ tự dòng gốc.
	excelRows []int
}

// summaryTMDTRows đổi số liệu gộp thành dòng cho bảng kết quả. Đơn TMĐT
// KHÔNG đổ từng dòng lên bảng: một tuần có thể ~2.500 đơn / ~5.000 dòng,
// vừa làm ngập bảng vừa phá cơ chế tick chọn PO để gửi Zalo.
func summaryTMDTRows(fileName string, groups []summaryKeyCount) []processing.OrderRow {
	rows := make([]processing.OrderRow, 0, len(groups))
	for _, g := range groups {
		kind := processing.StatusKindDone
		status := fmt.Sprintf("%s - %d đơn / %d dòng", processing.StatusDone, g.orders, g.lines)
		if g.hasNA {
			kind = processing.StatusKindWarning
			status = fmt.Sprintf("%s - %d đơn / %d dòng, còn mã #N/A", processing.StatusWarning, g.orders, g.lines)
		}
		rows = append(rows, processing.OrderRow{
			FileName: fileName,
			// SourceID gom MỌI NGÀY của cùng một shop về một khoá: frontend
			// (groupKeyFor) dùng đúng trường này để quyết định "một tin
			// Zalo", y hệt cách JIT gom nhiều trang PDF về một file. Bảng
			// kết quả vẫn giữ từng ngày một dòng để còn đối chiếu.
			SourceID: "tmdt|" + g.shop,
			// Page mang mã kho vì cột shipTo KHÔNG hiện trên bảng kết quả
			// (xem COLUMNS trong ResultTable.tsx); ShipTo bên dưới mới là
			// trường tin nhắn đọc.
			Page:        g.shipTo,
			System:      "TMĐT-" + g.channel,
			MaKhachHang: g.misa,
			PO:          fmt.Sprintf("%s · %s", g.shop, g.date),
			Status:      status,
			StatusKind:  kind,
			// DonGia mang TỔNG tiền của nhóm, không phải đơn giá một mã —
			// giống hệt điều nhánh JIT đã làm với cột này, và frontend cộng
			// dồn qua các ngày bằng đúng một phép cộng cho cả hai nhánh.
			DonGia:    strconv.FormatFloat(g.money, 'f', 0, 64),
			ShipTo:    g.shipTo,
			EntryDate: g.date,
			// Làm tròn thay vì cắt: qty là float64 vì Số lượng × SLTP đi qua
			// phép nhân số thực, và 7 cộng dồn có thể ra 6,999999999999999 —
			// int() sẽ cắt xuống 6, sai một sản phẩm trong tin nhắn.
			TotalQty:    int(math.Round(g.qty)),
			SKUs:        g.skus,
			TotalOrders: g.orders,
			ExcelRows:   g.excelRows,
		})
	}
	return rows
}

// processTMDTFile là toàn bộ nhánh TMĐT của một file. Trả về các dòng tóm
// tắt đã phát, để runReservedBatch đếm stt như với mọi file khác.
//
// recover() ở đây có cùng lý do như trong processOne: một file hỏng chỉ
// được làm hỏng phần việc của chính nó, không được giết cả lô.
func (a *App) processTMDTFile(emitter Emitter, path string, rng TMDTDateRange, emit func(processing.OrderRow)) (rows []processing.OrderRow, err error) {
	defer func() {
		if r := recover(); r != nil {
			rows, err = nil, fmt.Errorf("panic: %v", r)
		}
	}()

	fail := func(format string, args ...any) ([]processing.OrderRow, error) {
		msg := fmt.Sprintf(format, args...)
		emitter.Emit("process:log", "❌ "+msg)
		return nil, fmt.Errorf("%s", msg)
	}

	// File TMĐT lọt vào lô mà KHÔNG có khoảng ngày là chuyện người dùng
	// thấy được (frontend cũ, hoặc gọi thẳng qua bindings), không phải lỗi
	// lập trình: báo thành một dòng thất bại rồi để lô chạy tiếp, thay vì
	// để parseTMDTRange trả câu 'ngày bắt đầu "" không đúng dạng' khó hiểu.
	if strings.TrimSpace(rng.From) == "" && strings.TrimSpace(rng.To) == "" {
		return fail("chưa chọn khoảng thời gian cho file TMĐT này — bấm Xử lý lại và chọn khoảng ngày trong hộp thoại lịch")
	}

	settings, err := a.appSettingsStore.Load(resolveRepoFile("settings.ini"))
	if err != nil {
		return fail("không đọc được cấu hình: %v", err)
	}
	token := strings.TrimSpace(settings.Haravan["access_token"])
	if token == "" {
		return fail("chưa có access token Haravan — mở Cài đặt ▸ Haravan (TMĐT) và điền khoá access_token")
	}

	from, to, err := parseTMDTRange(rng, time.Now().In(haravan.VNLocation))
	if err != nil {
		return fail("khoảng thời gian không hợp lệ: %v", err)
	}

	tables, err := lookup.Load(path)
	if err != nil {
		return fail("không đọc được bảng tra cứu trong %s: %v", path, err)
	}

	// shopFilter hiện thực khoá cấu hình exclude_shops: danh sách shop bỏ
	// qua, ngăn bởi dấu phẩy. Rỗng (mặc định) = không loại đơn nào.
	shopFilter := haravan.NewShopFilter(settings.Haravan["exclude_shops"])
	if shopFilter.Len() > 0 {
		emitter.Emit("process:log", fmt.Sprintf("ℹ️ Bỏ qua đơn của shop: %s (khoá exclude_shops trong Cài đặt).",
			strings.Join(shopFilter.Names(), ", ")))
	}

	emitter.Emit("process:log", fmt.Sprintf("⏳ Đang lấy đơn TMĐT từ %s đến %s...",
		from.Format("02/01/2006"), to.Format("02/01/2006")))

	client := haravan.NewClient(token)
	// Logger của client in ra stdout kèm URL request — KHÔNG bao giờ chứa
	// token (token nằm ở header Authorization), nhưng vẫn tắt để log của
	// app chỉ có một nguồn duy nhất là process:log.
	client.Logger = nil

	// Dùng context của Wails khi có: đóng app giữa lúc đang tải 50 trang
	// thì vòng lặp HTTP dừng ngay thay vì chạy nốt trong nền.
	ctx := context.Background()
	if a.ctx != nil {
		ctx = a.ctx
	}

	var lines []tmdt.OrderLine
	boQua := 0
	opt := haravan.ListOptions{CreatedAtMin: from, CreatedAtMax: to}
	err = client.ListOrders(ctx, opt, func(page int, orders []haravan.Order) error {
		for i := range orders {
			o := &orders[i]
			shop := haravan.ShopName(o)
			if shopFilter.Excluded(shop) {
				boQua++
				continue
			}
			// DetectChannel trả "Shopee" | "TikTok Shop" | "" — chuỗi rỗng
			// nghĩa là không phải đơn sàn, bỏ qua.
			channel := haravan.DetectChannel(o, haravan.DefaultChannelRules)
			if channel == "" {
				continue
			}
			items := o.LineItems
			if len(items) == 0 {
				continue
			}
			for j := range items {
				li := &items[j]
				lines = append(lines, tmdt.OrderLine{
					OrderCode:    firstNonEmptyTMDT(o.Name, o.OrderNumber),
					Shop:         shop,
					KhoBan:       o.LocationName,
					KenhBanHang:  channel,
					CreatedAt:    o.CreatedAt.InVN(),
					Quantity:     float64(li.Quantity),
					Title:        li.Title,
					VariantTitle: li.VariantTitle,
					Price:        li.Price.Float(),
					Subtotal:     o.SubtotalPrice.Float(),
					Total:        o.TotalPrice.Float(),
					SKU:          li.SKU,
					Attributes:   haravan.LineItemAttributes(li),
				})
			}
		}
		emitter.Emit("process:log", fmt.Sprintf("   ...đã tải trang %d (%d dòng hàng)", page, len(lines)))
		return nil
	})
	if err != nil {
		return fail("gọi Haravan API thất bại: %v", err)
	}
	if boQua > 0 {
		emitter.Emit("process:log", fmt.Sprintf("ℹ️ Đã bỏ qua %d đơn thuộc danh sách exclude_shops.", boQua))
	}
	if len(lines) == 0 {
		emitter.Emit("process:log", "⚠️ Không có đơn TMĐT nào trong khoảng thời gian đã chọn.")
		return nil, nil
	}

	namer := a.tmdtProductNamer()
	res := tmdt.Build(lines, tables, tmdt.Options{ProductName: namer})

	if len(res.Missing) > 0 {
		// Dọn phản hồi lạc còn sót lại (file TMĐT trước vừa hết hạn giờ
		// đúng lúc người dùng bấm) rồi mới bật cờ chờ, và bật cờ TRƯỚC khi
		// phát tmdt:missing: frontend có thể trả lời gần như tức thì, mà
		// Resolve/Cancel đến trước lúc cờ bật sẽ bị chính cờ đó từ chối.
		select {
		case <-a.tmdtResolve:
		default:
		}
		a.tmdtWaiting.Store(true)

		emitter.Emit("process:log", fmt.Sprintf("⚠️ Có %d mã chưa khai báo trong sheet \"data shop\" — đang chờ bổ sung...", len(res.Missing)))
		emitter.Emit("tmdt:missing", res.Missing)

		resolution, answered := a.waitForTMDTResolution(tmdtMissingTimeout)
		switch {
		case !answered:
			emitter.Emit("process:log", "⚠️ Hết thời gian chờ khai mã — các dòng thiếu sẽ mang #N/A.")
		case resolution.cancel:
			emitter.Emit("process:log", "⚠️ Đã bỏ qua khai mã — các dòng thiếu sẽ mang #N/A.")
		default:
			combos := make([]lookup.ComboRow, 0, len(resolution.entries))
			for _, e := range resolution.entries {
				if strings.TrimSpace(e.TP[0]) == "" {
					continue // để trống = giữ #N/A cho mục này
				}
				combos = append(combos, lookup.ComboRow{
					Product: e.Product, Variant: e.Variant, Combo: e.Combo,
					TP: e.TP, SL: e.SL,
				})
			}
			if len(combos) > 0 {
				if _, err := lookup.AppendComboRows(path, combos); err != nil {
					return fail("không ghi được vào sheet \"data shop\": %v", err)
				}
				emitter.Emit("process:log", fmt.Sprintf("✅ Đã bổ sung %d dòng vào sheet \"data shop\".", len(combos)))
				tables, err = lookup.Load(path)
				if err != nil {
					return fail("không nạp lại được bảng tra cứu: %v", err)
				}
				res = tmdt.Build(lines, tables, tmdt.Options{ProductName: namer})
			}
		}
	}

	for _, line := range missingShopLogLines(res.MissingShops) {
		emitter.Emit("process:log", line)
	}
	for _, line := range noComponentLogLines(res.NoComponent) {
		emitter.Emit("process:log", line)
	}

	if err := tmdt.WriteHaravanSheet(path, res.SheetRows); err != nil {
		return fail("không ghi được sheet %q (file có đang mở trong Excel không?): %v", tmdt.SheetHaravan, err)
	}
	emitter.Emit("process:log", fmt.Sprintf("✅ Đã ghi %d dòng vào sheet %q.", len(res.SheetRows), tmdt.SheetHaravan))

	// GIỮ startRow: đó là thứ duy nhất nối mỗi dòng tóm tắt với những
	// dòng thật của nó trong sổ đặt hàng, và push MISA dựa vào đó để tách
	// file theo nhánh kế toán. Trước đây nó bị vứt (`if _, err :=`) nên
	// mọi đơn TMĐT đều bị modal push báo "chưa có dòng nào trong sổ đặt hàng".
	startRow, err := excelwriter.WriteTMDTRows(a.excelPath, res.OrderRows)
	if err != nil {
		return fail("không ghi được dondathang.xlsx (file có đang mở trong Excel không?): %v", err)
	}
	emitter.Emit("process:log", fmt.Sprintf("✅ Đã ghi %d dòng vào dondathang.xlsx.", len(res.OrderRows)))

	rows = summaryTMDTRows(baseNameTMDT(path), groupTMDTSummary(res, startRow))
	for _, row := range rows {
		emit(row)
	}
	return rows, nil
}

// missingShopLogLines dựng cảnh báo cho những shop chưa có trong sheet "Mã
// misa". Sắp xếp theo tên để log không đảo thứ tự giữa các lần chạy (map
// của Go duyệt ngẫu nhiên), và tách riêng khoá tmdt.ShopKhongTen: đó là
// NHÃN thay cho tên rỗng, không phải tên shop, nên câu 'Shop "(đơn không
// có tên shop)" chưa có trong sheet...' sẽ vô nghĩa.
func missingShopLogLines(shops map[string]int) []string {
	if len(shops) == 0 {
		return nil
	}
	names := make([]string, 0, len(shops))
	for name := range shops {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]string, 0, len(names))
	for _, name := range names {
		if name == tmdt.ShopKhongTen {
			out = append(out, fmt.Sprintf("⚠️ Có %d dòng hàng thuộc đơn không mang tên shop nên không tra được mã misa (→ Mã khách hàng = %s).",
				shops[name], lookup.NotAvailable))
			continue
		}
		out = append(out, fmt.Sprintf("⚠️ Shop %q chưa có trong sheet %q (%d dòng → Mã khách hàng = %s)",
			name, lookup.SheetMisa, shops[name], lookup.NotAvailable))
	}
	return out
}

// noComponentLogLines dựng log cho Result.NoComponent, và đây là chỗ HAI
// nguyên nhân phải được nói ở HAI mức khác nhau:
//
//   - tmdt.KhongKhaiThanhPham: dòng tra cứu cố ý không khai mã thành phẩm
//     (bảng thật đang có 6 dòng quà tặng như vậy) ⇒ xuất hiện ở gần như MỌI
//     lần chạy. Báo nó bằng ⚠️ là dựng lại đúng cái báo động giả mà vòng
//     rà soát trước đã gỡ bỏ, nên gộp thành MỘT dòng thông tin.
//   - tmdt.SLTPKhongDocDuoc: có khai MÃ TP mà SLTP không đọc được ⇒ đơn
//     hàng thật bị bỏ khỏi file hạch toán. Đây là lỗi dữ liệu người dùng
//     phải sửa, nên mỗi mã một dòng ⚠️ chỉ đúng tên mã.
func noComponentLogLines(nc map[string]int) []string {
	if len(nc) == 0 {
		return nil
	}
	var (
		quaTangMa   []string
		quaTangDong int
		loiKeys     []string
	)
	for key := range nc {
		switch {
		case strings.HasPrefix(key, tmdt.KhongKhaiThanhPham):
			quaTangMa = append(quaTangMa, key)
			quaTangDong += nc[key]
		case strings.HasPrefix(key, tmdt.SLTPKhongDocDuoc):
			loiKeys = append(loiKeys, key)
		}
	}
	sort.Strings(loiKeys)

	out := make([]string, 0, len(loiKeys)+1)
	if len(quaTangMa) > 0 {
		out = append(out, fmt.Sprintf("ℹ️ %d dòng hàng thuộc %d mã cố ý không khai mã thành phẩm (quà tặng) nên không vào dondathang.xlsx — đúng thiết kế, không cần sửa.",
			quaTangDong, len(quaTangMa)))
	}
	for _, key := range loiKeys {
		out = append(out, fmt.Sprintf("⚠️ Mã %s có khai MÃ TP nhưng không đọc được SLTP nào — %d dòng hàng bị bỏ khỏi dondathang.xlsx, hãy sửa cột SLTP trong sheet %q.",
			nhanMaTraCuu(strings.TrimPrefix(key, tmdt.SLTPKhongDocDuoc)), nc[key], lookup.SheetDataShop))
	}
	return out
}

// nhanMaTraCuu đổi khoá tmdt.MissingKey thành nhãn người đọc được. Hai
// tiền tố "sku:"/"pv:" là hình dạng khoá do tmdt.MissingKey sinh ra; khoá
// lạ (nếu hình dạng đó đổi) rơi xuống trả nguyên văn — vẫn đọc được, thay
// vì gán nhầm nhãn.
func nhanMaTraCuu(key string) string {
	switch {
	case strings.HasPrefix(key, "sku:"):
		return strings.TrimPrefix(key, "sku:")
	case strings.HasPrefix(key, "pv:"):
		parts := strings.SplitN(strings.TrimPrefix(key, "pv:"), "|", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
			return fmt.Sprintf("%s (%s)", parts[0], parts[1])
		}
		return parts[0]
	}
	return key
}

// tmdtProductNamer lấy hàm tra tên hàng theo mã thành phẩm từ Store đang
// dùng. Trả về hàm luôn rỗng khi processor là bản giả (test) hoặc chưa nạp
// xong dữ liệu — thiếu tên hàng không đáng để chặn cả lần chạy.
func (a *App) tmdtProductNamer() func(string) string {
	rp, ok := a.processor.(*processing.RealProcessor)
	if !ok || rp == nil || rp.Store == nil {
		return func(string) string { return "" }
	}
	store := rp.Store
	return func(tp string) string {
		info, found := store.GetProductInfo(tp)
		if !found {
			return ""
		}
		return info.Name
	}
}

// groupTMDTSummary gộp kết quả theo (shop, ngày) — đơn vị soát tự nhiên
// của người dùng: họ đối chiếu số đơn từng shop từng ngày với trang quản
// trị sàn.
func groupTMDTSummary(res tmdt.Result, startRow int) []summaryKeyCount {
	type key struct{ shop, date string }
	// Khoá của hai bộ đếm dưới đây là STRUCT chứ không phải chuỗi nối:
	// shop+ngày+mã nối thẳng có thể trùng nhau khi một trong ba phần rỗng
	// (ngày rỗng ở dòng dữ liệu lạ), và một va chạm ở đây nuốt mất một đơn
	// hoặc một mã khỏi tin nhắn mà không có dấu hiệu gì.
	type seen struct{ shop, date, value string }

	agg := map[key]*summaryKeyCount{}
	seenOrder := map[seen]bool{}

	seenSKU := map[seen]bool{}

	for i, r := range res.OrderRows {
		// EntryDate là "dd/mm/yyyy"; PO của dòng tóm tắt cần đúng dạng đó.
		shop := shopFromDescription(r.Description)
		k := key{shop: shop, date: r.EntryDate}
		g, ok := agg[k]
		if !ok {
			g = &summaryKeyCount{
				shop: shop, date: r.EntryDate,
				channel: channelFromOrderNumber(r.OrderNumber),
				misa:    r.CustomerCode, shipTo: r.ShipTo,
				// Khởi tạo rỗng chứ không để nil: nhóm mà MỌI dòng đều #N/A
				// sẽ không append mã nào, và slice nil ra JSON là `null` —
				// frontend lặp trên null là ném lỗi giữa lúc dựng tin nhắn.
				skus: []string{},
			}
			agg[k] = g
		}
		g.lines++
		// WriteTMDTRows ghi res.OrderRows theo đúng thứ tự, liên tiếp từ
		// startRow — nên dòng Excel của res.OrderRows[i] là startRow+i.
		g.excelRows = append(g.excelRows, startRow+i)
		g.money += r.Qty * r.UnitPrice
		g.qty += r.Qty
		if ord := (seen{k.shop, k.date, r.Note}); !seenOrder[ord] {
			seenOrder[ord] = true
			g.orders++
		}
		// #N/A không phải một mã hàng: đếm nó vào sẽ báo cho người nhận số
		// mã cao hơn thực tế, đúng lúc dữ liệu đang có vấn đề.
		if sk := (seen{k.shop, k.date, r.SKU}); r.SKU != "" && r.SKU != lookup.NotAvailable && !seenSKU[sk] {
			seenSKU[sk] = true
			g.skus = append(g.skus, r.SKU)
		}
		if r.SKU == lookup.NotAvailable || r.CustomerCode == lookup.NotAvailable {
			g.hasNA = true
		}
	}

	out := make([]summaryKeyCount, 0, len(agg))
	for _, g := range agg {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].shop != out[j].shop {
			return out[i].shop < out[j].shop
		}
		return out[i].date < out[j].date
	})
	return out
}

// shopFromDescription tách tên shop ra khỏi cột Diễn giải, vốn có dạng
// "TMĐT-{kênh} - {shop} - {mã đơn} - Ngày đổ {ngày} - {kho}". Đọc lại từ
// Diễn giải thay vì mang thêm một trường chỉ để tóm tắt: TMDTRow là hợp
// đồng ghi Excel, không phải chỗ nhồi dữ liệu phục vụ UI.
//
// Neo từ CUỐI chuỗi: ba phần cuối (mã đơn, "Ngày đổ ...", kho) do app tự
// ghép nên chắc chắn không chứa " - ", còn TÊN SHOP là do người dùng đặt
// trên sàn và hoàn toàn có thể chứa (" Blue - Chính hãng"). Cắt từ đầu sẽ
// trả về nửa tên; ghép lại phần giữa mới ra đúng tên.
func shopFromDescription(desc string) string {
	parts := strings.Split(desc, " - ")
	const soPhanCuoi = 3 // mã đơn + "Ngày đổ ..." + kho
	if len(parts) >= soPhanCuoi+2 {
		return strings.TrimSpace(strings.Join(parts[1:len(parts)-soPhanCuoi], " - "))
	}
	// Diễn giải lạ (dữ liệu cũ, chuỗi rỗng): trả phần thứ hai nếu có, chứ
	// không panic — dòng tóm tắt sai tên shop vẫn hơn là hỏng cả lô.
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func channelFromOrderNumber(orderNumber string) string {
	// "ĐĐHTMĐT-TikTok-585694..." → "TikTok"
	parts := strings.SplitN(orderNumber, "-", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func firstNonEmptyTMDT(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func baseNameTMDT(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}
