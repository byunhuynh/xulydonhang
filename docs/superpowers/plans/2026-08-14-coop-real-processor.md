# Coop RealProcessor (Phase 2a) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `MockProcessor` with a real Coop PDF-parsing pipeline (`RealProcessor`) that reads real Coop purchase-order PDFs, extracts products/pricing/promotions, and writes correctly-formatted rows into an Excel workbook — validated against 155 real archived Coop PDFs via a frozen golden-fixture corpus generated from the current Python app's actual output.

**Architecture:** A chain of small, independently-testable Go packages under `GO/internal/processing/` — `vendor` (vendor detection), `productdata` (cached `data.xlsx` lookups), `pricing` (batched Google Sheets price/promo fetch), `coop` (Coop-specific text parsing: dispatch, invoice info, product extraction, promo text), `excelwriter` (column-exact Excel row writing) — composed by `RealProcessor`, which implements the `processing.Processor` interface (revised in this plan to return `[]OrderRow` per file, since one Coop PDF can contain multiple purchase orders). A Python harness (throwaway, not part of the app) generates 155 golden fixtures from the current production logic; a Go integration test replays all 155 real PDFs through `RealProcessor` and diffs against those fixtures.

**Tech Stack:** Go 1.26 (existing), `github.com/ledongthuc/pdf` (pure-Go PDF text extraction, no CGO — verified against a real Coop PDF), `github.com/xuri/excelize/v2` (xlsx read/write), Python (existing `.venv`, for the throwaway fixture-generation harness only).

**Spec:** [docs/superpowers/specs/2026-08-14-coop-real-processor-design.md](../specs/2026-08-14-coop-real-processor-design.md)

## Global Constraints

- All new code lives under `GO/internal/processing/`; the only files outside that touched by this plan are `GO/app.go`, `GO/app_test.go`, and `GO/main.go` (all three already exist from Phase 1 — Task 11 updates them for the `Processor` interface's new `[]OrderRow` return type and to wire in `RealProcessor`).
- **This plan ports intentionally-preserved bugs from `xulydonhang.py`.** Every place this happens is called out in a code comment at the exact line. Do not "fix" any of them without an explicit decision from the user — the golden-fixture test in Task 15 is what proves correctness, not code review intuition. Known preserved quirks, found while extracting the exact Python source for this plan:
  - `get_makhachhang`: reads column C (not column B) for `po_location` matching due to `col_A, col_B, col_C = row[0], row[2], row[2]`.
  - `load_sku_mapping`: builds its mapping from every cell in `SanPham` from column C onward (weight/pack-size columns included, not just per-vendor SKU columns) since the Python source iterates `row[2:]`.
  - `catdonra_nhieutrang`: its per-keyword text split (`text.split(keyword, 1)`) is case-sensitive against a lowercase literal (`"pom343"`/`"pom346"`), while real Coop PDF text extracts as uppercase (`"POM343"`) — this split silently finds no match on real data. Confirmed via a live extraction spike on a real Coop PDF sample.
- **Debug/print-only statements are not ported.** `xulydonhang.py` has extensive `print(f"DEBUG: ...")` calls throughout the ported functions — these only ever reach a developer's terminal, never the UI, and are not ported. Only `self.log_signal.emit(...)` calls are behaviorally relevant (they reach `process:log` in Phase 1's UI) — those ARE ported, as plain text (strip the Python code's inline HTML markup like `<b>`/`<span style=...>`, since Phase 1's `LogPanel` renders plain text, not HTML).
- **No silent failures.** Per the spec, any page/file the current Python code would silently skip (mismatched POM/Sub Total counts, un-findable vendor, a product block extraction failure) must produce a `Failed`-status `OrderRow` with a specific reason instead of vanishing. Several of the "preserved bugs" above mean the Python original would actually *crash* in some of these cases (unhandled exception) rather than silently skip — where that's true, returning an `error`/Failed row from the Go port is the closer behavioral match, not a loosening of fidelity.
- Coop's `Processor.Process` writes into a **separate test workbook** (`GO/dondathang_test.xlsx`), never the real `dondathang.xlsx` at the repo root — per the spec, that switch happens in a later phase when the Go app is ready to replace `App.py`.
- `processing.OrderRow` already has `StatusKind` (`"done"|"warning"|"failed"`) and `Status*`/`StatusKind*` constants from the final review fix on Phase 1 — reuse them, do not redefine.

---

### Task 1: `internal/processing/vendor` — Coop detection

**Files:**
- Create: `GO/internal/processing/vendor/identify.go`
- Test: `GO/internal/processing/vendor/identify_test.go`

**Interfaces:**
- Produces: `vendor.Identify(text string) string` — returns `"Coop"` if the text matches Coop's vendor markers, `""` otherwise (later phases add more vendor branches to this same function; `""` means "not yet supported", not "definitely no vendor").

- [ ] **Step 1: Write the failing test**

```go
package vendor

import "testing"

func TestIdentify_RecognizesCoopByVendorID(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"vendor dash id 21569", "Bill To: Vendor - 21569 Co.opMart", "Coop"},
		{"vendor colon id 22856", "Vendor: 22856", "Coop"},
		{"vendor id with newline noise", "Vendor:\n  22856\nCo.opMart Nha Trang", "Coop"},
		{"unrelated vendor id", "Vendor - 99999", ""},
		{"unrelated text", "Purchase Order from BigC", ""},
		{"empty text", "", ""},
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

Run: `cd GO && go test ./internal/processing/vendor/... -v`
Expected: FAIL — package `vendor` doesn't exist yet (compile error).

- [ ] **Step 3: Write minimal implementation**

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
)

// Identify tries to recognize which retail vendor produced this
// page/PO text, mirroring xulydonhang.py's identify_vendor. Only Coop
// is implemented in this phase; every other vendor is a later phase's
// work, so Identify returns "" for anything that isn't Coop.
func Identify(text string) string {
	cleaned := strings.TrimSpace(whitespacePattern.ReplaceAllString(text, " "))
	if coopPattern.MatchString(cleaned) {
		return "Coop"
	}
	return ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd GO && go test ./internal/processing/vendor/... -v`
Expected: PASS, all 6 subtests.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/vendor/
git commit -m "feat(go): add Coop vendor detection"
```

---

### Task 2: `internal/processing/pricing` — settings.ini `<gid>` parser

**Files:**
- Create: `GO/internal/processing/pricing/gid.go`
- Test: `GO/internal/processing/pricing/gid_test.go`

**Interfaces:**
- Produces: `pricing.LoadGidMap(path string) (map[string]string, error)`.

- [ ] **Step 1: Write the failing test**

```go
package pricing

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSettingsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.ini")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing settings file: %v", err)
	}
	return path
}

func TestLoadGidMap_ParsesGidBlock(t *testing.T) {
	path := writeSettingsFile(t, "[GoogleSheets]\n<gid>\nCOOP = 1741405320\nBIGC = 925001622\n</gid>\n")

	gidMap, err := LoadGidMap(path)
	if err != nil {
		t.Fatalf("LoadGidMap returned error: %v", err)
	}
	if gidMap["COOP"] != "1741405320" {
		t.Fatalf("gidMap[COOP] = %q, want %q", gidMap["COOP"], "1741405320")
	}
	if gidMap["BIGC"] != "925001622" {
		t.Fatalf("gidMap[BIGC] = %q, want %q", gidMap["BIGC"], "925001622")
	}
}

func TestLoadGidMap_NoGidBlockReturnsEmptyMap(t *testing.T) {
	path := writeSettingsFile(t, "[GoogleSheets]\nCOOP = 1741405320\n")

	gidMap, err := LoadGidMap(path)
	if err != nil {
		t.Fatalf("LoadGidMap returned error: %v", err)
	}
	if len(gidMap) != 0 {
		t.Fatalf("gidMap = %v, want empty", gidMap)
	}
}

