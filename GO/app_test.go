package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/appsettings"
	"order-processor/internal/processing"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/zalosend"
)

type fakeEmitter struct {
	events []emittedEvent
}

type emittedEvent struct {
	name string
	data []interface{}
}

func (f *fakeEmitter) Emit(name string, data ...interface{}) {
	f.events = append(f.events, emittedEvent{name: name, data: data})
}

type stubProcessor struct {
	failOn string
}

func (s *stubProcessor) Process(ctx context.Context, filePath string) ([]processing.OrderRow, error) {
	if filePath == s.failOn {
		return nil, errors.New("stub failure")
	}
	return []processing.OrderRow{{FileName: filePath, PO: "PO1", Status: processing.StatusDone}}, nil
}

type streamingStubProcessor struct {
	emitted      []processing.OrderRow
	returned     []processing.OrderRow
	streamCalls  int
	processCalls int
}

type channelProcessor struct {
	entered chan struct{}
	release chan struct{}
}

func (p *channelProcessor) Process(context.Context, string) ([]processing.OrderRow, error) {
	close(p.entered)
	<-p.release
	return nil, nil
}

var _ processing.StreamingProcessor = (*streamingStubProcessor)(nil)

func (s *streamingStubProcessor) Process(ctx context.Context, filePath string) ([]processing.OrderRow, error) {
	s.processCalls++
	return s.returned, nil
}

func (s *streamingStubProcessor) ProcessStreaming(ctx context.Context, filePath string, emit func(processing.OrderRow)) ([]processing.OrderRow, error) {
	s.streamCalls++
	for _, row := range s.emitted {
		emit(row)
	}
	return s.returned, nil
}

func TestApp_InitializeApp_AllowsRetryAfterLoadFailure(t *testing.T) {
	attempts := 0
	wantProcessor := &stubProcessor{}
	a := &App{
		dataLoader: func() (processing.Processor, error) {
			attempts++
			if attempts == 1 {
				return nil, errors.New("network timeout")
			}
			return wantProcessor, nil
		},
	}

	if err := a.InitializeApp(); err == nil || err.Error() != "network timeout" {
		t.Fatalf("first InitializeApp error = %v, want network timeout", err)
	}
	if a.processor != nil {
		t.Fatalf("processor = %#v after failed load, want nil", a.processor)
	}

	if err := a.InitializeApp(); err != nil {
		t.Fatalf("retry InitializeApp returned error: %v", err)
	}
	if a.processor != wantProcessor {
		t.Fatalf("processor = %#v, want retry result %#v", a.processor, wantProcessor)
	}
	if attempts != 2 {
		t.Fatalf("loader attempts = %d, want 2", attempts)
	}
}

// freshOrderWorkbook copies the empty (8-header-row, no data) test
// template into a temp dir and returns its path - runBatch now calls
// excelwriter.ClearOrderRows(a.excelPath) before anything else (see
// app.go), which requires a real, valid xlsx file at that path; an
// empty/zero-value excelPath (as these tests used before that change)
// fails immediately with "not a valid zip file", aborting the batch
// before the stub processor ever runs.
func freshOrderWorkbook(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("internal/processing/excelwriter/testdata/dondathang.xlsx")
	if err != nil {
		t.Fatalf("failed reading test fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "dondathang.xlsx")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed writing temp workbook: %v", err)
	}
	return path
}

func TestRunBatch_EmitsLogRowPerFileThenDone(t *testing.T) {
	a := &App{processor: &stubProcessor{}, excelPath: freshOrderWorkbook(t)}
	emitter := &fakeEmitter{}

	a.runBatch(emitter, []string{"a.pdf", "b.pdf"}, nil)

	wantNames := []string{
		"process:progress",
		"process:log", "process:row", "process:progress",
		"process:log", "process:row", "process:progress",
		"process:done",
	}
	if len(emitter.events) != len(wantNames) {
		t.Fatalf("got %d events, want %d: %+v", len(emitter.events), len(wantNames), emitter.events)
	}
	for i, want := range wantNames {
		if emitter.events[i].name != want {
			t.Fatalf("event[%d] = %q, want %q", i, emitter.events[i].name, want)
		}
	}

	// process:done KHÔNG còn mang dữ liệu: bộ đếm STT (config.txt) đã bị bỏ
	// vì không nhánh xử lý nào đọc tới nó. Kiểm rỗng chứ không bỏ hẳn phép
	// kiểm — nếu ai đó gắn lại một payload thì frontend phải biết.
	lastEvent := emitter.events[len(emitter.events)-1]
	if lastEvent.name != "process:done" {
		t.Fatalf("last event = %q, want %q", lastEvent.name, "process:done")
	}
	if len(lastEvent.data) != 0 {
		t.Fatalf("process:done data = %#v, muốn rỗng", lastEvent.data)
	}
}

