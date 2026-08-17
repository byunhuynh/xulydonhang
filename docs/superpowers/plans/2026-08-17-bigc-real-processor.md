# BigC RealProcessor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the existing Go `RealProcessor` (Coop, Lotte, Satra already shipped) to also parse real BigC purchase-order PDFs — a genuinely different shape from every prior vendor: page 0 is a master price/product table (no Excel write), each subsequent page is one store (write), and the whole file shares one PO/customer code — validated against 29 real archived BigC PDFs via the same golden-fixture methodology. Also pays down architectural debt flagged at Satra's final review, before a 4th vendor makes it worse.

**Architecture:** Task 0/1 first extract the vendor-neutral helpers (`regionInfo`, `closeEnough`, `buildPromoBonusRow`, `buildInvoiceBonusRow`, `coopDebtDays`, `stripBlankLines`, `xPlus1Pattern`) and golden-test helpers (`fixtureData`, `frozenPricingFixture`, `compareRowsAgainstFixture`, `joinLines`, `copyFile`, `stringify`/`toFloat`/`floatCloseEnough`, `fixturePricingSource`, `copyTestWorkbookForProcessor`) out of `coop_processor.go`/`coop_processor_test.go`/`coop_golden_test.go`/`lotte_processor.go` into new shared files, and parameterize `buildPromoBonusRow`/`buildInvoiceBonusRow`'s order-number instead of post-patching. Then a new `GO/internal/processing/bigc/` package holds pure extraction functions (PO/dates, page-0 price list, customer-code lookup, per-store name/items/price-join), and `bigc_processor.go` adds a pre-check in `RealProcessor.Process` (before the existing per-page loop) that routes an entire BigC file to `processBigcDocument`, which handles page 0 once, then every store page, accumulating all successful stores' rows into ONE combined Excel write with an aggregate weight total — a deliberate, justified simplification of Python's "re-read the sheet and overwrite a cell" mechanism, made possible because Go's `processBigcDocument` already holds the whole file's pages in memory (Python's per-page-call architecture didn't have that luxury). `write_to_dondathang_bigc`'s promo/bonus-row logic is confirmed to differ structurally from Coop/Satra (no `khuyenmai.split('|')`, no `AU` column writes, different per-item no-brace fallback text) — BigC gets its own row-builder in `bigc_processor.go`, NOT a reuse of `buildPromoBonusRow`/`buildInvoiceBonusRow`.

**Tech Stack:** Same as Phase 2a/2b — Go, `github.com/xuri/excelize/v2`, `github.com/ledongthuc/pdf`. No new external dependencies.

**Spec:** [2026-08-17-bigc-real-processor-design.md](../specs/2026-08-17-bigc-real-processor-design.md)

## Global Constraints

- **Testing/divergence policy** (same as Lotte/Satra, different from Coop): golden-fixture tests compare against real Python output, but when Go intentionally computes a different, verified-more-correct value because Python is confirmed wrong (or because Python's per-page-call mechanism doesn't translate to Go's whole-file-at-once architecture — see the header/weight-aggregation redesign above), record it in a commented `knownDivergences_BigC` allowlist (key format `sourcePDF:rowIndex:column`, never the bare `rowIndex:column` form). Never edit fixture JSON files to force a pass.
- **Error handling is a deliberate, confirmed Go-side improvement over Python, not a port**: Python has NO per-page try/except anywhere in its BigC branch (`xulydonhang.py:9404-9536`) or its enclosing page loop (`:7210`) — a single page's exception aborts every remaining page in that file, caught only once at the whole-file level in `App.py`'s `ProcessThread.run()`. Go's design: **page 0 failure → fail the whole file** (1 Failed `OrderRow`, since every store page depends on page-0 data); **a store page's failure → isolate to that page** (1 Failed `OrderRow` for that page only; all other store pages, including ones after the failed page, still process normally and their rows still combine into the successful Excel write). This is a real, confirmed behavior difference from Python — implement the isolated version, do not "fix" it to match Python's abort-on-first-error behavior.
- **Reuse, do not reimplement**: `productdata.Store.ResolveSku/GetProductInfo`, `excelwriter.Row`/`WriteOrderRows`, `pricing.Index`/`PricingSource.FetchIndex`, `coop.ExtractDiscount`. After Task 0, also reuse `regionInfo`/`closeEnough`/`stripBlankLines` from the new `processor_shared.go` — **except regionInfo does NOT correctly cover BigC's warehouse table** (see Task 6's design note — BigC needs its own 3-way region function, reusing the shared 2-way `regionInfo` here would silently corrupt Satra's already-shipped, already-tested `MN_MT_*` codes if `regionInfo` were extended instead, and would give BigC's own `MN_MT_BIGCAC` codes the wrong warehouse if `regionInfo` were reused unmodified). Do NOT reuse `buildPromoBonusRow`/`buildInvoiceBonusRow` for BigC's promo/bonus-row logic — confirmed structurally different (see Task 6).
- Every exported function gets a doc comment citing the exact `xulydonhang.py` line range it mirrors. Every deviation from a literal Python behavior gets an inline comment explaining why.
- Run `go build ./...`, `go vet ./...`, and the relevant `go test` scope after every task, from the `GO/` directory.
- **New package** `GO/internal/processing/bigc/` for BigC-only extraction, mirroring the `lotte`/`satra` package shape. **New file** `GO/internal/processing/bigc_processor.go` (+ `bigc_processor_test.go`) — never append to `coop_processor.go`/`lotte_processor.go`/`satra_processor.go`.
- **A known, not-yet-confirmed Python quirk to watch for during Task 8 (golden fixtures):** `write_to_dondathang_bigc`'s per-item bonus-row block (`xulydonhang.py:4769`) calls `ProcessHandler.laycachbo_khuyenmai(value)` using `value` — the loop variable left over from the earlier `for col, value in results:` promo-matching loop (`:4668`) — **not** `khuyenmai` (the variable that actually holds this item's resolved promo string, used everywhere else in the function, including the very next line's `sheet[f"AQ{current_row}"] = khuyenmai` at `:4757`, one line above the `kiemtra` block that leads into this). When `results` is empty for the current item, `value` silently retains whatever it was left as from a **previous item's** loop iteration (or is undefined on the very first item ever, which would raise `NameError` in Python — never observed in practice because `check_value_in_sanpham`/the `X+1` regex match rarely coincide with an empty `results` list on the first item of a file). This plan's Task 6 implements the row-builder using `khuyenmai` (the sensible, current-item value) rather than reproducing this leftover-variable bug. If Task 8's golden-fixture run shows an `AO` mismatch traceable to this specific pattern (an item with empty `results` but a truthy `kiemtra`, immediately following an item that DID have a promo match), root-cause against `:4769` before assuming it's a Go bug — if confirmed, this is exactly the kind of case `knownDivergences_BigC` exists for.

---

### Task 0: Extract shared vendor-neutral helpers into their own files

**Files:**
- Modify: `GO/internal/processing/coop_processor.go` (remove moved code)
- Modify: `GO/internal/processing/coop_processor_test.go` (remove moved code)
- Modify: `GO/internal/processing/coop_golden_test.go` (remove moved code)
- Modify: `GO/internal/processing/lotte_processor.go` (remove moved code)
- Create: `GO/internal/processing/processor_shared.go`
- Create: `GO/internal/processing/golden_test_helpers_test.go`

**Interfaces:**
- Consumes: nothing new — this is a pure code-motion refactor.
- Produces: identical symbols (`regionInfo`, `closeEnough`, `buildPromoBonusRow`, `buildInvoiceBonusRow`, `coopDebtDays`, `stripBlankLines`, `xPlus1Pattern`, `fixtureData`, `frozenPricingFixture`, `compareRowsAgainstFixture`, `stringify`, `toFloatString`, `toFloat`, `floatCloseEnough`, `copyFile`, `joinLines`, `fixturePricingSource`, `copyTestWorkbookForProcessor`) now living in `processor_shared.go`/`golden_test_helpers_test.go` instead of the Coop/Lotte files — every later task in this plan consumes these from their new location. **No behavior changes** — every existing Coop/Lotte/Satra test must produce byte-identical results before and after this task.

- [ ] **Step 1: Create `processor_shared.go` with the moved production code**

Create `GO/internal/processing/processor_shared.go`:

