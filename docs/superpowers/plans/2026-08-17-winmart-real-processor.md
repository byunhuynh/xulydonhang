# Winmart RealProcessor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the existing Go `RealProcessor` (Coop, Lotte, Satra, BigC already shipped) to also parse real Winmart purchase-order PDFs — architecturally the simplest new vendor since Satra (per-page dispatch, no BigC-style whole-document state), but with 2 genuinely novel quirks (a zero-price "giao rời" row-skip that reads/marks a *previous* row, and an invoice-level bonus-row builder that can't reuse the shared helper because of a real Q-column shape difference) — validated against 16 real archived Winmart PDFs via the same golden-fixture methodology.

**Architecture:** New `GO/internal/processing/winmart/` package holds pure extraction functions (PO/dates/two-distinct-addresses, product list). `winmart_processor.go` adds `processWinmartSegment`, dispatched per-page exactly like `processSatraSegment`/`processLotteSegment` (no whole-document pre-check — Winmart's PDF structure is confirmed "1 page = 1 order", same family as Coop/Lotte/Satra). Customer-code lookup reuses `productdata.Store.GetCustomerCodeByFuzzyAddress("WINMART", ...)` **unmodified** — the exact same Go function Satra's phase already built, since Winmart's Python calls the identical `laymakhachhang_satra` function Satra does. The per-item promo bonus-row block is confirmed structurally identical to `buildPromoBonusRow`'s `index==0` case and is reused **as-is**; the invoice-level ("Hóa Đơn") bonus block needs its own builder (like BigC's) because it writes only the *first* matched SKU to column Q, not the shared helper's comma-joined list. A new `winmartRegionInfo` function (not an extension of the shared `regionInfo`) mirrors Winmart's own 3-way branch, including one exact-match special case (`MN_MT_WIN1326`).

**Tech Stack:** Same as Phase 2a/2b — Go, `github.com/xuri/excelize/v2`, `github.com/ledongthuc/pdf`. No new external dependencies.

**Spec:** [2026-08-17-winmart-real-processor-design.md](../specs/2026-08-17-winmart-real-processor-design.md)

## Global Constraints

- **Testing/divergence policy** (same as Lotte/Satra/BigC, different from Coop): golden-fixture tests compare against real Python output; intentional Go/Python divergences go in a `knownDivergences_Winmart` allowlist with `sourcePDF:rowIndex:column` keys and evidence citations — never force a fixture to pass by editing it.
- **Zero-price "giao rời" skip is a deliberate, confirmed Go-side improvement over Python, not a port.** Python's condition (`xulydonhang.py:4299`, `if dongia == 0 and current_row - 2 >= 9`) checks the **absolute** sheet row number, not a position relative to the current order — in a real production sheet (always far more than 9 rows deep from prior orders), this means a zero-price item that is the *first or second* product of its own order would read/overwrite a **previous, unrelated order's** `AO`/`AP` cells. Go's design: only mark `AO`/`AP` on a row that belongs to the **current order's own accumulated rows**; if there is no such prior row (the zero-price item is the first or second product processed for this order), skip the item cleanly instead of reaching outside the current order's row set. This is a real, confirmed behavior difference — implement the safe version, do not "fix" it to match Python's cross-order-reaching behavior.
- **Reuse, do not reimplement**: `productdata.Store.GetCustomerCodeByFuzzyAddress` (Satra's fuzzy-match function, called with `"WINMART"` — zero new Go code needed for customer-code lookup), `productdata.Store.ResolveSku/GetProductInfo/FindSkusMentioned`, `excelwriter.Row`/`WriteOrderRows`, `pricing.Index`/`PricingSource.FetchIndex`, `coop.ExtractDiscount/ExtractBraceContent/ExtractMoneyAmount/LastFourDigits/FormatWeightKg`, and (from `processor_shared.go`) `coopDebtDays`, `closeEnough`, `xPlus1Pattern`, **`buildPromoBonusRow`** (confirmed reusable as-is for Winmart's per-item bonus row, always called with `index=0` — Winmart has no multi-CTKM-per-item loop, so there is never an `index>0` case). Do **NOT** reuse `buildInvoiceBonusRow` for Winmart's invoice-level bonus row — confirmed structurally different (writes only the first matched SKU to column Q, not a joined list; see Task 4). Do **NOT** extend the shared `regionInfo` for Winmart's warehouse lookup — Winmart's own 3-way branch uses `LA_KHO2026` for its plain-MN case (not `LA_TP`, what shared `regionInfo` returns) and has the `MN_MT_WIN1326` exact-match override, neither of which the shared function supports; a `winmartRegionInfo` function is required (same reasoning that produced BigC's `bigcRegionInfo`).
- Every exported function gets a doc comment citing the exact `xulydonhang.py` line range it mirrors. Every deviation from a literal Python behavior gets an inline comment explaining why.
- Run `go build ./...`, `go vet ./...`, and the relevant `go test` scope after every task, from the `GO/` directory.
- **New package** `GO/internal/processing/winmart/` for Winmart-only extraction, mirroring the `lotte`/`satra`/`bigc` package shape. **New file** `GO/internal/processing/winmart_processor.go` (+ `winmart_processor_test.go`) — never append to `coop_processor.go`/`lotte_processor.go`/`satra_processor.go`/`bigc_processor.go`.
- **A known, not-yet-confirmed Python quirk to watch for during Task 6 (golden fixtures):** the per-item promo bonus-row block (`xulydonhang.py:4477`) calls `ProcessHandler.laycachbo_khuyenmai(value)` using `value` — the loop variable left over from the earlier `for col, value in results:` price-matching loop (`:4375`) — **not** `khuyenmai` (the variable actually written to column `AQ` at `:4463` and used for the `kiemtra`/`X+1` check at `:4462`/`:4464`). This is the same class of Python loop-variable-leak quirk already documented (and NOT ported) for BigC's equivalent block. `buildPromoBonusRow` (the shared helper this task reuses for Winmart's per-item bonus row) already takes the caller's promo string as an explicit parameter — since Winmart has only one promo attempt per item (no multi-CTKM loop), `khuyenmai` and the leaked `value` coincide in the overwhelming majority of real cases (whenever a match was found, or every attempted `value` was truthy) and only diverge in a narrow edge case (multiple `results` entries where a later one's `value` is empty after an earlier one already set `khuyenmai`). This plan's Task 4 passes `khuyenmai` (the sensible, current-item value) to `buildPromoBonusRow`, not the leaked `value` — if Task 6's golden-fixture run shows an `AO` mismatch traceable to this specific pattern, document it via `knownDivergences_Winmart`, don't treat it as a Go bug to chase.

---

### Task 1: `vendor.Identify` — recognize Winmart, appended after Satra

**Files:**
- Modify: `GO/internal/processing/vendor/identify.go`
- Modify: `GO/internal/processing/vendor/identify_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `Identify(text string) string` now also returns `"Winmart"` — consumed by Task 4's dispatch.

- [ ] **Step 1: Write the failing test**

Add to `GO/internal/processing/vendor/identify_test.go`:

```go
func TestIdentify_RecognizesWinmartBySupplierCode(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"supplier code present", "Header\nNhà cung cấp (Supplier): 0002011398\nfooter", "Winmart"},
		{"unrelated supplier code", "Nhà cung cấp (Supplier): 9999999999", ""},
		{"no marker at all", "nothing relevant here", ""},
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