func TestApp_RunBatchStreamsRowsWithoutDuplicates(t *testing.T) {
	t.Run("dòng cập nhật lại của một khoá đã phát không sinh thêm dòng mới", func(t *testing.T) {
		processor := &streamingStubProcessor{
			emitted: []processing.OrderRow{
				{ResultKey: "jit|1/1|PO1", StatusKind: processing.StatusKindProcessing},
				{ResultKey: "jit|1/1|PO1", StatusKind: processing.StatusKindDone},
			},
			returned: []processing.OrderRow{{ResultKey: "jit|1/1|PO1", StatusKind: processing.StatusKindDone}},
		}
		a := &App{processor: processor, excelPath: freshOrderWorkbook(t)}
		emitter := &fakeEmitter{}

		a.runBatch(emitter, []string{"jit.pdf"}, nil)

		var rows []processing.OrderRow
		for _, event := range emitter.events {
			if event.name == "process:row" {
				rows = append(rows, event.data[0].(processing.OrderRow))
			}
		}
		if len(rows) != 2 || rows[0].StatusKind != processing.StatusKindProcessing || rows[1].StatusKind != processing.StatusKindDone {
			t.Fatalf("streamed rows = %+v, want processing then final update", rows)
		}
		lastEvent := emitter.events[len(emitter.events)-1]
		if lastEvent.name != "process:done" || len(lastEvent.data) != 0 {
			t.Fatalf("sự kiện cuối = %q data %#v, muốn process:done không mang dữ liệu", lastEvent.name, lastEvent.data)
		}
	})

	t.Run("streaming processor emits each row once", func(t *testing.T) {
		processor := &streamingStubProcessor{
			emitted: []processing.OrderRow{
				{ResultKey: "a", StatusKind: processing.StatusKindProcessing, SkuLog: []string{"streamed sku"}},
				{ResultKey: "a", StatusKind: processing.StatusKindDone, SkuLog: []string{"streamed sku"}},
			},
			returned: []processing.OrderRow{
				{ResultKey: "a", SkuLog: []string{"streamed sku"}},
				{ResultKey: "b", SkuLog: []string{"returned sku"}},
			},
		}
		a := &App{
			processor: processor,
			excelPath: freshOrderWorkbook(t),
		}
		emitter := &fakeEmitter{}

		a.runBatch(emitter, []string{"a.pdf"}, nil)

		if processor.streamCalls != 1 || processor.processCalls != 0 {
			t.Fatalf("calls = streaming %d, regular %d; want streaming 1, regular 0", processor.streamCalls, processor.processCalls)
		}
		var keys []string
		for _, event := range emitter.events {
			if event.name != "process:row" {
				continue
			}
			row, ok := event.data[0].(processing.OrderRow)
			if !ok {
				t.Fatalf("process:row data = %T, want processing.OrderRow", event.data[0])
			}
			keys = append(keys, row.ResultKey)
		}
		if !reflect.DeepEqual(keys, []string{"a", "a", "b"}) {
			t.Fatalf("streamed row keys = %#v, want processing/final a then returned b", keys)
		}
		var names []string
		var logPayloads []string
		for _, event := range emitter.events {
			names = append(names, event.name)
			if event.name == "process:log" {
				logPayloads = append(logPayloads, event.data[0].(string))
			}
		}
		// Tiến trình mở màn trước dòng log đầu tiên và đóng lại sau file
		// cuối: thanh trạng thái biết tổng số file ngay từ đầu chứ không
		// phải đợi file đầu tiên chạy xong mới hiện ra.
		wantNames := []string{"process:progress", "process:log", "process:log", "process:row", "process:row", "process:log", "process:row", "process:progress", "process:done"}
		if !reflect.DeepEqual(names, wantNames) {
			t.Fatalf("event names = %#v, want %#v", names, wantNames)
		}
		wantLogs := []string{"Đang xử lý a.pdf...", "streamed sku", "returned sku"}
		if !reflect.DeepEqual(logPayloads, wantLogs) {
			t.Fatalf("process:log payloads = %#v, want each result key's SKU logs exactly once: %#v", logPayloads, wantLogs)
		}
	})

	t.Run("non-streaming processor still emits returned rows", func(t *testing.T) {
		a := &App{
			processor: &stubProcessor{},
			excelPath: freshOrderWorkbook(t),
		}
		emitter := &fakeEmitter{}

		a.runBatch(emitter, []string{"legacy.pdf"}, nil)

		var rows []processing.OrderRow
		for _, event := range emitter.events {
			if event.name != "process:row" {
				continue
			}
			rows = append(rows, event.data[0].(processing.OrderRow))
		}
		if len(rows) != 1 {
			t.Fatalf("process:row count = %d, want 1", len(rows))
		}
		wantSourceID := processing.SourceIDForPath("legacy.pdf")
		wantKey := "legacy:" + wantSourceID + ":::PO1"
		if rows[0].SourceID != wantSourceID || rows[0].ResultKey != wantKey {
			t.Fatalf("legacy processor identity = source %q key %q, want source %q key %q", rows[0].SourceID, rows[0].ResultKey, wantSourceID, wantKey)
		}
	})
}

