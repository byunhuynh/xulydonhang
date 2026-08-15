# Satra RealProcessor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the existing Go `RealProcessor` (Coop in Phase 2a, Lotte in Phase 2b-1) to also parse real Satra purchase-order PDFs, resolve customer codes via fuzzy address matching, compute pricing/promotions, and write results into the same "Don dat hang" Excel sheet — validated against 33 real archived Satra PDFs via the same golden-fixture methodology.

**Architecture:** New `GO/internal/processing/satra/` package for Satra-specific PDF extraction (PO#, entry/cancel date with a date-fallback quirk, ship-to address, product list) plus a fuzzy-matching customer-code lookup. A new `satra_processor.go` (created from the start — Phase 2b-1's final review found appending a new vendor's processor to `coop_processor.go` caused real bugs from copy-paste of Coop-specific assumptions; Satra gets its own file from Task 1 of this plan, no later split needed). Satra's promo/bonus-row logic in `write_to_dondathang_satra` is structurally **identical to Coop's** `write_to_dondathang` (confirmed by direct source comparison: same `khuyenmai.split('|')` + `enumerate` loop, same `"KM Bó Kèm - Che Barcode"` no-brace default) — unlike Lotte, which needed overrides on the shared helpers, Satra reuses `buildPromoBonusRow`/`buildInvoiceBonusRow` **unmodified**, no post-call correction needed.

**Tech Stack:** Same as Phase 2a/2b-1 — Go, `github.com/xuri/excelize/v2`, `github.com/ledongthuc/pdf`, plus a new dependency: `github.com/paul-mannino/go-fuzzywuzzy` (a direct Go port of the Python `fuzzywuzzy` library `xulydonhang.py` actually imports — see Global Constraints for the empirical validation already performed).

**Spec:** [2026-08-15-satra-real-processor-design.md](../specs/2026-08-15-satra-real-processor-design.md)

## Global Constraints

- **Testing/divergence policy** (same as Lotte/Phase 2b-1, different from Coop/Phase 2a): golden-fixture test compares against real Python output, but when Go intentionally computes a different, verified-more-correct value because Python is confirmed wrong, record it in a commented `knownDivergences_Satra` allowlist (key format `sourcePDF:rowIndex:column`, per Phase 2b-1's final-review fix — never the bare `rowIndex:column` form the original Lotte plan mistakenly used). Never edit fixture JSON files to force a pass.
- **`github.com/paul-mannino/go-fuzzywuzzy` is empirically pre-validated for this exact use case** — before writing this plan, its `PartialRatio(a, b string) int` function was run against all 33 real Satra addresses' normalized text, both (a) directly against each address's already-known-correct matching `data.xlsx` row, and (b) as a full 192-row argmax scan mirroring `laymakhachhang_satra`'s actual selection logic. Both checks: **0/33 mismatches**, scores identical to Python's real `fuzz.partial_ratio` (which itself runs on the `python-Levenshtein`-accelerated backend, confirmed installed in this repo's `.venv` — `go-fuzzywuzzy` is a from-scratch port of that same C library's algorithm, which is why the scores line up exactly). Pin the exact version already resolved: `github.com/paul-mannino/go-fuzzywuzzy v0.0.0-20241117160931-a1769aeb6b21`. Do not substitute a different fuzzy-matching library or hand-roll Levenshtein-ratio matching — this one is proven correct against real data, not a guess.
- **Reuse, do not reimplement**, these already-shipped functions: `regionInfo`, `closeEnough`, `buildPromoBonusRow`, `buildInvoiceBonusRow`, `orderNumber`-style formatting pattern, `coop.ExtractDiscount`, `coop.ExtractBraceContent`, `coop.ExtractMoneyAmount`, `coop.LastFourDigits`, `coop.FormatWeightKg`, `productdata.Store.FindSkusMentioned/ResolveSku/GetProductInfo`, `excelwriter.Row/WriteOrderRows`, `pricing.Index`/`PricingSource.FetchIndex`. Confirmed during design: Satra's promo/bonus-row logic needs **zero overrides** on `buildPromoBonusRow`/`buildInvoiceBonusRow` (unlike Lotte) — it matches Coop's calling convention exactly.
- **New package** `GO/internal/processing/satra/` for Satra-only extraction + fuzzy matching, mirroring the `lotte`/`coop` package shape.
- **New file** `GO/internal/processing/satra_processor.go` (+ `satra_processor_test.go`) from the start — do not append to `coop_processor.go` or `lotte_processor.go`.
- Every exported function gets a doc comment citing the exact `xulydonhang.py` line range it mirrors. Every deviation from a literal Python behavior gets an inline comment explaining why.
- Run `go build ./...`, `go vet ./...`, and the relevant `go test` scope after every task, from the `GO/` directory.
- **A known, not-yet-confirmed discrepancy to watch for during Task 7 (golden fixtures):** Satra's invoice-level bonus row (`xulydonhang.py:2664`) computes its `AU` (case count) from a **stale leftover variable** (`qty_ord_pcs`, left over from the LAST product in the per-product loop above) instead of `soluongkm` (the actual invoice-bonus quantity, correctly used for `AT` on the very next line, `:2666`) — a real, confirmed-by-reading bug in the Python source. The already-shipped `buildInvoiceBonusRow` helper (built during Coop's phase) correctly uses the bonus quantity for both `AT` and `AU`. If Task 7's golden-fixture run shows an `AU` mismatch on any fixture with an invoice-level bonus row, this is almost certainly why — root-cause against `:2664` before assuming it's a Go bug, and if confirmed, this is exactly the kind of case `knownDivergences_Satra` exists for (Go's shared helper is more correct; document, don't force-match Python's stale-variable bug).

---

### Task 1: `vendor.Identify` — recognize Satra

**Files:**
- Modify: `GO/internal/processing/vendor/identify.go`
- Modify: `GO/internal/processing/vendor/identify_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `Identify(text string) string` now also returns `"Satra"` — consumed by Task 5's dispatch in `RealProcessor.Process`.

- [ ] **Step 1: Write the failing test**

Add to `GO/internal/processing/vendor/identify_test.go`:

```go
func TestIdentify_RecognizesSatraByVDCode(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"VD code form 1", "Mã số thuế: VD-00002345 Satra Group", "Satra"},
		{"VD code form 2", "VD-00002547", "Satra"},
		{"unrelated VD code", "VD-00009999", ""},
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

Run: `cd GO && go test ./internal/processing/vendor/... -run TestIdentify_RecognizesSatraByVDCode -v`
Expected: FAIL — `Identify` returns `""` for every case.

- [ ] **Step 3: Implement**

Edit `GO/internal/processing/vendor/identify.go` (existing file) — add a `satraPattern` var and a check in `Identify`, following the exact pattern already used for `lottePattern`:

```go
	// Satra's two VD-code forms, mirroring identify_vendor's third
	// branch (xulydonhang.py:105-109): either literal substring
	// appearing anywhere in the (whitespace-normalized) page text.
	satraPattern = regexp.MustCompile(`VD-00002345|VD-00002547`)
```

Add to the `var (...)` block alongside `coopPattern`/`lottePattern`, and add to `Identify`:

```go
	if satraPattern.MatchString(cleaned) {
		return "Satra"
	}
```

placed after the Lotte check, before the final `return ""`. Update `Identify`'s doc comment to mention Coop, Lotte, and Satra are implemented.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/vendor/... -v`
Expected: PASS — all Coop and Lotte tests still pass (regression check), all new Satra tests pass.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/vendor/identify.go GO/internal/processing/vendor/identify_test.go
git commit -m "feat(go): recognize Satra vendor in identify.Identify"
```

---

### Task 2: `productdata.Store` — fuzzy-address customer code lookup

**Files:**
- Modify: `GO/go.mod` / `GO/go.sum` (add `github.com/paul-mannino/go-fuzzywuzzy`)
- Modify: `GO/internal/processing/productdata/store.go`
- Modify: `GO/internal/processing/productdata/store_test.go`
- Modify: `GO/internal/processing/productdata/testdata/data.xlsx` — add at least one `SATRA`-system row to the `MaKH` sheet with a realistic Vietnamese address in column D, for this task's test. (Follow the same pattern the existing `LOTTE 777 KH-LOTTE-003` row already there was added for — use a tool/script capable of appending an xlsx row; do not hand-edit binary. A minimal one-off Go or Python snippet run once, discarded after, is fine.)

**Interfaces:**
- Consumes: `Store.customerRows [][4]string` (already loaded by `Load`).
- Produces: `NormalizeText(s string) string` (package `productdata`, or `satra` — see Step 3 for the decision) and `(*Store) GetCustomerCodeByFuzzyAddress(system, address string) (code string, ok bool)` — consumed by Task 5.

- [ ] **Step 1: Add the dependency**

```bash
cd GO && go get github.com/paul-mannino/go-fuzzywuzzy@v0.0.0-20241117160931-a1769aeb6b21
```

Verify `go.mod` now lists it as a direct (non-indirect) requirement and `go.sum` has matching entries. Run `go build ./...` to confirm it resolves.

- [ ] **Step 2: Write the failing test**

First, add a `SATRA` row to `productdata/testdata/data.xlsx`'s `MaKH` sheet — column A = `SATRA`, column B = any store code, column C = a customer code like `MN_MT_TESTSTF`, column D = a realistic address, e.g. `"123 Nguyễn Huệ, Phường Bến Nghé, Quận 1, Tp.HCM, VNM"`. Confirm by reading the sheet back (a throwaway script) that the row landed correctly and existing rows (`COOP`/`COOPFOOD`/`LOTTE`) are untouched.

Add to `GO/internal/processing/productdata/store_test.go`:

```go
func TestGetCustomerCodeByFuzzyAddress_MatchesSatraByAddress(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	// Exact match (after normalization) must resolve with a high score.
	got, ok := store.GetCustomerCodeByFuzzyAddress("SATRA", "123 Nguyễn Huệ, Phường Bến Nghé, Quận 1, Tp.HCM, VNM")
	if !ok || got != "MN_MT_TESTSTF" {
		t.Fatalf("GetCustomerCodeByFuzzyAddress(SATRA, exact) = (%q, %v), want (%q, true)", got, ok, "MN_MT_TESTSTF")
	}
	// A wildly different address must not match (score well under the 95 threshold).
	if _, ok := store.GetCustomerCodeByFuzzyAddress("SATRA", "999 Đường Không Tồn Tại, Xã Lạ, Tỉnh Khác"); ok {
		t.Fatal("GetCustomerCodeByFuzzyAddress(SATRA, unrelated) = matched, want no match")
	}
	// System filter: querying under a system that has no rows must not match,
	// even with the exact same address text.
	if _, ok := store.GetCustomerCodeByFuzzyAddress("BIGC", "123 Nguyễn Huệ, Phường Bến Nghé, Quận 1, Tp.HCM, VNM"); ok {
		t.Fatal("GetCustomerCodeByFuzzyAddress(BIGC, exact SATRA address) = matched, want no match (system filter)")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/productdata/... -run TestGetCustomerCodeByFuzzyAddress -v`
Expected: FAIL with `undefined: GetCustomerCodeByFuzzyAddress`.

- [ ] **Step 4: Implement**

Add to `GO/internal/processing/productdata/store.go` (existing file; add `"strings"` and `"regexp"` imports if not already present, and import `fuzzywuzzy "github.com/paul-mannino/go-fuzzywuzzy"`):

```go
var normalizeTextPattern = regexp.MustCompile(`[^a-z0-9\s]`)
var normalizeWhitespacePattern = regexp.MustCompile(`\s+`)

// NormalizeText mirrors normalize_text (xulydonhang.py:217-222) exactly:
// lowercase, then strip every character that is not an ASCII letter,
// digit, or whitespace (this deliberately removes Vietnamese diacritic
// letters entirely, not just their diacritic marks — e.g. "Huệ" becomes
// "hu", not "hue" — because Python's [^a-z0-9\s] character class only
// allows literal ASCII a-z/0-9, and re operates on Unicode code points,
// so any non-ASCII letter is stripped whole), then collapse runs of
// whitespace to one space and trim.
func NormalizeText(s string) string {
	lower := strings.ToLower(s)
	stripped := normalizeTextPattern.ReplaceAllString(lower, "")
	return strings.TrimSpace(normalizeWhitespacePattern.ReplaceAllString(stripped, " "))
}

// GetCustomerCodeByFuzzyAddress mirrors laymakhachhang_satra
// (xulydonhang.py:263-287): filters customer rows to those whose column
// A (system), uppercased and trimmed, is a SUBSTRING of the given system
// string (Python: `col_A.upper() in hethong` — NOT equality; preserved
// exactly, since Satra's real call site passes the literal system name
// itself as `hethong`, e.g. laymakhachhang_satra(diachi, "SATRA"), so a
// column A of "SATRA" is trivially "in" it, but this is a real substring
// check, not coincidentally equivalent to equality for every possible
// input), then finds the row whose column D (address), both sides run
// through NormalizeText, has the highest PartialRatio score against the
// given address — returns that row's column C if the best score is
// STRICTLY greater than 95, mirroring Python's `best_score > 95` (not
// >=). Returns ("", false) if no row exceeds the threshold — mirrors
// Python returning None; the caller applies any "Không xác định"-style
// placeholder itself.
func (s *Store) GetCustomerCodeByFuzzyAddress(system, address string) (string, bool) {
	systemUpper := strings.ToUpper(system)
	addressNorm := NormalizeText(address)

	bestScore := 0
	bestCode := ""
	for _, row := range s.customerRows {
		colA, colC, colD := row[0], row[2], row[3]
		if !strings.Contains(systemUpper, strings.ToUpper(strings.TrimSpace(colA))) {
			continue
		}
		if colD == "" {
			continue
		}
		score := fuzzywuzzy.PartialRatio(addressNorm, NormalizeText(colD))
		if score > bestScore {
			bestScore = score
			bestCode = colC
		}
	}
	if bestScore > 95 {
		return bestCode, true
	}
	return "", false
}
```

**Important correctness note for the implementer:** double-check `s.customerRows`' shape in `store.go`'s `loadCustomerRows` — it currently loads `row[0..3]` into a `[4]string`, so `row[3]` is column D. Confirm this before writing the above; if `loadCustomerRows` needs no change, don't touch it.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/productdata/... -v`
Expected: PASS — all existing tests plus the 3 new assertions in the new test.

- [ ] **Step 6: Commit**

```bash
git add GO/go.mod GO/go.sum GO/internal/processing/productdata/store.go GO/internal/processing/productdata/store_test.go GO/internal/processing/productdata/testdata/data.xlsx
git commit -m "feat(go): add fuzzy-address customer code lookup for Satra"
```

---

### Task 3: `satra` package — PO number, entry date (with fallback), cancel date

**Files:**
- Create: `GO/internal/processing/satra/extract.go`
- Create: `GO/internal/processing/satra/extract_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `satra.ParsePONumber(text string) (string, bool)`; `satra.ParseEntryDate(text string) (string, bool)`; `satra.ParseCancelDate(text string) (string, bool)`. All consumed by Task 5.

- [ ] **Step 1: Write the failing tests**

Create `GO/internal/processing/satra/extract_test.go`:

```go
package satra

import "testing"

func TestParsePONumber_ExtractsBetweenAsterisks(t *testing.T) {
	text := "Header\n*P-005508192*\nmore text"
	got, ok := ParsePONumber(text)
	if !ok || got != "P-005508192" {
		t.Fatalf("ParsePONumber = (%q, %v), want (%q, true)", got, ok, "P-005508192")
	}
}

func TestParsePONumber_NoMatchReturnsFalse(t *testing.T) {
	if _, ok := ParsePONumber("no po number here"); ok {
		t.Fatal("ParsePONumber = matched, want no match")
	}
}

func TestParseEntryDate_UsesLineBeforeMarker(t *testing.T) {
	text := "some header\n08/13/2026\nNgày đặt hàng: 08/13/2026"
	got, ok := ParseEntryDate(text)
	if !ok || got != "13/08/2026" {
		t.Fatalf("ParseEntryDate = (%q, %v), want (%q, true)", got, ok, "13/08/2026")
	}
}

func TestParseEntryDate_FallsBackToNgayInWhenPlaceholderDate(t *testing.T) {
	// The PDF template literally renders "01/01/0001" when the "Ngày đặt
	// hàng" field is unset — Python detects this exact placeholder string
	// after formatting and retries against "Ngày in:" instead.
	text := "header\n01/01/0001\nNgày đặt hàng: 01/01/0001\nmore\n08/14/2026\nNgày in: 08/14/2026"
	got, ok := ParseEntryDate(text)
	if !ok || got != "14/08/2026" {
		t.Fatalf("ParseEntryDate (fallback) = (%q, %v), want (%q, true)", got, ok, "14/08/2026")
	}
}

func TestParseEntryDate_NoMatchReturnsFalse(t *testing.T) {
	if _, ok := ParseEntryDate("no date marker here"); ok {
		t.Fatal("ParseEntryDate = matched, want no match")
	}
}

func TestParseCancelDate_FindsFirstDateShapedLineInBlock(t *testing.T) {
	text := "Ngày giao hàng:\nKhẩn cấp\n08/20/2026\nĐịa chỉ giao hàng: 123 Đường ABC"
	got, ok := ParseCancelDate(text)
	if !ok || got != "20/08/2026" {
		t.Fatalf("ParseCancelDate = (%q, %v), want (%q, true)", got, ok, "20/08/2026")
	}
}

func TestParseCancelDate_NoMatchReturnsFalse(t *testing.T) {
	if _, ok := ParseCancelDate("no markers here"); ok {
		t.Fatal("ParseCancelDate = matched, want no match")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/satra/... -v`
Expected: FAIL — package doesn't exist yet / functions undefined.

- [ ] **Step 3: Implement**

Create `GO/internal/processing/satra/extract.go`:

```go
package satra

import (
	"regexp"
	"strings"
	"time"
)

var poNumberPattern = regexp.MustCompile(`\*(P-[^*]+)\*`)

// ParsePONumber mirrors the PO-number extraction at the top of
// process_file's Satra branch (xulydonhang.py:9309-9310): the PO number
// is whatever sits between two literal "*" characters, prefixed "P-".
// Python captures the WHOLE "*...*" match then strips the first/last
// character; this uses a capture group directly instead, equivalent.
func ParsePONumber(text string) (string, bool) {
	m := poNumberPattern.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	return m[1], true
}

var entryDateBlockPattern = regexp.MustCompile(`(?s)(.*?)\nNgày đặt hàng:`)
var printDateBlockPattern = regexp.MustCompile(`(?s)(.*?)\nNgày in:`)

const placeholderDate = "01/01/0001"

// ParseEntryDate mirrors xulydonhang.py:9326-9336: takes everything
// before the first "Ngày đặt hàng:" marker, uses its LAST line as the
// raw date, parses "MM/DD/YYYY" and reformats "DD/MM/YYYY". If the
// result is the literal placeholder "01/01/0001" (the PDF template
// renders this when the entry-date field itself is unset in the source
// system — not a parse failure), retries the same shape against
// "Ngày in:" instead. Returns false if neither marker is found or
// neither produces a parseable date.
func ParseEntryDate(text string) (string, bool) {
	date, ok := parseDateBeforeMarker(text, entryDateBlockPattern)
	if ok && date != "13/08/2026" && date == formatMDYtoDMY(placeholderDate) {
		// placeholder detected — fall through to retry below
		ok = false
	}
	if !ok {
		return parseDateBeforeMarker(text, printDateBlockPattern)
	}
	return date, true
}

func parseDateBeforeMarker(text string, blockPattern *regexp.Regexp) (string, bool) {
	m := blockPattern.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	lines := strings.Split(m[1], "\n")
	raw := strings.TrimSpace(lines[len(lines)-1])
	return formatMDYtoDMYChecked(raw)
}

func formatMDYtoDMYChecked(raw string) (string, bool) {
	t, err := time.Parse("01/02/2006", raw)
	if err != nil {
		return "", false
	}
	return t.Format("02/01/2006"), true
}

// formatMDYtoDMY is a small helper solely for comparing against the
// known placeholder string in ParseEntryDate's fallback check above; it
// panics on unparseable input by design since it's only ever called with
// the literal constant placeholderDate.
func formatMDYtoDMY(raw string) string {
	t, err := time.Parse("01/02/2006", raw)
	if err != nil {
		panic("formatMDYtoDMY: unparseable literal: " + raw)
	}
	return t.Format("02/01/2006")
}

var cancelDateBlockPattern = regexp.MustCompile(`(?s)Ngày giao hàng:\s*(.*?)\s*Địa chỉ giao hàng:`)
var cancelDateLinePattern = regexp.MustCompile(`\d{1,2}/\d{1,2}/\d{4}`)

// ParseCancelDate mirrors xulydonhang.py:9339-9347: within the block
// between "Ngày giao hàng:" and "Địa chỉ giao hàng:", finds the FIRST
// line containing a d/d/dddd-shaped date, parses "MM/DD/YYYY", reformats
// "DD/MM/YYYY". Returns false if the block or no date-shaped line within
// it is found.
func ParseCancelDate(text string) (string, bool) {
	m := cancelDateBlockPattern.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	for _, line := range strings.Split(m[1], "\n") {
		if cancelDateLinePattern.MatchString(line) {
			return formatMDYtoDMYChecked(strings.TrimSpace(line))
		}
	}
	return "", false
}
```

**Note for the implementer:** the `formatMDYtoDMY`/placeholder-comparison approach above is deliberately awkward (comparing formatted strings, with a stray hardcoded `"13/08/2026"` short-circuit that doesn't belong there) — this is a mistake in this brief's literal code, not something to transcribe as-is. Fix it: compare the RAW extracted date string (before formatting) against the literal `"01/01/0001"` directly, not the formatted output, and delete the bogus `"13/08/2026"` comparison entirely. Rewrite `ParseEntryDate` so `parseDateBeforeMarker` optionally returns the raw pre-format string too (or inline the block-pattern/last-line extraction in `ParseEntryDate` itself), check `raw == placeholderDate` directly, and only then decide whether to retry with `printDateBlockPattern`. Verify your rewrite against both entry-date tests above (the plain case and the fallback case) before moving on — the tests are correct and complete even though the reference implementation above has this bug.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/satra/... -v`
Expected: PASS — all 6 tests.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/satra/extract.go GO/internal/processing/satra/extract_test.go
git commit -m "feat(go): add satra package with PO number and date extraction"
```

---

### Task 4: `satra` package — ship-to address, product extraction

**Files:**
- Modify: `GO/internal/processing/satra/extract.go`
- Modify: `GO/internal/processing/satra/extract_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `satra.ParseShipToAddress(text string) (string, bool)`; `satra.Product{Barcode string; Qty, TotalPrice float64}`; `satra.ExtractProducts(text string) []Product`. Consumed by Task 5.

- [ ] **Step 1: Write the failing tests**

Add to `GO/internal/processing/satra/extract_test.go`:

```go
func TestParseShipToAddress_JoinsLinesBetweenMarkers(t *testing.T) {
	text := "Địa chỉ giao hàng:\n123 Nguyễn Huệ\nPhường Bến Nghé\nĐịa chỉ thanh toán:\nsomewhere else"
	got, ok := ParseShipToAddress(text)
	if !ok {
		t.Fatal("ParseShipToAddress: no match, want match")
	}
	want := "123 Nguyễn Huệ Phường Bến Nghé"
	if got != want {
		t.Fatalf("ParseShipToAddress = %q, want %q", got, want)
	}
}

func TestParseShipToAddress_NoMatchReturnsFalse(t *testing.T) {
	if _, ok := ParseShipToAddress("no markers here"); ok {
		t.Fatal("ParseShipToAddress = matched, want no match")
	}
}

func TestExtractProducts_ParsesBarcodeAnchoredBlocks(t *testing.T) {
	// Shape mirrors trichxuatsanpham_satra's expectations: a line with
	// "N D" (STT + something) followed by a 13-digit barcode line, then
	// free-form lines, one of which is "N,000"-shaped (quantity), the
	// NEXT line being the total price, ending before "Tổng cộng".
	text := "1 1\n1234567890123\nSome Product Name\n5,000\n199,000,00\n" +
		"2 1\n9876543210987\nAnother Product\n3,000\n99,000,00\n" +
		"Tổng cộng\nfooter text"
	got := ExtractProducts(text)
	want := []Product{
		{Barcode: "1234567890123", Qty: 5, TotalPrice: 199000},
		{Barcode: "9876543210987", Qty: 3, TotalPrice: 99000},
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

func TestExtractProducts_SkipsZeroPriceLines(t *testing.T) {
	text := "1 1\n1234567890123\nFree Sample\n1,000\n0,00\nTổng cộng"
	got := ExtractProducts(text)
	if len(got) != 0 {
		t.Fatalf("ExtractProducts = %+v, want empty (zero-price line must be skipped)", got)
	}
}

func TestExtractProducts_NoBarcodeMatchesReturnsEmpty(t *testing.T) {
	if got := ExtractProducts("no product data here\nTổng cộng"); len(got) != 0 {
		t.Fatalf("ExtractProducts = %+v, want empty", got)
	}
}
```

**Note on the test fixture text above:** this is a hand-constructed approximation of the real shape based on reading `trichxuatsanpham_satra`'s regex — the implementer MUST verify this test's input actually matches the regex pattern being implemented (run it, don't assume) and adjust the literal test text if the regex's actual match boundaries differ from what's assumed here, while preserving the SAME two assertions (correct barcode+qty+price extraction, and zero-price skip). Real Satra PDF text (available at `đơn hàng/08-2026/*.pdf`, 33 files matching Satra's vendor pattern) is the ultimate ground truth if the synthetic text proves ambiguous — Task 6 will validate against all 33 real files regardless, so a synthetic unit test that's approximately right and gets refined against real data during Task 6 is acceptable; getting the regex itself right (matching the brief's literal pattern) matters more than the synthetic test text being perfect on the first attempt.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/satra/... -run "TestParseShipToAddress|TestExtractProducts" -v`
Expected: FAIL — undefined functions/types.

- [ ] **Step 3: Implement**

Add to `GO/internal/processing/satra/extract.go` (append; add `"strconv"` to imports):

```go
var shipToAddressPattern = regexp.MustCompile(`(?s)Địa chỉ giao hàng:\s*((?:.*\n)+?)Địa chỉ thanh toán:`)

// ParseShipToAddress mirrors xulydonhang.py:9312-9314: the block of
// lines between "Địa chỉ giao hàng:" and "Địa chỉ thanh toán:", joined
// into one line (newlines replaced with a single space), with any
// double-space collapsed to one.
func ParseShipToAddress(text string) (string, bool) {
	m := shipToAddressPattern.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	joined := strings.TrimSpace(strings.ReplaceAll(m[1], "\n", " "))
	return strings.ReplaceAll(joined, "  ", " "), true
}

// Product is one product line extracted from a Satra order's product
// table.
type Product struct {
	Barcode    string
	Qty        float64
	TotalPrice float64
}

var totalCutoffPattern = regexp.MustCompile(`\bTổng cộng\b`)
var productBlockStartPattern = regexp.MustCompile(`(?m)^\s*\d+\s+\d+\s*\n\s*(\d{13})`)
var quantityLinePattern = regexp.MustCompile(`\b(\d{1,3}),000\b`)
var trailingZeroCentsPattern = regexp.MustCompile(`,00$`)

// ExtractProducts mirrors trichxuatsanpham_satra (xulydonhang.py:6492-6529):
// cuts the text at the first "Tổng cộng", finds every position where a
// line matching "STT count" is immediately followed by a 13-digit
// barcode line, and treats each such position as the start of one
// product's block (ending where the next one starts, or at the cutoff).
// Within each block, spaces are stripped from every line; the first line
// matching "N,000" (1-3 digits before the literal ",000") is the
// quantity (with ",000" replaced by just the digits), and the line right
// after it is the total price (with a trailing ",00" stripped). A block
// with no quantity-shaped line, or whose price fails to parse as a
// non-zero number, is skipped entirely.
func ExtractProducts(text string) []Product {
	cut := totalCutoffPattern.Split(text, 2)[0]
	cut = strings.TrimSpace(cut)

	matches := productBlockStartPattern.FindAllStringSubmatchIndex(cut, -1)
	if matches == nil {
		return nil
	}

	type position struct {
		start   int
		barcode string
	}
	positions := make([]position, 0, len(matches)+1)
	for _, m := range matches {
		positions = append(positions, position{start: m[0], barcode: cut[m[2]:m[3]]})
	}
	positions = append(positions, position{start: len(cut), barcode: ""})

	var products []Product
	for i := 0; i < len(positions)-1; i++ {
		start, barcode := positions[i].start, positions[i].barcode
		end := positions[i+1].start
		block := strings.TrimSpace(cut[start:end])

		var lines []string
		for _, line := range strings.Split(block, "\n") {
			line = strings.ReplaceAll(line, " ", "")
			if line != "" {
				lines = append(lines, line)
			}
		}

		qtyIndex := -1
		for i, line := range lines {
			if quantityLinePattern.MatchString(line) {
				qtyIndex = i
				break
			}
		}
		if qtyIndex == -1 || qtyIndex+1 >= len(lines) {
			continue
		}

		qtyStr := quantityLinePattern.FindStringSubmatch(lines[qtyIndex])[1]
		qty, err := strconv.ParseFloat(qtyStr, 64)
		if err != nil {
			continue
		}

		priceStr := trailingZeroCentsPattern.ReplaceAllString(lines[qtyIndex+1], "")
		price, err := strconv.ParseFloat(strings.ReplaceAll(priceStr, ",", ""), 64)
		if err != nil || price == 0 {
			continue
		}

		products = append(products, Product{Barcode: barcode, Qty: qty, TotalPrice: price})
	}
	return products
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/satra/... -v`
Expected: PASS — all tests in the package (Task 3's + this task's). If `TestExtractProducts_*` fails because the synthetic test text doesn't actually satisfy `productBlockStartPattern`/`quantityLinePattern` as constructed, adjust the TEST TEXT (not the regex, which is a direct transcription of the brief's literal Python pattern) until it does, preserving the same assertions.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/satra/extract.go GO/internal/processing/satra/extract_test.go
git commit -m "feat(go): add ParseShipToAddress and ExtractProducts to satra package"
```

---

### Task 5: `RealProcessor` — dispatch to Satra

**Files:**
- Modify: `GO/internal/processing/coop_processor.go` (only the `Process` dispatch switch — add one `case`)
- Create: `GO/internal/processing/satra_processor.go`
- Create: `GO/internal/processing/satra_processor_test.go`
- Create: `GO/internal/processing/testdata/sample_satra_order.pdf` (copy of a real file)

**Interfaces:**
- Consumes: `vendor.Identify` (Task 1), `productdata.GetCustomerCodeByFuzzyAddress` (Task 2), `satra.ParsePONumber/ParseEntryDate/ParseCancelDate/ParseShipToAddress/ExtractProducts/Product` (Tasks 3-4), and the already-shipped `regionInfo`, `closeEnough`, `buildPromoBonusRow`, `buildInvoiceBonusRow`, `coop.ExtractDiscount`, `coop.FormatWeightKg`, `coop.ExtractBraceContent`, `coop.LastFourDigits`, `productdata.FindSkusMentioned`.
- Produces: `RealProcessor.Process` now routes Satra pages to a new `processSatraSegment` method in `satra_processor.go`.

- [ ] **Step 1: Copy a real sample file into testdata**

Use `P-005508192.pdf` (already selected and verified during planning):

```bash
cp "đơn hàng/08-2026/P-005508192.pdf" GO/internal/processing/testdata/sample_satra_order.pdf
```

This file's real values (already extracted during planning, by running the real Python functions directly, not guessed): PO number `"P-005508192"`, ship-to address `"44 Đường Số 1, Phường Tân Mỹ,HCM,VNM"`, and — against the REAL production `data.xlsx` (192 SATRA rows) — fuzzy-matches to customer code `"MN_MT_STF1104"`. This last value is NOT expected to reproduce in Step 2's test, which uses the small `productdata/testdata/data.xlsx` fixture (only the one synthetic SATRA row Task 2 added, a different address) — the test must actually be run to see whether that synthetic row's address happens to fuzzy-match this real address above the >95 threshold or not; do not assume either outcome without running it.

- [ ] **Step 2: Write the failing test**

Create `GO/internal/processing/satra_processor_test.go`. Follow the exact structure of the existing Lotte equivalent (`TestRealProcessor_ProcessesRealSampleLotteFile` in `lotte_processor_test.go`) for scaffolding (`copyTestWorkbookForProcessor`, `fixturePricingSource`, etc. — these are already defined in this package, reuse them, don't redeclare):

```go
package processing

import (
	"context"
	"testing"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleSatraFile(t *testing.T) {
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
	rows, err := rp.Process(context.Background(), "testdata/sample_satra_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Process returned %d rows, want 1: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.System != "Satra" {
		t.Fatalf("row.System = %q, want %q", row.System, "Satra")
	}
	if row.PO != "P-005508192" {
		t.Fatalf("row.PO = %q, want %q", row.PO, "P-005508192")
	}
	// The test fixture's data.xlsx SATRA row (added in Task 2) uses a
	// different address than this real file's — run the test first to see
	// whether it fuzzy-matches above the >95 threshold anyway (normalized
	// short Vietnamese addresses can sometimes score higher than
	// expected). Assert MaKhachHang against whatever the ACTUAL observed
	// result is (either "Không xác định" or the Task 2 fixture row's real
	// code) — do not guess; add the assertion after seeing the real
	// output once, the same way every other test in this plan was pinned
	// to a real observed value rather than an assumption.
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/... -run TestRealProcessor_ProcessesRealSampleSatraFile -v`
Expected: FAIL — Satra isn't routed yet, falls into the dispatch's `default` case.

- [ ] **Step 4: Add the Satra case to `Process`'s dispatch**

Edit `GO/internal/processing/coop_processor.go` — in `Process`'s `switch v {` block (already has `case "Coop":` and `case "Lotte":` from earlier phases), add a new case between them or after `"Lotte"`, following the EXACT same shape as the existing `"Lotte"` case:

```go
		case "Satra":
			row, err := p.processSatraSegment(filePath, text, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Satra",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)
```

Read the actual current file first — do not assume its exact current shape from memory; Phase 2b-1 already restructured this into a switch, so this task ADDS one case to something that already exists, it does not rebuild the switch from scratch.

- [ ] **Step 5: Implement `processSatraSegment`**

Create `GO/internal/processing/satra_processor.go`:

```go
package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/satra"
)

// satraOrderNumber mirrors write_to_dondathang_satra's order-number
// field (xulydonhang.py:2379): f'ĐĐH{vendor}{STT_donhang_str}' where
// vendor is the uppercased literal "SATRA" and STT_donhang_str is
// f"-{po_number}".
func satraOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHSATRA-%s", poNumber)
}

// noSaturdayDeliveryCustomerCode mirrors the one hardcoded special case
// in write_to_dondathang_satra (xulydonhang.py:2371-2372): this specific
// customer code's order description gets a "- Không giao thứ 7" suffix.
const noSaturdayDeliveryCustomerCode = "MN_MT_stph"

// processSatraSegment mirrors the Satra branch of process_file
// (xulydonhang.py:9303-9394) plus write_to_dondathang_satra
// (:2330-2692). Unlike processLotteSegment, this needs NO overrides on
// buildPromoBonusRow/buildInvoiceBonusRow — Satra's promo-matching and
// bonus-row logic (xulydonhang.py:2555-2672) is structurally identical
// to Coop's write_to_dondathang (same khuyenmai.split('|') + enumerate
// loop, same "KM Bó Kèm - Che Barcode" no-brace default), confirmed by
// direct source comparison during planning — so this mirrors
// processSegment's promo loop shape, not processLotteSegment's
// single-call shape.
func (p *RealProcessor) processSatraSegment(filePath, text, pageLabel string) (OrderRow, error) {
	poNumber, ok := satra.ParsePONumber(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được số PO")
	}
	entryDate, ok := satra.ParseEntryDate(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không đọc được ngày đặt hàng")
	}
	cancelDate, _ := satra.ParseCancelDate(text) // best-effort, matches Python's silent-if-missing behavior
	shipTo, _ := satra.ParseShipToAddress(text)  // best-effort

	products := satra.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	customerCode, found := p.Store.GetCustomerCodeByFuzzyAddress("SATRA", shipTo)
	if !found {
		customerCode = "Không xác định"
	}

	priceIndex, err := p.Pricing.FetchIndex("SATRA")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := regionInfo(customerCode)
	description := fmt.Sprintf("SATRA %s", poNumber)
	if customerCode == noSaturdayDeliveryCustomerCode {
		description += " - Không giao thứ 7"
	}

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: satraOrderNumber(poNumber),
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
		qty := rawProduct.Qty
		lineWeight := productInfo.WeightKg * qty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(qty / productInfo.PackSize))
		}
		totalWeight += lineWeight

		invoicePrice := rawProduct.TotalPrice
		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice, _ := strconv.ParseFloat(strings.ReplaceAll(realPriceStr, ",", ""), 64)

		promos := priceIndex.FindPromotions(barcode, entryDate)
		lastExaminedPromo := ""
		matched := false
		finalPrice := realPrice

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
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: satraOrderNumber(poNumber),
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

		// Satra's promo/bonus-row loop DOES split on "|" and DOES use
		// buildPromoBonusRow's own default — no override needed, unlike
		// Lotte (xulydonhang.py:2555 confirms the same khuyenmai.split('|')
		// Coop uses; the AQ-write-every-iteration / i==0-vs-i>0 off-by-one
		// this mirrors is the same quirk documented in detail on Coop's
		// equivalent loop in coop_processor.go).
		currentRowIndex := productRowIndex
		for i, promoPart := range strings.Split(lastExaminedPromo, "|") {
			rows[currentRowIndex].PromoContent = lastExaminedPromo

			bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, promoPart,
				coop.Product{Barcode: barcode, Qty: qty}, i, entryDate, cancelDate, shipTo,
				customerCode, description, warehouse, region, statCode, poNumber)
			if !added {
				continue
			}
			bonusRow.OrderNumber = satraOrderNumber(poNumber) // buildPromoBonusRow hardcodes Coop's order number
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

	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, entryDate, cancelDate,
			shipTo, customerCode, description, warehouse, region, statCode, poNumber); added {
			bonusRow.OrderNumber = satraOrderNumber(poNumber)
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
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Satra", MaKhachHang: customerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
	}, nil
}
```

**Note on the known `AU`-on-invoice-bonus-row discrepancy** (see this plan's Global Constraints): the `buildInvoiceBonusRow` call above uses the shared, already-correct helper. Do not "fix" it to replicate Python's stale-variable bug (`xulydonhang.py:2664`) — if Task 7's golden-fixture run flags this specific mismatch, document it via `knownDivergences_Satra`, do not modify `buildInvoiceBonusRow`.

- [ ] **Step 6: Fill in the real PO number from Step 1, run tests, verify they pass**

Go back to Step 2's test and replace the placeholder with the real value. Run: `cd GO && go build ./... && go vet ./... && go test ./internal/processing/... -v`
Expected: PASS — the new Satra test, and every existing Coop/Lotte test unchanged (Coop's golden fixture test still at its documented baseline, Lotte's still 60/60).

- [ ] **Step 7: Commit**

```bash
git add GO/internal/processing/coop_processor.go GO/internal/processing/satra_processor.go GO/internal/processing/satra_processor_test.go GO/internal/processing/testdata/sample_satra_order.pdf
git commit -m "feat(go): dispatch RealProcessor to Satra via processSatraSegment"
```

---

### Task 6: Golden fixture generation script (throwaway) — generate 33 Satra fixtures

**Files:**
- Create: `GO/internal/processing/satra/testdata/generate_fixtures.py` (throwaway dev tool, adapted from the already-proven `GO/internal/processing/lotte/testdata/generate_fixtures.py`)

**Interfaces:**
- Consumes: the real `xulydonhang.py`'s `ProcessHandler.laymakhachhang_satra`, `trichxuatsanpham_satra`, `write_to_dondathang_satra`, `identify_vendor`, `find_price_by_sku`, `find_all_promotions_by_sku_and_time`, `get_gid` — all unmodified.
- Produces: `GO/internal/processing/satra/testdata/fixtures/*.json` + `_frozen_pricing.json`, same shape established in Phase 2b-1. Consumed by Task 7.

- [ ] **Step 1: Write the script**

Create `GO/internal/processing/satra/testdata/generate_fixtures.py`, adapted directly from `GO/internal/processing/lotte/testdata/generate_fixtures.py` — same `REPO_ROOT` resolution (6 `dirname()` calls — same directory depth: `GO/internal/processing/satra/testdata/generate_fixtures.py`), same UTF-8 stdout fix, same production-`dondathang.xlsx` backup/restore protocol, same price/promo caching monkeypatch (already generic over `sheet_name`, works for `"SATRA"` with no changes). Only `is_satra_pdf`/`process_one_pdf` are Satra-specific:

```python
def is_satra_pdf(path):
    doc = xulydonhang.fitz.open(path)
    try:
        text = ""
        for page in doc:
            text += page.get_text("text")
        return xulydonhang.ProcessHandler.identify_vendor(text) == "Satra"
    finally:
        doc.close()


def process_one_pdf(path):
    """Mirrors the Satra branch of process_file (xulydonhang.py:9303-9394)
    for every page identify_vendor recognizes as Satra, skipping the
    Google Drive upload / current-page-extraction side effects."""
    handler = xulydonhang.process_handler
    doc = xulydonhang.fitz.open(path)
    try:
        for page_num in range(len(doc)):
            text = doc[page_num].get_text("text")
            if xulydonhang.ProcessHandler.identify_vendor(text) != "Satra":
                continue

            po_number = xulydonhang.re.search(r"\*P-[^*]+\*", text)
            po_number = po_number.group(0)[1:-1]

            makhachhang = None
            match = xulydonhang.re.search(r"Địa chỉ giao hàng:\s*((?:.*\n)+?)Địa chỉ thanh toán:", text)
            diachi = ""
            if match:
                diachi = match.group(1).strip().replace("\n", " ").replace("  ", " ")
                makhachhang = xulydonhang.ProcessHandler.laymakhachhang_satra(diachi, "SATRA")

            entry_date = xulydonhang.re.search(r"(.*?)\nNgày đặt hàng:", text, xulydonhang.re.DOTALL)
            if entry_date:
                entry_date = entry_date.group(1).split("\n")[-1]
                entry_date = xulydonhang.datetime.strptime(entry_date, "%m/%d/%Y")
                entry_date = entry_date.strftime("%d/%m/%Y")
                if entry_date == "01/01/0001":
                    entry_date = xulydonhang.re.search(r"(.*?)\nNgày in:", text, xulydonhang.re.DOTALL)
                    if entry_date:
                        entry_date = entry_date.group(1).split("\n")[-1]
                        entry_date = xulydonhang.datetime.strptime(entry_date, "%m/%d/%Y")
                        entry_date = entry_date.strftime("%d/%m/%Y")

            cancel_date = xulydonhang.re.search(r"Ngày giao hàng:\s*(.*?)\s*Địa chỉ giao hàng:", text, xulydonhang.re.DOTALL)
            if cancel_date:
                cancel_date = cancel_date.group(1).strip()
                pattern = r"(\d{1,2}/\d{1,2}/\d{4})"
                for line in cancel_date.split("\n"):
                    if xulydonhang.re.search(pattern, line):
                        cancel_date = line
                        cancel_date = xulydonhang.datetime.strptime(cancel_date, "%m/%d/%Y")
                        cancel_date = cancel_date.strftime("%d/%m/%Y")
                        break

            product_text = xulydonhang.re.search(r"STT\s*(.*?)\s*Hàng phục vụ cho:", text, xulydonhang.re.DOTALL)
            product_text = product_text.group(1).strip()
            products = xulydonhang.ProcessHandler.trichxuatsanpham_satra(product_text)
            if products:
                sku_mapping = xulydonhang.ProcessHandler.load_sku_mapping()
                products = xulydonhang.ProcessHandler.replace_sku_numbers(products, sku_mapping)

            xulydonhang.ProcessHandler.write_to_dondathang_satra(
                handler, products, makhachhang, po_number, entry_date, cancel_date,
                1, "Satra", diachi, None,
            )
    finally:
        doc.close()
```

Everything else (`main`, `snapshot_rows`, `COLUMNS`, the pricing-cache monkeypatch, the backup/restore protocol) is copied verbatim from the Lotte harness, changing only: `FIXTURES_DIR` → `.../satra/testdata/fixtures`, `is_lotte_pdf`/`process_one_pdf` → the Satra versions above, and the final frozen-pricing capture call from `_capture_promo_raw_rows("LOTTE")` to `_capture_promo_raw_rows("SATRA")`.

- [ ] **Step 2: Back up the production workbook before running (safety)**

```bash
cp dondathang.xlsx dondathang.xlsx.manual_backup_before_satra_fixtures
```

- [ ] **Step 3: Run the script**

```bash
.venv/Scripts/python.exe GO/internal/processing/satra/testdata/generate_fixtures.py
```

Expected: "Found N candidate PDFs" (N is whatever the current total in `đơn hàng/08-2026/` is — do not assume it's still 308, more may have been added since), then one `OK`/`SKIP` line per Satra file, ending with "Done: 33 fixtures generated, 0 PDFs skipped" (33 is the count established during this plan's spec — if it differs, that's fine as long as every non-generated file has a clear SKIP reason, investigate before proceeding if any file silently produces neither).

- [ ] **Step 4: Verify the production workbook is untouched**

```bash
diff dondathang.xlsx dondathang.xlsx.manual_backup_before_satra_fixtures && echo "IDENTICAL — safe" || echo "DIFFERS — investigate before proceeding, do not continue"
```

If it differs: STOP, restore immediately (`mv dondathang.xlsx.manual_backup_before_satra_fixtures dondathang.xlsx`), investigate before doing anything else.

- [ ] **Step 5: Remove the manual backup once confirmed identical**

```bash
rm dondathang.xlsx.manual_backup_before_satra_fixtures
```

- [ ] **Step 6: Spot-check a few generated fixtures**

Read 2-3 files under `GO/internal/processing/satra/testdata/fixtures/*.json` and confirm plausible values (PO-shaped `B` column like `ĐĐHSATRA-P-...`, non-empty `S` product names, sane `X`/`Y`/`Z`).

- [ ] **Step 7: Commit**

```bash
git add GO/internal/processing/satra/testdata/generate_fixtures.py GO/internal/processing/satra/testdata/fixtures/
git commit -m "test(go): generate golden fixtures for Satra from real PDFs + production output"
```

---

### Task 7: Golden fixture integration test

**Files:**
- Create: `GO/internal/processing/satra_golden_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1-6; reuses `fixtureData`, `frozenPricingFixture`, `fixturePricingSource`, `compareRowsAgainstFixture` (with the fixture-scoped `allowedDivergences` parameter, already fixed in Phase 2b-1's final review), `stringify`, `toFloat`, `floatCloseEnough`, `copyFile`, `joinLines` — all already defined in the `processing` package.
- Produces: `TestRealProcessor_MatchesGoldenFixtures_Satra`.

- [ ] **Step 1: Write `satra_golden_test.go`**

Create `GO/internal/processing/satra_golden_test.go`, following `lotte_golden_test.go`'s exact structure (same package `processing`, same imports):

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

// knownDivergences_Satra lists (fixture, row index, column) cells where
// this Go port intentionally computes a different, verified-more-correct
// value than the frozen Python fixture. Key format:
// "<source_pdf>:<row index>:<column>", e.g.
// "P-005508192.pdf:3:AU" — see this plan's Global Constraints for the
// specific, already-anticipated AU/invoice-bonus-row case this may be
// needed for. Empty until a real, hand-verified case is confirmed; add
// entries here only with a comment citing the specific PDF/Python-line
// evidence — never to silence an unexplained diff.
var knownDivergences_Satra = map[string]bool{}

func loadFrozenSatraPricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("satra/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen Satra pricing fixture found (run Task 6's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen Satra pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

func TestRealProcessor_MatchesGoldenFixtures_Satra(t *testing.T) {
	fixturePaths, err := filepath.Glob("satra/testdata/fixtures/*.json")
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
		t.Skip("no golden fixtures found (run Task 6's generate_fixtures.py first)")
	}

	store, err := productdata.Load("../../../data.xlsx")
	if err != nil {
		t.Fatalf("failed loading production data.xlsx: %v", err)
	}
	pricingSource := loadFrozenSatraPricingSource(t)

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

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_Satra)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
}
```

**Note:** `compareRowsAgainstFixture`'s signature already takes `allowedDivergences map[string]bool` as its final parameter (fixed during Phase 2b-1's final whole-branch review) — if the implementer finds it does NOT (i.e. still takes only 4 params), that means this plan is somehow running against a checkout that predates that fix, which should not happen; stop and report this as a blocking inconsistency rather than guessing.

- [ ] **Step 2: Run the test**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures" -v`

Expected: Coop's and Lotte's tests still report their exact unchanged baselines. Satra's test will very likely fail on the first run with real mismatches — this is the actual verification work of this task, same as it was for Lotte's Task 9.

- [ ] **Step 3: Root-cause and fix every mismatch**

For each mismatch: read the specific fixture JSON and the source PDF, trace through `xulydonhang.py`'s actual Satra functions at the cited line numbers, and determine whether it's (a) a bug in this plan's Go port — fix the Go code; or (b) a case where Python is genuinely wrong (the `AU`-on-invoice-bonus-row case flagged in this plan's Global Constraints is the specific, already-anticipated candidate — but verify it actually occurs in a real fixture before assuming it's the cause of any given mismatch) — add a precise, evidence-citing entry to `knownDivergences_Satra` using the `sourcePDF:row:col` key format. Do not guess; every fix or allowlist entry must be traceable to specific evidence. Re-run after each fix.

If some failures turn out to be PDF-text-extraction-fidelity gaps (the same category of limitation Phase 2a's Coop plan and Phase 2b-1's Lotte plan both allowed for, though Lotte ultimately needed none), document them the same way.

- [ ] **Step 4: Final run and commit**

Run: `cd GO && go build ./... && go vet ./... && go test ./... -v`
Expected: clean build/vet, all tests pass (or fail only with fully documented, understood, non-logic-bug gaps).

```bash
git add GO/internal/processing/satra_golden_test.go
git commit -m "test(go): add Satra golden fixture integration test"
```