```go
package processing

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/productdata"
)

// coopDebtDays is songayno_MT in xulydonhang.py — one global constant,
// shared by every vendor's write function, not Coop-specific.
const coopDebtDays = 60

// xPlus1Pattern mirrors the "(\d+)\s*\+\s*1" match inside
// write_to_dondathang's promo-bonus-quantity logic.
var xPlus1Pattern = regexp.MustCompile(`(\d+)\s*\+\s*1`)

// regionInfo mirrors write_to_dondathang's warehouse/region branching:
// customer codes starting with "MB" (Miền Bắc) map to the Hà Nội
// warehouse; everything else defaults to Miền Nam / Long An. Confirmed
// vendor-neutral for Coop, Lotte, and Satra's customer-code shapes — but
// NOT a fit for BigC's warehouse table, which needs a genuine 3-way
// split (MB / MN_MT / MN_GC) with a different Miền Nam warehouse per
// branch (see bigc_processor.go's own bigcRegionInfo, which does NOT
// call this function — extending this one instead would silently change
// Satra's already-shipped "MN_MT_*" codes' resolved warehouse).
func regionInfo(customerCode string) (region, statCode, warehouse string) {
	if strings.HasPrefix(customerCode, "MB") {
		return "MT_MB", "HN", "TP_HN_12"
	}
	return "MT_MN", "LA", "LA_TP"
}

func closeEnough(a, b float64) bool {
	const relTol = 1e-4
	return math.Abs(a-b) <= relTol*math.Max(math.Abs(a), math.Abs(b))
}

// stripBlankLines drops every line that is empty or all-whitespace,
// rejoining the rest with "\n". Originally written for Lotte (see
// processLotteSegment's comment for why this is needed: it reconstructs
// the blank-line-free shape PyMuPDF's text extraction produces from this
// repo's Go PDF library's GetPlainText output, which inserts extra blank
// lines PyMuPDF does not) — confirmed the same underlying library quirk
// also affects Satra's PDF template (see normalizeSatraText in
// satra_processor.go), so this now lives here as shared infrastructure.
func stripBlankLines(text string) string {
	lines := strings.Split(text, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\n")
}

// buildPromoBonusRow mirrors Coop's write_to_dondathang bonus-row
// construction (xulydonhang.py:1174-1211) and Satra's identical-shaped
// equivalent (:2555-2612) — confirmed NOT a fit for BigC's
// write_to_dondathang_bigc (see bigc_processor.go's own row-builder).
// orderNumber is the CALLER's fully-formed order-number string (e.g.
// "ĐĐHCOOP-12345", "ĐĐHLOTTE-12345", "ĐĐHSATRA-P-12345") — this function
// no longer hardcodes Coop's own orderNumber() formatter internally
// (Task 1 of this plan removed that), so every caller must pass its own
// vendor-correct order number directly; no post-patch needed afterward.
func buildPromoBonusRow(store *productdata.Store, promoPart string, product coop.Product, index int,
	entryDate, cancelDate, shipTo, customerCode, description, warehouse, region, statCode, orderNumber string,
) (row excelwriter.Row, mainRowNote string, mainRowBundleSku string, added bool) {
	skus := store.FindSkusMentioned(promoPart)
	bonusMatch := xPlus1Pattern.FindStringSubmatch(promoPart)
	bonusQty := product.Qty
	bonusSku := ""
	if len(skus) > 0 {
		bonusSku = strings.Join(skus, ", ")
	}
	if bonusMatch != nil {
		x, _ := strconv.Atoi(bonusMatch[1])
		if bonusSku == "" {
			bonusSku = product.Barcode
		}
		if x >= 2 {
			bonusQty = math.Floor(bonusQty / float64(x))
		}
	}
	if bonusSku == "" {
		return excelwriter.Row{}, "", "", false
	}

	bonusInfo, _ := store.GetProductInfo(bonusSku)
	bonusWeight := bonusInfo.WeightKg * bonusQty
	bonusCase := 0
	if bonusInfo.PackSize > 0 {
		bonusCase = int(math.Ceil(bonusQty / bonusInfo.PackSize))
	}

	bundleNote := coop.ExtractBraceContent(promoPart)
	if bundleNote == "" {
		bundleNote = "KM Bó Kèm - Che Barcode"
	}
	lower := strings.ToLower(bundleNote)
	isBundle := strings.Contains(lower, "bó kèm") || strings.Contains(lower, "quấn kèm")
	bundleSkuValue := ""
	if isBundle {
		bundleSkuValue = fmt.Sprintf("%s_%s_1", coop.LastFourDigits(product.Barcode), coop.LastFourDigits(bonusSku))
	}

	row = excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
		Description: description, SKU: bonusSku, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsPromoItem: true, Qty: bonusQty, ProductName: bonusInfo.Name,
		CaseCount: bonusCase, LineWeightKg: bonusWeight, UseZFormula: false,
	}
	if isBundle {
		row.PromoBundleSku = bundleSkuValue
	}

	if index == 0 {
		// Python (xulydonhang.py:1201) writes the first promo item's AO
		// note onto the MAIN PRODUCT ROW, not this bonus row; AP goes
		// onto both the main row and this bonus row (already set above).
		mainRowNote = bundleNote
		mainRowBundleSku = bundleSkuValue
	} else {
		// Python (xulydonhang.py:1211) writes AO for i>0 onto that
		// item's own bonus row.
		row.PromoNote = bundleNote
	}

	return row, mainRowNote, mainRowBundleSku, true
}

// buildInvoiceBonusRow mirrors Coop's/Satra's invoice-level promo bonus
// row. orderNumber is the caller's fully-formed order-number string (see
// buildPromoBonusRow's doc comment — same Task 1 parameterization).
func buildInvoiceBonusRow(store *productdata.Store, invoicePromo string, totalValue float64,
	entryDate, cancelDate, shipTo, customerCode, description, warehouse, region, statCode, orderNumber string,
) (excelwriter.Row, bool) {
	skus := store.FindSkusMentioned(invoicePromo)
	amount, ok := coop.ExtractMoneyAmount(invoicePromo)
	if !ok || amount <= 0 || len(skus) == 0 {
		return excelwriter.Row{}, false
	}
	bonusQty := math.Floor(totalValue / float64(amount))
	bonusInfo, _ := store.GetProductInfo(skus[0])
	bonusWeight := bonusInfo.WeightKg * bonusQty
	bonusCase := 0
	if bonusInfo.PackSize > 0 {
		bonusCase = int(math.Ceil(bonusQty / bonusInfo.PackSize))
	}
	bundleNote := coop.ExtractBraceContent(invoicePromo)
	if bundleNote == "" {
		bundleNote = "KM Bó Kèm - Che Barcode"
	}
	return excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
		Description: description, SKU: strings.Join(skus, ", "), Warehouse: warehouse, VATPercent: 8,
		RegionCode: region, StatCode: statCode, IsPromoItem: true, Qty: bonusQty, ProductName: bonusInfo.Name,
		CaseCount: bonusCase, LineWeightKg: bonusWeight, PromoNote: bundleNote, PromoContent: invoicePromo,
		UseZFormula: false,
	}, true
}
```

Note: this Step already applies Task 1's parameter rename (`poNumber string` → `orderNumber string`, with the internal `orderNumber(poNumber)` call removed) inline, since writing the old signature here and changing it in Task 1 would mean editing this brand-new file again immediately — Task 1's job is exclusively updating the 6 call sites (Coop×2, Lotte×2, Satra×2) to match this already-parameterized signature, not touching `processor_shared.go` itself further.

- [ ] **Step 2: Remove the moved code from `coop_processor.go`**

Delete from `GO/internal/processing/coop_processor.go`:
- Line 19: `const coopDebtDays = 60 // ...` (whole line + its comment)
- Lines 131-133: the `xPlus1Pattern` doc comment + var declaration
- Lines 355-464: `regionInfo`, `closeEnough`, `buildPromoBonusRow`, `buildInvoiceBonusRow` (everything from the `regionInfo` doc comment through `buildInvoiceBonusRow`'s closing `}`) — but **keep** the `orderNumber` function (lines 347-353, Coop's own order-number formatter, genuinely Coop-specific, stays in `coop_processor.go`).

Read the file first to confirm these line ranges still match (this plan was written against a specific commit; if line numbers drifted, locate the same functions by name and remove them by content, not blindly by line number).

- [ ] **Step 3: Remove the moved code from `lotte_processor.go`**

Delete `stripBlankLines` (its doc comment + function body) from the end of `GO/internal/processing/lotte_processor.go`.

- [ ] **Step 4: Fix imports and run `go build ./...`**

`coop_processor.go` and `lotte_processor.go` will likely have unused imports after the removals (e.g. `math`, `regexp`, `strconv` if nothing else in the file still uses them). Run `cd GO && go build ./...`, and for every "imported and not used" error, remove that import line from the offending file. Do NOT remove an import that's still used elsewhere in the same file.

Expected: clean build once unused imports are removed.

- [ ] **Step 5: Create `golden_test_helpers_test.go` with the moved test code**

First, read `GO/internal/processing/coop_golden_test.go` lines 123 through the end of the file (the `compareRowsAgainstFixture` function through `joinLines`) and `GO/internal/processing/coop_processor_test.go` lines 14-33 (`fixturePricingSource` + `copyTestWorkbookForProcessor`) to confirm current exact content before moving — this plan cites line numbers from a specific commit; re-locate by function name if they've drifted.

Create `GO/internal/processing/golden_test_helpers_test.go`:

```go
package processing

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/processing/pricing"
)
```

(`fmt` and `excelize` are needed by `compareRowsAgainstFixture`/`stringify`/`toFloat`/`toFloatString`, moved in below — confirmed by reading their current bodies in `coop_golden_test.go` before writing this plan. If `go build` still reports an unused or missing import after the move, trust the compiler over this list — this is a mechanical move, not a rewrite.)

```go

// fixturePricingSource is a PricingSource that always returns the same
// frozen *pricing.Index — used by both the "real sample file" processor
// tests (with a small inline index) and the golden-fixture tests (with
// an index parsed from a captured _frozen_pricing.json).
type fixturePricingSource struct {
	index *pricing.Index
}

func (f *fixturePricingSource) FetchIndex(sheetKey string) (*pricing.Index, error) {
	return f.index, nil
}

func copyTestWorkbookForProcessor(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/dondathang.xlsx")
	if err != nil {
		t.Fatalf("failed reading test workbook fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "dondathang.xlsx")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed writing temp workbook: %v", err)
	}
	return path
}

// frozenPricingFixture mirrors _frozen_pricing.json's shape: the single
// raw CSV snapshot both the price and promotion views are derived from.
type frozenPricingFixture struct {
	RawRows [][]string `json:"raw_rows"`
}

type fixtureData struct {
	SourcePDF string           `json:"source_pdf"`
	Rows      []map[string]any `json:"rows"`
}

// compareRowsAgainstFixture, stringify, toFloatString, toFloat,
// floatCloseEnough, copyFile, joinLines: PASTE VERBATIM from
// coop_golden_test.go's current lines 123-304 (compareRowsAgainstFixture
// through the end of joinLines) — do not retype by hand, cut-and-paste
// to avoid transcription errors in this fixture-comparison logic that
// every vendor's golden test depends on for correctness.
```

The last comment block is an instruction to the implementer, not literal Go — replace it by actually moving `compareRowsAgainstFixture` (starting at the `// compareRowsAgainstFixture mirrors...` doc comment immediately before it, if one exists — check) through `joinLines`'s closing `}` from `coop_golden_test.go` into this file, unchanged.

- [ ] **Step 6: Remove the moved code from `coop_processor_test.go` and `coop_golden_test.go`**

From `GO/internal/processing/coop_processor_test.go`: delete the `fixturePricingSource` type + its `FetchIndex` method + `copyTestWorkbookForProcessor` (lines 14-33).

From `GO/internal/processing/coop_golden_test.go`: delete the `frozenPricingFixture` type, `fixtureData` type (lines 17-27), and `compareRowsAgainstFixture` through `joinLines` (lines 123-304) — **keep** `loadFrozenPricingSource` (Coop-specific, loads from `coop/testdata/fixtures/_frozen_pricing.json`) and `TestRealProcessor_MatchesGoldenFixtures` (Coop's own golden test) in place.

- [ ] **Step 7: Fix imports and run the full test suite**

Run `cd GO && go build ./... && go vet ./...`, fix any unused-import errors the same way as Step 4.

Run: `cd GO && go test ./... -v 2>&1 | tee /tmp/task0-after.txt` (or equivalent — capture full output).

Expected: **every** Coop/Lotte/Satra test that passed before this task still passes, with identical results — specifically confirm `TestRealProcessor_ProcessesRealSampleCoopFile`, `TestRealProcessor_PromoBonusRowFieldsMatchPythonRowTarget`, `TestRealProcessor_MatchesGoldenFixtures_Lotte` (60/60), `TestRealProcessor_MatchesGoldenFixtures_Satra` (36/36) are unchanged. `TestRealProcessor_MatchesGoldenFixtures` (Coop) is expected to still show its known pre-existing, unrelated failure (same mismatch count as before this task — confirm the count is IDENTICAL, not worse) — this task must not be blocked by that pre-existing issue, but must not make it worse either.

- [ ] **Step 8: Commit**

```bash
git add GO/internal/processing/processor_shared.go GO/internal/processing/golden_test_helpers_test.go GO/internal/processing/coop_processor.go GO/internal/processing/coop_processor_test.go GO/internal/processing/coop_golden_test.go GO/internal/processing/lotte_processor.go
git commit -m "refactor(go): extract shared vendor-neutral helpers into their own files"
```

---

### Task 1: Parameterize order-number on `buildPromoBonusRow`/`buildInvoiceBonusRow`

**Files:**
- Modify: `GO/internal/processing/coop_processor.go`
- Modify: `GO/internal/processing/lotte_processor.go`
- Modify: `GO/internal/processing/satra_processor.go`

**Interfaces:**
- Consumes: `buildPromoBonusRow`/`buildInvoiceBonusRow`'s new `orderNumber string` parameter (already defined this way in Task 0's `processor_shared.go` — this task only updates call sites).
- Produces: no post-patch of `bonusRow.OrderNumber` anywhere in the codebase after this task — consumed by Task 6 (BigC does NOT need this pattern at all, since it doesn't call these shared functions, but every OTHER vendor's call sites must be updated so the codebase has exactly one convention).

- [ ] **Step 1: Update Coop's 2 call sites in `coop_processor.go`**

Read the file first to confirm current line numbers (should be near where `regionInfo`/etc. used to be, in `processSegment`). Change:

```go
			bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, promoPart, product, i, entryDate, cancelDate, shipTo,
				customerCode, description, warehouse, region, statCode, info.PONumber)
```
to:
```go
			bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, promoPart, product, i, entryDate, cancelDate, shipTo,
				customerCode, description, warehouse, region, statCode, orderNumber(info.PONumber))
```

and:
```go
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, entryDate, cancelDate,
			shipTo, customerCode, description, warehouse, region, statCode, info.PONumber); added {
```
to:
```go
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, entryDate, cancelDate,
			shipTo, customerCode, description, warehouse, region, statCode, orderNumber(info.PONumber)); added {
```

