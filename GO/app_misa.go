package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"order-processor/internal/misapush"
)

// MisaRouteInput là dòng ĐẦU của một nhóm đơn trên bảng kết quả — đủ để
// tính khoá định tuyến của cả nhóm.
type MisaRouteInput struct {
	System       string `json:"system"`
	CustomerCode string `json:"customerCode"`
	ShipTo       string `json:"shipTo"`
}

// MisaRouteInfo là khoá định tuyến đã phân giải: khoá máy đọc, nhãn cho
// người đọc, và nhánh mặc định ("" khi chưa map).
type MisaRouteInfo struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Branch string `json:"branch"`
}

// MisaResolveRoutes phân giải khoá + nhãn + nhánh mặc định cho từng nhóm
// đơn. Modal push gọi đúng một lần khi mở, cho cả lô.
//
// Quy tắc định tuyến CHỈ tồn tại ở đây. Viết lại nó bằng TypeScript sẽ
// tạo ra hai bản sao của một quy tắc kế toán, và bản nào lệch thì đơn vào
// nhầm sổ trong khi test của bên kia vẫn xanh.
func (a *App) MisaResolveRoutes(rows []MisaRouteInput) ([]MisaRouteInfo, error) {
	settings, err := a.GetAppSettings()
	if err != nil {
		return nil, err
	}
	out := make([]MisaRouteInfo, 0, len(rows))
	for _, r := range rows {
		key := misapush.RouteKey(r.System, r.CustomerCode, r.ShipTo)
		out = append(out, MisaRouteInfo{
			Key:    key,
			Label:  misapush.Label(key),
			Branch: misapush.Lookup(settings.MisaRouting, key),
		})
	}
	return out, nil
}