func TestLoadGidMap_MalformedLineReturnsError(t *testing.T) {
	path := writeSettingsFile(t, "<gid>\nCOOP = 123 = 456\n</gid>\n")

	if _, err := LoadGidMap(path); err == nil {
		t.Fatal("LoadGidMap expected error for a line with more than one '=', got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/pricing/... -run TestLoadGidMap -v`
Expected: FAIL — `LoadGidMap` not defined.

- [ ] **Step 3: Write minimal implementation**

```go
package pricing

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var gidBlockPattern = regexp.MustCompile(`(?s)<gid>(.*?)</gid>`)

// LoadGidMap reads the <gid>...</gid> block from settings.ini — a
// bespoke tag, not real XML or an INI section — and returns a map of
// sheet name -> Google Sheets gid, mirroring xulydonhang.py's get_gid.
// A line inside the block with more than one "=" is an error, matching
// Python's `key, value = line.split("=")` unpacking failure.
func LoadGidMap(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("pricing: read %s: %w", path, err)
	}

	match := gidBlockPattern.FindStringSubmatch(string(content))
	if match == nil {
		return map[string]string{}, nil
	}

	gidMap := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(match[1]), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "=") {
			continue
		}
		parts := strings.Split(line, "=")
		if len(parts) != 2 {
			return nil, fmt.Errorf("pricing: malformed <gid> line (expected exactly one '='): %q", line)
		}
		gidMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return gidMap, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd GO && go test ./internal/processing/pricing/... -run TestLoadGidMap -v`
Expected: PASS, all 3 tests.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/pricing/gid.go GO/internal/processing/pricing/gid_test.go
git commit -m "feat(go): add settings.ini <gid> block parser"
```

---

### Task 3: `internal/processing/pricing` — CSV price/promotion index

**Files:**
- Create: `GO/internal/processing/pricing/index.go`
- Create: `GO/internal/processing/pricing/http_source.go`
- Test: `GO/internal/processing/pricing/index_test.go`

**Interfaces:**
- Consumes: `pricing.LoadGidMap` (Task 2).
- Produces: `pricing.Index` (built via `pricing.ParseIndex(csvRows [][]string) *Index`), `(*Index).FindPrice(sku string) (string, bool)`, `(*Index).FindPromotions(sku, timeToCheck string) []Promotion`, `(*Index).FindInvoicePromotion(timeToCheck string) string`, `type Promotion struct { Column, Value string }`. `pricing.NewHTTPSource(settingsPath string) *HTTPSource` with `(*HTTPSource).FetchCoopIndex() (*Index, error)` — the production data source; Task 12 depends on a `PricingSource` interface this satisfies.

**Design note (from the spec):** `find_price_by_sku` and `find_all_promotions_by_sku_and_time` fetch the *same* Google Sheet/gid at their real Coop call site (both use `gid = get_gid("COOP")`) — one CSV fetch builds both views instead of one network call per SKU.

- [ ] **Step 1: Write the failing test**

```go
package pricing

import "testing"

func csvRows() [][]string {
	return [][]string{
		{"STT", "Mã hàng", "Tên hàng", "Giá"},
		{"1", "1234567", "Nước giặt", "141.272"},
		{"2", "7654321", "Chai tay", "50.000"},
	}
}

func TestParseIndex_FindPrice(t *testing.T) {
	idx := ParseIndex(csvRows())

	price, ok := idx.FindPrice("1234567")
	if !ok {
		t.Fatal("FindPrice(1234567) = not found, want found")
	}
	if price != "141272" {
		t.Fatalf("FindPrice(1234567) = %q, want %q", price, "141272")
	}

	if _, ok := idx.FindPrice("0000000"); ok {
		t.Fatal("FindPrice(0000000) = found, want not found")
	}
}

func TestParseIndex_FindPriceRequiresExactWhitespaceStrippedQuery(t *testing.T) {
	idx := ParseIndex(csvRows())

	// The Python original strips whitespace only from the query SKU,
	// never from the stored CSV column value — preserved here.
	price, ok := idx.FindPrice("  1234567  ")
	if !ok || price != "141272" {
		t.Fatalf("FindPrice(with whitespace) = (%q, %v), want (%q, true)", price, ok, "141272")
	}
}

func promotionCsvRows() [][]string {
	return [][]string{
		{"Mã hàng", "1/1-15/1", "16/1-31/1"},
		{"1234567", "", "Mua 2 tặng 1 (cf mua 2 tặng 1)"},
	}
}

func TestParseIndex_FindPromotionsWithinDateRange(t *testing.T) {
	idx := ParseIndex(promotionCsvRows())

	promos := idx.FindPromotions("1234567", "20/01/2026")
	if len(promos) != 1 {
		t.Fatalf("FindPromotions = %d promos, want 1: %+v", len(promos), promos)
	}
	if promos[0].Column != "16/1-31/1" {
		t.Fatalf("promo column = %q, want %q", promos[0].Column, "16/1-31/1")
	}

	none := idx.FindPromotions("1234567", "05/01/2026")
	if len(none) != 0 {
		t.Fatalf("FindPromotions outside range = %d promos, want 0", len(none))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/pricing/... -run TestParseIndex -v`
Expected: FAIL — `ParseIndex` not defined.

- [ ] **Step 3: Write minimal implementation**

```go
package pricing

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Promotion is one (date-range column, promo text) match, mirroring the
// (col, value) tuples find_all_promotions_by_sku_and_time returns.
type Promotion struct {
	Column string
	Value  string
}

// Index holds one fetched Coop pricing/promotion CSV in memory, so a
// whole order's worth of SKU/promotion lookups costs one network fetch
// instead of one fetch per SKU (xulydonhang.py's find_price_by_sku and
// find_all_promotions_by_sku_and_time each fetch the sheet fresh on
// every single call).
type Index struct {
	rows           [][]string // raw rows, row 0 = header
	priceBySku     map[string]string
	header         []string
	skuColumnIndex int // -1 if no "Mã hàng" column found
}

// ParseIndex mirrors both find_price_by_sku's positional 4-column read
// (SKU at index 1, price at index 3, "." stripped) and
// find_all_promotions_by_sku_and_time's named-header read (first column
// containing "Mã hàng"), from the same underlying CSV rows.
func ParseIndex(csvRows [][]string) *Index {
	idx := &Index{rows: csvRows, priceBySku: make(map[string]string), skuColumnIndex: -1}

	if len(csvRows) > 0 {
		idx.header = normalizeHeader(csvRows[0])
		for i, h := range idx.header {
			if strings.Contains(h, "Mã hàng") {
				idx.skuColumnIndex = i
				break
			}
		}
	}

	for _, row := range csvRows[minInt(1, len(csvRows)):] {
		if len(row) < 4 {
			continue
		}
		sku := row[1]
		price := strings.ReplaceAll(row[3], ".", "")
		if strings.TrimSpace(price) != "" {
			idx.priceBySku[sku] = price
		}
	}

	return idx
}

func normalizeHeader(row []string) []string {
	out := make([]string, len(row))
	for i, h := range row {
		h = strings.TrimSpace(h)
		h = strings.ReplaceAll(h, "\n", " ")
		h = strings.ReplaceAll(h, "\r", "")
		out[i] = h
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// FindPrice mirrors find_price_by_sku's lookup: the query SKU has all
// whitespace stripped before comparison, but stored CSV values are
// compared as-is (not stripped) — preserved exactly.
func (idx *Index) FindPrice(sku string) (string, bool) {
	sku = wsPattern.ReplaceAllString(sku, "")
	price, ok := idx.priceBySku[sku]
	return price, ok
}

var wsPattern = regexp.MustCompile(`\s+`)
var dateRangePattern = regexp.MustCompile(`(\d{1,2}/\d{1,2})-(\d{1,2}/\d{1,2})`)
var yearSuffixPattern = regexp.MustCompile(`/\d{4}$`)

// FindPromotions mirrors find_all_promotions_by_sku_and_time: finds the
// SKU's row via the "Mã hàng"-named column, then returns every
// (column, value) pair whose column header is a "D/M-D/M" date range
// containing timeToCheck, skipping empty values.
func (idx *Index) FindPromotions(sku, timeToCheck string) []Promotion {
	if idx.skuColumnIndex < 0 || len(idx.rows) < 2 {
		return nil
	}

	var skuRow []string
	for _, row := range idx.rows[1:] {
		if idx.skuColumnIndex < len(row) && row[idx.skuColumnIndex] == sku {
			skuRow = row
			break
		}
	}
	if skuRow == nil {
		return nil
	}

	var promos []Promotion
	for i, h := range idx.header {
		if !isWithinDateRange(timeToCheck, h) {
			continue
		}
		if i >= len(skuRow) {
			continue
		}
		value := skuRow[i]
		if strings.TrimSpace(value) != "" {
			promos = append(promos, Promotion{Column: h, Value: value})
		}
	}
	return promos
}

// FindInvoicePromotion mirrors write_to_dondathang's invoice-level bonus
// lookup: find_all_promotions_by_sku_and_time("Hóa Đơn", entry_date,
// vendor) — same mechanism as FindPromotions but keyed on the literal
// SKU value "Hóa Đơn" instead of a real product SKU, then returns the
// single-column-string form used directly in write_to_dondathang
// (`kmhoadon = ProcessHandler.find_all_promotions_by_sku_and_time(...)`,
// then `if kmhoadon:` treats the whole returned value as one string,
// since exactly one date-range column is expected to be active at a
// time in practice). Returns "" if nothing matched.
func (idx *Index) FindInvoicePromotion(timeToCheck string) string {
	promos := idx.FindPromotions("Hóa Đơn", timeToCheck)
	if len(promos) == 0 {
		return ""
	}
	return promos[0].Value
}

func normalizeDDMM(s string) (string, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return "", errInvalidDate
	}
	day, err1 := strconv.Atoi(parts[0])
	month, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return "", errInvalidDate
	}
	return pad2(day) + "/" + pad2(month), nil
}

func pad2(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

var errInvalidDate = &dateError{}

type dateError struct{}

func (*dateError) Error() string { return "invalid date" }

func isWithinDateRange(timeToCheck, columnName string) bool {
	m := dateRangePattern.FindStringSubmatch(columnName)
	if m == nil {
		return false
	}
	startRaw, endRaw := m[1], m[2]

	// Mirrors is_within_date_range's swap heuristic: strip a trailing
	// /YYYY from timeToCheck, then if the first component is <=12 and
	// the second is >12, treat the input as M/D and swap to D/M.
	tc := yearSuffixPattern.ReplaceAllString(timeToCheck, "")
	parts := strings.Split(tc, "/")
	if len(parts) == 2 {
		p1, err1 := strconv.Atoi(parts[0])
		p2, err2 := strconv.Atoi(parts[1])
		if err1 == nil && err2 == nil && p1 <= 12 && p2 > 12 {
			tc = strconv.Itoa(p2) + "/" + strconv.Itoa(p1)
		}
	}

	start, err1 := normalizeDDMM(startRaw)
	end, err2 := normalizeDDMM(endRaw)
	tcNorm, err3 := normalizeDDMM(tc)
	if err1 != nil || err2 != nil || err3 != nil {
		return false
	}

	year := time.Now().Year()
	layout := "02/01/2006"
	tcDate, err4 := time.Parse(layout, tcNorm+"/"+strconv.Itoa(year))
	startDate, err5 := time.Parse(layout, start+"/"+strconv.Itoa(year))
	endDate, err6 := time.Parse(layout, end+"/"+strconv.Itoa(year))
	if err4 != nil || err5 != nil || err6 != nil {
		return false
	}

	return !tcDate.Before(startDate) && !tcDate.After(endDate)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd GO && go test ./internal/processing/pricing/... -run TestParseIndex -v`
Expected: PASS, all subtests.

- [ ] **Step 5: Write the HTTP data source (no dedicated test — this is a thin network wrapper; correctness is validated by the golden fixture test in Task 15, which uses a fixture-backed `PricingSource` instead)**

```go
package pricing

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"
)

const spreadsheetID = "1yvxE_SPYXKhofcZdhv1CSKAyiwdY1Mf4pFlsiMbtOr4"

// HTTPSource fetches Coop's live pricing/promotion sheet over HTTP. It
// is the production PricingSource; Task 12's tests substitute a
// fixture-backed implementation instead of hitting the network.
type HTTPSource struct {
	SettingsPath string
	Client       *http.Client
}

func NewHTTPSource(settingsPath string) *HTTPSource {
	return &HTTPSource{SettingsPath: settingsPath, Client: &http.Client{Timeout: 30 * time.Second}}
}

// FetchCoopIndex mirrors find_price_by_sku/find_all_promotions_by_sku_and_time's
// URL construction (both use gid = get_gid("COOP") at their real Coop
// call site) and fetches it once.
func (s *HTTPSource) FetchCoopIndex() (*Index, error) {
	gidMap, err := LoadGidMap(s.SettingsPath)
	if err != nil {
		return nil, err
	}
	gid, ok := gidMap["COOP"]
	if !ok {
		return nil, fmt.Errorf("pricing: no COOP gid in %s", s.SettingsPath)
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

- [ ] **Step 6: Confirm the package builds**

Run: `cd GO && go build ./internal/processing/pricing/...`
Expected: builds with no errors.

- [ ] **Step 7: Commit**

```bash
git add GO/internal/processing/pricing/
git commit -m "feat(go): add Coop pricing/promotion index and HTTP source"
```

---

### Task 4: `internal/processing/productdata` — cached `data.xlsx` lookups

**Files:**
- Create: `GO/internal/processing/productdata/store.go`
- Test: `GO/internal/processing/productdata/store_test.go`
- Test fixture: `GO/internal/processing/productdata/testdata/data.xlsx` (small hand-built copy, see Step 1)

**Interfaces:**
- Produces: `productdata.ProductInfo{Name string, WeightKg float64, PackSize float64}`, `productdata.Load(path string) (*productdata.Store, error)`, `(*Store).GetCustomerCode(poLocation string) string`, `(*Store).GetSystemForCustomer(customerCode string) string`, `(*Store).GetCoopfoodAddress(customerCode string) string`, `(*Store).GetProductInfo(sku string) (ProductInfo, bool)`, `(*Store).ResolveSku(barcode string) string`, `(*Store).FindSkusMentioned(text string) []string`.

- [ ] **Step 1: Build a minimal test `data.xlsx` fixture**

Create `GO/internal/processing/productdata/testdata/data.xlsx` with two sheets:

`MaKH` (header row 1, data from row 2):
| A (Hệ thống) | B (Mã ST) | C (Mã KH) | D (Địa chỉ) |
|---|---|---|---|
| COOP | 999 | KH-COOP-001 | (blank) |
| COOPFOOD | 888 | KH-CF-002 | 12 Nguyễn Huệ |
| LOTTE | 777 | KH-LOTTE-003 | (blank) |

`SanPham` (header row 1, data from row 2):
| A (Mã hàng Công ty) | B (Tên hàng) | C (Trọng lượng kg) | D (Quy cách thùng) | J (Mã hàng Coop) |
|---|---|---|---|---|
| SP0001 | Nước giặt Blue | 3.6 | 24 | 1234567 |
| SP0002 | Chai tay toilet | 0.18 | 48 | 7654321 |

You can build this with any spreadsheet tool, or with a short throwaway script using `github.com/xuri/excelize/v2`'s `NewFile()`/`SetCellValue`/`SaveAs` — either way, the result just needs to exist at that path with this exact shape before Step 2 runs.

- [ ] **Step 2: Write the failing test**

```go
package productdata

import "testing"

const testDataPath = "testdata/data.xlsx"

func TestGetCustomerCode_MatchesTrailingDigitsOfColumnC(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	// Preserves get_makhachhang's bug: it reads column C (index 2), not
	// column B (index 1), for the trailing-digit match — so matching
	// against "999" (column B's store code) must NOT work; matching
	// against the trailing digits of column C ("KH-COOP-001" -> "001")
	// must work instead.
	if got := store.GetCustomerCode("999"); got != "Không tìm thấy" {
		t.Fatalf("GetCustomerCode(999) = %q, want Không tìm thấy (bug preserved: column B is not actually read)", got)
	}
	if got := store.GetCustomerCode("001"); got != "KH-COOP-001" {
		t.Fatalf("GetCustomerCode(001) = %q, want %q", got, "KH-COOP-001")
	}
}

func TestGetSystemForCustomer(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := store.GetSystemForCustomer("KH-CF-002"); got != "COOPFOOD" {
		t.Fatalf("GetSystemForCustomer(KH-CF-002) = %q, want COOPFOOD", got)
	}
	if got := store.GetSystemForCustomer("no-such-code"); got != "" {
		t.Fatalf("GetSystemForCustomer(no-such-code) = %q, want empty", got)
	}
}

func TestGetCoopfoodAddress(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := store.GetCoopfoodAddress("KH-CF-002"); got != "12 Nguyễn Huệ" {
		t.Fatalf("GetCoopfoodAddress(KH-CF-002) = %q, want %q", got, "12 Nguyễn Huệ")
	}
}

func TestGetProductInfo(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	info, ok := store.GetProductInfo("SP0001")
	if !ok {
		t.Fatal("GetProductInfo(SP0001) not found")
	}
	if info.Name != "Nước giặt Blue" || info.WeightKg != 3.6 || info.PackSize != 24 {
		t.Fatalf("GetProductInfo(SP0001) = %+v, want Name=Nước giặt Blue WeightKg=3.6 PackSize=24", info)
	}
}

func TestResolveSku_MapsVendorSkuToInternalCode(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := store.ResolveSku("1234567-1"); got != "SP0001" {
		t.Fatalf("ResolveSku(1234567-1) = %q, want %q", got, "SP0001")
	}
	if got := store.ResolveSku("9999999-9"); got != "9999999" {
		t.Fatalf("ResolveSku(9999999-9) unmapped = %q, want cleaned-but-unmapped %q", got, "9999999")
	}
}

func TestFindSkusMentioned(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	got := store.FindSkusMentioned("Tặng kèm SP0001 khi mua 2")
	if len(got) != 1 || got[0] != "SP0001" {
		t.Fatalf("FindSkusMentioned = %v, want [SP0001]", got)
	}
	if got := store.FindSkusMentioned("không có mã nào ở đây"); len(got) != 0 {
		t.Fatalf("FindSkusMentioned(no match) = %v, want empty", got)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/productdata/... -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 4: Write minimal implementation**

```go
package productdata

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ProductInfo mirrors the columns in data.xlsx's SanPham sheet needed by
// the Coop pipeline: B=name, C=weight (kg), D=case pack size.
type ProductInfo struct {
	Name     string
	WeightKg float64
	PackSize float64
}

// Store is an in-memory index over data.xlsx's MaKH and SanPham sheets,
// loaded once at startup. xulydonhang.py re-opens data.xlsx from disk on
// almost every single lookup call; Store exists to avoid that.
type Store struct {
	customerRows   [][4]string
	products       map[string]ProductInfo
	skuMapping     map[string]string
	skuAlternation *regexp.Regexp
}

func Load(path string) (*Store, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("productdata: open %s: %w", path, err)
	}
	defer f.Close()

	customerRows, err := loadCustomerRows(f)
	if err != nil {
		return nil, err
	}
	products, skuMapping, err := loadProducts(f)
	if err != nil {
		return nil, err
	}

	return &Store{
		customerRows:   customerRows,
		products:       products,
		skuMapping:     skuMapping,
		skuAlternation: buildSkuAlternation(products),
	}, nil
}

func loadCustomerRows(f *excelize.File) ([][4]string, error) {
	rows, err := f.GetRows("MaKH")
	if err != nil {
		return nil, fmt.Errorf("productdata: read MaKH sheet: %w", err)
	}
	var out [][4]string
	for i, row := range rows {
		if i == 0 {
			continue
		}
		var r [4]string
		for c := 0; c < 4 && c < len(row); c++ {
			r[c] = row[c]
		}
		out = append(out, r)
	}
	return out, nil
}

func loadProducts(f *excelize.File) (map[string]ProductInfo, map[string]string, error) {
	rows, err := f.GetRows("SanPham")
	if err != nil {
		return nil, nil, fmt.Errorf("productdata: read SanPham sheet: %w", err)
	}

	products := make(map[string]ProductInfo)
	skuMapping := make(map[string]string)
	ws := regexp.MustCompile(`\s+`)

	for i, row := range rows {
		if i == 0 || len(row) == 0 || strings.TrimSpace(row[0]) == "" {
			continue
		}
		skuCode := strings.TrimSpace(row[0])

		info := ProductInfo{}
		if len(row) > 1 {
			info.Name = row[1]
		}
		if len(row) > 2 {
			info.WeightKg = parseFloat(row[2])
		}
		if len(row) > 3 {
			info.PackSize = parseFloat(row[3])
		}
		products[skuCode] = info

		// Mirrors load_sku_mapping: EVERY non-empty cell from column C
		// (index 2) onward maps back to this row's internal SKU,
		// including the weight/pack-size columns, not just the
		// per-vendor SKU columns further right — preserved verbatim
		// from xulydonhang.py, see this plan's Global Constraints.
		for c := 2; c < len(row); c++ {
			cell := ws.ReplaceAllString(row[c], "")
			if cell != "" {
				skuMapping[cell] = skuCode
			}
		}
	}

	return products, skuMapping, nil
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func buildSkuAlternation(products map[string]ProductInfo) *regexp.Regexp {
	if len(products) == 0 {
		return nil
	}
	skus := make([]string, 0, len(products))
	for sku := range products {
		skus = append(skus, regexp.QuoteMeta(sku))
	}
	sort.Strings(skus)
	return regexp.MustCompile(`\b(` + strings.Join(skus, "|") + `)\b`)
}

var trailingDigitsPattern = regexp.MustCompile(`(\d+)$`)

// GetCustomerCode mirrors get_makhachhang exactly, including its bug —
// see this plan's Global Constraints for the exact explanation.
func (s *Store) GetCustomerCode(poLocation string) string {
	poLocation = strings.TrimSpace(poLocation)
	for _, row := range s.customerRows {
		colA, colB, colC := row[0], row[2], row[2]
		system := strings.ToUpper(strings.TrimSpace(colA))
		if system != "COOP" && system != "COOPFOOD" {
			continue
		}
		if colB == "" {
			continue
		}
		m := trailingDigitsPattern.FindStringSubmatch(strings.TrimSpace(colB))
		if m == nil {
			continue
		}
		if m[1] == poLocation {
			return colC
		}
	}
	return "Không tìm thấy"
}

// GetSystemForCustomer mirrors layhethong_COOP: column C -> column A.
func (s *Store) GetSystemForCustomer(customerCode string) string {
	customerCode = strings.TrimSpace(customerCode)
	for _, row := range s.customerRows {
		colC := row[2]
		if strings.TrimSpace(colC) == customerCode {
			return row[0]
		}
	}
	return ""
}

// GetCoopfoodAddress mirrors laydiachi_coopfood: column C -> column D.
func (s *Store) GetCoopfoodAddress(customerCode string) string {
	customerCode = strings.TrimSpace(customerCode)
	for _, row := range s.customerRows {
		if strings.TrimSpace(row[2]) == customerCode {
			return row[3]
		}
	}
	return ""
}

// GetProductInfo merges timten_sanpham/timtrongluong_sanpham/
// timquycach_sanpham (three separate linear scans + file re-opens in
// Python) into one lookup against the Store loaded once at startup.
func (s *Store) GetProductInfo(sku string) (ProductInfo, bool) {
	info, ok := s.products[sku]
	return info, ok
}

var skuCleanupPattern = regexp.MustCompile(`(\d{7})-\d`)

// CleanSkuNumber mirrors clean_sku_number: strips a Coop-style
// "1234567-1" barcode down to its 7-digit prefix.
func CleanSkuNumber(sku string) string {
	m := skuCleanupPattern.FindStringSubmatch(sku)
	if m == nil {
		return sku
	}
	return m[1]
}

// ResolveSku mirrors clean_sku_number + replace_sku_numbers' mapping
// lookup: returns the internal SKU if a mapping entry exists, else the
// cleaned (but unmapped) value.
func (s *Store) ResolveSku(barcode string) string {
	cleaned := CleanSkuNumber(barcode)
	if mapped, ok := s.skuMapping[cleaned]; ok {
		return mapped
	}
	return cleaned
}

// FindSkusMentioned mirrors check_value_in_sanpham: scans free text for
// any known internal SKU as a whole word, returning every match in the
// order found (duplicates included, matching Python's re.findall).
func (s *Store) FindSkusMentioned(text string) []string {
	if text == "" || s.skuAlternation == nil {
		return nil
	}
	return s.skuAlternation.FindAllString(text, -1)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd GO && go test ./internal/processing/productdata/... -v`
Expected: PASS, all tests.

- [ ] **Step 6: Commit**

```bash
git add GO/internal/processing/productdata/
git commit -m "feat(go): add cached data.xlsx product/customer lookups"
```

---

### Task 5: `internal/processing/coop` — page dispatch (`CountPOsOnPage`, `SplitMultiPO`)

**Files:**
- Create: `GO/internal/processing/coop/dispatch.go`
- Test: `GO/internal/processing/coop/dispatch_test.go`

**Interfaces:**
- Produces: `coop.PageCounts{POM343, SubTotal int}`, `coop.CountPOsOnPage(text string) PageCounts`, `coop.SplitMultiPO(text string) []string`.

- [ ] **Step 1: Write the failing test**

```go
package coop

import "testing"

func TestCountPOsOnPage(t *testing.T) {
	text := "POM343 first order\nSub Total\nPOM343 second order\nSub Total"
	got := CountPOsOnPage(text)
	if got.POM343 != 2 || got.SubTotal != 2 {
		t.Fatalf("CountPOsOnPage = %+v, want POM343=2 SubTotal=2", got)
	}
}

func TestCountPOsOnPage_LowercaseNormalized(t *testing.T) {
	text := "pom343 order\nSub Total"
	got := CountPOsOnPage(text)
	if got.POM343 != 1 || got.SubTotal != 1 {
		t.Fatalf("CountPOsOnPage(lowercase) = %+v, want POM343=1 SubTotal=1", got)
	}
}

func TestSplitMultiPO_UppercaseInputFindsNoSegments(t *testing.T) {
	// Regression test for the preserved bug documented in this plan's
	// Global Constraints: catdonra_nhieutrang's split uses a lowercase
	// literal keyword against text that, on real Coop PDFs, is
	// uppercase ("POM343") — so on real data this returns no
	// mid-document segments beyond the first "Sub Total" boundary.
	text := "header stuff\nSub Total\nPOM343 second order text\nSub Total\nfooter"
	segments := SplitMultiPO(text)
	if len(segments) != 1 {
		t.Fatalf("SplitMultiPO(uppercase POM343) = %d segments, want 1 (bug preserved): %v", len(segments), segments)
	}
}

func TestSplitMultiPO_LowercaseInputSplitsCorrectly(t *testing.T) {
	text := "header stuff\nSub Total\npom343 second order text\nSub Total\nfooter"
	segments := SplitMultiPO(text)
	if len(segments) != 2 {
		t.Fatalf("SplitMultiPO(lowercase pom343) = %d segments, want 2: %v", len(segments), segments)
	}
	if segments[1] != "POM343 second order text" {
		t.Fatalf("segments[1] = %q, want %q", segments[1], "POM343 second order text")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/coop/... -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Write minimal implementation**

```go
package coop

import (
	"regexp"
	"strings"
)

var (
	pom34Pattern         = regexp.MustCompile(`POM34[36]\b`)
	subTotalCountPattern = regexp.MustCompile(`\bSub\s*(?:Total|Tot\s*al)\b`)
)

// PageCounts mirrors demsodonhang1trang_coop's returned dict.
type PageCounts struct {
	POM343   int
	SubTotal int
}

// CountPOsOnPage mirrors demsodonhang1trang_coop.
func CountPOsOnPage(text string) PageCounts {
	text = strings.NewReplacer(" ", " ", "pom343", "POM343", "pom346", "POM346").Replace(text)
	return PageCounts{
		POM343:   len(pom34Pattern.FindAllString(text, -1)),
		SubTotal: len(subTotalCountPattern.FindAllString(text, -1)),
	}
}

// SplitMultiPO mirrors catdonra_nhieutrang exactly, including its
// latent case-sensitivity bug (see this plan's Global Constraints): the
// membership check (`keyword in text.lower()`) is case-insensitive, but
// the actual split (`text.split(keyword, 1)`) is case-sensitive against
// the lowercase literal keyword.
func SplitMultiPO(text string) []string {
	var segments []string

	parts := strings.SplitN(text, "Sub Total", 2)
	if len(parts) < 2 {
		return segments
	}
	segments = append(segments, parts[0])
	text = parts[1]

	for containsAny(strings.ToLower(text), "pom343", "pom346") && strings.Contains(text, "Sub Total") {
		for _, keyword := range []string{"pom343", "pom346"} {
			if !strings.Contains(strings.ToLower(text), keyword) {
				continue
			}
			// Case-sensitive on purpose — see the doc comment above.
			splitParts := strings.SplitN(text, keyword, 2)
			text = splitParts[len(splitParts)-1]

			subParts := strings.SplitN(text, "Sub Total", 2)
			if len(subParts) > 1 {
				segments = append(segments, strings.ToUpper(keyword)+subParts[0])
				text = subParts[1]
			} else {
				segments = append(segments, strings.ToUpper(keyword)+text)
				return segments
			}
		}
	}

	return segments
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd GO && go test ./internal/processing/coop/... -v`
Expected: PASS, all 4 tests — in particular, confirm `TestSplitMultiPO_UppercaseInputFindsNoSegments` passes, proving the bug is faithfully preserved.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/coop/dispatch.go GO/internal/processing/coop/dispatch_test.go
git commit -m "feat(go): add Coop page dispatch (count/split multi-PO pages)"
```

---

### Task 6: `internal/processing/coop` — invoice info parsing

**Files:**
- Create: `GO/internal/processing/coop/invoice.go`
- Test: `GO/internal/processing/coop/invoice_test.go`

**Interfaces:**
- Produces: `coop.InvoiceInfo{PONumber, POLocation, EntryDate, CancelDate string}`, `coop.ParseInvoiceInfo(text string) InvoiceInfo`, `coop.RemoveLineNumbers(text string) string`, `coop.StripAllWhitespace(text string) string`, `coop.ExtractNotes(text string) string`, `coop.ExtractShipTo(text string) string`, `coop.ConvertDateFormat(dateStr string) string`, `coop.ResolveCancelDate(entryDate, cancelDate string) (string, error)`.

- [ ] **Step 1: Write the failing test**

```go
package coop

import "testing"

func TestParseInvoiceInfo_ExtractsCoreFields(t *testing.T) {
	text := "P/O Number:   102945235-00\nP/O Location:    140\nEntry Date           - 23/07/26\nCancel Date          - 26/09/26\nCurrency- VND Viet Nam Dong"
	info := ParseInvoiceInfo(text)

	if info.PONumber != "102945235-00" {
		t.Fatalf("PONumber = %q, want %q", info.PONumber, "102945235-00")
	}
	if info.POLocation != "140" {
		t.Fatalf("POLocation = %q, want %q", info.POLocation, "140")
	}
	if info.EntryDate != "23/07/26" {
		t.Fatalf("EntryDate = %q, want %q", info.EntryDate, "23/07/26")
	}
	if info.CancelDate != "26/09/26" {
		t.Fatalf("CancelDate = %q, want %q", info.CancelDate, "26/09/26")
	}
}

func TestParseInvoiceInfo_FallsBackToStoreForLocation(t *testing.T) {
	text := "P/O Number: 999-00\nStore-   42   Vendor\nEntry Date - 01/01/26"
	info := ParseInvoiceInfo(text)
	if info.POLocation != "42" {
		t.Fatalf("POLocation (Store fallback) = %q, want %q", info.POLocation, "42")
	}
}

func TestParseInvoiceInfo_MissingFieldsReturnKhongTimThay(t *testing.T) {
	info := ParseInvoiceInfo("nothing relevant here")
	if info.PONumber != "Không tìm thấy" || info.CancelDate != "Không tìm thấy" {
		t.Fatalf("info = %+v, want all fields Không tìm thấy", info)
	}
}

func TestConvertDateFormat(t *testing.T) {
	if got := ConvertDateFormat("23/07/26"); got != "23/07/2026" {
		t.Fatalf("ConvertDateFormat(23/07/26) = %q, want %q", got, "23/07/2026")
	}
	if got := ConvertDateFormat("Không tìm thấy"); got != "Không tìm thấy" {
		t.Fatalf("ConvertDateFormat(not found) = %q, want unchanged", got)
	}
	if got := ConvertDateFormat("not-a-date"); got != "Không hợp lệ" {
		t.Fatalf("ConvertDateFormat(garbage) = %q, want %q", got, "Không hợp lệ")
	}
}

func TestResolveCancelDate_DefaultsTo65DaysAfterEntry(t *testing.T) {
	got, err := ResolveCancelDate("23/07/2026", "Không tìm thấy")
	if err != nil {
		t.Fatalf("ResolveCancelDate returned error: %v", err)
	}
	if got != "26/09/2026" {
		t.Fatalf("ResolveCancelDate = %q, want %q", got, "26/09/2026")
	}
}

func TestResolveCancelDate_KeepsExplicitCancelDate(t *testing.T) {
	got, err := ResolveCancelDate("23/07/2026", "01/08/2026")
	if err != nil {
		t.Fatalf("ResolveCancelDate returned error: %v", err)
	}
	if got != "01/08/2026" {
		t.Fatalf("ResolveCancelDate = %q, want %q", got, "01/08/2026")
	}
}

func TestExtractNotes_StripsBoilerplateAndDedupes(t *testing.T) {
	text := "Notes - Xin vui long kem DDH khi giao hang. Mot Hoa Don chi xuat cho mot PO. Ghi chu rieng Ghi chu rieng FOB - SHIPPING POINT"
	got := ExtractNotes(text)
	if got != "Ghi chu rieng ." && got != "Ghi chu rieng" {
		// Punctuation from the source boilerplate sentences may remain
		// adjacent to "Ghi chu rieng" depending on exact spacing; the
		// key behavior under test is de-duplication of the repeated
		// phrase and removal of the known boilerplate sentences.
		t.Logf("ExtractNotes = %q (informational — verify against golden fixtures in Task 15, not this loose check)", got)
	}
	if got == "" {
		t.Fatal("ExtractNotes returned empty, expected the de-duplicated custom note to survive")
	}
}

func TestExtractShipTo_PrefersStatusReleasedForm(t *testing.T) {
	text := "Ship To: Status- 3 RELEASED Co.opMart Nha Trang Contact- none"
	got := ExtractShipTo(text)
	if got != "Co.opMart Nha Trang" {
		t.Fatalf("ExtractShipTo = %q, want %q", got, "Co.opMart Nha Trang")
	}
}

func TestExtractShipTo_FallsBackToStoreVendorForm(t *testing.T) {
	text := "Store- Co.opMart District 1 Vendor: 21569"
	got := ExtractShipTo(text)
	if got != "Co.opMart District 1" {
		t.Fatalf("ExtractShipTo = %q, want %q", got, "Co.opMart District 1")
	}
}

func TestExtractShipTo_EmptyWhenNeitherFormMatches(t *testing.T) {
	if got := ExtractShipTo("nothing relevant"); got != "" {
		t.Fatalf("ExtractShipTo = %q, want empty", got)
	}
}
```

Note on `TestExtractNotes_StripsBoilerplateAndDedupes`: this test is deliberately loose (it logs rather than hard-asserts the exact string) because the precise punctuation-adjacency outcome of `ExtractNotes` on hand-written sample text is hard to predict by eye without running it — Task 15's golden fixture test is the actual correctness gate for this function against 155 real notes fields. Do not spend time hand-tuning this test to pass on a specific string; keep the loose check and rely on Task 15.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/coop/... -run TestParseInvoiceInfo -v`
Expected: FAIL — the new functions don't exist yet.

- [ ] **Step 3: Write minimal implementation**

```go
package coop

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const notFound = "Không tìm thấy"

var whitespacePattern = regexp.MustCompile(`\s+`)

// StripAllWhitespace mirrors xoakhoangtrang: removes ALL whitespace
// (not just collapsing runs) from text.
func StripAllWhitespace(text string) string {
	return whitespacePattern.ReplaceAllString(text, "")
}

var firstTokenPattern = regexp.MustCompile(`^\s*(\S+)(?:\s+(.*))?$`)

// RemoveLineNumbers mirrors remove_line_numbers: strips a leading "1"
// through "10" token (and the whitespace after it) from each line,
// dropping lines that contain only such a token.
func RemoveLineNumbers(text string) string {
	validNumbers := map[string]bool{}
	for i := 1; i <= 10; i++ {
		validNumbers[fmt.Sprintf("%d", i)] = true
	}

	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			cleaned = append(cleaned, line)
			continue
		}
		m := firstTokenPattern.FindStringSubmatch(line)
		if m == nil {
			cleaned = append(cleaned, line)
			continue
		}
		first, rest := m[1], m[2]
		if validNumbers[first] {
			if rest != "" {
				cleaned = append(cleaned, rest)
			}
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.Join(cleaned, "\n")
}

func spacedPattern(literal string) string {
	var b strings.Builder
	for i, r := range literal {
		if i > 0 {
			b.WriteString(`\s*`)
		}
		b.WriteString(regexp.QuoteMeta(string(r)))
	}
	return b.String()
}

var (
	poNumberPattern      = regexp.MustCompile(spacedPattern("P/O Number") + `\s*[:-]?\s*([\d-]+)`)
	poLocationPattern    = regexp.MustCompile(spacedPattern("P/O Location") + `\s*:\s*(\d+)`)
	entryDatePattern     = regexp.MustCompile(spacedPattern("Entry Date") + `\s*-\s*([\d/]+)`)
	cancelDatePattern    = regexp.MustCompile(`Cancel\s*Date-\s*([\d/]+)`)
	storeLocationPattern = regexp.MustCompile(`Store-\s*(\d+)`)
)

// InvoiceInfo mirrors extract_info's returned dict.
type InvoiceInfo struct {
	PONumber   string
	POLocation string
	EntryDate  string
	CancelDate string
}

// ParseInvoiceInfo mirrors xulydonhang.py's extract_info. Note that,
// like the Python original, it strips ALL whitespace from the text
// (not just collapsing it) before matching — the `\s*` in every pattern
// above then matches zero characters, so this still works whether the
// source PDF text had normal spacing, character-by-character spacing,
// or none at all.
func ParseInvoiceInfo(text string) InvoiceInfo {
	text = RemoveLineNumbers(text)
	text = whitespacePattern.ReplaceAllString(text, " ")
	text = StripAllWhitespace(text)
	if idx := strings.Index(text, "Currency"); idx >= 0 {
		text = text[:idx]
	}

	info := InvoiceInfo{
		PONumber:   matchGroup(poNumberPattern, text),
		POLocation: matchGroup(poLocationPattern, text),
		EntryDate:  matchGroup(entryDatePattern, text),
		CancelDate: matchGroup(cancelDatePattern, text),
	}
	if info.POLocation == "" {
		info.POLocation = matchGroup(storeLocationPattern, text)
	}

	if info.PONumber == "" {
		info.PONumber = notFound
	}
	if info.POLocation == "" {
		info.POLocation = notFound
	}
	if info.EntryDate == "" {
		info.EntryDate = notFound
	}
	if info.CancelDate == "" {
		info.CancelDate = notFound
	}
	return info
}

func matchGroup(pattern *regexp.Regexp, text string) string {
	m := pattern.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return m[1]
}

// ConvertDateFormat mirrors process_coop_invoice's inline
// convert_date_format: "dd/mm/yy" -> "dd/mm/yyyy".
func ConvertDateFormat(dateStr string) string {
	if dateStr == "" || dateStr == notFound {
		return notFound
	}
	t, err := time.Parse("02/01/06", dateStr)
	if err != nil {
		return "Không hợp lệ"
	}
	return t.Format("02/01/2006")
}

// ResolveCancelDate mirrors "if cancle_date == Không tìm thấy: entry_date + 65 days".
func ResolveCancelDate(entryDate, cancelDate string) (string, error) {
	if cancelDate != notFound {
		return cancelDate, nil
	}
	if entryDate == notFound {
		return notFound, nil
	}
	t, err := time.Parse("02/01/2006", entryDate)
	if err != nil {
		return "", fmt.Errorf("coop: entry date không hợp lệ: %q", entryDate)
	}
	return t.AddDate(0, 0, 65).Format("02/01/2006"), nil
}

var (
	notesPattern = regexp.MustCompile(`(?is)` + spacedPattern("Notes") + `\s*-\s*(.*?)\s*FOB`)
	notesCleanupPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)` + spacedPattern("Xin vui long kem DDH khi giao hang")),
		regexp.MustCompile(`(?i)\*\s*=\s*` + spacedPattern("This SKU Discounted")),
		regexp.MustCompile(`(?i)` + spacedPattern("Mot Hoa Don chi xuat cho mot PO")),
		regexp.MustCompile(`(?i)` + spacedPattern("mua1TANG1CUNGLOAI")),
		regexp.MustCompile(`(?i)` + spacedPattern("1TANG1CUNGLOAI")),
	}
)

// ExtractNotes mirrors the "Notes-...FOB" extraction + boilerplate
// stripping + word-dedup block inside process_coop_invoice.
func ExtractNotes(text string) string {
	notes := ""
	if m := notesPattern.FindStringSubmatch(text); m != nil {
		notes = strings.TrimSpace(m[1])
	}
	for _, p := range notesCleanupPatterns {
		notes = p.ReplaceAllString(notes, "")
	}
	notes = strings.TrimSpace(whitespacePattern.ReplaceAllString(notes, " "))

	words := strings.Fields(notes)
	seen := make(map[string]bool, len(words))
	var deduped []string
	for _, w := range words {
		if !seen[w] {
			seen[w] = true
			deduped = append(deduped, w)
		}
	}
	return strings.Join(deduped, " ")
}

var (
	shipToStatusPattern = regexp.MustCompile(`(?is)` + spacedPattern("Ship To") + `:` + spacedPattern("Status") + `-\s*\d+\s*` + spacedPattern("RELEASED") + `\s*(.*?)\s*` + spacedPattern("Contact") + `-`)
	shipToStorePattern  = regexp.MustCompile(`(?is)` + spacedPattern("Store") + `-\s*(.*?)\s*` + spacedPattern("Vendor"))
)

// ExtractShipTo mirrors process_coop_invoice's Ship To extraction: try
// "Ship To: Status-... RELEASED ... Contact-" first, then "Store- ...
// Vendor", then "" if neither matches.
func ExtractShipTo(text string) string {
	normalized := whitespacePattern.ReplaceAllString(text, " ")
	if m := shipToStatusPattern.FindStringSubmatch(normalized); m != nil {
		return strings.TrimSpace(m[1])
	}
	if m := shipToStorePattern.FindStringSubmatch(normalized); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd GO && go test ./internal/processing/coop/... -run "TestParseInvoiceInfo|TestConvertDateFormat|TestResolveCancelDate|TestExtractNotes|TestExtractShipTo" -v`
Expected: PASS (the notes test logs rather than hard-fails, per its note above).

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/coop/invoice.go GO/internal/processing/coop/invoice_test.go
git commit -m "feat(go): add Coop invoice info parsing (PO, dates, notes, ship-to)"
```

---

### Task 7: `internal/processing/coop` — promo text helpers

**Files:**
- Create: `GO/internal/processing/coop/promo.go`
- Test: `GO/internal/processing/coop/promo_test.go`

**Interfaces:**
- Produces: `coop.SplitPromoText(text, system string) string`, `coop.ExtractDiscount(value string) float64`, `coop.ExtractBraceContent(value string) string`, `coop.ExtractMoneyAmount(text string) (int, bool)`, `coop.LastFourDigits(text string) string`, `coop.FormatWeightKg(kg float64) string`.

- [ ] **Step 1: Write the failing test**

```go
package coop

import "testing"

func TestSplitPromoText(t *testing.T) {
	text := "cm Mua 2 tặng 1 (cf Mua 3 tặng 1)"
	if got := SplitPromoText(text, "COOPMART"); got != "cm Mua 2 tặng 1" {
		t.Fatalf("SplitPromoText(COOPMART) = %q, want %q", got, "cm Mua 2 tặng 1")
	}
	if got := SplitPromoText(text, "COOPFOOD"); got != "cf Mua 3 tặng 1" {
		t.Fatalf("SplitPromoText(COOPFOOD) = %q, want %q", got, "cf Mua 3 tặng 1")
	}
	if got := SplitPromoText(text, "OTHER"); got != text {
		t.Fatalf("SplitPromoText(OTHER) = %q, want unchanged", got)
	}
}

func TestSplitPromoText_NoCfMeansCmTakesWholeText(t *testing.T) {
	text := "cm Giảm 10% toàn bộ đơn hàng"
	if got := SplitPromoText(text, "COOPMART"); got != text {
		t.Fatalf("SplitPromoText(no cf) = %q, want unchanged %q", got, text)
	}
}

func TestExtractDiscount(t *testing.T) {
	if got := ExtractDiscount("Giảm 15% cho mã này"); got != 15 {
		t.Fatalf("ExtractDiscount = %v, want 15", got)
	}
	if got := ExtractDiscount("Giảm 12.5%"); got != 12.5 {
		t.Fatalf("ExtractDiscount = %v, want 12.5", got)
	}
	if got := ExtractDiscount("Không có giảm giá"); got != 0 {
		t.Fatalf("ExtractDiscount(none) = %v, want 0", got)
	}
}

func TestExtractBraceContent(t *testing.T) {
	if got := ExtractBraceContent("Mua 2 tặng 1 {KM Bó Kèm}"); got != "KM Bó Kèm" {
		t.Fatalf("ExtractBraceContent = %q, want %q", got, "KM Bó Kèm")
	}
	if got := ExtractBraceContent("không có ngoặc"); got != "" {
		t.Fatalf("ExtractBraceContent(none) = %q, want empty", got)
	}
}

func TestExtractMoneyAmount(t *testing.T) {
	cases := []struct {
		text string
		want int
		ok   bool
	}{
		{"Tặng quà khi mua trên 199k", 199000, true},
		{"Tặng quà khi mua trên 199 K", 199000, true},
		{"Tặng quà khi mua trên 150000 đồng", 150000, true},
		{"không có số tiền hợp lệ", 0, false},
	}
	for _, c := range cases {
		got, ok := ExtractMoneyAmount(c.text)
		if ok != c.ok || got != c.want {
			t.Fatalf("ExtractMoneyAmount(%q) = (%d, %v), want (%d, %v)", c.text, got, ok, c.want, c.ok)
		}
	}
}

func TestLastFourDigits(t *testing.T) {
	if got := LastFourDigits("SP0001234_extra"); got != "1234" {
		t.Fatalf("LastFourDigits = %q, want %q", got, "1234")
	}
	if got := LastFourDigits("ab"); got != "ab" {
		t.Fatalf("LastFourDigits(short) = %q, want %q", got, "ab")
	}
}

func TestFormatWeightKg(t *testing.T) {
	if got := FormatWeightKg(500); got != "500 kg" {
		t.Fatalf("FormatWeightKg(500) = %q, want %q", got, "500 kg")
	}
	if got := FormatWeightKg(1500); got != "1.5 tấn" {
		t.Fatalf("FormatWeightKg(1500) = %q, want %q", got, "1.5 tấn")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/coop/... -run "TestSplitPromoText|TestExtractDiscount|TestExtractBraceContent|TestExtractMoneyAmount|TestLastFourDigits|TestFormatWeightKg" -v`
Expected: FAIL — functions don't exist yet.

- [ ] **Step 3: Write minimal implementation**

```go
package coop

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	cfParenPattern  = regexp.MustCompile(`(?i)\(([^)]*cf[^)]*)\)`)
	cfLinePattern   = regexp.MustCompile(`(?i)cf[^\n]*`)
	cmLinePattern   = regexp.MustCompile(`(?i)cm[^\n]*`)
	cmFromPattern   = regexp.MustCompile(`(?is)cm.*`)
	discountPattern = regexp.MustCompile(`(\d+(?:\.\d+)?)%`)
	braceContentPattern = regexp.MustCompile(`(?s)\{(.*?)\}`)
	moneyKPattern    = regexp.MustCompile(`(?i)\b(\d{1,3})\s*k\b`)
	moneyFullPattern = regexp.MustCompile(`\b\d{5,6}\b`)
)

// SplitPromoText mirrors tachkhuyenmai_coop: a promo-text cell can
// bundle both a Coopmart ("cm...") and Coopfood ("cf...") variant in
// one string; returns whichever matches `system`, or text unchanged
// for any other system.
func SplitPromoText(text, system string) string {
	text = strings.TrimSpace(text)
	system = strings.TrimSpace(system)

	cfResult := ""
	if m := cfParenPattern.FindStringSubmatch(text); m != nil {
		cfResult = strings.TrimSpace(m[1])
	} else if m := cfLinePattern.FindString(text); m != "" {
		cfResult = strings.TrimSpace(m)
	}

	cmResult := ""
	if cfResult != "" {
		cfStart := strings.Index(strings.ToLower(text), strings.ToLower(cfResult))
		if cfStart >= 0 {
			cmCandidate := text[:cfStart]
			if m := cmFromPattern.FindString(cmCandidate); m != "" {
				cmResult = strings.TrimSpace(m)
			}
		}
	} else if m := cmLinePattern.FindString(text); m != "" {
		cmResult = strings.TrimSpace(m)
	}

	switch strings.ToUpper(system) {
	case "COOPMART":
		if cmResult != "" {
			return cmResult
		}
	case "COOPFOOD":
		if cfResult != "" {
			return cfResult
		}
	default:
		return text
	}
	return text
}

// ExtractDiscount mirrors extract_discount.
func ExtractDiscount(value string) float64 {
	m := discountPattern.FindStringSubmatch(value)
	if m == nil {
		return 0
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	return v
}

// ExtractBraceContent mirrors laycachbo_khuyenmai.
func ExtractBraceContent(value string) string {
	m := braceContentPattern.FindStringSubmatch(value)
	if m == nil {
		return ""
	}
	return m[1]
}

// ExtractMoneyAmount mirrors tachtien_khuyenmai: "199k"/"199 K" -> 199000,
// or a bare 5-6 digit number as itself.
func ExtractMoneyAmount(text string) (int, bool) {
	if m := moneyKPattern.FindStringSubmatch(text); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n * 1000, true
		}
	}
	if m := moneyFullPattern.FindString(text); m != "" {
		if n, err := strconv.Atoi(m); err == nil {
			return n, true
		}
	}
	return 0, false
}

// LastFourDigits mirrors layduoi_mahang: text before the first "_",
// last 4 runes of that (or the whole thing if shorter).
func LastFourDigits(text string) string {
	base := strings.SplitN(text, "_", 2)[0]
	runes := []rune(base)
	if len(runes) <= 4 {
		return base
	}
	return string(runes[len(runes)-4:])
}

// FormatWeightKg mirrors format_weight_kg: < 1000kg shows kg,
// >= 1000kg converts to tấn, both rounded to 2 decimals.
func FormatWeightKg(kg float64) string {
	if kg >= 1000 {
		return fmt.Sprintf("%s tấn", trimFloat(kg/1000, 2))
	}
	return fmt.Sprintf("%s kg", trimFloat(kg, 2))
}

func trimFloat(v float64, decimals int) string {
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd GO && go test ./internal/processing/coop/... -run "TestSplitPromoText|TestExtractDiscount|TestExtractBraceContent|TestExtractMoneyAmount|TestLastFourDigits|TestFormatWeightKg" -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/coop/promo.go GO/internal/processing/coop/promo_test.go
git commit -m "feat(go): add Coop promo text helpers"
```

---

### Task 8: `internal/processing/coop` — `ExtractProducts`

**Files:**
- Create: `GO/internal/processing/coop/extract.go`
- Test: `GO/internal/processing/coop/extract_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks (pure text in, structs out).
- Produces: `coop.Product{Barcode string, Qty, Cost float64}`, `coop.ExtractProducts(text string) ([]Product, error)`.

**Design note:** `xulydonhang.py`'s `extract_products` uses a regex with negative lookbehind/lookahead (`(?<![a-zA-Z])\d[\d,]*\.\d+(?![a-zA-Z])`) that Go's RE2 engine cannot express directly — replicated below via `regexp.FindAllStringIndex` plus manual boundary checks on the byte immediately before/after each match (safe to do byte-wise here since ASCII letters can never be a continuation byte of a multi-byte UTF-8 sequence). Also: the Python original appends a product to its result list whenever a SKU barcode was found, *even if* quantity/cost extraction failed for that block — which means the Python code goes on to crash later (`AttributeError` on `None.is_integer()`) when `write_to_dondathang` uses that product. This port treats an unresolved quantity/cost as an error for the whole page instead, which is the closer behavioral match (a crash also aborts processing that PO) and matches the spec's "no silent bad data" principle.

- [ ] **Step 1: Write the failing test**

```go
package coop

import "testing"

func TestExtractProducts_SingleProductBlock(t *testing.T) {
	text := "3564270-4  Chai tay toilet CHUNGBLUE180g   EA   C24   809424.00   809424.00   1.00   24.00   .00   809,424.00\nSub Total"
	products, err := ExtractProducts(text)
	if err != nil {
		t.Fatalf("ExtractProducts returned error: %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("ExtractProducts = %d products, want 1: %+v", len(products), products)
	}
	if products[0].Barcode != "3564270-4" {
		t.Fatalf("Barcode = %q, want %q", products[0].Barcode, "3564270-4")
	}
	if products[0].Qty != 24 {
		t.Fatalf("Qty = %v, want 24", products[0].Qty)
	}
	if products[0].Cost != 809424 {
		t.Fatalf("Cost = %v, want 809424", products[0].Cost)
	}
}

func TestExtractProducts_TwoProductBlocks(t *testing.T) {
	text := "3564270-4  Chai tay toilet   1.00   24.00   809,424.00\n" +
		"3564271-9  Chai tay khac    1.00   12.00   400,000.00\nSub Total"
	products, err := ExtractProducts(text)
	if err != nil {
		t.Fatalf("ExtractProducts returned error: %v", err)
	}
	if len(products) != 2 {
		t.Fatalf("ExtractProducts = %d products, want 2: %+v", len(products), products)
	}
	if products[0].Barcode != "3564270-4" || products[1].Barcode != "3564271-9" {
		t.Fatalf("barcodes = %q, %q", products[0].Barcode, products[1].Barcode)
	}
}

func TestExtractProducts_NoSkuAnchorsReturnsEmpty(t *testing.T) {
	products, err := ExtractProducts("no product lines here\nSub Total")
	if err != nil {
		t.Fatalf("ExtractProducts returned error: %v", err)
	}
	if len(products) != 0 {
		t.Fatalf("ExtractProducts = %v, want empty", products)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/coop/... -run TestExtractProducts -v`
Expected: FAIL — `ExtractProducts` not defined.

- [ ] **Step 3: Write minimal implementation**

```go
package coop

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Product is one extracted line item, mirroring extract_products'
// {"Barcode", "Qty Ord/Pcs", "Extended Cost"} dict.
type Product struct {
	Barcode string
	Qty     float64
	Cost    float64
}

var (
	subTotalSplitPattern = regexp.MustCompile(`(?i)` + spacedPattern("SubTotal"))
	vndSplitPattern       = regexp.MustCompile(`(?i)` + spacedPattern("VND Viet Nam Dong"))
	skuLinePattern         = regexp.MustCompile(`\d{7}-\s*\d`)
	decimalNumberPattern    = regexp.MustCompile(`\d[\d,]*\.\d+`)
)

const (
	minSanePrice = 1000.0
	maxSanePrice = 2000000.0
)

// ExtractProducts mirrors xulydonhang.py's extract_products: an
// empirically-tuned heuristic that finds Coop SKU anchors (7 digits,
// dash, 1 digit) and guesses which numbers in the text block between
// two anchors are the quantity and the extended cost, based on block
// position (last block vs. normal) and how many comma-formatted
// ("large") numbers are present. This is not a clean grammar — it
// reproduces the original's exact branch structure on purpose. See
// this task's design note for the one behavioral difference (errors
// instead of silently producing a product with a missing qty/cost).
func ExtractProducts(text string) ([]Product, error) {
	if loc := subTotalSplitPattern.FindStringIndex(text); loc != nil {
		text = text[:loc[0]]
	}
	if locs := vndSplitPattern.FindAllStringIndex(text, -1); len(locs) > 0 {
		last := locs[len(locs)-1]
		text = text[last[1]:]
	}

	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	var skuIndices []int
	for i, line := range lines {
		if skuLinePattern.MatchString(line) {
			skuIndices = append(skuIndices, i)
		}
	}

	var products []Product
	for i, start := range skuIndices {
		end := len(lines)
		if i+1 < len(skuIndices) {
			end = skuIndices[i+1]
		}
		block := make([]string, end-start)
		for j, line := range lines[start:end] {
			line = strings.ReplaceAll(line, ", ", ",")
			line = strings.ReplaceAll(line, ". ", ".")
			block[j] = line
		}

		barcode := ""
		if m := skuLinePattern.FindString(block[0]); m != "" {
			barcode = strings.ReplaceAll(m, " ", "")
		}
		if barcode == "" {
			continue
		}

		joined := strings.Join(block, " ")
		nums := findDecimalNumbers(joined)
		var large []string
		for _, n := range nums {
			if strings.Contains(n, ",") {
				large = append(large, n)
			}
		}

		var qtyStr, costStr string
		var ok bool
		if i == len(skuIndices)-1 {
			qtyStr, costStr, ok = selectLastBlockQtyCost(nums, large)
		} else {
			qtyStr, costStr, ok = selectNormalBlockQtyCost(nums, large)
		}
		if !ok {
			return nil, fmt.Errorf("không xác định được số lượng/đơn giá cho mã hàng %s", barcode)
		}

		qty, err := strconv.ParseFloat(strings.ReplaceAll(qtyStr, ",", ""), 64)
		if err != nil {
			return nil, fmt.Errorf("số lượng không hợp lệ cho mã hàng %s: %q", barcode, qtyStr)
		}
		cost, err := strconv.ParseFloat(strings.ReplaceAll(costStr, ",", ""), 64)
		if err != nil {
			return nil, fmt.Errorf("đơn giá không hợp lệ cho mã hàng %s: %q", barcode, costStr)
		}

		products = append(products, Product{Barcode: barcode, Qty: qty, Cost: cost})
	}

	return products, nil
}

// findDecimalNumbers mirrors the regex
// `(?<![a-zA-Z])\d[\d,]*\.\d+(?![a-zA-Z])` — RE2 has no lookaround, so
// the boundary check is manual: keep a match only if the byte
// immediately before/after it (if any) is not an ASCII letter.
func findDecimalNumbers(text string) []string {
	indices := decimalNumberPattern.FindAllStringIndex(text, -1)
	var out []string
	for _, idx := range indices {
		start, end := idx[0], idx[1]
		if start > 0 && isASCIILetterByte(text[start-1]) {
			continue
		}
		if end < len(text) && isASCIILetterByte(text[end]) {
			continue
		}
		out = append(out, text[start:end])
	}
	return out
}

func isASCIILetterByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func selectLastBlockQtyCost(nums, large []string) (qty, cost string, ok bool) {
	if len(large) > 0 {
		type candidate struct {
			str string
			val float64
		}
		candidates := make([]candidate, len(large))
		for i, s := range large {
			v, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
			candidates[i] = candidate{s, v}
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].val < candidates[j].val })

		for _, c := range candidates {
			costIdx := indexOf(nums, c.str)
			if costIdx < 0 {
				continue
			}
			if costIdx > 0 {
				if up, ok2 := impliedUnitPrice(c.str, nums[costIdx-1]); ok2 && isSanePrice(up) {
					return nums[costIdx-1], c.str, true
				}
			}
			if costIdx > 1 {
				if up, ok2 := impliedUnitPrice(c.str, nums[costIdx-2]); ok2 && isSanePrice(up) {
					return nums[costIdx-2], c.str, true
				}
			}
		}
	}
	if len(nums) >= 2 {
		return nums[len(nums)-2], nums[len(nums)-1], true
	}
	return "", "", false
}

func selectNormalBlockQtyCost(nums, large []string) (qty, cost string, ok bool) {
	if len(large) >= 2 {
		costStr := nums[len(nums)-1]
		idx0 := indexOf(nums, large[0])
		if idx0 > 0 {
			return nums[idx0-1], costStr, true
		}
		return "", "", false
	}
	if len(nums) >= 2 {
		return nums[len(nums)-2], nums[len(nums)-1], true
	}
	return "", "", false
}

func indexOf(items []string, target string) int {
	for i, v := range items {
		if v == target {
			return i
		}
	}
	return -1
}

func impliedUnitPrice(costStr, qtyStr string) (float64, bool) {
	cost, err1 := strconv.ParseFloat(strings.ReplaceAll(costStr, ",", ""), 64)
	qty, err2 := strconv.ParseFloat(strings.ReplaceAll(qtyStr, ",", ""), 64)
	if err1 != nil || err2 != nil || qty == 0 {
		return 0, false
	}
	return cost / qty, true
}

func isSanePrice(unitPrice float64) bool {
	return unitPrice > minSanePrice && unitPrice < maxSanePrice
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd GO && go test ./internal/processing/coop/... -run TestExtractProducts -v`
Expected: PASS, all 3 tests.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/coop/extract.go GO/internal/processing/coop/extract_test.go
git commit -m "feat(go): add Coop product extraction (SKU block heuristics)"
```

---

### Task 9: `internal/processing/excelwriter` — column-exact Excel writing

**Files:**
- Create: `GO/internal/processing/excelwriter/dondathang.go`
- Test: `GO/internal/processing/excelwriter/dondathang_test.go`

**Interfaces:**
- Produces: `excelwriter.Row{...}` (see fields below), `excelwriter.WriteOrderRows(path string, rows []Row, headerDescription string) error`.

- [ ] **Step 1: Create a minimal test workbook fixture**

Create `GO/internal/processing/excelwriter/testdata/dondathang.xlsx` with one sheet named `Don dat hang`, header text in row 8 (content doesn't matter for this test, any placeholder text is fine — `WriteOrderRows` starts appending at `existing_row_count + 1`, so an 8-row file means writing starts at row 9, matching the real workbook's layout).

- [ ] **Step 2: Write the failing test**

```go
package excelwriter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func copyTestWorkbook(t *testing.T) string {
	t.Helper()
	src := "testdata/dondathang.xlsx"
	dst := filepath.Join(t.TempDir(), "dondathang.xlsx")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed reading test fixture: %v", err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("failed writing temp workbook: %v", err)
	}
	return dst
}

func TestWriteOrderRows_WritesColumnsAndFormula(t *testing.T) {
	path := copyTestWorkbook(t)

	rows := []Row{
		{EntryDate: "23/07/2026", OrderNumber: "ĐĐHCOOP-102945235-00", Status: "Chưa thực hiện", IsNoteRow: true, ProductName: "COOPMART PO102945235-00"},
		{SKU: "3564270-4", Qty: 24, UnitPrice: 33726, ProductName: "Chai tay toilet", UseZFormula: true},
	}

	if err := WriteOrderRows(path, rows, "COOPMART PO102945235-00 (Tổng trọng lượng: 4.32 kg)"); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()

	sku, _ := f.GetCellValue("Don dat hang", "Q10")
	if sku != "3564270-4" {
		t.Fatalf("Q10 = %q, want %q", sku, "3564270-4")
	}
	formula, _ := f.GetCellFormula("Don dat hang", "Z10")
	if formula != "Y10*X10" {
		t.Fatalf("Z10 formula = %q, want %q", formula, "Y10*X10")
	}
	desc, _ := f.GetCellValue("Don dat hang", "L9")
	if desc != "COOPMART PO102945235-00 (Tổng trọng lượng: 4.32 kg)" {
		t.Fatalf("L9 (header description) = %q, want the total-weight description", desc)
	}
}

func TestWriteOrderRows_PriceMismatchGetsRedFillAndComment(t *testing.T) {
	path := copyTestWorkbook(t)

	rows := []Row{
		{SKU: "3564270-4", Qty: 24, UnitPrice: 33000, InvoicePrice: 33726, PriceMismatch: true, UseZFormula: true},
	}
	if err := WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()

	styleID, err := f.GetCellStyle("Don dat hang", "Y9")
	if err != nil {
		t.Fatalf("GetCellStyle returned error: %v", err)
	}
	if styleID == 0 {
		t.Fatal("Y9 has default style, want the red-fill mismatch style applied")
	}

	comment, err := f.GetComments("Don dat hang")
	if err != nil {
		t.Fatalf("GetComments returned error: %v", err)
	}
	if len(comment) != 1 {
		t.Fatalf("comments = %d, want 1: %+v", len(comment), comment)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/excelwriter/... -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 4: Write minimal implementation**

```go
package excelwriter

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

const sheetName = "Don dat hang"

// Row is one row to write into the "Don dat hang" sheet, matching the
// column layout of xulydonhang.py's write_to_dondathang (see the
// spec's column table). UseZFormula controls whether Z (Thành tiền)
// gets the formula "=Y{row}*X{row}" (main product rows) or the literal
// 0 (header row and promo bonus rows) — both are real, distinct
// behaviors in the Python original.
type Row struct {
	EntryDate      string
	DebtDays       int
	OrderNumber    string
	Status         string
	CancelDate     string
	ShipTo         string
	CustomerCode   string
	Description    string
	SKU            string
	Warehouse      string
	VATPercent     int
	RegionCode     string
	StatCode       string
	IsPromoItem    bool
	IsNoteRow      bool
	Qty            float64
	UnitPrice      float64
	ProductName    string
	CaseCount      int
	LineWeightKg   float64
	PromoNote      string
	PromoBundleSku string
	PromoContent   string
	PriceMismatch  bool
	InvoicePrice   float64
	UseZFormula    bool
}

// WriteOrderRows appends rows to the "Don dat hang" sheet, mirroring
// write_to_dondathang's column layout and price-mismatch formatting.
// headerDescription, if non-empty, overwrites the Description (L) cell
// of the first row written — mirroring write_to_dondathang's final
// `sheet[f"L{start_row}"] = ...` step, which only happens once the
// order's total weight is known.
func WriteOrderRows(path string, rows []Row, headerDescription string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("excelwriter: open %s: %w", path, err)
	}
	defer f.Close()

	existingRows, err := f.GetRows(sheetName)
	if err != nil {
		return fmt.Errorf("excelwriter: read %s: %w", sheetName, err)
	}
	currentRow := len(existingRows) + 1
	firstRow := currentRow

	redFill, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"FF0000"}, Pattern: 1},
	})
	if err != nil {
		return fmt.Errorf("excelwriter: create red fill style: %w", err)
	}

	for _, row := range rows {
		if err := writeRow(f, currentRow, row, redFill); err != nil {
			return err
		}
		currentRow++
	}

	if headerDescription != "" {
		if err := f.SetCellValue(sheetName, fmt.Sprintf("L%d", firstRow), headerDescription); err != nil {
			return fmt.Errorf("excelwriter: set header description: %w", err)
		}
	}

	if err := f.Save(); err != nil {
		return fmt.Errorf("excelwriter: save %s: %w", path, err)
	}
	return nil
}

func writeRow(f *excelize.File, rowNum int, row Row, redFillStyle int) error {
	set := func(col string, value interface{}) error {
		return f.SetCellValue(sheetName, fmt.Sprintf("%s%d", col, rowNum), value)
	}
	yesNo := func(b bool) string {
		if b {
			return "Có"
		}
		return "Không"
	}

	writes := []struct {
		col   string
		value interface{}
	}{
		{"A", row.EntryDate},
		{"AV", row.DebtDays},
		{"B", row.OrderNumber},
		{"C", row.Status},
		{"D", row.CancelDate},
		{"E", row.ShipTo},
		{"G", row.CustomerCode},
		{"L", row.Description},
		{"Q", row.SKU},
		{"V", row.Warehouse},
		{"AE", row.VATPercent},
		{"AJ", row.RegionCode},
		{"AM", row.StatCode},
		{"U", yesNo(row.IsPromoItem)},
		{"T", yesNo(row.IsNoteRow)},
		{"X", row.Qty},
		{"S", row.ProductName},
		{"AU", row.CaseCount},
		{"AT", row.LineWeightKg},
		{"AO", row.PromoNote},
		{"AP", row.PromoBundleSku},
		{"AQ", row.PromoContent},
	}
	for _, w := range writes {
		if err := set(w.col, w.value); err != nil {
			return fmt.Errorf("excelwriter: set %s%d: %w", w.col, rowNum, err)
		}
	}

	if row.UseZFormula {
		if err := f.SetCellFormula(sheetName, fmt.Sprintf("Z%d", rowNum), fmt.Sprintf("Y%d*X%d", rowNum, rowNum)); err != nil {
			return fmt.Errorf("excelwriter: set Z%d formula: %w", rowNum, err)
		}
	} else if err := set("Z", 0); err != nil {
		return err
	}

	if err := set("Y", row.UnitPrice); err != nil {
		return err
	}
	if row.PriceMismatch {
		cell := fmt.Sprintf("Y%d", rowNum)
		if err := f.SetCellStyle(sheetName, cell, cell, redFillStyle); err != nil {
			return fmt.Errorf("excelwriter: apply red fill to %s: %w", cell, err)
		}
		diff := row.InvoicePrice - row.UnitPrice
		text := fmt.Sprintf("Kiểm tra lại giá mã này! - Giá hóa đơn: %v - Chênh lệch: %v", row.InvoicePrice, diff)
		if err := f.AddComment(sheetName, excelize.Comment{Cell: cell, Author: "System", Text: text}); err != nil {
			return fmt.Errorf("excelwriter: add comment to %s: %w", cell, err)
		}
	}

	return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd GO && go test ./internal/processing/excelwriter/... -v`
Expected: PASS, both tests.

- [ ] **Step 6: Commit**

```bash
git add GO/internal/processing/excelwriter/
git commit -m "feat(go): add Coop Excel row writer with price-mismatch formatting"
```

---

### Task 10: PDF text extraction + `internal/processing` — `RealProcessor` orchestration

**Files:**
- Create: `GO/internal/processing/pdfextract.go`
- Create: `GO/internal/processing/coop_processor.go`
- Modify: `GO/internal/processing/types.go` (add `StatusKind` field if not already present from Phase 1's final review — confirm first, see Step 0)
- Test: `GO/internal/processing/coop_processor_test.go`

**Interfaces:**
- Consumes: `vendor.Identify`, `coop.CountPOsOnPage`/`SplitMultiPO`/`ParseInvoiceInfo`/`ExtractNotes`/`ExtractShipTo`/`ConvertDateFormat`/`ResolveCancelDate`/`ExtractProducts`/`SplitPromoText`/`ExtractDiscount`/`ExtractBraceContent`/`ExtractMoneyAmount`/`LastFourDigits`/`FormatWeightKg`, `productdata.Store` (all methods), `pricing.Index`/`Promotion` (`FindPrice`/`FindPromotions`/`FindInvoicePromotion`), `excelwriter.Row`/`WriteOrderRows`.
- Produces: `processing.PricingSource` interface (`FetchCoopIndex() (*pricing.Index, error)`), `processing.RealProcessor{Store *productdata.Store, Pricing PricingSource, ExcelPath string}` implementing the (already-revised, see Task 11) `Processor` interface with `Process(ctx, filePath string, stt int) ([]OrderRow, error)`.

**Step 0 — confirm `OrderRow.StatusKind` exists:** Phase 1's final whole-branch review added `StatusKind string` (`"done"`/`"warning"`/`"failed"`) and `StatusKindDone`/`StatusKindWarning`/`StatusKindFailed` constants to `processing.OrderRow` in `GO/internal/processing/types.go`, to replace fragile emoji-substring status matching on the frontend. Read that file first — if these fields/constants are present (they should be, per the Phase 1 plan's final commit), use them as-is; do not redefine.

- [ ] **Step 1: Verify the PDF library API and write the failing test for page extraction**

```go
package processing

import "testing"

func TestExtractPageTexts_ReturnsOnePerPage(t *testing.T) {
	// Use any real single-page Coop PDF from the repo's đơn hàng/08-2026/
	// folder as a smoke-test fixture — copy one into testdata/ (see
	// Step 2) rather than depending on the live folder, so this test
	// doesn't break if that folder's contents change.
	pages, err := extractPageTexts("testdata/sample_coop_order.pdf")
	if err != nil {
		t.Fatalf("extractPageTexts returned error: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("extractPageTexts returned no pages")
	}
	if !containsSubstring(pages[0], "POM343") && !containsSubstring(pages[0], "P/O Number") {
		t.Fatalf("page 0 text doesn't look like a Coop PO, got: %.200s", pages[0])
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
```

- [ ] **Step 2: Copy a real sample PDF into testdata**

```bash
mkdir -p "GO/internal/processing/testdata"
cp "đơn hàng/08-2026/102945235-00.pdf" "GO/internal/processing/testdata/sample_coop_order.pdf"
```

(Any of the 155 real Coop PDFs works — `102945235-00.pdf` was the one used to verify the `ledongthuc/pdf` library during this plan's design and is known-good.)

- [ ] **Step 3: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/... -run TestExtractPageTexts -v`
Expected: FAIL — `extractPageTexts` not defined.

- [ ] **Step 4: Write `pdfextract.go`**

```go
package processing

import "fmt"

func extractPageTexts(path string) ([]string, error) {
	file, r, err := pdfOpen(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	numPages := r.NumPage()
	pages := make([]string, 0, numPages)
	for i := 1; i <= numPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			return nil, fmt.Errorf("trang %d: %w", i, err)
		}
		pages = append(pages, text)
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("không đọc được nội dung trang nào")
	}
	return pages, nil
}
```

- [ ] **Step 5: Add the `pdfOpen` wrapper (isolates the third-party import to one line, per this codebase's existing pattern of small focused files)**

```go
package processing

import "github.com/ledongthuc/pdf"

func pdfOpen(path string) (*os.File, *pdf.Reader, error) {
	return pdf.Open(path)
}
```

Note: this needs `import "os"` too — add it. Verified against the actual installed module (`github.com/ledongthuc/pdf@v0.0.0-20250511090121-5959a4027728`): `func Open(file string) (*os.File, *Reader, error)`, `func (r *Reader) NumPage() int`, `func (r *Reader) Page(num int) Page` where `Page.V` is a `Value` with `IsNull() bool`, and `func (p Page) GetPlainText(fonts map[string]*Font) (result string, err error)`. If `go get`/`go mod tidy` pulls a newer version with a different API, re-check these signatures with `go doc github.com/ledongthuc/pdf` before adjusting.

- [ ] **Step 6: Add the dependency and run test to verify it passes**

```bash
cd GO && go get github.com/ledongthuc/pdf@v0.0.0-20250511090121-5959a4027728
go test ./internal/processing/... -run TestExtractPageTexts -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add GO/internal/processing/pdfextract.go GO/internal/processing/testdata/ GO/go.mod GO/go.sum
git commit -m "feat(go): add per-page PDF text extraction"
```

- [ ] **Step 8: Write the failing test for `RealProcessor`**

```go
package processing

import (
	"context"
	"testing"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

type fixturePricingSource struct {
	index *pricing.Index
}

func (f *fixturePricingSource) FetchCoopIndex() (*pricing.Index, error) {
	return f.index, nil
}

func TestRealProcessor_ProcessesRealSampleCoopFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá"},
		{"1", "1234567", "Nước giặt", "141.272"},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_coop_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Process returned %d rows, want 1: %+v", len(rows), rows)
	}
	if rows[0].System != "COOPMART" && rows[0].System != "COOPFOOD" {
		t.Fatalf("System = %q, want COOPMART or COOPFOOD", rows[0].System)
	}
	if rows[0].PO == "" {
		t.Fatal("PO is empty, want a parsed PO number")
	}
}

func TestRealProcessor_NonCoopFileProducesFailedRow(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	rp := &RealProcessor{Store: store, Pricing: &fixturePricingSource{index: pricing.ParseIndex(nil)}, ExcelPath: copyTestWorkbookForProcessor(t)}

	// Any file whose text doesn't match Coop's vendor markers — a
	// second copy of the same PDF works for this table-stakes check
	// too, since a text-substitution fixture is simpler to construct
	// than a whole different-vendor PDF; the point under test is the
	// "vendor not recognized" branch of Process, not real BigC parsing.
	rows, err := rp.Process(context.Background(), "testdata/not_a_coop_file.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error (should return a Failed row, not an error): %v", err)
	}
	if len(rows) != 1 || rows[0].StatusKind != StatusKindFailed {
		t.Fatalf("rows = %+v, want exactly 1 Failed row", rows)
	}
}
```

- [ ] **Step 9: Add the second test fixture and the workbook-copy helper**

```bash
cp "GO/internal/processing/excelwriter/testdata/dondathang.xlsx" "GO/internal/processing/testdata/dondathang.xlsx"
```

For `testdata/not_a_coop_file.pdf`: build a minimal one-page PDF whose extracted text does NOT contain `Vendor - 21569` or `Vendor - 22856` — the simplest way is a throwaway Python one-liner using the existing `.venv`'s `fitz`:

```bash
".venv/Scripts/python.exe" -c "
import fitz
doc = fitz.open()
page = doc.new_page()
page.insert_text((72, 72), 'Purchase Order from BigC - not Coop')
doc.save('GO/internal/processing/testdata/not_a_coop_file.pdf')
"
```

Add the workbook-copy test helper (in `coop_processor_test.go`, alongside the tests above):

```go
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
```

(add `"os"` and `"path/filepath"` to the test file's imports)

- [ ] **Step 10: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/... -run TestRealProcessor -v`
Expected: FAIL — `RealProcessor` not defined.

- [ ] **Step 11: Write `coop_processor.go`**

```go
package processing

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
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
	"order-processor/internal/processing/vendor"
)

const coopDebtDays = 60 // songayno_MT in xulydonhang.py

// PricingSource abstracts fetching Coop's price/promotion data for one
// order, so tests substitute a fixture-backed implementation instead of
// a live Google Sheets fetch. Production wiring uses *pricing.HTTPSource.
type PricingSource interface {
	FetchCoopIndex() (*pricing.Index, error)
}

// RealProcessor implements processing.Processor for the Coop vendor.
// Any page whose text doesn't match Coop's vendor markers produces a
// single Failed OrderRow explaining why, rather than being silently
// skipped — support for other vendors is added in later phases by
// extending this same dispatch.
type RealProcessor struct {
	Store     *productdata.Store
	Pricing   PricingSource
	ExcelPath string
}

func (p *RealProcessor) Process(ctx context.Context, filePath string, stt int) ([]OrderRow, error) {
	pageTexts, err := extractPageTexts(filePath)
	if err != nil {
		return []OrderRow{{
			FileName:   filepath.Base(filePath),
			Status:     StatusFailed + " - không đọc được PDF: " + err.Error(),
			StatusKind: StatusKindFailed,
		}}, nil
	}

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

func splitPageIntoPOs(text string) ([]string, bool) {
	counts := coop.CountPOsOnPage(text)
	if counts.POM343 == 0 || counts.SubTotal == 0 || counts.POM343 != counts.SubTotal {
		return nil, false
	}
	if counts.POM343 == 1 {
		return []string{text}, true
	}
	segments := coop.SplitMultiPO(text)
	if len(segments) == 0 {
		return nil, false
	}
	return segments, true
}

// xPlus1Pattern mirrors the "(\d+)\s*\+\s*1" match inside
// write_to_dondathang's promo-bonus-quantity logic.
var xPlus1Pattern = regexp.MustCompile(`(\d+)\s*\+\s*1`)

func (p *RealProcessor) processSegment(filePath, text, pageLabel string) (OrderRow, error) {
	info := coop.ParseInvoiceInfo(text)
	notes := coop.ExtractNotes(text)
	shipTo := coop.ExtractShipTo(text)

	entryDate := coop.ConvertDateFormat(info.EntryDate)
	cancelDate := coop.ConvertDateFormat(info.CancelDate)
	cancelDate, err := coop.ResolveCancelDate(entryDate, cancelDate)
	if err != nil {
		return OrderRow{}, err
	}

	customerCode := "Không tìm thấy"
	if info.POLocation != "" && info.POLocation != "Không tìm thấy" {
		customerCode = p.Store.GetCustomerCode(info.POLocation)
		if customerCode == "Không tìm thấy" && len(info.POLocation) > 1 {
			half := info.POLocation[:len(info.POLocation)/2]
			customerCode = p.Store.GetCustomerCode(half)
		}
	}

	products, err := coop.ExtractProducts(text)
	if err != nil {
		return OrderRow{}, err
	}
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}
	for i := range products {
		products[i].Barcode = p.Store.ResolveSku(products[i].Barcode)
	}

	system := p.Store.GetSystemForCustomer(customerCode)
	if system == "COOPFOOD" {
		if addr := p.Store.GetCoopfoodAddress(customerCode); addr != "" {
			shipTo = shipTo + " - " + addr
		}
	} else {
		system = "COOPMART"
	}

	priceIndex, err := p.Pricing.FetchCoopIndex()
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := regionInfo(customerCode)
	description := fmt.Sprintf("%s PO%s", system, info.PONumber)
	if notes != "" {
		description += " - " + notes
	}

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber(info.PONumber),
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
		Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: fmt.Sprintf("%s PO%s", system, info.PONumber),
	})

	saigia := 0
	totalWeight := 0.0
	totalValue := 0.0

	for _, product := range products {
		productInfo, _ := p.Store.GetProductInfo(product.Barcode)
		lineWeight := productInfo.WeightKg * product.Qty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(product.Qty / productInfo.PackSize))
		}
		totalWeight += lineWeight

		invoicePrice := product.Cost / product.Qty
		realPriceStr, _ := priceIndex.FindPrice(product.Barcode)
		realPrice, _ := strconv.ParseFloat(strings.ReplaceAll(realPriceStr, ",", ""), 64)

		promos := priceIndex.FindPromotions(product.Barcode, entryDate)
		matchedPromo := ""
		matched := false
		finalPrice := realPrice

		for _, promo := range promos {
			value := coop.SplitPromoText(promo.Value, system)
			if value == "" {
				continue
			}
			candidatePrice := realPrice
			if discount := coop.ExtractDiscount(value); discount != 0 {
				candidatePrice = realPrice - (realPrice * discount / 100)
			}
			if closeEnough(invoicePrice, candidatePrice) {
				finalPrice, matchedPromo, matched = candidatePrice, value, true
				break
			}
		}
		if !matched && closeEnough(invoicePrice, realPrice) {
			finalPrice, matched = realPrice, true
		}

		productRow := excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber(info.PONumber),
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
			Description: description, SKU: product.Barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: product.Qty, UnitPrice: finalPrice,
			ProductName: productInfo.Name, CaseCount: caseCount, LineWeightKg: lineWeight, UseZFormula: true,
		}
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			productRow.PromoContent = matchedPromo
			saigia++
		}
		rows = append(rows, productRow)
		totalValue += finalPrice * product.Qty

		for i, promoPart := range strings.Split(matchedPromo, "|") {
			bonusRow, added := buildPromoBonusRow(p.Store, promoPart, product, i, entryDate, cancelDate, shipTo,
				customerCode, description, warehouse, region, statCode, info.PONumber)
			if !added {
				continue
			}
			totalWeight += bonusRow.LineWeightKg
			rows = append(rows, bonusRow)
		}
	}

	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, entryDate, cancelDate,
			shipTo, customerCode, description, warehouse, region, statCode, info.PONumber); added {
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
		FileName: filepath.Base(filePath), Page: pageLabel, System: system, MaKhachHang: customerCode,
		PO: info.PONumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
	}, nil
}

// orderNumber mirrors write_to_dondathang's order-number field: it
// always uses the literal vendor code "COOP" (the string
// process_coop_invoice hardcodes when calling write_to_dondathang),
// NOT the resolved system (COOPMART/COOPFOOD) — preserve exactly.
func orderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHCOOP-%s", poNumber)
}

// regionInfo mirrors write_to_dondathang's warehouse/region branching:
// customer codes starting with "MB" (Miền Bắc) map to the Hà Nội
// warehouse; everything else defaults to Miền Nam / Long An.
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

func buildPromoBonusRow(store *productdata.Store, promoPart string, product coop.Product, index int,
	entryDate, cancelDate, shipTo, customerCode, description, warehouse, region, statCode, poNumber string,
) (excelwriter.Row, bool) {
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
		return excelwriter.Row{}, false
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

	row := excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber(poNumber),
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
		Description: description, SKU: bonusSku, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsPromoItem: true, Qty: bonusQty, ProductName: bonusInfo.Name,
		CaseCount: bonusCase, LineWeightKg: bonusWeight, UseZFormula: false,
	}
	if index == 0 {
		row.PromoNote = bundleNote
	}
	lower := strings.ToLower(bundleNote)
	if strings.Contains(lower, "bó kèm") || strings.Contains(lower, "quấn kèm") {
		row.PromoBundleSku = fmt.Sprintf("%s_%s_1", coop.LastFourDigits(product.Barcode), coop.LastFourDigits(bonusSku))
	}
	return row, true
}

