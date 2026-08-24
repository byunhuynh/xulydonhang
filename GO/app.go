package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"order-processor/internal/applock"
	"order-processor/internal/appsettings"
	"order-processor/internal/config"
	"order-processor/internal/driveupload"
	"order-processor/internal/fileset"
	"order-processor/internal/processing"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
	"order-processor/internal/zalosend"
)

// Emitter trừu tượng hoá runtime.EventsEmit để logic của App test được mà
// không cần một Wails context thật.
type Emitter interface {
	Emit(eventName string, data ...interface{})
}

type wailsEmitter struct {
	ctx context.Context
}

func (e *wailsEmitter) Emit(eventName string, data ...interface{}) {
	runtime.EventsEmit(e.ctx, eventName, data...)
}

const orderFolderName = "đơn hàng"
const configFileName = "config.txt"

// App struct
type App struct {
	ctx              context.Context
	cfg              *config.Store
	appSettingsStore *appsettings.Store
	processor        processing.Processor
	emitter          Emitter
	orderDir         string
	excelPath        string
	resolvedRows     map[int]bool
	resolvedMu       sync.Mutex
	processing       atomic.Bool
	zaloSender       zalosend.ZaloSender
	sending          atomic.Bool
}

// resolveRepoFile looks for filename starting in the current working
// directory and then each parent directory up to 5 levels, returning
// the first path where the file actually exists. This is needed
// because data.xlsx/settings.ini live at the repo root, but the app's
// working directory differs between `wails dev` (GO/) and the built
// .exe (GO/build/bin/, confirmed empirically via Phase 1's config.txt
// landing there) — a single hardcoded relative path only works for
// one of the two. Falls back to the bare filename if not found
// anywhere, so the resulting "file not found" error from the caller
// is still informative rather than this silently returning garbage.
func resolveRepoFile(filename string) string {
	dir, err := os.Getwd()
	if err != nil {
		return filename
	}
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(dir, filename)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filename
}

// resolveRepoDir returns the directory resolveRepoFile would resolve
// filename into (the repo root, in practice), so callers that need a
// place to write a new file near data.xlsx/settings.ini (rather than
// open an existing one) can reuse the same directory-walk logic. Falls
// back to "." if filename can't be found anywhere in the walk, matching
// resolveRepoFile's own fallback.
func resolveRepoDir(filename string) string {
	resolved := resolveRepoFile(filename)
	if dir := filepath.Dir(resolved); dir != "" {
		return dir
	}
	return "."
}

// NewApp creates a new App application struct
func NewApp() (*App, error) {
	// Cấu hình (gid Google Sheets/Zalo/nhắc nhở) giờ đọc từ
	// settings.bhconfig thay vì settings.ini — appSettingsPath PHẢI tính
	// qua resolveRepoDir, KHÔNG dùng resolveRepoFile trực tiếp: file
	// .bhconfig chưa tồn tại ở lần chạy đầu tiên, resolveRepoFile chỉ
	// tìm file ĐÃ CÓ SẴN nên sẽ trả về đường dẫn tương đối sai chỗ (phụ
	// thuộc working directory hiện tại, khác nhau giữa `wails dev` và
	// bản .exe đã build). resolveRepoDir("settings.ini") lấy đúng thư
	// mục chứa settings.ini (file chắc chắn tồn tại vì app đã và đang
	// chạy dựa vào nó), cho ra đường dẫn ổn định dùng được cho cả đọc
	// lẫn ghi, mọi lần chạy.
	appSettingsPath := filepath.Join(resolveRepoDir("settings.ini"), "settings.bhconfig")
	appSettingsStore := appsettings.NewStore(appSettingsPath)
	settings, err := appSettingsStore.Load(resolveRepoFile("settings.ini"))
	if err != nil {
		return nil, fmt.Errorf("app: load app settings: %w", err)
	}

	// Customer/product data now comes from a live Google Sheet, not the
	// local data.xlsx file — a prior update to just this one machine's
	// data.xlsx never reached any OTHER machine running this app, the
	// exact class of staleness pricing/promotion data already avoids via
	// its own live fetch (pricing.HTTPSource). No offline fallback to
	// data.xlsx: a network failure here fails NewApp entirely, matching
	// this app's already-full dependency on the same network for live
	// pricing lookups mid-processing — a deliberate choice, not an
	// oversight (see productdata.LoadFromSheets' own doc comment).
	// data.xlsx itself is left on disk, untouched; nothing reads it
	// anymore.
	store, err := productdata.LoadFromSheets(settings.Gid, productdata.NewHTTPClient())
	if err != nil {
		return nil, fmt.Errorf("app: load customer/product data from Google Sheets: %w", err)
	}

	excelPath := resolveRepoFile("dondathang.xlsx")

	// orderDir must be resolved the same way excelPath/appSettingsPath
	// already are - a bare "đơn hàng" literal resolves relative to the
	// process's current working directory, which differs between
	// `wails dev` (run from GO/) and the built .exe (CWD depends on how
	// it was launched - a double-click sets it to the exe's own folder,
	// not the repo root two levels up). Left unresolved, EnsureMonthlyFolder
	// would silently create/scan an empty "đơn hàng" folder next to the
	// exe instead of the real one at the repo root, so reloading the
	// file list would show nothing even though real order files exist.
	orderDir := filepath.Join(resolveRepoDir("settings.ini"), orderFolderName)

	processor := &processing.RealProcessor{
		Store:       store,
		Pricing:     pricing.NewHTTPSource(settings.Gid),
		ExcelPath:   excelPath,
		DriveClient: driveupload.NewHTTPClient(),
	}

	app := &App{
		cfg:              config.NewStore(configFileName),
		appSettingsStore: appSettingsStore,
		processor:        processor,
		orderDir:         orderDir,
		excelPath:        excelPath,
		zaloSender: &zalosend.ChromedpSender{
			ProfileDir: filepath.Join(resolveRepoDir("settings.ini"), "zalo_profile"),
		},
	}

	processor.LogFunc = func(msg string) {
		if app.emitter != nil {
			app.emitter.Emit("process:log", msg)
		}
	}

	return app, nil
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.emitter = &wailsEmitter{ctx: ctx}

	runtime.OnFileDrop(ctx, func(x, y int, paths []string) {
		valid := fileset.FilterValid(paths)
		if len(valid) > 0 {
			a.emitter.Emit("files:dropped", valid)
		}
	})

	go a.runLockChecker(ctx)
}