func TestRunBatch_FileErrorEmitsLogAndContinues(t *testing.T) {
	a := &App{processor: &stubProcessor{failOn: "bad.pdf"}, excelPath: freshOrderWorkbook(t)}
	emitter := &fakeEmitter{}

	a.runBatch(emitter, []string{"bad.pdf", "good.pdf"}, nil)

	wantNames := []string{
		"process:progress",
		"process:log", "process:log", "process:row", "process:progress",
		"process:log", "process:row", "process:progress",
		"process:done",
	}
	if len(emitter.events) != len(wantNames) {
		t.Fatalf("got %d events, want %d: %+v", len(emitter.events), len(wantNames), emitter.events)
	}
	for i, want := range wantNames {
		if emitter.events[i].name != want {
			t.Fatalf("event[%d] = %q, want %q", i, emitter.events[i].name, want)
		}
	}

	// Tìm theo tên sự kiện chứ không theo chỉ số: chỉ số đổi mỗi lần
	// thêm một loại sự kiện mới vào lô, còn "dòng lỗi đầu tiên" thì không.
	var failureRow processing.OrderRow
	found := false
	for _, event := range emitter.events {
		if event.name != "process:row" {
			continue
		}
		row, ok := event.data[0].(processing.OrderRow)
		if !ok {
			t.Fatalf("process:row data = %#v, want processing.OrderRow", event.data)
		}
		if row.StatusKind == processing.StatusKindFailed {
			failureRow, found = row, true
			break
		}
	}
	if !found {
		t.Fatal("không có dòng failed nào cho bad.pdf")
	}
	if failureRow.FileName != "bad.pdf" {
		t.Fatalf("failure row FileName = %q, want %q", failureRow.FileName, "bad.pdf")
	}
	if failureRow.Status != processing.StatusFailed {
		t.Fatalf("failure row Status = %q, want %q", failureRow.Status, processing.StatusFailed)
	}
	if failureRow.StatusKind != processing.StatusKindFailed {
		t.Fatalf("failure row StatusKind = %q, want %q", failureRow.StatusKind, processing.StatusKindFailed)
	}

	lastEvent := emitter.events[len(emitter.events)-1]
	if lastEvent.name != "process:done" {
		t.Fatalf("last event = %q, want %q", lastEvent.name, "process:done")
	}
	if len(lastEvent.data) != 0 {
		t.Fatalf("process:done data = %#v, muốn rỗng", lastEvent.data)
	}
}

// chdirForTest changes the process working directory to dir and
// registers a t.Cleanup that restores the original working directory,
// even on test failure/panic. resolveRepoFile's tests need this since
// it walks up from os.Getwd(), which os.Chdir is the only way to
// simulate without changing resolveRepoFile's signature.
func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origWD); err != nil {
			t.Fatalf("restore Chdir: %v", err)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
}