func buildInvoiceBonusRow(store *productdata.Store, invoicePromo string, totalValue float64,
	entryDate, cancelDate, shipTo, customerCode, description, warehouse, region, statCode, poNumber string,
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
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber(poNumber),
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
		Description: description, SKU: strings.Join(skus, ", "), Warehouse: warehouse, VATPercent: 8,
		RegionCode: region, StatCode: statCode, IsPromoItem: true, Qty: bonusQty, ProductName: bonusInfo.Name,
		CaseCount: bonusCase, LineWeightKg: bonusWeight, PromoNote: bundleNote, PromoContent: invoicePromo,
		UseZFormula: false,
	}, true
}
```

- [ ] **Step 12: Run test to verify it passes**

Run: `cd GO && go test ./internal/processing/... -run TestRealProcessor -v`
Expected: PASS, both tests.

- [ ] **Step 13: Run the full package test suite**

Run: `cd GO && go vet ./... && go test ./... -v`
Expected: everything from Phase 1 plus Tasks 1-10 of this plan passes.

- [ ] **Step 14: Commit**

```bash
git add GO/internal/processing/
git commit -m "feat(go): add RealProcessor orchestrating the full Coop pipeline"
```

---

### Task 11: `internal/processing` — widen `Processor` to `[]OrderRow`; `GO/app.go` wiring

**Files:**
- Modify: `GO/internal/processing/processor.go` (interface + `MockProcessor`)
- Modify: `GO/internal/processing/processor_test.go` (both existing tests)
- Modify: `GO/app.go` (`runBatch`, `processOne`, `NewApp`)
- Modify: `GO/app_test.go` (both existing `TestRunBatch_*` tests)
- Modify: `GO/main.go` (handle `NewApp`'s new error return)

**Interfaces:**
- Consumes: `processing.RealProcessor`, `processing.PricingSource`, `pricing.NewHTTPSource`, `productdata.Load` (Tasks 3, 4, 10).
- Produces: `processing.Processor.Process(ctx, filePath string, stt int) ([]OrderRow, error)` (was `(OrderRow, error)`), `NewApp() (*App, error)` (was `*App`).

- [ ] **Step 1: Update the `Processor` interface and `MockProcessor`**

In `GO/internal/processing/processor.go`, change the interface:

```go
type Processor interface {
	Process(ctx context.Context, filePath string, stt int) ([]OrderRow, error)
}
```

And `MockProcessor.Process` (wrap its existing single-row result):

```go
func (m *MockProcessor) Process(ctx context.Context, filePath string, stt int) ([]OrderRow, error) {
	select {
	case <-time.After(m.Delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	system := mockVendors[m.Rand.Intn(len(mockVendors))]
	status := mockStatuses[m.Rand.Intn(len(mockStatuses))]

	return []OrderRow{{
		FileName:    filepath.Base(filePath),
		Page:        "1",
		System:      system,
		MaKhachHang: fmt.Sprintf("MN_KH%04d", m.Rand.Intn(9999)),
		PO:          fmt.Sprintf("PO%06d", stt),
		DonGia:      fmt.Sprintf("%d", 10000+m.Rand.Intn(90000)),
		Status:      status,
	}}, nil
}
```

- [ ] **Step 2: Update `processor_test.go`'s two existing tests for the new signature**

```go
func TestMockProcessor_ReturnsRowWithKnownVendorAndPO(t *testing.T) {
	p := &MockProcessor{Rand: rand.New(rand.NewSource(1)), Delay: 0}

	rows, err := p.Process(context.Background(), "/tmp/order1.pdf", 108)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Process returned %d rows, want 1", len(rows))
	}
	row := rows[0]
	if row.FileName != "order1.pdf" {
		t.Fatalf("FileName = %q, want %q", row.FileName, "order1.pdf")
	}
	if row.PO != "PO000108" {
		t.Fatalf("PO = %q, want %q", row.PO, "PO000108")
	}

	found := false
	for _, v := range mockVendors {
		if v == row.System {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("System = %q, not in known vendor list", row.System)
	}
}

func TestMockProcessor_ContextCancelledReturnsError(t *testing.T) {
	p := &MockProcessor{Rand: rand.New(rand.NewSource(1)), Delay: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := p.Process(ctx, "/tmp/order1.pdf", 1); err == nil {
		t.Fatal("Process expected error when context is already cancelled, got nil")
	}
}
```

- [ ] **Step 3: Run test to verify it fails (compile error, since `App`/`app_test.go` still assume the old signature)**

Run: `cd GO && go build ./...`
Expected: FAIL to compile — `app.go`'s `runBatch`/`processOne` and `app_test.go`'s `stubProcessor` still implement the old `(OrderRow, error)` signature.

- [ ] **Step 4: Update `GO/app_test.go`'s `stubProcessor` and both `TestRunBatch_*` tests**

```go
type stubProcessor struct {
	failOn string
}

func (s *stubProcessor) Process(ctx context.Context, filePath string, stt int) ([]processing.OrderRow, error) {
	if filePath == s.failOn {
		return nil, errors.New("stub failure")
	}
	return []processing.OrderRow{{FileName: filePath, PO: "PO1", Status: processing.StatusDone}}, nil
}
```

The two `TestRunBatch_*` test bodies (`TestRunBatch_EmitsLogRowPerFileThenDone`, `TestRunBatch_FileErrorEmitsLogAndContinues`) do not need their assertions changed — `runBatch`'s event sequence per file is unchanged (one file still produces one `process:row` per row `stubProcessor` returns, which is still exactly one row per successful file in these tests) — only the `stubProcessor.Process` signature above needs updating for the code to compile.

- [ ] **Step 5: Update `GO/app.go`'s `runBatch`/`processOne` to iterate a row slice**

```go
func (a *App) runBatch(emitter Emitter, files []string, stt int) {
	defer func() {
		if r := recover(); r != nil {
			emitter.Emit("process:log", fmt.Sprintf("❌ Lỗi không mong muốn: %v", r))
		}
		emitter.Emit("process:done", stt)
	}()

	current := stt
	for _, f := range files {
		emitter.Emit("process:log", fmt.Sprintf("Đang xử lý %s...", filepath.Base(f)))
		rows, err := a.processOne(f, current)
		if err != nil {
			emitter.Emit("process:log", fmt.Sprintf("❌ Lỗi xử lý %s: %v", filepath.Base(f), err))
			emitter.Emit("process:row", processing.OrderRow{
				FileName: filepath.Base(f), Status: processing.StatusFailed, StatusKind: processing.StatusKindFailed,
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
```

Note: `emitter.Emit("process:done", stt)` above uses the loop-scoped `current`, not the parameter `stt` — check the existing code in `app.go` from Phase 1's final review fix (which already added the `process:done` payload) and keep using whatever variable that fix used (it should be `current`, since that's the final accumulated STT after the loop — if you see a bug where it's using the pre-loop `stt` instead, that's worth flagging, not silently keeping).

- [ ] **Step 6: Update `TestRunBatch_*` in `app_test.go` if the payload assertions reference specific event counts**

Re-run the existing tests after Step 4/5's changes — they should still pass unchanged, since neither test's file-to-row cardinality changed (`stubProcessor` still returns exactly one row per successful file). If a test fails, read the failure message before changing anything — it likely means Step 5's `runBatch` edit introduced an unintended behavior change, not that the test itself needs updating.

- [ ] **Step 7: Update `NewApp` to return an error and wire `RealProcessor`**

```go
func NewApp() (*App, error) {
	store, err := productdata.Load("data.xlsx")
	if err != nil {
		return nil, fmt.Errorf("app: load data.xlsx: %w", err)
	}

	return &App{
		cfg: config.NewStore(configFileName),
		processor: &processing.RealProcessor{
			Store:     store,
			Pricing:   pricing.NewHTTPSource("settings.ini"),
			ExcelPath: "dondathang_test.xlsx",
		},
		orderDir: orderFolderName,
	}, nil
}
```

Add the new imports (`"order-processor/internal/processing/pricing"`, `"order-processor/internal/processing/productdata"`) to `app.go`'s import block.

- [ ] **Step 8: Update `GO/main.go` to handle `NewApp`'s error**

```go
func main() {
	app, err := NewApp()
	if err != nil {
		println("Error:", err.Error())
		return
	}

	err = wails.Run(&options.App{
		// ... unchanged from Phase 1 ...
		OnStartup: app.startup,
		Bind:      []interface{}{app},
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
```

- [ ] **Step 9: Run the full Go test suite and build**

Run: `cd GO && go vet ./... && go test ./... -v && go build ./...`
Expected: everything passes and builds — this is the first point where the whole `GO/` module (Phase 1 + this plan's Tasks 1-11) compiles together end to end.

- [ ] **Step 10: Commit**

```bash
git add GO/internal/processing/processor.go GO/internal/processing/processor_test.go GO/app.go GO/app_test.go GO/main.go
git commit -m "feat(go): widen Processor to []OrderRow, wire RealProcessor into App"
```

---

### Task 12: Fixture-generation harness (Python, throwaway) — 155 golden fixtures

**Files:**
- Create: `GO/internal/processing/coop/testdata/generate_fixtures.py` (throwaway dev tool, not part of the shipped app)
- Create (generated by running the script): `GO/internal/processing/coop/testdata/fixtures/*.json`, `GO/internal/processing/coop/testdata/fixtures/_frozen_pricing.json`

**Interfaces:**
- Produces: one JSON fixture file per successfully-processed Coop PDF, each shaped as:
  ```json
  {
    "source_pdf": "102945235-00.pdf",
    "rows": [
      {"row_number_offset": 0, "A": "...", "B": "...", "Z_is_formula": false, "Y_has_comment": false, "Y_fill": null, "...": "..."},
      {"row_number_offset": 1, "Q": "...", "S": "...", "X": 24, "Y": 33726, "Z_is_formula": true, "Y_has_comment": false, "Y_fill": null, "...": "..."}
    ]
  }
  ```
  and `_frozen_pricing.json` shaped as `{"raw_rows": [[...row 0 as literal data, not headers...], [...row 1...], ...]}` — **exactly the raw CSV rows Coop's Google Sheet returns**, captured once with `header=None` (matching how `find_all_promotions_by_sku_and_time` fetches it in Python, and how `pricing.HTTPSource.FetchCoopIndex` fetches it in Go). This is deliberately a single flat snapshot, not a pre-split "prices" dict plus "promotions" table — `pricing.ParseIndex` already derives both the positional price view and the named-header promotion view from one such row set (see Task 3's design note), so the frozen fixture only needs to hold the one thing both views are built from. Task 13's Go test calls `pricing.ParseIndex(frozen.RawRows)` directly on this snapshot, exactly mirroring what `HTTPSource.FetchCoopIndex` does with a live fetch.

- [ ] **Step 1: Write the harness script**

```python
"""
Throwaway dev tool — NOT part of the shipped Go or Python app.

Runs the CURRENT xulydonhang.py Coop pipeline against every real PDF in
đơn hàng/08-2026/ that identify_vendor recognizes as Coop, capturing the
resulting dondathang.xlsx rows (and the live-fetched Google Sheets
price/promotion data) into JSON fixtures under
GO/internal/processing/coop/testdata/fixtures/. The Go golden test
(Task 13) diffs RealProcessor's output against these fixtures instead of
against a live Google Sheets fetch, so it's deterministic and offline.

Run from the repo root:
    .venv/Scripts/python.exe GO/internal/processing/coop/testdata/generate_fixtures.py
"""
import glob
import json
import os
import re
import shutil
import sys

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))))
sys.path.insert(0, REPO_ROOT)
os.chdir(REPO_ROOT)  # xulydonhang.py's functions use relative paths ("data.xlsx", "settings.ini")

import openpyxl  # noqa: E402
import xulydonhang  # noqa: E402

FIXTURES_DIR = os.path.join(
    REPO_ROOT, "GO", "internal", "processing", "coop", "testdata", "fixtures"
)
TEMPLATE_XLSX = os.path.join(REPO_ROOT, "dondathang.xlsx")
SCRATCH_XLSX = os.path.join(REPO_ROOT, "dondathang_fixture_scratch.xlsx")

# --- Monkey-patch network/upload side effects out ---

_price_cache = {}
_promo_cache = {}
_promo_raw_rows = None  # captured once, shared across all SKUs (same sheet)


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
    """Fetch the raw CSV once and stash it so we can freeze it verbatim,
    in addition to letting the real function do its own fetch+parse for
    the actual lookup (keeps this harness simple — one extra fetch is
    fine for a one-time fixture-generation run)."""
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


# --- Excel row capture ---

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


def is_coop_pdf(path):
    doc = xulydonhang.fitz.open(path)
    try:
        text = ""
        for page in doc:
            text += page.get_text("text")
            if len(text) > 3000:
                break
        return xulydonhang.ProcessHandler.identify_vendor(text) == "Coop"
    finally:
        doc.close()


def process_one_pdf(path):
    """Runs the Coop dispatch branch of process_file for a single-page
    (or multi-page) PDF, mirroring the relevant slice of process_file's
    logic without needing the full GUI-oriented method."""
    handler = xulydonhang.process_handler
    doc = xulydonhang.fitz.open(path)
    try:
        for page_num in range(len(doc)):
            text = doc[page_num].get_text("text")
            if xulydonhang.ProcessHandler.identify_vendor(text) != "Coop":
                continue

            counts = xulydonhang.ProcessHandler.demsodonhang1trang_coop(text)
            pom_count, sub_count = counts["POM343"], counts["Sub Total"]
            if pom_count == 0 or sub_count == 0 or pom_count != sub_count:
                continue

            if pom_count == 1:
                segments = [text]
            else:
                segments = xulydonhang.ProcessHandler.catdonra_nhieutrang(text)

            for i, segment in enumerate(segments):
                page_label = "1/1" if len(segments) == 1 else f"{i + 1}/{len(segments)}"
                handler.process_coop_invoice(segment, 1, path, page_label, doc)
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
            if not is_coop_pdf(path):
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

    # _promo_raw_rows is the ONE raw CSV snapshot Coop's Google Sheet
    # returns for gid=get_gid("COOP") — the same sheet find_price_by_sku
    # and find_all_promotions_by_sku_and_time both fetch (see Task 3's
    # design note). This single snapshot is what gets frozen; there is
    # no separate "prices" artifact, matching pricing.ParseIndex's
    # one-CSV-in, two-views-out design in the Go port.
    if _promo_raw_rows is None:
        _capture_promo_raw_rows("COOP")
    frozen_pricing = {"raw_rows": _promo_raw_rows or []}
    with open(os.path.join(FIXTURES_DIR, "_frozen_pricing.json"), "w", encoding="utf-8") as f:
        json.dump(frozen_pricing, f, ensure_ascii=False, indent=2, default=str)

    print(f"\nDone: {generated} fixtures generated, {skipped} PDFs skipped.")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Back up the real `dondathang.xlsx` before running (safety net beyond the script's own backup/restore)**

```bash
cp dondathang.xlsx dondathang.xlsx.manual_backup_before_fixtures
```

- [ ] **Step 3: Run the harness**

```bash
".venv/Scripts/python.exe" GO/internal/processing/coop/testdata/generate_fixtures.py
```

Expected: prints one `OK <file> -> N rows` line per successfully-processed Coop PDF, ending with a summary line. Some PDFs may print `SKIP` — that's expected for edge cases (e.g. any that hit the `catdonra_nhieutrang` case-sensitivity bug documented in this plan's Global Constraints and produce zero usable segments); note the skip count.

- [ ] **Step 4: Verify the real `dondathang.xlsx` is unchanged and remove the manual backup**

```bash
diff dondathang.xlsx dondathang.xlsx.manual_backup_before_fixtures && echo "unchanged, good" && rm dondathang.xlsx.manual_backup_before_fixtures
```

If this reports a difference, STOP — restore from the backup (`cp dondathang.xlsx.manual_backup_before_fixtures dondathang.xlsx`) and investigate the harness's backup/restore logic before proceeding; do not continue with a corrupted production file.

- [ ] **Step 5: Spot-check a couple of generated fixtures by eye**

```bash
cat "GO/internal/processing/coop/testdata/fixtures/102945235-00.json"
```

Confirm it has a plausible `rows` array (a header row followed by product rows) and that values look like real Vietnamese product names/prices, not error placeholders.

- [ ] **Step 6: Commit the fixtures**

```bash
git add GO/internal/processing/coop/testdata/
git commit -m "test(go): add golden fixtures generated from real Coop PDFs"
```

(The fixtures directory will contain up to 155 JSON files plus `_frozen_pricing.json` — this is expected and intentional; they're the correctness oracle for Task 13.)

---

### Task 13: Golden fixture integration test

**Files:**
- Create: `GO/internal/processing/coop_golden_test.go`

**Interfaces:**
- Consumes: `RealProcessor`, `pricing.ParseIndex`, `productdata.Load` (Tasks 3, 4, 10, 11), the fixtures generated in Task 12.

- [ ] **Step 1: Write the golden test**

```go
package processing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

// frozenPricingFixture mirrors _frozen_pricing.json's shape: the single
// raw CSV snapshot both the price and promotion views are derived from
// (see Task 3's design note and Task 12's harness comment).
type frozenPricingFixture struct {
	RawRows [][]string `json:"raw_rows"`
}

type fixtureData struct {
	SourcePDF string           `json:"source_pdf"`
	Rows      []map[string]any `json:"rows"`
}

// fixturePricingSource wraps a single frozen *pricing.Index so the
// golden test's RealProcessor doesn't hit the network — it satisfies
// the same PricingSource interface as the production HTTPSource.
type fixturePricingSource struct {
	index *pricing.Index
}

func (f *fixturePricingSource) FetchCoopIndex() (*pricing.Index, error) {
	return f.index, nil
}

func loadFrozenPricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("coop/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen pricing fixture found (run Task 12's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

func TestRealProcessor_MatchesGoldenFixtures(t *testing.T) {
	fixturePaths, err := filepath.Glob("coop/testdata/fixtures/*.json")
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
		t.Skip("no golden fixtures found (run Task 12's generate_fixtures.py first)")
	}

	store, err := productdata.Load("../../../data.xlsx")
	if err != nil {
		t.Fatalf("failed loading production data.xlsx: %v", err)
	}
	pricingSource := loadFrozenPricingSource(t)

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

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
}

// compareRowsAgainstFixture re-reads excelPath's "Don dat hang" sheet
// starting at the row RealProcessor just wrote (existing header rows
// before it are whatever the copied testdata/dondathang.xlsx template
// had — always 8, per Task 9's fixture), and diffs every column Task
// 12's harness captured against what's actually on disk. Text/SKU
// columns must match exactly; Y (price) and AT (line weight) allow a
// small float tolerance, since values round-trip through JSON and
// openpyxl/excelize float formatting in ways that can differ in the
// last decimal digit without being a real bug.
func compareRowsAgainstFixture(t *testing.T, excelPath string, fixture fixtureData, mismatches *[]string) {
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
	// RealProcessor appended len(fixture.Rows) rows starting right after
	// whatever was already in the sheet before Process ran; since this
	// test copies a fresh 8-row-header template per fixture (Task 9's
	// testdata/dondathang.xlsx), the written rows start at row 9 —
	// compute it from the actual row count instead of hardcoding 9, so
	// this still works if that template's header size ever changes.
	startRow := len(existingRows) - len(fixture.Rows) + 1
	if startRow < 1 {
		*mismatches = append(*mismatches, fmt.Sprintf("%s: sheet has fewer rows (%d) than the fixture expects (%d)", fixture.SourcePDF, len(existingRows), len(fixture.Rows)))
		return
	}

	textColumns := []string{"A", "B", "C", "D", "E", "G", "L", "Q", "S", "T", "U", "V", "AJ", "AM", "AO", "AP", "AQ"}
	floatColumns := []string{"X", "Y", "AT"}
	intColumns := []string{"AE", "AU", "AV"}

	for i, expectedRow := range fixture.Rows {
		rowNum := startRow + i
		cell := func(col string) string {
			v, _ := f.GetCellValue("Don dat hang", fmt.Sprintf("%s%d", col, rowNum))
			return v
		}

		for _, col := range textColumns {
			expected := stringify(expectedRow[col])
			got := cell(col)
			if expected != got {
				*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col %s: got %q, want %q", fixture.SourcePDF, i, col, got, expected))
			}
		}

		for _, col := range floatColumns {
			expected := toFloat(expectedRow[col])
			got := toFloat(cell(col))
			if !floatCloseEnough(expected, got) {
				*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col %s: got %v, want %v", fixture.SourcePDF, i, col, got, expected))
			}
		}

		for _, col := range intColumns {
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
	}
}

func stringify(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return toFloatString(t)
	case bool:
		if t {
			return "Có"
		}
		return "Không"
	default:
		return fmt.Sprintf("%v", t)
	}
}

func toFloatString(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%v", f)
}

func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		var f float64
		fmt.Sscanf(t, "%f", &f)
		return f
	default:
		return 0
	}
}

func floatCloseEnough(a, b float64) bool {
	const tolerance = 0.01
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed reading %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("failed writing %s: %v", dst, err)
	}
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += "  - " + l + "\n"
	}
	return out
}
```

**Note on this comparison's precision:** `compareRowsAgainstFixture` compares every column Task 12's harness captured except the price-mismatch fill/comment flags (`Y_has_comment`, `Y_fill`) — add those two checks too if Task 13's first golden-test run shows real fixtures with `Y_has_comment: true` (i.e., real price mismatches occurred in the sample data), using `f.GetComments("Don dat hang")` and `f.GetCellStyle` the same way Task 9's `excelwriter` test already does. This plan doesn't hardcode that check up front because whether any of the 155 real fixtures actually hit the price-mismatch path is an empirical question this task's first run answers — don't add untested code for a case you haven't confirmed occurs in the data.

- [ ] **Step 2: Run the test, expect it to fail or need adjustment, iterate against real fixture data**

Run: `cd GO && go test ./internal/processing/... -run TestRealProcessor_MatchesGoldenFixtures -v`

This is the step where the cumulative translation work from Tasks 1-11 gets checked against 155 real orders. Expect this to surface real mismatches on the first run — that is the test doing its job, not a sign something upstream is broken. For each mismatch:
1. Read the fixture JSON and the actual Go output side by side.
2. Find the specific Task (2-11) whose ported function produced the wrong value.
3. Re-read that function's exact Python source (included in that task) against the Go translation, line by line.
4. Fix the Go code (not the fixture, and not this test's tolerances, unless the mismatch is a legitimate floating-point rounding difference).
5. Re-run.

Do not consider this task done until every fixture matches or every remaining mismatch has been triaged with a written explanation (in a fix report, per this repo's subagent-driven-development ledger conventions) of why it's an acceptable, understood difference rather than a bug.

- [ ] **Step 3: Run the full suite one more time**

Run: `cd GO && go vet ./... && go test ./... -v`
Expected: everything passes, including all 155 golden fixtures.

- [ ] **Step 4: Commit**

```bash
git add GO/internal/processing/coop_golden_test.go
git commit -m "test(go): add golden fixture integration test against 155 real Coop PDFs"
```

---

## After this plan

Phase 2a is done when Task 13's golden test passes clean against all (or a fully-triaged subset of) the 155 real Coop fixtures, and a human has run the compiled app against a handful of real Coop PDFs end-to-end (pick file → process → verify the written `dondathang_test.xlsx` by eye against what the current Python app would have produced) — the same kind of human acceptance pass Phase 1 closed with. Phase 2b (BigC) is a separate spec/plan, built on this phase's `Processor`/`vendor.Identify`/`excelwriter` seams rather than starting from scratch.
