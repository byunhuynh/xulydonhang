# Kingfood RealProcessor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port Kingfood order processing from `xulydonhang.py` to Go, producing a `processKingfoodSegment` that plugs into `RealProcessor.Process`'s existing per-page dispatch — the 7th and final vendor in the Phase 2b roadmap.

**Architecture:** New package `GO/internal/processing/kingfood/` (pure text extraction: tab-to-space normalization, PO/date line-scan, product-table extraction with a Vietnamese-format number field) + new file `GO/internal/processing/kingfood_processor.go` (dispatch, region lookup, row builder reusing `buildPromoBonusRow`/`buildInvoiceBonusRow` from `processor_shared.go`). Kingfood is "1 PDF page = 1 order" (same family as Coop/Lotte/Satra/Winmart/Emart/FujiMart), not BigC's whole-document shape.

**Tech Stack:** Go 1.x, `excelize/v2`, existing `processing`/`productdata`/`pricing`/`excelwriter`/`coop` packages.

**Spec:** [docs/superpowers/specs/2026-08-19-kingfood-real-processor-design.md](../specs/2026-08-19-kingfood-real-processor-design.md)

## Global Constraints

- **Testing/divergence policy** (same as every vendor since Lotte): golden-fixture tests compare against real Python output; intentional Go/Python divergences go in a `knownDivergences_Kingfood` allowlist with `sourcePDF:rowIndex:column` keys and evidence citations — never force a fixture to pass by editing it.
- **⚠️ NEW PDF-extraction divergence class, confirmed via a Task 0 smoke test run directly against this repo's own Go pipeline on 3 real Kingfood PDFs, then cross-checked against PyMuPDF's real output on the SAME files**: Go's `extractPageTexts` inserts a literal TAB character between words within a multi-word label line (e.g. `"PO\tNumber:"`), where PyMuPDF (what Python's real code runs against) inserts a plain space (`"PO Number:"`). Line-break positions are IDENTICAL between the two pipelines — this is purely an intra-line word-separator difference, never seen in any of the 6 prior vendors (all of which only had line-break-position divergences). **Fix: normalize `text = strings.ReplaceAll(text, "\t", " ")` at the entry point of every extraction function in the `kingfood` package**, restoring the exact space-separated shape Python's literal marker strings assume. This is simpler than the "tolerate 2 different line layouts" technique used for Emart/FujiMart — just one global character substitution, not conditional line-scan logic.
- **Kingfood is the FIRST vendor whose `vendor.Identify` case must be INSERTED MID-CHAIN, not appended at the end.** Real Python order (`xulydonhang.py:90-129`): `...Emart(111) → Kingfood(114) → [CN-HCM, unported](118) → Winmart(121) → [SHOPEE-CHOICE, unported](125) → FujiMart(128)`. Go's current chain (`Coop → BigC → Lotte → Satra → Emart → Winmart → FujiMart`) already matches Python's real relative order among ported vendors — Kingfood must go **between the Emart check and the Winmart check**, not after FujiMart.
- **No cross-validate/fallback date logic** — unlike FujiMart/Winmart/Emart, Python's Kingfood branch has no ±N-day backfill for a malformed date; it calls `datetime.strptime` directly with no `try/except`, which would crash Python outright on a bad date. This port returns a clean `ok=false` in that case instead of reproducing the crash — established project policy.
- **Delivery address and customer code are BOTH hardcoded constants** (`"MN_MT_KFMSL"` and `"Số 324, đường ĐT743A, Phường Đông Hoà, Thành phố Hồ Chí Minh"`, `xulydonhang.py:9273-9274`) — no PDF extraction, no fuzzy matching, no OCR needed for either. Simpler than every prior vendor in this respect.
- **Kingfood's product-price field uses Vietnamese/European number formatting (period = thousands separator, comma = decimal separator)** — e.g. `"52.195,073"` → `52195.073`. This is the OPPOSITE convention from every other vendor in this project (which only ever strip commas, US-style thousands, no decimal point in that field). The shared `parseNumericField` helper (`bigc_processor.go`) is NOT a fit for this field — a new Kingfood-scoped `parseKingfoodPrice` function is required. Kingfood's quantity field, by contrast, only ever strips periods (`quantity.replace('.', '')`, matching `parseNumericField`'s existing behavior) — no format mismatch there.
- **Kingfood's `"Total Price"` product field is misleadingly named — it is actually a PER-UNIT final price (post-discount), not a line total.** Confirmed via direct trace of `laydanhsachsanpham_kingfood`'s regex (`xulydonhang.py:6698-6758`): the captured `price` group is the "Đơn giá cuối (-VAT)" column (final unit price after every discount), not "Thành tiền" (line total). `write_to_dondathang_kingfood` uses it directly as `giahoadon = dongia = float(product["Total Price"])` with **no division by quantity** (`xulydonhang.py:3925,3970`) — unlike FujiMart/Winmart, whose `TotalPrice` field IS a line total requiring `totalPrice / ouQty` to get a per-unit price. **Do not copy FujiMart's/Winmart's division pattern here** — Kingfood's `invoicePrice` is the parsed price field used directly.
- **Kingfood's per-item promo fallback text is `"KM Giao Rời - Không Che Barcode"` (`xulydonhang.py:4096`) — identical to Winmart's fallback string** — and, like Winmart's (and unlike FujiMart's), this fallback does **NOT** write column AP (`PromoBundleSku`) at all (only the `cachbokem` branch at `xulydonhang.py:4093-4094` writes AP) — the shared `buildPromoBonusRow` helper's own Coop-flavored default DOES write AP, so this call site needs the same override Winmart/Emart already use: clear `mainRowBundleSku` and `bonusRow.PromoBundleSku` in addition to overriding the AO text.
- **Kingfood writes AU (case count) normally** — matches Coop/Satra/Lotte/Winmart/FujiMart, unlike Emart/BigC (which never write AU). Do NOT set `NoCaseCount` on any row.
- **No zero-price skip logic** — every extracted product gets its own row regardless of price, matching FujiMart/Coop/Satra/Lotte (not Winmart's zero-price "giao rời" skip).
- **Region info needs THREE branches, not two** — `xulydonhang.py:3871-3883`: `makhachhang[:2]=="MB"` → HN branch; else if `makhachhang=="MN_MT_JM0001"` → a SEPARATE Miền Nam branch using warehouse `"LA_TP"` (not `"LA_KHO2026"`); else → the usual Miền Nam `"LA_KHO2026"` branch. Kingfood is the first vendor in this project with a genuine 3-way region split (FujiMart/Winmart only ever had 2). Since `makhachhang` is always the hardcoded literal `"MN_MT_KFMSL"` (never `"MB..."`, never exactly `"MN_MT_JM0001"`), only the third (`LA_KHO2026`) branch is reachable with real input today — still implement all 3 branches fully, matching established per-vendor precedent (write the full branch structure even when only one path is reachable, for architectural consistency and to correctly handle a hypothetical future customer-code change).
- **CR-to-LF normalization must be applied to BOTH the per-item promo value AND the invoice-level promo value from the very first commit** — `openpyxl` (Python) silently mangles `\r`→`\n` on xlsx read/write round-trip; `excelize` (Go) preserves `\r` literally, which shows up as a visible `&#xD;` artifact in a real production workbook. This exact omission (forgetting the invoice-level normalization while remembering the per-item one) was caught in FujiMart's own final-review fix wave — do not repeat that mistake here; both call sites get `strings.ReplaceAll(value, "\r", "\n")` from Task 4's first commit, not added later as a fix-round patch.
- **`excelwriter.Row` needs NO new fields.** Every column Kingfood writes (A,AV,B,C,D,G,L,V,AE,AJ,AM,U,Z,S,T,X,Y,E,Q,AQ,AO,AP,AT,AU) is already supported by the existing struct. Column `AR` (`Mahang`) is written by Python but its source value is always empty in practice (`laydanhsachsanpham_kingfood`'s `Product` dict never has a `"Mahang"` key) — do not add an `AR` field unless a real golden fixture in Task 6 shows a non-empty value (it should not).
- **`settings.ini` already has a `KINGFOOD` gid entry** (`settings.ini:8`, `KINGFOOD = 281168437`) — no changes needed there.
- **9 real Kingfood PDFs available now** (see Task 5's exact source paths, all in the archive tree `đơn hàng/mẫu đơn hàng/*/`, none in the live `đơn hàng/08-2026/` folder as of planning) — this is a point-in-time count; more may exist. **Source PDFs are committed into `GO/internal/processing/kingfood/testdata/realpdfs/`** (git-tracked, stable, immune to the live folder's ongoing reorganization by a concurrently-running production instance of this same application) rather than read from the live folder at test-run time, per the same pattern established for Emart and FujiMart — **confirm this decision explicitly with the project owner before Task 5's commit, do not assume the same answer carries over automatically from prior vendors.**
- Every exported function gets a doc comment citing the exact `xulydonhang.py` line range it mirrors. Every deviation from a literal Python behavior gets an inline comment explaining why.
- Run `go build ./...`, `go vet ./...`, and the relevant `go test` scope after every task, from the `GO/` directory.
- **New package** `GO/internal/processing/kingfood/` for Kingfood-only extraction. **New file** `GO/internal/processing/kingfood_processor.go` — never append to any other vendor's `_processor.go` file.

---

### Task 1: `vendor.Identify` — recognize Kingfood, INSERTED between Emart and Winmart

**Files:**
- Modify: `GO/internal/processing/vendor/identify.go`
- Modify: `GO/internal/processing/vendor/identify_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `Identify(text string) string` now also returns `"Kingfood"` — consumed by Task 4's dispatch.

- [ ] **Step 1: Write the failing tests**

Add to `GO/internal/processing/vendor/identify_test.go`:

```go
func TestIdentify_RecognizesKingfoodByTaxCode(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"real tax code", "Header\n0313403198\nfooter", "Kingfood"},
		{"unrelated number", "Header\n999999999999\nfooter", ""},
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

func TestIdentify_KingfoodCheckedBetweenEmartAndWinmart(t *testing.T) {
	// Real xulydonhang.py order (xulydonhang.py:90-129): ...Emart(111) ->
	// Kingfood(114) -> [CN-HCM, unported](118) -> Winmart(121) ->
	// [SHOPEE-CHOICE, unported](125) -> FujiMart(128). Kingfood is the
	// FIRST vendor in this project whose Identify case must be inserted
	// mid-chain rather than appended at the end — every prior vendor's
	// correct relative position happened to already be "at the end" of
	// the then-current Go chain. There's no genuine ordering CONFLICT to
	// construct here (Kingfood's own marker, a plain tax-code substring,
	// doesn't overlap any other vendor's pattern), so this test
	// documents the intent for a future reader, mirroring
	// TestIdentify_EmartCheckedBetweenSatraAndWinmart's own rationale.
	got := Identify("0313403198")
	if got != "Kingfood" {
		t.Fatalf("Identify with Kingfood marker = %q, want %q", got, "Kingfood")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/vendor/... -run TestIdentify_Kingfood -v`
Expected: FAIL (compile error — `Identify` doesn't recognize Kingfood's marker yet).

- [ ] **Step 3: Add `kingfoodPattern` and wire it into `Identify`, INSERTED between the Emart and Winmart checks**

In `GO/internal/processing/vendor/identify.go`, add to the `var (...)` block, after `emartPattern` and before `winmartPattern`:

```go
	// Kingfood's identify pattern (xulydonhang.py:114-115): a single
	// literal numeric substring (the vendor's own tax code), no
	// alternation. Real Python order places Kingfood immediately after
	// Emart and before CN-HCM (unported)/Winmart — see Identify's own
	// doc comment for the full chain.
	kingfoodPattern = regexp.MustCompile(`0313403198`)
```

Update the doc comment on `Identify` to mention Kingfood is now implemented, inserted between Emart and Winmart (matching the file's existing style — see how it already documents Kingfood/CN-HCM/SHOPEE-CHOICE as gaps). Insert the case inside `Identify`, between the `emartPattern` check and the `winmartPattern` check:

```go
	if emartPattern.MatchString(cleaned) {
		return "Emart"
	}
	if kingfoodPattern.MatchString(cleaned) {
		return "Kingfood"
	}
	if winmartPattern.MatchString(cleaned) {
		return "Winmart"
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/vendor/... -v`
Expected: PASS, all tests including the new ones, and all pre-existing tests (especially `TestIdentify_WinmartCheckedAfterSatra`/`TestIdentify_EmartCheckedBetweenSatraAndWinmart`-style ordering tests) still pass unchanged.

- [ ] **Step 5: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/vendor/identify.go GO/internal/processing/vendor/identify_test.go
git commit -m "feat(go): recognize Kingfood vendor in identify.Identify, inserted between Emart and Winmart"
```

---

### Task 2: `kingfood` package — `ParseOrderInfo` (PO number, dates, tab normalization)

**Files:**
- Create: `GO/internal/processing/kingfood/extract.go`
- Test: `GO/internal/processing/kingfood/extract_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `ParseOrderInfo(text string) (poNumber, entryDate, cancelDate string, ok bool)` — consumed by Task 4's `processKingfoodSegment`.

- [ ] **Step 1: Write the failing tests**

Create `GO/internal/processing/kingfood/extract_test.go`:

```go
package kingfood

import "testing"

func TestParseOrderInfo_ExtractsRealSampleFields(t *testing.T) {
	// Text shape mirrors this repo's OWN extractPageTexts output against
	// a real sample Kingfood PDF (confirmed during planning by running
	// the actual Go PDF pipeline directly, then cross-checked against
	// PyMuPDF's output on the SAME file) — including the tab characters
	// Go's extraction inserts between words in multi-word labels, where
	// PyMuPDF inserts plain spaces. \t below is a literal tab character.
	text := "\n" +
		"Page\t1\t/\t2\n" +
		"PO\tNumber:\n" +
		"PO1002601888\n" +
		"Nơi\tgiao:\n" +
		"KHO\tSEEDLOG\n" +
		"Ngày\tGiao\tHàng\tDự\tKiến:\n" +
		"05-08-2026\n" +
		"Ngày\tGiao\tHàng\tNCC\tXác\n" +
		"Nhận:\n" +
		"05-08-2026\n" +
		"Ngày\tĐặt\tHàng:\n" +
		"03-08-2026\n" +
		"Quá\tcảnh:\n"

	poNumber, entryDate, cancelDate, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if poNumber != "PO1002601888" {
		t.Errorf("poNumber = %q, want %q", poNumber, "PO1002601888")
	}
	if entryDate != "03/08/2026" {
		t.Errorf("entryDate = %q, want %q", entryDate, "03/08/2026")
	}
	if cancelDate != "05/08/2026" {
		t.Errorf("cancelDate = %q, want %q", cancelDate, "05/08/2026")
	}
}

func TestParseOrderInfo_MissingPONumberMarkerFailsCleanly(t *testing.T) {
	// No "PO Number:" marker anywhere -> poNumber resolves empty ->
	// ok=false. Mirrors Python's real crash risk here (a downstream
	// datetime.strptime on an unresolved/garbage date string would raise
	// ValueError, uncaught) with a clean failure instead, per this
	// codebase's established policy — Kingfood has NO cross-validate/
	// fallback logic to backfill a missing date (unlike FujiMart/
	// Winmart/Emart), so a single missing marker is unrecoverable.
	_, _, _, ok := ParseOrderInfo("nothing relevant here\nno markers at all\n")
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for text with no markers, want false")
	}
}

func TestParseOrderInfo_MalformedDateFailsCleanly(t *testing.T) {
	// A date that doesn't match dd-mm-yyyy should fail cleanly rather
	// than reproducing Python's real datetime.strptime crash.
	text := "PO\tNumber:\n" +
		"PO1002601888\n" +
		"Ngày\tGiao\tHàng\tNCC\tXác\n" +
		"Nhận:\n" +
		"not-a-date\n" +
		"Ngày\tĐặt\tHàng:\n" +
		"03-08-2026\n"
	_, _, _, ok := ParseOrderInfo(text)
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for a malformed cancelDate, want false")
	}
}

func TestNormalizeTabs_ReplacesTabsWithSpaces(t *testing.T) {
	got := normalizeTabs("PO\tNumber:\nKHO\tSEEDLOG")
	want := "PO Number:\nKHO SEEDLOG"
	if got != want {
		t.Errorf("normalizeTabs(...) = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/kingfood/... -v`
Expected: FAIL with a build error (package `kingfood` doesn't exist yet).

- [ ] **Step 3: Write the implementation**

Create `GO/internal/processing/kingfood/extract.go`:

```go
package kingfood

import (
	"regexp"
	"strings"
	"time"
)

var dateHyphenPattern = regexp.MustCompile(`^\d{2}-\d{2}-\d{4}$`)

// normalizeTabs corrects for a Go-PDF-library-specific quirk confirmed
// during planning by running this repo's own extractPageTexts against 3
// real Kingfood PDFs and cross-checking against PyMuPDF's output on the
// SAME files: Go's extraction inserts a literal tab character between
// words within a multi-word label line (e.g. "PO\tNumber:") where
// PyMuPDF inserts a plain space ("PO Number:"). Line-break positions are
// IDENTICAL between the two pipelines — only the intra-line word
// separator differs. Replacing tabs with spaces restores the exact
// space-separated shape Python's literal-space marker regexes
// (xulydonhang.py:9239,9243) assume.
func normalizeTabs(text string) string {
	return strings.ReplaceAll(text, "\t", " ")
}

func splitNonEmptyLines(text string) []string {
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			lines = append(lines, t)
		}
	}
	return lines
}

// valueAfterLabel mirrors po_number's and entry_date's real extraction
// shape (xulydonhang.py:9239-9247): find the line matching label
// exactly, return the line immediately after it. Python's regex
// (label + `\s*\n([^\n]*\n)?([^\n]*)` + `.group(1)`) always resolves to
// "the line immediately after the label line" for real Kingfood PDFs —
// confirmed during planning that there is no genuine blank-line gap in
// real data between these labels and their values — so this line-scan
// is the direct equivalent, not an approximation.
func valueAfterLabel(lines []string, label string) string {
	for i, l := range lines {
		if l == label && i+1 < len(lines) {
			return lines[i+1]
		}
	}
	return ""
}

// valueAfterMultilineLabel mirrors cancel_date's extraction
// (xulydonhang.py:9249-9257): the label "Ngày Giao Hàng NCC Xác Nhận:"
// itself spans TWO physical lines in the real PDF's own text layer —
// confirmed IDENTICAL in both PyMuPDF's and Go's extraction (this is a
// genuine line-wrap already present in the source PDF, not a Go-vs-
// Python divergence). Python's regex uses `\s*` between every word,
// which matches across the embedded newline; this checks whether lines
// i and i+1 together spell out the full label, and if so returns the
// line after i+1.
func valueAfterMultilineLabel(lines []string, labelPart1, labelPart2 string) string {
	for i := 0; i+2 < len(lines); i++ {
		if lines[i] == labelPart1 && lines[i+1] == labelPart2 {
			return lines[i+2]
		}
	}
	return ""
}

// parseKingfoodDate parses the PDF's real dd-mm-yyyy date format
// (hyphens) and reformats to dd/mm/yyyy (slashes), matching Python's own
// `.replace("-","/")` plus `datetime.strptime`/`.strftime` round-trip
// (xulydonhang.py:9244-9247,9257-9261). Python calls strptime with no
// try/except around it — a malformed date crashes Python outright; this
// returns ok=false instead, per this codebase's established policy.
func parseKingfoodDate(s string) (string, bool) {
	if !dateHyphenPattern.MatchString(s) {
		return "", false
	}
	t, err := time.Parse("02-01-2006", s)
	if err != nil {
		return "", false
	}
	return t.Format("02/01/2006"), true
}

// ParseOrderInfo mirrors the Kingfood branch of process_file
// (xulydonhang.py:9230-9263). Unlike FujiMart/Winmart/Emart, Kingfood
// has NO cross-validate/fallback ±N-day logic — a single unresolved
// marker or malformed date is unrecoverable, matching Python's own
// lack of a fallback path (Python would crash on datetime.strptime
// instead; this port fails cleanly).
func ParseOrderInfo(text string) (poNumber, entryDate, cancelDate string, ok bool) {
	lines := splitNonEmptyLines(normalizeTabs(text))

	poNumber = valueAfterLabel(lines, "PO Number:")

	rawEntryDate := valueAfterLabel(lines, "Ngày Đặt Hàng:")
	parsedEntryDate, entryOk := parseKingfoodDate(rawEntryDate)
	entryDate = parsedEntryDate

	rawCancelDate := valueAfterMultilineLabel(lines, "Ngày Giao Hàng NCC Xác", "Nhận:")
	parsedCancelDate, cancelOk := parseKingfoodDate(rawCancelDate)
	cancelDate = parsedCancelDate

	ok = poNumber != "" && entryOk && cancelOk
	return poNumber, entryDate, cancelDate, ok
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/kingfood/... -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/kingfood/extract.go GO/internal/processing/kingfood/extract_test.go
git commit -m "feat(go): add kingfood package with PO/date extraction and tab normalization"
```

---

### Task 3: `kingfood` package — product-table extraction

**Files:**
- Modify: `GO/internal/processing/kingfood/extract.go`
- Modify: `GO/internal/processing/kingfood/extract_test.go`

**Interfaces:**
- Consumes: nothing new (independent of Task 2's functions).
- Produces: `type Product struct { Barcode, OUQty, TotalPrice string }` and `ExtractProducts(text string) []Product` — consumed by Task 4's `processKingfoodSegment`.

- [ ] **Step 1: Write the failing tests**

Append to `GO/internal/processing/kingfood/extract_test.go`:

```go
func TestExtractProducts_ParsesRealSampleSingleProduct(t *testing.T) {
	// Exact shape of this repo's OWN extractPageTexts output for the
	// product-table region of a real single-product Kingfood PDF,
	// confirmed during planning by running the actual Go PDF pipeline —
	// including tab characters within the multi-word "Khu vực"/
	// "TỔNG CỘNG" markers and the product name line.
	text := "%\tHSD\n" +
		"Khu\tvực\n" +
		"1\n" +
		"8936156732620\n" +
		"BLUE\t-\tVIÊN\tGIẶT\tXẢ\tPHẤN\tHỒNG\tTÚI\n" +
		"30\tVIÊN\n" +
		"TÚI\n" +
		"300\n" +
		"12\n" +
		"25\tThùng\n" +
		"102.143\n" +
		"27%\n" +
		"0%\n" +
		"30%\n" +
		"52.195,073\n" +
		"8%\n" +
		"1.252.682\n" +
		"15.658.522\n" +
		"16.911.204\n" +
		"80%\n" +
		"Nhiệt\tđộ\n" +
		"phòng\n" +
		"TỔNG\tCỘNG\n" +
		"300\n"

	products := ExtractProducts(text)
	if len(products) != 1 {
		t.Fatalf("len(products) = %d, want 1", len(products))
	}
	want := Product{Barcode: "8936156732620", OUQty: "300", TotalPrice: "52.195,073"}
	if products[0] != want {
		t.Errorf("products[0] = %+v, want %+v", products[0], want)
	}
}

func TestExtractProducts_ParsesRealSampleTwoProducts(t *testing.T) {
	// Confirmed during planning: a real Kingfood PDF (PO1002586301) has
	// 2 distinct products in one order — this must loop correctly, not
	// just handle the single-product case.
	text := "Khu\tvực\n" +
		"1\n" +
		"8936156730992\n" +
		"BLUE\t-\tNƯỚC\tGIẶT\tXẢ\tĐẬM\tĐẶC\tTÚI\n" +
		"3.6\tL\n" +
		"TÚI\n" +
		"120\n" +
		"10\n" +
		"12\tThùng\n" +
		"85.000\n" +
		"20%\n" +
		"0%\n" +
		"10%\n" +
		"61.200,000\n" +
		"8%\n" +
		"500.000\n" +
		"6.500.000\n" +
		"7.000.000\n" +
		"90%\n" +
		"Nhiệt\tđộ\n" +
		"phòng\n" +
		"2\n" +
		"8936156732620\n" +
		"BLUE\t-\tVIÊN\tGIẶT\tXẢ\tPHẤN\tHỒNG\tTÚI\n" +
		"30\tVIÊN\n" +
		"TÚI\n" +
		"300\n" +
		"12\n" +
		"25\tThùng\n" +
		"102.143\n" +
		"27%\n" +
		"0%\n" +
		"30%\n" +
		"52.195,073\n" +
		"8%\n" +
		"1.252.682\n" +
		"15.658.522\n" +
		"16.911.204\n" +
		"80%\n" +
		"Nhiệt\tđộ\n" +
		"phòng\n" +
		"TỔNG\tCỘNG\n" +
		"420\n"

	products := ExtractProducts(text)
	if len(products) != 2 {
		t.Fatalf("len(products) = %d, want 2", len(products))
	}
	if products[0].Barcode != "8936156730992" {
		t.Errorf("products[0].Barcode = %q, want %q", products[0].Barcode, "8936156730992")
	}
	if products[1].Barcode != "8936156732620" {
		t.Errorf("products[1].Barcode = %q, want %q", products[1].Barcode, "8936156732620")
	}
}

func TestExtractProducts_NoTableMarkerReturnsEmpty(t *testing.T) {
	products := ExtractProducts("no khu vuc marker or tong cong anywhere in this text")
	if products != nil {
		t.Errorf("ExtractProducts = %v, want nil", products)
	}
}

func TestExtractProducts_NoEndMarkerReturnsEmpty(t *testing.T) {
	products := ExtractProducts("Khu\tvực\n1\n8936156732620\nsome text with no end marker\n")
	if products != nil {
		t.Errorf("ExtractProducts = %v, want nil", products)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/kingfood/... -run TestExtractProducts -v`
Expected: FAIL with a build error (`Product`/`ExtractProducts` don't exist yet).

- [ ] **Step 3: Write the implementation**

Append to `GO/internal/processing/kingfood/extract.go`:

```go
// tableStartPattern mirrors lamsachdonhang_kingfood's start marker
// (xulydonhang.py:6674): case-insensitive "Khu vực", the last column
// header immediately before the first product row. Applied to
// TAB-NORMALIZED text (Go's raw extraction has "Khu\tvực", a tab
// between the two words, same class of divergence as the PO/date
// labels in ParseOrderInfo).
var tableStartPattern = regexp.MustCompile(`(?i)Khu vực`)

// productLinePattern mirrors laydanhsachsanpham_kingfood's compiled
// regex (xulydonhang.py:6707-6724) exactly: 6 capturing groups — STT,
// 13-digit barcode, a non-greedy multi-line product name, unit (one of
// a fixed 5-word set), quantity, then a fixed 4-line skip block before
// the final price field. Go has no re.VERBOSE mode; the pattern below
// is the same shape with the VERBOSE-only whitespace/comments removed.
// No re.MULTILINE equivalent needed — the pattern never references ^/$.
var productLinePattern = regexp.MustCompile(`(\d+)\s*\n(\d{13})\s*\n((?:.+\n)+?)(HỘP|TÚI|CHAI|LON|GÓI)\s*\n([\d.]+)\s*\n\d+\s*\n.+\s*\n(?:.*\n){4}([0-9.,]+)`)

// Product is one extracted Kingfood product line. Only Barcode, OUQty,
// and TotalPrice are used downstream by processKingfoodSegment — Python
// captures "Product Name"/Unit too (xulydonhang.py:6752-6758) but
// write_to_dondathang_kingfood never reads them (product name is always
// re-looked-up via timten_sanpham, xulydonhang.py:3946), so this struct
// omits them entirely.
//
// TotalPrice keeps the field's Python name even though the value is
// actually a PER-UNIT final price (post-discount), not a line total —
// see processKingfoodSegment's own doc comment (Task 4) for the full
// explanation; this struct intentionally does NOT rename the field, to
// stay traceable to the source column name "Total Price" in
// laydanhsachsanpham_kingfood's own dict.
//
// TotalPrice is left as a RAW string (e.g. "52.195,073", Vietnamese/
// European number format: period=thousands, comma=decimal) — NOT
// parsed here. parseKingfoodPrice (kingfood_processor.go, Task 4)
// converts it; this package has no float-parsing dependency.
type Product struct {
	Barcode    string
	OUQty      string
	TotalPrice string
}

// extractProductTable mirrors lamsachdonhang_kingfood (xulydonhang.py:
// 6672-6695): find the FIRST "Khu vực" (case-insensitive, xulydonhang.py's
// own re.search takes the first match, not the last — confirmed by
// direct reading, no rfind used here unlike FujiMart's marker), take
// everything after it, then cut at the first "TỔNG CỘNG" that begins its
// own line (Python's `(?<=\n)TỔNG CỘNG` lookbehind — Go's RE2 has no
// lookbehind support, so this searches for the literal "\nTỔNG CỘNG"
// substring instead, equivalent for this purpose). If either marker is
// missing, Python returns the literal string "Không có sản phẩm" (never
// crashes); this returns "" instead, treated as "no products" by
// ExtractProducts.
func extractProductTable(text string) string {
	normalized := normalizeTabs(text)
	loc := tableStartPattern.FindStringIndex(normalized)
	if loc == nil {
		return ""
	}
	after := normalized[loc[1]:]
	endIdx := strings.Index(after, "\nTỔNG CỘNG")
	if endIdx < 0 {
		return ""
	}
	return strings.TrimSpace(after[:endIdx])
}

// ExtractProducts mirrors laydanhsachsanpham_kingfood
// (xulydonhang.py:6698-6758) plus the table-isolation step that always
// runs immediately before it (lamsachdonhang_kingfood, xulydonhang.py:
// 6672-6695, called from :6700).
func ExtractProducts(text string) []Product {
	table := extractProductTable(text)
	if table == "" {
		return nil
	}

	var products []Product
	for _, m := range productLinePattern.FindAllStringSubmatch(table, -1) {
		products = append(products, Product{
			Barcode:    m[2],
			OUQty:      strings.ReplaceAll(m[5], ".", ""),
			TotalPrice: m[6],
		})
	}
	return products
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/kingfood/... -v`
Expected: PASS, all tests.

- [ ] **Step 5: Verify against a REAL sample PDF's actual Go-extracted text, not just the literal test strings above**

The test strings above were transcribed from this repo's own `extractPageTexts` output against real samples, captured during planning — but transcription errors are possible, and the 2-product case was constructed by hand (only its barcodes/quantities are independently confirmed real; the rest of that specific test's numeric fields are illustrative, not verified against an actual PDF's exact printed values). Run a throwaway scratch test (or `go run` snippet, deleted before committing) calling `extractPageTexts` (package `processing`, not `kingfood`) directly against at least 2 real files — e.g. `đơn hàng/mẫu đơn hàng/03-08-2026/03-08-2026_[Kingfood][03-08-2026][MN_MT_KFMSL][05-08-2026][PO1002601888].pdf` (1 product) and `đơn hàng/mẫu đơn hàng/20-07-2026/20-07-2026_[Kingfood][20-07-2026][MN_MT_KFMSL][22-07-2026][PO1002586301].pdf` (2 products) — then feed that real output through `kingfood.ExtractProducts`. Confirm the barcode count and values match what's printed in the real PDF (open it visually if needed to cross-check). If the real extraction doesn't match, that's a real bug in the regex to fix — do not adjust the test to match incorrect real-PDF behavior instead. Remove the scratch code before committing — do not leave it in the repo.

- [ ] **Step 6: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/kingfood/extract.go GO/internal/processing/kingfood/extract_test.go
git commit -m "feat(go): add product-table extraction to kingfood package"
```

---

### Task 4: `kingfood_processor.go` (dispatch + row builder)

**Files:**
- Create: `GO/internal/processing/kingfood_processor.go`
- Create: `GO/internal/processing/kingfood_processor_test.go`
- Modify: `GO/internal/processing/coop_processor.go` (add `case "Kingfood":` INSERTED between the existing `case "Emart":` and `case "Winmart":` blocks)
- Create: `GO/internal/processing/testdata/sample_kingfood_order.pdf` (copy from `đơn hàng/mẫu đơn hàng/03-08-2026/03-08-2026_[Kingfood][03-08-2026][MN_MT_KFMSL][05-08-2026][PO1002601888].pdf`)

**Interfaces:**
- Consumes: `kingfood.ParseOrderInfo`, `kingfood.Product`, `kingfood.ExtractProducts` (Tasks 2-3); `buildPromoBonusRow`, `buildInvoiceBonusRow`, `coopDebtDays`, `closeEnough`, `parseNumericField` (existing shared helpers from `processor_shared.go`/`bigc_processor.go`).
- Produces: `processKingfoodSegment`, `kingfoodRegionInfo`, `kingfoodOrderNumber`, `parseKingfoodPrice` — consumed by Task 6's golden test only indirectly (via `RealProcessor.Process`).

- [ ] **Step 1: Copy the sample PDF**

```bash
cp "đơn hàng/mẫu đơn hàng/03-08-2026/03-08-2026_[Kingfood][03-08-2026][MN_MT_KFMSL][05-08-2026][PO1002601888].pdf" GO/internal/processing/testdata/sample_kingfood_order.pdf
```

Verify byte-identical: `cmp "đơn hàng/mẫu đơn hàng/03-08-2026/03-08-2026_[Kingfood][03-08-2026][MN_MT_KFMSL][05-08-2026][PO1002601888].pdf" GO/internal/processing/testdata/sample_kingfood_order.pdf`.

If this exact path is no longer available (the live app may have reorganized the archive further), search `đơn hàng/mẫu đơn hàng/*/` for any filename containing `PO1002601888` and use that instead — note the actual path used in the task report.

- [ ] **Step 2: Write the failing processor tests**

Create `GO/internal/processing/kingfood_processor_test.go`:

```go
package processing

import (
	"context"
	"testing"

	"github.com/xuri/excelize/v2"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleKingfoodFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	pricingSource := &fixturePricingSource{index: pricing.ParseIndex([][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
	})}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_kingfood_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].System != "Kingfood" {
		t.Fatalf("System = %q, want %q", rows[0].System, "Kingfood")
	}
	if rows[0].StatusKind == StatusKindFailed {
		t.Fatalf("Process produced a Failed row: %+v", rows[0])
	}
	if rows[0].MaKhachHang != kingfoodCustomerCode {
		t.Fatalf("MaKhachHang = %q, want the hardcoded constant %q", rows[0].MaKhachHang, kingfoodCustomerCode)
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening written workbook: %v", err)
	}
	defer f.Close()
	sheetRows, err := f.GetRows("Don dat hang")
	if err != nil {
		t.Fatalf("failed reading Don dat hang rows: %v", err)
	}
	// 1 header + 1 product = 2 new rows (the real sample has 1 product);
	// no promo bonus row expected since the synthetic pricing source
	// above has no real promo data.
	if len(sheetRows) != 8+2 {
		t.Fatalf("total rows = %d, want %d (8 template + 1 header + 1 product)", len(sheetRows), 8+2)
	}
}

func TestKingfoodRegionInfo(t *testing.T) {
	cases := []struct {
		name                                     string
		customerCode                             string
		wantRegion, wantStatCode, wantWarehouse string
	}{
		{
			name:          "the real, always-used hardcoded constant (else branch)",
			customerCode:  kingfoodCustomerCode,
			wantRegion:    "MT_MN",
			wantStatCode:  "LA",
			wantWarehouse: "LA_KHO2026",
		},
		{
			name:          "MB branch (unreachable with real input today, still tested)",
			customerCode:  "MB_SOMETHING",
			wantRegion:    "MT_MB",
			wantStatCode:  "HN",
			wantWarehouse: "TP_HN_12",
		},
		{
			name:          "MN_MT_JM0001 exact-match branch (unreachable with real input today, still tested)",
			customerCode:  "MN_MT_JM0001",
			wantRegion:    "MT_MN",
			wantStatCode:  "LA",
			wantWarehouse: "LA_TP",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRegion, gotStatCode, gotWarehouse := kingfoodRegionInfo(tc.customerCode)
			if gotRegion != tc.wantRegion || gotStatCode != tc.wantStatCode || gotWarehouse != tc.wantWarehouse {
				t.Errorf("kingfoodRegionInfo(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.customerCode, gotRegion, gotStatCode, gotWarehouse,
					tc.wantRegion, tc.wantStatCode, tc.wantWarehouse)
			}
		})
	}
}

func TestParseKingfoodPrice(t *testing.T) {
	cases := []struct {
		input string
		want  float64
	}{
		{"52.195,073", 52195.073},
		{"1.252.682", 1252682},
		{"85.000", 85000},
		{"0", 0},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got := parseKingfoodPrice(c.input)
			if got != c.want {
				t.Errorf("parseKingfoodPrice(%q) = %v, want %v", c.input, got, c.want)
			}
		})
	}
}

// TestRealProcessor_KingfoodNoBraceBonusRowDoesNotWriteAP regression-tests
// Kingfood's own no-{...}-brace fallback text ("KM Giao Rời - Không Che
// Barcode", xulydonhang.py:4096) — the shared buildPromoBonusRow's
// default fallback ("KM Bó Kèm - Che Barcode") must be overridden at
// this call site, AND (unlike FujiMart, like Winmart/Emart) column AP
// must be cleared, not left at buildPromoBonusRow's own default.
//
// Uses sample_kingfood_order.pdf's real single product (barcode
// 8936156732620, OU Qty 300, price "52.195,073" -> parseKingfoodPrice =
// 52195.073 — confirmed by direct extraction during planning) with a
// "2+1 SP0002" promo (an "X+1" match mentioning SP0002, a known internal
// SKU already present in the productdata test fixture) and NO {...}
// braces.
func TestRealProcessor_KingfoodNoBraceBonusRowDoesNotWriteAP(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	const promoValue = "2+1 SP0002"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156732620", "Viên giặt xả", "52195.073", promoValue},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_kingfood_order.pdf", 1); err != nil {
		t.Fatalf("Process returned error: %v", err)
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening written workbook: %v", err)
	}
	defer f.Close()
	sheetRows, err := f.GetRows("Don dat hang")
	if err != nil {
		t.Fatalf("failed reading Don dat hang rows: %v", err)
	}

	const colSKU, colPromoNote, colPromoBundleSku = 16, 40, 41
	cell := func(row []string, idx int) string {
		if idx < len(row) {
			return row[idx]
		}
		return ""
	}

	var mainRow, bonusRow []string
	for _, row := range sheetRows {
		switch cell(row, colSKU) {
		case "8936156732620":
			mainRow = row
		case "SP0002":
			bonusRow = row
		}
	}
	if mainRow == nil || bonusRow == nil {
		t.Fatalf("missing expected rows: main=%v bonus=%v", mainRow, bonusRow)
	}

	if got := cell(mainRow, colPromoNote); got != "KM Giao Rời - Không Che Barcode" {
		t.Errorf("main row PromoNote (AO) = %q, want %q (Kingfood's own no-brace fallback)", got, "KM Giao Rời - Không Che Barcode")
	}
	if got := cell(mainRow, colPromoBundleSku); got != "" {
		t.Errorf("main row PromoBundleSku (AP) = %q, want empty (Kingfood's no-brace branch does NOT write AP, matching Winmart/Emart, unlike FujiMart)", got)
	}
	if got := cell(bonusRow, colPromoBundleSku); got != "" {
		t.Errorf("bonus row PromoBundleSku (AP) = %q, want empty", got)
	}
}

// TestRealProcessor_KingfoodInvoiceLevelPromoBonusRow covers the
// invoice-level ("Hóa Đơn") promo bonus row (xulydonhang.py:4131-4177)
// — Q gets only the FIRST mentioned SKU, not a joined list, same
// divergence already handled for Winmart/Emart/FujiMart.
func TestRealProcessor_KingfoodInvoiceLevelPromoBonusRow(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156732620", "Viên giặt xả", "52195.073", ""},
		{"2", "Hóa Đơn", "", "0", "100000 SP0001 SP0002"},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_kingfood_order.pdf", 1); err != nil {
		t.Fatalf("Process returned error: %v", err)
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening written workbook: %v", err)
	}
	defer f.Close()
	sheetRows, err := f.GetRows("Don dat hang")
	if err != nil {
		t.Fatalf("failed reading Don dat hang rows: %v", err)
	}

	const colSKU, colIsPromoItem, colQty = 16, 20, 23
	cell := func(row []string, idx int) string {
		if idx < len(row) {
			return row[idx]
		}
		return ""
	}

	var bonusRow []string
	for _, row := range sheetRows {
		if cell(row, colSKU) == "SP0001" {
			bonusRow = row
			break
		}
	}
	if bonusRow == nil {
		t.Fatalf("no row with SKU (Q) = %q found; sheet rows: %+v", "SP0001", sheetRows)
	}
	if got := cell(bonusRow, colIsPromoItem); got != "Có" {
		t.Errorf("invoice bonus row IsPromoItem (U) = %q, want %q", got, "Có")
	}
	wantQty := "156" // floor(300 (OU Qty) * 52195.073 (unit price) / 100000)
	if got := cell(bonusRow, colQty); got != wantQty {
		t.Errorf("invoice bonus row Qty (X) = %q, want %q (floor(totalValue / amount))", got, wantQty)
	}

	for _, row := range sheetRows {
		if cell(row, colSKU) == "SP0002" {
			t.Errorf("found a row with SKU (Q) = %q, want none (only the first mentioned SKU, SP0001, should get an invoice bonus row)", "SP0002")
		}
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_ProcessesRealSampleKingfoodFile|TestKingfoodRegionInfo|TestParseKingfoodPrice|TestRealProcessor_KingfoodNoBraceBonusRowDoesNotWriteAP|TestRealProcessor_KingfoodInvoiceLevelPromoBonusRow" -v`
Expected: FAIL with a build error (`processKingfoodSegment`/`kingfoodRegionInfo`/`parseKingfoodPrice`/`kingfoodCustomerCode` don't exist yet, and `vendor.Identify` isn't wired into the dispatch switch).

- [ ] **Step 4: Write `kingfood_processor.go`**

Create `GO/internal/processing/kingfood_processor.go`:

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
	"order-processor/internal/processing/kingfood"
)

// kingfoodCustomerCode mirrors write_to_dondathang_kingfood's default/
// only makhachhang value — the literal "MN_MT_KFMSL", hardcoded at the
// process_file call site (xulydonhang.py:9273). Kingfood never derives a
// customer code from the PDF.
const kingfoodCustomerCode = "MN_MT_KFMSL"

// kingfoodDeliveryAddress mirrors write_to_dondathang_kingfood's default/
// only delivery value — hardcoded at the process_file call site
// (xulydonhang.py:9274), never extracted from the PDF even though the
// same string DOES appear in the PDF's own "Địa chỉ giao hàng:" field
// (confirmed during planning) — Python simply doesn't read it from
// there.
const kingfoodDeliveryAddress = "Số 324, đường ĐT743A, Phường Đông Hoà, Thành phố Hồ Chí Minh"

// kingfoodRegionInfo mirrors write_to_dondathang_kingfood's warehouse/
// region branching (xulydonhang.py:3871-3883) — a genuine 3-way split,
// the first in this project (FujiMart/Winmart only ever had 2). The MB
// and "MN_MT_JM0001" branches are unreachable with real Kingfood input
// today — customerCode is always the hardcoded constant
// kingfoodCustomerCode, which is neither "MB"-prefixed nor exactly
// "MN_MT_JM0001" — but this is modeled as a full 3-branch function
// anyway, matching the fujimartRegionInfo/winmartRegionInfo precedent,
// for architectural consistency.
func kingfoodRegionInfo(customerCode string) (region, statCode, warehouse string) {
	switch {
	case strings.HasPrefix(customerCode, "MB"):
		return "MT_MB", "HN", "TP_HN_12"
	case customerCode == "MN_MT_JM0001":
		return "MT_MN", "LA", "LA_TP"
	default:
		return "MT_MN", "LA", "LA_KHO2026"
	}
}

// kingfoodOrderNumber mirrors write_to_dondathang_kingfood's order-
// number field (xulydonhang.py:3899): f'ĐĐH{vendor}{STT_donhang_str}'
// where vendor is the uppercased literal "KINGFOOD" and
// STT_donhang_str is f"-{po_number}".
func kingfoodOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHKINGFOOD-%s", poNumber)
}

// parseKingfoodPrice mirrors laydanhsachsanpham_kingfood's price
// parsing (xulydonhang.py:6744): price_str.replace('.', '').replace(',', '.')
// — Vietnamese/European number format (period = thousands separator,
// comma = decimal separator), the OPPOSITE convention from every other
// vendor in this project (which only ever strip commas, US-style
// thousands, with no decimal-comma). NOT a drop-in replacement for the
// shared parseNumericField helper — scoped to Kingfood's price field
// only; Kingfood's quantity field uses parseNumericField as usual (only
// strips periods, matching Python's quantity.replace('.', '')).
func parseKingfoodPrice(s string) float64 {
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// processKingfoodSegment mirrors the Kingfood branch of process_file
// (xulydonhang.py:9230-9310) plus write_to_dondathang_kingfood
// (:3848-4196). Kingfood is "1 page = 1 order", the same family as
// Coop/Lotte/Satra/Winmart/Emart/FujiMart. A trailing PDF page that
// lacks Kingfood's identify marker falls through to the shared per-page
// dispatch loop's default case (coop_processor.go), which emits a
// Failed/"Thất bại" OrderRow for that page.
func (p *RealProcessor) processKingfoodSegment(filePath, text, pageLabel string) (OrderRow, error) {
	poNumber, entryDate, cancelDate, ok := kingfood.ParseOrderInfo(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được số PO/ngày đặt hàng")
	}

	products := kingfood.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	priceIndex, err := p.Pricing.FetchIndex("KINGFOOD")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := kingfoodRegionInfo(kingfoodCustomerCode)
	orderNum := kingfoodOrderNumber(poNumber)
	description := fmt.Sprintf("KINGFOOD %s", poNumber)

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: kingfoodDeliveryAddress, CustomerCode: kingfoodCustomerCode,
		Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: description,
	})

	saigia := 0
	totalWeight := 0.0
	totalValue := 0.0

	for _, rawProduct := range products {
		barcode := p.Store.ResolveSku(rawProduct.Barcode)
		productInfo, _ := p.Store.GetProductInfo(barcode)
		ouQty := parseNumericField(rawProduct.OUQty)

		// invoicePrice (giahoadon = dongia, xulydonhang.py:3925,3970): the
		// "Total Price" field from ExtractProducts is actually a PER-UNIT
		// final price (post-discount) for Kingfood, NOT a line total —
		// see kingfood.Product's own doc comment. Used DIRECTLY, no
		// division by ouQty (unlike FujiMart/Winmart, whose TotalPrice IS
		// a line total).
		invoicePrice := parseKingfoodPrice(rawProduct.TotalPrice)

		lineWeight := productInfo.WeightKg * ouQty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(ouQty / productInfo.PackSize))
		}
		totalWeight += lineWeight

		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice := parseNumericField(realPriceStr)

		promos := priceIndex.FindPromotions(barcode, entryDate)
		khuyenmai := ""
		matched := false
		finalPrice := realPrice

		for _, promo := range promos {
			// CR normalization (openpyxl-vs-excelize \r round-trip
			// divergence, same class of fix already shipped for BigC/
			// Emart/FujiMart) — applied here from the FIRST commit, not
			// added later as a fix-round patch (FujiMart's final review
			// caught exactly this omission on its own invoice-level
			// block; Kingfood gets both call sites right from the start).
			value := strings.ReplaceAll(promo.Value, "\r", "\n")
			if value == "" {
				continue
			}
			khuyenmai = value
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
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: kingfoodDeliveryAddress, CustomerCode: kingfoodCustomerCode,
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

		// Per-item promo bonus row (xulydonhang.py:4074-4128) — single
		// attempt, buildPromoBonusRow always called with index=0 (no
		// "|"-split multi-CTKM loop).
		bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, khuyenmai,
			coop.Product{Barcode: barcode, Qty: ouQty}, 0, entryDate, cancelDate, kingfoodDeliveryAddress,
			kingfoodCustomerCode, description, warehouse, region, statCode, orderNum)
		if added {
			totalWeight += bonusRow.LineWeightKg

			// Kingfood's own no-{...}-brace fallback text
			// (xulydonhang.py:4096, "KM Giao Rời - Không Che Barcode")
			// differs from buildPromoBonusRow's shared Coop-flavored
			// default ("KM Bó Kèm - Che Barcode"). Unlike FujiMart's
			// equivalent fallback (which still writes AP because its own
			// text contains "bó kèm"), Kingfood's fallback text does NOT
			// contain "bó kèm"/"quấn kèm", so this ALSO needs to
			// explicitly clear AP — matching Winmart's/Emart's identical
			// fix, confirmed against xulydonhang.py:4092-4096 (only the
			// cachbokem branch writes AP; the else/fallback branch never
			// does).
			if coop.ExtractBraceContent(khuyenmai) == "" {
				mainRowNote = "KM Giao Rời - Không Che Barcode"
				mainRowBundleSku = ""
				bonusRow.PromoBundleSku = ""
			}

			rows[productRowIndex].PromoNote = mainRowNote
			if mainRowBundleSku != "" {
				rows[productRowIndex].PromoBundleSku = mainRowBundleSku
			}
			rows = append(rows, bonusRow)
		}
	}

	// Invoice-level ("Hóa Đơn") promo bonus row (xulydonhang.py:4131-4177).
	// Does NOT reuse the shared buildInvoiceBonusRow — Q gets only the
	// first matched SKU (kiemtra[0]), not a joined list, the same
	// divergence already handled for Winmart/Emart/FujiMart.
	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		invoicePromo = strings.ReplaceAll(invoicePromo, "\r", "\n")
		invoiceSkus := p.Store.FindSkusMentioned(invoicePromo)
		if amount, ok := coop.ExtractMoneyAmount(invoicePromo); ok && amount > 0 && len(invoiceSkus) > 0 {
			invoiceSku := invoiceSkus[0] // xulydonhang.py:4147 — kiemtra[0], not a joined list
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
				invoiceNote = "KM Bó Kèm - Che Barcode" // xulydonhang.py:4171
			}

			rows = append(rows, excelwriter.Row{
				EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
				Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: kingfoodDeliveryAddress, CustomerCode: kingfoodCustomerCode,
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
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Kingfood", MaKhachHang: kingfoodCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
	}, nil
}
```

- [ ] **Step 5: Wire the `case "Kingfood":` into `RealProcessor.Process`'s dispatch switch, INSERTED between Emart and Winmart**

In `GO/internal/processing/coop_processor.go`, find the existing `case "Emart":` block (it ends with `rows = append(rows, row)` followed by a blank line, immediately before `case "Winmart":`). Insert the new `case "Kingfood":` block in that gap, so the switch reads `... case "Emart": ... case "Kingfood": ... case "Winmart": ...`:

```go
		case "Kingfood":
			row, err := p.processKingfoodSegment(filePath, text, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Kingfood",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_ProcessesRealSampleKingfoodFile|TestKingfoodRegionInfo|TestParseKingfoodPrice|TestRealProcessor_KingfoodNoBraceBonusRowDoesNotWriteAP|TestRealProcessor_KingfoodInvoiceLevelPromoBonusRow" -v`
Expected: PASS, all tests.

Also run the full existing suite to confirm no other vendor regressed:
Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures" -v`
Expected: every other vendor's own suite unaffected by this task's changes (their pass/fail state may currently reflect the live `đơn hàng/` folder's own unrelated availability — see this plan's Global Constraints and prior vendors' plan history; confirm via `git stash` if there's any doubt whether THIS task's own commit changed anything).

- [ ] **Step 7: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/kingfood_processor.go GO/internal/processing/kingfood_processor_test.go GO/internal/processing/coop_processor.go GO/internal/processing/testdata/sample_kingfood_order.pdf
git commit -m "feat(go): dispatch RealProcessor to Kingfood via processKingfoodSegment"
```

---

### Task 5: Copy real PDFs into stable testdata + golden fixture generation script (throwaway)

**Files:**
- Create: `GO/internal/processing/kingfood/testdata/realpdfs/*.pdf` (9 files, copied)
- Create: `GO/internal/processing/kingfood/testdata/generate_fixtures.py`

**Interfaces:**
- Consumes: real, unmodified `xulydonhang.py` (repo root) — never modified by this task.
- Produces: `GO/internal/processing/kingfood/testdata/fixtures/*.json` + `_frozen_pricing.json` — consumed by Task 6.

**This decision needs explicit project-owner confirmation before this task's commit** (do not assume the same answer carries over automatically from Emart/FujiMart): commit the 9 currently-available real Kingfood PDFs into a stable, git-tracked local directory, rather than reading from the live `đơn hàng/` tree at test-run time (demonstrated repeatedly in this project to be an unstable dependency for the other 5 vendors' golden tests).

- [ ] **Step 1: Copy the 9 real PDFs into stable testdata**

Create `GO/internal/processing/kingfood/testdata/realpdfs/` and copy each of the 9 files below from its current location to a clean `<PONumber>.pdf` name in that directory. Use these EXACT source paths (verified to exist during planning — re-verify each still exists before copying, since the archive tree could have moved again by the time this task runs; if it has, search `đơn hàng/mẫu đơn hàng/*/` for a `[Kingfood]`-tagged file with the same PO number in brackets, the same technique already used for FujiMart):

```
"đơn hàng/mẫu đơn hàng/03-08-2026/03-08-2026_[Kingfood][03-08-2026][MN_MT_KFMSL][05-08-2026][PO1002601888].pdf" -> GO/internal/processing/kingfood/testdata/realpdfs/PO1002601888.pdf
"đơn hàng/mẫu đơn hàng/06-08-2026/06-08-2026_[Kingfood][06-08-2026][MN_MT_KFMSL][08-08-2026][PO1002605686].pdf" -> GO/internal/processing/kingfood/testdata/realpdfs/PO1002605686.pdf
"đơn hàng/mẫu đơn hàng/10-07-2026/10-07-2026_[Kingfood][10-07-2026][MN_MT_KFMSL][13-07-2026][PO1002575355].pdf" -> GO/internal/processing/kingfood/testdata/realpdfs/PO1002575355.pdf
"đơn hàng/mẫu đơn hàng/13-07-2026/13-07-2026_[Kingfood][13-07-2026][MN_MT_KFMSL][15-07-2026][PO1002578376].pdf" -> GO/internal/processing/kingfood/testdata/realpdfs/PO1002578376.pdf
"đơn hàng/mẫu đơn hàng/13-08-2026/13-08-2026_[Kingfood][10-08-2026][MN_MT_KFMSL][14-08-2026][PO1002610000].pdf" -> GO/internal/processing/kingfood/testdata/realpdfs/PO1002610000.pdf
"đơn hàng/mẫu đơn hàng/16-07-2026/16-07-2026_[Kingfood][16-07-2026][MN_MT_KFMSL][18-07-2026][PO1002582369].pdf" -> GO/internal/processing/kingfood/testdata/realpdfs/PO1002582369.pdf
"đơn hàng/mẫu đơn hàng/20-07-2026/20-07-2026_[Kingfood][20-07-2026][MN_MT_KFMSL][22-07-2026][PO1002586301].pdf" -> GO/internal/processing/kingfood/testdata/realpdfs/PO1002586301.pdf
"đơn hàng/mẫu đơn hàng/28-07-2026/28-07-2026_[Kingfood][27-07-2026][MN_MT_KFMSL][29-07-2026][PO1002594163].pdf" -> GO/internal/processing/kingfood/testdata/realpdfs/PO1002594163.pdf
"đơn hàng/mẫu đơn hàng/30-07-2026/30-07-2026_[Kingfood][30-07-2026][MN_MT_KFMSL][01-08-2026][PO1002597903].pdf" -> GO/internal/processing/kingfood/testdata/realpdfs/PO1002597903.pdf
```

These are COPIES (read-only source access) — do not move, rename, or modify anything under `đơn hàng/` itself. `đơn hàng/` is git-ignored (`.gitignore:19`, `**/đơn hàng/`) so nothing under it is trackable or committed by this repo anyway — only the new copies under `GO/internal/processing/kingfood/testdata/realpdfs/` (not gitignored) will be committed.

Verify each copy is byte-identical to its source (`cmp` or equivalent) before proceeding. The `PO1002601888.pdf` copy should also be byte-identical to Task 4's already-committed `GO/internal/processing/testdata/sample_kingfood_order.pdf` — confirm this too.

- [ ] **Step 2: Write the fixture-generation script**

Create `GO/internal/processing/kingfood/testdata/generate_fixtures.py`. Adapted from the same base as FujiMart's harness (`GO/internal/processing/fujimart/testdata/generate_fixtures.py` — read it in full first) with the same structural shape: reads PDFs from the stable local `realpdfs/` directory, same backup/restore protocol, retry-with-backoff, UTF-8 stdout fix, pricing/promotion monkeypatch caching. Kingfood needs NO OCR step (unlike FujiMart) — simpler harness.

```python
"""
Throwaway dev tool — NOT part of the shipped Go or Python app.

Runs the CURRENT xulydonhang.py Kingfood pipeline against the 9 real
Kingfood PDFs copied into
GO/internal/processing/kingfood/testdata/realpdfs/ (see this task's own
Step 1 — committed directly into this repo's testdata from the start,
per explicit project-owner confirmation for this vendor), capturing the
resulting dondathang.xlsx rows (and the live-fetched Google Sheets
price/promotion data for the KINGFOOD sheet) into JSON fixtures under
GO/internal/processing/kingfood/testdata/fixtures/. The Go golden test
(Task 6) diffs RealProcessor's output against these fixtures instead of
against a live Google Sheets fetch, so it's deterministic and offline.

Like Satra/Lotte/Winmart/Emart/FujiMart (one page == one order,
write_to_dondathang_kingfood appends immediately, no explicit start-row
argument needed), and UNLIKE BigC, this harness computes start_row once
up front and takes a single snapshot after process_one_pdf's per-page
loop has finished.

Run from the repo root:
    .venv/Scripts/python.exe GO/internal/processing/kingfood/testdata/generate_fixtures.py
"""
import glob
import json
import os
import shutil
import sys
import time

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))))
sys.path.insert(0, REPO_ROOT)
os.chdir(REPO_ROOT)  # xulydonhang.py's functions use relative paths ("data.xlsx", "settings.ini")

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="backslashreplace")
if hasattr(sys.stderr, "reconfigure"):
    sys.stderr.reconfigure(encoding="utf-8", errors="backslashreplace")

import openpyxl  # noqa: E402
import xulydonhang  # noqa: E402

REALPDFS_DIR = os.path.join(
    REPO_ROOT, "GO", "internal", "processing", "kingfood", "testdata", "realpdfs"
)
FIXTURES_DIR = os.path.join(
    REPO_ROOT, "GO", "internal", "processing", "kingfood", "testdata", "fixtures"
)
TEMPLATE_XLSX = os.path.join(REPO_ROOT, "dondathang.xlsx")
SCRATCH_XLSX = os.path.join(REPO_ROOT, "dondathang_fixture_scratch.xlsx")

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


def process_one_pdf(path):
    """Mirrors the Kingfood branch of process_file (xulydonhang.py:9230-
    9310) for every page identify_vendor recognizes as Kingfood, skipping
    the Google Drive upload side effect (monkeypatched to a no-op above).
    No OCR needed for Kingfood (unlike FujiMart's harness)."""
    handler = xulydonhang.process_handler
    doc = xulydonhang.fitz.open(path)
    try:
        for page_num in range(len(doc)):
            text = doc[page_num].get_text("text")
            if xulydonhang.ProcessHandler.identify_vendor(text) != "Kingfood":
                continue

            import re
            from datetime import datetime

            tranggoc = doc[0].get_text("text")

            po_number = re.search(r"PO Number:\s*\n([^\n]*\n)?([^\n]*)", tranggoc)
            po_number = po_number.group(1).strip() if po_number else "Không tìm thấy PO Number"

            entry_date = re.search(r"Ngày Đặt Hàng:\s*\n([^\n]*\n)?([^\n]*)", tranggoc)
            entry_date = entry_date.group(1).replace("-", "/").strip() if entry_date else "Không tìm thấy ngày đặt hàng"
            entry_date = datetime.strptime(entry_date, "%d/%m/%Y")
            entry_date = entry_date.strftime("%d/%m/%Y")

            cancel_date = re.search(
                r"Ngày\s*Giao\s*Hàng\s*NCC\s*Xác\s*Nhận:\s*\n*([^\n]*\n)?([^\n]*)",
                tranggoc,
                re.IGNORECASE,
            )
            cancel_date = cancel_date.group(1).replace("-", "/").strip() if cancel_date else "Không tìm thấy ngày giao hàng"
            cancel_date = datetime.strptime(cancel_date, "%d/%m/%Y")
            cancel_date = cancel_date.strftime("%d/%m/%Y")

            products = xulydonhang.ProcessHandler.laydanhsachsanpham_kingfood(text)
            if not products:
                continue
            sku_mapping = xulydonhang.ProcessHandler.load_sku_mapping()
            products = xulydonhang.ProcessHandler.replace_sku_numbers(products, sku_mapping)

            store_code = "MN_MT_KFMSL"
            delivery = "Số 324, đường ĐT743A, Phường Đông Hoà, Thành phố Hồ Chí Minh"

            xulydonhang.ProcessHandler.write_to_dondathang_kingfood(
                handler, products, store_code, po_number, entry_date, cancel_date,
                1, "Kingfood", delivery, None,
            )
    finally:
        doc.close()


def _remove_with_retry(path, attempts=5, delay=0.5):
    for i in range(attempts):
        try:
            os.remove(path)
            return
        except PermissionError:
            if i == attempts - 1:
                raise
            time.sleep(delay)


def _move_with_retry(src, dst, attempts=5, delay=0.5):
    for i in range(attempts):
        try:
            shutil.move(src, dst)
            return
        except PermissionError:
            if i == attempts - 1:
                raise
            time.sleep(delay)


def main():
    os.makedirs(FIXTURES_DIR, exist_ok=True)

    pdf_paths = sorted(set(
        glob.glob(os.path.join(REALPDFS_DIR, "*.pdf")) +
        glob.glob(os.path.join(REALPDFS_DIR, "*.PDF"))
    ))
    print(f"Found {len(pdf_paths)} candidate PDFs in {REALPDFS_DIR}")

    generated = 0
    skipped = 0
    for path in pdf_paths:
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
            _remove_with_retry(real_target)
            _move_with_retry(backup, real_target)
            if os.path.exists(SCRATCH_XLSX):
                os.remove(SCRATCH_XLSX)

        if rows is None or len(rows) == 0:
            if rows is not None:
                print(f"SKIP (no rows written, likely no products extracted) {os.path.basename(path)}")
                skipped += 1
            continue

        fixture = {"source_pdf": os.path.basename(path), "rows": rows}
        fixture_name = os.path.splitext(os.path.basename(path))[0] + ".json"
        with open(os.path.join(FIXTURES_DIR, fixture_name), "w", encoding="utf-8") as f:
            json.dump(fixture, f, ensure_ascii=False, indent=2, default=str)
        generated += 1
        print(f"OK {os.path.basename(path)} -> {len(rows)} rows")

    if _promo_raw_rows is None:
        _capture_promo_raw_rows("KINGFOOD")
    frozen_pricing = {"raw_rows": _promo_raw_rows or []}
    with open(os.path.join(FIXTURES_DIR, "_frozen_pricing.json"), "w", encoding="utf-8") as f:
        json.dump(frozen_pricing, f, ensure_ascii=False, indent=2, default=str)

    print(f"\nDone: {generated} fixtures generated, {skipped} PDFs skipped.")


if __name__ == "__main__":
    main()
```

- [ ] **Step 3-6: Production workbook backup/run/verify/cleanup protocol, then spot-check**

```bash
cp dondathang.xlsx dondathang.xlsx.manual_backup_before_kingfood_fixtures
.venv/Scripts/python.exe GO/internal/processing/kingfood/testdata/generate_fixtures.py
diff dondathang.xlsx dondathang.xlsx.manual_backup_before_kingfood_fixtures
```
Expected: `diff` reports no differences. If IDENTICAL, remove the backup. If NOT identical, STOP — investigate via `log.log` timestamps before touching anything else (this file is live production data, and a concurrent real process may be writing to it — same protocol every prior vendor's harness has used).

Read 2-3 of the generated `GO/internal/processing/kingfood/testdata/fixtures/*.json` files directly. Confirm plausibility: `B` column looks like `"ĐĐHKINGFOOD-<po_number>"`, `E` column (ShipTo) shows the exact hardcoded string `"Số 324, đường ĐT743A, Phường Đông Hoà, Thành phố Hồ Chí Minh"` on every row of every fixture (never anything else — this is a hardcoded constant, so 100% consistency across all 9 fixtures is the expected, correct result, not a red flag), `AU` is populated (non-null) on product rows, `Q`/`X`/`Y` are sane, `Y` values are NOT integers-only (Kingfood's real prices have a fractional/decimal component after the comma-to-dot conversion, e.g. `52195.073` — confirm at least one fixture shows this). Document anything surprising for Task 6's awareness.

- [ ] **Step 7: Commit**

```bash
git add GO/internal/processing/kingfood/testdata/realpdfs/ GO/internal/processing/kingfood/testdata/generate_fixtures.py GO/internal/processing/kingfood/testdata/fixtures/
git commit -m "test(go): copy 9 real Kingfood PDFs into stable testdata and generate golden fixtures"
```

---

### Task 6: Golden fixture integration test — Kingfood

**Files:**
- Create: `GO/internal/processing/kingfood_golden_test.go`

**Interfaces:**
- Consumes: `GO/internal/processing/kingfood/testdata/fixtures/*.json` and `GO/internal/processing/kingfood/testdata/realpdfs/*.pdf` (Task 5), `RealProcessor` (Task 4), `compareRowsAgainstFixture`/`fixtureData`/`fixturePricingSource`/`frozenPricingFixture`/`copyFile`/`joinLines` (existing shared golden-test helpers).
- Produces: nothing consumed by a later task — this is the plan's final task, and Kingfood is the final vendor in the Phase 2b roadmap.

- [ ] **Step 1: Write the test**

Create `GO/internal/processing/kingfood_golden_test.go`:

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

// knownDivergences_Kingfood lists (fixture, row index, column) cells
// where this Go port intentionally computes a different, verified-more-
// correct value than the frozen Python fixture. Key format:
// "<source_pdf>:<row index>:<column>". Empty until a real, hand-verified
// case is confirmed; add entries here only with a comment citing the
// specific PDF/Python-line evidence — never to silence an unexplained
// diff.
//
// Coverage note: this test validates against all 9 real Kingfood PDFs
// available when this plan was executed (committed into
// kingfood/testdata/realpdfs/, not read from the live đơn hàng/ tree —
// see Task 5). More real Kingfood PDFs may exist beyond these 9; adding
// them later is a matter of copying into realpdfs/ and re-running
// generate_fixtures.py — this test globs its inputs, so no code change
// is needed here when that happens.
var knownDivergences_Kingfood = map[string]bool{}

func loadFrozenKingfoodPricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("kingfood/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen Kingfood pricing fixture found (run Task 5's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen Kingfood pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

func TestRealProcessor_MatchesGoldenFixtures_Kingfood(t *testing.T) {
	fixturePaths, err := filepath.Glob("kingfood/testdata/fixtures/*.json")
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
	pricingSource := loadFrozenKingfoodPricingSource(t)

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

		pdfPath := filepath.Join("kingfood", "testdata", "realpdfs", fixture.SourcePDF)
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

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_Kingfood)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
}
```

- [ ] **Step 2: Run — expect RED, investigate every mismatch**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures_Kingfood" -v`

For every mismatch reported, investigate the root cause before deciding whether it's:
1. A real Go bug (fix it, in whichever file actually needs the fix — this may be outside this task's own file list; this project's established methodology explicitly authorizes fixing real bugs found via the golden-fixture process wherever they're found, including shared infrastructure if that's genuinely where the bug lives).
2. A genuine, evidence-backed Python quirk this port deliberately does not reproduce — document it in `knownDivergences_Kingfood` with a comment citing the specific PDF and `xulydonhang.py` line evidence.
3. A pre-existing, unrelated failure in a DIFFERENT vendor's suite — not this task's concern; confirm via `git stash` that this task's own commit didn't change any other vendor's pass/fail state.

**Specific things to check if a mismatch appears, given this plan's own flagged uncertainties:**
- Any `E` (ShipTo) mismatch should NOT happen at all — it's a hardcoded constant identical for every row of every fixture. If one appears, that's a real bug (a typo in the constant, or accidental Vietnamese-diacritic encoding mismatch between the Go source file's literal string and the fixture's captured value) — investigate the exact byte difference directly, don't assume it's cosmetic.
- Any `Y` (unit price) or Y-derived mismatch — check whether `parseKingfoodPrice`'s comma/period handling is actually correct against this SPECIFIC fixture's real captured price string; a single fixture with an unusual price format (e.g. no decimal comma at all, or an unexpected extra separator) would surface here first.
- Any mismatch on product COUNT for the 2-product real PDF (`PO1002586301`) — if `ExtractProducts` returns the wrong number of products for a multi-product order, that's a real regex bug in the `(?:.*\n){4}` fixed-skip-block assumption (see this plan's own risk note: one of the 4 skipped fields might span more than 1 line on some real file, desynchronizing the whole loop for every subsequent product on that page).
- Any `PO`/`A`/`D` (date) mismatch traceable to the tab-vs-space normalization not holding for a PDF whose layout differs slightly from the 3 checked during planning, or to the multi-line "Ngày Giao Hàng NCC Xác Nhận:" label wrapping differently on a different PDF (e.g. 3 lines instead of 2, if the label happens to wrap differently at a different point).

- [ ] **Step 3: Fix, re-run, repeat until GREEN**

Iterate Steps 2-3 until `go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures_Kingfood" -v` passes clean.

- [ ] **Step 4: Final run and commit**

Run: `cd GO && go build ./... && go vet ./...`
Expected: clean build/vet.

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures_Kingfood" -v`
Expected: all 9 fixtures matched.

Do NOT treat a bare `go test ./...` failure elsewhere in the module as a gate for this task — every OTHER vendor's golden test may currently be affected by the live `đơn hàng/` folder's own unrelated availability (see this plan's Global Constraints). Confirm via `git stash` that this task's own commit specifically didn't change any other vendor's pass/fail state.

```bash
git add GO/internal/processing/kingfood_golden_test.go
git commit -m "test(go): add Kingfood golden fixture integration test (9 real PDFs)"
```

This is the final task of the final vendor (2b-7) in the Phase 2b roadmap — after this task's review is clean, the plan proceeds to a final whole-branch review (per `subagent-driven-development`), then `finishing-a-development-branch`.