// runLockChecker periodically re-checks applock.Check and emits its
// result as an "applock:status" event ("locked" | "unlocked" |
// "checking") for the frontend to react to live, without requiring an
// app restart for a revoked/renewed license to take effect. Checks
// immediately on startup, then every lockCheckInterval while healthy;
// any error (network unreachable, row not found, bad date) is reported
// as "checking" rather than "locked" - status genuinely undetermined,
// not a confirmed expiry - and retried sooner, at lockRetryInterval,
// until a determinate result comes back. Runs until ctx is cancelled
// (app shutdown).
func (a *App) runLockChecker(ctx context.Context) {
	const lockCheckInterval = 30 * time.Minute
	const lockRetryInterval = 1 * time.Minute

	client := applock.NewHTTPClient()
	wait := time.Duration(0) // fire immediately on the first iteration
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		status, err := applock.Check(client, time.Now())
		switch {
		case err != nil:
			a.emitter.Emit("applock:status", "checking")
			wait = lockRetryInterval
		case status.Locked:
			a.emitter.Emit("applock:status", "locked")
			wait = lockCheckInterval
		default:
			a.emitter.Emit("applock:status", "unlocked")
			wait = lockCheckInterval
		}
	}
}

// GetSTT trả về số thứ tự đơn hàng bắt đầu hiện tại.
func (a *App) GetSTT() (int, error) {
	return a.cfg.GetSTT()
}

// SetSTT ghi lại số thứ tự đơn hàng bắt đầu.
func (a *App) SetSTT(v int) error {
	return a.cfg.SetSTT(v)
}

// GetAppSettings trả về toàn bộ cấu hình hiện tại (gid/zalo/reminder)
// để popup cài đặt hiển thị.
func (a *App) GetAppSettings() (appsettings.Settings, error) {
	return a.appSettingsStore.Load(resolveRepoFile("settings.ini"))
}

// SaveAppSettings ghi cấu hình mới. Thay đổi gid CHỈ có hiệu lực ở lần
// khởi động app tiếp theo — Pricing/Store hiện tại (RealProcessor) đã
// được tạo sẵn với gid map cũ lúc NewApp chạy, không tự động cập nhật
// lại giữa phiên làm việc (quyết định rõ ràng của user, tránh phức tạp
// reload).
func (a *App) SaveAppSettings(settings appsettings.Settings) error {
	return a.appSettingsStore.Save(settings)
}

