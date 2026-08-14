package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"order-processor/internal/config"
	"order-processor/internal/fileset"
	"order-processor/internal/processing"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
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
	ctx        context.Context
	cfg        *config.Store
	processor  processing.Processor
	emitter    Emitter
	orderDir   string
	processing atomic.Bool
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

// NewApp creates a new App application struct
func NewApp() (*App, error) {
	store, err := productdata.Load(resolveRepoFile("data.xlsx"))
	if err != nil {
		return nil, fmt.Errorf("app: load data.xlsx: %w", err)
	}

	return &App{
		cfg: config.NewStore(configFileName),
		processor: &processing.RealProcessor{
			Store:     store,
			Pricing:   pricing.NewHTTPSource(resolveRepoFile("settings.ini")),
			ExcelPath: "dondathang_test.xlsx",
		},
		orderDir: orderFolderName,
	}, nil
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
}

// GetSTT trả về số thứ tự đơn hàng bắt đầu hiện tại.
func (a *App) GetSTT() (int, error) {
	return a.cfg.GetSTT()
}

// SetSTT ghi lại số thứ tự đơn hàng bắt đầu.
func (a *App) SetSTT(v int) error {
	return a.cfg.SetSTT(v)
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
