package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/appsettings"
	"order-processor/internal/misa"
	"order-processor/internal/misapush"
)

type fakePusher struct {
	mu       sync.Mutex
	requests []misapush.Request
	rowsSeen [][]string // số đơn hàng (cột B) đọc được trong file mỗi lần đẩy
	failOn   string     // Database nào thì trả lỗi
	result   *misa.ImportResult
}

func (f *fakePusher) Push(_ context.Context, req misapush.Request) (*misa.ImportResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	f.rowsSeen = append(f.rowsSeen, readPOColumn(req.FilePath))
	if req.Database == f.failOn {
		return nil, errors.New("giả lập lỗi")
	}
	if f.result != nil {
		return f.result, nil
	}
	return &misa.ImportResult{Committed: true, Valid: 1}, nil
}

func readPOColumn(path string) []string {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	rows, err := f.GetRows(misapush.SheetName)
	if err != nil {
		return nil
	}
	var out []string
	for r := misapush.FirstDataRow; r <= len(rows); r++ {
		v, _ := f.GetCellValue(misapush.SheetName, "B"+strconv.Itoa(r))
		out = append(out, v)
	}
	return out
}

// seedPushWorkbook dựng dondathang.xlsx với n đơn, dòng r mang số đơn "PO-r".
func seedPushWorkbook(t *testing.T, path string, n int) {
	t.Helper()
	f := excelize.NewFile()
	idx, err := f.NewSheet(misapush.SheetName)
	if err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	f.SetActiveSheet(idx)
	f.DeleteSheet("Sheet1")
	if err := f.SetCellValue(misapush.SheetName, "A8", "Ngày đơn hàng (*)"); err != nil {
		t.Fatalf("SetCellValue A8: %v", err)
	}
	for i := 0; i < n; i++ {
		r := misapush.FirstDataRow + i
		if err := f.SetCellValue(misapush.SheetName, "B"+strconv.Itoa(r), "PO-"+strconv.Itoa(r)); err != nil {
			t.Fatalf("SetCellValue B%d: %v", r, err)
		}
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func newTestAppForPush(t *testing.T, pusher misapush.Pusher, misaCfg map[string]string) (*App, *fakeEmitter) {
	t.Helper()
	dir := t.TempDir()
	store := appsettings.NewStore(filepath.Join(dir, "settings.bhconfig"))
	if err := store.Save(appsettings.Settings{Misa: misaCfg, MisaRouting: misapush.SeedRouting()}); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	xlsx := filepath.Join(dir, "dondathang.xlsx")
	seedPushWorkbook(t, xlsx, 6) // r9..r14

	emitter := &fakeEmitter{}
	app := &App{
		appSettingsStore: store,
		excelPath:        xlsx,
		misaPusher:       pusher,
		misaSessionPath:  filepath.Join(dir, "misa-session.json"),
		emitter:          emitter,
	}
	return app, emitter
}

func defaultMisaCfg() map[string]string {
	return map[string]string{"db_ha_thanh": "HÀ THÀNH", "db_htla": "Long An", "sid_url": "https://script/x"}
}

func pushedEvents(t *testing.T, events []emittedEvent) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, e := range events {
		if e.name == "misa:pushed" {
			data, ok := e.data[0].(map[string]any)
			if !ok {
				t.Fatalf("misa:pushed data không phải map[string]any: %#v", e.data)
			}
			out = append(out, data)
		}
	}
	return out
}

func TestRunMisaPush_MỗiNhánhMộtLầnĐẩyĐúngDòng(t *testing.T) {
	pusher := &fakePusher{}
	app, _ := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(app.emitter, []MisaPushJob{
		{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9, 10}},
		{PO: "B", RouteKey: "Emart", Branch: misapush.BranchHaThanh, ExcelRows: []int{11}},
		{PO: "C", RouteKey: "Satra", Branch: misapush.BranchHTLA, ExcelRows: []int{13, 12}},
	})

	if len(pusher.requests) != 2 {
		t.Fatalf("số lần Push = %d, want 2 (một nhánh một lần, không phải một đơn một lần)", len(pusher.requests))
	}

	byDB := map[string][]string{}
	for i, req := range pusher.requests {
		byDB[req.Database] = pusher.rowsSeen[i]
	}
	if want := []string{"PO-9", "PO-10", "PO-12", "PO-13"}; !reflect.DeepEqual(byDB["Long An"], want) {
		t.Errorf("file nhánh HTLA chứa %v, want %v", byDB["Long An"], want)
	}
	if want := []string{"PO-11"}; !reflect.DeepEqual(byDB["HÀ THÀNH"], want) {
		t.Errorf("file nhánh Hà Thành chứa %v, want %v", byDB["HÀ THÀNH"], want)
	}
}

