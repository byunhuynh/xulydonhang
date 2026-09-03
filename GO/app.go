package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"order-processor/internal/applock"
	"order-processor/internal/appsettings"
	"order-processor/internal/driveupload"
	"order-processor/internal/fileset"
	"order-processor/internal/misapush"
	"order-processor/internal/processing"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
	"order-processor/internal/processing/warehouse"
	"order-processor/internal/tmdt"
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

// zaloJobTimeout là hạn giờ cho MỘT job gửi Zalo (tìm hội thoại + dán +
// chờ Zalo xác nhận đã gửi). Không có nó, 1 lời gọi chromedp treo vì DOM
// Zalo đổi selector sẽ khoá cờ a.sending mãi mãi và nút gửi chết cho tới
// khi khởi động lại app. Mỗi job có deadline RIÊNG — job chậm không ăn
// mất thời gian của job sau.
const zaloJobTimeout = 90 * time.Second

// isZaloTimeoutErr báo lỗi gửi Zalo có phải do HẾT GIỜ hay không - dùng
// cả errors.Is LẪN kiểm tra chuỗi: thực tế quan sát log cho thấy lỗi
// chromedp/cdproto trả về khi context hết hạn không phải lúc nào cũng bọc
// đúng chuỗi %w tới context.DeadlineExceeded (một số lớp bên dưới stringify
// bằng %v làm đứt chain errors.Is), dù chuỗi hiển thị vẫn luôn là "context
// deadline exceeded" - kiểm tra chuỗi làm lưới an toàn thứ 2.
func isZaloTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return strings.Contains(err.Error(), "context deadline exceeded")
}

// App struct
type App struct {
	ctx                 context.Context
	appSettingsStore    *appsettings.Store
	processor           processing.Processor
	emitter             Emitter
	orderDir            string
	excelPath           string
	resolvedRows        map[int]bool
	resolvedMu          sync.Mutex
	workbookAdmissionMu sync.Mutex
	excelMu             sync.Mutex
	processing          atomic.Bool
	zaloSender          zalosend.ZaloSender
	sending             atomic.Bool
	zaloLoginMu         sync.Mutex
	zaloLoginCancel     context.CancelFunc
	initMu              sync.Mutex
	dataLoader          func() (processing.Processor, error)
	updateJITPeriodFn   func(string, []int, string, string, string) error
	// tmdtResolve nhận phản hồi của modal sửa mã thiếu. Đệm 1 để
	// ResolveTMDTMissing/CancelTMDTMissing không bị chặn nếu nhánh TMĐT
	// vừa hết giờ chờ đúng lúc người dùng bấm.
	tmdtResolve chan tmdtResolution
	// tmdtWaiting cho biết đang có nhánh TMĐT chờ phản hồi — dùng để từ
	// chối lời gọi Resolve/Cancel lạc (người dùng bấm khi không có modal).
	tmdtWaiting atomic.Bool
	// misaPusher thực hiện một lần đẩy cho một nhánh. Thay được trong
	// test để không phải chạm mạng — cùng khuôn với zaloSender.
	misaPusher misapush.Pusher
	// pushing khoá lượt đẩy MISA, đúng vai trò a.sending làm cho Zalo:
	// hai lượt đẩy chồng nhau sẽ đọc cùng một workbook trong lúc file
	// tạm của nhau đang được cắt.
	pushing atomic.Bool
	// misaSessionPath là file phiên đăng nhập MISA, nằm cạnh
	// settings.bhconfig. Nó thay được mật khẩu trong 24h nên đã được
	// .gitignore loại ra.
	misaSessionPath string
}

