package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/appsettings"
	"order-processor/internal/config"
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

func (s *stubProcessor) Process(ctx context.Context, filePath string, stt int) ([]processing.OrderRow, error) {
	if filePath == s.failOn {
		return nil, errors.New("stub failure")
	}
	return []processing.OrderRow{{FileName: filePath, PO: "PO1", Status: processing.StatusDone}}, nil
}

type streamingStubProcessor struct {
	emitted []processing.OrderRow
	returned []processing.OrderRow
	streamCalls int
	processCalls int
}

var _ processing.StreamingProcessor = (*streamingStubProcessor)(nil)

func (s *streamingStubProcessor) Process(ctx context.Context, filePath string, stt int) ([]processing.OrderRow, error) {
	s.processCalls++
	return s.returned, nil
}

func (s *streamingStubProcessor) ProcessStreaming(ctx context.Context, filePath string, stt int, emit func(processing.OrderRow)) ([]processing.OrderRow, error) {
	s.streamCalls++
	for _, row := range s.emitted { emit(row) }
	return s.returned, nil
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
	cfg := config.NewStore(filepath.Join(t.TempDir(), "config.txt"))
	a := &App{cfg: cfg, processor: &stubProcessor{}, excelPath: freshOrderWorkbook(t)}
	emitter := &fakeEmitter{}

	a.runBatch(emitter, []string{"a.pdf", "b.pdf"}, 10)

	wantNames := []string{"process:log", "process:row", "process:log", "process:row", "process:done"}
	if len(emitter.events) != len(wantNames) {
		t.Fatalf("got %d events, want %d: %+v", len(emitter.events), len(wantNames), emitter.events)
	}
	for i, want := range wantNames {
		if emitter.events[i].name != want {
			t.Fatalf("event[%d] = %q, want %q", i, emitter.events[i].name, want)
		}
	}

	wantDoneData := []interface{}{12}
	lastEvent := emitter.events[len(emitter.events)-1]
	if lastEvent.name != "process:done" {
		t.Fatalf("last event = %q, want %q", lastEvent.name, "process:done")
	}
	if !reflect.DeepEqual(lastEvent.data, wantDoneData) {
		t.Fatalf("process:done data = %#v, want %#v", lastEvent.data, wantDoneData)
	}

	gotSTT, err := cfg.GetSTT()
	if err != nil {
		t.Fatalf("GetSTT returned error: %v", err)
	}
	if gotSTT != 12 {
		t.Fatalf("STT after batch = %d, want 12", gotSTT)
	}
}

func TestApp_RunBatchStreamsRowsWithoutDuplicates(t *testing.T) {
	t.Run("streaming processor emits each row once", func(t *testing.T) {
		processor := &streamingStubProcessor{
			emitted: []processing.OrderRow{{ResultKey: "a", SkuLog: []string{"streamed sku"}}},
			returned: []processing.OrderRow{
				{ResultKey: "a", SkuLog: []string{"streamed sku"}},
				{ResultKey: "b", SkuLog: []string{"returned sku"}},
			},
		}
		a := &App{cfg: config.NewStore(filepath.Join(t.TempDir(), "config.txt")), processor: processor, excelPath: freshOrderWorkbook(t)}
		emitter := &fakeEmitter{}
		a.runBatch(emitter, []string{"a.pdf"}, 1)
		if processor.streamCalls != 1 || processor.processCalls != 0 {
			t.Fatalf("calls = streaming %d, regular %d; want streaming 1, regular 0", processor.streamCalls, processor.processCalls)
		}
		var keys []string
		for _, event := range emitter.events {
			if event.name == "process:row" {
				row, ok := event.data[0].(processing.OrderRow)
				if !ok { t.Fatalf("process:row data = %T, want processing.OrderRow", event.data[0]) }
				keys = append(keys, row.ResultKey)
			}
		}
		if !reflect.DeepEqual(keys, []string{"a", "b"}) { t.Fatalf("streamed row keys = %#v, want []string{\"a\", \"b\"}", keys) }
		var names []string
		for _, event := range emitter.events { names = append(names, event.name) }
		wantNames := []string{"process:log", "process:log", "process:row", "process:log", "process:row", "process:done"}
		if !reflect.DeepEqual(names, wantNames) { t.Fatalf("event names = %#v, want %#v", names, wantNames) }
	})
	t.Run("non-streaming processor still emits returned rows", func(t *testing.T) {
		a := &App{cfg: config.NewStore(filepath.Join(t.TempDir(), "config.txt")), processor: &stubProcessor{}, excelPath: freshOrderWorkbook(t)}
		emitter := &fakeEmitter{}
		a.runBatch(emitter, []string{"legacy.pdf"}, 1)
		var rows []processing.OrderRow
		for _, event := range emitter.events { if event.name == "process:row" { rows = append(rows, event.data[0].(processing.OrderRow)) } }
		if len(rows) != 1 { t.Fatalf("process:row count = %d, want 1", len(rows)) }
		if rows[0].ResultKey != "legacy:legacy.pdf:::PO1" { t.Fatalf("legacy processor ResultKey = %q, want deterministic fallback", rows[0].ResultKey) }
	})
}