func TestResolveRepoFile_FindsFileInAncestorDirectory(t *testing.T) {
	base := t.TempDir()
	const markerName = "marker.txt"
	if err := os.WriteFile(filepath.Join(base, markerName), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// base/a/b/c/d is 4 directories below base. resolveRepoFile checks
	// cwd plus up to 4 parents (5 directories total), so base sits
	// exactly at the edge of what it can find.
	nested := filepath.Join(base, "a", "b", "c", "d")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	chdirForTest(t, nested)

	got := resolveRepoFile(markerName)
	wantAbs, err := filepath.Abs(filepath.Join(base, markerName))
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	gotAbs, err := filepath.Abs(got)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	if gotAbs != wantAbs {
		t.Fatalf("resolveRepoFile(%q) = %q, want %q", markerName, gotAbs, wantAbs)
	}
}

func TestResolveRepoFile_FallsBackToBareNameBeyondSearchDepth(t *testing.T) {
	base := t.TempDir()
	const markerName = "marker.txt"
	if err := os.WriteFile(filepath.Join(base, markerName), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// One directory deeper than the "found" test above puts base just
	// outside resolveRepoFile's cwd+4-parents search window, so the
	// file — despite existing — must not be found, and the bare
	// filename must be returned instead.
	nested := filepath.Join(base, "a", "b", "c", "d", "e")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	chdirForTest(t, nested)

	got := resolveRepoFile(markerName)
	if got != markerName {
		t.Fatalf("resolveRepoFile(%q) = %q, want bare filename %q (not found within search depth)", markerName, got, markerName)
	}
}

func TestApp_ConfirmPrice_DelegatesToExcelwriter(t *testing.T) {
	src := "internal/processing/excelwriter/testdata/dondathang.xlsx"
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed reading test fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "dondathang.xlsx")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed writing temp workbook: %v", err)
	}

	rows := []excelwriter.Row{
		{SKU: "3564270-4", Qty: 24, UnitPrice: 33000, InvoicePrice: 33726, PriceMismatch: true, UseZFormula: true},
	}
	if _, err := excelwriter.WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	a := &App{excelPath: path}
	if err := a.ConfirmPrice(9, 33726); err != nil {
		t.Fatalf("App.ConfirmPrice returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()
	val, _ := f.GetCellValue("Don dat hang", "Y9")
	if val != "33726" {
		t.Fatalf("Y9 = %q, want %q", val, "33726")
	}
}

func TestApp_UpdateJITPeriodUpdatesSelectedExcelRows(t *testing.T) {
	path := freshOrderWorkbook(t)
	if _, err := excelwriter.WriteOrderRows(path, []excelwriter.Row{
		{OrderNumber: "old", Description: "old"},
		{OrderNumber: "old", Description: "old"},
	}, ""); err != nil {
		t.Fatal(err)
	}
	a := &App{excelPath: path}
	if err := a.UpdateJITPeriod([]int{9, 10}, "24/08/2026", "WH6_HN", "Chiều"); err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, _ := f.GetCellValue("Don dat hang", "B10")
	if got != "ĐĐHJIT-24/08/2026 (chiều)-WH6_HN" {
		t.Fatalf("B10 = %q", got)
	}
}

func TestApp_WorkbookMutationCannotOverlapNewBatch(t *testing.T) {
	mutationEntered := make(chan struct{})
	releaseMutation := make(chan struct{})
	mutationDone := make(chan error, 1)
	batchEntered := make(chan struct{})
	releaseBatch := make(chan struct{})
	batchDone := make(chan struct{})

	a := &App{
		excelPath: freshOrderWorkbook(t),
		processor: &channelProcessor{entered: batchEntered, release: releaseBatch},
		updateJITPeriodFn: func(string, []int, string, string, string) error {
			close(mutationEntered)
			<-releaseMutation
			return nil
		},
	}

	go func() {
		mutationDone <- a.UpdateJITPeriod([]int{9}, "24/08/2026", "WH6_HN", "Chiều")
	}()
	<-mutationEntered

	go func() {
		a.runBatch(&fakeEmitter{}, []string{"batch.pdf"}, nil)
		close(batchDone)
	}()

	select {
	case <-batchEntered:
		t.Fatal("batch processor entered while UpdateJITPeriod still owned the workbook mutation critical section")
	case <-time.After(150 * time.Millisecond):
	}

	close(releaseMutation)
	if err := <-mutationDone; err != nil {
		t.Fatalf("UpdateJITPeriod returned error: %v", err)
	}

	select {
	case <-batchEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("batch did not enter after workbook mutation completed")
	}
	close(releaseBatch)
	select {
	case <-batchDone:
	case <-time.After(2 * time.Second):
		t.Fatal("batch did not finish after processor was released")
	}
}

func TestApp_ConfirmPrice_RejectsWhileProcessingBatch(t *testing.T) {
	src := "internal/processing/excelwriter/testdata/dondathang.xlsx"
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed reading test fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "dondathang.xlsx")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed writing temp workbook: %v", err)
	}

	rows := []excelwriter.Row{
		{SKU: "3564270-4", Qty: 24, UnitPrice: 33000, InvoicePrice: 33726, PriceMismatch: true, UseZFormula: true},
	}
	if _, err := excelwriter.WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	a := &App{excelPath: path}
	a.processing.Store(true)

	err = a.ConfirmPrice(9, 33726)
	if err == nil {
		t.Fatal("ConfirmPrice returned nil error while a.processing was true, want a rejection")
	}

	// Confirm nothing was written — the row must be untouched.
	f, ferr := excelize.OpenFile(path)
	if ferr != nil {
		t.Fatalf("failed reopening workbook: %v", ferr)
	}
	defer f.Close()
	val, _ := f.GetCellValue("Don dat hang", "Y9")
	if val == "33726" {
		t.Fatal("Y9 was written despite a.processing being true — the guard did not actually block the write")
	}
}

