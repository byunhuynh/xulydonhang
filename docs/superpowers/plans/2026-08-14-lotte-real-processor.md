# Lotte RealProcessor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the existing Go `RealProcessor` (built for Coop in Phase 2a) to also parse real Lotte purchase-order PDFs, compute pricing/promotions, and write results into the same "Don dat hang" Excel sheet — validated against 60 real archived Lotte PDFs via the same golden-fixture methodology used for Coop.

**Architecture:** New `GO/internal/processing/lotte/` package for Lotte-specific PDF extraction (PO#/date, cancel date, store name, product list). A new `processLotteSegment` method on the existing `RealProcessor` struct, added as a new dispatch branch inside `Process`, reuses the Coop-built promo/pricing/bonus-row machinery (`coop.ExtractDiscount`, `coop.ExtractBraceContent`, `coop.ExtractMoneyAmount`, `coop.LastFourDigits`, `coop.FormatWeightKg`, `productdata.FindSkusMentioned`, `regionInfo`, `closeEnough`, `buildPromoBonusRow`, `buildInvoiceBonusRow`) almost unchanged — Lotte's `write_to_dondathang_lotte` and Coop's `write_to_dondathang` share the exact same promo-matching and bonus-row logic in `xulydonhang.py`, confirmed by direct source comparison during design. `excelwriter.Row`/`WriteOrderRows` is reused with zero changes (same sheet, same columns, confirmed identical layout).

**Tech Stack:** Same as Phase 2a — Go, `github.com/xuri/excelize/v2`, `github.com/ledongthuc/pdf` (already wired, no changes needed to PDF text extraction itself).

**Spec:** [2026-08-14-lotte-real-processor-design.md](../specs/2026-08-14-lotte-real-processor-design.md)

## Global Constraints

- **Testing/divergence policy (differs from Phase 2a's Coop plan):** the golden-fixture test compares against real Python output like Coop's did, but when Go intentionally computes a *different, verified-more-correct* value because Python is confirmed wrong on that specific field, the difference is recorded in an explicit, commented allowlist in the test — NOT silently skipped, NOT forced to match Python's wrong value, and the fixture JSON itself is never edited. Every other mismatch is a real bug to fix, same as Phase 2a. See Task 9.
- **Never edit fixture JSON files** under `GO/internal/processing/lotte/testdata/fixtures/` to force a pass — they are machine-generated ground truth from Task 8's harness.
- **Reuse, do not reimplement**, these already-shipped Phase 2a functions (all in package `processing` or its `coop`/`productdata` subpackages, already tested, already reviewed): `coop.ExtractDiscount`, `coop.ExtractBraceContent`, `coop.ExtractMoneyAmount`, `coop.LastFourDigits`, `coop.FormatWeightKg`, `productdata.Store.FindSkusMentioned`, `productdata.Store.ResolveSku`, `productdata.Store.GetProductInfo`, the unexported `regionInfo`, `closeEnough`, `buildPromoBonusRow`, `buildInvoiceBonusRow` functions in `coop_processor.go`, and `excelwriter.Row`/`excelwriter.WriteOrderRows`. Confirmed during design that Lotte's Python source (`write_to_dondathang_lotte`, `xulydonhang.py:1968-2318`) uses the exact same helper functions as Coop's `write_to_dondathang`, just with vendor="Lotte" instead of "COOPMART"/"COOPFOOD".
- **Package for new Lotte-only extraction logic:** `GO/internal/processing/lotte/`, mirroring the shape of the existing `GO/internal/processing/coop/` package.
- All new Go code follows the same conventions already established in the `coop`/`productdata`/`pricing` packages: exported functions get a doc comment citing the exact `xulydonhang.py` line range they mirror; every deviation from a literal Python behavior gets an inline comment explaining why.
- Run `go build ./...`, `go vet ./...`, and the relevant `go test` scope after every task, from the `GO/` directory.

---

### Task 1: `vendor.Identify` — recognize Lotte

**Files:**
- Modify: `GO/internal/processing/vendor/identify.go`
- Modify: `GO/internal/processing/vendor/identify_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `Identify(text string) string` now also returns `"Lotte"` — consumed by Task 7's dispatch in `RealProcessor.Process`.

- [ ] **Step 1: Write the failing test**

Add to `GO/internal/processing/vendor/identify_test.go`:

```go
func TestIdentify_RecognizesLotteByVendorTaxID(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"tax id form 1", "Ven cd: 0107889783 009333 CONG TY CP HA THANH", "Lotte"},
		{"tax id form 2", "1102018142 010544 CONG TY CP HA THANH", "Lotte"},
		{"tax id split across lines", "0107889783\n  009333\nCONG TY CP HA THANH", "Lotte"},
		{"unrelated tax id", "0107889783 999999", ""},
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/vendor/... -run TestIdentify_RecognizesLotteByVendorTaxID -v`
Expected: FAIL — `Identify` returns `""` for every case (Lotte pattern doesn't exist yet).

- [ ] **Step 3: Implement**

Edit `GO/internal/processing/vendor/identify.go` (existing file):

```go
package vendor

import (
	"regexp"
	"strings"
)

var (
	whitespacePattern = regexp.MustCompile(`\s+`)
	// Coop's two internal vendor IDs, exactly as in
	// xulydonhang.py:identify_vendor's first branch (checked first
	// because it's checked first there — order matters for the other
	// ~18 vendor branches that exist in Python but aren't ported here).
	coopPattern = regexp.MustCompile(`Vendor\s*[-:]\s*(21569|22856)`)
	// Lotte's two tax-ID forms, mirroring identify_vendor's second
	// branch (xulydonhang.py:102-103): either of two ID pairs appearing
	// anywhere in the (whitespace-normalized) page text.
	lottePattern = regexp.MustCompile(`0107889783\s*009333|1102018142\s*010544`)
)

// Identify tries to recognize which retail vendor produced this
// page/PO text, mirroring xulydonhang.py's identify_vendor. Coop and
// Lotte are implemented; every other vendor is a later phase's work, so
// Identify returns "" for anything that isn't one of those two.
func Identify(text string) string {
	cleaned := strings.TrimSpace(whitespacePattern.ReplaceAllString(text, " "))
	if coopPattern.MatchString(cleaned) {
		return "Coop"
	}
	if lottePattern.MatchString(cleaned) {
		return "Lotte"
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/vendor/... -v`
Expected: PASS — all Coop tests still pass (regression check), all new Lotte tests pass.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/vendor/identify.go GO/internal/processing/vendor/identify_test.go
git commit -m "feat(go): recognize Lotte vendor in identify.Identify"
```

---

### Task 2: Generalize `pricing.HTTPSource` to fetch any vendor's sheet

**Files:**
- Modify: `GO/internal/processing/pricing/http_source.go`
- Modify: `GO/internal/processing/coop_processor.go`
- Modify: `GO/internal/processing/coop_processor_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `HTTPSource.FetchIndex(sheetKey string) (*Index, error)` (replaces `FetchCoopIndex()`); `PricingSource` interface's method renamed to match. Task 7 calls `p.Pricing.FetchIndex("LOTTE")`.

**Why this is safe:** confirmed in `xulydonhang.py` (`find_price_by_sku:5584-5610`, `laygiathucte_CNHCM:5616`, and 2 other functions in the same block) that every vendor's pricing lookup uses the SAME hardcoded Google Sheet ID (`1yvxE_SPYXKhofcZdhv1CSKAyiwdY1Mf4pFlsiMbtOr4`), differing only by `gid` — resolved via a `sheet_name` parameter (Coop's call site always passes `"COOP"`). `settings.ini` already has `LOTTE = 435921079` in its `<gid>` block.

- [ ] **Step 1: Update `HTTPSource`**

Edit `GO/internal/processing/pricing/http_source.go` (existing file), replacing the whole file:

```go
package pricing

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"
)

const spreadsheetID = "1yvxE_SPYXKhofcZdhv1CSKAyiwdY1Mf4pFlsiMbtOr4"

// HTTPSource fetches a vendor's live pricing/promotion sheet over HTTP.
// Every vendor's sheet lives in the same Google Sheet (spreadsheetID),
// on a different tab selected by gid — confirmed in xulydonhang.py's
// find_price_by_sku/find_all_promotions_by_sku_and_time family, which
// all hardcode the same sheet_id and vary only the sheet_name param used
// to resolve gid. It is the production PricingSource; tests substitute a
// fixture-backed implementation instead of hitting the network.
type HTTPSource struct {
	SettingsPath string
	Client       *http.Client
}

func NewHTTPSource(settingsPath string) *HTTPSource {
	return &HTTPSource{SettingsPath: settingsPath, Client: &http.Client{Timeout: 30 * time.Second}}
}

// FetchIndex mirrors find_price_by_sku/find_all_promotions_by_sku_and_time's
// URL construction: sheetKey is the same value as their sheet_name
// parameter (e.g. "COOP", "LOTTE" — must match a key in settings.ini's
// <gid> block), resolved to a gid and fetched once.
func (s *HTTPSource) FetchIndex(sheetKey string) (*Index, error) {
	gidMap, err := LoadGidMap(s.SettingsPath)
	if err != nil {
		return nil, err
	}
	gid, ok := gidMap[sheetKey]
	if !ok {
		return nil, fmt.Errorf("pricing: no %s gid in %s", sheetKey, s.SettingsPath)
	}

	url := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/gviz/tq?tqx=out:csv&gid=%s", spreadsheetID, gid)
	resp, err := s.Client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("pricing: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pricing: fetch %s: HTTP %d", url, resp.StatusCode)
	}

	reader := csv.NewReader(resp.Body)
	reader.FieldsPerRecord = -1 // rows can have varying column counts
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("pricing: parse CSV from %s: %w", url, err)
	}

	return ParseIndex(rows), nil
}
```

- [ ] **Step 2: Update the `PricingSource` interface and its Coop call site**

Edit `GO/internal/processing/coop_processor.go`: change

```go
type PricingSource interface {
	FetchCoopIndex() (*pricing.Index, error)
}
```

to

```go
// PricingSource abstracts fetching a vendor's price/promotion data for
// one order, so tests substitute a fixture-backed implementation instead
// of a live Google Sheets fetch. Production wiring uses *pricing.HTTPSource.
type PricingSource interface {
	FetchIndex(sheetKey string) (*pricing.Index, error)
}
```

and change the one call site (currently `priceIndex, err := p.Pricing.FetchCoopIndex()`) to:

```go
	priceIndex, err := p.Pricing.FetchIndex("COOP")
```

- [ ] **Step 3: Update the test double**

Edit `GO/internal/processing/coop_processor_test.go`: change

```go
func (f *fixturePricingSource) FetchCoopIndex() (*pricing.Index, error) {
	return f.index, nil
}
```

to

```go
func (f *fixturePricingSource) FetchIndex(sheetKey string) (*pricing.Index, error) {
	return f.index, nil
}
```

(`sheetKey` is intentionally ignored — the test double wraps a single already-selected `*pricing.Index`, same as before the rename.)

- [ ] **Step 4: Run the full existing suite to confirm no regression**

Run: `cd GO && go build ./... && go vet ./... && go test ./internal/processing/... -v`
Expected: PASS — every existing Coop test (`coop_processor_test.go`, `coop_golden_test.go` — the latter should still report 93/155, unchanged) still passes; this task must not change Coop's behavior or pass count at all, only the method name/signature.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/pricing/http_source.go GO/internal/processing/coop_processor.go GO/internal/processing/coop_processor_test.go
git commit -m "refactor(go): generalize pricing fetch from Coop-only to any vendor sheet"
```

---

### Task 3: `productdata.Store` — Lotte-style customer code lookup

**Files:**
- Modify: `GO/internal/processing/productdata/store.go`
- Modify: `GO/internal/processing/productdata/store_test.go`

**Interfaces:**
- Consumes: `Store.customerRows [][4]string` (already loaded by `Load`).
- Produces: `(*Store) GetCustomerCodeBySuffix(system, storeCode string) string` — consumed by Task 7.

- [ ] **Step 1: Write the failing test**

Add to `GO/internal/processing/productdata/store_test.go` (the existing `testdata/data.xlsx` fixture already has a `LOTTE 777 KH-LOTTE-003` row in its `MaKH` sheet — added ahead of this task specifically for this test):

```go
func TestGetCustomerCodeBySuffix_MatchesLotteBySystemAndSuffix(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := store.GetCustomerCodeBySuffix("LOTTE", "003"); got != "KH-LOTTE-003" {
		t.Fatalf("GetCustomerCodeBySuffix(LOTTE, 003) = %q, want %q", got, "KH-LOTTE-003")
	}
	// "001" is a real suffix of the COOP row's column C ("KH-COOP-001")
	// — querying it under system "LOTTE" must NOT cross over and match
	// that row; the system filter must be applied first.
	if got := store.GetCustomerCodeBySuffix("LOTTE", "001"); got != "" {
		t.Fatalf("GetCustomerCodeBySuffix(LOTTE, 001) = %q, want empty (system filter must exclude the COOP row)", got)
	}
	if got := store.GetCustomerCodeBySuffix("LOTTE", "999"); got != "" {
		t.Fatalf("GetCustomerCodeBySuffix(LOTTE, 999) = %q, want empty (no matching suffix)", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/productdata/... -run TestGetCustomerCodeBySuffix -v`
Expected: FAIL with `undefined: GetCustomerCodeBySuffix` (compile error).

- [ ] **Step 3: Implement**

Add to `GO/internal/processing/productdata/store.go` (existing file), after `GetCustomerCode`:

```go
// GetCustomerCodeBySuffix mirrors get_makhachhang_lotte
// (xulydonhang.py:307-321): filters customer rows to the given system
// (column A, case-insensitive exact match), then returns the first
// row's column C value whose trimmed content ends with storeCode.
// Unlike GetCustomerCode (Coop's get_makhachhang, which has a genuine
// double-read-column-C bug preserved from Python), this reads columns A
// and C correctly — get_makhachhang_lotte also reads column B into a
// variable but never actually uses it in its comparison, so that read is
// simply omitted here (dead in the original, not a behavior to
// replicate). Returns "" (not Python's None) when nothing matches;
// callers that need a "Không xác định" placeholder apply that
// themselves, mirroring where Python applies it — the caller
// (xulydonhang.py:9128-9129), not this function.
func (s *Store) GetCustomerCodeBySuffix(system, storeCode string) string {
	system = strings.ToUpper(strings.TrimSpace(system))
	storeCode = strings.TrimSpace(storeCode)
	for _, row := range s.customerRows {
		colA, colC := row[0], row[2]
		if strings.ToUpper(strings.TrimSpace(colA)) != system {
			continue
		}
		trimmedC := strings.TrimSpace(colC)
		if trimmedC != "" && strings.HasSuffix(trimmedC, storeCode) {
			return trimmedC
		}
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/productdata/... -v`
Expected: PASS — all existing tests plus the new one.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/productdata/store.go GO/internal/processing/productdata/store_test.go
git commit -m "feat(go): add Lotte-style suffix customer code lookup to productdata.Store"
```

---

### Task 4: `lotte` package — `LinesBetween` helper + `ParseOrderInfo`

**Files:**
- Create: `GO/internal/processing/lotte/linesbetween.go`
- Create: `GO/internal/processing/lotte/linesbetween_test.go`
- Create: `GO/internal/processing/lotte/extract.go`
- Create: `GO/internal/processing/lotte/extract_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `lotte.LinesBetween(text, startPrefix, endMarker string) []string`; `lotte.OrderInfo{PONumber, EntryDate, StoreCode string}`; `lotte.ParseOrderInfo(text string) (OrderInfo, error)`. Both consumed by Task 5 (`LinesBetween`) and Task 7 (`ParseOrderInfo`).

- [ ] **Step 1: Write the failing tests**

Create `GO/internal/processing/lotte/linesbetween_test.go`:

```go
package lotte

import "testing"

func TestLinesBetween_ReturnsLinesStrictlyBetweenMarkers(t *testing.T) {
	text := "before\nSTART here\nline1\nline2\nEND\nafter"
	got := LinesBetween(text, "START", "END")
	want := []string{"line1", "line2"}
	if len(got) != len(want) {
		t.Fatalf("LinesBetween returned %d lines, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("LinesBetween()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLinesBetween_AdjacentMarkersReturnEmptySlice(t *testing.T) {
	got := LinesBetween("START\nEND", "START", "END")
	if len(got) != 0 {
		t.Fatalf("LinesBetween with adjacent markers = %v, want empty", got)
	}
}

func TestLinesBetween_MissingStartReturnsNil(t *testing.T) {
	if got := LinesBetween("nothing\nEND", "START", "END"); got != nil {
		t.Fatalf("LinesBetween with no start match = %v, want nil", got)
	}
}

func TestLinesBetween_MissingEndReturnsNil(t *testing.T) {
	if got := LinesBetween("START\nline1", "START", "END"); got != nil {
		t.Fatalf("LinesBetween with no end match = %v, want nil", got)
	}
}

func TestLinesBetween_EndMarkerBeforeStartReturnsNil(t *testing.T) {
	// Mirrors the Python source's single-pass-with-break behavior: once
	// end_marker is found, the scan stops immediately, so a start match
	// that would only appear LATER in the text is never reached.
	if got := LinesBetween("END\nSTART\nline1", "START", "END"); got != nil {
		t.Fatalf("LinesBetween with end before start = %v, want nil", got)
	}
}
```

Create `GO/internal/processing/lotte/extract_test.go`:

```go
package lotte

import "testing"

func TestParseOrderInfo_ParsesPageSecondLine(t *testing.T) {
	// Real sample from đơn hàng/08-2026/260727-01013-00057.pdf, page 1:
	// line 0 is "Ord sheet", line 1 is the raw 16-digit PO string.
	text := "Ord sheet\n2607270101300057\nPage\n:\n1 / 1"
	got, err := ParseOrderInfo(text)
	if err != nil {
		t.Fatalf("ParseOrderInfo returned error: %v", err)
	}
	want := OrderInfo{PONumber: "260727-01013-00057", EntryDate: "27/07/2026", StoreCode: "01013"}
	if got != want {
		t.Fatalf("ParseOrderInfo = %+v, want %+v", got, want)
	}
}

func TestParseOrderInfo_TooFewLinesReturnsError(t *testing.T) {
	if _, err := ParseOrderInfo("only one line, no second line at all"); err == nil {
		t.Fatal("expected error for text with fewer than 2 lines, got nil")
	}
}

func TestParseOrderInfo_MalformedSecondLineReturnsError(t *testing.T) {
	if _, err := ParseOrderInfo("header\nnotadigitstring"); err == nil {
		t.Fatal("expected error for a non-numeric second line, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/lotte/... -v`
Expected: FAIL with build errors (`undefined: LinesBetween`, `undefined: OrderInfo`, `undefined: ParseOrderInfo` — none of these exist yet).

- [ ] **Step 3: Implement `LinesBetween`**

Create `GO/internal/processing/lotte/linesbetween.go`:

```go
package lotte

import "strings"

// LinesBetween finds the first line starting with startPrefix, then —
// continuing the SAME single pass — the first line (from the very
// beginning of text, not from the start match) whose trimmed content
// exactly equals endMarker, stopping the scan the instant that line is
// found. Returns every line strictly between the two matches (excluding
// both), or nil if no valid (start, end) pair with start before end was
// found.
//
// Mirrors the identical "find a start line, find an end line, take
// what's between" scan duplicated across three Lotte functions in
// xulydonhang.py: tachcancledate_lotte (:6051-6071, used by
// ExtractCancelDate) and lamsachdonhang_lotte (:6405-6423, used by
// ExtractProducts's cleanup step). laytenstore_lotte (:6565-6584) has
// the same scan shape but needs the raw start/end indices (not just the
// slice between them) for a case this helper's return value can't
// represent — see ExtractStoreName's own comment for why it doesn't use
// this helper.
func LinesBetween(text, startPrefix, endMarker string) []string {
	lines := strings.Split(text, "\n")
	startIndex := -1
	endIndex := -1
	for i, line := range lines {
		if startIndex == -1 && strings.HasPrefix(line, startPrefix) {
			startIndex = i
		}
		if strings.TrimSpace(line) == endMarker {
			endIndex = i
			break
		}
	}
	if startIndex == -1 || endIndex == -1 || startIndex >= endIndex {
		return nil
	}
	return lines[startIndex+1 : endIndex]
}
```

- [ ] **Step 4: Implement `ParseOrderInfo`**

Create `GO/internal/processing/lotte/extract.go`:

```go
package lotte

import (
	"fmt"
	"strings"
	"time"
)

// OrderInfo holds the PO number, entry date, and store code derived from
// a Lotte page's first two lines.
type OrderInfo struct {
	PONumber  string // formatted "yyMMdd-STORECODE-ORDER", e.g. "260727-01013-00057"
	EntryDate string // "dd/MM/yyyy"
	StoreCode string // the 5-digit middle segment of PONumber, e.g. "01013"
}

// ParseOrderInfo mirrors the PO-number/entry-date derivation at the top
// of process_file's Lotte branch (xulydonhang.py:9081-9092): the raw PO
// number is the page's SECOND line (index 1) — a 16-digit string with no
// separators (6-digit date + 5-digit store code + 5-digit order number).
// Hyphens are inserted at fixed byte offsets 6 and 12 to produce the
// formatted PO number, which is then split on "-" to recover each part.
//
// Deliberately more defensive than the Python original here: Python's
// po_number.split("-") is unpacked directly into 3 variables and raises
// an unhandled ValueError if the line doesn't produce exactly 3 hyphen-
// separated parts (e.g. too short to reach either insertion point) —
// this returns an error instead, per this plan's "correct main flow,
// don't need bug-for-bug parity" policy. Every real sample so far
// produces exactly 3 parts.
func ParseOrderInfo(text string) (OrderInfo, error) {
	lines := strings.Split(text, "\n")
	raw := ""
	if len(lines) > 1 {
		raw = lines[1]
	}

	poNumber := raw
	if len(poNumber) >= 7 {
		poNumber = poNumber[:6] + "-" + poNumber[6:]
	}
	if len(poNumber) >= 12 {
		poNumber = poNumber[:12] + "-" + poNumber[12:]
	}

	parts := strings.Split(poNumber, "-")
	if len(parts) != 3 {
		return OrderInfo{}, fmt.Errorf("không tách được số PO từ dòng thứ 2 của trang: %q", raw)
	}
	timePart, storeCode := parts[0], parts[1]

	entryDate, err := time.Parse("060102", timePart)
	if err != nil {
		return OrderInfo{}, fmt.Errorf("không đọc được ngày đặt hàng từ %q: %w", timePart, err)
	}

	return OrderInfo{
		PONumber:  poNumber,
		EntryDate: entryDate.Format("02/01/2006"),
		StoreCode: storeCode,
	}, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/lotte/... -v`
Expected: PASS — all `LinesBetween` and `ParseOrderInfo` tests.

- [ ] **Step 6: Commit**

```bash
git add GO/internal/processing/lotte/linesbetween.go GO/internal/processing/lotte/linesbetween_test.go GO/internal/processing/lotte/extract.go GO/internal/processing/lotte/extract_test.go
git commit -m "feat(go): add lotte package with LinesBetween and ParseOrderInfo"
```

---

### Task 5: `lotte` package — `ExtractCancelDate` + `ExtractStoreName`

**Files:**
- Modify: `GO/internal/processing/lotte/extract.go`
- Modify: `GO/internal/processing/lotte/extract_test.go`

**Interfaces:**
- Consumes: `LinesBetween` (Task 4).
- Produces: `lotte.ExtractCancelDate(text, poNumber string) string`; `lotte.ExtractStoreName(text, poNumber string) string`. Both consumed by Task 7.

- [ ] **Step 1: Write the failing tests**

Add to `GO/internal/processing/lotte/extract_test.go`:

```go
func TestExtractCancelDate_KeepsOnlyDateShapedLines(t *testing.T) {
	// Real shape from 260727-01013-00057.pdf: the line starting with the
	// PO number is followed by a priority label, then the cancel date,
	// then "00:00".
	text := "before\n260727-01013-00057 Khẩn cấp\n30/07/2026\n00:00\nafter"
	got := ExtractCancelDate(text, "260727-01013-00057")
	if got != "30/07/2026" {
		t.Fatalf("ExtractCancelDate = %q, want %q", got, "30/07/2026")
	}
}

func TestExtractCancelDate_NoMarkersReturnsEmpty(t *testing.T) {
	if got := ExtractCancelDate("no markers here", "260727-01013-00057"); got != "" {
		t.Fatalf("ExtractCancelDate = %q, want empty", got)
	}
}

func TestExtractStoreName_ReturnsLastLineBeforePoNumber(t *testing.T) {
	// Real shape from 260727-01013-00057.pdf.
	text := "DOAN TUAN ANH\n0304741634-011\nCONG TY CP TRUNG TAM\nTHUONG MAI LOTTE VIET\nNha trang\n260727-01013-00057\nKhẩn cấp"
	got := ExtractStoreName(text, "260727-01013-00057")
	if got != "Nha trang" {
		t.Fatalf("ExtractStoreName = %q, want %q", got, "Nha trang")
	}
}

func TestExtractStoreName_AdjacentMarkersReturnsAnchorLine(t *testing.T) {
	// When the anchor and the PO-number line are directly adjacent (no
	// lines between them), Python's lines[end_index-1] resolves to the
	// anchor line itself, not to "" — this asserts that exact edge-case
	// behavior, not a simplified "empty" result.
	text := "DOAN TUAN ANH\n260727-01013-00057"
	got := ExtractStoreName(text, "260727-01013-00057")
	if got != "DOAN TUAN ANH" {
		t.Fatalf("ExtractStoreName (adjacent markers) = %q, want %q", got, "DOAN TUAN ANH")
	}
}

func TestExtractStoreName_NoMarkersReturnsEmpty(t *testing.T) {
	if got := ExtractStoreName("no markers here", "260727-01013-00057"); got != "" {
		t.Fatalf("ExtractStoreName = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/lotte/... -run "TestExtractCancelDate|TestExtractStoreName" -v`
Expected: FAIL with `undefined: ExtractCancelDate`, `undefined: ExtractStoreName`.

- [ ] **Step 3: Implement**

Add to `GO/internal/processing/lotte/extract.go` (append to the existing file; add `"regexp"` to the import block):

```go
var dateLinePattern = regexp.MustCompile(`\d{1,2}/\d{1,2}/\d{4}`)

// ExtractCancelDate mirrors tachcancledate_lotte (xulydonhang.py:6051-6071):
// scans the lines between the line starting with poNumber and the line
// "00:00", keeping only lines that contain a d/m/yyyy-shaped date,
// joined back with a single newline. Returns "" if the (start, end)
// markers aren't both found (LinesBetween returns nil).
func ExtractCancelDate(text, poNumber string) string {
	between := LinesBetween(text, poNumber, "00:00")
	if between == nil {
		return ""
	}
	var filtered []string
	for _, line := range between {
		if dateLinePattern.MatchString(line) {
			filtered = append(filtered, strings.TrimSpace(line))
		}
	}
	return strings.Join(filtered, "\n")
}

// ExtractStoreName mirrors laytenstore_lotte (xulydonhang.py:6565-6584)
// exactly, including its edge-case behavior when the "DOAN TUAN ANH"
// anchor line and the poNumber line are adjacent — in that case Python's
// lines[end_index-1] resolves to the anchor line itself, not "".
//
// Not implemented via LinesBetween: that helper's returned slice can't
// distinguish "zero lines between the markers" from "the line
// immediately before the end marker IS the start marker itself", which
// this function needs to tell apart to match Python exactly.
func ExtractStoreName(text, poNumber string) string {
	lines := strings.Split(text, "\n")
	startIndex := -1
	endIndex := -1
	for i, line := range lines {
		if startIndex == -1 && strings.HasPrefix(line, "DOAN TUAN ANH") {
			startIndex = i
		}
		if strings.TrimSpace(line) == poNumber {
			endIndex = i
			break
		}
	}
	if startIndex == -1 || endIndex == -1 || startIndex >= endIndex {
		return ""
	}
	return strings.TrimSpace(lines[endIndex-1])
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/lotte/... -v`
Expected: PASS — all tests in the package, including Task 4's.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/lotte/extract.go GO/internal/processing/lotte/extract_test.go
git commit -m "feat(go): add ExtractCancelDate and ExtractStoreName to lotte package"
```

---

### Task 6: `lotte` package — `ExtractProducts`

**Files:**
- Modify: `GO/internal/processing/lotte/extract.go`
- Modify: `GO/internal/processing/lotte/extract_test.go`

**Interfaces:**
- Consumes: `LinesBetween` (Task 4).
- Produces: `lotte.Product{Barcode string; QtyBox, BoxQty int; TotalPrice float64}`; `lotte.ExtractProducts(text string) []Product`. Consumed by Task 7.

- [ ] **Step 1: Write the failing test**

Add to `GO/internal/processing/lotte/extract_test.go`:

```go
func TestExtractProducts_ParsesRealSampleProductBlock(t *testing.T) {
	// Verbatim block (from "Sply qty" through "Tot add tax") extracted
	// from đơn hàng/08-2026/260727-01013-00057.pdf, page 1. Verified by
	// hand against the same page's visible "Sply prc"/"Sply amt"
	// columns: 65,296 (unit price) * 24 (= 8 * 3) = 1,567,104 (total),
	// for both of this file's two product lines.
	text := "Sply qty\n01013\n1-197039-000 8936156730244\n20-DETERGENT\n" +
		"NG BLUE THAO MOC TUI 2.1L\n8\nBOX\n3\n3\n65,296\n1,567,104\n1\n8\n2.1L\n" +
		"1-197040-000 8936156730329\n20-DETERGENT\nNG BLUE NUOC HOA TUI 2.1L\n" +
		"8\nBOX\n3\n3\n65,296\n1,567,104\n2\n8\n2.1L\nTot\nTot add tax"

	got := ExtractProducts(text)
	want := []Product{
		{Barcode: "8936156730244", QtyBox: 8, BoxQty: 3, TotalPrice: 1567104},
		{Barcode: "8936156730329", QtyBox: 8, BoxQty: 3, TotalPrice: 1567104},
	}
	if len(got) != len(want) {
		t.Fatalf("ExtractProducts returned %d products, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ExtractProducts()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestExtractProducts_NoMarkersReturnsEmpty(t *testing.T) {
	if got := ExtractProducts("no markers here"); len(got) != 0 {
		t.Fatalf("ExtractProducts = %+v, want empty", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/lotte/... -run TestExtractProducts -v`
Expected: FAIL with `undefined: ExtractProducts`, `undefined: Product`.

- [ ] **Step 3: Implement**

Add to `GO/internal/processing/lotte/extract.go` (append; add `"strconv"` to the import block):

```go
// Product is one product line extracted from a Lotte order's product
// table. Field names mirror tachsanpham_lotte's dict keys ("Qty-Box" ->
// QtyBox is the unit count per box; "Box Quantity" -> BoxQty is the
// number of boxes — write_to_dondathang_lotte computes total ordered
// quantity as QtyBox * BoxQty). "Product Code" and "Loose Quantity" are
// captured by Python's regex but never read anywhere in
// write_to_dondathang_lotte — omitted here (YAGNI).
type Product struct {
	Barcode    string
	QtyBox     int
	BoxQty     int
	TotalPrice float64
}

// productLinePattern mirrors tachsanpham_lotte's regex exactly
// (xulydonhang.py:6076): group 1 = product code (unused), group 2 =
// barcode, group 3 = unit count ("Qty-Box"), group 4 = box quantity
// ("Box Quantity"), group 5 = loose quantity (unused), group 6 = total
// price.
var productLinePattern = regexp.MustCompile(`(\d{1,2}-\d{6}-\d{3})\s+(\d{12,13})[\s\S]*?(\d+)\s+BOX\s+(\d+)\s+(\d+)\s+[\d,]+\s+([\d,]+)`)

// ExtractProducts mirrors tachsanpham_lotte (xulydonhang.py:6074-6091):
// cleans the order text down to the block between "Sply qty" and
// "Tot add tax" (lamsachdonhang_lotte, :6405-6423 — raw lines joined
// back with newlines, no per-line filtering), then extracts every
// product line matching productLinePattern.
func ExtractProducts(text string) []Product {
	between := LinesBetween(text, "Sply qty", "Tot add tax")
	if between == nil {
		return nil
	}
	cleaned := strings.Join(between, "\n")

	matches := productLinePattern.FindAllStringSubmatch(cleaned, -1)
	products := make([]Product, 0, len(matches))
	for _, m := range matches {
		qtyBox, _ := strconv.Atoi(m[3])
		boxQty, _ := strconv.Atoi(m[4])
		totalPrice, _ := strconv.ParseFloat(strings.ReplaceAll(m[6], ",", ""), 64)
		products = append(products, Product{
			Barcode:    m[2],
			QtyBox:     qtyBox,
			BoxQty:     boxQty,
			TotalPrice: totalPrice,
		})
	}
	return products
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/lotte/... -v`
Expected: PASS — all tests in the package.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/lotte/extract.go GO/internal/processing/lotte/extract_test.go
git commit -m "feat(go): add ExtractProducts to lotte package"
```

---

### Task 7: `RealProcessor` — dispatch to Lotte

**Files:**
- Modify: `GO/internal/processing/coop_processor.go`
- Modify: `GO/internal/processing/coop_processor_test.go`
- Create: `GO/internal/processing/testdata/sample_lotte_order.pdf` (copy of a real file)

**Interfaces:**
- Consumes: `vendor.Identify` (Task 1), `pricing.Index`/`PricingSource.FetchIndex` (Task 2), `productdata.Store.GetCustomerCodeBySuffix` (Task 3), `lotte.ParseOrderInfo`/`ExtractCancelDate`/`ExtractStoreName`/`ExtractProducts`/`Product` (Tasks 4-6), and the already-shipped `regionInfo`, `closeEnough`, `buildPromoBonusRow`, `buildInvoiceBonusRow`, `coop.ExtractDiscount`, `coop.FormatWeightKg`.
- Produces: `RealProcessor.Process` now routes Lotte pages to a new `processLotteSegment` method, returning `[]OrderRow` same as the Coop path.

- [ ] **Step 1: Copy a real sample file into testdata**

```bash
cp "đơn hàng/08-2026/260727-01013-00057.pdf" GO/internal/processing/testdata/sample_lotte_order.pdf
```

- [ ] **Step 2: Write the failing test**

Add to `GO/internal/processing/coop_processor_test.go`:

```go
func TestRealProcessor_ProcessesRealSampleLotteFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	// Empty price index on purpose: this file's real barcodes
	// (8936156730244/8936156730329) aren't in the small test fixture, so
	// both products are expected to come back as price mismatches
	// (Warning), not Done — that's still a fully exercised, deterministic
	// code path.
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(nil)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_lotte_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Process returned %d rows, want 1: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.System != "Lotte" {
		t.Fatalf("row.System = %q, want %q", row.System, "Lotte")
	}
	if row.PO != "260727-01013-00057" {
		t.Fatalf("row.PO = %q, want %q", row.PO, "260727-01013-00057")
	}
	if row.MaKhachHang != "Không xác định" {
		t.Fatalf("row.MaKhachHang = %q, want %q (test fixture's data.xlsx has no store 1013)", row.MaKhachHang, "Không xác định")
	}
	if row.StatusKind != StatusKindWarning {
		t.Fatalf("row.StatusKind = %q, want %q (both products should price-mismatch against the empty index)", row.StatusKind, StatusKindWarning)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/... -run TestRealProcessor_ProcessesRealSampleLotteFile -v`
Expected: FAIL — the page currently falls into the `else` branch ("nhà cung cấp Lotte chưa được hỗ trợ"), so `row.StatusKind` is `StatusKindFailed`, not `StatusKindWarning`.

- [ ] **Step 4: Implement — restructure `Process`'s dispatch and add `processLotteSegment`**

Edit `GO/internal/processing/coop_processor.go`. First, add imports for the new `lotte` package:

```go
import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/lotte"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
	"order-processor/internal/processing/vendor"
)

const coopDebtDays = 60 // songayno_MT in xulydonhang.py — one global constant, shared by every vendor's write function, not Coop-specific
```

Then replace the body of `Process`'s per-page loop — change this:

```go
	var rows []OrderRow
	for pageIdx, text := range pageTexts {
		pageLabel := fmt.Sprintf("%d/%d", pageIdx+1, len(pageTexts))
		v := vendor.Identify(text)
		if v != "Coop" {
			reason := "không nhận diện được nhà cung cấp"
			if v != "" {
				reason = "nhà cung cấp " + v + " chưa được hỗ trợ"
			}
			rows = append(rows, OrderRow{
				FileName: filepath.Base(filePath), Page: pageLabel, System: v,
				Status: StatusFailed + " - " + reason, StatusKind: StatusKindFailed,
			})
			continue
		}

		segments, ok := splitPageIntoPOs(text)
		if !ok {
			rows = append(rows, OrderRow{
				FileName: filepath.Base(filePath), Page: pageLabel, System: "Coop",
				Status: StatusFailed + " - không đếm khớp số đơn trên trang", StatusKind: StatusKindFailed,
			})
			continue
		}

		for segIdx, segment := range segments {
			segLabel := fmt.Sprintf("%d/%d", segIdx+1, len(segments))
			row, err := p.processSegment(filePath, segment, segLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: segLabel, System: "Coop",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)
		}
	}

	return rows, nil
}
```

with this (Coop's inner logic is untouched, only wrapped in a `case`; Lotte is a new `case`, one segment per page — Python's Lotte branch never counts/splits multiple POs per page the way Coop does):

```go
	var rows []OrderRow
	for pageIdx, text := range pageTexts {
		pageLabel := fmt.Sprintf("%d/%d", pageIdx+1, len(pageTexts))
		v := vendor.Identify(text)

		switch v {
		case "Coop":
			segments, ok := splitPageIntoPOs(text)
			if !ok {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Coop",
					Status: StatusFailed + " - không đếm khớp số đơn trên trang", StatusKind: StatusKindFailed,
				})
				continue
			}
			for segIdx, segment := range segments {
				segLabel := fmt.Sprintf("%d/%d", segIdx+1, len(segments))
				row, err := p.processSegment(filePath, segment, segLabel)
				if err != nil {
					rows = append(rows, OrderRow{
						FileName: filepath.Base(filePath), Page: segLabel, System: "Coop",
						Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
					})
					continue
				}
				rows = append(rows, row)
			}

		case "Lotte":
			row, err := p.processLotteSegment(filePath, text, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Lotte",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)

		default:
			reason := "không nhận diện được nhà cung cấp"
			if v != "" {
				reason = "nhà cung cấp " + v + " chưa được hỗ trợ"
			}
			rows = append(rows, OrderRow{
				FileName: filepath.Base(filePath), Page: pageLabel, System: v,
				Status: StatusFailed + " - " + reason, StatusKind: StatusKindFailed,
			})
		}
	}

	return rows, nil
}
```

Finally, append `processLotteSegment` and `lotteOrderNumber` to the end of the file (after `buildInvoiceBonusRow`):

```go
// lotteOrderNumber mirrors write_to_dondathang_lotte's order-number
// field (xulydonhang.py:2018): f'ĐĐH{vendor}{STT_donhang_str}' where
// vendor is the uppercased literal "LOTTE" and STT_donhang_str is
// f"-{po_number}".
func lotteOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHLOTTE-%s", poNumber)
}

// processLotteSegment mirrors the Lotte branch of process_file
// (xulydonhang.py:9079-9139) plus write_to_dondathang_lotte
// (:1968-2318). Structurally identical to processSegment's promo-
// matching/bonus-row logic — write_to_dondathang_lotte calls the exact
// same helper functions Coop's write_to_dondathang does
// (find_price_by_sku, find_all_promotions_by_sku_and_time,
// extract_discount, check_value_in_sanpham, laycachbo_khuyenmai,
// tachtien_khuyenmai, layduoi_mahang), just with vendor="LOTTE" instead
// of "COOPMART"/"COOPFOOD". Two confirmed differences from Coop's path:
// (1) Lotte's promo value is used as-is — no SplitPromoText (that's
// Coop's cm/cf-bundling convention only, write_to_dondathang_lotte never
// calls tachkhuyenmai_coop); (2) no ShipTo-address special-casing (no
// COOPFOOD-equivalent concept for Lotte).
func (p *RealProcessor) processLotteSegment(filePath, text, pageLabel string) (OrderRow, error) {
	info, err := lotte.ParseOrderInfo(text)
	if err != nil {
		return OrderRow{}, err
	}

	cancelDate := lotte.ExtractCancelDate(text, info.PONumber)
	storeName := lotte.ExtractStoreName(text, info.PONumber)
	shipTo := "Lotte " + storeName

	products := lotte.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	// get_makhachhang_lotte(store_code[1:]) — the leading digit of the
	// 5-digit store code is dropped before matching (xulydonhang.py:9109).
	customerCode := ""
	if len(info.StoreCode) > 1 {
		customerCode = p.Store.GetCustomerCodeBySuffix("LOTTE", info.StoreCode[1:])
	}
	if customerCode == "" {
		customerCode = "Không xác định"
	}

	priceIndex, err := p.Pricing.FetchIndex("LOTTE")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := regionInfo(customerCode)
	description := fmt.Sprintf("LOTTE PO%s", info.PONumber)

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: info.EntryDate, DebtDays: coopDebtDays, OrderNumber: lotteOrderNumber(info.PONumber),
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
		Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: description,
	})

	saigia := 0
	totalWeight := 0.0
	totalValue := 0.0

	for _, rawProduct := range products {
		barcode := p.Store.ResolveSku(rawProduct.Barcode)
		productInfo, _ := p.Store.GetProductInfo(barcode)
		qty := float64(rawProduct.QtyBox * rawProduct.BoxQty)
		lineWeight := productInfo.WeightKg * qty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(qty / productInfo.PackSize))
		}
		totalWeight += lineWeight

		invoicePrice := rawProduct.TotalPrice / qty
		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice, _ := strconv.ParseFloat(strings.ReplaceAll(realPriceStr, ",", ""), 64)

		promos := priceIndex.FindPromotions(barcode, info.EntryDate)
		lastExaminedPromo := ""
		matched := false
		finalPrice := realPrice

		// No SplitPromoText here (see function doc): Lotte's promo cell
		// is used exactly as returned, not split into cm/cf variants.
		for _, promo := range promos {
			value := promo.Value
			lastExaminedPromo = value
			if value == "" {
				continue
			}
			candidatePrice := realPrice
			if discount := coop.ExtractDiscount(value); discount != 0 {
				candidatePrice = realPrice - (realPrice * discount / 100)
			}
			finalPrice = candidatePrice
			if closeEnough(invoicePrice, candidatePrice) {
				matched = true
				break
			}
		}
		if len(promos) == 0 && closeEnough(invoicePrice, realPrice) {
			matched = true
		}

		productRow := excelwriter.Row{
			EntryDate: info.EntryDate, DebtDays: coopDebtDays, OrderNumber: lotteOrderNumber(info.PONumber),
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
			Description: description, SKU: barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: qty, UnitPrice: finalPrice,
			ProductName: productInfo.Name, CaseCount: caseCount, LineWeightKg: lineWeight, UseZFormula: true,
			PromoContent: lastExaminedPromo,
		}
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
		}

		productRowIndex := len(rows)
		rows = append(rows, productRow)
		totalValue += finalPrice * qty

		currentRowIndex := productRowIndex
		for i, promoPart := range strings.Split(lastExaminedPromo, "|") {
			rows[currentRowIndex].PromoContent = lastExaminedPromo

			bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, promoPart,
				coop.Product{Barcode: barcode, Qty: qty}, i, info.EntryDate, cancelDate, shipTo,
				customerCode, description, warehouse, region, statCode, info.PONumber)
			if !added {
				continue
			}
			bonusRow.OrderNumber = lotteOrderNumber(info.PONumber) // buildPromoBonusRow hardcodes Coop's order number
			totalWeight += bonusRow.LineWeightKg
			if i == 0 {
				rows[productRowIndex].PromoNote = mainRowNote
				if mainRowBundleSku != "" {
					rows[productRowIndex].PromoBundleSku = mainRowBundleSku
				}
			}
			rows = append(rows, bonusRow)
			currentRowIndex = len(rows) - 1
		}
	}

	if invoicePromo := priceIndex.FindInvoicePromotion(info.EntryDate); invoicePromo != "" {
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, info.EntryDate, cancelDate,
			shipTo, customerCode, description, warehouse, region, statCode, info.PONumber); added {
			bonusRow.OrderNumber = lotteOrderNumber(info.PONumber) // buildInvoiceBonusRow hardcodes Coop's order number
			totalWeight += bonusRow.LineWeightKg
			rows = append(rows, bonusRow)
		}
	}

	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	if err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription); err != nil {
		return OrderRow{}, err
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Lotte", MaKhachHang: customerCode,
		PO: info.PONumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
	}, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd GO && go build ./... && go vet ./... && go test ./internal/processing/... -v`
Expected: PASS — the new Lotte test, and every existing Coop test unchanged (93/155 on the golden fixture test, same as before this task).

- [ ] **Step 6: Commit**

```bash
git add GO/internal/processing/coop_processor.go GO/internal/processing/coop_processor_test.go GO/internal/processing/testdata/sample_lotte_order.pdf
git commit -m "feat(go): dispatch RealProcessor to Lotte via processLotteSegment"
```

---

### Task 8: Golden fixture generation script (throwaway) — generate 60 Lotte fixtures

**Files:**
- Create: `GO/internal/processing/lotte/testdata/generate_fixtures.py` (throwaway dev tool, not part of the shipped app — same status as Task 12's Coop equivalent, `GO/internal/processing/coop/testdata/generate_fixtures.py`, which stayed in the repo after use)

**Interfaces:**
- Consumes: the real `xulydonhang.py`'s `ProcessHandler.tachcancledate_lotte`, `laytenstore_lotte`, `tachsanpham_lotte`, `get_makhachhang_lotte`, `write_to_dondathang_lotte`, `identify_vendor`, `find_price_by_sku`, `find_all_promotions_by_sku_and_time`, `get_gid` — all unmodified.
- Produces: `GO/internal/processing/lotte/testdata/fixtures/*.json` (one per PDF) and `_frozen_pricing.json`, in the exact same shape Task 12 already established for Coop (`{"source_pdf": ..., "rows": [...]}`, `{"raw_rows": [...]}`) — consumed by Task 9.

- [ ] **Step 1: Write the script**

Create `GO/internal/processing/lotte/testdata/generate_fixtures.py`, adapted directly from the already-proven Coop harness (`GO/internal/processing/coop/testdata/generate_fixtures.py`) — same REPO_ROOT resolution, same UTF-8 stdout fix, same production-`dondathang.xlsx` backup/restore protocol, same price/promo caching monkeypatch (already generic over `sheet_name`, needs no changes), only the vendor-detection and per-file processing functions are Lotte-specific:

```python
"""
Throwaway dev tool — NOT part of the shipped Go or Python app.

Runs the CURRENT xulydonhang.py Lotte pipeline against every real PDF in
đơn hàng/08-2026/ that identify_vendor recognizes as Lotte, capturing the
resulting dondathang.xlsx rows (and the live-fetched Google Sheets
price/promotion data for the LOTTE sheet) into JSON fixtures under
GO/internal/processing/lotte/testdata/fixtures/. The Go golden test
(Task 9) diffs RealProcessor's output against these fixtures instead of
against a live Google Sheets fetch, so it's deterministic and offline.

Run from the repo root:
    .venv/Scripts/python.exe GO/internal/processing/lotte/testdata/generate_fixtures.py
"""
import glob
import json
import os
import shutil
import sys

# Same depth as Coop's harness: this script sits 5 directory levels below
# repo root (GO/internal/processing/lotte/testdata/generate_fixtures.py),
# so reaching repo root from os.path.abspath(__file__) requires 6
# dirname() calls (one to strip the filename, five more to strip
# GO/internal/processing/lotte/testdata).
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))))
sys.path.insert(0, REPO_ROOT)
os.chdir(REPO_ROOT)  # xulydonhang.py's functions use relative paths ("data.xlsx", "settings.ini")

# See Coop's harness for why this is needed: process_file's debug print()
# calls contain emoji that the legacy cp1252 console codepage can't
# encode, aborting processing partway through if not fixed here first.
if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="backslashreplace")
if hasattr(sys.stderr, "reconfigure"):
    sys.stderr.reconfigure(encoding="utf-8", errors="backslashreplace")

import openpyxl  # noqa: E402
import xulydonhang  # noqa: E402

FIXTURES_DIR = os.path.join(
    REPO_ROOT, "GO", "internal", "processing", "lotte", "testdata", "fixtures"
)
TEMPLATE_XLSX = os.path.join(REPO_ROOT, "dondathang.xlsx")
SCRATCH_XLSX = os.path.join(REPO_ROOT, "dondathang_fixture_scratch.xlsx")

# --- Monkey-patch network/upload side effects out (identical shape to
# Coop's harness; find_price_by_sku/find_all_promotions_by_sku_and_time
# are already generic over sheet_name, so this works for "LOTTE" too
# with no changes to the caching logic itself) ---

_price_cache = {}
_promo_cache = {}
_promo_raw_rows = None


def _cached_find_price_by_sku(sku_number, sheet_name="COOP"):
    key = (sku_number, sheet_name)
    if key not in _price_cache:
        _price_cache[key] = _real_find_price_by_sku(sku_number, sheet_name)
    return _price_cache[key]


def _cached_find_all_promotions(sku_code, time_to_check, sheet_name="Coop"):
    global _promo_raw_rows
    if _promo_raw_rows is None:
        _capture_promo_raw_rows(sheet_name)
    key = (sku_code, time_to_check, sheet_name)
    if key not in _promo_cache:
        _promo_cache[key] = _real_find_all_promotions(sku_code, time_to_check, sheet_name)
    return _promo_cache[key]


def _capture_promo_raw_rows(sheet_name):
    global _promo_raw_rows
    import pandas as pd

    gid = xulydonhang.ProcessHandler.get_gid(sheet_name)
    if not gid:
        _promo_raw_rows = []
        return
    sheet_id = "1yvxE_SPYXKhofcZdhv1CSKAyiwdY1Mf4pFlsiMbtOr4"
    url = f"https://docs.google.com/spreadsheets/d/{sheet_id}/gviz/tq?tqx=out:csv&gid={gid}"
    df = pd.read_csv(url, dtype=str, header=None)
    df.fillna("", inplace=True)
    _promo_raw_rows = df.values.tolist()


def _noop_upload_file_to_drive(path, output_name=None, vendor=None, entry_date=None, makhachhang=None, cancle_date=None):
    return {"url": "https://example.invalid/skipped-during-fixture-generation"}


_real_find_price_by_sku = xulydonhang.ProcessHandler.find_price_by_sku
_real_find_all_promotions = xulydonhang.ProcessHandler.find_all_promotions_by_sku_and_time
xulydonhang.ProcessHandler.find_price_by_sku = staticmethod(_cached_find_price_by_sku)
xulydonhang.ProcessHandler.find_all_promotions_by_sku_and_time = staticmethod(_cached_find_all_promotions)
xulydonhang.ProcessHandler.upload_file_to_drive = staticmethod(_noop_upload_file_to_drive)


# --- Excel row capture (same columns as Coop's harness — same sheet, same layout) ---

COLUMNS = [
    "A", "B", "C", "D", "E", "G", "L", "Q", "S", "T", "U", "V", "X", "Y", "Z",
    "AE", "AJ", "AM", "AO", "AP", "AQ", "AT", "AU", "AV",
]


def snapshot_rows(path, start_row):
    wb = openpyxl.load_workbook(path)
    sheet = wb["Don dat hang"]
    rows = []
    for r in range(start_row, sheet.max_row + 1):
        row = {"row_number_offset": r - start_row}
        for col in COLUMNS:
            cell = sheet[f"{col}{r}"]
            value = cell.value
            row[col] = value
            if col == "Z":
                row["Z_is_formula"] = isinstance(value, str) and value.startswith("=")
        comment = sheet[f"Y{r}"].comment
        row["Y_has_comment"] = comment is not None
        row["Y_fill"] = sheet[f"Y{r}"].fill.fgColor.rgb if sheet[f"Y{r}"].fill else None
        rows.append(row)
    wb.close()
    return rows


def is_lotte_pdf(path):
    doc = xulydonhang.fitz.open(path)
    try:
        text = ""
        for page in doc:
            text += page.get_text("text")
        return xulydonhang.ProcessHandler.identify_vendor(text) == "Lotte"
    finally:
        doc.close()


def process_one_pdf(path):
    """Mirrors the Lotte branch of process_file (xulydonhang.py:9079-9139)
    for every page identify_vendor recognizes as Lotte, skipping the
    Google Drive upload / current-page-extraction side effects (not
    needed to capture the Excel row output this harness cares about)."""
    handler = xulydonhang.process_handler
    doc = xulydonhang.fitz.open(path)
    try:
        for page_num in range(len(doc)):
            text = doc[page_num].get_text("text")
            if xulydonhang.ProcessHandler.identify_vendor(text) != "Lotte":
                continue

            lines = text.splitlines()
            po_number = lines[1] if len(lines) > 1 else ""
            if len(po_number) >= 7:
                po_number = po_number[:6] + "-" + po_number[6:]
            if len(po_number) >= 12:
                po_number = po_number[:12] + "-" + po_number[12:]

            time_part, store_code, order_number = po_number.split("-")
            entry_date = xulydonhang.datetime.strptime(time_part, "%y%m%d").strftime("%d/%m/%Y")

            cancel_date = xulydonhang.ProcessHandler.tachcancledate_lotte(text, po_number)
            tenstore = xulydonhang.ProcessHandler.laytenstore_lotte(text, po_number)
            diachigiaohang = "Lotte " + (tenstore or "")

            product_details = xulydonhang.ProcessHandler.tachsanpham_lotte(text)
            store_code_resolved = xulydonhang.ProcessHandler.get_makhachhang_lotte(store_code[1:])

            xulydonhang.ProcessHandler.write_to_dondathang_lotte(
                handler, product_details, store_code_resolved, po_number,
                entry_date, cancel_date, 1, "Lotte", diachigiaohang, None,
            )
    finally:
        doc.close()


def main():
    os.makedirs(FIXTURES_DIR, exist_ok=True)

    pdf_paths = sorted(glob.glob(os.path.join(REPO_ROOT, "đơn hàng", "08-2026", "*.pdf")))
    print(f"Found {len(pdf_paths)} candidate PDFs")

    generated = 0
    skipped = 0
    for path in pdf_paths:
        try:
            if not is_lotte_pdf(path):
                continue
        except Exception as e:
            print(f"SKIP (vendor check failed) {os.path.basename(path)}: {e}")
            skipped += 1
            continue

        shutil.copyfile(TEMPLATE_XLSX, SCRATCH_XLSX)
        os.chdir(REPO_ROOT)
        real_target = os.path.join(REPO_ROOT, "dondathang.xlsx")
        backup = real_target + ".fixture_backup"
        shutil.move(real_target, backup)
        shutil.copyfile(SCRATCH_XLSX, real_target)
        try:
            wb = openpyxl.load_workbook(real_target)
            start_row = wb["Don dat hang"].max_row + 1
            wb.close()

            process_one_pdf(path)

            rows = snapshot_rows(real_target, start_row)
        except Exception as e:
            print(f"SKIP (processing failed) {os.path.basename(path)}: {e}")
            skipped += 1
            rows = None
        finally:
            os.remove(real_target)
            shutil.move(backup, real_target)
            if os.path.exists(SCRATCH_XLSX):
                os.remove(SCRATCH_XLSX)

        if rows is None:
            continue

        fixture = {"source_pdf": os.path.basename(path), "rows": rows}
        fixture_name = os.path.splitext(os.path.basename(path))[0] + ".json"
        with open(os.path.join(FIXTURES_DIR, fixture_name), "w", encoding="utf-8") as f:
            json.dump(fixture, f, ensure_ascii=False, indent=2, default=str)
        generated += 1
        print(f"OK {os.path.basename(path)} -> {len(rows)} rows")

    if _promo_raw_rows is None:
        _capture_promo_raw_rows("LOTTE")
    frozen_pricing = {"raw_rows": _promo_raw_rows or []}
    with open(os.path.join(FIXTURES_DIR, "_frozen_pricing.json"), "w", encoding="utf-8") as f:
        json.dump(frozen_pricing, f, ensure_ascii=False, indent=2, default=str)

    print(f"\nDone: {generated} fixtures generated, {skipped} PDFs skipped.")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Back up the production workbook before running (safety)**

```bash
cp dondathang.xlsx dondathang.xlsx.manual_backup_before_lotte_fixtures
```

- [ ] **Step 3: Run the script**

```bash
.venv/Scripts/python.exe GO/internal/processing/lotte/testdata/generate_fixtures.py
```

Expected: prints "Found 60 candidate PDFs" (or close to it — the 1 new Lotte file spotted after the spec was written may push this to 61; either is fine), then one `OK`/`SKIP` line per file, ending with "Done: N fixtures generated, M PDFs skipped."

- [ ] **Step 4: Verify the production workbook is untouched**

```bash
diff dondathang.xlsx dondathang.xlsx.manual_backup_before_lotte_fixtures && echo "IDENTICAL — safe" || echo "DIFFERS — investigate before proceeding, do not continue"
```

Expected: "IDENTICAL — safe". If it differs, STOP — restore from the backup immediately (`mv dondathang.xlsx.manual_backup_before_lotte_fixtures dondathang.xlsx`) and investigate before doing anything else; this must never happen given the script's backup/restore protocol, but the check must run every time regardless.

- [ ] **Step 5: Remove the manual backup once confirmed identical**

```bash
rm dondathang.xlsx.manual_backup_before_lotte_fixtures
```

- [ ] **Step 6: Spot-check a few generated fixtures**

Read 2-3 files under `GO/internal/processing/lotte/testdata/fixtures/*.json` (not `_frozen_pricing.json`) and confirm they contain plausible values (non-null PO-shaped `B` column, non-empty `S`/product-name values, sane-looking numbers in `X`/`Y`/`Z`) — a sanity check, not exhaustive verification (Task 9's test does that).

- [ ] **Step 7: Commit**

```bash
git add GO/internal/processing/lotte/testdata/generate_fixtures.py GO/internal/processing/lotte/testdata/fixtures/
git commit -m "test(go): generate golden fixtures for Lotte from real PDFs + production output"
```

---

### Task 9: Golden fixture integration test

**Files:**
- Modify: `GO/internal/processing/coop_golden_test.go` (small, additive change — add an optional divergence allowlist parameter to the shared `compareRowsAgainstFixture`)
- Create: `GO/internal/processing/lotte_golden_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1-8; reuses `fixtureData`, `frozenPricingFixture`, `fixturePricingSource`, `stringify`, `toFloat`, `floatCloseEnough`, `copyFile`, `joinLines` — all already defined in the `processing` package by Coop's own golden test, no redeclaration needed.
- Produces: `TestRealProcessor_MatchesGoldenFixtures_Lotte`.

- [ ] **Step 1: Add an optional divergence allowlist to the shared comparator**

Edit `GO/internal/processing/coop_golden_test.go`. This is the mechanism this plan's testing policy (see Global Constraints) needs: a place to record specific, evidence-backed cases where Go intentionally computes a different value than the frozen Python fixture. Change `compareRowsAgainstFixture`'s signature and its two `*mismatches = append(...)` call sites inside the `textColumns`/`floatColumns`/`intColumns` loops to consult it first:

```go
func compareRowsAgainstFixture(t *testing.T, excelPath string, fixture fixtureData, mismatches *[]string, allowedDivergences map[string]bool) {
	t.Helper()

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		*mismatches = append(*mismatches, fmt.Sprintf("%s: failed reopening workbook: %v", fixture.SourcePDF, err))
		return
	}
	defer f.Close()

	existingRows, err := f.GetRows("Don dat hang")
	if err != nil {
		*mismatches = append(*mismatches, fmt.Sprintf("%s: failed reading sheet: %v", fixture.SourcePDF, err))
		return
	}
	startRow := len(existingRows) - len(fixture.Rows) + 1
	if startRow < 1 {
		*mismatches = append(*mismatches, fmt.Sprintf("%s: sheet has fewer rows (%d) than the fixture expects (%d)", fixture.SourcePDF, len(existingRows), len(fixture.Rows)))
		return
	}

	comments, err := f.GetComments("Don dat hang")
	if err != nil {
		*mismatches = append(*mismatches, fmt.Sprintf("%s: failed reading comments: %v", fixture.SourcePDF, err))
		return
	}
	commentedCells := make(map[string]bool, len(comments))
	for _, c := range comments {
		commentedCells[c.Cell] = true
	}

	textColumns := []string{"A", "B", "C", "D", "E", "G", "L", "Q", "S", "T", "U", "V", "AJ", "AM", "AO", "AP", "AQ"}
	floatColumns := []string{"X", "Y", "AT"}
	intColumns := []string{"AE", "AU", "AV"}

	isAllowed := func(rowIdx int, col string) bool {
		if allowedDivergences == nil {
			return false
		}
		return allowedDivergences[fmt.Sprintf("%d:%s", rowIdx, col)]
	}

	for i, expectedRow := range fixture.Rows {
		rowNum := startRow + i
		cell := func(col string) string {
			v, _ := f.GetCellValue("Don dat hang", fmt.Sprintf("%s%d", col, rowNum))
			return v
		}

		for _, col := range textColumns {
			if isAllowed(i, col) {
				continue
			}
			expected := stringify(expectedRow[col])
			got := cell(col)
			if expected != got {
				*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col %s: got %q, want %q", fixture.SourcePDF, i, col, got, expected))
			}
		}

		for _, col := range floatColumns {
			if isAllowed(i, col) {
				continue
			}
			expected := toFloat(expectedRow[col])
			got := toFloat(cell(col))
			if !floatCloseEnough(expected, got) {
				*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col %s: got %v, want %v", fixture.SourcePDF, i, col, got, expected))
			}
		}

		for _, col := range intColumns {
			if isAllowed(i, col) {
				continue
			}
			expected := stringify(expectedRow[col])
			got := cell(col)
			if expected != got {
				*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col %s: got %q, want %q", fixture.SourcePDF, i, col, got, expected))
			}
		}

		expectedFormula, _ := expectedRow["Z_is_formula"].(bool)
		gotFormula, _ := f.GetCellFormula("Don dat hang", fmt.Sprintf("Z%d", rowNum))
		if expectedFormula != (gotFormula != "") {
			*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col Z: formula present = %v, want %v", fixture.SourcePDF, i, gotFormula != "", expectedFormula))
		}

		yCell := fmt.Sprintf("Y%d", rowNum)
		expectedHasComment, _ := expectedRow["Y_has_comment"].(bool)
		gotHasComment := commentedCells[yCell]
		if expectedHasComment != gotHasComment {
			*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col Y: has comment = %v, want %v", fixture.SourcePDF, i, gotHasComment, expectedHasComment))
		}

		expectedFillStr, _ := expectedRow["Y_fill"].(string)
		expectedHasFill := expectedFillStr != "" && expectedFillStr != "00000000"
		styleID, err := f.GetCellStyle("Don dat hang", yCell)
		if err != nil {
			*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col Y: failed reading style: %v", fixture.SourcePDF, i, err))
			continue
		}
		gotHasFill := styleID != 0
		if expectedHasFill != gotHasFill {
			*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col Y: has red fill = %v, want %v", fixture.SourcePDF, i, gotHasFill, expectedHasFill))
		}
	}
}
```

Update the one existing call site inside `TestRealProcessor_MatchesGoldenFixtures` (Coop's test) from `compareRowsAgainstFixture(t, excelPath, fixture, &mismatches)` to `compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, nil)` — `nil` allowlist means Coop's behavior is byte-for-byte unchanged.

- [ ] **Step 2: Write `lotte_golden_test.go`**

Create `GO/internal/processing/lotte_golden_test.go`:

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

// knownDivergences_Lotte lists (fixture row index, column) cells where
// this Go port intentionally computes a different, verified-more-correct
// value than the frozen Python fixture — per this plan's testing policy
// (see the plan's Global Constraints / the spec's "Chiến lược kiểm
// chứng"). Empty until a real, hand-verified case is found; add entries
// here only with a comment citing the specific PDF evidence that proves
// Python is wrong on that cell — never to silence an unexplained diff.
// Key format: "<fixture row index>:<column>", e.g. "0:D".
var knownDivergences_Lotte = map[string]bool{}

func loadFrozenLottePricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("lotte/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen Lotte pricing fixture found (run Task 8's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen Lotte pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

func TestRealProcessor_MatchesGoldenFixtures_Lotte(t *testing.T) {
	fixturePaths, err := filepath.Glob("lotte/testdata/fixtures/*.json")
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
		t.Skip("no golden fixtures found (run Task 8's generate_fixtures.py first)")
	}

	store, err := productdata.Load("../../../data.xlsx")
	if err != nil {
		t.Fatalf("failed loading production data.xlsx: %v", err)
	}
	pricingSource := loadFrozenLottePricingSource(t)

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
		if len(rows) == 0 || rows[0].StatusKind == StatusKindFailed {
			mismatches = append(mismatches, fixture.SourcePDF+": Process produced a Failed row")
			continue
		}

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_Lotte)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
}
```

- [ ] **Step 3: Run the test**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures" -v`

Expected: Coop's test still reports the same pass count as before this plan (regression check on the shared helper's refactor). Lotte's test will very likely fail on the first run with real mismatches — this is expected and is the actual verification work of this task, same as Task 13 was for Coop.

- [ ] **Step 4: Root-cause and fix every mismatch**

For each mismatch: read the specific fixture JSON and the source PDF, trace through `xulydonhang.py`'s actual Lotte functions at the cited line numbers, and determine whether it's (a) a bug in this plan's Go port — fix the Go code; or (b) a case where Python is genuinely wrong and Go's different output is verifiably more correct — add a precise, evidence-citing entry to `knownDivergences_Lotte` (never edit the fixture JSON itself). Do not guess; every fix or allowlist entry must be traceable to specific evidence (a Python line number and/or a hand-checked value from the source PDF), following the same discipline established throughout Phase 2a's Task 13. Re-run after each fix.

If some failures turn out to be PDF-text-extraction-fidelity gaps (the same category of limitation Phase 2a's Coop plan hit, documented in that plan's Task 13), document them the same way — do not force a workaround into the extraction library, park them as a documented, understood gap.

- [ ] **Step 5: Final run and commit**

Run: `cd GO && go build ./... && go vet ./... && go test ./... -v`
Expected: clean build/vet, all tests pass (or fail only with fully documented, understood, non-logic-bug gaps — same acceptance bar Coop's Task 13 used).

```bash
git add GO/internal/processing/coop_golden_test.go GO/internal/processing/lotte_golden_test.go
git commit -m "test(go): add Lotte golden fixture integration test"
```