func TestRunBatch_FileErrorEmitsLogAndContinues(t *testing.T) {
	cfg := config.NewStore(filepath.Join(t.TempDir(), "config.txt"))
	a := &App{cfg: cfg, processor: &stubProcessor{failOn: "bad.pdf"}, excelPath: freshOrderWorkbook(t)}
	emitter := &fakeEmitter{}

	a.runBatch(emitter, []string{"bad.pdf", "good.pdf"}, 1)

	wantNames := []string{"process:log", "process:log", "process:row", "process:log", "process:row", "process:done"}
	if len(emitter.events) != len(wantNames) {
		t.Fatalf("got %d events, want %d: %+v", len(emitter.events), len(wantNames), emitter.events)
	}
	for i, want := range wantNames {
		if emitter.events[i].name != want {
			t.Fatalf("event[%d] = %q, want %q", i, emitter.events[i].name, want)
		}
	}

	failureRow, ok := emitter.events[2].data[0].(processing.OrderRow)
	if !ok {
		t.Fatalf("event[2].data[0] is not an OrderRow: %#v", emitter.events[2].data)
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

	wantDoneData := []interface{}{3}
	lastEvent := emitter.events[len(emitter.events)-1]
	if lastEvent.name != "process:done" {
		t.Fatalf("last event = %q, want %q", lastEvent.name, "process:done")
	}
	if !reflect.DeepEqual(lastEvent.data, wantDoneData) {
		t.Fatalf("process:done data = %#v, want %#v", lastEvent.data, wantDoneData)
	}

	gotSTT, err := cfg.GetSTT()
	if err != nil {
		t.Fatalf("GetSTT returned error: %v", err)
	}
	if gotSTT != 3 {
		t.Fatalf("STT after batch = %d, want 3", gotSTT)
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
	loginErr   error
	sendErrs   map[string]error // key = contactQuery
	loginCalls int
	sentTo     []string
}

func (f *fakeZaloSender) EnsureLoggedIn(ctx context.Context) error {
	f.loginCalls++
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
	a := newTestAppForZalo(t, sender, map[string]string{"COOP": "Nhom Coop", "BIGC": "Nhom BigC"})
	emitter := &fakeEmitter{}

	a.runZaloBatch(emitter, []ZaloJob{
		{PO: "PO1", System: "COOP", Message: "noi dung 1"},
		{PO: "PO2", System: "BIGC", Message: "noi dung 2"},
	})

	if sender.loginCalls != 1 {
		t.Fatalf("loginCalls = %d, want 1", sender.loginCalls)
	}
	wantSentTo := []string{"Nhom Coop", "Nhom BigC"}
	if !reflect.DeepEqual(sender.sentTo, wantSentTo) {
		t.Fatalf("sentTo = %#v, want %#v", sender.sentTo, wantSentTo)
	}

	lastEvent := emitter.events[len(emitter.events)-1]
	if lastEvent.name != "zalo:done" {
		t.Fatalf("last event = %q, want zalo:done", lastEvent.name)
	}

	sent := sentEventsOf(t, emitter.events)
	if len(sent) != 2 || sent[0]["po"] != "PO1" || sent[0]["ok"] != true || sent[1]["po"] != "PO2" || sent[1]["ok"] != true {
		t.Fatalf("zalo:sent events = %#v", sent)
	}
}

func TestRunZaloBatch_SkipsJobWithoutContact(t *testing.T) {
	sender := &fakeZaloSender{}
	a := newTestAppForZalo(t, sender, map[string]string{"COOP": "Nhom Coop"})
	emitter := &fakeEmitter{}

	a.runZaloBatch(emitter, []ZaloJob{
		{PO: "PO1", System: "UNKNOWN", Message: "noi dung 1"},
		{PO: "PO2", System: "COOP", Message: "noi dung 2"},
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
	a := newTestAppForZalo(t, sender, map[string]string{"LOI": "Nhom Loi", "COOP": "Nhom Coop"})
	emitter := &fakeEmitter{}

	a.runZaloBatch(emitter, []ZaloJob{
		{PO: "PO1", System: "LOI", Message: "x"},
		{PO: "PO2", System: "COOP", Message: "y"},
	})

	if !reflect.DeepEqual(sender.sentTo, []string{"Nhom Loi", "Nhom Coop"}) {
		t.Fatalf("sentTo = %#v, want both contacts attempted despite the first failing", sender.sentTo)
	}
	sent := sentEventsOf(t, emitter.events)
	if len(sent) != 2 || sent[0]["ok"] != false || sent[1]["ok"] != true {
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
