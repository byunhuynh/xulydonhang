package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"order-processor/internal/misa"
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
	// Key là khoá nhóm của đơn trên bảng kết quả (groupKeyFor phía
	// frontend) — trạng thái từng đơn báo ngược về theo đúng khoá này.
	Key       string `json:"key"`
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
func (a *App) PushMisa(jobs []MisaPushJob, force bool) {
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
	go a.runMisaPush(a.emitter, jobs, force)
}

func (a *App) runMisaPush(emitter Emitter, jobs []MisaPushJob, force bool) {
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

	jobsByBranch := map[string][]MisaPushJob{}
	for _, job := range jobs {
		jobsByBranch[job.Branch] = append(jobsByBranch[job.Branch], job)
	}

	for _, branch := range misaBranchOrder {
		if len(jobsByBranch[branch]) == 0 {
			continue
		}
		a.pushOneBranch(emitter, settings.Misa, branch, jobsByBranch[branch], force)
	}
}

// Trạng thái của TỪNG đơn sau một lượt đẩy — phát qua misa:orders. MISA
// chỉ nhận/trả theo nhánh (một file một lần), nhưng bước kiểm tra của họ
// trả về từng chứng từ kèm refno, đủ để chỉ đích danh đơn nào hỏng.
const (
	misaOrderPushing = "pushing" // đang đẩy
	misaOrderOK      = "ok"      // đã ghi vào sổ
	misaOrderExists  = "exists"  // số đơn hàng đã có trên MISA
	misaOrderInvalid = "invalid" // MISA từ chối vì dữ liệu của chính đơn này
	misaOrderHeld    = "held"    // hợp lệ, nhưng nhánh không ghi vì đơn khác lỗi
	misaOrderFailed  = "failed"  // cả nhánh hỏng trước khi MISA kịp kiểm tra
	misaOrderUnknown = "unknown" // lỗi giữa lúc ghi sổ — chưa rõ đã vào sổ chưa
)