// ConfirmPrice ghi đè giá (cột Y) của một dòng sản phẩm đã bị đánh dấu
// sai giá, theo lựa chọn của người dùng — giữ giá trên PO hoặc dùng giá
// hệ thống. Từ chối khi đang có một batch xử lý chạy (ProcessFiles ghi
// vào CÙNG file Excel — cho 2 lần mở/lưu chạy đồng thời có thể khiến
// bên lưu sau âm thầm ghi đè mất các dòng bên kia vừa thêm, không có
// lỗi báo). Lần đầu gọi cho một dòng, yêu cầu dòng đó ĐANG ở trạng thái
// chờ xác nhận (còn comment cảnh báo sai giá — xem
// excelwriter.ConfirmPrice); các lần gọi SAU cho CÙNG dòng đó (người
// dùng đổi ý giữa giá PO và giá hệ thống) bỏ qua kiểm tra đó, vì comment
// đã bị xóa ngay từ lần xác nhận đầu tiên — xem excelwriter.SetPrice.
func (a *App) ConfirmPrice(row int, price float64) error {
	if a.processing.Load() {
		return fmt.Errorf("đang xử lý đơn hàng, vui lòng đợi hoàn tất trước khi áp dụng giá")
	}

	a.resolvedMu.Lock()
	alreadyResolved := a.resolvedRows[row]
	a.resolvedMu.Unlock()

	if alreadyResolved {
		return excelwriter.SetPrice(a.excelPath, row, price)
	}

	if err := excelwriter.ConfirmPrice(a.excelPath, row, price); err != nil {
		return err
	}

	a.resolvedMu.Lock()
	if a.resolvedRows == nil {
		a.resolvedRows = make(map[int]bool)
	}
	a.resolvedRows[row] = true
	a.resolvedMu.Unlock()
	return nil
}

// ScanOrderFolder quét thư mục "đơn hàng/MM-YYYY" hiện tại (tự tạo nếu
// thiếu) và trả về danh sách file hợp lệ.
func (a *App) ScanOrderFolder() ([]string, error) {
	dir, err := fileset.EnsureMonthlyFolder(a.orderDir, time.Now())
	if err != nil {
		return nil, err
	}
	return fileset.ListFiles(dir)
}

// SelectFiles mở dialog chọn nhiều file, lọc theo đuôi hợp lệ.
func (a *App) SelectFiles() ([]string, error) {
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Chọn file đơn hàng",
		Filters: []runtime.FileFilter{
			{DisplayName: "Đơn hàng (*.pdf;*.xlsx;*.txt)", Pattern: "*.pdf;*.xlsx;*.txt"},
		},
	})
	if err != nil {
		return nil, err
	}
	return fileset.FilterValid(paths), nil
}

// ProcessFiles chạy xử lý các file đã chọn trong nền, phát sự kiện
// process:log / process:row / process:done về frontend.
func (a *App) ProcessFiles(files []string, stt int) {
	if !a.processing.CompareAndSwap(false, true) {
		a.emitter.Emit("process:log", "⚠️ Đã có một batch đang xử lý, vui lòng đợi hoàn tất.")
		return
	}
	go a.runBatch(a.emitter, files, stt)
}

func (a *App) runBatch(emitter Emitter, files []string, stt int) {
	current := stt
	defer func() {
		if r := recover(); r != nil {
			emitter.Emit("process:log", fmt.Sprintf("❌ Lỗi không mong muốn: %v", r))
		}
		a.processing.Store(false)
		emitter.Emit("process:done", current)
	}()

	// Mirrors xulydonhang.py's xu_ly_don_hang: clear every existing data
	// row in dondathang.xlsx (via excelwriter.ClearOrderRows) BEFORE
	// writing this batch's results, so the file only ever holds the
	// most recent processing run's output rather than accumulating rows
	// across every click - confirmed as the real, intended production
	// behavior (App.py:545, xoa_du_lieu_don_dat_hang() called first
	// thing inside the "Xác nhận" button handler). If this fails (most
	// commonly: the file is currently open in Excel and locked), abort
	// the whole batch without processing anything - writing new rows on
	// top of stale ones that failed to clear would be worse than not
	// starting at all.
	if err := excelwriter.ClearOrderRows(a.excelPath); err != nil {
		emitter.Emit("process:log", fmt.Sprintf("❌ Không xóa được dữ liệu cũ trong dondathang.xlsx (có thể file đang mở trong Excel, hãy đóng lại rồi thử lại): %v", err))
		return
	}

	for _, f := range files {
		emitter.Emit("process:log", fmt.Sprintf("Đang xử lý %s...", filepath.Base(f)))
		rows, err := a.processOne(f, current)
		if err != nil {
			emitter.Emit("process:log", fmt.Sprintf("❌ Lỗi xử lý %s: %v", filepath.Base(f), err))
			emitter.Emit("process:row", processing.OrderRow{
				FileName:   filepath.Base(f),
				Status:     processing.StatusFailed,
				StatusKind: processing.StatusKindFailed,
			})
			current++
			continue
		}
		for _, row := range rows {
			for _, line := range row.SkuLog {
				emitter.Emit("process:log", line)
			}
			emitter.Emit("process:row", row)
			current++
		}
	}
	_ = a.cfg.SetSTT(current)
}