// MisaRouteOptions liệt kê mọi khoá định tuyến đã biết — hợp của bảng
// gieo và những khoá đã lưu trong cấu hình — sắp theo nhãn để danh sách
// trong Cài đặt không nhảy chỗ khi có khoá mới.
func (a *App) MisaRouteOptions() ([]MisaRouteInfo, error) {
	settings, err := a.GetAppSettings()
	if err != nil {
		return nil, err
	}

	branches := misapush.SeedRouting()
	// Giá trị đã lưu ĐÈ LÊN bảng gieo, kể cả khi người dùng đã đổi khác
	// mặc định — đây là điểm cả tính năng dựa vào.
	for k, v := range settings.MisaRouting {
		branches[k] = v
	}

	out := make([]MisaRouteInfo, 0, len(branches))
	for k, v := range branches {
		out = append(out, MisaRouteInfo{Key: k, Label: misapush.Label(k), Branch: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out, nil
}

// MisaPushJob là một nhóm đơn mà người dùng đã tick và đã gán nhánh.
type MisaPushJob struct {
	PO        string `json:"po"`
	RouteKey  string `json:"routeKey"`
	Branch    string `json:"branch"`
	ExcelRows []int  `json:"excelRows"`
}

// misaBranchOrder cố định thứ tự đẩy, để log và màn hình kết quả không
// đảo chỗ giữa hai lần chạy giống hệt nhau.
var misaBranchOrder = []string{misapush.BranchHaThanh, misapush.BranchHTLA}

// misaBranchLabel là tên hiển thị của nhánh; khoá lưu trữ vẫn là chuỗi
// máy đọc trong misapush.
var misaBranchLabel = map[string]string{
	misapush.BranchHaThanh: "Hà Thành",
	misapush.BranchHTLA:    "HTLA",
}

// misaDatabaseKey là khoá tra tên bộ dữ liệu trong Cài đặt > MISA.
var misaDatabaseKey = map[string]string{
	misapush.BranchHaThanh: "db_ha_thanh",
	misapush.BranchHTLA:    "db_htla",
}

// PushMisa đẩy các nhóm đơn đã chọn lên AMIS Kế toán trong một goroutine
// nền, phát misa:log/misa:pushed/misa:done — cùng khuôn
// SendZaloMessages/runZaloBatch.
func (a *App) PushMisa(jobs []MisaPushJob) {
	if len(jobs) == 0 {
		return
	}
	// Lô xử lý đang ghi vào CHÍNH workbook mà bước tách sắp đọc; cắt file
	// giữa lúc nó đang được ghi là đẩy đi một bản dở dang.
	if a.processing.Load() {
		a.emitter.Emit("misa:log", "⚠️ Đang xử lý đơn hàng, vui lòng đợi hoàn tất rồi đẩy lên MISA.")
		a.emitter.Emit("misa:done", nil)
		return
	}
	if !a.pushing.CompareAndSwap(false, true) {
		a.emitter.Emit("misa:log", "⚠️ Đã có một lượt đẩy MISA đang chạy, vui lòng đợi hoàn tất.")
		return
	}
	go a.runMisaPush(a.emitter, jobs)
}

func (a *App) runMisaPush(emitter Emitter, jobs []MisaPushJob) {
	defer func() {
		if r := recover(); r != nil {
			emitter.Emit("misa:log", fmt.Sprintf("❌ Lỗi không mong muốn: %v", r))
		}
		a.pushing.Store(false)
		emitter.Emit("misa:done", nil)
	}()

	settings, err := a.GetAppSettings()
	if err != nil {
		emitter.Emit("misa:log", fmt.Sprintf("❌ Không đọc được cấu hình: %v", err))
		return
	}

	rowsByBranch := map[string][]int{}
	for _, job := range jobs {
		rowsByBranch[job.Branch] = append(rowsByBranch[job.Branch], job.ExcelRows...)
	}

	for _, branch := range misaBranchOrder {
		rows := dedupSorted(rowsByBranch[branch])
		if len(rows) == 0 {
			continue
		}
		a.pushOneBranch(emitter, settings.Misa, branch, rows)
	}
}

// pushOneBranch tách workbook cho đúng một nhánh rồi đẩy. Mọi lỗi ở đây
// chỉ dừng nhánh này — nhánh còn lại vẫn chạy, vì người dùng thà vào sổ
// được một nửa còn hơn phải làm lại cả hai.
func (a *App) pushOneBranch(emitter Emitter, misaCfg map[string]string, branch string, rows []int) {
	label := misaBranchLabel[branch]

	fail := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		emitter.Emit("misa:log", fmt.Sprintf("❌ %s: %s", label, msg))
		emitter.Emit("misa:pushed", map[string]any{
			"branch": branch, "ok": false, "valid": 0, "invalid": 0, "message": msg,
		})
	}

	database := strings.TrimSpace(misaCfg[misaDatabaseKey[branch]])
	if database == "" {
		fail("chưa khai bộ dữ liệu kế toán (Cài đặt > MISA > %s)", misaDatabaseKey[branch])
		return
	}

	tmp, err := os.CreateTemp("", "misa-*.xlsx")
	if err != nil {
		fail("không tạo được file tạm: %v", err)
		return
	}
	tmpPath := tmp.Name()
	tmp.Close()
	// Xoá dù đi ra bằng đường nào: file này là bản sao của sổ đặt hàng,
	// để lại trong thư mục tạm là rò dữ liệu khách hàng.
	defer os.Remove(tmpPath)

	if err := misapush.SplitWorkbook(a.excelPath, tmpPath, rows); err != nil {
		fail("không tách được đơn của nhánh: %v", err)
		return
	}

	emitter.Emit("misa:log", fmt.Sprintf("📤 %s: đang đẩy %d dòng lên %q…", label, len(rows), database))

	res, err := a.misaPusher.Push(context.Background(), misapush.Request{
		SessionPath: a.misaSessionPath,
		SidURL:      strings.TrimSpace(misaCfg["sid_url"]),
		Database:    database,
		FilePath:    tmpPath,
		Log:         func(line string) { emitter.Emit("misa:log", fmt.Sprintf("   %s: %s", label, line)) },
	})

	if err != nil {
		// Liệt kê ĐỦ dòng hỏng, không chỉ dòng đầu nằm trong thông điệp
		// lỗi — sửa được hết trong một lượt thay vì lặp lại từng dòng.
		if res != nil {
			for _, e := range res.RowErrors {
				emitter.Emit("misa:log", fmt.Sprintf("   %s: ✗ %s", label, e))
			}
		}
		fail("%v", err)
		return
	}

	emitter.Emit("misa:log", fmt.Sprintf("✅ %s: đã ghi vào sổ %d chứng từ hợp lệ, %d lỗi, %d bỏ qua",
		label, res.Valid, res.Invalid, res.Skipped))
	emitter.Emit("misa:pushed", map[string]any{
		"branch": branch, "ok": true, "valid": res.Valid, "invalid": res.Invalid,
		"message": fmt.Sprintf("đã ghi %d chứng từ", res.Valid),
	})
}

// dedupSorted loại trùng và sắp tăng dần. Trùng là chuyện thường: hai
// nhóm đơn khác nhau vẫn có thể trỏ vào cùng một dòng Excel khi một dòng
// mang nhiều mã hàng.
func dedupSorted(rows []int) []int {
	if len(rows) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(rows))
	out := make([]int, 0, len(rows))
	for _, r := range rows {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	sort.Ints(out)
	return out
}