func TestApp_ConfirmPrice_SecondCallForSameRowUsesSetPriceNotConfirmPrice(t *testing.T) {
	src := "internal/processing/excelwriter/testdata/dondathang.xlsx"
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed reading test fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "dondathang.xlsx")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed writing temp workbook: %v", err)
	}

	rows := []excelwriter.Row{
		{SKU: "3564270-4", Qty: 24, UnitPrice: 33000, InvoicePrice: 33726, PriceMismatch: true, UseZFormula: true},
	}
	if _, err := excelwriter.WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	a := &App{excelPath: path}

	// First call: row 9 genuinely has a mismatch comment, so this goes
	// through excelwriter.ConfirmPrice's normal path and succeeds.
	if err := a.ConfirmPrice(9, 33726); err != nil {
		t.Fatalf("first ConfirmPrice call returned error: %v", err)
	}

	// Second call for the SAME row, a different price (simulating the
	// user changing their mind): the comment excelwriter.ConfirmPrice
	// deleted on the first call is gone, so a naive second call to
	// excelwriter.ConfirmPrice would now be rejected — proving this must
	// route through the resolvedRows bypass (excelwriter.SetPrice)
	// instead.
	if err := a.ConfirmPrice(9, 30000); err != nil {
		t.Fatalf("second ConfirmPrice call (change of mind) returned error: %v — re-toggle is broken", err)
	}

	f, ferr := excelize.OpenFile(path)
	if ferr != nil {
		t.Fatalf("failed reopening workbook: %v", ferr)
	}
	defer f.Close()
	val, _ := f.GetCellValue("Don dat hang", "Y9")
	if val != "30000" {
		t.Fatalf("Y9 = %q after the second (change-of-mind) call, want %q", val, "30000")
	}
}

type fakeZaloSender struct {
	loginErr         error
	sendErrs         map[string]error // key = contactQuery
	qrFrames         []string         // svgMarkup values EnsureLoggedIn should feed to onQR, in order
	blockUntilCancel bool             // simulate ChromedpSender's real login-wait loop honoring ctx cancellation
	loginCalls       int
	sentTo           []string
	refreshQRCalls   int
}

func (f *fakeZaloSender) EnsureLoggedIn(ctx context.Context, onQR func(svgMarkup string)) error {
	f.loginCalls++
	if onQR != nil {
		for _, frame := range f.qrFrames {
			onQR(frame)
		}
	}
	if f.blockUntilCancel {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.loginErr
}

func (f *fakeZaloSender) SendMessage(ctx context.Context, contactQuery, message string) error {
	f.sentTo = append(f.sentTo, contactQuery)
	if f.sendErrs != nil {
		if err, ok := f.sendErrs[contactQuery]; ok {
			return err
		}
	}
	return nil
}

func (f *fakeZaloSender) RefreshQR(ctx context.Context) error {
	f.refreshQRCalls++
	return nil
}

func (f *fakeZaloSender) Close() error { return nil }

func newTestAppForZalo(t *testing.T, sender zalosend.ZaloSender, zaloMap map[string]string) *App {
	t.Helper()
	store := appsettings.NewStore(filepath.Join(t.TempDir(), "settings.bhconfig"))
	if err := store.Save(appsettings.Settings{Zalo: zaloMap}); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	return &App{appSettingsStore: store, zaloSender: sender}
}

func sentEventsOf(t *testing.T, events []emittedEvent) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, e := range events {
		if e.name == "zalo:sent" {
			data, ok := e.data[0].(map[string]any)
			if !ok {
				t.Fatalf("zalo:sent data is not map[string]any: %#v", e.data)
			}
			out = append(out, data)
		}
	}
	return out
}