(Coop's own `orderNumber(poNumber string) string` function, kept in `coop_processor.go` by Task 0's Step 2, now gets called explicitly at each call site instead of implicitly inside the shared helpers.)

- [ ] **Step 2: Update Lotte's 2 call sites in `lotte_processor.go`**

Change:
```go
		bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, lastExaminedPromo,
			coop.Product{Barcode: barcode, Qty: qty}, 0, info.EntryDate, cancelDate, shipTo,
			customerCode, description, warehouse, region, statCode, info.PONumber)
		if added {
			bonusRow.OrderNumber = lotteOrderNumber(info.PONumber) // buildPromoBonusRow hardcodes Coop's order number
			totalWeight += bonusRow.LineWeightKg
```
to:
```go
		bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, lastExaminedPromo,
			coop.Product{Barcode: barcode, Qty: qty}, 0, info.EntryDate, cancelDate, shipTo,
			customerCode, description, warehouse, region, statCode, lotteOrderNumber(info.PONumber))
		if added {
			totalWeight += bonusRow.LineWeightKg
```

and:
```go
	if invoicePromo := priceIndex.FindInvoicePromotion(info.EntryDate); invoicePromo != "" {
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, info.EntryDate, cancelDate,
			shipTo, customerCode, description, warehouse, region, statCode, info.PONumber); added {
			bonusRow.OrderNumber = lotteOrderNumber(info.PONumber) // buildInvoiceBonusRow hardcodes Coop's order number
			totalWeight += bonusRow.LineWeightKg
```
to:
```go
	if invoicePromo := priceIndex.FindInvoicePromotion(info.EntryDate); invoicePromo != "" {
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, info.EntryDate, cancelDate,
			shipTo, customerCode, description, warehouse, region, statCode, lotteOrderNumber(info.PONumber)); added {
			totalWeight += bonusRow.LineWeightKg
```

- [ ] **Step 3: Update Satra's 2 call sites in `satra_processor.go`**

Change:
```go
			bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, promoPart,
				coop.Product{Barcode: barcode, Qty: qty}, i, entryDate, cancelDate, shipTo,
				customerCode, noteText, warehouse, region, statCode, poNumber)
			if !added {
				continue
			}
			bonusRow.OrderNumber = satraOrderNumber(poNumber) // buildPromoBonusRow hardcodes Coop's order number
			totalWeight += bonusRow.LineWeightKg
```
to:
```go
			bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, promoPart,
				coop.Product{Barcode: barcode, Qty: qty}, i, entryDate, cancelDate, shipTo,
				customerCode, noteText, warehouse, region, statCode, satraOrderNumber(poNumber))
			if !added {
				continue
			}
			totalWeight += bonusRow.LineWeightKg
```

and:
```go
	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, entryDate, cancelDate,
			shipTo, customerCode, noteText, warehouse, region, statCode, poNumber); added {
			bonusRow.OrderNumber = satraOrderNumber(poNumber)
			totalWeight += bonusRow.LineWeightKg
```
to:
```go
	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, entryDate, cancelDate,
			shipTo, customerCode, noteText, warehouse, region, statCode, satraOrderNumber(poNumber)); added {
			totalWeight += bonusRow.LineWeightKg
```

- [ ] **Step 4: Run the full test suite**

Run: `cd GO && go build ./... && go vet ./... && go test ./... -v`

Expected: identical results to Task 0's Step 7 baseline — this task is a pure call-site update with no behavior change (the value passed as `orderNumber` is exactly what the old post-patch used to set `bonusRow.OrderNumber` to, just passed in upfront instead of assigned after). `TestRealProcessor_PromoBonusRowFieldsMatchPythonRowTarget` and every golden-fixture test's `OrderNumber`/`B`-column assertions must be byte-identical to before.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/coop_processor.go GO/internal/processing/lotte_processor.go GO/internal/processing/satra_processor.go
git commit -m "refactor(go): parameterize buildPromoBonusRow/buildInvoiceBonusRow's order-number"
```

---

### Task 2: `vendor.Identify` — recognize BigC, inserted between Coop and Lotte

**Files:**
- Modify: `GO/internal/processing/vendor/identify.go`
- Modify: `GO/internal/processing/vendor/identify_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `Identify(text string) string` now also returns `"BigC"` — consumed by Task 6's dispatch pre-check AND the fallback per-page dispatch in `RealProcessor.Process`.

- [ ] **Step 1: Write the failing test**

Add to `GO/internal/processing/vendor/identify_test.go`:

```go
func TestIdentify_RecognizesBigCByCodeOrCompanyString(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"3005382 substring", "PO Number 3005382 something else", "BigC"},
		{"CTY TNHH DV EB, case-insensitive", "cty tnhh dv eb ThuanKieu", "BigC"},
		{"CTY TNHH DV EB, real casing", "Header CTY TNHH DV EB Footer", "BigC"},
		{"unrelated text", "nothing matches here", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Identify(c.text)
			if got != c.want {
				t.Fatalf("Identify(%q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
}

func TestIdentify_BigCCheckedBeforeLotte(t *testing.T) {
	// A page whose text happens to contain both BigC's "3005382" marker
	// and (hypothetically) some other vendor's marker must resolve to
	// BigC first, matching Python's real check order (Coop -> BigC ->
	// Lotte -> Satra -> ...). This test only has BigC's own marker
	// available to construct with today, but documents the intent so a
	// future vendor addition that touches this ordering notices the
	// contract.
	got := Identify("3005382 unrelated content")
	if got != "BigC" {
		t.Fatalf("Identify with BigC marker = %q, want %q", got, "BigC")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/vendor/... -run "TestIdentify_RecognizesBigC|TestIdentify_BigCCheckedBeforeLotte" -v`
Expected: FAIL — `Identify` returns `""` for the BigC cases.

- [ ] **Step 3: Implement**

Read `GO/internal/processing/vendor/identify.go` first to confirm its current exact shape (the existing `Coop`/`Lotte`/`Satra` pattern vars and the `Identify` function body) before editing — this plan's snippet below shows the INTENDED end state, insert BigC's pattern and case in the correct position relative to whatever is actually there.

Add a `bigcPattern` var alongside the existing `coopPattern`/`lottePattern`/`satraPattern`:

```go
	// BigC's identify pattern (xulydonhang.py:99): either a literal
	// "3005382" substring, or "CTY TNHH DV EB" case-insensitive, in the
	// whitespace-normalized page text. Confirmed on real BigC PDFs that
	// "CTY TNHH DV EB" alone appears on EVERY page (not just page 0) —
	// "3005382" is the page-0-exclusive one. Both are checked via one
	// combined regex here, matching Python's `or`.
	bigcPattern = regexp.MustCompile(`(?i)3005382|CTY TNHH DV EB`)
```

In `Identify`, insert the BigC check **immediately after the Coop check and before the Lotte check** — this order matters and mirrors `identify_vendor`'s real sequence (`xulydonhang.py:90-179`: Coop → BigC → Lotte → Satra → ...):

```go
	if bigcPattern.MatchString(cleaned) {
		return "BigC"
	}
```

Update `Identify`'s doc comment to mention Coop, BigC, Lotte, and Satra are implemented, in that order, and note that this order is load-bearing (mirrors Python's real `identify_vendor` precedence) — a future vendor addition must insert its own case at the correct position in this sequence, not simply append it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/vendor/... -v`
Expected: PASS — all Coop/Lotte/Satra tests still pass (regression check), all new BigC tests pass.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/vendor/identify.go GO/internal/processing/vendor/identify_test.go
git commit -m "feat(go): recognize BigC vendor in identify.Identify, inserted before Lotte"
```

---

### Task 3: `bigc` package — PO number, entry date, cancel date (page 0)

**Files:**
- Create: `GO/internal/processing/bigc/extract.go`
- Create: `GO/internal/processing/bigc/extract_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `bigc.ParseOrderInfo(pageZeroText string) (poNumber, entryDate, cancelDate string, ok bool)`. Consumed by Task 6.

- [ ] **Step 1: Write the failing tests**

Create `GO/internal/processing/bigc/extract_test.go`:

```go
package bigc

import "testing"

func TestParseOrderInfo_ExtractsPOAndEntryDate(t *testing.T) {
	// PO number is a 13+ digit number immediately followed by a
	// DD/MM/YY-shaped date; cancel date here comes from the region after
	// the LAST "Total Net Purchase Price" occurrence.
	text := "Header\n2631057733376 31/07/26\nsome content\nTotal Net Purchase Price\n04/08/26\nfooter"
	po, entry, cancel, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo: no match, want match")
	}
	if po != "2631057733376" {
		t.Fatalf("po = %q, want %q", po, "2631057733376")
	}
	if entry != "31/07/2026" {
		t.Fatalf("entry = %q, want %q", entry, "31/07/2026")
	}
	if cancel != "04/08/2026" {
		t.Fatalf("cancel = %q, want %q", cancel, "04/08/2026")
	}
}

func TestParseOrderInfo_UsesLastTotalNetPurchasePriceOccurrence(t *testing.T) {
	text := "2631057733376 31/07/26\n" +
		"Total Net Purchase Price\n01/01/26 (this one must be ignored)\n" +
		"more text\n" +
		"Total Net Purchase Price\n04/08/26 (this is the real one)"
	_, _, cancel, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo: no match, want match")
	}
	if cancel != "04/08/2026" {
		t.Fatalf("cancel = %q, want %q (must use the LAST occurrence)", cancel, "04/08/2026")
	}
}

func TestParseOrderInfo_FallsBackToEntryDatePlus5DaysWhenNoCancelDateFound(t *testing.T) {
	// No "Total Net Purchase Price" marker at all -> fallback fires.
	text := "2631057733376 31/07/26\nno marker anywhere in this text"
	po, entry, cancel, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo: no match, want match")
	}
	if po != "2631057733376" || entry != "31/07/2026" {
		t.Fatalf("po/entry = %q/%q, want %q/%q", po, entry, "2631057733376", "31/07/2026")
	}
	if cancel != "05/08/2026" {
		t.Fatalf("cancel (fallback) = %q, want %q (entry + 5 days)", cancel, "05/08/2026")
	}
}

func TestParseOrderInfo_NoMatchReturnsFalse(t *testing.T) {
	_, _, _, ok := ParseOrderInfo("no PO-shaped number and date pair here")
	if ok {
		t.Fatal("ParseOrderInfo: matched, want no match")
	}
}