// exeDir trả về thư mục chứa chính file .exe đang chạy.
//
// Đây là mọi neo cuối cùng khi không dò ra settings.ini ở đâu cả — tức
// trường hợp bản triển khai thật, nơi exe nằm cạnh settings.bhconfig,
// misa-session.json, dondathang.xlsx và thư mục "đơn hàng" chứ không có
// settings.ini đời cũ.
//
// Trước đây neo này là "." (thư mục làm việc hiện hành). Bấm đúp vào exe
// thì CWD trùng thư mục chứa exe nên chạy đúng, nhưng chạy qua shortcut có
// "Start in" khác, từ cửa sổ lệnh ở thư mục khác, hay từ một trình khởi
// chạy đặt CWD về System32 thì app đọc cấu hình RỖNG mà không báo gì —
// triệu chứng là "Không tải được dữ liệu" hoặc Cài đặt trắng trơn. Neo
// theo exe làm cả thư mục chép đi đâu cũng chạy, khởi động kiểu gì cũng đúng.
//
// Lấy không được thì lùi về "." như cũ, vì còn hơn là không có gì.
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
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
	// Không thấy ở đâu: trả đường dẫn cạnh exe thay vì tên trần. Tên
	// trần là đường dẫn tương đối, nên mọi thứ đi theo CWD — xem exeDir.
	return filepath.Join(exeDir(), filename)
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

	// Vật chất hoá bảng định tuyến mặc định xuống settings.bhconfig ngay
	// lần chạy đầu, chỉ điền khoá còn thiếu. Xem misapush.ApplySeed cho
	// lý do đầy đủ: nếu bảng gieo chỉ sống trong code như giá trị dự
	// phòng, một lần sửa hằng số ở bản sau sẽ lặng lẽ đổi nhánh của mọi
	// mục người dùng chưa từng chạm vào. Lỗi ghi đĩa KHÔNG chặn khởi
	// động — app vẫn chạy được đầy đủ, chỉ là lần sau gieo lại.
	// Mã kho từng nhánh vendor được gieo xuống đĩa vì đúng lý do trên:
	// nếu mã mặc định chỉ sống trong code, một lần sửa hằng số ở bản sau
	// sẽ lặng lẽ đổi kho của những nhánh người dùng chưa từng chạm.
	if misapush.ApplySeed(settings.MisaRouting) || warehouse.ApplySeed(settings.Warehouse) {
		_ = appSettingsStore.Save(settings)
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

	app := &App{
		appSettingsStore: appSettingsStore,
		orderDir:         orderDir,
		excelPath:        excelPath,
		zaloSender: &zalosend.ChromedpSender{
			ProfileDir: filepath.Join(resolveRepoDir("settings.ini"), "zalo_profile"),
		},
		tmdtResolve:     make(chan tmdtResolution, 1),
		misaPusher:      &misapush.HTTPPusher{},
		misaSessionPath: filepath.Join(resolveRepoDir("settings.ini"), "misa-session.json"),
	}

	app.dataLoader = func() (processing.Processor, error) {
		store, err := productdata.LoadFromSheets(settings.Gid, productdata.NewHTTPClient())
		if err != nil {
			return nil, fmt.Errorf("app: load customer/product data from Google Sheets: %w", err)
		}
		processor := &processing.RealProcessor{
			Store:       store,
			Pricing:     pricing.NewHTTPSource(settings.Gid),
			ExcelPath:   excelPath,
			Warehouses:  warehouse.NewResolver(settings.Warehouse),
			DriveClient: driveupload.NewHTTPClient(),
		}
		processor.LogFunc = func(msg string) {
			if app.emitter != nil {
				app.emitter.Emit("process:log", msg)
			}
		}
		return processor, nil
	}

	return app, nil
}