func TestRunZaloBatch_SendsEachJobAndEmitsEvents(t *testing.T) {
	sender := &fakeZaloSender{}
	a := newTestAppForZalo(t, sender, map[string]string{"MNCOOPMART": "Nhom Coop", "MNBIGC": "Nhom BigC"})
	emitter := &fakeEmitter{}

	a.runZaloBatch(emitter, []ZaloJob{
		{PO: "PO1", System: "COOP", CustomerCode: "MN0001", Message: "noi dung 1"},
		{PO: "PO2", System: "BIGC", CustomerCode: "MN0002", Message: "noi dung 2"},
	})

	if sender.loginCalls != 1 {
		t.Fatalf("loginCalls = %d, want 1", sender.loginCalls)
	}
	// Không khẳng định THỨ TỰ cụ thể ở đây — runZaloBatch cố ý sắp xếp
	// lại theo liên hệ (xem TestRunZaloBatch_GroupsJobsByContact), test
	// này chỉ quan tâm CẢ 2 job đều được gửi và đều báo thành công.
	wantSentTo := []string{"Nhom BigC", "Nhom Coop"}
	gotSentTo := append([]string{}, sender.sentTo...)
	sort.Strings(gotSentTo)
	if !reflect.DeepEqual(gotSentTo, wantSentTo) {
		t.Fatalf("sentTo (sorted) = %#v, want %#v", gotSentTo, wantSentTo)
	}

	lastEvent := emitter.events[len(emitter.events)-1]
	if lastEvent.name != "zalo:done" {
		t.Fatalf("last event = %q, want zalo:done", lastEvent.name)
	}

	sent := sentEventsOf(t, emitter.events)
	if len(sent) != 2 {
		t.Fatalf("zalo:sent events = %#v, want 2", sent)
	}
	okByPO := map[string]any{sent[0]["po"].(string): sent[0]["ok"], sent[1]["po"].(string): sent[1]["ok"]}
	if okByPO["PO1"] != true || okByPO["PO2"] != true {
		t.Fatalf("zalo:sent events = %#v, want both PO1 and PO2 ok=true", sent)
	}
}

// runZaloBatch resolves every job's contact FIRST, then stable-sorts by
// contact name so jobs sharing one Zalo group land next to each other —
// the browser shouldn't have to search/open the same conversation twice
// in a row when the user's original selection interleaved groups (vd
// Satra, BigC, Satra). Contacts here are chosen so sorting genuinely
// reorders "S1"/"S3" (System) away from their original PO1/PO3
// positions next to "S2" (a different contact) in between.
func TestRunZaloBatch_GroupsJobsByContact(t *testing.T) {
	sender := &fakeZaloSender{}
	a := newTestAppForZalo(t, sender, map[string]string{
		"MNAAA": "Nhom AAA",
		"MNBBB": "Nhom BBB",
	})
	emitter := &fakeEmitter{}

	// PO1/PO3 both resolve to "Nhom AAA", PO2 to "Nhom BBB" - selection
	// order interleaves them (AAA, BBB, AAA); after grouping, both AAA
	// sends must be adjacent (in either relative position vs BBB, but
	// never split apart by BBB again).
	a.runZaloBatch(emitter, []ZaloJob{
		{PO: "PO1", System: "AAA", CustomerCode: "MN0001", Message: "x"},
		{PO: "PO2", System: "BBB", CustomerCode: "MN0002", Message: "y"},
		{PO: "PO3", System: "AAA", CustomerCode: "MN0003", Message: "z"},
	})

	if len(sender.sentTo) != 3 {
		t.Fatalf("sentTo = %#v, want 3 sends", sender.sentTo)
	}
	// Find the two "Nhom AAA" sends and confirm they are ADJACENT in
	// the actual send order (no "Nhom BBB" send landed between them).
	aaaPositions := []int{}
	for i, contact := range sender.sentTo {
		if contact == "Nhom AAA" {
			aaaPositions = append(aaaPositions, i)
		}
	}
	if len(aaaPositions) != 2 {
		t.Fatalf("sentTo = %#v, want exactly 2 sends to Nhom AAA", sender.sentTo)
	}
	if aaaPositions[1]-aaaPositions[0] != 1 {
		t.Fatalf("sentTo = %#v, the two Nhom AAA sends are not adjacent (grouping failed)", sender.sentTo)
	}
}

func TestRunZaloBatch_SkipsJobWithoutContact(t *testing.T) {
	sender := &fakeZaloSender{}
	a := newTestAppForZalo(t, sender, map[string]string{"MNCOOPMART": "Nhom Coop"})
	emitter := &fakeEmitter{}

	a.runZaloBatch(emitter, []ZaloJob{
		{PO: "PO1", System: "UNKNOWN", CustomerCode: "MN0001", Message: "noi dung 1"},
		{PO: "PO2", System: "COOP", CustomerCode: "MN0002", Message: "noi dung 2"},
	})

	if !reflect.DeepEqual(sender.sentTo, []string{"Nhom Coop"}) {
		t.Fatalf("sentTo = %#v, want only the configured contact attempted", sender.sentTo)
	}

	sent := sentEventsOf(t, emitter.events)
	if len(sent) != 2 || sent[0]["po"] != "PO1" || sent[0]["ok"] != false || sent[1]["po"] != "PO2" || sent[1]["ok"] != true {
		t.Fatalf("zalo:sent events = %#v", sent)
	}
}

