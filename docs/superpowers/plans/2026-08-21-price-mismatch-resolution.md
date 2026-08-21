# Price Mismatch Resolution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After processing, let the user review every price-mismatched product per order and choose — per SKU — whether to keep the PO's invoice price or the system's computed price, writing the choice directly into the already-written Excel cell.

**Architecture:** Add a `PriceMismatchDetail` (SKU/name/PO price/system price/real Excel row) collected by every vendor processor at the exact point it already computes `matched`/`finalPrice`/`invoicePrice`; a new `excelwriter.ConfirmPrice` function edits one already-written Y cell (value + clears the mismatch red-fill/comment) after an explicit safety check that the cell is still actually flagged; a new Wails-bound `App.ConfirmPrice` exposes it to the frontend; `ResultTable.tsx` gets an expandable per-order detail view with two buttons per mismatched SKU.

**Tech Stack:** Go 1.x, `excelize/v2`, existing `processing`/`excelwriter` packages; React + TypeScript + Tailwind (frontend, no new dependencies).

**Spec:** [docs/superpowers/specs/2026-08-21-price-mismatch-resolution-design.md](../specs/2026-08-21-price-mismatch-resolution-design.md)

## Global Constraints

- **`excelwriter.WriteOrderRows`'s signature changes from `(path string, rows []Row, headerDescription string) error` to `(path string, rows []Row, headerDescription string) (startRow int, err error)`.** This is a breaking change to every one of its 9 vendor call sites (`coop_processor.go`, `lotte_processor.go`, `satra_processor.go`, `bigc_processor.go`, `winmart_processor.go`, `emart_processor.go`, `fujimart_processor.go`, `kingfood_processor.go`, `jmart_processor.go`) plus 3 call sites in `excelwriter/dondathang_test.go`. **All 12 call sites must be updated in the SAME task/commit as the signature change** — the `processing` package will not compile with the signature changed but call sites unfixed, so this cannot be split per-vendor across multiple tasks or commits.
- **`startRow` is the value `WriteOrderRows` already computes internally as `firstRow`** (`GO/internal/processing/excelwriter/dondathang.go:79`, `firstRow := currentRow` — `currentRow := len(existingRows) + 1` one line above) — just return it. No new computation.
- **Every vendor's product loop already has `productRowIndex := len(rows)`** (captured before appending that product's row) **except BigC**, which builds `PromoNote`/`PromoBundleSku` directly into the row literal instead of mutating `rows[productRowIndex]` afterward, so it has never needed this variable — BigC's task step adds `productRowIndex := len(rows)` fresh, immediately before its own `rows = append(rows, productRow)` call.
- **`PriceMismatchDetail.ExcelRow` computation is a 2-step add, not a single formula**, because `startRow` is only known AFTER `WriteOrderRows` returns (at the very end of each vendor function), while `productRowIndex` is captured mid-loop: collect `PriceMismatchDetail{..., ExcelRow: productRowIndex}` during the loop (temporarily storing just the local index), then immediately after the `WriteOrderRows` call succeeds, loop once more and do `detail.ExcelRow += startRow` for every collected detail, before constructing the final returned `OrderRow`.
- **BigC needs a 3-step add, not 2**, because `processBigcDocument` combines EVERY store page's rows into ONE `WriteOrderRows` call (`bigc_processor.go:144`) — a given store's `productRowIndex` (local to that store's own `rows` slice inside `processBigcStorePage`) must first be adjusted by how many rows every EARLIER store already contributed to the combined `allRows` slice (`len(allRows)` at the point that store's results are being folded in, BEFORE its own `result.rows` are appended), and only THEN by the final `startRow` once `WriteOrderRows` returns. See Task 3's BigC subsection for the exact ordering.
- **Verified empirically (Task 0, this plan's own author, against `excelwriter/testdata/dondathang.xlsx`) before writing any of the code below**, so nothing in this plan is guessed:
  - The test template's own Y-column cells have style ID **0** before any write — `excelize`'s `SetCellStyle(sheet, cell, cell, 0)` genuinely restores the ORIGINAL style, not a different "default" style, so resetting a mismatch-flagged cell's style to 0 is safe and loses no formatting.
  - `f.GetComments(sheet)` returns `[]excelize.Comment` for the WHOLE SHEET — the caller must loop and match `.Cell == "Y9"` (or whatever cell) itself; there is no single-cell lookup method.
  - `f.DeleteComment(sheet, cell)` on a cell that has NO comment returns `nil` (silent no-op, not an error) — **cannot be relied on to reject "this row was never actually flagged."**
  - `f.SetCellValue(sheet, "Y99999", ...)` (a row far outside the sheet's real data) also returns `nil` — **excelize does not validate row bounds.** Together with the previous point, this means `excelwriter.ConfirmPrice`'s OWN explicit "does `Y{row}` currently have a comment" check (Task 1) is the ONLY thing standing between a stale/bad `row` argument and either a silent no-op or a nonsense write far outside the real sheet — it is not optional defensive code, it is the actual safety mechanism.
- **`OrderRow.PriceMismatchDetails` is JSON-serialized to the frontend** (`json:"priceMismatchDetails"`) — unlike `SkuLog` (`json:"-"`), which is deliberately NOT sent as part of `process:row` (see `types.go`'s existing comment on `SkuLog`).
- Run `go build ./...`, `go vet ./...`, and the relevant `go test` scope after every task, from the `GO/` directory. After Task 4, also run `cd GO/frontend && npx tsc --noEmit`.
- New Go code: every exported function gets a doc comment. Every deviation from an existing established pattern in this codebase gets an inline comment explaining why (matching this project's own established convention, visible throughout every `*_processor.go` file already in this repo).

---

### Task 1: `excelwriter.ConfirmPrice`

**Files:**
- Modify: `GO/internal/processing/excelwriter/dondathang.go`
- Test: `GO/internal/processing/excelwriter/dondathang_test.go`

**Interfaces:**
- Consumes: nothing new (uses `excelize.File` methods directly: `OpenFile`, `GetComments`, `DeleteComment`, `SetCellStyle`, `SetCellValue`, `Save`).
- Produces: `func ConfirmPrice(path string, row int, price float64) error` — used by Task 2's `App.ConfirmPrice`.

This task DOES change `WriteOrderRows`'s signature (Step 3 below) — it is additive in the sense that it adds a new function and a new return value, not in the sense of leaving every existing call site untouched. This task's OWN 3 call sites (all in `dondathang_test.go`, fixed in Step 4) keep the `excelwriter` package itself compiling and its own tests passing throughout this task — but the 9 vendor `*_processor.go` files elsewhere in the repo still call the OLD single-return form until Task 3, so `go build ./...` for the WHOLE repo stays red until Task 3 lands (see Step 7 below and this plan's Global Constraints).

- [ ] **Step 1: Write the failing tests**

Add to `GO/internal/processing/excelwriter/dondathang_test.go`:

```go
func TestConfirmPrice_OverwritesValueAndClearsMismatchFlag(t *testing.T) {
	path := copyTestWorkbook(t)
	rows := []Row{
		{SKU: "3564270-4", Qty: 24, UnitPrice: 33000, InvoicePrice: 33726, PriceMismatch: true, UseZFormula: true},
	}
	if _, err := WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	if err := ConfirmPrice(path, 9, 33726); err != nil {
		t.Fatalf("ConfirmPrice returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()

	val, err := f.GetCellValue("Don dat hang", "Y9")
	if err != nil {
		t.Fatalf("GetCellValue: %v", err)
	}
	if val != "33726" {
		t.Fatalf("Y9 = %q, want %q", val, "33726")
	}

	styleID, err := f.GetCellStyle("Don dat hang", "Y9")
	if err != nil {
		t.Fatalf("GetCellStyle: %v", err)
	}
	if styleID != 0 {
		t.Fatalf("Y9 style = %d, want 0 (red-fill mismatch style cleared)", styleID)
	}

	comments, err := f.GetComments("Don dat hang")
	if err != nil {
		t.Fatalf("GetComments: %v", err)
	}
	for _, c := range comments {
		if c.Cell == "Y9" {
			t.Fatalf("comment still present at Y9 after ConfirmPrice: %+v", c)
		}
	}
}

func TestConfirmPrice_RejectsRowWithNoMismatchComment(t *testing.T) {
	path := copyTestWorkbook(t)
	rows := []Row{
		{SKU: "3564270-4", Qty: 24, UnitPrice: 33726, ProductName: "Chai tay toilet", UseZFormula: true},
	}
	if _, err := WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	err := ConfirmPrice(path, 9, 30000)
	if err == nil {
		t.Fatal("ConfirmPrice returned nil error, want a rejection — row 9 was never flagged as a price mismatch")
	}

	f, err2 := excelize.OpenFile(path)
	if err2 != nil {
		t.Fatalf("failed reopening workbook: %v", err2)
	}
	defer f.Close()
	val, _ := f.GetCellValue("Don dat hang", "Y9")
	if val != "33726" {
		t.Fatalf("Y9 = %q, want unchanged %q (rejected before any write)", val, "33726")
	}
}

func TestConfirmPrice_RejectsRowOutsideSheetBounds(t *testing.T) {
	path := copyTestWorkbook(t)
	rows := []Row{
		{SKU: "3564270-4", Qty: 24, UnitPrice: 33000, InvoicePrice: 33726, PriceMismatch: true, UseZFormula: true},
	}
	if _, err := WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	if err := ConfirmPrice(path, 99999, 33726); err == nil {
		t.Fatal("ConfirmPrice returned nil error for a row far outside the real sheet, want a rejection")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/excelwriter/... -run TestConfirmPrice -v`
Expected: FAIL with `undefined: ConfirmPrice` (compile error — the function doesn't exist yet).

- [ ] **Step 3: Write the implementation**

Add to `GO/internal/processing/excelwriter/dondathang.go` (after `WriteOrderRows`, before `writeRow`):

```go
// ConfirmPrice overwrites the price (column Y) of a row that
// WriteOrderRows already wrote and flagged as a price mismatch —
// clearing the red-fill style and the "Kiểm tra lại giá mã này!"
// comment, since the user has now explicitly reviewed and decided
// which price to keep (the PO's own invoice price, or the system's
// computed price — the caller passes whichever one it wants written).
//
// Requires Y{row} to currently carry a mismatch comment. excelize does
// NOT validate row bounds on SetCellValue (confirmed empirically: it
// silently writes far outside a sheet's real data rather than
// erroring), and DeleteComment on an uncommented cell is a silent
// no-op rather than an error — so this function's own explicit
// "does a comment exist at Y{row}" check is the ONLY thing that
// rejects a stale or out-of-range row argument. Without it, a bad
// `row` value would either silently do nothing (DeleteComment) or
// silently create a nonsense cell far outside the real sheet
// (SetCellValue) instead of surfacing as an error.
func ConfirmPrice(path string, row int, price float64) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("excelwriter: open %s: %w", path, err)
	}
	defer f.Close()

	cell := fmt.Sprintf("Y%d", row)

	comments, err := f.GetComments(sheetName)
	if err != nil {
		return fmt.Errorf("excelwriter: read comments: %w", err)
	}
	hasMismatchComment := false
	for _, c := range comments {
		if c.Cell == cell {
			hasMismatchComment = true
			break
		}
	}
	if !hasMismatchComment {
		return fmt.Errorf("excelwriter: %s không còn ở trạng thái chờ xác nhận giá (không có comment cảnh báo sai giá)", cell)
	}

	if err := f.SetCellValue(sheetName, cell, price); err != nil {
		return fmt.Errorf("excelwriter: set %s: %w", cell, err)
	}
	if err := f.DeleteComment(sheetName, cell); err != nil {
		return fmt.Errorf("excelwriter: delete comment at %s: %w", cell, err)
	}
	if err := f.SetCellStyle(sheetName, cell, cell, 0); err != nil {
		return fmt.Errorf("excelwriter: reset style at %s: %w", cell, err)
	}

	if err := f.Save(); err != nil {
		return fmt.Errorf("excelwriter: save %s: %w", path, err)
	}
	return nil
}
```

Change `WriteOrderRows`'s signature and its two `return` statements (`GO/internal/processing/excelwriter/dondathang.go:67-105`):

```go
func WriteOrderRows(path string, rows []Row, headerDescription string) (startRow int, err error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return 0, fmt.Errorf("excelwriter: open %s: %w", path, err)
	}
	defer f.Close()

	existingRows, err := f.GetRows(sheetName)
	if err != nil {
		return 0, fmt.Errorf("excelwriter: read %s: %w", sheetName, err)
	}
	currentRow := len(existingRows) + 1
	firstRow := currentRow

	redFill, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"FF0000"}, Pattern: 1},
	})
	if err != nil {
		return 0, fmt.Errorf("excelwriter: create red fill style: %w", err)
	}

	for _, row := range rows {
		if err := writeRow(f, currentRow, row, redFill); err != nil {
			return 0, err
		}
		currentRow++
	}

	if headerDescription != "" {
		if err := f.SetCellValue(sheetName, fmt.Sprintf("L%d", firstRow), headerDescription); err != nil {
			return 0, fmt.Errorf("excelwriter: set header description: %w", err)
		}
	}

	if err := f.Save(); err != nil {
		return 0, fmt.Errorf("excelwriter: save %s: %w", path, err)
	}
	return firstRow, nil
}
```

Note the doc comment above `Row` (`dondathang.go:11-16`) and above `WriteOrderRows` (`dondathang.go:61-66`) don't need changes — they don't describe the return type.

- [ ] **Step 4: Fix `WriteOrderRows`'s 3 existing call sites in this same test file**

`GO/internal/processing/excelwriter/dondathang_test.go` has 3 pre-existing calls that only capture `error` — update each to also capture (and ignore via `_`, since these 3 existing tests don't need it) the new first return value:

Line 33: `if err := WriteOrderRows(path, rows, "COOPMART PO102945235-00 (Tổng trọng lượng: 4.32 kg)"); err != nil {` → `if _, err := WriteOrderRows(path, rows, "COOPMART PO102945235-00 (Tổng trọng lượng: 4.32 kg)"); err != nil {`

Line 65: `if err := WriteOrderRows(path, rows, ""); err != nil {` → `if _, err := WriteOrderRows(path, rows, ""); err != nil {`

Line 91: `if err := WriteOrderRows(path, rows, ""); err != nil {` → `if _, err := WriteOrderRows(path, rows, ""); err != nil {`

(The 3 new tests from Step 1 already use `if _, err := WriteOrderRows(...)` correctly.)

- [ ] **Step 5: Add a `startRow` assertion to the existing basic test**

In `TestWriteOrderRows_WritesColumnsAndFormula` (`dondathang_test.go:25-55`), change the call and add an assertion:

```go
	startRow, err := WriteOrderRows(path, rows, "COOPMART PO102945235-00 (Tổng trọng lượng: 4.32 kg)")
	if err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}
	if startRow != 9 {
		t.Fatalf("startRow = %d, want 9 (the test template has 8 existing header rows)", startRow)
	}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/excelwriter/... -v`
Expected: PASS — all tests in the package, including the 3 new `TestConfirmPrice_*` tests and the updated `TestWriteOrderRows_WritesColumnsAndFormula`.

- [ ] **Step 7: Confirm the rest of the repo still builds**

Run: `cd GO && go build ./... 2>&1 | head -20`
Expected: FAIL — every vendor `*_processor.go` file still calls the OLD single-return-value form of `WriteOrderRows`. This is expected and gets fixed in Task 3, not here (Global Constraints explains why this can't be split). Confirm the failures are ONLY `WriteOrderRows`-related "assignment mismatch" errors in `*_processor.go` files, nothing else — if anything else fails, stop and investigate before proceeding.

- [ ] **Step 8: Commit**

```bash
cd "GO" && git add internal/processing/excelwriter/dondathang.go internal/processing/excelwriter/dondathang_test.go
git commit -m "feat(go): add excelwriter.ConfirmPrice, WriteOrderRows now returns startRow"
```

(The repo will not fully build again until Task 3 lands — this is expected and matches Global Constraints; do not attempt to make this single commit build clean on its own.)

---

### Task 2: `App.excelPath` + `App.ConfirmPrice`

**Files:**
- Modify: `GO/app.go`
- Test: `GO/app_test.go`

**Interfaces:**
- Consumes: `excelwriter.ConfirmPrice(path string, row int, price float64) error` (Task 1).
- Produces: `func (a *App) ConfirmPrice(row int, price float64) error` — Wails auto-generates `wailsjs/go/main/App.ConfirmPrice` for the frontend the next time `wails dev`/`wails build` runs (Task 4 depends on this binding existing).

- [ ] **Step 1: Write the failing test**

Add to `GO/app_test.go`:

```go
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
```

Add the two new imports this test needs to `app_test.go`'s existing `import (...)` block (`app_test.go:1-13`): `"github.com/xuri/excelize/v2"` and `"order-processor/internal/processing/excelwriter"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd GO && go test . -run TestApp_ConfirmPrice -v`
Expected: FAIL — `app.go`'s `App` struct has no `excelPath` field and no `ConfirmPrice` method yet (compile error).

- [ ] **Step 3: Write the implementation**

In `GO/app.go`, add `excelPath` to the `App` struct (`app.go:38-45`):

```go
type App struct {
	ctx        context.Context
	cfg        *config.Store
	processor  processing.Processor
	emitter    Emitter
	orderDir   string
	excelPath  string
	processing atomic.Bool
}
```

Set it in `NewApp()` (`app.go:91-106`), reusing the exact same value already passed to `RealProcessor.ExcelPath`:

```go
func NewApp() (*App, error) {
	store, err := productdata.Load(resolveRepoFile("data.xlsx"))
	if err != nil {
		return nil, fmt.Errorf("app: load data.xlsx: %w", err)
	}

	excelPath := resolveRepoFile("dondathang_test.xlsx")

	return &App{
		cfg: config.NewStore(configFileName),
		processor: &processing.RealProcessor{
			Store:     store,
			Pricing:   pricing.NewHTTPSource(resolveRepoFile("settings.ini")),
			ExcelPath: excelPath,
		},
		orderDir:  orderFolderName,
		excelPath: excelPath,
	}, nil
}
```

Add the new method (after `SetSTT`, `app.go:127-130`, matching that pattern exactly):

```go
// ConfirmPrice ghi đè giá (cột Y) của một dòng sản phẩm đã bị đánh dấu
// sai giá, theo lựa chọn của người dùng — giữ giá trên PO hoặc dùng giá
// hệ thống. Yêu cầu dòng đó ĐANG ở trạng thái chờ xác nhận (còn comment
// cảnh báo); nếu không sẽ trả lỗi thay vì âm thầm ghi đè.
func (a *App) ConfirmPrice(row int, price float64) error {
	return excelwriter.ConfirmPrice(a.excelPath, row, price)
}
```

Add the new import to `app.go`'s existing `import (...)` block (`app.go:3-18`): `"order-processor/internal/processing/excelwriter"`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd GO && go test . -run TestApp_ConfirmPrice -v`
Expected: PASS.

- [ ] **Step 5: Run the full `main` package test suite**

Run: `cd GO && go test . -v`
Expected: PASS for every test in the package, including the pre-existing `TestRunBatch_*`/`TestResolveRepoFile_*` tests — this task's changes are additive to `App` and don't touch `runBatch`/`resolveRepoFile`.

- [ ] **Step 6: Commit**

```bash
cd "GO" && git add app.go app_test.go
git commit -m "feat(go): add App.excelPath + App.ConfirmPrice Wails method"
```

(`go build ./...` for the WHOLE repo still fails at this point — same reason as Task 1, fixed in Task 3.)

---

### Task 3: `PriceMismatchDetail` + wire it into all 9 vendors (the atomic task)

**Files:**
- Modify: `GO/internal/processing/types.go`
- Modify: `GO/internal/processing/coop_processor.go`
- Modify: `GO/internal/processing/lotte_processor.go`
- Modify: `GO/internal/processing/satra_processor.go`
- Modify: `GO/internal/processing/bigc_processor.go`
- Modify: `GO/internal/processing/winmart_processor.go`
- Modify: `GO/internal/processing/emart_processor.go`
- Modify: `GO/internal/processing/fujimart_processor.go`
- Modify: `GO/internal/processing/kingfood_processor.go`
- Modify: `GO/internal/processing/jmart_processor.go`
- Test: `GO/internal/processing/jmart_processor_test.go`

**Interfaces:**
- Consumes: `excelwriter.WriteOrderRows`'s new `(startRow int, err error)` return (Task 1).
- Produces: `OrderRow.PriceMismatchDetails []PriceMismatchDetail` — consumed by Task 4's frontend.

This is the ONE task where the repo goes from "doesn't build" back to "builds and all tests pass" — see Global Constraints for why it cannot be split by vendor.

- [ ] **Step 1: Add the `PriceMismatchDetail` struct and `OrderRow` field**

In `GO/internal/processing/types.go`, add above `OrderRow`:

```go
// PriceMismatchDetail is ONE product flagged as a price mismatch —
// surfaced so the user can review it after processing and choose, per
// SKU, whether to keep the PO's own invoice price or the system's
// computed price (see excelwriter.ConfirmPrice). ExcelRow is the real
// row number in the "Don dat hang" sheet this product was written to —
// required to edit the right cell later, since nothing else in the
// returned OrderRow identifies individual product rows.
type PriceMismatchDetail struct {
	SKU          string  `json:"sku"`
	ProductName  string  `json:"productName"`
	InvoicePrice float64 `json:"invoicePrice"`
	SystemPrice  float64 `json:"systemPrice"`
	ExcelRow     int     `json:"excelRow"`
}
```

Add the field to `OrderRow` (`types.go`, right after the existing `SkuLog` field):

```go
	// PriceMismatchDetails holds one entry per product flagged as a
	// price mismatch in this row's file/page — unlike SkuLog, this IS
	// serialized to JSON (sent to the frontend as part of "process:row"),
	// since the frontend needs it to let the user review/resolve each
	// mismatch after processing.
	PriceMismatchDetails []PriceMismatchDetail `json:"priceMismatchDetails"`
```

- [ ] **Step 2: JMart — collect mismatch details and compute `ExcelRow`**

`GO/internal/processing/jmart_processor.go`:

Add `var mismatchDetails []PriceMismatchDetail` next to the existing `var skuLog []string` (line 87):

```go
	saigia := 0
	totalWeight := 0.0
	totalValue := 0.0
	var skuLog []string
	var mismatchDetails []PriceMismatchDetail
```

In the `if !matched` block (lines 145-149), add the detail collection:

```go
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
			mismatchDetails = append(mismatchDetails, PriceMismatchDetail{
				SKU: barcode, ProductName: productInfo.Name,
				InvoicePrice: invoicePrice, SystemPrice: finalPrice,
				ExcelRow: productRowIndex,
			})
		}
```

Note `productRowIndex` isn't assigned until the line right after this block (`productRowIndex := len(rows)`, line 151) — reorder so the detail collection happens AFTER that assignment. The full corrected block, replacing lines 145-153:

```go
		productRowIndex := len(rows)
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
			mismatchDetails = append(mismatchDetails, PriceMismatchDetail{
				SKU: barcode, ProductName: productInfo.Name,
				InvoicePrice: invoicePrice, SystemPrice: finalPrice,
				ExcelRow: productRowIndex,
			})
		}
		rows = append(rows, productRow)
		totalValue += finalPrice * ouQty
```

(This moves `productRowIndex := len(rows)` one line earlier than it currently sits — before the `if !matched` check instead of after — so it's available for the mismatch detail. Its value is identical either way, since nothing between the old and new position changes `len(rows)`.)

Change the `WriteOrderRows` call (line 231) to capture `startRow`, and add the row-number fixup right after:

```go
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}
```

Add `PriceMismatchDetails: mismatchDetails` to the final `return OrderRow{...}` (line 242-246):

```go
	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "JMart", MaKhachHang: jmartCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

- [ ] **Step 3: Kingfood, Winmart, FujiMart — same pattern, `khuyenmai`-named vendors**

These 3 files share JMart's exact shape (`productRowIndex := len(rows)` currently sits immediately after the `if !matched` block, `formatSkuLogLine` uses `khuyenmai`/`khuyenmaiColumn`). Apply the IDENTICAL transformation as Step 2 to each:

`GO/internal/processing/kingfood_processor.go` — `var mismatchDetails []PriceMismatchDetail` next to `var skuLog []string` (near line 111); reorder `productRowIndex := len(rows)` (currently line 184) to sit BEFORE the `if !matched` block (currently starts line 178) and add mismatch-detail collection inside it; `startRow, err := excelwriter.WriteOrderRows(...)` at line 275 + fixup loop; add `PriceMismatchDetails: mismatchDetails` to the return (`return OrderRow{` at line 286, `SkuLog: skuLog, PriceMismatchCount: saigia,` at line 289).

`GO/internal/processing/winmart_processor.go` — same, `var mismatchDetails` near line 174, reorder `productRowIndex := len(rows)` (currently line 251) before the `if !matched` block (currently starts line 245), `WriteOrderRows` call at line 322 + fixup, return (`return OrderRow{` at line 333, `SkuLog: skuLog, PriceMismatchCount: saigia,` at line 336).

`GO/internal/processing/fujimart_processor.go` — same, `var mismatchDetails` near line 83, reorder `productRowIndex := len(rows)` (currently line 162) before the `if !matched` block (currently starts line 156), `WriteOrderRows` call at line 244 + fixup, return (`return OrderRow{` at line 255, `SkuLog: skuLog, PriceMismatchCount: saigia,` at line 258).

For all 3, the `if !matched { ... }` block becomes exactly:

```go
		productRowIndex := len(rows)
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
			mismatchDetails = append(mismatchDetails, PriceMismatchDetail{
				SKU: barcode, ProductName: productInfo.Name,
				InvoicePrice: invoicePrice, SystemPrice: finalPrice,
				ExcelRow: productRowIndex,
			})
		}
		rows = append(rows, productRow)
```

(`barcode` — every one of these 3 files already names its resolved-SKU variable `barcode` at this point, same as JMart.)

- [ ] **Step 4: Coop, Lotte, Satra, Emart — same pattern, `lastExaminedPromo`-named vendors**

These 4 files use `lastExaminedPromo`/`lastExaminedPromoColumn` instead of `khuyenmai`/`khuyenmaiColumn`, but the mismatch-detail collection itself doesn't reference either variable, so the transformation is identical to Step 3:

`GO/internal/processing/coop_processor.go` — `var mismatchDetails []PriceMismatchDetail` near line 272 (next to `var skuLog []string`); reorder `productRowIndex := len(rows)` (currently line 360) before the `if !matched` block (currently starts line 354, right after `saigia++`); the loop variable is `product.Barcode` (not a bare `barcode`) and `productInfo` — use `product.Barcode` for `SKU:`. `WriteOrderRows` call at line 408 + fixup. Return (`return OrderRow{` at line 419, `SkuLog: skuLog, PriceMismatchCount: saigia,` at line 422).

`GO/internal/processing/lotte_processor.go` — `var mismatchDetails` near line 99, reorder `productRowIndex := len(rows)` (currently line 163) before `if !matched` (currently starts line 157), uses `barcode`. `WriteOrderRows` call at line 230 + fixup. Return (`return OrderRow{` at line 241, `SkuLog: skuLog, PriceMismatchCount: saigia,` at line 244).

`GO/internal/processing/satra_processor.go` — `var mismatchDetails` near line 147, reorder `productRowIndex := len(rows)` (currently line 228) before `if !matched` (currently starts line 222), uses `barcode`. `WriteOrderRows` call at line 269 + fixup. Return (`return OrderRow{` at line 280, `SkuLog: skuLog, PriceMismatchCount: saigia,` at line 283).

`GO/internal/processing/emart_processor.go` — `var mismatchDetails` near line 123, reorder `productRowIndex := len(rows)` (currently line 201) before `if !matched` (currently starts line 195), uses `barcode`. `WriteOrderRows` call at line 297 + fixup. Return (`return OrderRow{` at line 311, `SkuLog: skuLog, PriceMismatchCount: saigia,` at line 314) — note: Emart's status also has an OR-condition for `storeName == ""` — do not touch that logic, only add `PriceMismatchDetails: mismatchDetails` to the returned struct literal.

For Coop specifically, the `if !matched` block becomes:

```go
		productRowIndex := len(rows)
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
			mismatchDetails = append(mismatchDetails, PriceMismatchDetail{
				SKU: product.Barcode, ProductName: productInfo.Name,
				InvoicePrice: invoicePrice, SystemPrice: finalPrice,
				ExcelRow: productRowIndex,
			})
		}
		rows = append(rows, productRow)
```

For Lotte, Satra, and Emart (all three use a bare `barcode` variable, same as Step 3's group):

```go
		productRowIndex := len(rows)
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
			mismatchDetails = append(mismatchDetails, PriceMismatchDetail{
				SKU: barcode, ProductName: productInfo.Name,
				InvoicePrice: invoicePrice, SystemPrice: finalPrice,
				ExcelRow: productRowIndex,
			})
		}
		rows = append(rows, productRow)
```

- [ ] **Step 5: BigC — the one genuinely different vendor**

`GO/internal/processing/bigc_processor.go` needs 3 distinct changes, because it combines every store page's rows into ONE `WriteOrderRows` call. See Global Constraints for why this needs a 3-step (not 2-step) row-number fixup.

**5a. `storePageResult` gets a new field** (`bigc_processor.go:62-69`):

```go
type storePageResult struct {
	rows            []excelwriter.Row
	weightKg        float64
	saigia          int
	tongtien        float64
	skuLog          []string
	mismatchDetails []PriceMismatchDetail
	err             error
}
```

**5b. `processBigcStorePage` collects details, adding `productRowIndex` fresh** (this file has never needed that variable before — see Global Constraints). Add `var mismatchDetails []PriceMismatchDetail` next to the existing `var skuLog []string` (`bigc_processor.go:215`):

```go
	var weightKg, tongtien float64
	saigia := 0
	var skuLog []string
	var mismatchDetails []PriceMismatchDetail
```

Change the `if !matched` block and the `rows = append` line right after it (`bigc_processor.go:368-373`) from:

```go
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
		}
		rows = append(rows, productRow)
```

to:

```go
		productRowIndex := len(rows)
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
			mismatchDetails = append(mismatchDetails, PriceMismatchDetail{
				SKU: barcode, ProductName: productInfo.Name,
				InvoicePrice: invoicePrice, SystemPrice: finalPrice,
				ExcelRow: productRowIndex,
			})
		}
		rows = append(rows, productRow)
```

Change the function's final `return` (`bigc_processor.go:452`) from:

```go
	return storePageResult{rows: rows, weightKg: weightKg, saigia: saigia, tongtien: tongtien, skuLog: skuLog}
```

to:

```go
	return storePageResult{rows: rows, weightKg: weightKg, saigia: saigia, tongtien: tongtien, skuLog: skuLog, mismatchDetails: mismatchDetails}
```

**5c. `processBigcDocument` does the cross-store offset, then the final `startRow` offset.** Change the per-store loop body (`bigc_processor.go:110-140`) — specifically, insert the cross-store adjustment BEFORE `allRows = append(allRows, result.rows...)`, and add `PriceMismatchDetails` to the per-store `OrderRow`:

```go
	for pageIdx := 1; pageIdx < len(pageTexts); pageIdx++ {
		pageLabel := fmt.Sprintf("%d/%d", pageIdx+1, len(pageTexts))
		result := p.processBigcStorePage(pageTexts[pageIdx], priceList, priceIndex, orderNum, entryDate, cancelDate,
			customerCode, deliveryWarehouse, description, warehouse, region, statCode, !headerWritten)

		if result.err != nil {
			orderRows = append(orderRows, OrderRow{
				FileName: filepath.Base(filePath), Page: pageLabel, System: "BigC",
				Status: fmt.Sprintf("%s - %v", StatusFailed, result.err), StatusKind: StatusKindFailed,
			})
			continue
		}

		if !headerWritten {
			headerWritten = true
		}

		// Adjust each detail's ExcelRow from "index within THIS store's
		// own local rows" to "index within the combined allRows slice"
		// by adding how many rows every earlier successful store already
		// contributed — snapshotted as len(allRows) BEFORE this store's
		// own rows are appended below. The final absolute Excel row
		// (adding startRow, common to every store in this one combined
		// write) is applied once, after WriteOrderRows returns, below.
		for i := range result.mismatchDetails {
			result.mismatchDetails[i].ExcelRow += len(allRows)
		}

		allRows = append(allRows, result.rows...)
		totalWeight += result.weightKg

		statusKind := StatusKindDone
		statusText := StatusDone
		if result.saigia > 0 {
			statusKind = StatusKindWarning
			statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, result.saigia)
		}
		orderRows = append(orderRows, OrderRow{
			FileName: filepath.Base(filePath), Page: pageLabel, System: "BigC", MaKhachHang: customerCode,
			PO: poNumber, DonGia: fmt.Sprintf("%.0f", result.tongtien), Status: statusText, StatusKind: statusKind,
			SkuLog: result.skuLog, PriceMismatchCount: result.saigia, PriceMismatchDetails: result.mismatchDetails,
		})
	}
```

Change the final `WriteOrderRows` call block (`bigc_processor.go:142-147`) from:

```go
	if len(allRows) > 0 {
		headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
		if err := excelwriter.WriteOrderRows(p.ExcelPath, allRows, headerDescription); err != nil {
			return nil, err
		}
	}
```

to:

```go
	if len(allRows) > 0 {
		headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
		startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, allRows, headerDescription)
		if err != nil {
			return nil, err
		}
		for i := range orderRows {
			for j := range orderRows[i].PriceMismatchDetails {
				orderRows[i].PriceMismatchDetails[j].ExcelRow += startRow
			}
		}
	}
```

- [ ] **Step 6: Build the whole repo**

Run: `cd GO && go build ./...`
Expected: clean (no output). This is the first point since Task 1 where the whole repo compiles again.

- [ ] **Step 7: `go vet` and the full test suite**

Run: `cd GO && go vet ./...`
Expected: clean.

Run: `cd GO && go test ./...`
Expected: every package passes EXCEPT the root `processing` package's `TestRealProcessor_MatchesGoldenFixtures` (Coop's own golden test) — that failure is pre-existing (a diagnosed, already-documented gap unrelated to this work, root-caused to shared PDF-library reliability issues — see project history) and must show the SAME mismatch count/signatures as before this task, not new or different ones. Every other vendor's golden test (`_BigC`, `_Emart`, `_Fujimart`, `_JMart`, `_Kingfood`, `_Lotte`, `_Satra`, `_Winmart`) must pass. If Coop's failure count or signatures changed, or if it changed from failing to passing, stop and investigate — this task's changes are meant to be purely additive.

- [ ] **Step 8: Extend the JMart real-sample test to verify `PriceMismatchDetails` end-to-end**

`GO/internal/processing/jmart_processor_test.go` already has `TestRealProcessor_ProcessesRealSampleJMartFile`, which uses a `pricingSource` with no real price data — so all 3 real products in `testdata/sample_jmart_order.pdf` legitimately mismatch (confirmed by this test's own existing `PriceMismatchCount != 3` assertion). Add, right after the existing `PriceMismatchCount` check near the end of that test function:

```go
	// PriceMismatchDetails: same 3 real mismatches, now as structured
	// per-SKU detail — verify not just the computed values but that
	// ExcelRow genuinely points at the real cell excelwriter flagged
	// (comment + non-default style), by reopening the written workbook
	// directly rather than trusting the arithmetic alone.
	if len(rows[0].PriceMismatchDetails) != 3 {
		t.Fatalf("len(PriceMismatchDetails) = %d, want 3", len(rows[0].PriceMismatchDetails))
	}
	firstDetail := rows[0].PriceMismatchDetails[0]
	if firstDetail.SKU != "8936156730886" {
		t.Errorf("PriceMismatchDetails[0].SKU = %q, want %q", firstDetail.SKU, "8936156730886")
	}
	if firstDetail.SystemPrice != 0 {
		t.Errorf("PriceMismatchDetails[0].SystemPrice = %v, want 0 (this test's pricingSource has no real price data)", firstDetail.SystemPrice)
	}
	if firstDetail.InvoicePrice <= 0 {
		t.Errorf("PriceMismatchDetails[0].InvoicePrice = %v, want a real positive extracted price", firstDetail.InvoicePrice)
	}

	fVerify, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening workbook for ExcelRow verification: %v", err)
	}
	defer fVerify.Close()
	verifyCell := fmt.Sprintf("Y%d", firstDetail.ExcelRow)
	styleID, err := fVerify.GetCellStyle("Don dat hang", verifyCell)
	if err != nil {
		t.Fatalf("GetCellStyle(%s): %v", verifyCell, err)
	}
	if styleID == 0 {
		t.Errorf("ExcelRow=%d (cell %s) has default style, want the red-fill mismatch style — ExcelRow doesn't point at the real flagged cell", firstDetail.ExcelRow, verifyCell)
	}
	comments, err := fVerify.GetComments("Don dat hang")
	if err != nil {
		t.Fatalf("GetComments: %v", err)
	}
	foundComment := false
	for _, c := range comments {
		if c.Cell == verifyCell {
			foundComment = true
		}
	}
	if !foundComment {
		t.Errorf("no mismatch comment found at %s — ExcelRow=%d doesn't point at the real flagged cell", verifyCell, firstDetail.ExcelRow)
	}
```

Add `"fmt"` to this test file's imports if not already present (check the existing `import (...)` block at the top of `jmart_processor_test.go` first — it currently imports `"context"`, `"strings"`, `"testing"`, `"github.com/xuri/excelize/v2"`, `"order-processor/internal/processing/pricing"`, `"order-processor/internal/processing/productdata"`; add `"fmt"` to that list).

- [ ] **Step 9: Run the extended test**

Run: `cd GO && go test ./internal/processing/ -run TestRealProcessor_ProcessesRealSampleJMartFile -v`
Expected: PASS.

- [ ] **Step 10: Full suite one more time**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: same result as Step 7 (only the pre-existing Coop gap failing, everything else green).

- [ ] **Step 11: Commit**

```bash
cd "GO" && git add internal/processing/types.go internal/processing/coop_processor.go internal/processing/lotte_processor.go internal/processing/satra_processor.go internal/processing/bigc_processor.go internal/processing/winmart_processor.go internal/processing/emart_processor.go internal/processing/fujimart_processor.go internal/processing/kingfood_processor.go internal/processing/jmart_processor.go internal/processing/jmart_processor_test.go
git commit -m "feat(go): collect PriceMismatchDetail (with real Excel row) across all 9 vendors"
```

---

### Task 4: Frontend — expandable mismatch review + apply buttons

**Files:**
- Modify: `GO/frontend/src/types.ts`
- Modify: `GO/frontend/src/components/ResultTable.tsx`

**Interfaces:**
- Consumes: `OrderRow.priceMismatchDetails` (JSON field from Task 3, camelCase per Go's `json:"priceMismatchDetails"` tag), `ConfirmPrice(row: number, price: number): Promise<void>` (Wails binding auto-generated from Task 2's `App.ConfirmPrice`, appears at `GO/frontend/wailsjs/go/main/App.d.ts`/`.js` the next time `wails dev` or `wails build` runs).
- Produces: nothing consumed by a later task — this is the last task.

- [ ] **Step 1: Regenerate Wails bindings**

Run: `cd GO && wails dev -browser -loglevel Error` — start it, wait for "Vite Server URL" and "Watching (sub)/directory" to appear in the log (confirms bindings were generated), then stop it (Ctrl+C, or kill the process if run in the background). This is required before Step 3 — without it, `import { ConfirmPrice } from '../../wailsjs/go/main/App'` won't resolve.

Verify: `grep -n "ConfirmPrice" GO/frontend/wailsjs/go/main/App.d.ts` — Expected: a line declaring `export function ConfirmPrice(arg1:number,arg2:number):Promise<void>;` (or equivalent — confirm it exists, exact formatting may vary by Wails version).

- [ ] **Step 2: Add the TypeScript type**

In `GO/frontend/src/types.ts`, add above `OrderRow`:

```typescript
export interface PriceMismatchDetail {
  sku: string
  productName: string
  invoicePrice: number
  systemPrice: number
  excelRow: number
}
```

Add the field to `OrderRow`:

```typescript
export interface OrderRow {
  fileName: string
  page: string
  system: string
  maKhachHang: string
  po: string
  donGia: string
  status: string
  statusKind: string
  priceMismatchCount: number
  priceMismatchDetails: PriceMismatchDetail[]
}
```

- [ ] **Step 3: Add the expandable detail row to `ResultTable.tsx`**

Read the current file first — it already has `useState` for `copiedKey`, a `columns` array including a `priceMismatchCount` column with `priceMeta()`, and per-cell click-to-copy with `stopPropagation` NOT currently needed there (the cell's own `onClick` handles copy; this task adds a SEPARATE clickable element inside that same cell that needs its own `stopPropagation` so clicking it doesn't ALSO trigger the cell's copy handler).

Add imports (`GO/frontend/src/components/ResultTable.tsx`, top of file):

```typescript
import { useState } from 'react'
import { FaCircle, FaCheck, FaCircleCheck, FaTriangleExclamation, FaChevronDown, FaChevronRight } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import type { OrderRow } from '../types'
import { SectionHeader } from './SectionHeader'
import { ConfirmPrice } from '../../wailsjs/go/main/App'
```

Add local state for which row is expanded and which SKUs have been resolved this session, inside the `ResultTable` function, alongside the existing `copiedKey` state:

```typescript
  const [copiedKey, setCopiedKey] = useState<string | null>(null)
  const [expandedRow, setExpandedRow] = useState<number | null>(null)
  const [resolvedChoice, setResolvedChoice] = useState<Record<string, 'po' | 'system'>>({})
  const appendLog = useAppStore((s) => s.appendLog)
```

Add the apply handler, alongside the existing `handleCopy` function:

```typescript
  async function handleApplyPrice(rowIndex: number, detail: PriceMismatchDetail, useInvoicePrice: boolean) {
    const price = useInvoicePrice ? detail.invoicePrice : detail.systemPrice
    const key = `${rowIndex}-${detail.excelRow}`
    try {
      await ConfirmPrice(detail.excelRow, price)
      setResolvedChoice((prev) => ({ ...prev, [key]: useInvoicePrice ? 'po' : 'system' }))
    } catch (err) {
      appendLog(`❌ Lỗi áp dụng giá cho ${detail.sku}: ${String(err)}`)
    }
  }
```

Add `PriceMismatchDetail` to the existing type import (`import type { OrderRow } from '../types'` becomes `import type { OrderRow, PriceMismatchDetail } from '../types'`).

Change the price-reconciliation `<td>` rendering (the `c.key === 'priceMismatchCount'` branch) so the badge itself is clickable when there ARE mismatches, with `stopPropagation` so it doesn't also fire the cell's own copy handler:

```tsx
                        ) : c.key === 'priceMismatchCount' ? (
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation()
                              if (row.priceMismatchCount > 0) {
                                setExpandedRow((cur) => (cur === i ? null : i))
                              }
                            }}
                            className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 font-sans font-semibold ${price.classes} ${
                              row.priceMismatchCount > 0 ? 'cursor-pointer' : 'cursor-default'
                            }`}
                          >
                            {price.icon === 'ok' && <FaCircleCheck size={11} />}
                            {price.icon === 'warn' && <FaTriangleExclamation size={11} />}
                            {price.label}
                            {row.priceMismatchCount > 0 &&
                              (expandedRow === i ? <FaChevronDown size={9} /> : <FaChevronRight size={9} />)}
                          </button>
                        ) : c.key === 'donGia' ? (
```

(This REPLACES the plain `<span>` currently used for that branch — same visual badge, now a `<button>` so it's independently clickable, with the chevron only shown when there's something to expand.)

Add the expanded detail row right after the closing `</tr>` of the main row loop, still inside the `rows.map((row, i) => { ... })` callback — change the callback to return an array (main `<tr>` plus, conditionally, the detail `<tr>`) using a `<>...</>` fragment:

```tsx
            {rows.map((row, i) => {
              const meta = statusMeta(row)
              const price = priceMeta(row)
              return (
                <>
                  <tr key={i} className="transition-colors hover:bg-white/[0.03]">
                    {/* ... existing columns.map(...) block, unchanged ... */}
                  </tr>
                  {expandedRow === i && row.priceMismatchDetails.length > 0 && (
                    <tr key={`${i}-detail`} className="bg-bg/60">
                      <td colSpan={columns.length} className="p-0">
                        <table className="w-full border-collapse font-mono text-[11px]">
                          <thead>
                            <tr className="border-b border-border">
                              <th className="px-3 py-1.5 text-left font-sans font-semibold text-muted">Mã</th>
                              <th className="px-3 py-1.5 text-left font-sans font-semibold text-muted">Tên SP</th>
                              <th className="px-3 py-1.5 text-left font-sans font-semibold text-muted">Giá PO</th>
                              <th className="px-3 py-1.5 text-left font-sans font-semibold text-muted">Giá hệ thống</th>
                              <th className="px-3 py-1.5 text-left font-sans font-semibold text-muted">Áp dụng</th>
                            </tr>
                          </thead>
                          <tbody>
                            {row.priceMismatchDetails.map((detail) => {
                              const key = `${i}-${detail.excelRow}`
                              const choice = resolvedChoice[key]
                              return (
                                <tr key={key} className="border-b border-border last:border-0">
                                  <td className="px-3 py-1.5 text-ink">{detail.sku}</td>
                                  <td className="px-3 py-1.5 text-ink">{detail.productName}</td>
                                  <td className="px-3 py-1.5 text-accent">{detail.invoicePrice.toLocaleString('vi-VN')}</td>
                                  <td className="px-3 py-1.5 text-accent">{detail.systemPrice.toLocaleString('vi-VN')}</td>
                                  <td className="px-3 py-1.5">
                                    <div className="flex gap-1.5">
                                      <button
                                        type="button"
                                        onClick={() => handleApplyPrice(i, detail, true)}
                                        className={`rounded px-2 py-1 font-sans text-[10px] font-semibold transition-colors ${
                                          choice === 'po'
                                            ? 'bg-accent text-[#0a1620]'
                                            : 'border border-border text-muted hover:border-accent hover:text-accent'
                                        }`}
                                      >
                                        Dùng giá PO
                                      </button>
                                      <button
                                        type="button"
                                        onClick={() => handleApplyPrice(i, detail, false)}
                                        className={`rounded px-2 py-1 font-sans text-[10px] font-semibold transition-colors ${
                                          choice === 'system'
                                            ? 'bg-accent text-[#0a1620]'
                                            : 'border border-border text-muted hover:border-accent hover:text-accent'
                                        }`}
                                      >
                                        Dùng giá hệ thống
                                      </button>
                                    </div>
                                  </td>
                                </tr>
                              )
                            })}
                          </tbody>
                        </table>
                      </td>
                    </tr>
                  )}
                </>
              )
            })}
```

- [ ] **Step 4: Type-check**

Run: `cd GO/frontend && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 5: Build the app and manually verify**

Run: `cd GO && wails build`
Expected: builds cleanly, `GO/build/bin/order-processor.exe` produced.

Manual check (no automated frontend test framework in this project — matches how every other frontend change in this project has been verified): run the built exe (or `wails dev`), process a real file that produces at least one price mismatch (or use a pricing source that guarantees one, matching how `TestRealProcessor_ProcessesRealSampleJMartFile` does), confirm:
1. The "Đối soát giá" badge for a mismatched order shows a chevron and is clickable.
2. Clicking it expands a sub-table listing the mismatched SKU(s) with both prices shown, formatted with `vi-VN` thousand separators.
3. Clicking "Dùng giá PO" or "Dùng giá hệ thống" highlights that button and does not throw a console error.
4. Reopening the Excel file (or re-reading it programmatically) shows the Y cell now holds the chosen price, with no red fill or comment remaining — confirming the round-trip through `ConfirmPrice` actually worked end-to-end, not just that the button click didn't crash.

- [ ] **Step 6: Commit**

```bash
cd "GO" && git add frontend/src/types.ts frontend/src/components/ResultTable.tsx
git commit -m "feat(frontend): review price mismatches per SKU, apply PO or system price"
```

---

## Self-Review Notes (from the plan author, before handing off)

- **Spec coverage**: every section of the spec (data model, `WriteOrderRows` startRow, `ConfirmPrice`'s safety check, `App.excelPath`/`App.ConfirmPrice`, frontend expand/apply UI, no-confirmation-dialog, per-SKU not per-order, testing requirements) maps to a task above. No gaps found.
- **Type consistency checked**: `PriceMismatchDetail` (Go) fields (`SKU`, `ProductName`, `InvoicePrice`, `SystemPrice`, `ExcelRow`) match their JSON tags (`sku`, `productName`, `invoicePrice`, `systemPrice`, `excelRow`) match the TypeScript interface field names exactly (Task 4 Step 2) — verified by hand, not assumed.
- **`ConfirmPrice` naming collision, deliberately not a conflict**: `excelwriter.ConfirmPrice` (Task 1) and `App.ConfirmPrice` (Task 2) share a name because one calls the other directly — this mirrors the existing `excelwriter`-function-name/`App`-method-name pairing pattern already used nowhere else in this codebase, but is unambiguous since they're in different packages and the frontend only ever sees `App.ConfirmPrice` via the Wails binding.