func TestParseOrderInfo_WhitespaceIsCollapsedBeforeMatching(t *testing.T) {
	// Python collapses all whitespace runs to a single space before
	// matching (re.sub(r"\s+", " ", text)) — confirm the Go port does
	// the same, so a PO number split across a line break still matches.
	text := "2631057733376\n\n   31/07/26\nTotal Net Purchase Price\n\n04/08/26"
	po, entry, cancel, ok := ParseOrderInfo(text)
	if !ok || po != "2631057733376" || entry != "31/07/2026" || cancel != "04/08/2026" {
		t.Fatalf("ParseOrderInfo(whitespace-heavy) = (%q, %q, %q, %v), want (%q, %q, %q, true)",
			po, entry, cancel, ok, "2631057733376", "31/07/2026", "04/08/2026")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/bigc/... -v`
Expected: FAIL — package doesn't exist yet / `ParseOrderInfo` undefined.

- [ ] **Step 3: Implement**

Create `GO/internal/processing/bigc/extract.go`:

```go
package bigc

import (
	"regexp"
	"strings"
	"time"
)

// poEntryDatePattern mirrors trichxuatinfo_donbigc's PO-number/entry-date
// match (xulydonhang.py:5948): r"(\d{13,})\s+(\d{2}/\d{2}/\d{2})" against
// whitespace-collapsed text — group 1 is the PO number, group 2 is the
// entry date in DD/MM/YY form (not yet century-expanded).
var poEntryDatePattern = regexp.MustCompile(`(\d{13,})\s+(\d{2}/\d{2}/\d{2})`)

var whitespaceCollapsePattern = regexp.MustCompile(`\s+`)
var totalNetPurchasePricePattern = regexp.MustCompile(`Total Net Purchase Price`)
var twoDigitDatePattern = regexp.MustCompile(`\d{2}/\d{2}/\d{2}`)
var twoDigitDMYPattern = regexp.MustCompile(`^(\d{2})/(\d{2})/(\d{2})$`)

// ParseOrderInfo mirrors trichxuatinfo_donbigc (xulydonhang.py:5941-5973)
// in full. Extracts the PO number and entry date from the first
// "<13+ digit number> <DD/MM/YY>" match in the whitespace-collapsed
// page-0 text (xulydonhang.py:5945,5948), then the cancel date from the
// first DD/MM/YY-shaped date found after the LAST occurrence of "Total
// Net Purchase Price" (:5952-5960) — falling back to entryDate + 5 days
// (:5964-5971) if no such date is found. Both dates are returned already
// century-expanded DD/MM/YY -> DD/MM/20YY (mirrors convert_entry_date,
// :5333-5340, applied to both entry_date and cancel_date at the end of
// the Python function).
//
// ok=false only when no PO/entry-date match is found at all, or the
// matched entry-date text isn't a real calendar date. Python's
// equivalent failure mode (entry_date stays None, or convert_entry_date
// receives an unparseable string) crashes with an unhandled TypeError/
// ValueError deep in the call chain — Go returns a clean failure instead
// (Phase 2b's "correct main flow" policy, same principle Satra's
// ParsePONumber/ParseEntryDate already established). If the cancel-date
// portion can't be resolved (neither the region-scan nor the +5-day
// fallback produces a parseable date — only possible if the matched
// entry-date digits aren't a real calendar date, e.g. "99/99/99"),
// cancelDate is returned as "" rather than failing the whole parse —
// best-effort, matching how ParseCancelDate works for other vendors in
// this codebase.
func ParseOrderInfo(text string) (poNumber, entryDate, cancelDate string, ok bool) {
	cleaned := strings.TrimSpace(whitespaceCollapsePattern.ReplaceAllString(text, " "))

	m := poEntryDatePattern.FindStringSubmatch(cleaned)
	if m == nil {
		return "", "", "", false
	}
	poNumber = m[1]
	rawEntryDate := m[2] // DD/MM/YY, not yet century-expanded

	entryDateConverted, entryOk := convertEntryDate(rawEntryDate)
	if !entryOk {
		return "", "", "", false
	}

	rawCancelDate := ""
	if matches := totalNetPurchasePricePattern.FindAllStringIndex(cleaned, -1); len(matches) > 0 {
		last := matches[len(matches)-1]
		region := cleaned[last[1]:]
		rawCancelDate = twoDigitDatePattern.FindString(region)
	}
	if rawCancelDate == "" {
		if t, err := time.Parse("02/01/06", rawEntryDate); err == nil {
			rawCancelDate = t.AddDate(0, 0, 5).Format("02/01/06")
		}
	}
	if rawCancelDate != "" {
		cancelDate, _ = convertEntryDate(rawCancelDate) // best-effort
	}

	return poNumber, entryDateConverted, cancelDate, true
}

// convertEntryDate mirrors convert_entry_date (xulydonhang.py:5333-5340):
// DD/MM/YY -> DD/MM/20YY. Always assumes the 2000s (Python's literal
// f"20{year}", not a general pivot-year rule) — deliberately not
// "future-proofed" beyond what the Python source actually does.
func convertEntryDate(raw string) (string, bool) {
	m := twoDigitDMYPattern.FindStringSubmatch(raw)
	if m == nil {
		return "", false
	}
	return m[1] + "/" + m[2] + "/20" + m[3], true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/bigc/... -v`
Expected: PASS — all 5 tests.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/bigc/extract.go GO/internal/processing/bigc/extract_test.go
git commit -m "feat(go): add bigc package with PO number, entry date, and cancel date extraction"
```

---

### Task 4: `bigc` package — page-0 price list and customer-code lookup

**Files:**
- Modify: `GO/internal/processing/bigc/extract.go`
- Modify: `GO/internal/processing/bigc/extract_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `bigc.Product{Barcode, SKUOrUnit, OrderedUnitQty string; UnitPrice, TotalNetPurchasePrice float64}`; `bigc.ExtractPriceList(pageZeroText string) []Product`; `bigc.ResolveCustomerCode(pageZeroText string) (customerCode, deliveryWarehouse string)`. Consumed by Task 6.

- [ ] **Step 1: Write the failing tests**

Add to `GO/internal/processing/bigc/extract_test.go`:

```go
func TestExtractPriceList_ParsesProductLinesAfterArticleHeader(t *testing.T) {
	// Shape mirrors laydanhsachsanpham_bigc's expected line: 13-digit
	// barcode, some description text, then "Pack <level> <SKU/OU>
	// <OU Qty> <another number> <unit price with comma> <unit> <total
	// price with comma>".
	text := "Preamble that must be ignored\n" +
		"Article Description Pack Level SKU OUQty More Unit TotalPrice\n" +
		"8936156730879 Nuoc giat Blue Pack 1 4 20 1 37,188 PC 148,750\n" +
		"8936156730992 Nuoc xa Pink Pack 1 6 12 1 25,000 PC 150,000\n"
	got := ExtractPriceList(text)
	want := []Product{
		{Barcode: "8936156730879", SKUOrUnit: "4", OrderedUnitQty: "20", UnitPrice: 37188, TotalNetPurchasePrice: 148750},
		{Barcode: "8936156730992", SKUOrUnit: "6", OrderedUnitQty: "12", UnitPrice: 25000, TotalNetPurchasePrice: 150000},
	}
	if len(got) != len(want) {
		t.Fatalf("ExtractPriceList returned %d products, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ExtractPriceList()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestExtractPriceList_NoArticleHeaderReturnsEmpty(t *testing.T) {
	if got := ExtractPriceList("no header keyword anywhere"); len(got) != 0 {
		t.Fatalf("ExtractPriceList = %+v, want empty", got)
	}
}

func TestExtractPriceList_TextBeforeArticleHeaderIsIgnored(t *testing.T) {
	// A product-shaped line appearing BEFORE the "Article" header must
	// not be matched (mirrors slicing the text at match_start.start()
	// before running the product-line regex).
	text := "8936156730879 Preamble Pack 1 4 20 1 37,188 PC 148,750\n" +
		"Article\n" +
		"8936156730992 Nuoc xa Pink Pack 1 6 12 1 25,000 PC 150,000\n"
	got := ExtractPriceList(text)
	if len(got) != 1 || got[0].Barcode != "8936156730992" {
		t.Fatalf("ExtractPriceList = %+v, want exactly 1 product (8936156730992)", got)
	}
}

func TestResolveCustomerCode_MBLinfoxCombination(t *testing.T) {
	code, warehouse := ResolveCustomerCode("some text 3006900 more text LINFOX WAREHOUSE (802) footer")
	if code != "MB_GC_BIGC" || warehouse != "LINFOX WAREHOUSE (802)" {
		t.Fatalf("ResolveCustomerCode = (%q, %q), want (%q, %q)", code, warehouse, "MB_GC_BIGC", "LINFOX WAREHOUSE (802)")
	}
}

func TestResolveCustomerCode_AllFourCombinationsAndDefault(t *testing.T) {
	cases := []struct {
		name           string
		text           string
		wantCode       string
		wantWarehouse  string
	}{
		{"3006900+LINFOX", "3006900 LINFOX WAREHOUSE (802)", "MB_GC_BIGC", "LINFOX WAREHOUSE (802)"},
		{"3005382+LINFOX", "3005382 LINFOX WAREHOUSE (802)", "MB_MT_BIGC", "LINFOX WAREHOUSE (802)"},
		{"3005382+FMLOGISTIC", "3005382 FM LOGISTIC VSIP 2 (806)", "MN_MT_BIGCAC", "FM LOGISTIC VSIP 2 (806)"},
		{"3006900+FMLOGISTIC", "3006900 FM LOGISTIC VSIP 2 (806)", "MN_GC_BIGCAC", "FM LOGISTIC VSIP 2 (806)"},
		{"neither signal", "nothing relevant here", "MN_MT_BIGCAC", "FM LOGISTIC VSIP 2 (806)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, warehouse := ResolveCustomerCode(c.text)
			if code != c.wantCode || warehouse != c.wantWarehouse {
				t.Fatalf("ResolveCustomerCode(%q) = (%q, %q), want (%q, %q)", c.text, code, warehouse, c.wantCode, c.wantWarehouse)
			}
		})
	}
}

func TestResolveCustomerCode_CheckOrderMatchesPythonWhenMultipleSignalsPresent(t *testing.T) {
	// If text somehow contains BOTH "3006900" and "3005382" plus LINFOX,
	// Python's if/elif order means the FIRST matching branch wins
	// ("3006900"+LINFOX checked before "3005382"+LINFOX).
	code, warehouse := ResolveCustomerCode("3006900 3005382 LINFOX WAREHOUSE (802)")
	if code != "MB_GC_BIGC" || warehouse != "LINFOX WAREHOUSE (802)" {
		t.Fatalf("ResolveCustomerCode(both signals) = (%q, %q), want (%q, %q) (first-matching-branch-wins)", code, warehouse, "MB_GC_BIGC", "LINFOX WAREHOUSE (802)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/bigc/... -run "TestExtractPriceList|TestResolveCustomerCode" -v`
Expected: FAIL — undefined functions/types.

- [ ] **Step 3: Implement**

Add to `GO/internal/processing/bigc/extract.go` (append; add `"strconv"` to imports):

```go
// Product is one row of BigC's page-0 master price/product list.
type Product struct {
	Barcode        string
	SKUOrUnit      string
	OrderedUnitQty string
	// UnitPrice is Python's "Total Price" dict key (laydanhsachsanpham_bigc,
	// xulydonhang.py:5869) — despite the name, it holds the PER-UNIT net
	// purchase price, not a line total; renamed here for clarity, not
	// literal fidelity to the (misleading) Python key name.
	UnitPrice float64
	// TotalNetPurchasePrice is captured for fidelity but never read
	// downstream anywhere in xulydonhang.py either — kept for
	// completeness/debugging only.
	TotalNetPurchasePrice float64
}

var articleHeaderPattern = regexp.MustCompile(`\bArticle\b`)
var priceListLinePattern = regexp.MustCompile(`(?s)(\d{13})\s+.+?\s+Pack\s+\d+\s+(\d+)\s+(\d+)\s+\d+\s+([\d,]+)\s+\w+\s+([\d,]+)`)

// ExtractPriceList mirrors laydanhsachsanpham_bigc (xulydonhang.py:5831-5873):
// slices page-0 text to everything from the first "Article" header word
// onward (xulydonhang.py:5837-5843), then extracts every matching
// product line from that slice. A line whose 4th/5th numeric field fails
// to parse (after stripping "," separators) is silently skipped —
// mirrors Python's `continue` on a malformed match; Go's regex only ever
// produces exactly 5 capture groups per match (unlike Python's
// len(match) != 5 check, which is checking tuple arity from findall,
// not field validity), so the equivalent Go failure mode is a
// strconv.ParseFloat error on group 4 or 5.
func ExtractPriceList(pageZeroText string) []Product {
	loc := articleHeaderPattern.FindStringIndex(pageZeroText)
	if loc == nil {
		return nil
	}
	text := strings.TrimSpace(pageZeroText[loc[0]:])

	var products []Product
	for _, m := range priceListLinePattern.FindAllStringSubmatch(text, -1) {
		unitPrice, err1 := strconv.ParseFloat(strings.ReplaceAll(m[4], ",", ""), 64)
		totalPrice, err2 := strconv.ParseFloat(strings.ReplaceAll(m[5], ",", ""), 64)
		if err1 != nil || err2 != nil {
			continue
		}
		products = append(products, Product{
			Barcode: strings.TrimSpace(m[1]), SKUOrUnit: strings.TrimSpace(m[2]), OrderedUnitQty: strings.TrimSpace(m[3]),
			UnitPrice: unitPrice, TotalNetPurchasePrice: totalPrice,
		})
	}
	return products
}

// ResolveCustomerCode mirrors the 4-branch customer-code lookup inline
// in process_file's BigC branch (xulydonhang.py:9419-9433): a
// cross-product of 2 supplier codes x 2 warehouse names, checked via
// plain substring containment against page-0's raw text, in this exact
// order, with a default fallback matching Python's else branch. Returns
// both the resolved customer code AND the delivery-warehouse string
// (diachigiao, xulydonhang.py's second assigned variable in every
// branch — written to Excel column E downstream).
func ResolveCustomerCode(pageZeroText string) (customerCode, deliveryWarehouse string) {
	has3006900 := strings.Contains(pageZeroText, "3006900")
	has3005382 := strings.Contains(pageZeroText, "3005382")
	hasLinfox := strings.Contains(pageZeroText, "LINFOX WAREHOUSE (802)")
	hasFMLogistic := strings.Contains(pageZeroText, "FM LOGISTIC VSIP 2 (806)")

	switch {
	case has3006900 && hasLinfox:
		return "MB_GC_BIGC", "LINFOX WAREHOUSE (802)"
	case has3005382 && hasLinfox:
		return "MB_MT_BIGC", "LINFOX WAREHOUSE (802)"
	case has3005382 && hasFMLogistic:
		return "MN_MT_BIGCAC", "FM LOGISTIC VSIP 2 (806)"
	case has3006900 && hasFMLogistic:
		return "MN_GC_BIGCAC", "FM LOGISTIC VSIP 2 (806)"
	default:
		return "MN_MT_BIGCAC", "FM LOGISTIC VSIP 2 (806)"
	}
}
```

**Note for the implementer:** the test fixture text for `ExtractPriceList` above is a hand-constructed approximation of the real line shape — verify it actually satisfies `priceListLinePattern` as constructed (run it, don't assume) and adjust the TEST TEXT (not the regex, which is a direct transcription of the confirmed Python pattern) if the match boundaries differ from what's assumed here, while preserving the same assertions. Task 7's real-PDF fixture generation is the ultimate ground truth regardless.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/bigc/... -v`
Expected: PASS — all tests in the package so far. Adjust test text per the note above if `TestExtractPriceList_*` fails on first run due to the synthetic fixture not matching the regex exactly.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/bigc/extract.go GO/internal/processing/bigc/extract_test.go
git commit -m "feat(go): add page-0 price list extraction and customer-code lookup to bigc package"
```

---

### Task 5: `bigc` package — per-store name, item list, price join

**Files:**
- Modify: `GO/internal/processing/bigc/extract.go`
- Modify: `GO/internal/processing/bigc/extract_test.go`

**Interfaces:**
- Consumes: `Product` (Task 4).
- Produces: `bigc.ExtractStoreName(storePageText string) (string, bool)`; `bigc.StoreItem{Barcode, SKUOrUnit, OrderedUnitQty string; UnitPrice float64}`; `bigc.ExtractStoreItems(storePageText string) []StoreItem`; `bigc.JoinItemsWithPrices(items []StoreItem, priceList []Product) []StoreItem`. Consumed by Task 6.

- [ ] **Step 1: Write the failing tests**

Add to `GO/internal/processing/bigc/extract_test.go`:

```go
func TestExtractStoreName_FindsLineAfterWarehouseAndVietnam(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"FM LOGISTIC", "header\nFM LOGISTIC VSIP 2, Binh Duong, Vietnam\nGO! DONG NAI\nfooter", "GO! DONG NAI"},
		{"LINFOX", "header\nLINFOX WAREHOUSE (802), Hanoi, Vietnam\nGO! AN LAC\nfooter", "GO! AN LAC"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ExtractStoreName(c.text)
			if !ok || got != c.want {
				t.Fatalf("ExtractStoreName(%q) = (%q, %v), want (%q, true)", c.text, got, ok, c.want)
			}
		})
	}
}

func TestExtractStoreName_NoMatchReturnsFalse(t *testing.T) {
	if _, ok := ExtractStoreName("no warehouse marker here"); ok {
		t.Fatal("ExtractStoreName: matched, want no match")
	}
}

func TestExtractStoreItems_ParsesBarcodeAnchoredLines(t *testing.T) {
	// Shape mirrors trichxuatdanhsachforstore_bigc's expectation:
	// "<13-digit barcode>\n<description>\nPack\n<level>\n<SKU/OU>\n<qty>".
	text := "8936156730879\nNuoc giat Blue 3.8kg\nPack\n1\n4\n20\n" +
		"8936156730992\nNuoc xa Pink 2L\nPack\n1\n6\n12\n"
	got := ExtractStoreItems(text)
	want := []StoreItem{
		{Barcode: "8936156730879", SKUOrUnit: "4", OrderedUnitQty: "20"},
		{Barcode: "8936156730992", SKUOrUnit: "6", OrderedUnitQty: "12"},
	}
	if len(got) != len(want) {
		t.Fatalf("ExtractStoreItems returned %d items, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ExtractStoreItems()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestExtractStoreItems_MatchesAtStartOfTextToo(t *testing.T) {
	// Go's port uses "(?:^|\n)" in place of Python's lookbehind
	// "(?<=\n)" (RE2 has no lookbehind support) — confirm a barcode at
	// the very start of the text (no preceding newline at all) still
	// matches, same as Python's lookbehind would at position 0... wait,
	// Python's (?<=\n) actually requires a literal preceding newline, so
	// it would NOT match at true position 0 of the whole page text
	// either. This test documents the Go port matches Python exactly:
	// position 0 DOES match here because Go's "^" (no multiline flag)
	// anchors the start of the whole string, same effective behavior as
	// every real store page having some preceding header text before
	// the first item line in practice. If this ever diverges from a
	// real fixture in Task 7/8, treat Python's literal (?<=\n) as
	// authoritative and adjust the Go pattern.
	text := "8936156730879\nNuoc giat Blue\nPack\n1\n4\n20\n"
	got := ExtractStoreItems(text)
	if len(got) != 1 || got[0].Barcode != "8936156730879" {
		t.Fatalf("ExtractStoreItems(text starting with barcode) = %+v, want 1 item with barcode 8936156730879", got)
	}
}

func TestExtractStoreItems_NoMatchesReturnsEmpty(t *testing.T) {
	if got := ExtractStoreItems("no item lines here"); len(got) != 0 {
		t.Fatalf("ExtractStoreItems = %+v, want empty", got)
	}
}

func TestJoinItemsWithPrices_LooksUpByBarcode(t *testing.T) {
	items := []StoreItem{
		{Barcode: "8936156730879", SKUOrUnit: "4", OrderedUnitQty: "20"},
		{Barcode: "8936156730992", SKUOrUnit: "6", OrderedUnitQty: "12"},
	}
	priceList := []Product{
		{Barcode: "8936156730879", UnitPrice: 37188},
		{Barcode: "8936156730992", UnitPrice: 25000},
	}
	got := JoinItemsWithPrices(items, priceList)
	if got[0].UnitPrice != 37188 || got[1].UnitPrice != 25000 {
		t.Fatalf("JoinItemsWithPrices = %+v, want prices 37188 and 25000", got)
	}
}

func TestJoinItemsWithPrices_MissingBarcodeSilentlyDefaultsToZero(t *testing.T) {
	// Mirrors ghepgia_donhangbigc's product_dict.get(article, 0) — the
	// Python comment claims this "reports an error" but the real code
	// does not; port the silent-zero behavior faithfully.
	items := []StoreItem{{Barcode: "9999999999999", SKUOrUnit: "1", OrderedUnitQty: "1"}}
	priceList := []Product{{Barcode: "8936156730879", UnitPrice: 37188}}
	got := JoinItemsWithPrices(items, priceList)
	if got[0].UnitPrice != 0 {
		t.Fatalf("JoinItemsWithPrices(unmatched barcode) = %+v, want UnitPrice 0", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/bigc/... -run "TestExtractStoreName|TestExtractStoreItems|TestJoinItemsWithPrices" -v`
Expected: FAIL — undefined functions/types.

- [ ] **Step 3: Implement**

Add to `GO/internal/processing/bigc/extract.go` (append):

```go
var storeNamePattern = regexp.MustCompile(`(?s)(FM LOGISTIC VSIP 2|LINFOX WAREHOUSE \(802\)).*?Vietnam\s*\n(.*?)\n`)

// ExtractStoreName mirrors lay_ten_store (xulydonhang.py:5878-5884): the
// line immediately following the first "Vietnam" occurrence after the
// warehouse name (FM LOGISTIC VSIP 2 or LINFOX WAREHOUSE (802)) on a
// single store page. Returns ("", false) if no match — mirrors Python
// returning None.
func ExtractStoreName(storePageText string) (string, bool) {
	m := storeNamePattern.FindStringSubmatch(storePageText)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[2]), true
}

// StoreItem is one line of a single store page's item list. UnitPrice is
// zero until JoinItemsWithPrices fills it in from the page-0 master
// list — ExtractStoreItems alone never sets it (mirrors
// trichxuatdanhsachforstore_bigc producing dicts with no price field at
// all, xulydonhang.py:5906).
type StoreItem struct {
	Barcode        string
	SKUOrUnit      string
	OrderedUnitQty string
	UnitPrice      float64
}

// storeItemPattern mirrors trichxuatdanhsachforstore_bigc's regex
// (xulydonhang.py:5902): r"(?<=\n)(\d{13})\s*\n(.*?)\s*\nPack\s*\n\d+\s*\n(\d+)\s*\n(\d+)"
// with re.DOTALL. Go's RE2 engine has no lookbehind support, so
// "(?<=\n)" is replaced with "(?:^|\n)" (non-capturing, doesn't shift
// group numbering) — equivalent for this use, since both only need to
// confirm the barcode starts at a line boundary. Group 2 (the
// description line between the barcode and "Pack") is matched but
// deliberately discarded, matching Python's list comprehension only
// keeping groups 1, 3, 4 (xulydonhang.py:5906: m[0], m[2], m[3] from a
// 0-indexed findall tuple).
var storeItemPattern = regexp.MustCompile(`(?s)(?:^|\n)(\d{13})\s*\n(.*?)\s*\nPack\s*\n\d+\s*\n(\d+)\s*\n(\d+)`)

// ExtractStoreItems mirrors trichxuatdanhsachforstore_bigc
// (xulydonhang.py:5900-5907).
func ExtractStoreItems(storePageText string) []StoreItem {
	var items []StoreItem
	for _, m := range storeItemPattern.FindAllStringSubmatch(storePageText, -1) {
		items = append(items, StoreItem{Barcode: m[1], SKUOrUnit: m[3], OrderedUnitQty: m[4]})
	}
	return items
}

// JoinItemsWithPrices mirrors ghepgia_donhangbigc (xulydonhang.py:5888-5897):
// looks up each item's UnitPrice from the page-0 price list by barcode.
// An item whose barcode isn't in the price list silently gets UnitPrice
// 0 — Python's Vietnamese comment claims this "reports an error" but the
// actual code does not; faithfully reproduced, not "fixed" — a resulting
// 0 price surfaces downstream in bigc_processor.go as a price mismatch,
// same as any other genuinely-zero real price would.
func JoinItemsWithPrices(items []StoreItem, priceList []Product) []StoreItem {
	prices := make(map[string]float64, len(priceList))
	for _, p := range priceList {
		prices[p.Barcode] = p.UnitPrice
	}
	joined := make([]StoreItem, len(items))
	for i, item := range items {
		item.UnitPrice = prices[item.Barcode] // zero value if not found — matches Python's dict.get(article, 0)
		joined[i] = item
	}
	return joined
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/bigc/... -v`
Expected: PASS — all tests in the package (Tasks 3+4's plus this task's).

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/bigc/extract.go GO/internal/processing/bigc/extract_test.go
git commit -m "feat(go): add per-store name/item extraction and price join to bigc package"
```

---

### Task 6: `RealProcessor` — dispatch to BigC via `processBigcDocument`

**Files:**
- Modify: `GO/internal/processing/coop_processor.go` (only `Process`'s top-level dispatch — add a pre-check before the existing per-page loop, and one `case "BigC":` inside the loop as a defensive fallback)
- Create: `GO/internal/processing/bigc_processor.go`
- Create: `GO/internal/processing/bigc_processor_test.go`
- Create: `GO/internal/processing/testdata/sample_bigc_order.pdf` (copy of a real file)

**Interfaces:**
- Consumes: `vendor.Identify` (Task 2), `bigc.ParseOrderInfo/ExtractPriceList/ResolveCustomerCode/ExtractStoreName/ExtractStoreItems/JoinItemsWithPrices` (Tasks 3-5), and the shared `productdata.Store.ResolveSku/GetProductInfo`, `excelwriter.Row/WriteOrderRows`, `pricing.PricingSource.FetchIndex`, `coop.ExtractDiscount`.
- Produces: `RealProcessor.Process` now routes BigC files to `processBigcDocument`.

- [ ] **Step 1: Copy a real sample file into testdata**

```bash
cp "đơn hàng/08-2026/2631057733376.pdf" GO/internal/processing/testdata/sample_bigc_order.pdf
```

This file's real values (confirmed during planning by running the real Python functions directly against it): 20 pages (page 0 + 19 store pages), PO `2631057733376`, entry date `31/07/2026`, cancel date `04/08/2026` (the real "Deliver To Warehouse Before" field, NOT the +5-day fallback — confirmed by checking the fallback would compute `05/08/2026`, which does not match), customer code `MN_MT_BIGCAC`, delivery warehouse `FM LOGISTIC VSIP 2 (806)`, 16 products in the page-0 price list. Store pages include (in order): page1 `GO! DONG NAI` (6 items), page2 `GO! AN LAC` (4 items), page3/4 both `GO! DA NANG` (5 then 1 items — same store spanning 2 consecutive physical pages), ..., page19 `GO! NINH THUAN` (7 items).

- [ ] **Step 2: Write the failing test**

Create `GO/internal/processing/bigc_processor_test.go`. Follow the structure of `satra_processor_test.go`/`lotte_processor_test.go` for scaffolding (`copyTestWorkbookForProcessor`, `fixturePricingSource` — now in `golden_test_helpers_test.go` after Task 0, reuse, don't redeclare):

```go
package processing

import (
	"context"
	"testing"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleBigcFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	// Empty price index on purpose: this file's real barcodes aren't in
	// the small test fixture, so products are expected to come back as
	// price mismatches (Warning), not Done.
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(nil)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_bigc_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	// 19 store pages -> 19 OrderRows (one per store page, matching
	// Python's per-page-call saigia/tongtien scoping — see this plan's
	// Task 6 design notes on why this is "1 row per store", not "1 row
	// per file", despite all stores sharing one PO and one combined
	// Excel write).
	if len(rows) != 19 {
		t.Fatalf("Process returned %d rows, want 19: %+v", len(rows), rows)
	}
	for i, row := range rows {
		if row.System != "BigC" {
			t.Fatalf("rows[%d].System = %q, want %q", i, row.System, "BigC")
		}
		if row.PO != "2631057733376" {
			t.Fatalf("rows[%d].PO = %q, want %q", i, row.PO, "2631057733376")
		}
		if row.StatusKind != StatusKindWarning {
			t.Fatalf("rows[%d].StatusKind = %v, want %v (empty price index -> every product mismatches)", i, row.StatusKind, StatusKindWarning)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/... -run TestRealProcessor_ProcessesRealSampleBigcFile -v`
Expected: FAIL — BigC isn't routed yet.

- [ ] **Step 4: Add the pre-check to `Process`**

Read `GO/internal/processing/coop_processor.go`'s current `Process` function first to confirm its exact shape (Task 0/1 didn't touch this function, only the file's other functions, so it should be unchanged from before this plan). Add the pre-check **before** the existing `for pageIdx, text := range pageTexts` loop:

```go
func (p *RealProcessor) Process(ctx context.Context, filePath string, stt int) ([]OrderRow, error) {
	pageTexts, err := extractPageTexts(filePath)
	if err != nil {
		return []OrderRow{{
			FileName:   filepath.Base(filePath),
			Status:     StatusFailed + " - không đọc được PDF: " + err.Error(),
			StatusKind: StatusKindFailed,
		}}, nil
	}

	// BigC's identifying markers are present on every page of a real
	// BigC file (see vendor.Identify's bigcPattern doc comment), but
	// only page 0 carries the master price list, customer code, and
	// PO/dates every store page's row-building depends on — a per-page
	// dispatch can't supply that cross-page state. Pre-check page 0
	// specifically and, if it's BigC, hand the WHOLE file to
	// processBigcDocument instead of entering the per-page loop below.
	if len(pageTexts) > 0 && vendor.Identify(pageTexts[0]) == "BigC" {
		return p.processBigcDocument(filePath, pageTexts)
	}

	var rows []OrderRow
	for pageIdx, text := range pageTexts {
		// ... existing per-page loop, unchanged ...
```

The rest of the existing per-page loop (Coop/Lotte/Satra cases, `default` fallback) stays exactly as-is — do NOT add a `case "BigC":` inside it, since a BigC file never reaches this loop (the pre-check above always intercepts it). If you're tempted to add one "for completeness," don't — an unreachable case is dead code the compiler won't catch and a future reader will wrongly think is load-bearing.

- [ ] **Step 5: Implement `processBigcDocument` and its row-builder**

Create `GO/internal/processing/bigc_processor.go`:

```go
package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"order-processor/internal/processing/bigc"
	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/pricing"
)

// bigcRegionInfo mirrors the kho/khuvuc/mien branching inline in
// write_to_dondathang_bigc (xulydonhang.py:4569-4580) — a genuine 3-way
// split (MB / MN_MT / MN_GC) that the shared regionInfo (processor_shared.go)
// does NOT correctly cover: regionInfo only distinguishes "MB" vs
// everything else, giving every non-MB code the same warehouse
// ("LA_TP") — correct for BigC's MN_GC_BIGCAC code by coincidence, but
// WRONG for MN_MT_BIGCAC (needs "LA_KHO2026", not "LA_TP"). Extending
// the shared regionInfo instead of adding this BigC-only function would
// have silently changed Satra's already-shipped "MN_MT_*" customer
// codes' resolved warehouse (Satra's codes also start with "MN_MT") —
// confirmed during planning, not touched.
//
// Python has no else/default branch here (xulydonhang.py:4569-4580) —
// would raise UnboundLocalError if ever reached with an unmatched code.
// ResolveCustomerCode only ever returns the 4 codes covered below, so
// the default case is unreachable in practice; it returns the MN_GC
// branch's values defensively rather than panicking, since panicking
// mid-file would abort every remaining store page's processing for no
// evidence-backed reason.
func bigcRegionInfo(customerCode string) (region, statCode, warehouse string) {
	switch {
	case strings.HasPrefix(customerCode, "MB"):
		return "MT_MB", "HN", "TP_HN_12"
	case strings.HasPrefix(customerCode, "MN_MT"):
		return "MT_MN", "LA", "LA_KHO2026"
	case strings.HasPrefix(customerCode, "MN_GC"):
		return "MT_MN", "LA", "LA_TP"
	default:
		return "MT_MN", "LA", "LA_TP"
	}
}

// bigcOrderNumber mirrors write_to_dondathang_bigc's order-number field
// (xulydonhang.py:4613): f'ĐĐH{vendor}{STT_donhang_str}' where vendor is
// the uppercased literal "BIGC" and STT_donhang_str is f"-{po_number}"
// (same shape as every other vendor's order-number formatter in this
// codebase).
func bigcOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHBIGC-%s", poNumber)
}

// storePageResult is the outcome of processing ONE store page: either a
// set of rows to append to the file's combined Excel write, or a failure
// reason — never both. Mirrors the per-page isolation this plan's Global
// Constraints section commits to (a store page's failure never aborts
// other store pages, unlike Python's real, unguarded behavior).
type storePageResult struct {
	rows       []excelwriter.Row
	weightKg   float64
	saigia     int
	tongtien   float64
	err        error
}

// processBigcDocument mirrors process_file's BigC branch
// (xulydonhang.py:9404-9536) plus write_to_dondathang_bigc
// (:4541-4897), for the WHOLE file at once rather than Python's
// per-page-call design — see this plan's top-level Architecture note for
// why: Go's processBigcDocument already holds every page's text in
// memory, so it can accumulate every SUCCESSFUL store's rows into ONE
// excelwriter.WriteOrderRows call with a correctly pre-computed
// aggregate weight, instead of replicating Python's "write once per
// page, then re-read the sheet and overwrite a cell on the last page"
// mechanism — same final outcome (one combined block, one header, one
// aggregate weight total), simpler mechanism enabled by the chosen
// architecture, not a behavior change.
func (p *RealProcessor) processBigcDocument(filePath string, pageTexts []string) ([]OrderRow, error) {
	poNumber, entryDate, cancelDate, ok := bigc.ParseOrderInfo(pageTexts[0])
	if !ok {
		return []OrderRow{{
			FileName: filepath.Base(filePath), Page: fmt.Sprintf("1/%d", len(pageTexts)), System: "BigC",
			Status: StatusFailed + " - không tách được số PO/ngày đặt hàng từ trang 0", StatusKind: StatusKindFailed,
		}}, nil
	}
	priceList := bigc.ExtractPriceList(pageTexts[0])
	customerCode, deliveryWarehouse := bigc.ResolveCustomerCode(pageTexts[0])
	region, statCode, warehouse := bigcRegionInfo(customerCode)
	orderNum := bigcOrderNumber(poNumber)
	description := fmt.Sprintf("BIGC PO%s", poNumber)

	priceIndex, err := p.Pricing.FetchIndex("BIGC")
	if err != nil {
		return []OrderRow{{
			FileName: filepath.Base(filePath), Page: fmt.Sprintf("1/%d", len(pageTexts)), System: "BigC",
			Status: fmt.Sprintf("%s - không tải được giá/khuyến mãi: %v", StatusFailed, err), StatusKind: StatusKindFailed,
		}}, nil
	}

	var allRows []excelwriter.Row
	var totalWeight float64
	var orderRows []OrderRow
	headerWritten := false

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
		})
	}

	if len(allRows) > 0 {
		headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
		if err := excelwriter.WriteOrderRows(p.ExcelPath, allRows, headerDescription); err != nil {
			return nil, err
		}
	}

	return orderRows, nil
}

// processBigcStorePage handles ONE store page: extracts its name and
// item list, joins prices from the page-0 master list, price/promo
// matches every item, and builds this store's rows. isFirstSuccessful
// controls whether a header/note row is prepended (mirrors Python's
// header-only-on-page_num==1 behavior — but here it's "only on the
// first SUCCESSFULLY processed store", since a failed page 1 must not
// prevent page 2 from getting the header).
func (p *RealProcessor) processBigcStorePage(storePageText string, priceList []bigc.Product, priceIndex *pricing.Index,
	orderNum, entryDate, cancelDate, customerCode, shipTo, description, warehouse, region, statCode string, isFirstSuccessful bool,
) storePageResult {
	storeName, ok := bigc.ExtractStoreName(storePageText)
	if !ok {
		return storePageResult{err: fmt.Errorf("không tách được tên store")}
	}
	rawItems := bigc.ExtractStoreItems(storePageText)
	if len(rawItems) == 0 {
		return storePageResult{err: fmt.Errorf("không trích xuất được sản phẩm nào cho store %q", storeName)}
	}
	items := bigc.JoinItemsWithPrices(rawItems, priceList)

	var rows []excelwriter.Row
	if isFirstSuccessful {
		rows = append(rows, excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
			Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
			StatCode: statCode, IsNoteRow: true, ProductName: description,
		})
	}

	var weightKg, tongtien float64
	saigia := 0

	for _, item := range items {
		barcode := p.Store.ResolveSku(item.Barcode)
		productInfo, _ := p.Store.GetProductInfo(barcode)

		skuOU := parseNumericField(item.SKUOrUnit)
		ouQty := parseNumericField(item.OrderedUnitQty)
		qtyOrdPcs := ouQty * skuOU // xulydonhang.py:4642 — item["OU Qty"] * item["SKU/OU"], NOT "OU Qty" alone

		lineWeight := productInfo.WeightKg * qtyOrdPcs
		weightKg += lineWeight

		invoicePrice := item.UnitPrice // giahoadon: the joined-in per-unit price from page 0
		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice := parseNumericField(realPriceStr) // giathuctegoc
		finalPrice := realPrice                        // giathucte, mutates through the promo loop below

		promos := priceIndex.FindPromotions(barcode, entryDate)
		khuyenmai := ""
		matched := false

		for _, promo := range promos {
			if promo.Value == "" {
				continue
			}
			khuyenmai = promo.Value
			if discount := coop.ExtractDiscount(promo.Value); discount != 0 {
				// xulydonhang.py:4685 recomputes from the ORIGINAL
				// fetched price (giathuctegoc), not from whatever
				// finalPrice already holds from a prior iteration —
				// but when discount == 0 for an iteration, Python's
				// "else: giathucte = giathucte" is a literal no-op, so
				// finalPrice deliberately carries over UNCHANGED from
				// the previous iteration in that case (not reset to
				// realPrice) — port this exact quirk, do not "fix" it
				// by resetting finalPrice every iteration.
				finalPrice = realPrice - (realPrice * discount / 100)
			}
			if closeEnough(invoicePrice, finalPrice) {
				matched = true
				break
			}
		}
		if len(promos) == 0 && closeEnough(invoicePrice, finalPrice) {
			matched = true
		}

		productRow := excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
			Description: description, SKU: barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: qtyOrdPcs, UnitPrice: finalPrice,
			ProductName: productInfo.Name, LineWeightKg: lineWeight, UseZFormula: true, PromoContent: khuyenmai,
		}
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
		}
		rows = append(rows, productRow)
		tongtien += finalPrice * qtyOrdPcs // xulydonhang.py:4749 — uses qtyOrdPcs BEFORE any promo-bonus division below

		// Promo bonus-row check (xulydonhang.py:4754-4808). BigC has NO
		// khuyenmai.split('|') loop (confirmed structurally different
		// from Coop/Satra during planning) — exactly one bonus row per
		// item, driven by the single khuyenmai string this item ended
		// up with.
		bonusSku := p.Store.FindSkusMentioned(khuyenmai)
		bonusQty := qtyOrdPcs
		bonusBarcode := ""
		if len(bonusSku) > 0 {
			bonusBarcode = strings.Join(bonusSku, ", ")
		}
		if xm := xPlus1Pattern.FindStringSubmatch(khuyenmai); xm != nil {
			// xPlus1Pattern is already in scope here — it's declared in
			// processor_shared.go (Task 0), same package `processing`
			// as this file, so no new import or helper is needed.
			x, _ := strconv.Atoi(xm[1])
			if bonusBarcode == "" {
				bonusBarcode = barcode
			}
			if x >= 2 {
				bonusQty = math.Floor(qtyOrdPcs / float64(x))
			}
		}
		if bonusBarcode != "" {
			bonusInfo, _ := p.Store.GetProductInfo(bonusBarcode)
			bonusWeight := bonusInfo.WeightKg * bonusQty
			weightKg += bonusWeight

			// xulydonhang.py:4769's laycachbo_khuyenmai(value) uses the
			// leftover "value" loop variable from the promo-matching
			// loop above, not "khuyenmai" — this plan's Global
			// Constraints section documents this as a confirmed Python
			// quirk NOT being ported; using khuyenmai (this item's own
			// resolved promo string) here instead, per Phase 2b's
			// "correct main flow" policy. Flag via knownDivergences_BigC
			// during Task 8 if a real fixture traces a mismatch to this.
			bundleNote := coop.ExtractBraceContent(khuyenmai)
			bonusRow := excelwriter.Row{
				EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
				Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
				Description: description, SKU: bonusBarcode, Warehouse: warehouse, VATPercent: 8,
				RegionCode: region, StatCode: statCode, IsPromoItem: true, Qty: bonusQty,
				ProductName: bonusInfo.Name, LineWeightKg: bonusWeight, UseZFormula: false,
			}
			if bundleNote != "" {
				bonusRow.PromoNote = bundleNote
			} else {
				// BigC's per-item no-brace fallback text is genuinely
				// different from Coop/Satra's "KM Bó Kèm - Che Barcode"
				// (xulydonhang.py:4777: "KM Rời - Không Che Barcode") —
				// confirmed during planning, not a transcription error.
				bonusRow.PromoNote = "KM Rời - Không Che Barcode"
			}
			rows = append(rows, bonusRow)
		}
	}

	return storePageResult{rows: rows, weightKg: weightKg, saigia: saigia, tongtien: tongtien}
}

// parseNumericField mirrors the repeated "strip commas, coerce to
// float/int" pattern applied to item["SKU/OU"] and item["OU Qty"]
// (xulydonhang.py:4632-4640) and to a fetched price string — returns 0
// on any parse failure rather than panicking, since a malformed numeric
// field should surface as a price mismatch / zero quantity downstream,
// not crash the whole store page.
func parseNumericField(s string) float64 {
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil {
		return 0
	}
	return v
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run:
```
cd GO && go build ./... && go vet ./... && go test ./internal/processing/... -v
```
Expected: PASS — the new BigC test, and every existing Coop/Lotte/Satra test unchanged from Task 1's baseline (Coop's golden-fixture test still at its documented pre-existing baseline, Lotte's still 60/60, Satra's still 36/36).

- [ ] **Step 7: Commit**

```bash
git add GO/internal/processing/coop_processor.go GO/internal/processing/bigc_processor.go GO/internal/processing/bigc_processor_test.go GO/internal/processing/testdata/sample_bigc_order.pdf
git commit -m "feat(go): dispatch RealProcessor to BigC via processBigcDocument"
```

---

### Task 7: Golden fixture generation script (throwaway) — generate BigC fixtures

**Files:**
- Create: `GO/internal/processing/bigc/testdata/generate_fixtures.py` (throwaway dev tool, adapted from `GO/internal/processing/satra/testdata/generate_fixtures.py`)

**Interfaces:**
- Consumes: the real `xulydonhang.py`'s `ProcessHandler.trichxuatinfo_donbigc`, `laydanhsachsanpham_bigc`, `trichxuatdanhsachforstore_bigc`, `ghepgia_donhangbigc`, `lay_ten_store`, `write_to_dondathang_bigc`, `identify_vendor`, `find_price_by_sku`, `find_all_promotions_by_sku_and_time`, `get_gid` — all unmodified.
- Produces: `GO/internal/processing/bigc/testdata/fixtures/*.json` + `_frozen_pricing.json`. Consumed by Task 8.

- [ ] **Step 1: Write the script**

Create `GO/internal/processing/bigc/testdata/generate_fixtures.py`, adapted directly from `GO/internal/processing/satra/testdata/generate_fixtures.py` (read it first) — same `REPO_ROOT` resolution (6 `dirname()` calls, same directory depth: `GO/internal/processing/bigc/testdata/generate_fixtures.py`), same UTF-8 stdout fix, same production-`dondathang.xlsx` backup/restore protocol, same price/promo caching monkeypatch (already generic over `sheet_name`, works for `"BIGC"` with no changes), same `upload_file_to_drive` no-op patch. Only `is_bigc_pdf`/`process_one_pdf` are BigC-specific — and BigC's `process_one_pdf` must call the REAL `process_file`-equivalent flow across an ENTIRE file's pages (page 0 setup, then every store page's `write_to_dondathang_bigc` call), not a single-page call like Coop/Lotte/Satra's harnesses:

```python
def is_bigc_pdf(path):
    doc = xulydonhang.fitz.open(path)
    try:
        text = ""
        for page in doc:
            text += page.get_text("text")
        return xulydonhang.ProcessHandler.identify_vendor(text) == "BigC"
    finally:
        doc.close()


def process_one_pdf(path):
    """Mirrors the BigC branch of process_file (xulydonhang.py:9404-9536)
    for a whole file: page 0 sets up shared state (po_number, entry_date,
    cancel_date, products, makhachhang, diachigiao), then every
    subsequent page calls write_to_dondathang_bigc once as a store page —
    skipping the Google Drive upload side effect (already no-op'd by the
    monkeypatch above)."""
    handler = xulydonhang.process_handler
    doc = xulydonhang.fitz.open(path)
    try:
        page0_text = doc[0].get_text("text")
        if xulydonhang.ProcessHandler.identify_vendor(page0_text) != "BigC":
            return  # first page isn't BigC after all; skip whole file

        po_number, entry_date, cancel_date = xulydonhang.ProcessHandler.trichxuatinfo_donbigc(page0_text)
        products = xulydonhang.ProcessHandler.laydanhsachsanpham_bigc(page0_text)

        trangdaubigc = page0_text
        if "3006900" in trangdaubigc and "LINFOX WAREHOUSE (802)" in trangdaubigc:
            makhachhang = "MB_GC_BIGC"; diachigiao = "LINFOX WAREHOUSE (802)"
        elif "3005382" in trangdaubigc and "LINFOX WAREHOUSE (802)" in trangdaubigc:
            makhachhang = "MB_MT_BIGC"; diachigiao = "LINFOX WAREHOUSE (802)"
        elif "3005382" in trangdaubigc and "FM LOGISTIC VSIP 2 (806)" in trangdaubigc:
            makhachhang = "MN_MT_BIGCAC"; diachigiao = "FM LOGISTIC VSIP 2 (806)"
        elif "3006900" in trangdaubigc and "FM LOGISTIC VSIP 2 (806)" in trangdaubigc:
            makhachhang = "MN_GC_BIGCAC"; diachigiao = "FM LOGISTIC VSIP 2 (806)"
        else:
            makhachhang = "MN_MT_BIGCAC"; diachigiao = "FM LOGISTIC VSIP 2 (806)"

        vendor = "BIGC"
        start_row = None

        for page_num in range(1, len(doc)):
            text = doc[page_num].get_text("text")
            tenstore = xulydonhang.ProcessHandler.lay_ten_store(text)
            items = xulydonhang.ProcessHandler.trichxuatdanhsachforstore_bigc(text)

            if page_num == 1:
                # Mirror process_file's start_row capture (xulydonhang.py's
                # equivalent bookkeeping right before the first
                # write_to_dondathang_bigc call) — read the CURRENT sheet's
                # next row before this write, so this fixture-generation
                # harness's snapshot_rows call below (same helper Coop/
                # Lotte/Satra's harnesses already use) captures every row
                # from here through the end of this whole file's processing.
                wb = xulydonhang.openpyxl.load_workbook(xulydonhang.process_handler.dondathang_path if hasattr(xulydonhang.process_handler, "dondathang_path") else "dondathang.xlsx")
                start_row = wb["Don dat hang"].max_row + 1
                wb.close()

            bat_dau = False
            url = None
            if page_num == len(doc) - 1:
                bat_dau = start_row
                url = "https://example.invalid/skipped-during-fixture-generation"

            xulydonhang.ProcessHandler.write_to_dondathang_bigc(
                handler, products, items, po_number, entry_date, cancel_date,
                tenstore, 1, makhachhang, vendor, page_num, diachigiao, bat_dau, url,
            )
    finally:
        doc.close()
```

**Implementer note:** the `start_row` capture above (`wb["Don dat hang"].max_row + 1`, read via a fresh `openpyxl.load_workbook` before page 1's write) mirrors what `process_file` itself does around its own BigC branch's page-0 setup (`xulydonhang.py`, search for where `start_row` is assigned near line 9450-9455 per this plan's research) — **read that exact code in `xulydonhang.py` before transcribing this block**, since the harness must capture the row count at the SAME point Python does (this plan's research did not extract the exact `start_row` line, only that it's "captured once at page 0, lines 9450-9455" — confirm the precise mechanism, e.g. does it read `wb.max_row` from an already-open workbook handle `process_file` holds, or reopen the file — and match it exactly, since an off-by-one here would corrupt every fixture's row range).

Everything else (`main`, `snapshot_rows`, `COLUMNS`, the pricing-cache monkeypatch, the backup/restore protocol) is copied verbatim from the Satra harness, changing only: `FIXTURES_DIR` → `.../bigc/testdata/fixtures`, `is_satra_pdf`/`process_one_pdf` → the BigC versions above, and the final frozen-pricing capture call to `_capture_promo_raw_rows("BIGC")`. **Unlike Satra/Lotte's harnesses (which call the per-page write function once and immediately snapshot), this harness must snapshot AFTER the entire per-store-page loop completes** — call `snapshot_rows(real_target, start_row)` once, after the `for page_num in range(1, len(doc))` loop finishes, capturing every row written across every store page of this one file as a SINGLE fixture (matching this plan's Task 6 design: one combined Excel write per file, `fixtureData.Rows` already supports an arbitrary row count per fixture — no shape change needed).

- [ ] **Step 2: Back up the production workbook before running (safety)**

```bash
cp dondathang.xlsx dondathang.xlsx.manual_backup_before_bigc_fixtures
```

- [ ] **Step 3: Run the script**

```bash
.venv/Scripts/python.exe GO/internal/processing/bigc/testdata/generate_fixtures.py
```

Expected: "Found N candidate PDFs" (N is whatever the current total in `đơn hàng/08-2026/` is — do not assume it's still 319, more may have been added since this plan was written), then one `OK`/`SKIP` line per BigC file, ending with "Done: 29 fixtures generated, 0 PDFs skipped" (29 is the count established during this plan's research — if it differs, that's fine as long as every non-generated file has a clear SKIP reason; investigate before proceeding if any file silently produces neither).

- [ ] **Step 4: Verify the production workbook is untouched**

```bash
diff dondathang.xlsx dondathang.xlsx.manual_backup_before_bigc_fixtures && echo "IDENTICAL — safe" || echo "DIFFERS — investigate before proceeding, do not continue"
```

If it differs: STOP, restore immediately (`mv dondathang.xlsx.manual_backup_before_bigc_fixtures dondathang.xlsx`), investigate before doing anything else.

- [ ] **Step 5: Remove the manual backup once confirmed identical**

```bash
rm dondathang.xlsx.manual_backup_before_bigc_fixtures
```

- [ ] **Step 6: Spot-check a few generated fixtures**

Read 2-3 files under `GO/internal/processing/bigc/testdata/fixtures/*.json` and confirm plausible values (PO-shaped `B` column like `ĐĐHBIGC-...`, non-empty `S` product names, sane `X`/`Y`/`AT` values, and — since BigC's fixtures should each contain MANY rows spanning multiple stores — confirm the row count for a multi-store file looks like the sum of all its stores' item counts, not just one store's worth).

- [ ] **Step 7: Commit**

```bash
git add GO/internal/processing/bigc/testdata/generate_fixtures.py GO/internal/processing/bigc/testdata/fixtures/
git commit -m "test(go): generate golden fixtures for BigC from real PDFs + production output"
```

---

### Task 8: Golden fixture integration test

**Files:**
- Create: `GO/internal/processing/bigc_golden_test.go`

**Interfaces:**
- Consumes: everything from Tasks 0-7; reuses `fixtureData`, `frozenPricingFixture`, `fixturePricingSource`, `compareRowsAgainstFixture`, `stringify`, `toFloat`, `floatCloseEnough`, `copyFile`, `joinLines` — all now in `golden_test_helpers_test.go` after Task 0.
- Produces: `TestRealProcessor_MatchesGoldenFixtures_BigC`.

- [ ] **Step 1: Write `bigc_golden_test.go`**

Create `GO/internal/processing/bigc_golden_test.go`, following `satra_golden_test.go`'s exact structure:

```go
package processing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

// knownDivergences_BigC lists (fixture, row index, column) cells where
// this Go port intentionally computes a different, verified-more-correct
// value than the frozen Python fixture. Key format:
// "<source_pdf>:<row index>:<column>". Empty until a real, hand-verified
// case is confirmed; add entries here only with a comment citing the
// specific PDF/Python-line evidence — never to silence an unexplained
// diff. See this plan's Global Constraints for the specific,
// already-anticipated "leftover value variable in laycachbo_khuyenmai"
// case this may be needed for.
var knownDivergences_BigC = map[string]bool{}

func loadFrozenBigcPricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("bigc/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen BigC pricing fixture found (run Task 7's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen BigC pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

func TestRealProcessor_MatchesGoldenFixtures_BigC(t *testing.T) {
	fixturePaths, err := filepath.Glob("bigc/testdata/fixtures/*.json")
	if err != nil {
		t.Fatalf("failed globbing fixtures: %v", err)
	}
	var realFixtures []string
	for _, p := range fixturePaths {
		if filepath.Base(p) != "_frozen_pricing.json" {
			realFixtures = append(realFixtures, p)
		}
	}
	if len(realFixtures) == 0 {
		t.Skip("no golden fixtures found (run Task 7's generate_fixtures.py first)")
	}

	store, err := productdata.Load("../../../data.xlsx")
	if err != nil {
		t.Fatalf("failed loading production data.xlsx: %v", err)
	}
	pricingSource := loadFrozenBigcPricingSource(t)

	var mismatches []string
	for _, fixturePath := range realFixtures {
		raw, err := os.ReadFile(fixturePath)
		if err != nil {
			t.Fatalf("failed reading %s: %v", fixturePath, err)
		}
		var fixture fixtureData
		if err := json.Unmarshal(raw, &fixture); err != nil {
			t.Fatalf("failed parsing %s: %v", fixturePath, err)
		}

		pdfPath := filepath.Join("..", "..", "..", "đơn hàng", "08-2026", fixture.SourcePDF)
		excelPath := filepath.Join(t.TempDir(), "dondathang.xlsx")
		copyFile(t, "excelwriter/testdata/dondathang.xlsx", excelPath)

		rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
		rows, err := rp.Process(context.Background(), pdfPath, 1)
		if err != nil {
			mismatches = append(mismatches, fixture.SourcePDF+": Process returned error: "+err.Error())
			continue
		}
		if len(rows) == 0 {
			mismatches = append(mismatches, fixture.SourcePDF+": Process produced no rows")
			continue
		}
		for _, row := range rows {
			if row.StatusKind == StatusKindFailed {
				mismatches = append(mismatches, fixture.SourcePDF+": Process produced a Failed row: "+row.Status)
			}
		}

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_BigC)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
}
```

**Note on the multi-`OrderRow`-per-file shape:** unlike Lotte/Satra's golden tests (which assert `len(rows) == 1`), BigC's `Process` call returns MULTIPLE `OrderRow`s per file (one per store page, per Task 6's design) — the test above checks every returned row for `StatusKindFailed` individually instead of asserting a single row's status, and relies on `compareRowsAgainstFixture`'s own Excel-row-range comparison (which reads the actual written sheet, not the `[]OrderRow` return value) to validate the combined write's content. This matches how the fixture itself was captured in Task 7 (one fixture per FILE, covering every store's rows combined).

- [ ] **Step 2: Run the test**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures" -v`

Expected: Coop's and Lotte's and Satra's tests still report their exact unchanged baselines. BigC's test will very likely fail on the first run with real mismatches — this is the actual verification work of this task, same as it was for Lotte's Task 9 and Satra's Task 7.

- [ ] **Step 3: Root-cause and fix every mismatch**

For each mismatch: read the specific fixture JSON and the source PDF, trace through `xulydonhang.py`'s actual BigC functions at the cited line numbers, and determine whether it's (a) a bug in this plan's Go port — fix the Go code; or (b) a case where Python is genuinely wrong or where Go's whole-file-at-once architecture legitimately produces a different (more correct, or simply differently-mechanized-but-equivalent) result than Python's per-page-call mechanism — add a precise, evidence-citing entry to `knownDivergences_BigC` using the `sourcePDF:row:col` key format. Do not guess; every fix or allowlist entry must be traceable to specific evidence. Re-run after each fix.

Specific things likely to need investigation, flagged during planning:
- The `laycachbo_khuyenmai(value)` vs `khuyenmai` leftover-variable quirk (this plan's Global Constraints) — check any `AO`-column mismatch against this first.
- The exact final header-row weight text format (this plan's Task 6 uses the same `"%s (Tổng trọng lượng: %s)"` shape already established for Coop/Lotte/Satra — confirmed NOT verified against BigC's real final-overwrite format during planning; if the `L`-column mismatch is purely about text format/wording rather than the weight NUMBER being wrong, this is the most likely place to look).
- `qty_ord_pcs`'s promo-driven floor-division (`xulydonhang.py:4763`) happening AFTER `tongtien`'s accumulation (`:4749`) but BEFORE the bonus row's own `X` value is written (`:4792`) — confirm the Go port's `bonusQty` (not `qtyOrdPcs` itself) is what's floor-divided, and that `tongtien`/`AT` on the MAIN product row still use the un-divided `qtyOrdPcs`.

If some failures turn out to be PDF-text-extraction-fidelity gaps (the same category of limitation Phase 2a's Coop plan allowed for, though Lotte and Satra ultimately needed none beyond `stripBlankLines`/`normalizeSatraText`-style preprocessing), document them the same way — and if a preprocessing step turns out to be needed (same pattern as Satra's `normalizeSatraText`), add it to `bigc_processor.go`, not `bigc/extract.go`'s already-reviewed regexes.

- [ ] **Step 4: Final run and commit**

Run: `cd GO && go build ./... && go vet ./... && go test ./... -v`
Expected: clean build/vet, all tests pass (or fail only with fully documented, understood, non-logic-bug gaps — Coop's pre-existing unrelated failure is the one known exception).

```bash
git add GO/internal/processing/bigc_golden_test.go
git commit -m "test(go): add BigC golden fixture integration test"
```