func (a *App) processOne(f string, stt int) (rows []processing.OrderRow, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return a.processor.Process(context.Background(), f, stt)
}

// ZaloJob là 1 lần gửi cần thực hiện: nội dung tin nhắn ĐÃ được frontend
// build sẵn bằng buildZaloMessageForPO (y hệt nội dung modal xem trước
// đã hiển thị cho người dùng) — Go không build lại text, chỉ resolve
// liên hệ (theo System) rồi gửi.
type ZaloJob struct {
	PO      string `json:"po"`
	System  string `json:"system"`
	Message string `json:"message"`
}

// SendZaloMessages gửi tuần tự từng job trong 1 goroutine nền, phát sự
// kiện zalo:log/zalo:sent/zalo:done — cùng pattern ProcessFiles/runBatch.
// Từ chối nếu đang có 1 lượt gửi khác chạy (atomic.Bool, giống
// a.processing) — không cho 2 batch gửi chồng lên nhau trên cùng 1
// trình duyệt.
func (a *App) SendZaloMessages(jobs []ZaloJob) {
	if !a.sending.CompareAndSwap(false, true) {
		a.emitter.Emit("zalo:log", "⚠️ Đã có một lượt gửi Zalo đang chạy, vui lòng đợi hoàn tất.")
		return
	}
	go a.runZaloBatch(a.emitter, jobs)
}

func (a *App) runZaloBatch(emitter Emitter, jobs []ZaloJob) {
	defer func() {
		if r := recover(); r != nil {
			emitter.Emit("zalo:log", fmt.Sprintf("❌ Lỗi không mong muốn: %v", r))
		}
		a.sending.Store(false)
		emitter.Emit("zalo:done", nil)
	}()

	ctx := context.Background()
	emitter.Emit("zalo:log", "🔐 Đang kiểm tra đăng nhập Zalo (quét QR trên cửa sổ trình duyệt nếu được yêu cầu)...")
	if err := a.zaloSender.EnsureLoggedIn(ctx); err != nil {
		emitter.Emit("zalo:log", fmt.Sprintf("❌ Không đăng nhập được Zalo: %v", err))
		return
	}

	settings, err := a.appSettingsStore.Load(resolveRepoFile("settings.ini"))
	if err != nil {
		emitter.Emit("zalo:log", fmt.Sprintf("❌ Không đọc được cấu hình liên hệ Zalo: %v", err))
		return
	}

	for _, job := range jobs {
		contact, err := zalosend.ResolveContact(job.System, settings.Zalo)
		if err != nil {
			emitter.Emit("zalo:log", fmt.Sprintf("❌ %s: chưa cấu hình liên hệ Zalo cho %s (sửa ở Cài đặt > Zalo)", job.PO, job.System))
			emitter.Emit("zalo:sent", map[string]any{"po": job.PO, "ok": false})
			continue
		}
		emitter.Emit("zalo:log", fmt.Sprintf("📤 Đang gửi %s → %s...", job.PO, contact))
		if err := a.zaloSender.SendMessage(ctx, contact, job.Message); err != nil {
			emitter.Emit("zalo:log", fmt.Sprintf("❌ Gửi %s thất bại: %v", job.PO, err))
			emitter.Emit("zalo:sent", map[string]any{"po": job.PO, "ok": false})
			continue
		}
		emitter.Emit("zalo:log", fmt.Sprintf("✅ Đã gửi %s", job.PO))
		emitter.Emit("zalo:sent", map[string]any{"po": job.PO, "ok": true})
	}
}

// shutdown đóng trình duyệt Zalo (nếu đã mở) khi app thoát — tránh để
// lại 1 tiến trình Chrome mồ côi chạy nền sau khi đóng cửa sổ chính.
func (a *App) shutdown(ctx context.Context) {
	if a.zaloSender != nil {
		_ = a.zaloSender.Close()
	}
}