func TestIdentify_WinmartCheckedAfterSatra(t *testing.T) {
	// Python's real identify_vendor order (xulydonhang.py:90-179) is
	// Coop -> BigC -> Lotte -> Satra -> Satra(2nd form) -> Emart ->
	// Kingfood -> CN-HCM -> Winmart -> SHOPEE-CHOICE -> ... Since Emart/
	// Kingfood/CN-HCM are not yet ported to Go, Winmart's case only needs
	// to be appended after Satra's (not inserted mid-sequence like BigC
	// was) to preserve the correct relative order among vendors that
	// actually exist in Go today. This test doesn't have a genuine
	// ordering conflict to construct (no unported vendor's pattern is
	// available), so it documents the intent for a future reader.
	got := Identify("Nhà cung cấp (Supplier): 0002011398")
	if got != "Winmart" {
		t.Fatalf("Identify with Winmart marker = %q, want %q", got, "Winmart")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/vendor/... -run "TestIdentify_RecognizesWinmart|TestIdentify_WinmartCheckedAfterSatra" -v`
Expected: FAIL — `Identify` returns `""` for the Winmart cases.

- [ ] **Step 3: Implement**

Read `GO/internal/processing/vendor/identify.go` first to confirm its current exact shape (Coop → BigC → Lotte → Satra) before editing.

Add a `winmartPattern` var alongside the existing pattern vars:

```go
	// Winmart's identify pattern (xulydonhang.py:121-122): a single
	// literal regex against the whitespace-normalized page text, no
	// alternation, no case-insensitivity flag in Python (the supplier
	// code string itself is fixed, so case sensitivity is moot).
	winmartPattern = regexp.MustCompile(`Nhà cung cấp \(Supplier\): 0002011398`)
```

In `Identify`, append the Winmart check **after** the Satra check (Python's real position — after CN-HCM, which is unimplemented — collapses to "after Satra" for the vendors Go currently has):

```go
	if winmartPattern.MatchString(cleaned) {
		return "Winmart"
	}
```

Update `Identify`'s doc comment to mention Coop, BigC, Lotte, Satra, and Winmart are implemented, in that order, and to note (as the existing comment already does for prior additions) that Python's real full order has several more vendors between Satra and Winmart (Emart, Kingfood, CN-HCM) that aren't ported yet — a future implementer adding one of those must insert it at the correct relative position, not simply append.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/vendor/... -v`
Expected: PASS — all Coop/BigC/Lotte/Satra tests still pass (regression check), all new Winmart tests pass.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/vendor/identify.go GO/internal/processing/vendor/identify_test.go
git commit -m "feat(go): recognize Winmart vendor in identify.Identify, appended after Satra"
```

---

### Task 2: `winmart` package — PO number, dates, two distinct addresses

**Files:**
- Create: `GO/internal/processing/winmart/extract.go`
- Create: `GO/internal/processing/winmart/extract_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `winmart.ParseOrderInfo(pageText string) (poNumber, entryDate, cancelDate, note string, ok bool)`; `winmart.ParseDeliveryAddress(pageText string) (string, bool)`; `winmart.ParseFuzzyMatchAddress(pageText string) (string, bool)`. Consumed by Task 4.

- [ ] **Step 1: Write the failing tests**

Create `GO/internal/processing/winmart/extract_test.go`:

```go
package winmart

import "testing"

func TestParseOrderInfo_ExtractsPONumberDatesAndNote(t *testing.T) {
	text := "header\n" +
		"Ngày đặt hàng (PO date)\n07.31.2026\n" +
		"Số đơn hàng (PO No.)\n4194002858\n" +
		"Ngày giao (Delivery Date)\n08.08.2026\n" +
		"Ghi chú\nNguyễn Quang Phi_0396035541\nNhà cung cấp (Supplier): 0002011398\nfooter"
	po, entry, cancel, note, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo: no match, want match")
	}
	if po != "4194002858" {
		t.Fatalf("po = %q, want %q", po, "4194002858")
	}
	if entry != "07/31/2026" {
		t.Fatalf("entry = %q, want %q (dots converted to slashes, no reordering)", entry, "07/31/2026")
	}
	if cancel != "08/08/2026" {
		t.Fatalf("cancel = %q, want %q", cancel, "08/08/2026")
	}
	if note != "Nguyễn Quang Phi_0396035541" {
		t.Fatalf("note = %q, want %q", note, "Nguyễn Quang Phi_0396035541")
	}
}

func TestParseOrderInfo_MissingPOMarkerReturnsFalse(t *testing.T) {
	_, _, _, _, ok := ParseOrderInfo("no PO marker anywhere in this text")
	if ok {
		t.Fatal("ParseOrderInfo: matched, want no match")
	}
}

func TestParseOrderInfo_NoteWithMultipleLinesIsJoinedWithSpaces(t *testing.T) {
	text := "Ngày đặt hàng (PO date)\n07.31.2026\n" +
		"Số đơn hàng (PO No.)\n4194002858\n" +
		"Ngày giao (Delivery Date)\n08.08.2026\n" +
		"Ghi chú\nline one\nline two\nNhà cung cấp (Supplier): 0002011398\n"
	_, _, _, note, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo: no match, want match")
	}
	if note != "line one line two" {
		t.Fatalf("note = %q, want %q", note, "line one line two")
	}
}

func TestParseDeliveryAddress_JoinsLinesFilteringWMPlusDuplicates(t *testing.T) {
	text := "header\n" +
		"Địa chỉ giao hàng (Delivery Address)\n" +
		"1357-WMT_AMBIENT_HAIPHONG\n" +
		"1357 - WMT_AMBIENT_HAIPHONG Lô CN4.1L\n" +
		"Khu Công Nghiệp Đình Vũ\n" +
		"Thông tin đơn hàng (Information)\n" +
		"footer"
	got, ok := ParseDeliveryAddress(text)
	if !ok {
		t.Fatal("ParseDeliveryAddress: no match, want match")
	}
	// The 2nd line contains "WM+"? No -- it contains "WMT_AMBIENT" not
	// "WM+". Use a case that actually has the literal "WM+" duplicate
	// marker the real Python filters on, per xulydonhang.py:9032's
	// comment example ("6863 - WM+ HCM 60 Liên khu 10-11"):
	want := "1357-WMT_AMBIENT_HAIPHONG - 1357-WMT_AMBIENT_HAIPHONG Lô CN4.1L Khu Công Nghiệp Đình Vũ"
	if got != want {
		t.Fatalf("ParseDeliveryAddress = %q, want %q", got, want)
	}
}

func TestParseDeliveryAddress_FiltersWMPlusDuplicateLines(t *testing.T) {
	text := "Địa chỉ giao hàng (Delivery Address)\n" +
		"6863\n" +
		"Real address line one\n" +
		"6863 - WM+ HCM 60 Liên khu 10-11\n" +
		"Real address line two\n" +
		"Thông tin đơn hàng (Information)\n"
	got, ok := ParseDeliveryAddress(text)
	if !ok {
		t.Fatal("ParseDeliveryAddress: no match, want match")
	}
	want := "6863 - Real address line one Real address line two"
	if got != want {
		t.Fatalf("ParseDeliveryAddress = %q, want %q (the WM+ line must be filtered out)", got, want)
	}
}

func TestParseDeliveryAddress_NoMarkerReturnsFalse(t *testing.T) {
	if _, ok := ParseDeliveryAddress("no delivery address marker here"); ok {
		t.Fatal("ParseDeliveryAddress: matched, want no match")
	}
}

func TestParseFuzzyMatchAddress_FindsBlockAfterWincommerceMarker(t *testing.T) {
	text := "header\n" +
		"TỔNG HỢP\n" +
		"WINCOMMERCE\n" +
		"Khu trung tâm thương mại Vincom Lê Thánh Tông\n" +
		"Số 5 Đường Lê Thánh Tông\n" +
		"MST: 0100109106\n" +
		"footer"
	got, ok := ParseFuzzyMatchAddress(text)
	if !ok {
		t.Fatal("ParseFuzzyMatchAddress: no match, want match")
	}
	want := "Khu trung tâm thương mại Vincom Lê Thánh Tông Số 5 Đường Lê Thánh Tông"
	if got != want {
		t.Fatalf("ParseFuzzyMatchAddress = %q, want %q", got, want)
	}
}

func TestParseFuzzyMatchAddress_WincommerceAloneOnOneLineAlsoMatches(t *testing.T) {
	text := "header\n" +
		"Some Wincommerce Branch Line\n" +
		"Address line one\n" +
		"Address line two\n" +
		"Địa chỉ giao hàng: somewhere\n"
	got, ok := ParseFuzzyMatchAddress(text)
	if !ok {
		t.Fatal("ParseFuzzyMatchAddress: no match, want match")
	}
	want := "Address line one Address line two"
	if got != want {
		t.Fatalf("ParseFuzzyMatchAddress = %q, want %q", got, want)
	}
}

func TestParseFuzzyMatchAddress_StopsAtMSTOrDiaChiGiaoHangCaseInsensitive(t *testing.T) {
	text := "wincommerce\n" +
		"line a\n" +
		"line b\n" +
		"Địa Chỉ Giao Hàng: ignored from here\n" +
		"line c\n"
	got, ok := ParseFuzzyMatchAddress(text)
	if !ok {
		t.Fatal("ParseFuzzyMatchAddress: no match, want match")
	}
	want := "line a line b"
	if got != want {
		t.Fatalf("ParseFuzzyMatchAddress = %q, want %q (must stop before the case-insensitive marker)", got, want)
	}
}

func TestParseFuzzyMatchAddress_NoMarkerReturnsFalse(t *testing.T) {
	if _, ok := ParseFuzzyMatchAddress("no wincommerce marker here"); ok {
		t.Fatal("ParseFuzzyMatchAddress: matched, want no match")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/winmart/... -v`
Expected: FAIL — package doesn't exist yet / functions undefined.

- [ ] **Step 3: Implement**

Create `GO/internal/processing/winmart/extract.go`:

```go
package winmart

import "strings"

// ParseOrderInfo mirrors the PO-number/date/note extraction inline in
// process_file's Winmart branch (xulydonhang.py:8989-9004ish — line-scan
// logic, not a regex function). Each of "Ngày đặt hàng (PO date)", "Số
// đơn hàng (PO No.)", and "Ngày giao (Delivery Date)" is a marker line;
// the value is the LINE IMMEDIATELY AFTER it. Dates are returned with
// "." replaced by "/" (Python's raw `.replace('.', '/')` — no reordering
// of month/day/year components, a literal character substitution only).
// ok=false only when the PO-number marker/line isn't found — mirrors
// Python's real crash-on-None behavior (a missing PO number makes
// several downstream string operations fail) with a clean failure
// instead, per this codebase's established error-handling policy.
//
// note mirrors `ghichu` (xulydonhang.py:8994-9000): the text between the
// literal "Ghi chú" marker and the literal supplier-ID string "Nhà cung
// cấp (Supplier): 0002011398", with the LAST line of that block dropped
// (Python's `.splitlines()[:-1]`) and the rest joined with a single
// space (Python's `.replace('\n', ' ')` after a `"\n".join(...)` —
// equivalent to joining with spaces directly). Returns "" (not a failure)
// if the "Ghi chú"/supplier-ID markers aren't found — Python's real code
// would raise an IndexError in that case (`text.split("Ghi chú")[1]` on
// a list of length 1); a missing note is not itself fatal in Go's port.
func ParseOrderInfo(text string) (poNumber, entryDate, cancelDate, note string, ok bool) {
	lines := strings.Split(text, "\n")

	entryDate, entryOk := lineAfterMarker(lines, "Ngày đặt hàng (PO date)")
	if entryOk {
		entryDate = strings.ReplaceAll(entryDate, ".", "/")
	}

	poNumber, poOk := lineAfterMarker(lines, "Số đơn hàng (PO No.)")
	if !poOk {
		return "", "", "", "", false
	}

	cancelDate, cancelOk := lineAfterMarker(lines, "Ngày giao (Delivery Date)")
	if cancelOk {
		cancelDate = strings.ReplaceAll(cancelDate, ".", "/")
	}

	note = parseNote(text)

	return poNumber, entryDate, cancelDate, note, true
}

// lineAfterMarker mirrors the repeated
// "idx = next(...); value = lines[idx+1].strip() if idx != -1 ... else None"
// pattern used for entry_date/po_number/cancel_date (xulydonhang.py:8989-9008).
func lineAfterMarker(lines []string, marker string) (string, bool) {
	for i, line := range lines {
		if strings.Contains(line, marker) {
			if i+1 < len(lines) {
				return strings.TrimSpace(lines[i+1]), true
			}
			return "", false
		}
	}
	return "", false
}

const supplierIDMarker = "Nhà cung cấp (Supplier): 0002011398"

// parseNote mirrors xulydonhang.py:8994-9000 exactly.
func parseNote(text string) string {
	parts := strings.SplitN(text, "Ghi chú", 2)
	if len(parts) != 2 {
		return ""
	}
	block := strings.SplitN(parts[1], supplierIDMarker, 2)[0]
	block = strings.TrimSpace(block)
	lines := strings.Split(block, "\n")
	if len(lines) == 0 {
		return ""
	}
	lines = lines[:len(lines)-1] // Python's .splitlines()[:-1] — drop the last line
	return strings.Join(lines, " ")
}

const (
	deliveryAddressMarker = "Địa chỉ giao hàng (Delivery Address)"
	deliveryAddressStop   = "Thông tin đơn hàng (Information)"
)

// ParseDeliveryAddress mirrors xulydonhang.py:9013-9041 (the
// diachigiaohang block, written to Excel column E — NOT the same as
// ParseFuzzyMatchAddress below, a genuinely separate scan over the same
// page text). The line immediately after the marker is a warehouse code
// ("ma_kho"); subsequent lines up to (not including) the stop marker are
// joined with " ", skipping any line containing the literal substring
// "WM+" (a duplicate-line artifact in the real PDF template,
// xulydonhang.py:9031-9033's comment gives the example
// "6863 - WM+ HCM 60 Liên khu 10-11"). Final result is
// "<ma_kho> - <joined address lines>". Returns ("", false) if the marker
// isn't found — mirrors Python's diachigiaohang staying None.
func ParseDeliveryAddress(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	idx := -1
	for i, line := range lines {
		if strings.Contains(line, deliveryAddressMarker) {
			idx = i
			break
		}
	}
	if idx == -1 || idx+1 >= len(lines) {
		return "", false
	}
	maKho := strings.TrimSpace(lines[idx+1])

	var addressLines []string
	for _, line := range lines[idx+2:] {
		if strings.Contains(line, deliveryAddressStop) {
			break
		}
		line = strings.TrimSpace(line)
		if strings.Contains(line, "WM+") {
			continue
		}
		if line != "" {
			addressLines = append(addressLines, line)
		}
	}
	return maKho + " - " + strings.Join(addressLines, " "), true
}

// ParseFuzzyMatchAddress mirrors xulydonhang.py:9062-9087 — a
// SEPARATE scan from ParseDeliveryAddress over the same page text,
// producing the address string used ONLY as fuzzy-match input to
// productdata.Store.GetCustomerCodeByFuzzyAddress("WINMART", ...) — this
// value is never written to any Excel column directly. Anchors on
// either two consecutive lines where the first contains "tổng hợp"
// (case-insensitive) and the second contains "wincommerce", OR a single
// line containing "wincommerce" alone — whichever is found first
// scanning top to bottom. From the line after the anchor, collects
// lines until (not including) the first line containing "mst" or
// "địa chỉ giao hàng" (both checked case-insensitively, matching
// Python's `.lower()` comparisons), joined with " ". Returns ("", false)
// if no anchor is found — mirrors Python's diachi staying None.
func ParseFuzzyMatchAddress(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	idx := -1
	for i := 0; i < len(lines)-1; i++ {
		lineLower := strings.ToLower(lines[i])
		nextLower := strings.ToLower(lines[i+1])
		if strings.Contains(lineLower, "tổng hợp") && strings.Contains(nextLower, "wincommerce") {
			idx = i
			break
		}
		if strings.Contains(lineLower, "wincommerce") {
			idx = i
			break
		}
	}
	if idx == -1 {
		return "", false
	}

	var collected []string
	for _, line := range lines[idx+1:] {
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, "mst") || strings.Contains(lineLower, "địa chỉ giao hàng") {
			break
		}
		collected = append(collected, strings.TrimSpace(line))
	}
	return strings.Join(collected, " "), true
}
```

**Note for the implementer:** verify `TestParseDeliveryAddress_JoinsLinesFilteringWMPlusDuplicates`'s expected output by hand-tracing the function against the test's literal input before assuming it's correct — the brief's test text was constructed to demonstrate the join behavior, not copied from a real fixture; adjust the TEST TEXT (not the function, a direct transcription of the confirmed Python logic) if the exact spacing doesn't match on first run.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/winmart/... -v`
Expected: PASS — all tests.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/winmart/extract.go GO/internal/processing/winmart/extract_test.go
git commit -m "feat(go): add winmart package with PO/date/note extraction and two address parsers"
```

---

### Task 3: `winmart` package — product extraction

**Files:**
- Modify: `GO/internal/processing/winmart/extract.go`
- Modify: `GO/internal/processing/winmart/extract_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `winmart.Product{Barcode, OUQty, TotalPrice string}`; `winmart.ExtractProducts(pageText string) []Product`. Consumed by Task 4.

- [ ] **Step 1: Write the failing tests**

Add to `GO/internal/processing/winmart/extract_test.go`:

```go
func TestExtractProducts_ParsesSevenFieldBlocks(t *testing.T) {
	// Shape mirrors trichxuatsanpham_winmart's expectation: STT, article
	// code, barcode, qty, unit code (2-4 uppercase letters/digits), unit
	// price, amount -- each on its own line.
	text := "1\n" +
		"100234\n" +
		"8936156731203\n" +
		"4\n" +
		"CS\n" +
		"162,272\n" +
		"649,088\n" +
		"2\n" +
		"100567\n" +
		"8936156732767\n" +
		"12\n" +
		"PC\n" +
		"71,600\n" +
		"859,200\n"
	got := ExtractProducts(text)
	want := []Product{
		{Barcode: "8936156731203", OUQty: "4", TotalPrice: "649088"},
		{Barcode: "8936156732767", OUQty: "12", TotalPrice: "859200"},
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

func TestExtractProducts_CollapsesRunsOfSpacesAndTabsFirst(t *testing.T) {
	// Python does re.sub(r"[ \t]+", " ", text) before matching -- confirm
	// the Go port does the same (this does NOT collapse newlines, only
	// horizontal whitespace runs within a line).
	text := "1\n100234\n8936156731203\n4\nCS\n162,272\n649,088\n"
	// Inject extra horizontal whitespace mid-line (should not break the match).
	text = "1  \n100234\t\n8936156731203\n4\nCS\n162,272\n649,088\n"
	got := ExtractProducts(text)
	if len(got) != 1 || got[0].Barcode != "8936156731203" {
		t.Fatalf("ExtractProducts(extra horizontal whitespace) = %+v, want 1 product with barcode 8936156731203", got)
	}
}

func TestExtractProducts_NoMatchesReturnsEmpty(t *testing.T) {
	if got := ExtractProducts("no product-shaped lines here"); len(got) != 0 {
		t.Fatalf("ExtractProducts = %+v, want empty", got)
	}
}

func TestExtractProducts_AcceptsJoinedMultiPageText(t *testing.T) {
	// trichxuatsanpham_winmart accepts either a string or a list of page
	// strings (joined with "\n" first) -- xulydonhang.py:6774-6776. This
	// Go port only takes a single string; callers are responsible for
	// joining multi-page text themselves before calling ExtractProducts
	// (Task 4's processWinmartSegment operates per-page, per this plan's
	// confirmed "1 page = 1 order" architecture, so joining is not
	// actually needed in practice -- this test just confirms the
	// underlying regex has no per-page assumption baked in).
	text := "1\n100234\n8936156731203\n4\nCS\n162,272\n649,088\n" +
		"2\n100567\n8936156732767\n12\nPC\n71,600\n859,200\n"
	got := ExtractProducts(text)
	if len(got) != 2 {
		t.Fatalf("ExtractProducts(joined text) returned %d products, want 2: %+v", len(got), got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/winmart/... -run TestExtractProducts -v`
Expected: FAIL — `Product`/`ExtractProducts` undefined.

- [ ] **Step 3: Implement**

Add to `GO/internal/processing/winmart/extract.go` (append; add `"regexp"` to imports):

```go
// Product is one row of Winmart's product table. Only the 3 fields
// trichxuatsanpham_winmart actually returns (xulydonhang.py:6794-6798) —
// "no" (STT), "article", and "unit_price" are matched by the regex but
// deliberately discarded, matching Python's returned dict shape exactly
// (no per-unit-price field survives extraction; Task 4 derives a unit
// price from TotalPrice/OUQty for Winmart specifically, per the
// giahoadon formula at xulydonhang.py:4347-4351).
type Product struct {
	Barcode    string
	OUQty      string
	TotalPrice string
}

var horizontalWhitespaceRunPattern = regexp.MustCompile(`[ \t]+`)

// productLinePattern mirrors trichxuatsanpham_winmart's re.VERBOSE
// pattern (xulydonhang.py:6779-6789) exactly: 7 fields, each on its own
// line — STT, article code, barcode, quantity, a 2-4 character unit code
// (uppercase letters/digits), unit price, and amount. Go's regexp
// package has no re.VERBOSE mode (which lets Python's pattern span
// multiple source lines with embedded whitespace/comments ignored) — the
// pattern below is the same shape with the VERBOSE-only whitespace
// removed, functionally identical to what Python's compiled pattern
// actually matches against real text.
var productLinePattern = regexp.MustCompile(`(?m)^(\d+)\s*\n(\d+)\s*\n(\d+)\s*\n([\d,]+)\s*\n[A-Z0-9]{2,4}\s*\n([\d,]+)\s*\n([\d,]+)`)

// ExtractProducts mirrors trichxuatsanpham_winmart (xulydonhang.py:6774-6805):
// collapses runs of spaces/tabs to a single space (re.sub(r"[ \t]+", " ", text),
// NOT touching newlines), then extracts every matching 7-field block.
// Group 1 (STT/"no"), group 2 ("article"), and group 5 ("unit_price")
// are matched but discarded — only Barcode (group 3), OUQty (group 4,
// commas stripped), and TotalPrice (group 6, commas stripped) survive
// into the returned Product, exactly matching Python's returned dict
// keys ("Barcode", "OU Qty", "Total Price").
func ExtractProducts(text string) []Product {
	collapsed := horizontalWhitespaceRunPattern.ReplaceAllString(text, " ")

	var products []Product
	for _, m := range productLinePattern.FindAllStringSubmatch(collapsed, -1) {
		products = append(products, Product{
			Barcode:    m[3],
			OUQty:      strings.ReplaceAll(m[4], ",", ""),
			TotalPrice: strings.ReplaceAll(m[6], ",", ""),
		})
	}
	return products
}
```

**Note for the implementer:** the `(?m)^...` anchoring above is this plan's interpretation of how to make a line-by-line 7-field regex work in Go without `re.VERBOSE`'s multi-line pattern authoring — verify this actually matches the test fixture text as constructed (run it, don't assume), and adjust the TEST TEXT first if it doesn't match on the first attempt (the same "test text may need adjustment, not the regex" rule used throughout this whole vendor-porting series applies here too) — Task 6's real 16-PDF fixture run is the ultimate ground truth regardless.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/winmart/... -v`
Expected: PASS — all tests in the package (Task 2's + this task's).

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/winmart/extract.go GO/internal/processing/winmart/extract_test.go
git commit -m "feat(go): add product extraction to winmart package"
```

---

### Task 4: `RealProcessor` — dispatch to Winmart via `processWinmartSegment`

**Files:**
- Modify: `GO/internal/processing/coop_processor.go` (only `Process`'s per-page dispatch switch — add one `case`)
- Modify: `GO/internal/processing/productdata/testdata/data.xlsx` (add one `WINMART` row to the `MaKH` sheet for tests, same pattern Satra's Task 2 used)
- Create: `GO/internal/processing/winmart_processor.go`
- Create: `GO/internal/processing/winmart_processor_test.go`
- Create: `GO/internal/processing/testdata/sample_winmart_order.pdf` (copy of a real file)

**Interfaces:**
- Consumes: `vendor.Identify` (Task 1), `winmart.ParseOrderInfo/ParseDeliveryAddress/ParseFuzzyMatchAddress/ExtractProducts/Product` (Tasks 2-3), `productdata.Store.GetCustomerCodeByFuzzyAddress` (already shipped, Satra's Phase 2b-2), `processor_shared.go`'s `coopDebtDays`/`closeEnough`/`xPlus1Pattern`/`buildPromoBonusRow`.
- Produces: `RealProcessor.Process` now routes Winmart pages to a new `processWinmartSegment` method in `winmart_processor.go`.

- [ ] **Step 1: Copy a real sample file into testdata, add a WINMART test-fixture row**

```bash
cp "đơn hàng/08-2026/4194002858.pdf" GO/internal/processing/testdata/sample_winmart_order.pdf
```

This file's real values (confirmed during planning by running the real Python functions directly against it): 1 page, PO `4194002858`, entry date `07/31/2026`, cancel date `08/08/2026`, note `Nguyễn Quang Phi_0396035541_phinq@winmart.m`, delivery address `1357-WMT_AMBIENT_HAIPHONG - 1357 - WMT_AMBIENT_HAIPHONG Lô CN4.1L, Khu Công Nghiệp Đình Vũ, thuộc Khu Kinh Tế Đình Vũ- Cát Hải, P. Đông Hải 2 TP. Hải Phòng Việt Nam`, fuzzy-match input `Khu trung tâm thương mại Vincom Lê Thánh Tông, Số 5 Đường Lê Thánh Tông, Phường Gia Viên, Thành phố Hải Phòng, Việt Nam`, 2 products (`8936156731203`/qty 4/total 649088, `8936156732767`/qty 12/total 859200) — and, against the REAL production `data.xlsx`, fuzzy-matches to customer code `MB_MT_WIM1336` (starts with `MB`, so region resolves to `MT_MB`/`TP_HN_12`/`HN`).

Add a `WINMART` row to `GO/internal/processing/productdata/testdata/data.xlsx`'s `MaKH` sheet (currently 6 rows: header + COOP/COOPFOOD/LOTTE/SATRA/blank-column-A — confirmed no `WINMART` row exists yet) — column A `WINMART`, column B any store code, column C a customer code like `MN_MT_TESTWIN`, column D a realistic address, e.g. `"789 Trần Hưng Đạo, Phường Cầu Kho, Quận 1, Tp.HCM, VNM"`. Use a throwaway script to append the row (do not hand-edit the binary), same pattern Satra's Task 2 used. Confirm by reading the sheet back that the row landed correctly and existing rows are untouched.

- [ ] **Step 2: Write the failing test**

Create `GO/internal/processing/winmart_processor_test.go`. Follow the exact structure of the existing Satra/BigC equivalents (`copyTestWorkbookForProcessor`, `fixturePricingSource` — both now in `golden_test_helpers_test.go` since BigC's Task 0, reuse, don't redeclare):

```go
package processing

import (
	"context"
	"testing"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleWinmartFile(t *testing.T) {
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
	rows, err := rp.Process(context.Background(), "testdata/sample_winmart_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Process returned %d rows, want 1: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.System != "Winmart" {
		t.Fatalf("row.System = %q, want %q", row.System, "Winmart")
	}
	if row.PO != "4194002858" {
		t.Fatalf("row.PO = %q, want %q", row.PO, "4194002858")
	}
	// The test fixture's data.xlsx WINMART row (added in Step 1) uses a
	// different address than this real file's -- run the test first to
	// see whether it fuzzy-matches above the >95 threshold anyway,
	// exactly as every prior vendor's equivalent test in this series has
	// done. Assert MaKhachHang against whatever the ACTUAL observed
	// result is -- do not guess.
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/... -run TestRealProcessor_ProcessesRealSampleWinmartFile -v`
Expected: FAIL — Winmart isn't routed yet.

- [ ] **Step 4: Add the Winmart case to `Process`'s dispatch**

Read the current `coop_processor.go`'s `Process` function first (its switch already has `case "Coop"`/`"Lotte"`/`"Satra"`/`default` — BigC's Task 6 added a whole-document pre-check before this loop, which does NOT apply to Winmart, since Winmart is per-page like Coop/Lotte/Satra). Add, following the exact same shape as the existing `"Satra"` case:

```go
		case "Winmart":
			row, err := p.processWinmartSegment(filePath, text, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Winmart",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)
```

- [ ] **Step 5: Implement `processWinmartSegment`**

Create `GO/internal/processing/winmart_processor.go`:

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
	"order-processor/internal/processing/winmart"
)

// winmartRegionInfo mirrors the kho/khuvuc/mien branching inline in
// write_to_dondathang_winmart (xulydonhang.py:4226-4238) — a 3-way
// result the shared regionInfo (processor_shared.go) does NOT correctly
// cover: shared regionInfo's non-MB branch returns warehouse "LA_TP",
// but Winmart's own non-MB branch needs "LA_KHO2026" — confirmed by
// direct source comparison during planning, not the same value Coop/
// Satra use. Also has one exact-match override no other vendor has:
// customer code literally "MN_MT_WIN1326" always resolves to Đà Nẵng
// (khuvuc "MT_MN", kho "TP_DN_1", mien "DN"), checked AFTER (and
// overriding) the MB/else branch — mirrored here as a switch with the
// exact-match case checked first so the result is equivalent without
// needing sequential mutation.
func winmartRegionInfo(customerCode string) (region, statCode, warehouse string) {
	switch {
	case customerCode == "MN_MT_WIN1326":
		return "MT_MN", "DN", "TP_DN_1"
	case strings.HasPrefix(customerCode, "MB"):
		return "MT_MB", "HN", "TP_HN_12"
	default:
		return "MT_MN", "LA", "LA_KHO2026"
	}
}

// winmartOrderNumber mirrors write_to_dondathang_winmart's order-number
// field (xulydonhang.py:4219,4262): f'ĐĐH{vendor}{STT_donhang_str}'
// where vendor is the uppercased literal "WINMART" and STT_donhang_str
// is f"-{po_number}" — captured from the ORIGINAL po_number, BEFORE any
// ghichu (note) text is appended to it for the L-column description
// (xulydonhang.py:4255-4256 reassigns po_number for diengiai's sake
// only, AFTER STT_donhang_str is already built) — so the order number
// itself never includes the note text, even when one is present.
func winmartOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHWINMART-%s", poNumber)
}

// processWinmartSegment mirrors the Winmart branch of process_file
// (xulydonhang.py:8984-9160) plus write_to_dondathang_winmart
// (:4203-4579). Winmart is "1 page = 1 order", the same family as Coop/
// Lotte/Satra (confirmed during planning: trailing PDF pages that lack
// Winmart's identify marker are silently skipped as "Unknown" by the
// existing per-page dispatch loop, exactly as intended — no BigC-style
// whole-document state is needed). The per-item promo bonus-row block
// reuses the shared buildPromoBonusRow helper unmodified (confirmed
// structurally identical to Coop's index==0 case) — but the
// invoice-level ("Hóa Đơn") bonus row does NOT reuse buildInvoiceBonusRow,
// because Winmart's version writes only the FIRST matched SKU to column
// Q (xulydonhang.py:4537, kiemtra[0]), not buildInvoiceBonusRow's
// comma-joined list of every matched SKU — the same divergence BigC's
// invoice-level block had, confirmed independently for Winmart by direct
// source comparison during planning.
func (p *RealProcessor) processWinmartSegment(filePath, text, pageLabel string) (OrderRow, error) {
	poNumber, entryDate, cancelDate, note, ok := winmart.ParseOrderInfo(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được số PO")
	}
	deliveryAddress, _ := winmart.ParseDeliveryAddress(text) // best-effort, matches Python's diachigiaohang staying None
	fuzzyAddress, _ := winmart.ParseFuzzyMatchAddress(text)  // best-effort, matches Python's diachi staying None

	products := winmart.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	customerCode, found := p.Store.GetCustomerCodeByFuzzyAddress("WINMART", fuzzyAddress)
	if !found {
		customerCode = "Không xác định"
	}

	priceIndex, err := p.Pricing.FetchIndex("WINMART")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := winmartRegionInfo(customerCode)
	orderNum := winmartOrderNumber(poNumber)

	// diengiai (xulydonhang.py:4255-4258): built from po_number AFTER the
	// note (ghichu) is appended, if present — so the L-column description
	// includes the note, but the order number (built earlier, above)
	// never does.
	descriptionPO := poNumber
	if note != "" {
		descriptionPO = fmt.Sprintf("%s - %s", poNumber, note)
	}
	description := fmt.Sprintf("WINMART PO%s", descriptionPO)
	// The header row's S column (xulydonhang.py:4275) re-splits the
	// note-appended po_number on the first "-" and keeps only what's
	// before it — for a real Winmart PO number (always plain digits,
	// confirmed during planning) this exactly cancels the " - <note>"
	// suffix back off, reproducing the original po_number. Faithfully
	// ported as a literal split, not "fixed" to just reuse the original
	// po_number directly — if a future PO number ever contains a literal
	// "-" itself, this would truncate early, matching Python's own
	// behavior exactly (not something to guard against here).
	headerProductName := fmt.Sprintf("WINMART PO%s", strings.SplitN(descriptionPO, "-", 2)[0])

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: customerCode,
		Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: headerProductName,
	})

	saigia := 0
	totalWeight := 0.0
	totalValue := 0.0

	for _, rawProduct := range products {
		barcode := p.Store.ResolveSku(rawProduct.Barcode)
		productInfo, _ := p.Store.GetProductInfo(barcode)

		ouQty := parseNumericField(rawProduct.OUQty)
		totalPrice := parseNumericField(rawProduct.TotalPrice)

		// Zero-price "giao rời" skip (xulydonhang.py:4299-4304): Python's
		// condition is an ABSOLUTE sheet row number ("current_row - 2 >=
		// 9"), which in a real production sheet is nearly always true
		// regardless of THIS order's own row count — meaning a zero-price
		// item that's the first or second product of its own order would
		// read/overwrite a PREVIOUS order's AO/AP cells. This port
		// deliberately checks only THIS order's own accumulated rows
		// instead (see this plan's Global Constraints) — if there's no
		// prior row in `rows` to mark (fewer than 2 rows accumulated so
		// far, i.e. only the header row or nothing yet), skip the
		// zero-price item cleanly rather than reaching outside this
		// order's row set.
		if totalPrice == 0 {
			if len(rows) >= 3 { // header row + at least 2 product/bonus rows already appended
				rows[len(rows)-2].PromoNote = "KM Giao Rời - Không Che"
				rows[len(rows)-2].PromoBundleSku = ""
				rows[len(rows)-1].PromoBundleSku = ""
			}
			continue
		}

		lineWeight := productInfo.WeightKg * ouQty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(ouQty / productInfo.PackSize))
		}
		totalWeight += lineWeight

		// giahoadon (xulydonhang.py:4347-4351): WINMART divides the
		// line's total price by quantity to get a per-unit invoice
		// price — the sibling "BC MART" branch (out of scope for this
		// plan, BC Mart isn't a ported vendor) uses the total AS the
		// unit price directly instead. Only the WINMART branch is
		// ported/tested here.
		invoicePrice := totalPrice / ouQty

		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice := parseNumericField(realPriceStr)

		promos := priceIndex.FindPromotions(barcode, entryDate)
		khuyenmai := ""
		matched := false
		finalPrice := realPrice

		for _, promo := range promos {
			value := promo.Value
			khuyenmai = value
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
		if len(promos) == 0 && closeEnough(invoicePrice, finalPrice) {
			matched = true
		}

		productRow := excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: customerCode,
			Description: description, SKU: barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: ouQty, UnitPrice: finalPrice,
			ProductName: productInfo.Name, CaseCount: caseCount, LineWeightKg: lineWeight, UseZFormula: true,
			PromoContent: khuyenmai,
		}
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
		}

		productRowIndex := len(rows)
		rows = append(rows, productRow)
		totalValue += finalPrice * ouQty

		// Per-item promo bonus row: confirmed structurally identical to
		// buildPromoBonusRow's index==0 case (same field mapping, same
		// "AO on product row, AP on both rows" placement, same X+1
		// quantity-divide logic) — reused unmodified. Winmart has no
		// multi-CTKM-per-item loop (xulydonhang.py's per-item block never
		// splits khuyenmai on "|"), so there is only ever one bonus
		// attempt per item, always at index 0.
		bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, khuyenmai,
			coop.Product{Barcode: barcode, Qty: ouQty}, 0, entryDate, cancelDate, deliveryAddress,
			customerCode, description, warehouse, region, statCode, orderNum)
		if added {
			totalWeight += bonusRow.LineWeightKg
			rows[productRowIndex].PromoNote = mainRowNote
			if mainRowBundleSku != "" {
				rows[productRowIndex].PromoBundleSku = mainRowBundleSku
			}
			rows = append(rows, bonusRow)
		}
	}

	// Invoice-level ("Hóa Đơn") promo bonus row (xulydonhang.py:4521-4562).
	// Does NOT reuse the shared buildInvoiceBonusRow — see this function's
	// doc comment for why (Q column gets only the first matched SKU, not
	// a joined list).
	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		invoiceSkus := p.Store.FindSkusMentioned(invoicePromo)
		if amount, ok := coop.ExtractMoneyAmount(invoicePromo); ok && amount > 0 && len(invoiceSkus) > 0 {
			invoiceSku := invoiceSkus[0] // xulydonhang.py:4537 — kiemtra[0], not a joined list
			soluongkm := math.Floor(totalValue / float64(amount))
			invoiceInfo, _ := p.Store.GetProductInfo(invoiceSku)
			invoiceWeight := invoiceInfo.WeightKg * soluongkm
			invoiceCase := 0
			if invoiceInfo.PackSize > 0 {
				invoiceCase = int(math.Ceil(soluongkm / invoiceInfo.PackSize))
			}
			totalWeight += invoiceWeight

			invoiceNote := coop.ExtractBraceContent(invoicePromo)
			if invoiceNote == "" {
				invoiceNote = "KM Bó Kèm - Che Barcode" // xulydonhang.py:4558
			}

			rows = append(rows, excelwriter.Row{
				EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
				Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: customerCode,
				Description: description, SKU: invoiceSku, Warehouse: warehouse, VATPercent: 8,
				RegionCode: region, StatCode: statCode, IsPromoItem: true, Qty: soluongkm,
				ProductName: invoiceInfo.Name, CaseCount: invoiceCase, LineWeightKg: invoiceWeight, UseZFormula: false,
				PromoContent: invoicePromo, PromoNote: invoiceNote,
			})
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
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Winmart", MaKhachHang: customerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
	}, nil
}

// parseNumericField mirrors the repeated "strip commas, coerce to
// float" pattern applied to product["OU Qty"]/product["Total Price"]
// (xulydonhang.py:4327-4337) and to a fetched price string — returns 0
// on any parse failure rather than panicking.
func parseNumericField(s string) float64 {
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil {
		return 0
	}
	return v
}
```

**Note for the implementer on the zero-price skip's row-count check (`len(rows) >= 3`):** this plan's reference code marks `rows[len(rows)-2]` and `rows[len(rows)-1]` when at least 3 rows are accumulated (header + 2 more). Verify this indexing is actually correct by hand-tracing against the two intended scenarios before trusting it: (a) a zero-price item immediately following a product row that had NO bonus row (rows = [header, product] at the time — only 2 rows, so the guard correctly skips, since Python's own block only ever targets a *pair* of rows including a bonus row, per its `AP{current_row-1}` write existing unconditionally alongside `AO{current_row-2}`); (b) a zero-price item following a product+bonus row pair (rows = [header, product, bonus] — 3 rows, guard passes, `rows[len(rows)-2]` is the product row, `rows[len(rows)-1]` is its bonus row) — confirm this matches the Python source's intent (AO on the product row, AP cleared on both) before relying on it, and adjust if real fixture behavior in Task 6 disagrees.

- [ ] **Step 6: Fill in the real customer-code assertion, run tests, verify they pass**

Go back to Step 2's test and add the `MaKhachHang` assertion based on the actually-observed value (run the test, read what it produces, assert that). Run: `cd GO && go build ./... && go vet ./... && go test ./internal/processing/... -v`
Expected: PASS — the new Winmart test, and every existing Coop/Lotte/Satra/BigC test unchanged.

- [ ] **Step 7: Commit**

```bash
git add GO/internal/processing/coop_processor.go GO/internal/processing/winmart_processor.go GO/internal/processing/winmart_processor_test.go GO/internal/processing/testdata/sample_winmart_order.pdf GO/internal/processing/productdata/testdata/data.xlsx
git commit -m "feat(go): dispatch RealProcessor to Winmart via processWinmartSegment"
```

---

### Task 5: Golden fixture generation script (throwaway) — generate 16 Winmart fixtures

**Files:**
- Create: `GO/internal/processing/winmart/testdata/generate_fixtures.py` (throwaway dev tool, adapted from `GO/internal/processing/bigc/testdata/generate_fixtures.py`)

**Interfaces:**
- Consumes: the real `xulydonhang.py`'s `ProcessHandler.laymakhachhang_satra`, `trichxuatsanpham_winmart`, `write_to_dondathang_winmart`, `identify_vendor`, `find_price_by_sku`, `find_all_promotions_by_sku_and_time`, `get_gid` — all unmodified.
- Produces: `GO/internal/processing/winmart/testdata/fixtures/*.json` + `_frozen_pricing.json`. Consumed by Task 6.

- [ ] **Step 1: Write the script**

Create `GO/internal/processing/winmart/testdata/generate_fixtures.py`, adapted directly from `GO/internal/processing/bigc/testdata/generate_fixtures.py` (read it first) — same `REPO_ROOT` resolution (6 `dirname()` calls, same directory depth), same UTF-8 stdout fix, same production-`dondathang.xlsx` backup/restore protocol **with the retry-with-backoff hardening BigC's Task 7 added** (`_remove_with_retry`/`_move_with_retry` around the `finally:` block's restore sequence — copy this hardening from the start this time, don't wait for a transient-lock crash to add it), same price/promo caching monkeypatch (generic over `sheet_name`, works for `"WINMART"` with no changes), same `upload_file_to_drive` no-op patch. Winmart is per-page like Satra/Lotte (NOT whole-document like BigC), so the harness shape is closer to Satra's/Lotte's `generate_fixtures.py` than BigC's — only `is_winmart_pdf`/`process_one_pdf` are Winmart-specific:

```python
def is_winmart_pdf(path):
    doc = xulydonhang.fitz.open(path)
    try:
        text = ""
        for page in doc:
            text += page.get_text("text")
        return xulydonhang.ProcessHandler.identify_vendor(text) == "Winmart"
    finally:
        doc.close()


def process_one_pdf(path):
    """Mirrors the Winmart branch of process_file (xulydonhang.py:8984-9160)
    for every page identify_vendor recognizes as Winmart, skipping the
    Google Drive upload / current-page-extraction side effects."""
    handler = xulydonhang.process_handler
    doc = xulydonhang.fitz.open(path)
    try:
        for page_num in range(len(doc)):
            text = doc[page_num].get_text("text")
            if xulydonhang.ProcessHandler.identify_vendor(text) != "Winmart":
                continue

            lines = text.split("\n")

            idx = next((i for i, line in enumerate(lines) if "Ngày đặt hàng (PO date)" in line), -1)
            entry_date = lines[idx + 1].strip() if idx != -1 and idx + 1 < len(lines) else None
            entry_date = entry_date.replace('.', '/') if entry_date else None

            ghichu = "\n".join(
                text.split("Ghi chú")[1]
                .split("Nhà cung cấp (Supplier): 0002011398")[0]
                .strip()
                .splitlines()[:-1]
            )
            ghichu = ghichu.replace('\n', ' ')

            idx = next((i for i, line in enumerate(lines) if "Số đơn hàng (PO No.)" in line), -1)
            po_number = lines[idx + 1].strip() if idx != -1 and idx + 1 < len(lines) else None

            idx = next((i for i, line in enumerate(lines) if "Ngày giao (Delivery Date)" in line), -1)
            cancel_date = lines[idx + 1].strip() if idx != -1 and idx + 1 < len(lines) else None
            cancel_date = cancel_date.replace('.', '/') if cancel_date else None

            idx = next((i for i, line in enumerate(lines) if "Địa chỉ giao hàng (Delivery Address)" in line), -1)
            if idx != -1:
                ma_kho = lines[idx + 1].strip()
                address_lines = []
                for line in lines[idx + 2:]:
                    if "Thông tin đơn hàng (Information)" in line:
                        break
                    line = line.strip()
                    if "WM+" in line:
                        continue
                    if line:
                        address_lines.append(line)
                diachigiaohang = f"{ma_kho} - {' '.join(address_lines)}"
            else:
                diachigiaohang = None

            idx = -1
            for i in range(len(lines) - 1):
                line_lower = lines[i].lower()
                next_line_lower = lines[i + 1].lower()
                if "tổng hợp" in line_lower and "wincommerce" in next_line_lower:
                    idx = i
                    break
                elif "wincommerce" in line_lower:
                    idx = i
                    break
            if idx != -1:
                diachi_lines = []
                for i in range(idx + 1, len(lines)):
                    line_lower = lines[i].lower()
                    if "mst" in line_lower or "địa chỉ giao hàng" in line_lower:
                        break
                    diachi_lines.append(lines[i].strip())
                diachi = " ".join(diachi_lines)
            else:
                diachi = None

            products = xulydonhang.ProcessHandler.trichxuatsanpham_winmart(text)

            makhachhang = xulydonhang.ProcessHandler.laymakhachhang_satra(diachi, "WINMART")
            if not makhachhang:
                makhachhang = "Không xác định"

            xulydonhang.ProcessHandler.write_to_dondathang_winmart(
                handler, products, makhachhang, po_number, entry_date, cancel_date,
                1, "Winmart", diachigiaohang, ghichu, None,
            )
    finally:
        doc.close()
```

Everything else (`main`, `snapshot_rows`, `COLUMNS`, the pricing-cache monkeypatch, the backup/restore protocol, the retry-with-backoff helpers) is copied verbatim from the BigC harness, changing only: `FIXTURES_DIR` → `.../winmart/testdata/fixtures`, `is_bigc_pdf`/`process_one_pdf` → the Winmart versions above, and the final frozen-pricing capture call to `_capture_promo_raw_rows("WINMART")`. **Unlike BigC's harness (which snapshots once after a whole file's multi-page loop), this harness snapshots once per PAGE, immediately after each `write_to_dondathang_winmart` call** — matching Satra's/Lotte's per-page snapshot pattern, since Winmart is per-page like those vendors, not whole-document like BigC.

- [ ] **Step 2: Back up the production workbook before running (safety)**

```bash
cp dondathang.xlsx dondathang.xlsx.manual_backup_before_winmart_fixtures
```

- [ ] **Step 3: Run the script**

```bash
.venv/Scripts/python.exe GO/internal/processing/winmart/testdata/generate_fixtures.py
```

Expected: "Found N candidate PDFs" (N is whatever the current total in `đơn hàng/08-2026/` is — do not assume it's still 342, more may have been added since this plan was written), then one `OK`/`SKIP` line per Winmart file, ending with "Done: 16 fixtures generated, 0 PDFs skipped" (16 is the count established during this plan's research — if it differs, that's fine as long as every non-generated file has a clear SKIP reason; investigate before proceeding if any file silently produces neither).

- [ ] **Step 4: Verify the production workbook is untouched**

```bash
diff dondathang.xlsx dondathang.xlsx.manual_backup_before_winmart_fixtures && echo "IDENTICAL — safe" || echo "DIFFERS — investigate before proceeding, do not continue"
```

If it differs: STOP, restore immediately (`mv dondathang.xlsx.manual_backup_before_winmart_fixtures dondathang.xlsx`), investigate before doing anything else.

- [ ] **Step 5: Remove the manual backup once confirmed identical**

```bash
rm dondathang.xlsx.manual_backup_before_winmart_fixtures
```

- [ ] **Step 6: Spot-check a few generated fixtures**

Read 2-3 files under `GO/internal/processing/winmart/testdata/fixtures/*.json` and confirm plausible values (PO-shaped `B` column like `ĐĐHWINMART-...`, non-empty `S` product names, sane `X`/`Y`/`AT`/`AU` values — note `AU` should be a real non-null number for every product/bonus row, unlike BigC's fixtures where it's always null). If any fixture contains a zero-price product line, specifically check whether the resulting row set shows the expected AO/AP-on-previous-rows pattern (per this plan's Task 4 design) or something else — note it for Task 6's investigation either way, don't "fix" the fixture.

- [ ] **Step 7: Commit**

```bash
git add GO/internal/processing/winmart/testdata/generate_fixtures.py GO/internal/processing/winmart/testdata/fixtures/
git commit -m "test(go): generate golden fixtures for Winmart from real PDFs + production output"
```

---

### Task 6: Golden fixture integration test

**Files:**
- Create: `GO/internal/processing/winmart_golden_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1-5; reuses `fixtureData`, `frozenPricingFixture`, `fixturePricingSource`, `compareRowsAgainstFixture`, `stringify`, `toFloat`, `floatCloseEnough`, `copyFile`, `joinLines` — all in `golden_test_helpers_test.go` since BigC's Task 0.
- Produces: `TestRealProcessor_MatchesGoldenFixtures_Winmart`.

- [ ] **Step 1: Write `winmart_golden_test.go`**

Create `GO/internal/processing/winmart_golden_test.go`, following `satra_golden_test.go`'s exact structure (Winmart is per-page like Satra, not whole-document like BigC, so Satra's golden test — not BigC's — is the right template):

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

// knownDivergences_Winmart lists (fixture, row index, column) cells
// where this Go port intentionally computes a different, verified-more-
// correct value than the frozen Python fixture. Key format:
// "<source_pdf>:<row index>:<column>". Empty until a real, hand-verified
// case is confirmed; add entries here only with a comment citing the
// specific PDF/Python-line evidence — never to silence an unexplained
// diff. See this plan's Global Constraints for the specific,
// already-anticipated "leaked loop variable in laycachbo_khuyenmai" case
// this may be needed for, and the zero-price-skip design decision that
// may produce AO/AP divergences on real fixtures containing a zero-price
// product as the first or second item of an order.
var knownDivergences_Winmart = map[string]bool{}

func loadFrozenWinmartPricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("winmart/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen Winmart pricing fixture found (run Task 5's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen Winmart pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

func TestRealProcessor_MatchesGoldenFixtures_Winmart(t *testing.T) {
	fixturePaths, err := filepath.Glob("winmart/testdata/fixtures/*.json")
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
		t.Skip("no golden fixtures found (run Task 5's generate_fixtures.py first)")
	}

	store, err := productdata.Load("../../../data.xlsx")
	if err != nil {
		t.Fatalf("failed loading production data.xlsx: %v", err)
	}
	pricingSource := loadFrozenWinmartPricingSource(t)

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

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_Winmart)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
}
```

- [ ] **Step 2: Run the test**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures" -v`

Expected: Coop's, Lotte's, Satra's, and BigC's tests still report their exact unchanged baselines (Coop: its known pre-existing 62-fixture gap, unchanged; Lotte 60/60; Satra 36/36; BigC 29/29). Winmart's test will very likely fail on the first run with real mismatches — this is the actual verification work of this task, same as every prior vendor's equivalent task.

- [ ] **Step 3: Root-cause and fix every mismatch**

For each mismatch: read the specific fixture JSON and the source PDF, trace through `xulydonhang.py`'s actual Winmart functions at the cited line numbers, and determine whether it's (a) a bug in this plan's Go port — fix the Go code; or (b) a case where Python is genuinely wrong or Go's design deliberately diverges (the zero-price-skip design and the `value`-vs-`khuyenmai` leaked-loop-variable case are the two already-anticipated candidates from this plan's Global Constraints — but verify each actually occurs in a real fixture before assuming it's the cause of any given mismatch) — add a precise, evidence-citing entry to `knownDivergences_Winmart` using the `sourcePDF:row:col` key format. Do not guess; every fix or allowlist entry must be traceable to specific evidence. Re-run after each fix.

**Specific things to check if a mismatch appears:**
- Any `AO`/`AP` mismatch on a row adjacent to a zero-price product — trace whether the real Python fixture actually exercised the "reach into a previous, unrelated order's row" bug (only possible if this order happens to start at absolute sheet row < 9 AND has a zero-price item as its first/second product — unlikely in a real production-history sheet, but verify rather than assume) versus this port's deliberately-scoped-to-current-order design; document per Global Constraints if the divergence is real and confirmed.
- Any `Q`/`X`/`S` mismatch on an invoice-level bonus row — confirm `Q` uses only the first matched SKU (not joined), matching this plan's `processWinmartSegment` design; this should NOT need a divergence entry (the plan already implements the correct, Winmart-specific shape) — if it mismatches anyway, that's a real Go bug to fix, not a divergence to document.
- Any `AU`/`AT` mismatch — Winmart writes real values here (unlike BigC), so any mismatch is a real computation bug, not a documented-absence case.

- [ ] **Step 4: Final run and commit**

Run: `cd GO && go build ./... && go vet ./... && go test ./... -v`
Expected: clean build/vet, all tests pass (or fail only with fully documented, understood, non-logic-bug gaps — Coop's pre-existing unrelated failure is the one known exception).

```bash
git add GO/internal/processing/winmart_golden_test.go
git commit -m "test(go): add Winmart golden fixture integration test"
```