func TestRunZaloBatch_ContinuesAfterOneJobFails(t *testing.T) {
	sender := &fakeZaloSender{sendErrs: map[string]error{"Nhom Loi": errors.New("boom")}}
	a := newTestAppForZalo(t, sender, map[string]string{"MNLOI": "Nhom Loi", "MNCOOPMART": "Nhom Coop"})
	emitter := &fakeEmitter{}

	a.runZaloBatch(emitter, []ZaloJob{
		{PO: "PO1", System: "LOI", CustomerCode: "MN0001", Message: "x"},
		{PO: "PO2", System: "COOP", CustomerCode: "MN0002", Message: "y"},
	})

	// Sắp xếp theo tên liên hệ: "Nhom Coop" < "Nhom Loi" - COOP (PO2, gửi
	// thành công) gửi trước, LOI (PO1, gửi lỗi) gửi sau.
	if !reflect.DeepEqual(sender.sentTo, []string{"Nhom Coop", "Nhom Loi"}) {
		t.Fatalf("sentTo = %#v, want both contacts attempted despite one failing", sender.sentTo)
	}
	sent := sentEventsOf(t, emitter.events)
	if len(sent) != 2 || sent[0]["ok"] != true || sent[1]["ok"] != false {
		t.Fatalf("zalo:sent events = %#v", sent)
	}
}

func TestRunZaloBatch_AbortsWholeBatchIfLoginFails(t *testing.T) {
	sender := &fakeZaloSender{loginErr: errors.New("login timeout")}
	a := newTestAppForZalo(t, sender, map[string]string{"COOP": "Nhom Coop"})
	emitter := &fakeEmitter{}

	a.runZaloBatch(emitter, []ZaloJob{{PO: "PO1", System: "COOP", Message: "x"}})

	if len(sender.sentTo) != 0 {
		t.Fatalf("sentTo = %#v, want no send attempted after login failure", sender.sentTo)
	}
	lastEvent := emitter.events[len(emitter.events)-1]
	if lastEvent.name != "zalo:done" {
		t.Fatalf("last event = %q, want zalo:done", lastEvent.name)
	}
}

func TestApp_SendZaloMessages_RejectsWhileAlreadySending(t *testing.T) {
	sender := &fakeZaloSender{}
	a := newTestAppForZalo(t, sender, map[string]string{"COOP": "Nhom Coop"})
	emitter := &fakeEmitter{}
	a.emitter = emitter
	a.sending.Store(true)

	a.SendZaloMessages([]ZaloJob{{PO: "PO1", System: "COOP", Message: "x"}})

	if len(emitter.events) != 1 || emitter.events[0].name != "zalo:log" {
		t.Fatalf("events = %#v, want a single zalo:log warning", emitter.events)
	}
	if sender.loginCalls != 0 {
		t.Fatalf("loginCalls = %d, want 0 (must not start a new batch while one is running)", sender.loginCalls)
	}
}

// An empty job list must be a complete no-op: without the guard it would
// claim the sending flag, open a real browser and potentially sit waiting
// up to 120s for a QR scan just to send nothing at all.
func TestApp_SendZaloMessages_IgnoresEmptyJobList(t *testing.T) {
	sender := &fakeZaloSender{}
	a := newTestAppForZalo(t, sender, map[string]string{"COOP": "Nhom Coop"})
	emitter := &fakeEmitter{}
	a.emitter = emitter

	a.SendZaloMessages(nil)
	a.SendZaloMessages([]ZaloJob{})

	if sender.loginCalls != 0 {
		t.Fatalf("loginCalls = %d, want 0 (must not open a browser for an empty batch)", sender.loginCalls)
	}
	if len(emitter.events) != 0 {
		t.Fatalf("events = %#v, want none emitted for an empty batch", emitter.events)
	}
	if a.sending.Load() {
		t.Fatal("sending flag left set by an empty batch")
	}
}

// runZaloBatch's onQR closure must relay every QR frame EnsureLoggedIn
// hands it straight through as a zalo:qr event, in order, so the
// frontend's QR popup receives each new code as it's read from the page.
func TestRunZaloBatch_RelaysQRFramesAsEvents(t *testing.T) {
	sender := &fakeZaloSender{qrFrames: []string{"<svg>1</svg>", "<svg>2</svg>"}}
	a := newTestAppForZalo(t, sender, map[string]string{"COOP": "Nhom Coop"})
	emitter := &fakeEmitter{}

	a.runZaloBatch(emitter, []ZaloJob{{PO: "PO1", System: "COOP", Message: "x"}})

	var qrEvents []string
	for _, e := range emitter.events {
		if e.name == "zalo:qr" {
			data, ok := e.data[0].(string)
			if !ok {
				t.Fatalf("zalo:qr data is not a string: %#v", e.data)
			}
			qrEvents = append(qrEvents, data)
		}
	}
	want := []string{"<svg>1</svg>", "<svg>2</svg>"}
	if !reflect.DeepEqual(qrEvents, want) {
		t.Fatalf("zalo:qr events = %#v, want %#v", qrEvents, want)
	}
}