// MisaOrderStatus là trạng thái một đơn, khoá theo MisaPushJob.Key.
type MisaOrderStatus struct {
	Key     string `json:"key"`
	PO      string `json:"po"`
	Branch  string `json:"branch"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func emitOrderStatuses(emitter Emitter, statuses []MisaOrderStatus) {
	if len(statuses) > 0 {
		emitter.Emit("misa:orders", statuses)
	}
}

// sameStatusForAll gán cùng một trạng thái cho mọi đơn của nhánh — dùng
// khi lỗi xảy ra trước khi MISA đọc tới từng chứng từ.
func sameStatusForAll(jobs []MisaPushJob, branch, status, message string) []MisaOrderStatus {
	out := make([]MisaOrderStatus, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, MisaOrderStatus{Key: j.Key, PO: j.PO, Branch: branch, Status: status, Message: message})
	}
	return out
}

func normalizeRefNo(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// classifyMisaOrders suy trạng thái từng đơn từ kết quả của cả nhánh.
// numbers là "Số đơn hàng" (cột B) của từng dòng Excel — MISA trả lỗi theo
// refno, chính là cột này, nên đây là cách nối lỗi về đúng đơn.
func classifyMisaOrders(jobs []MisaPushJob, branch string, numbers map[int]string,
	res *misa.ImportResult, pushErr error, force bool) []MisaOrderStatus {

	errByRef := map[string][]string{}
	if res != nil {
		for _, e := range res.RowErrors {
			if ref := normalizeRefNo(e.RefNo); ref != "" {
				errByRef[ref] = append(errByRef[ref], e.Description)
			}
		}
	}

	out := make([]MisaOrderStatus, 0, len(jobs))
	for _, j := range jobs {
		st := MisaOrderStatus{Key: j.Key, PO: j.PO, Branch: branch}

		// Một đơn có thể trải trên nhiều dòng (nhiều mã hàng) và — với BigC
		// — nhiều số đơn hàng; gom lỗi của mọi số đơn thuộc đơn này.
		seenRef := map[string]bool{}
		seenDesc := map[string]bool{}
		var descs []string
		for _, r := range j.ExcelRows {
			ref := normalizeRefNo(numbers[r])
			if ref == "" || seenRef[ref] {
				continue
			}
			seenRef[ref] = true
			for _, d := range errByRef[ref] {
				if !seenDesc[d] {
					seenDesc[d] = true
					descs = append(descs, d)
				}
			}
		}

		switch {
		case len(descs) > 0:
			st.Status = misaOrderInvalid
			for _, d := range descs {
				if strings.Contains(strings.ToLower(d), "đã tồn tại") {
					st.Status = misaOrderExists
				}
			}
			st.Message = strings.Join(descs, "; ")
			if force && (pushErr == nil || (res != nil && res.CommitStarted)) {
				st.Message += " — đã bỏ qua, không ghi đơn này"
			}
		case pushErr == nil:
			st.Status = misaOrderOK
			st.Message = "đã ghi vào sổ"
		case res != nil && res.CommitStarted:
			st.Status = misaOrderUnknown
			st.Message = fmt.Sprintf("lỗi giữa lúc ghi sổ (%v) — kiểm tra trên MISA trước khi đẩy lại", pushErr)
		case res != nil && len(res.RowErrors) > 0:
			st.Status = misaOrderHeld
			st.Message = "hợp lệ, nhưng chưa ghi vì có đơn khác cùng nhánh bị lỗi"
		default:
			st.Status = misaOrderFailed
			st.Message = pushErr.Error()
		}
		out = append(out, st)
	}
	return out
}

// pushOneBranch tách workbook cho đúng một nhánh rồi đẩy. Mọi lỗi ở đây
// chỉ dừng nhánh này — nhánh còn lại vẫn chạy, vì người dùng thà vào sổ
// được một nửa còn hơn phải làm lại cả hai.
func (a *App) pushOneBranch(emitter Emitter, misaCfg map[string]string, branch string, jobs []MisaPushJob, force bool) {
	label := misaBranchLabel[branch]

	var all []int
	for _, j := range jobs {
		all = append(all, j.ExcelRows...)
	}
	rows := dedupSorted(all)

	emitOrderStatuses(emitter, sameStatusForAll(jobs, branch, misaOrderPushing, "đang đẩy lên MISA"))

	fail := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		emitter.Emit("misa:log", fmt.Sprintf("❌ %s: %s", label, msg))
		emitOrderStatuses(emitter, sameStatusForAll(jobs, branch, misaOrderFailed, msg))
		emitter.Emit("misa:pushed", map[string]any{
			"branch": branch, "ok": false, "valid": 0, "invalid": 0, "message": msg,
			// Lỗi ở đây (chưa khai bộ dữ liệu, không tách được file, đăng
			// nhập hỏng) là loại mà ghi phần hợp lệ không cứu được gì.
			"canForce": false,
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

	// Giữ excelMu đúng lúc đọc + tách workbook, KHÔNG giữ suốt lời gọi
	// mạng bên dưới (đẩy lên MISA mất hàng chục giây) - giữ khoá cả quãng
	// đó sẽ treo mọi thao tác Excel khác (ConfirmPrice, UpdateJITPeriod,
	// và cả ProcessFiles một khi batch mới được phép chạy lại). Đây là
	// khoá CHỐNG một batch xử lý mới ghi đè a.excelPath giữa lúc
	// SplitWorkbook đang đọc nó - reserveBatch() đã chặn batch mới trong
	// lúc a.pushing bật, nhưng khoá này vẫn cần cho đúng phần đọc/ghi
	// thật sự tranh chấp, cùng khuôn ConfirmPrice/UpdateJITPeriod dùng.
	var numbers map[int]string
	splitErr := func() error {
		a.excelMu.Lock()
		defer a.excelMu.Unlock()
		var err error
		if numbers, err = misapush.OrderNumbers(a.excelPath, rows); err != nil {
			return err
		}
		return misapush.SplitWorkbook(a.excelPath, tmpPath, rows)
	}()
	if splitErr != nil {
		fail("không tách được đơn của nhánh: %v", splitErr)
		return
	}

	emitter.Emit("misa:log", fmt.Sprintf("📤 %s: đang đẩy %d dòng lên %q…", label, len(rows), database))

	res, err := a.misaPusher.Push(context.Background(), misapush.Request{
		SessionPath: a.misaSessionPath,
		SidURL:      strings.TrimSpace(misaCfg["sid_url"]),
		Database:    database,
		FilePath:    tmpPath,
		Force:       force,
		Log:         func(line string) { emitter.Emit("misa:log", fmt.Sprintf("   %s: %s", label, line)) },
	})

	if err != nil {
		valid, invalid, canForce := 0, 0, false
		if res != nil {
			// Liệt kê ĐỦ dòng hỏng, không chỉ dòng đầu nằm trong thông điệp
			// lỗi — sửa được hết trong một lượt thay vì lặp lại từng dòng.
			for _, e := range res.RowErrors {
				emitter.Emit("misa:log", fmt.Sprintf("   %s: ✗ %s", label, e))
			}
			valid, invalid = res.ValidRows, len(res.RowErrors)
			// Chỉ đề nghị ghi phần hợp lệ khi MISA THẬT SỰ đã đọc được file
			// và chỉ ra dòng nào hỏng, VÀ còn dòng hợp lệ để ghi. Lỗi đăng
			// nhập, sai bộ dữ liệu hay thiếu cột bắt buộc thì Force không
			// cứu được gì — đề nghị ở đó chỉ khiến người dùng bấm vô ích.
			// Lượt đã bật Force rồi mà vẫn lỗi thì cũng không đề nghị lại.
			canForce = !force && invalid > 0 && valid > 0
		}
		msg := err.Error()
		emitter.Emit("misa:log", fmt.Sprintf("❌ %s: %s", label, msg))
		emitOrderStatuses(emitter, classifyMisaOrders(jobs, branch, numbers, res, err, force))
		emitter.Emit("misa:pushed", map[string]any{
			"branch": branch, "ok": false,
			"valid": valid, "invalid": invalid,
			"message": msg, "canForce": canForce,
		})
		return
	}

	emitter.Emit("misa:log", fmt.Sprintf("✅ %s: đã ghi vào sổ %d chứng từ hợp lệ, %d lỗi, %d bỏ qua",
		label, res.Valid, res.Invalid, res.Skipped))
	emitOrderStatuses(emitter, classifyMisaOrders(jobs, branch, numbers, res, nil, force))
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

// beforeClose chặn thoát app giữa lúc đang đẩy MISA. Lượt đẩy chạy trong
// goroutine của chính tiến trình này: thoát là cắt ngang nó, nhánh đang
// đẩy có thể đã hoặc chưa vào sổ, và lần mở sau không còn gì để biết.
// Đóng modal Push MISA thì KHÔNG sao — goroutine vẫn chạy nốt.
func (a *App) beforeClose(ctx context.Context) bool {
	if !a.pushing.Load() {
		return false
	}
	choice, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:  runtime.QuestionDialog,
		Title: "Đang đẩy đơn lên MISA",
		Message: "Lượt đẩy MISA vẫn đang chạy. Thoát bây giờ sẽ cắt ngang giữa chừng: " +
			"đơn có thể đã hoặc chưa vào sổ, phải tự kiểm tra trên MISA trước khi đẩy lại.\n\n" +
			"Vẫn thoát?",
		DefaultButton: "No",
		CancelButton:  "No",
	})
	if err != nil {
		// Không hiện được hộp thoại thì đừng nhốt người dùng trong app.
		return false
	}
	return choice != "Yes"
}