func TestRunMisaPush_KhôngGọiChoNhánhRỗng(t *testing.T) {
	pusher := &fakePusher{}
	app, _ := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(app.emitter, []MisaPushJob{
		{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
	})

	if len(pusher.requests) != 1 {
		t.Fatalf("số lần Push = %d, want 1", len(pusher.requests))
	}
	if pusher.requests[0].Database != "Long An" {
		t.Errorf("Database = %q, want %q", pusher.requests[0].Database, "Long An")
	}
}

func TestRunMisaPush_LoạiTrùngVàSắpTăngDần(t *testing.T) {
	pusher := &fakePusher{}
	app, _ := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(app.emitter, []MisaPushJob{
		{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{11, 9}},
		{PO: "B", RouteKey: "Satra", Branch: misapush.BranchHTLA, ExcelRows: []int{9, 10}},
	})

	if want := []string{"PO-9", "PO-10", "PO-11"}; !reflect.DeepEqual(pusher.rowsSeen[0], want) {
		t.Errorf("dòng đã đẩy = %v, want %v", pusher.rowsSeen[0], want)
	}
}

func TestRunMisaPush_ThiếuTênBộDữLiệuThìBỏNhánhĐóThôi(t *testing.T) {
	pusher := &fakePusher{}
	app, emitter := newTestAppForPush(t, pusher, map[string]string{"db_ha_thanh": "HÀ THÀNH"})

	app.runMisaPush(emitter, []MisaPushJob{
		{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
		{PO: "B", RouteKey: "Emart", Branch: misapush.BranchHaThanh, ExcelRows: []int{10}},
	})

	if len(pusher.requests) != 1 || pusher.requests[0].Database != "HÀ THÀNH" {
		t.Fatalf("requests = %#v, want đúng một lần cho HÀ THÀNH", pusher.requests)
	}
	events := pushedEvents(t, emitter.events)
	if len(events) != 2 {
		t.Fatalf("số misa:pushed = %d, want 2 (cả nhánh hỏng cũng phải báo)", len(events))
	}
	for _, e := range events {
		if e["branch"] == misapush.BranchHTLA && e["ok"] != false {
			t.Errorf("nhánh HTLA ok = %v, want false", e["ok"])
		}
	}
}

func TestRunMisaPush_NhánhLỗiKhôngChặnNhánhCònLại(t *testing.T) {
	pusher := &fakePusher{failOn: "Long An"}
	app, emitter := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(emitter, []MisaPushJob{
		{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
		{PO: "B", RouteKey: "Emart", Branch: misapush.BranchHaThanh, ExcelRows: []int{10}},
	})

	if len(pusher.requests) != 2 {
		t.Fatalf("số lần Push = %d, want 2 — nhánh lỗi không được chặn nhánh kia", len(pusher.requests))
	}
	byBranch := map[string]bool{}
	for _, e := range pushedEvents(t, emitter.events) {
		byBranch[e["branch"].(string)] = e["ok"].(bool)
	}
	if byBranch[misapush.BranchHTLA] != false || byBranch[misapush.BranchHaThanh] != true {
		t.Errorf("kết quả từng nhánh = %#v, want htla=false ha_thanh=true", byBranch)
	}
}

func TestRunMisaPush_XoáFileTạmKểCảKhiLỗi(t *testing.T) {
	pusher := &fakePusher{failOn: "Long An"}
	app, _ := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(app.emitter, []MisaPushJob{
		{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
	})

	if len(pusher.requests) != 1 {
		t.Fatalf("số lần Push = %d, want 1", len(pusher.requests))
	}
	if _, err := os.Stat(pusher.requests[0].FilePath); !os.IsNotExist(err) {
		t.Errorf("file tạm %s còn sót lại sau khi Push lỗi", pusher.requests[0].FilePath)
	}
}

func TestPushMisa_TừChốiKhiĐangXửLýLô(t *testing.T) {
	pusher := &fakePusher{}
	app, emitter := newTestAppForPush(t, pusher, defaultMisaCfg())
	app.processing.Store(true)

	app.PushMisa([]MisaPushJob{{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9}}})

	if len(pusher.requests) != 0 {
		t.Error("đã đẩy dù đang có lô xử lý chạy — workbook lúc đó đang bị ghi dở")
	}
	if len(emitter.events) == 0 {
		t.Error("không báo gì cho người dùng khi từ chối")
	}
}

func TestPushMisa_TừChốiLờiGọiThứHaiKhiĐangĐẩy(t *testing.T) {
	pusher := &fakePusher{}
	app, emitter := newTestAppForPush(t, pusher, defaultMisaCfg())
	app.pushing.Store(true)

	app.PushMisa([]MisaPushJob{{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9}}})

	if len(pusher.requests) != 0 {
		t.Error("đã đẩy dù đang có lượt đẩy khác chạy")
	}
	if len(emitter.events) == 0 {
		t.Error("không báo gì cho người dùng khi từ chối")
	}
}

func TestRunMisaPush_TruyềnPhiênVàSidURLXuốngPusher(t *testing.T) {
	pusher := &fakePusher{}
	app, _ := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(app.emitter, []MisaPushJob{
		{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
	})

	req := pusher.requests[0]
	if req.SessionPath != app.misaSessionPath {
		t.Errorf("SessionPath = %q, want %q", req.SessionPath, app.misaSessionPath)
	}
	if req.SidURL != "https://script/x" {
		t.Errorf("SidURL = %q, want %q", req.SidURL, "https://script/x")
	}
}