// CancelZaloLogin must reach an EnsureLoggedIn call already in flight on
// a different goroutine (exactly how runZaloBatch/SendZaloMessages
// actually run in production) and cause runZaloBatch to abort the whole
// batch without sending anything.
func TestApp_CancelZaloLogin_AbortsInFlightLogin(t *testing.T) {
	sender := &fakeZaloSender{blockUntilCancel: true}
	a := newTestAppForZalo(t, sender, map[string]string{"COOP": "Nhom Coop"})
	emitter := &fakeEmitter{}
	a.emitter = emitter

	done := make(chan struct{})
	go func() {
		a.runZaloBatch(emitter, []ZaloJob{{PO: "PO1", System: "COOP", Message: "x"}})
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		a.zaloLoginMu.Lock()
		hasCancel := a.zaloLoginCancel != nil
		a.zaloLoginMu.Unlock()
		if hasCancel {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for runZaloBatch to register a cancel func")
		}
		time.Sleep(time.Millisecond)
	}

	a.CancelZaloLogin()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runZaloBatch did not return after CancelZaloLogin")
	}

	if len(sender.sentTo) != 0 {
		t.Fatalf("sentTo = %#v, want no send attempted after a cancelled login", sender.sentTo)
	}
	a.zaloLoginMu.Lock()
	leftoverCancel := a.zaloLoginCancel
	a.zaloLoginMu.Unlock()
	if leftoverCancel != nil {
		t.Fatal("zaloLoginCancel not cleared after runZaloBatch returned")
	}
}

// A CancelZaloLogin call with no login in flight (never started, or
// already finished) must be a safe no-op.
func TestApp_CancelZaloLogin_NoopWhenNothingInFlight(t *testing.T) {
	sender := &fakeZaloSender{}
	a := newTestAppForZalo(t, sender, map[string]string{"COOP": "Nhom Coop"})

	a.CancelZaloLogin() // must not panic
}

func TestApp_RefreshZaloQR_DelegatesToSender(t *testing.T) {
	sender := &fakeZaloSender{}
	a := newTestAppForZalo(t, sender, map[string]string{"COOP": "Nhom Coop"})

	if err := a.RefreshZaloQR(); err != nil {
		t.Fatalf("RefreshZaloQR returned error: %v", err)
	}
	if sender.refreshQRCalls != 1 {
		t.Fatalf("refreshQRCalls = %d, want 1", sender.refreshQRCalls)
	}
}

func TestApp_RunBatchReportsProgressPerFile(t *testing.T) {
	// Frontend không có cách nào tự đếm "còn bao nhiêu file nữa": một file
	// có thể phát ra dòng tạm rồi mới xong (BigC, JIT), nên đếm theo dòng
	// sẽ báo xong sớm. Backend đang lặp qua danh sách file nên chỉ nó biết
	// con số thật - đây là chỗ nói ra.
	a := &App{processor: &stubProcessor{}, excelPath: freshOrderWorkbook(t)}
	emitter := &fakeEmitter{}

	a.runBatch(emitter, []string{"a.pdf", "b.pdf", "c.pdf"}, nil)

	var got []BatchProgress
	for _, event := range emitter.events {
		if event.name == "process:progress" {
			got = append(got, event.data[0].(BatchProgress))
		}
	}
	want := []BatchProgress{
		{Done: 0, Total: 3},
		{Done: 1, Total: 3},
		{Done: 2, Total: 3},
		{Done: 3, Total: 3},
	}
	if len(got) != len(want) {
		t.Fatalf("progress events = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("progress[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestApp_RunBatchReportsProgressEvenWhenAFileFails(t *testing.T) {
	a := &App{processor: &stubProcessor{failOn: "b.pdf"}, excelPath: freshOrderWorkbook(t)}
	emitter := &fakeEmitter{}

	a.runBatch(emitter, []string{"a.pdf", "b.pdf"}, nil)

	last := BatchProgress{}
	for _, event := range emitter.events {
		if event.name == "process:progress" {
			last = event.data[0].(BatchProgress)
		}
	}
	if (last != BatchProgress{Done: 2, Total: 2}) {
		t.Fatalf("progress cuối = %+v, want mọi file đều được tính kể cả file lỗi", last)
	}
}