// InitializeApp tải dữ liệu mạng sau khi cửa sổ Wails đã hiển thị. Lỗi
// được trả về frontend để hiện ngay trên màn hình; gọi lại hàm này là một
// lần thử mới, nên người dùng không cần đóng/mở app khi mạng vừa hồi phục.
func (a *App) InitializeApp() error {
	a.initMu.Lock()
	defer a.initMu.Unlock()
	if a.processor != nil {
		return nil
	}
	processor, err := a.dataLoader()
	if err != nil {
		return err
	}
	a.processor = processor
	return nil
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

// onSecondInstanceLaunch chay TRONG ban dang mo, khi nguoi dung mo app them
// mot lan nua. Wails da chan ban thu hai lai (xem singleInstanceID trong
// main.go) va chi chuyen tin sang day, viec con lai la keo cua so cu ra
// truoc mat nguoi dung - neu khong ho se thay mot cu double-click khong lam
// gi ca va tuong app hong.
//
// runtime.WindowShow lo ca hai trang thai an: cua so dang thu nho thi duoc
// khoi phuc, cua so dang bi che thi duoc dua len truoc va nhan focus.
//
// ctx con nil nghia la ban dau chua chay xong startup - double-click hai lan
// that nhanh la du. Goi runtime.WindowShow(nil) se panic, keo sap dung ban
// dang can duoc danh thuc, nen truong hop do bo qua: cua so sap tu hien ra
// bang duong khoi dong binh thuong roi.
func (a *App) onSecondInstanceLaunch(options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	runtime.WindowShow(a.ctx)
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

// GetAppSettings trả về toàn bộ cấu hình hiện tại (gid/zalo/reminder)
// để popup cài đặt hiển thị.
func (a *App) GetAppSettings() (appsettings.Settings, error) {
	return a.appSettingsStore.Load(resolveRepoFile("settings.ini"))
}

// SaveAppSettings ghi cấu hình mới và áp dụng ngay trong phiên làm việc
// hiện tại — không cần khởi động lại app. Zalo/Reminder áp dụng tự nhiên
// (đọc lại từ file mỗi lần dùng - xem SendZaloMessages). Gid là trường
// hợp duy nhất cần xử lý riêng: Store/Pricing của RealProcessor được tạo
// sẵn một lần lúc NewApp chạy, nên khi map gid đổi, phải gọi
// reloadDataSources để dựng lại chúng - chỉ khi gid THỰC SỰ đổi (network
// fetch không cần thiết cho các lần lưu chỉ đổi Zalo/Reminder).
//
// Ghi đĩa LUÔN thành công trước, độc lập với reload: nếu reloadDataSources
// lỗi (mất mạng, hoặc đang có batch xử lý), cấu hình mới vẫn đã được lưu
// - chỉ riêng việc áp dụng ngay bị hoãn, báo qua log panel thay vì chặn
// người dùng lưu lại.
func (a *App) SaveAppSettings(settings appsettings.Settings) error {
	prev, err := a.appSettingsStore.Load(resolveRepoFile("settings.ini"))
	if err != nil {
		return err
	}
	if err := a.appSettingsStore.Save(settings); err != nil {
		return err
	}
	if !reflect.DeepEqual(prev.Gid, settings.Gid) {
		if err := a.reloadDataSources(settings.Gid); err != nil && a.emitter != nil {
			a.emitter.Emit("process:log", fmt.Sprintf("⚠️ Đã lưu cấu hình GID nhưng chưa áp dụng ngay được (%v) — lưu lại hoặc khởi động lại app để áp dụng.", err))
		}
	}
	if !reflect.DeepEqual(prev.Warehouse, settings.Warehouse) {
		if err := a.applyWarehouseSettings(settings.Warehouse); err != nil && a.emitter != nil {
			a.emitter.Emit("process:log", fmt.Sprintf("⚠️ Đã lưu mã kho nhưng chưa áp dụng ngay được (%v) — lưu lại hoặc khởi động lại app để áp dụng.", err))
		}
	}
	return nil
}

// reloadDataSources dựng lại Store (customer/product) và Pricing
// (price/promotion) của RealProcessor từ gid map mới, thay cho việc chờ
// app khởi động lại. Dùng CHÍNH cờ a.processing mà ProcessFiles/runBatch
// đã dùng để loại trừ lẫn nhau: RealProcessor.Store/.Pricing được
// runBatch's goroutine đọc không qua khóa riêng trong lúc Process() chạy,
// nên việc gán field mới ở đây chỉ an toàn khi CHẮC CHẮN không có batch
// nào đang chạy - CompareAndSwap vừa ngăn hàm này chạy giữa lúc có batch,
// vừa ngăn ProcessFiles khởi động batch mới giữa lúc reload (nó dùng
// CompareAndSwap y hệt và sẽ thất bại cho tới khi defer bên dưới trả cờ
// về false). Không làm gì (trả nil) nếu a.processor không phải
// *RealProcessor - trường hợp test dùng processor giả, không có gì để
// nạp lại.
func (a *App) reloadDataSources(gid map[string]string) error {
	rp, ok := a.processor.(*processing.RealProcessor)
	if !ok {
		return nil
	}
	if !a.processing.CompareAndSwap(false, true) {
		return fmt.Errorf("đang xử lý đơn hàng, vui lòng thử lưu lại sau khi xử lý xong")
	}
	defer a.processing.Store(false)

	store, err := productdata.LoadFromSheets(gid, productdata.NewHTTPClient())
	if err != nil {
		return err
	}
	rp.Store = store
	rp.Pricing = pricing.NewHTTPSource(gid)
	return nil
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
	if err := a.lockWorkbookMutation("đang xử lý đơn hàng, vui lòng đợi hoàn tất trước khi áp dụng giá"); err != nil {
		return err
	}
	defer a.excelMu.Unlock()

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

// UpdateJITPeriod applies one delivery period to every Excel row generated
// from a selected JIT PDF. It shares the workbook lock with price updates and
// is disabled while a processing batch is writing the same file.
func (a *App) UpdateJITPeriod(rows []int, orderDate, warehouse, period string) error {
	if err := a.lockWorkbookMutation("đang xử lý đơn hàng, vui lòng đợi hoàn tất trước khi đổi buổi JIT"); err != nil {
		return err
	}
	defer a.excelMu.Unlock()
	if a.updateJITPeriodFn != nil {
		return a.updateJITPeriodFn(a.excelPath, rows, orderDate, warehouse, period)
	}
	return excelwriter.UpdateJITPeriod(a.excelPath, rows, orderDate, warehouse, period)
}

// lockWorkbookMutation admits a workbook mutation atomically with batch
// reservation. The global lock order is workbookAdmissionMu -> excelMu.
// A mutation releases the admission gate as soon as it owns excelMu; a batch
// marks processing before it waits for excelMu. Batch completion releases
// excelMu before touching the admission gate again, so no inverse order exists.
func (a *App) lockWorkbookMutation(processingMessage string) error {
	a.workbookAdmissionMu.Lock()
	if a.processing.Load() {
		a.workbookAdmissionMu.Unlock()
		return fmt.Errorf("%s", processingMessage)
	}
	a.excelMu.Lock()
	a.workbookAdmissionMu.Unlock()
	return nil
}

// reserveBatch từ chối cả khi đã có batch khác đang chạy LẪN khi đang có
// một lượt đẩy MISA đang chạy: PushMisa/pushOneBranch đọc a.excelPath và
// tách workbook ra file tạm cho từng nhánh - một batch mới ClearOrderRows
// rồi ghi đè đúng lúc đó sẽ khiến SplitWorkbook đọc phải nội dung đã đổi,
// và đơn của lô mới bị đẩy nhầm vào sổ kế toán của nhánh đang tách dở.
func (a *App) reserveBatch() bool {
	a.workbookAdmissionMu.Lock()
	defer a.workbookAdmissionMu.Unlock()
	if a.pushing.Load() {
		return false
	}
	return a.processing.CompareAndSwap(false, true)
}

func (a *App) releaseBatchReservation() {
	a.workbookAdmissionMu.Lock()
	a.processing.Store(false)
	a.workbookAdmissionMu.Unlock()
}

// ScanOrderFolder quét trực tiếp thư mục "đơn hàng" (tự tạo nếu thiếu)
// và trả về danh sách file hợp lệ — KHÔNG đi vào các thư mục con (vd
// "MM-YYYY", "Code", "mẫu đơn hàng"): những thư mục đó là nơi lưu file
// đã xử lý/backup, không phải việc cần làm, ListFiles vốn đã bỏ qua thư
// mục con nên chỉ cần trỏ thẳng vào a.orderDir.
func (a *App) ScanOrderFolder() ([]string, error) {
	if err := os.MkdirAll(a.orderDir, 0o755); err != nil {
		return nil, err
	}
	return fileset.ListFiles(a.orderDir)
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
// process:log / process:row / process:done về frontend. ranges gắn khoảng
// ngày cho từng file TMĐT (khoá là đúng đường dẫn file); batch không có
// file TMĐT thì truyền map rỗng hoặc nil.
func (a *App) ProcessFiles(files []string, ranges map[string]TMDTDateRange) {
	if !a.reserveBatch() {
		a.emitter.Emit("process:log", "⚠️ Đã có một batch đang xử lý hoặc đang đẩy lên MISA, vui lòng đợi hoàn tất rồi thử lại.")
		return
	}
	go a.runReservedBatch(a.emitter, files, ranges)
}

func (a *App) runBatch(emitter Emitter, files []string, ranges map[string]TMDTDateRange) {
	if !a.reserveBatch() {
		emitter.Emit("process:log", "⚠️ Đã có một batch đang xử lý hoặc đang đẩy lên MISA, vui lòng đợi hoàn tất rồi thử lại.")
		return
	}
	a.runReservedBatch(emitter, files, ranges)
}

func (a *App) runReservedBatch(emitter Emitter, files []string, ranges map[string]TMDTDateRange) {
	a.excelMu.Lock()
	defer func() {
		if r := recover(); r != nil {
			emitter.Emit("process:log", fmt.Sprintf("❌ Lỗi không mong muốn: %v", r))
		}
		a.excelMu.Unlock()
		a.releaseBatchReservation()
		emitter.Emit("process:done")
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

	emitter.Emit("process:progress", BatchProgress{Done: 0, Total: len(files)})
	for i, f := range files {
		emitter.Emit("process:log", fmt.Sprintf("Đang xử lý %s...", filepath.Base(f)))
		streamed := map[string]bool{}
		loggedSkuKeys := map[string]bool{}
		emitRow := func(row processing.OrderRow) {
			row = ensureResultIdentity(row, f)
			row = emitProcessRowOncePerSkuKey(emitter, row, loggedSkuKeys)
			if row.ResultKey != "" {
				streamed[row.ResultKey] = true
			}
		}
		// Nhánh rẽ TMĐT: cùng một lô có thể trộn file PDF vendor với
		// workbook TMĐT, nên rẽ theo NỘI DUNG file (tmdt.IsWorkbook đọc
		// hai sheet tra cứu) chứ không theo tên file hay lựa chọn của
		// người dùng.
		var rows []processing.OrderRow
		var err error
		if tmdt.IsWorkbook(f) {
			rows, err = a.processTMDTFile(emitter, f, ranges[f], emitRow)
		} else {
			rows, err = a.processOne(f, emitRow)
		}
		if err != nil {
			emitter.Emit("process:log", fmt.Sprintf("❌ Lỗi xử lý %s: %v", filepath.Base(f), err))
			emitProcessRow(emitter, ensureResultIdentity(processing.OrderRow{
				FileName:   filepath.Base(f),
				Status:     processing.StatusFailed,
				StatusKind: processing.StatusKindFailed,
			}, f))
			emitter.Emit("process:progress", BatchProgress{Done: i + 1, Total: len(files)})
			continue
		}
		for _, row := range rows {
			row = ensureResultIdentity(row, f)
			if row.ResultKey != "" && streamed[row.ResultKey] {
				continue
			}
			emitProcessRowOncePerSkuKey(emitter, row, loggedSkuKeys)
		}
		// Một file lỗi vẫn là một file đã xong phần việc của nó: nếu bỏ
		// qua ở nhánh trên thì thanh tiến trình sẽ đứng im ở con số cũ
		// cho tới hết lô và người dùng tưởng app treo.
		emitter.Emit("process:progress", BatchProgress{Done: i + 1, Total: len(files)})
	}
}

// BatchProgress là tiến trình của một lô, phát qua "process:progress" khi
// lô bắt đầu và sau mỗi file. Frontend không tự suy ra được con số này:
// một file có thể phát dòng tạm trước khi thực sự xong (BigC gửi 23 dòng
// "đang xử lý" trước lần ghi Excel gộp), nên đếm theo dòng sẽ báo xong
// sớm hơn thực tế.
type BatchProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

func (a *App) processOne(f string, emit func(processing.OrderRow)) (rows []processing.OrderRow, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	if processor, ok := a.processor.(processing.StreamingProcessor); ok {
		return processor.ProcessStreaming(context.Background(), f, emit)
	}
	return a.processor.Process(context.Background(), f)
}

func emitProcessRow(emitter Emitter, row processing.OrderRow) processing.OrderRow {
	row = ensureResultKey(row)
	for _, line := range row.SkuLog {
		emitter.Emit("process:log", line)
	}
	emitter.Emit("process:row", row)
	return row
}

func emitProcessRowOncePerSkuKey(emitter Emitter, row processing.OrderRow, logged map[string]bool) processing.OrderRow {
	row = ensureResultKey(row)
	if len(row.SkuLog) > 0 {
		if logged[row.ResultKey] {
			row.SkuLog = nil
		} else {
			logged[row.ResultKey] = true
		}
	}
	return emitProcessRow(emitter, row)
}

func ensureResultIdentity(row processing.OrderRow, sourcePath string) processing.OrderRow {
	if row.SourceID == "" {
		row.SourceID = processing.SourceIDForPath(sourcePath)
	}
	return ensureResultKey(row)
}

func ensureResultKey(row processing.OrderRow) processing.OrderRow {
	if row.ResultKey == "" {
		sourceID := row.SourceID
		if sourceID == "" {
			sourceID = row.FileName
		}
		row.ResultKey = fmt.Sprintf("legacy:%s:%s:%s:%s", sourceID, row.Page, row.System, row.PO)
	}
	return row
}

// ZaloJob là 1 lần gửi cần thực hiện: nội dung tin nhắn ĐÃ được frontend
// build sẵn bằng buildZaloMessageForPO (y hệt nội dung modal xem trước
// đã hiển thị cho người dùng) — Go không build lại text, chỉ resolve
// liên hệ (theo System + CustomerCode, xem zalosend.ResolveContact) rồi
// gửi. CustomerCode là OrderRow.MaKhachHang của dòng đầu tiên thuộc PO
// này — 2 ký tự đầu của nó là miền (MN/MB), cần để ghép đúng key Cài đặt
// > Zalo (vd "MNBIGC") vì bản thân System ("BigC") không phân biệt miền.
type ZaloJob struct {
	PO           string `json:"po"`
	System       string `json:"system"`
	CustomerCode string `json:"customerCode"`
	Message      string `json:"message"`
	// DisplayLabel là nhãn CHỈ để hiện trong log ("Đang gửi {nhãn} →
	// ..."), tách khỏi PO. Cho mọi vendor khác PO đã là 1 po thật, dễ
	// đọc, nên frontend để trống DisplayLabel và Go rơi về dùng PO (xem
	// bên dưới). JIT thì khác: PO giờ mang sourceId (hash 64 ký tự, khoá
	// gộp nhóm THẬT của 1 file PDF - xem groupKeyFor, lib/zaloGrouping.ts
	// phía frontend) để deselectPO khớp đúng dòng đã tick chọn sau khi
	// gửi xong - PO không còn dùng để hiện log được nữa, nên frontend gửi
	// kèm tên file PDF qua trường này.
	DisplayLabel string `json:"displayLabel"`
}

// SendZaloMessages gửi tuần tự từng job trong 1 goroutine nền, phát sự
// kiện zalo:log/zalo:sent/zalo:done — cùng pattern ProcessFiles/runBatch.
// Từ chối nếu đang có 1 lượt gửi khác chạy (atomic.Bool, giống
// a.processing) — không cho 2 batch gửi chồng lên nhau trên cùng 1
// trình duyệt.
func (a *App) SendZaloMessages(jobs []ZaloJob) {
	// Không có job nào thì không làm gì cả — nếu vẫn chạy tiếp, batch rỗng
	// sẽ bật cờ sending, mở trình duyệt và có thể đứng chờ quét QR tới 120
	// giây để rồi không gửi được tin nào. UI hiện chỉ hiện nút gửi khi đã
	// chọn ít nhất 1 PO, nhưng đây là method frontend gọi trực tiếp được.
	if len(jobs) == 0 {
		return
	}
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

	// loginCtx huỷ được riêng cho bước đăng nhập (KHÔNG dùng cho phần gửi
	// tin phía dưới, vốn tự đặt deadline riêng cho từng job) — cho phép
	// người dùng bấm "Đóng" trên popup QR để dừng hẳn việc chờ đăng nhập
	// thay vì phải đợi hết 120s. zaloLoginCancel được CancelZaloLogin gọi
	// từ 1 goroutine khác (Wails tự điều phối lời gọi bound method), nên
	// phải lưu dưới lock; xoá lại (defer) khi bước đăng nhập đã xong để
	// CancelZaloLogin gọi muộn không huỷ nhầm context của lượt sau.
	loginCtx, cancelLogin := context.WithCancel(context.Background())
	a.zaloLoginMu.Lock()
	a.zaloLoginCancel = cancelLogin
	a.zaloLoginMu.Unlock()
	defer func() {
		cancelLogin()
		a.zaloLoginMu.Lock()
		a.zaloLoginCancel = nil
		a.zaloLoginMu.Unlock()
	}()

	ctx := context.Background()
	emitter.Emit("zalo:log", "🔐 Đang kiểm tra đăng nhập Zalo...")
	onQR := func(svgMarkup string) { emitter.Emit("zalo:qr", svgMarkup) }
	if err := a.zaloSender.EnsureLoggedIn(loginCtx, onQR); err != nil {
		if loginCtx.Err() != nil {
			emitter.Emit("zalo:log", "🚫 Đã huỷ đăng nhập Zalo.")
		} else {
			emitter.Emit("zalo:log", fmt.Sprintf("❌ Không đăng nhập được Zalo: %v", err))
		}
		return
	}

	settings, err := a.appSettingsStore.Load(resolveRepoFile("settings.ini"))
	if err != nil {
		emitter.Emit("zalo:log", fmt.Sprintf("❌ Không đọc được cấu hình liên hệ Zalo: %v", err))
		return
	}

	// Resolve TRƯỚC liên hệ của mọi job rồi SẮP XẾP ỔN ĐỊNH (stable sort)
	// theo tên liên hệ — gộp các đơn CÙNG 1 nhóm Zalo đứng cạnh nhau,
	// tránh trình duyệt phải tìm/mở lại hội thoại liên tục khi thứ tự
	// người dùng chọn ban đầu xen kẽ giữa nhiều nhóm (vd Satra, BigC,
	// Satra, BigC...). Sort ỔN ĐỊNH giữ nguyên thứ tự chọn ban đầu GIỮA
	// các đơn cùng 1 liên hệ, chỉ đổi thứ tự TƯƠNG ĐỐI giữa các nhóm
	// liên hệ khác nhau. Job resolve lỗi (contact rỗng) tự nhiên gộp về
	// đầu danh sách (chuỗi rỗng sort trước mọi tên liên hệ thật) — vô
	// hại, các job đó chỉ log lỗi rồi bỏ qua, không mở hội thoại nào.
	type resolvedZaloJob struct {
		job     ZaloJob
		contact string
		err     error
	}
	resolved := make([]resolvedZaloJob, len(jobs))
	for i, job := range jobs {
		contact, err := zalosend.ResolveContact(job.System, job.CustomerCode, settings.Zalo)
		resolved[i] = resolvedZaloJob{job: job, contact: contact, err: err}
	}
	sort.SliceStable(resolved, func(i, j int) bool {
		return resolved[i].contact < resolved[j].contact
	})

	for _, r := range resolved {
		// Nhãn hiện log: DisplayLabel nếu frontend có gửi kèm (JIT - PO
		// lúc này mang sourceId, không đọc được), rơi về PO cho mọi
		// trường hợp còn lại (đã là 1 po thật, dễ đọc sẵn).
		label := r.job.DisplayLabel
		if label == "" {
			label = r.job.PO
		}
		if r.err != nil {
			// err bọc ErrNoContact kèm đúng key đã ghép (vd "MNBIGC") ở
			// cuối chuỗi (xem ResolveContact) — cắt bỏ phần tiền tố lỗi kỹ
			// thuật, chỉ hiện key đó cho người dùng biết CHÍNH XÁC dòng cần
			// thêm trong Cài đặt > Zalo.
			key := strings.TrimPrefix(r.err.Error(), zalosend.ErrNoContact.Error()+": ")
			emitter.Emit("zalo:log", fmt.Sprintf("❌ %s: chưa cấu hình liên hệ Zalo cho %s (sửa ở Cài đặt > Zalo, thêm dòng khoá %q)", label, r.job.System, key))
			emitter.Emit("zalo:sent", map[string]any{"po": r.job.PO, "ok": false})
			continue
		}
		emitter.Emit("zalo:log", fmt.Sprintf("📤 Đang gửi %s → %s...", label, r.contact))
		// Deadline riêng cho từng job, giải phóng timer ngay khi job xong
		// (không defer tới cuối vòng lặp) — batch dài không tích luỹ timer.
		// Thử lại 1 lần NẾU lỗi là hết giờ (context.DeadlineExceeded): đây
		// là kiểu lỗi ngẫu nhiên do Chrome/Zalo Web đơ tạm thời ở 1 bước
		// nào đó (thực tế người dùng xác nhận lỗi này xảy ra ngẫu nhiên,
		// không cố định ở vị trí tin nhắn hay việc đổi liên hệ) - không thử
		// lại các lỗi khác (vd "không tìm thấy hội thoại") vì gửi lại
		// chắc chắn cũng lỗi y hệt, chỉ tốn thời gian.
		var sendErr error
		for attempt := 1; attempt <= 2; attempt++ {
			jobCtx, cancel := context.WithTimeout(ctx, zaloJobTimeout)
			sendErr = a.zaloSender.SendMessage(jobCtx, r.contact, r.job.Message)
			cancel()
			if sendErr == nil || !isZaloTimeoutErr(sendErr) || attempt == 2 {
				break
			}
			emitter.Emit("zalo:log", fmt.Sprintf("⏱️ %s: hết giờ, thử gửi lại lần 2...", label))
		}
		if sendErr != nil {
			emitter.Emit("zalo:log", fmt.Sprintf("❌ Gửi %s thất bại: %v", label, sendErr))
			emitter.Emit("zalo:sent", map[string]any{"po": r.job.PO, "ok": false})
			continue
		}
		emitter.Emit("zalo:log", fmt.Sprintf("✅ Đã gửi %s", label))
		emitter.Emit("zalo:sent", map[string]any{"po": r.job.PO, "ok": true})
	}
}

// CancelZaloLogin dừng hẳn việc chờ đăng nhập Zalo đang chạy (nếu có) —
// frontend gọi khi người dùng bấm "Đóng" trên popup QR vì không còn
// muốn đăng nhập nữa. Không làm gì nếu không có lượt đăng nhập nào đang
// chờ (đã xong, hoặc chưa từng bắt đầu) — an toàn gọi bất cứ lúc nào.
func (a *App) CancelZaloLogin() {
	a.zaloLoginMu.Lock()
	cancel := a.zaloLoginCancel
	a.zaloLoginMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// RefreshZaloQR bấm nút "Lấy mã mới" trên trang đăng nhập Zalo — frontend
// gọi khi người dùng bấm "Làm mới mã QR" trong popup QR (xem zalo:qr).
// An toàn gọi bất cứ lúc nào, kể cả trong lúc runZaloBatch vẫn đang chờ
// ở EnsureLoggedIn trên goroutine khác (không làm gì nếu chưa mở trình
// duyệt hoặc không có mã QR nào đang chờ).
func (a *App) RefreshZaloQR() error {
	return a.zaloSender.RefreshQR(context.Background())
}

// shutdown đóng trình duyệt Zalo (nếu đã mở) khi app thoát — tránh để
// lại 1 tiến trình Chrome mồ côi chạy nền sau khi đóng cửa sổ chính.
func (a *App) shutdown(ctx context.Context) {
	if a.zaloSender != nil {
		_ = a.zaloSender.Close()
	}
}
