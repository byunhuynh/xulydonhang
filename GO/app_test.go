package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/config"
	"order-processor/internal/processing"
	"order-processor/internal/processing/excelwriter"
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
