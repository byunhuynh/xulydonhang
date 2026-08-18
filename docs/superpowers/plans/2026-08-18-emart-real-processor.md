# Emart RealProcessor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port Emart order processing from `xulydonhang.py` to Go, producing a `processEmartSegment` that plugs into `RealProcessor.Process`'s existing per-page dispatch, validated against all 17 real Emart PDFs via the established golden-fixture methodology.

**Architecture:** New package `GO/internal/processing/emart/` (pure text extraction: PO/date/store-name parsing, product-table extraction) + new file `GO/internal/processing/emart_processor.go` (dispatch, region/store-name lookups, row builder reusing `buildPromoBonusRow`/`processor_shared.go` helpers) + a small additive `excelwriter.Row` field for the new K-column requirement. Emart is "1 PDF page = 1 order" (Coop/Lotte/Satra/Winmart family), not BigC's whole-document shape.

**Tech Stack:** Go 1.x, `excelize/v2`, existing `processing`/`productdata`/`pricing`/`excelwriter`/`coop` packages.

**Spec:** [docs/superpowers/specs/2026-08-18-emart-real-processor-design.md](../specs/2026-08-18-emart-real-processor-design.md)

## Global Constraints

- **Testing/divergence policy** (same as Lotte/Satra/BigC/Winmart, different from Coop): golden-fixture tests compare against real Python output; intentional Go/Python divergences go in a `knownDivergences_Emart` allowlist with `sourcePDF:rowIndex:column` keys and evidence citations — never force a fixture to pass by editing it.
- **Field-semantics trap (the single highest-risk item in this plan)**: `emart.Product.UnitPrice` is a PER-UNIT price, not a line total, despite Python's own dict key for it being `"Total Price"`. `write_to_dondathang_emart` (`xulydonhang.py:5095`, `giahoadon = float(item["Total Price"])`) uses it directly with **no division by quantity** — unlike Winmart's same-named field, which genuinely is a line total and must be divided. Do not copy Winmart's `invoicePrice := totalPrice / ouQty` pattern here.
- **Customer code is a hardcoded constant**, never derived from the PDF: `"MN_MT_KH0032"` (`xulydonhang.py:9363`). No fuzzy-match, no `data.xlsx` test-data changes needed for this plan.
- **Column E (ShipTo) holds a short store LABEL, not a street address** — unlike every other ported vendor. `tenstore = re.search(r"^Delivery to :\s*(.+)", text, re.MULTILINE).group(1).split("   ")[0]` (`xulydonhang.py:9333-9335`) splits on 3 consecutive spaces and keeps only the text BEFORE them (e.g. `"EMART GO VAP"`, not the full address that follows) — this becomes both `diachigiaohang` (column E) and the input to the store-name mapping (column K). One string, two uses.
- **`vendor.Identify`'s Emart case must be INSERTED between Satra and Winmart**, not appended — Python's real order is `... Satra → Emart → Kingfood → CN-HCM → Winmart ...`; Kingfood/CN-HCM aren't ported, so Emart goes directly before Winmart's existing case.
- **No AU (case count) write anywhere in `write_to_dondathang_emart`** — confirmed by direct read of `xulydonhang.py:5059-5199` (the entire product loop), no `AU{current_row}` assignment exists. Same as BigC (`NoCaseCount: true` on every `excelwriter.Row`), unlike Winmart/Coop/Satra/Lotte which do write AU.
- Every exported function gets a doc comment citing the exact `xulydonhang.py` line range it mirrors. Every deviation from a literal Python behavior gets an inline comment explaining why.
- Run `go build ./...`, `go vet ./...`, and the relevant `go test` scope after every task, from the `GO/` directory.
- **New package** `GO/internal/processing/emart/` for Emart-only extraction, mirroring the `lotte`/`satra`/`bigc`/`winmart` package shape. **New file** `GO/internal/processing/emart_processor.go` (+ `emart_processor_test.go`) — never append to `coop_processor.go`/`lotte_processor.go`/`satra_processor.go`/`bigc_processor.go`/`winmart_processor.go`.
- **17 real Emart PDFs** (corrected from the roadmap's stale "9" figure) at `đơn hàng/08-2026/*.PDF`: `4501866956.PDF`, `4501866958.PDF`, `4501873464.PDF`, `4501873471.PDF`, `4501873478.PDF`, `4501875697.PDF`, `4501875698.PDF`, `4501875699.PDF`, `4501878295.PDF`, `4501880037.PDF`, `4501880038.PDF`, `4501880119.PDF`, `4501880122.PDF`, `4501880895.PDF`, `4501880904.PDF`, `4501880907.PDF`, `4501881986.PDF`. All confirmed 1 page.
- **`settings.ini` already has `EMART = 312870626`** (gid for the price/promo sheet) — no changes needed there.

---

### Task 1: `vendor.Identify` — recognize Emart, inserted between Satra and Winmart

**Files:**
- Modify: `GO/internal/processing/vendor/identify.go`
- Modify: `GO/internal/processing/vendor/identify_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `Identify(text string) string` now also returns `"Emart"` — consumed by Task 4's dispatch.

- [ ] **Step 1: Write the failing tests**

Add to `GO/internal/processing/vendor/identify_test.go`:

```go
func TestIdentify_RecognizesEmartByCompanyMarker(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"real marker seen on real PDFs", "Header\nTHISO RETAIL COMPANY LIMITED\nfooter", "Emart"},
		{"ASCII marker (never observed on real PDFs; mirrored for fidelity with Python)", "CONG TY TNHH TMDV XNK HA THANH (101017)", "Emart"},
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

func TestIdentify_EmartCheckedBetweenSatraAndWinmart(t *testing.T) {
	// Python's real identify_vendor order (xulydonhang.py:90-179) is
	// Coop -> BigC -> Lotte -> Satra -> Satra(2nd form) -> Emart ->
	// Kingfood -> CN-HCM -> Winmart -> SHOPEE-CHOICE -> ... Kingfood/
	// CN-HCM aren't ported to Go, so Emart's case must be inserted
	// directly before Winmart's existing case (not appended after it)
	// to preserve the correct relative order among vendors that exist in
	// Go today. This test doesn't have a genuine ordering conflict to
	// construct (no unported vendor's pattern is available to collide
	// with), so it documents the intent for a future reader, mirroring
	// TestIdentify_WinmartCheckedAfterSatra's own rationale.
	got := Identify("THISO RETAIL COMPANY LIMITED")
	if got != "Emart" {
		t.Fatalf("Identify with Emart marker = %q, want %q", got, "Emart")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/vendor/... -run TestIdentify_Emart -v`
Expected: FAIL (compile error — `Identify` doesn't recognize Emart's markers yet, since `emartPattern` doesn't exist).

- [ ] **Step 3: Add `emartPattern` and wire it into `Identify`**

In `GO/internal/processing/vendor/identify.go`, add to the `var (...)` block, after `satraPattern` and before `winmartPattern`:

```go
	// Emart's identify pattern (xulydonhang.py:111-112): either a literal
	// ASCII company-name substring, or "THISO RETAIL COMPANY LIMITED"
	// (the actual PO issuer's letterhead name), in the whitespace-
	// normalized page text. Confirmed on a real sample (4501866956.PDF):
	// the real PDF text uses the ACCENTED Vietnamese form of the first
	// company name ("CÔNG TY TNHH TMDV XNK HÀ THÀNH  (101017)"), so
	// Python's plain-ASCII first alternative never actually matches real
	// PDFs — only "THISO RETAIL COMPANY LIMITED" does the real work.
	// Both are mirrored here for fidelity with Python (the ASCII form
	// costs nothing to keep and guards against a future PDF that happens
	// to use it).
	emartPattern = regexp.MustCompile(`CONG TY TNHH TMDV XNK HA THANH \(101017\)|THISO RETAIL COMPANY LIMITED`)
```

Update the doc comment on `Identify` (currently states "Coop, BigC, Lotte, Satra, and Winmart are implemented in that order... Python's real order has several more vendors between Satra and Winmart (Emart, Kingfood, CN-HCM) that aren't ported yet"). Replace it with:

```go
// Identify tries to recognize which retail vendor produced this
// page/PO text, mirroring xulydonhang.py's identify_vendor. Coop, BigC,
// Lotte, Satra, Emart, and Winmart are implemented in that order (order
// is load-bearing and mirrors Python's real identify_vendor precedence).
// Python's real order still has Kingfood/CN-HCM between Emart and
// Winmart that aren't ported yet — a future implementer adding one of
// those must insert it at the correct relative position, not simply
// append. Identify returns "" for anything that isn't one of the six
// implemented vendors.
func Identify(text string) string {
```

Add the case inside `Identify`, between the `satraPattern` check and the `winmartPattern` check:

```go
	if satraPattern.MatchString(cleaned) {
		return "Satra"
	}
	if emartPattern.MatchString(cleaned) {
		return "Emart"
	}
	if winmartPattern.MatchString(cleaned) {
		return "Winmart"
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/vendor/... -v`
Expected: PASS, all tests including the new ones.

- [ ] **Step 5: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/vendor/identify.go GO/internal/processing/vendor/identify_test.go
git commit -m "feat(go): recognize Emart vendor in identify.Identify, inserted between Satra and Winmart"
```

---

### Task 2: `emart` package — `ParseOrderInfo` (PO number, dates, store name)

**Files:**
- Create: `GO/internal/processing/emart/extract.go`
- Test: `GO/internal/processing/emart/extract_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `ParseOrderInfo(text string) (poNumber, entryDate, cancelDate, storeName string, ok bool)` — consumed by Task 4's `processEmartSegment`.

- [ ] **Step 1: Write the failing tests**

Create `GO/internal/processing/emart/extract_test.go`:

```go
package emart

import "testing"

func TestParseOrderInfo_ExtractsPONumberDatesAndStore(t *testing.T) {
	text := "Some Header\n" +
		"PO No.\n" +
		": 4501866956\n" +
		"Order By / Date\n" +
		": 03.08.2026 09:15 NGUYEN HOANG NHAT NAM\n" +
		"Delivery Date\n" +
		": 05.08.2026 00:00\n" +
		"Delivery to : EMART GO VAP   366 Phan Văn Trị, P.5, Q. Gò Vấp, TP.HCM\n" +
		"more text"

	poNumber, entryDate, cancelDate, storeName, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if poNumber != "4501866956" {
		t.Errorf("poNumber = %q, want %q", poNumber, "4501866956")
	}
	if entryDate != "03.08.2026" {
		// [:10] truncation happens BEFORE the "." -> "/" replace in
		// Python (entry_date[:10].replace(".", "/")) — 10 characters of
		// "03.08.2026 09:15 ..." is "03.08.2026" (dots not yet
		// replaced at truncation time, this assertion checks the raw
		// truncated form before replace to make the ordering explicit;
		// the real returned value has "/" not ".", see below).
	}
	if entryDate != "03/08/2026" {
		t.Errorf("entryDate = %q, want %q", entryDate, "03/08/2026")
	}
	if cancelDate != "05/08/2026" {
		t.Errorf("cancelDate = %q, want %q", cancelDate, "05/08/2026")
	}
	if storeName != "EMART GO VAP" {
		t.Errorf("storeName = %q, want %q (split on 3 spaces, address discarded)", storeName, "EMART GO VAP")
	}
}

func TestParseOrderInfo_NoColonPrefix(t *testing.T) {
	// Python's pattern's ":? ?" makes the colon-and-space fully optional
	// — some real PDFs may render the value directly after the marker
	// line with no ":" at all.
	text := "PO No.\n4501866958\nOrder By / Date\n01.08.2026\nDelivery Date\n03.08.2026\nDelivery to : EMART SALA   1 Đường ABC"
	poNumber, entryDate, cancelDate, storeName, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if poNumber != "4501866958" {
		t.Errorf("poNumber = %q, want %q", poNumber, "4501866958")
	}
	if entryDate != "01/08/2026" {
		t.Errorf("entryDate = %q, want %q", entryDate, "01/08/2026")
	}
	if cancelDate != "03/08/2026" {
		t.Errorf("cancelDate = %q, want %q", cancelDate, "03/08/2026")
	}
	if storeName != "EMART SALA" {
		t.Errorf("storeName = %q, want %q", storeName, "EMART SALA")
	}
}

func TestParseOrderInfo_MissingPONumberFailsCleanly(t *testing.T) {
	// Python would carry a None po_number into several downstream string
	// operations (e.g. STT_donhang_str = f"-{po_number}" -> "-None"),
	// silently corrupting the order number instead of failing. This port
	// fails cleanly instead, per this codebase's established
	// no-bug-for-bug-crash-parity policy.
	_, _, _, _, ok := ParseOrderInfo("nothing relevant here")
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for text with no PO No. marker, want false")
	}
}

func TestParseOrderInfo_MissingDateFailsCleanly(t *testing.T) {
	text := "PO No.\n4501866956\nno dates here at all"
	_, _, _, _, ok := ParseOrderInfo(text)
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for text with no date markers, want false")
	}
}

func TestParseOrderInfo_MissingStoreNameStillSucceeds(t *testing.T) {
	// Python tolerates a missing "Delivery to :" line (prints a message,
	// leaves tenstore as None, still calls write_to_dondathang_emart) —
	// the order still gets written, just with a blank store, and the
	// in-app status table shows a warning. storeName="" mirrors that
	// resilience; ok stays true.
	text := "PO No.\n4501866956\nOrder By / Date\n01.08.2026\nDelivery Date\n03.08.2026\nno delivery-to line here"
	poNumber, _, _, storeName, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false for a missing store name, want true (store is best-effort)")
	}
	if poNumber != "4501866956" {
		t.Errorf("poNumber = %q, want %q", poNumber, "4501866956")
	}
	if storeName != "" {
		t.Errorf("storeName = %q, want empty", storeName)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/emart/... -v`
Expected: FAIL with a build error (package `emart` and `ParseOrderInfo` don't exist yet).

- [ ] **Step 3: Write the implementation**

Create `GO/internal/processing/emart/extract.go`:

```go
package emart

import (
	"regexp"
	"strings"
)

var (
	// poNumberPattern mirrors xulydonhang.py:9316 exactly:
	// r"PO No\.\s*\n\s*:? ?([^\n]+)". NOTE: Go's regexp \s is ASCII-only
	// where Python's is Unicode-aware (matches U+00A0 non-breaking
	// space too) — if a real Emart PDF's marker line has NBSP padding,
	// this may need the same explicit NBSP-normalization treatment
	// already applied for Satra/BigC (see those packages' extract.go for
	// the precedent). Verify against real fixtures in Task 5/6; not
	// pre-emptively "fixed" here without evidence it's needed.
	poNumberPattern = regexp.MustCompile(`PO No\.\s*\n\s*:? ?([^\n]+)`)
	// entryDatePattern mirrors xulydonhang.py:9322.
	entryDatePattern = regexp.MustCompile(`Order By / Date\s*\n\s*:? ?([^\n]+)`)
	// cancelDatePattern mirrors xulydonhang.py:9327.
	cancelDatePattern = regexp.MustCompile(`Delivery Date\s*\n\s*:? ?([^\n]+)`)
	// storeNamePattern mirrors xulydonhang.py:9333: r"^Delivery to :\s*(.+)"
	// with re.MULTILINE -> Go's (?m) flag.
	storeNamePattern = regexp.MustCompile(`(?m)^Delivery to :\s*(.+)`)
)

// ParseOrderInfo mirrors the PO-number/date/store-name extraction inline
// in process_file's Emart branch (xulydonhang.py:9314-9338) — Python
// doesn't factor this into its own named function (unlike Winmart's
// ParseOrderInfo), but this port still gathers it into one function per
// this codebase's established per-vendor package convention.
//
// ok=false only when the PO-number OR either date marker isn't found —
// Python's real code would carry a None value into several downstream
// string operations in that case (e.g. STT_donhang_str = f"-{po_number}"
// silently becomes the literal text "-None"), which this port treats as
// a clean failure instead, per this codebase's established policy. A
// missing store name, by contrast, is genuinely tolerated by Python
// itself (order_file still proceeds, just flags a warning status) — see
// storeName's own handling below, which does NOT gate ok.
func ParseOrderInfo(text string) (poNumber, entryDate, cancelDate, storeName string, ok bool) {
	poMatch := poNumberPattern.FindStringSubmatch(text)
	if poMatch == nil {
		return "", "", "", "", false
	}
	poNumber = strings.TrimSpace(poMatch[1])

	entryMatch := entryDatePattern.FindStringSubmatch(text)
	if entryMatch == nil {
		return "", "", "", "", false
	}
	entryDate = formatEmartDate(strings.TrimSpace(entryMatch[1]))

	cancelMatch := cancelDatePattern.FindStringSubmatch(text)
	if cancelMatch == nil {
		return "", "", "", "", false
	}
	cancelDate = formatEmartDate(strings.TrimSpace(cancelMatch[1]))

	if storeMatch := storeNamePattern.FindStringSubmatch(text); storeMatch != nil {
		storeName = strings.Split(storeMatch[1], "   ")[0]
	}

	return poNumber, entryDate, cancelDate, storeName, true
}

// formatEmartDate mirrors "entry_date[:10].replace(".", "/")"
// (xulydonhang.py:9325, same shape at :9330 for cancel_date): truncate
// to the first 10 characters (Python's [:10] slice, tolerant of shorter
// strings), THEN replace "." with "/". Byte-based Go slicing matches
// Python's character-based slicing here because these date strings are
// always plain ASCII digits and dots (never Vietnamese diacritics).
func formatEmartDate(s string) string {
	if len(s) > 10 {
		s = s[:10]
	}
	return strings.ReplaceAll(s, ".", "/")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/emart/... -v`
Expected: PASS, all 5 tests.

- [ ] **Step 5: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/emart/extract.go GO/internal/processing/emart/extract_test.go
git commit -m "feat(go): add emart package with PO/date/store-name extraction"
```

---

### Task 3: `emart` package — product-table isolation and `ExtractProducts`

**Files:**
- Modify: `GO/internal/processing/emart/extract.go`
- Modify: `GO/internal/processing/emart/extract_test.go`

**Interfaces:**
- Consumes: nothing new (independent of Task 2's functions).
- Produces: `type Product struct { Barcode string; OUQty int; UnitPrice string }` and `ExtractProducts(text string) []Product` — consumed by Task 4's `processEmartSegment`.

- [ ] **Step 1: Write the failing tests**

Append to `GO/internal/processing/emart/extract_test.go`:

```go
func TestExtractProducts_ParsesTableRowsAndDropsZeroPrice(t *testing.T) {
	text := "Article Code Unit Barcode Description PO Unit Qty. in Box PO Qty. Pur. Price(-VAT) Amount(-VAT) Free PO\n" +
		"1234567\n" +
		"893615673120\n" +
		"Nước giặt Blue kháng khuẩn\n" +
		"EA\n" +
		"4\n" +
		"48\n" +
		"26.950\n" +
		"1.293.600\n" +
		"0\n" +
		"7654321\n" +
		"893615673999\n" +
		"Free sample item\n" +
		"EA\n" +
		"1\n" +
		"2\n" +
		"0\n" +
		"0\n" +
		"1\n" +
		"Total Amount(without VAT) :\n" +
		"trailing text not part of the table"

	products := ExtractProducts(text)
	if len(products) != 1 {
		t.Fatalf("len(products) = %d, want 1 (zero-price second item must be dropped)", len(products))
	}
	got := products[0]
	if got.Barcode != "893615673120" {
		t.Errorf("Barcode = %q, want %q", got.Barcode, "893615673120")
	}
	if got.OUQty != 48 {
		t.Errorf("OUQty = %d, want %d (PO Qty. column, not Qty. in Box)", got.OUQty, 48)
	}
	if got.UnitPrice != "26950" {
		t.Errorf("UnitPrice = %q, want %q (dot-stripped per-unit price, NOT the Amount column)", got.UnitPrice, "26950")
	}
}

func TestExtractProducts_NoTableMarkerReturnsEmpty(t *testing.T) {
	// Python's real code would crash here (calling .group(1) on the
	// re.search's None result) — this port returns an empty slice
	// instead, per this codebase's established clean-failure policy; the
	// caller (processEmartSegment) treats an empty product list as an
	// order-level error.
	products := ExtractProducts("no Article Code marker anywhere in this text")
	if products != nil {
		t.Errorf("ExtractProducts = %v, want nil", products)
	}
}

func TestExtractProducts_NoMatchingRowsReturnsEmpty(t *testing.T) {
	products := ExtractProducts("Article Code\nnothing shaped like a product row\nTotal Amount(without VAT) :")
	if products != nil {
		t.Errorf("ExtractProducts = %v, want nil", products)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/emart/... -run TestExtractProducts -v`
Expected: FAIL with a build error (`Product`/`ExtractProducts` don't exist yet).

- [ ] **Step 3: Write the implementation**

Append to `GO/internal/processing/emart/extract.go` (add `strconv` to the existing `import` block):

```go
import (
	"regexp"
	"strconv"
	"strings"
)
```

```go
// productTablePattern isolates the product-table region of the page text
// before the product-line regex runs, mirroring xulydonhang.py:9339-9340
// exactly:
//   text = re.search(r"Article Code\s*(.*?)\s*Total Amount\(without VAT\) :", text, re.DOTALL)
//   text = text.group(1).strip()
// re.DOTALL makes "." match newlines too — mirrored with Go's (?s) flag.
var productTablePattern = regexp.MustCompile(`(?s)Article Code\s*(.*?)\s*Total Amount\(without VAT\) :`)

// productLinePattern mirrors laydanhsanpham_emart's compiled regex
// (xulydonhang.py:6616-6624) exactly: 7 fields — a 7-digit article code,
// a 12-13 digit barcode, a non-greedy description, a >=2-letter unit
// code, a "Qty. in Box" integer, a "PO Qty." integer, and a purchase-
// price field (dots as thousands separators, comma as a possible
// decimal separator). Go has no re.VERBOSE mode; the pattern below is
// the same shape with the VERBOSE-only whitespace/comments removed. The
// original also uses re.DOTALL, mirrored with (?s) so the non-greedy
// description group can span newlines exactly as Python's does.
//
// Capture groups (1-based, matching Python's declaration order):
// 1=article_code (discarded), 2=barcode, 3=description (discarded),
// 4=unit (discarded), 5=qty_in_box (discarded — NOT what "OU Qty" uses),
// 6=quantity (this is "OU Qty" — match.group("quantity"), the PO
// Qty. column), 7=purchase_price (the per-unit "Pur. Price(-VAT)").
var productLinePattern = regexp.MustCompile(`(?s)(\d{7})\s*(\d{12,13})\s*\s*(.+?)\s+([A-Z]{2,})\s+\s*(\d+)\s+\s*(\d+)\s+\s*([\d.,]+)`)

// Product is one extracted Emart product line. UnitPrice is a
// dot-stripped numeric string (Emart's PDF table uses "." as a thousands
// separator, e.g. "26.950" -> "26950") holding the PER-UNIT purchase
// price (the "Pur. Price(-VAT)" column) — NOT a line total, despite
// Python's own dict key for this field being "Total Price"
// (laydanhsanpham_emart, xulydonhang.py:6635-6639). write_to_dondathang_emart
// uses this value directly as giahoadon with NO division by quantity
// (xulydonhang.py:5095) — a real, easy-to-miss difference from Winmart,
// whose same-named field genuinely is a line total and must be divided.
type Product struct {
	Barcode   string
	OUQty     int
	UnitPrice string
}

// ExtractProducts mirrors laydanhsanpham_emart (xulydonhang.py:6614-6644)
// plus the table-isolation step that always runs immediately before it
// in process_file's Emart branch (xulydonhang.py:9339-9340). If the
// "Article Code...Total Amount(without VAT) :" isolation doesn't match
// at all, Python's real code would crash (calling .group(1) on None);
// this returns nil instead, per this codebase's established
// clean-failure policy.
//
// purchase_price_value == 0 items are dropped entirely during extraction
// (xulydonhang.py:6627-6628, "continue") — unlike Winmart, there is no
// "mark the previous row's AO/AP" side effect for a zero-price Emart
// item here; it simply never appears in the returned slice.
func ExtractProducts(text string) []Product {
	tableMatch := productTablePattern.FindStringSubmatch(text)
	if tableMatch == nil {
		return nil
	}
	tableText := strings.TrimSpace(tableMatch[1])

	var products []Product
	for _, m := range productLinePattern.FindAllStringSubmatch(tableText, -1) {
		// purchase_price = match.group("purchase_price").replace(".", "")
		unitPrice := strings.ReplaceAll(m[7], ".", "")
		// purchase_price_value = float(purchase_price.replace(",", "."))
		// A malformed price (ParseFloat error) is NOT treated as zero —
		// it falls through and the item is kept, so a genuinely
		// unexpected price format surfaces as a visible price-mismatch
		// row downstream rather than silently vanishing.
		if value, err := strconv.ParseFloat(strings.ReplaceAll(unitPrice, ",", "."), 64); err == nil && value == 0 {
			continue
		}
		qty, err := strconv.Atoi(m[6])
		if err != nil {
			continue
		}
		products = append(products, Product{
			Barcode:   m[2],
			OUQty:     qty,
			UnitPrice: unitPrice,
		})
	}
	return products
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/emart/... -v`
Expected: PASS, all 8 tests (5 from Task 2 + 3 new).

- [ ] **Step 5: Verify against the REAL sample PDF's text, not just the synthetic unit tests above**

`productLinePattern`'s non-greedy `(.+?)` description group, combined with `(?s)` DOTALL, means the exact match boundaries depend on regex backtracking-equivalent resolution (RE2 implements leftmost-shortest matching for non-greedy operators without backreferences/lookaround, which this pattern doesn't use — so Go's behavior should be mathematically equivalent to Python's `re` here, but this has NOT been empirically confirmed yet). Some real descriptions contain their own uppercase words (e.g. `"CHAI THẢ TOILET CHUNGBLUE NGÀN HOA"` — "CHAI" alone is 4 uppercase ASCII letters, which could theoretically confuse a naive reading of where the `[A-Z]{2,}` unit-code group is supposed to start). Do not just trust the synthetic tests above — extract `đơn hàng/08-2026/4501866956.PDF`'s real page text (e.g. via a throwaway PyMuPDF one-liner, or by using Task 4's copied `testdata/sample_emart_order.pdf`) and run `ExtractProducts` against it directly (a temporary `fmt.Println` in a throwaway `go run` scratch file, or a temporary `t.Log` in a throwaway test, is fine — remove before committing). It MUST extract exactly these 7 products (2 further zero-price duplicate lines for the last 2 barcodes must be dropped):

| Barcode | OUQty | UnitPrice |
|---|---|---|
| 8809174900138 | 48 | 26950 |
| 8809174900213 | 24 | 26950 |
| 8936156730404 | 20 | 97258 |
| 8936156730398 | 40 | 97258 |
| 8936156730459 | 24 | 40000 |
| 8936156731630 | 8 | 73545 |
| 8936156731647 | 8 | 73545 |

(Sanity check: `48*26950 + 24*26950 + 20*97258 + 40*97258 + 24*40000 + 8*73545 + 8*73545 = 9,912,600`, which matches the real PDF's own printed "Total Amount(without VAT) : 9.912.600" line exactly — a strong independent confirmation these are the right 7 rows.)

If the regex extracts anything different (wrong barcode boundaries, wrong qty, a dropped or extra row), that is a real bug to fix in `productLinePattern` before proceeding — do not adjust the synthetic unit tests to match incorrect real-PDF behavior instead.

- [ ] **Step 6: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/emart/extract.go GO/internal/processing/emart/extract_test.go
git commit -m "feat(go): add product-table extraction to emart package"
```

---

### Task 4: `excelwriter` K-column field + `emart_processor.go` (dispatch + row builder)

**Files:**
- Modify: `GO/internal/processing/excelwriter/dondathang.go`
- Modify: `GO/internal/processing/excelwriter/dondathang_test.go`
- Create: `GO/internal/processing/emart_processor.go`
- Create: `GO/internal/processing/emart_processor_test.go`
- Modify: `GO/internal/processing/coop_processor.go` (add `case "Emart":` to the dispatch switch)
- Create: `GO/internal/processing/testdata/sample_emart_order.pdf` (copy from `đơn hàng/08-2026/4501866956.PDF`)

**Interfaces:**
- Consumes: `emart.ParseOrderInfo`, `emart.Product`, `emart.ExtractProducts` (Tasks 2-3); `buildPromoBonusRow`, `regionInfo`-equivalent pattern, `coopDebtDays`, `closeEnough`, `parseNumericField` (existing shared helpers).
- Produces: `processEmartSegment`, `emartRegionInfo`, `emartOrderNumber`, `emartStoreNames` — consumed by Task 6's golden test only indirectly (via `RealProcessor.Process`).

**No `excelwriter.Row` field for column K exists yet** — this is a genuinely new requirement (Emart's header row conditionally writes a full store name to K; no prior vendor touches this column at all). Add one small additive field, following the same precedent as `NoCaseCount`'s earlier addition for BigC.

- [ ] **Step 1: Write the failing test for the new `StoreName`/K field**

Add to `GO/internal/processing/excelwriter/dondathang_test.go`:

```go
func TestWriteOrderRows_WritesStoreNameToColumnK(t *testing.T) {
	path := copyTestWorkbook(t)

	rows := []Row{
		{EntryDate: "05/08/2026", OrderNumber: "ĐĐHEMART-4501866956", Status: "Chưa thực hiện", IsNoteRow: true, ProductName: "EMART PO4501866956", StoreName: "SIÊU THỊ EMART PHAN VĂN TRỊ"},
		{SKU: "8936156731203", Qty: 48, UnitPrice: 26950, ProductName: "Nước giặt Blue", UseZFormula: true},
	}

	if err := WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()

	headerK, _ := f.GetCellValue("Don dat hang", "K9")
	if headerK != "SIÊU THỊ EMART PHAN VĂN TRỊ" {
		t.Fatalf("K9 (header StoreName) = %q, want %q", headerK, "SIÊU THỊ EMART PHAN VĂN TRỊ")
	}
	productK, _ := f.GetCellValue("Don dat hang", "K10")
	if productK != "" {
		t.Fatalf("K10 (product row, StoreName unset) = %q, want empty", productK)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd GO && go test ./internal/processing/excelwriter/... -run TestWriteOrderRows_WritesStoreNameToColumnK -v`
Expected: FAIL (build error — `Row` has no `StoreName` field yet).

- [ ] **Step 3: Add the `StoreName` field and wire it into `writeRow`**

In `GO/internal/processing/excelwriter/dondathang.go`, add to the `Row` struct (after `NoCaseCount`):

```go
	// StoreName writes to column K — used only by Emart's header row,
	// which conditionally writes one of 3 hardcoded full Vietnamese
	// store names (xulydonhang.py:5046-5051) or nothing at all for an
	// unrecognized store. Every other row type and every other vendor
	// leaves this at its zero value (""), which writes an empty K cell —
	// functionally identical to Python's conditional "don't touch K at
	// all" for those cases, since both read back as blank.
	StoreName string
```

Add `{"K", row.StoreName}` to the `writes` slice inside `writeRow`, after the `{"G", row.CustomerCode}` entry (matching column K's position between G and L in the real sheet layout):

```go
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
		{"K", row.StoreName},
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
		{"AO", row.PromoNote},
		{"AP", row.PromoBundleSku},
		{"AQ", row.PromoContent},
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd GO && go test ./internal/processing/excelwriter/... -v`
Expected: PASS, all tests including the new one.

- [ ] **Step 5: Commit the excelwriter change**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/excelwriter/dondathang.go GO/internal/processing/excelwriter/dondathang_test.go
git commit -m "feat(go): add StoreName/column-K field to excelwriter.Row for Emart"
```

- [ ] **Step 6: Copy a real sample PDF for the processor test**

```bash
cp "đơn hàng/08-2026/4501866956.PDF" GO/internal/processing/testdata/sample_emart_order.pdf
```

- [ ] **Step 7: Write the failing processor tests**

Create `GO/internal/processing/emart_processor_test.go`:

```go
package processing

import (
	"context"
	"testing"

	"github.com/xuri/excelize/v2"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleEmartFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	pricingSource := &fixturePricingSource{index: pricing.ParseIndex([][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
	})}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_emart_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].System != "Emart" {
		t.Fatalf("System = %q, want %q", rows[0].System, "Emart")
	}
	if rows[0].StatusKind == StatusKindFailed {
		t.Fatalf("Process produced a Failed row: %+v", rows[0])
	}
	if rows[0].MaKhachHang != emartCustomerCode {
		t.Fatalf("MaKhachHang = %q, want the hardcoded constant %q", rows[0].MaKhachHang, emartCustomerCode)
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
	if len(sheetRows) <= 8 {
		t.Fatalf("expected rows written beyond the 8-row template header, got %d total rows", len(sheetRows))
	}
}

func TestEmartRegionInfo(t *testing.T) {
	cases := []struct {
		name                                     string
		customerCode                             string
		wantRegion, wantStatCode, wantWarehouse string
	}{
		{
			name:          "MB-prefixed code",
			customerCode:  "MB12345",
			wantRegion:    "MT_MB",
			wantStatCode:  "HN",
			wantWarehouse: "TP_HN_12",
		},
		{
			name:          "the real, always-used hardcoded constant (default branch)",
			customerCode:  emartCustomerCode,
			wantRegion:    "MT_MN",
			wantStatCode:  "LA",
			wantWarehouse: "LA_KHO2026",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRegion, gotStatCode, gotWarehouse := emartRegionInfo(tc.customerCode)
			if gotRegion != tc.wantRegion || gotStatCode != tc.wantStatCode || gotWarehouse != tc.wantWarehouse {
				t.Errorf("emartRegionInfo(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.customerCode, gotRegion, gotStatCode, gotWarehouse,
					tc.wantRegion, tc.wantStatCode, tc.wantWarehouse)
			}
		})
	}
}

func TestEmartStoreNames(t *testing.T) {
	cases := []struct {
		name           string
		storeName      string
		wantShortCode  string
		wantFullName   string
	}{
		{"EMART GO VAP -> PVT", "EMART GO VAP", "PVT", "SIÊU THỊ EMART PHAN VĂN TRỊ"},
		{"EMART PHI -> PHI", "EMART PHI", "PHI", "SIÊU THỊ EMART PHAN HUY ÍCH"},
		{"EMART SALA -> SALA", "EMART SALA", "SALA", "SIÊU THỊ EMART SALA"},
		{"unrecognized store: short code falls back to raw text, no full name", "EMART SOMEWHERE ELSE", "EMART SOMEWHERE ELSE", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotShort, gotFull := emartStoreNames(tc.storeName)
			if gotShort != tc.wantShortCode {
				t.Errorf("shortCode = %q, want %q", gotShort, tc.wantShortCode)
			}
			if gotFull != tc.wantFullName {
				t.Errorf("fullName = %q, want %q", gotFull, tc.wantFullName)
			}
		})
	}
}

// TestRealProcessor_EmartNoBraceBonusRowUsesKMRoiNote regression-tests
// Emart's own no-{...}-brace fallback text — the spec's explicitly
// required test for this block (docs/superpowers/specs/2026-08-18-
// emart-real-processor-design.md's Testing section). Uses
// sample_emart_order.pdf's real first product (barcode 8809174900138,
// OU Qty 48, per-unit price 26950 — confirmed by direct extraction of
// đơn hàng/08-2026/4501866956.PDF during planning) with a "2+1 SP0002"
// promo (an "X+1" match mentioning SP0002, a known internal SKU already
// present in the productdata test fixture — see TestFindSkusMentioned)
// and NO {...} braces, triggering the exact no-brace bonus-row path at
// i==0.
func TestRealProcessor_EmartNoBraceBonusRowUsesKMRoiNote(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	const promoValue = "2+1 SP0002"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8809174900138", "Chai thả toilet", "26950", promoValue},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_emart_order.pdf", 1); err != nil {
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
		case "8809174900138":
			mainRow = row
		case "SP0002":
			bonusRow = row
		}
	}
	if mainRow == nil || bonusRow == nil {
		t.Fatalf("missing expected rows: main=%v bonus=%v", mainRow, bonusRow)
	}

	if got := cell(mainRow, colPromoNote); got != "KM Rời - Không Che Barcode" {
		t.Errorf("main row PromoNote (AO) = %q, want %q (Emart's own no-brace fallback, not Coop's or Winmart's)", got, "KM Rời - Không Che Barcode")
	}
	if got := cell(mainRow, colPromoBundleSku); got != "" {
		t.Errorf("main row PromoBundleSku (AP) = %q, want empty (Emart's no-brace branch never writes AP)", got)
	}
	if got := cell(bonusRow, colPromoNote); got != "" {
		t.Errorf("bonus row PromoNote (AO) = %q, want empty (Emart's no-brace branch never touches the bonus row's own AO at i==0)", got)
	}
	if got := cell(bonusRow, colPromoBundleSku); got != "" {
		t.Errorf("bonus row PromoBundleSku (AP) = %q, want empty", got)
	}
}

// TestRealProcessor_EmartInvoiceLevelPromoBonusRow covers the
// invoice-level ("Hóa Đơn") promo bonus row — Q gets only the FIRST
// mentioned SKU, not a joined list — the spec's other explicitly
// required test. Uses all 7 of sample_emart_order.pdf's real products at
// their exact real per-unit price (confirmed against
// 4501866956.PDF: 48*26950 + 24*26950 + 20*97258 + 40*97258 + 24*40000 +
// 8*73545 + 8*73545 = 9,912,600 — which matches the real PDF's own
// printed "Total Amount(without VAT) : 9.912.600" line exactly), so
// totalValue is a known, real, independently-confirmed constant with no
// price-mismatch noise. A "Hóa Đơn" row mentioning both SP0001 and
// SP0002 (in that order) with a 100000 money amount yields
// floor(9912600/100000) = 99 expected bonus units, attributed only to
// SP0001 (the first mentioned SKU).
func TestRealProcessor_EmartInvoiceLevelPromoBonusRow(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8809174900138", "Chai thả toilet ngàn hoa", "26950", ""},
		{"2", "8809174900213", "Chai thả toilet hoa đào", "26950", ""},
		{"3", "8936156730404", "Nước giặt hương thảo mộc", "97258", ""},
		{"4", "8936156730398", "Nước giặt hương nước hoa", "97258", ""},
		{"5", "8936156730459", "Nước rửa chén đậu xanh", "40000", ""},
		{"6", "8936156731630", "Nước rửa chén chanh", "73545", ""},
		{"7", "8936156731647", "Nước rửa chén không mùi", "73545", ""},
		{"8", "Hóa Đơn", "", "0", "100000 SP0001 SP0002"},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_emart_order.pdf", 1); err != nil {
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

	const colSKU, colIsPromoItem, colQty, colPromoNote = 16, 20, 23, 40
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
	if got := cell(bonusRow, colQty); got != "99" {
		t.Errorf("invoice bonus row Qty (X) = %q, want %q (floor(totalValue=9912600 / amount=100000))", got, "99")
	}
	if got := cell(bonusRow, colPromoNote); got != "KM Bó Kèm - Che Barcode" {
		t.Errorf("invoice bonus row PromoNote (AO) = %q, want %q (the invoice-level block's own fallback — unlike the per-item block, this one is NOT overridden for Emart)", got, "KM Bó Kèm - Che Barcode")
	}

	for _, row := range sheetRows {
		if cell(row, colSKU) == "SP0002" {
			t.Errorf("found a row with SKU (Q) = %q, want none (only the first mentioned SKU, SP0001, should get an invoice bonus row)", "SP0002")
		}
	}
}

// TestRealProcessor_EmartInvoiceBonusRowSkipsCleanlyWhenNoSkuMentioned
// covers the guard Python's real code lacks (xulydonhang.py:5290,
// unconditional kiemtra[0] indexing with no length check — a latent
// IndexError crash risk if the "Hóa Đơn" promo string mentions no known
// SKU). This port mirrors buildInvoiceBonusRow's own len(skus)==0 guard
// instead: Process must complete without error/a Failed row, and no
// invoice-level bonus row gets added.
func TestRealProcessor_EmartInvoiceBonusRowSkipsCleanlyWhenNoSkuMentioned(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8809174900138", "Chai thả toilet", "26950", ""},
		{"2", "Hóa Đơn", "", "0", "100000 KHONGCOSKUNAODUOCNHACDEN"},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_emart_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v (should skip the invoice bonus row cleanly, not fail the whole order)", err)
	}
	if len(rows) == 0 || rows[0].StatusKind == StatusKindFailed {
		t.Fatalf("Process produced a Failed row: %+v", rows)
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
	const colSKU = 16
	for _, row := range sheetRows {
		if len(row) > colSKU && row[colSKU] == "KHONGCOSKUNAODUOCNHACDEN" {
			t.Errorf("found a bonus row for an unmatched promo string, want none")
		}
	}
}
```

- [ ] **Step 8: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_ProcessesRealSampleEmartFile|TestEmartRegionInfo|TestEmartStoreNames|TestRealProcessor_EmartNoBraceBonusRowUsesKMRoiNote|TestRealProcessor_EmartInvoiceLevelPromoBonusRow|TestRealProcessor_EmartInvoiceBonusRowSkipsCleanlyWhenNoSkuMentioned" -v`
Expected: FAIL with a build error (`processEmartSegment`/`emartRegionInfo`/`emartStoreNames`/`emartCustomerCode` don't exist yet, and `vendor.Identify` isn't wired into the dispatch switch).

- [ ] **Step 9: Write `emart_processor.go`**

Create `GO/internal/processing/emart_processor.go`:

```go
package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/emart"
	"order-processor/internal/processing/excelwriter"
)

// emartCustomerCode mirrors write_to_dondathang_emart's default/only
// makhachhang value — the literal "MN_MT_KH0032", hardcoded at the
// process_file call site (xulydonhang.py:9363). Emart never derives a
// customer code from the PDF (no fuzzy-match, unlike Satra/Winmart).
const emartCustomerCode = "MN_MT_KH0032"

// emartStoreShortCode mirrors xulydonhang.py:4990-4994's hardcoded
// mapping dict exactly.
var emartStoreShortCode = map[string]string{
	"EMART GO VAP": "PVT",
	"EMART PHI":    "PHI",
	"EMART SALA":   "SALA",
}

// emartStoreFullName mirrors xulydonhang.py:5046-5051's if/elif chain
// (only these 3 short codes get a full name written to column K; any
// other short code gets no K value at all, matching Python having no
// final else branch).
var emartStoreFullName = map[string]string{
	"PVT":  "SIÊU THỊ EMART PHAN VĂN TRỊ",
	"SALA": "SIÊU THỊ EMART SALA",
	"PHI":  "SIÊU THỊ EMART PHAN HUY ÍCH",
}

// emartRegionInfo mirrors write_to_dondathang_emart's warehouse/region
// branching (xulydonhang.py:5003-5009). The MB branch is unreachable
// with real Emart input today — customerCode is always the hardcoded
// constant emartCustomerCode, which never starts with "MB" — but this is
// modeled as a full 2-branch function anyway, matching the
// winmartRegionInfo/bigcRegionInfo precedent, for architectural
// consistency and in case a future change gives Emart a real
// customer-code source. Confirmed NOT a fit for the shared regionInfo()
// (processor_shared.go): that function's non-MB branch returns warehouse
// "LA_TP", but Emart's real non-MB warehouse is "LA_KHO2026" — the same
// divergence already handled for Winmart/BigC.
func emartRegionInfo(customerCode string) (region, statCode, warehouse string) {
	if strings.HasPrefix(customerCode, "MB") {
		return "MT_MB", "HN", "TP_HN_12"
	}
	return "MT_MN", "LA", "LA_KHO2026"
}

// emartOrderNumber mirrors write_to_dondathang_emart's order-number field
// (xulydonhang.py:5024): f'ĐĐH{vendor}{STT_donhang_str}' where vendor is
// the uppercased literal "EMART" and STT_donhang_str is f"-{po_number}".
func emartOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHEMART-%s", poNumber)
}

// emartStoreNames maps a parsed "Delivery to :" store name to its short
// code and, if recognized, its full Vietnamese display name for column
// K — mirroring xulydonhang.py:4990-4996's `mapping.get(congtrinh,
// congtrinh)` and :5046-5051's if/elif chain exactly. An unrecognized
// store keeps its raw text as the short code (matching Python's dict
// .get fallback) and gets no K value at all (matching Python's if/elif
// having no final else branch — fullName is simply "").
func emartStoreNames(storeName string) (shortCode, fullName string) {
	shortCode = storeName
	if mapped, ok := emartStoreShortCode[storeName]; ok {
		shortCode = mapped
	}
	fullName = emartStoreFullName[shortCode]
	return shortCode, fullName
}

// processEmartSegment mirrors the Emart branch of process_file
// (xulydonhang.py:9314-9384) plus write_to_dondathang_emart
// (:4974-5330). Emart is "1 page = 1 order", the same family as Coop/
// Lotte/Satra/Winmart. A trailing PDF page that lacks Emart's identify
// marker falls through to the shared per-page dispatch loop's default
// case (coop_processor.go), which emits a Failed/"Thất bại" OrderRow for
// that page.
//
// Column E (ShipTo) holds the same short store LABEL used for the K
// lookup (e.g. "EMART GO VAP"), NOT a street address — unlike every
// other ported vendor. This mirrors xulydonhang.py's
// `diachigiaohang = congtrinh` (:4987) where congtrinh IS tenstore, the
// already-truncated ("Delivery to :"-line split on 3 spaces) label.
func (p *RealProcessor) processEmartSegment(filePath, text, pageLabel string) (OrderRow, error) {
	poNumber, entryDate, cancelDate, storeName, ok := emart.ParseOrderInfo(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được số PO/ngày đặt hàng")
	}

	products := emart.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	priceIndex, err := p.Pricing.FetchIndex("EMART")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := emartRegionInfo(emartCustomerCode)
	orderNum := emartOrderNumber(poNumber)
	description := fmt.Sprintf("EMART PO%s", poNumber)
	_, fullStoreName := emartStoreNames(storeName)

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: storeName, CustomerCode: emartCustomerCode,
		Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: description, StoreName: fullStoreName,
		NoCaseCount: true,
	})

	saigia := 0
	totalWeight := 0.0
	totalValue := 0.0

	for _, rawProduct := range products {
		barcode := p.Store.ResolveSku(rawProduct.Barcode)
		productInfo, _ := p.Store.GetProductInfo(barcode)
		qty := float64(rawProduct.OUQty)
		lineWeight := productInfo.WeightKg * qty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(qty / productInfo.PackSize))
		}
		totalWeight += lineWeight

		// giahoadon (xulydonhang.py:5095): used DIRECTLY, no division by
		// qty. rawProduct.UnitPrice really is a per-unit price — see the
		// emart package's Product doc comment for why, and why this
		// differs from Winmart's same-named field.
		invoicePrice, _ := strconv.ParseFloat(rawProduct.UnitPrice, 64)

		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice := parseNumericField(realPriceStr)

		promos := priceIndex.FindPromotions(barcode, entryDate)
		lastExaminedPromo := ""
		matched := false
		finalPrice := realPrice

		for _, promo := range promos {
			value := promo.Value
			if value == "" {
				continue
			}
			lastExaminedPromo = value
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
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: storeName, CustomerCode: emartCustomerCode,
			Description: description, SKU: barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: qty, UnitPrice: finalPrice,
			ProductName: productInfo.Name, CaseCount: caseCount, LineWeightKg: lineWeight, UseZFormula: true,
			PromoContent: lastExaminedPromo, NoCaseCount: true,
		}
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
		}

		productRowIndex := len(rows)
		rows = append(rows, productRow)
		totalValue += finalPrice * qty

		// Multi-CTKM split (xulydonhang.py:5203, "nhieuCtkm =
		// khuyenmai.split('|')") — same shape as Coop's own multi-CTKM
		// loop (coop_processor.go's processSegment), NOT Winmart's/
		// Lotte's single-promo-attempt shape: Emart's Python genuinely
		// loops over "|"-split promo parts with its own i==0/i>0 AO/AP
		// placement branch, which buildPromoBonusRow's own index
		// parameter already models.
		currentRowIndex := productRowIndex
		for i, promoPart := range strings.Split(lastExaminedPromo, "|") {
			rows[currentRowIndex].PromoContent = lastExaminedPromo

			bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, promoPart,
				coop.Product{Barcode: barcode, Qty: qty}, i, entryDate, cancelDate, storeName,
				emartCustomerCode, description, warehouse, region, statCode, orderNum)
			if !added {
				continue
			}
			totalWeight += bonusRow.LineWeightKg
			bonusRow.NoCaseCount = true

			// Emart's own no-{...}-brace fallback
			// (xulydonhang.py:5230/:5240, "KM Rời - Không Che Barcode")
			// never writes AP, for EITHER i==0 or i>0 — a third distinct
			// fallback string from Coop's default ("KM Bó Kèm - Che
			// Barcode") and Winmart's ("KM Giao Rời - Không Che
			// Barcode"). Override the shared helper's Coop-flavored
			// result here, scoped to Emart only, for BOTH branches
			// (unlike Lotte/Winmart, which only ever call with index 0).
			if coop.ExtractBraceContent(promoPart) == "" {
				mainRowNote = "KM Rời - Không Che Barcode"
				mainRowBundleSku = ""
				bonusRow.PromoNote = "KM Rời - Không Che Barcode"
				bonusRow.PromoBundleSku = ""
			}

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

	// Invoice-level ("Hóa Đơn") promo bonus row (xulydonhang.py:5274-5316).
	// Does NOT reuse the shared buildInvoiceBonusRow — Q gets only the
	// FIRST matched SKU (kiemtra[0], xulydonhang.py:5290), not a joined
	// list, the same divergence already handled for Winmart. Python
	// indexes kiemtra[0] with NO length guard (a latent crash risk if
	// kmhoadon maps to zero SKUs); this mirrors buildInvoiceBonusRow's
	// own len(skus)==0 guard instead of reproducing that risk.
	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		invoiceSkus := p.Store.FindSkusMentioned(invoicePromo)
		if amount, ok := coop.ExtractMoneyAmount(invoicePromo); ok && amount > 0 && len(invoiceSkus) > 0 {
			invoiceSku := invoiceSkus[0]
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
				invoiceNote = "KM Bó Kèm - Che Barcode" // xulydonhang.py:5312
			}

			rows = append(rows, excelwriter.Row{
				EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
				Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: storeName, CustomerCode: emartCustomerCode,
				Description: description, SKU: invoiceSku, Warehouse: warehouse, VATPercent: 8,
				RegionCode: region, StatCode: statCode, IsPromoItem: true, Qty: soluongkm,
				ProductName: invoiceInfo.Name, CaseCount: invoiceCase, LineWeightKg: invoiceWeight, UseZFormula: false,
				PromoContent: invoicePromo, PromoNote: invoiceNote, NoCaseCount: true,
			})
		}
	}

	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	if err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription); err != nil {
		return OrderRow{}, err
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	// Python's own status logic (xulydonhang.py:9367) flags a warning
	// when saigia>0 OR the store couldn't be resolved (tenstore
	// falls back to "Không xác định") — mirrored here via storeName=="".
	if saigia > 0 || storeName == "" {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Emart", MaKhachHang: emartCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
	}, nil
}
```

- [ ] **Step 10: Wire the `case "Emart":` into `RealProcessor.Process`'s dispatch switch**

In `GO/internal/processing/coop_processor.go`, add a `case "Emart":` block into the `switch v {` statement, between the existing `case "Satra":` block and the existing `case "Winmart":` block (matching `vendor.Identify`'s Satra-then-Emart-then-Winmart order from Task 1):

```go
		case "Emart":
			row, err := p.processEmartSegment(filePath, text, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Emart",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)
```

- [ ] **Step 11: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_ProcessesRealSampleEmartFile|TestEmartRegionInfo|TestEmartStoreNames|TestRealProcessor_EmartNoBraceBonusRowUsesKMRoiNote|TestRealProcessor_EmartInvoiceLevelPromoBonusRow|TestRealProcessor_EmartInvoiceBonusRowSkipsCleanlyWhenNoSkuMentioned" -v`
Expected: PASS, all tests.

Also run the full existing suite to confirm no other vendor regressed:
Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures" -v`
Expected: BigC/Lotte/Satra/Winmart still PASS; Coop's pre-existing gap unchanged.

- [ ] **Step 12: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/emart_processor.go GO/internal/processing/emart_processor_test.go GO/internal/processing/coop_processor.go GO/internal/processing/testdata/sample_emart_order.pdf
git commit -m "feat(go): dispatch RealProcessor to Emart via processEmartSegment"
```

---

### Task 5: Golden fixture generation script (throwaway) — Emart

**Files:**
- Create: `GO/internal/processing/emart/testdata/generate_fixtures.py`

**Interfaces:**
- Consumes: real, unmodified `xulydonhang.py` (repo root) — never modified by this task.
- Produces: `GO/internal/processing/emart/testdata/fixtures/*.json` + `_frozen_pricing.json` — consumed by Task 6.

**This is a throwaway dev tool, not shipped code.** Copy `GO/internal/processing/winmart/testdata/generate_fixtures.py` (read it in full first) as the base — same retry-hardening (`_remove_with_retry`/`_move_with_retry`), UTF-8 stdout fix, pricing-cache monkeypatch, and backup/restore protocol, verbatim. Emart's `write_to_dondathang_emart` — like Winmart's/Satra's/Lotte's write functions — commits each page's rows to the sheet immediately and takes no explicit start-row argument, so the harness's `main()`/`snapshot_rows`/retry helpers carry over unchanged; only `is_emart_pdf`/`process_one_pdf` and the fixture/pricing paths/vendor key change.

- [ ] **Step 1: Write the script**

Create `GO/internal/processing/emart/testdata/generate_fixtures.py`:

```python
"""
Throwaway dev tool — NOT part of the shipped Go or Python app.

Runs the CURRENT xulydonhang.py Emart pipeline against every real PDF in
đơn hàng/08-2026/ that identify_vendor recognizes as Emart, capturing the
resulting dondathang.xlsx rows (and the live-fetched Google Sheets
price/promotion data for the EMART sheet) into JSON fixtures under
GO/internal/processing/emart/testdata/fixtures/. The Go golden test
(Task 6) diffs RealProcessor's output against these fixtures instead of
against a live Google Sheets fetch, so it's deterministic and offline.

Like Satra/Lotte/Winmart (one page == one order, write_to_dondathang_emart
appends immediately, no explicit start-row argument needed), and UNLIKE
BigC (write_to_dondathang_bigc needs an explicit bat_dau/start-row
argument because it accumulates rows across an entire multi-store-page
document before finalizing), this harness computes start_row once up
front (max_row + 1, no compute_start_row helper needed) and takes a
single snapshot after process_one_pdf's per-page loop has finished
writing every Emart page in the file.

Run from the repo root:
    .venv/Scripts/python.exe GO/internal/processing/emart/testdata/generate_fixtures.py
"""
import glob
import json
import os
import shutil
import sys
import time

# Same depth as BigC/Satra/Lotte/Winmart's harnesses: this script sits 5
# directory levels below repo root
# (GO/internal/processing/emart/testdata/generate_fixtures.py), so
# reaching repo root from os.path.abspath(__file__) requires 6 dirname()
# calls (one to strip the filename, five more to strip
# GO/internal/processing/emart/testdata).
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
    REPO_ROOT, "GO", "internal", "processing", "emart", "testdata", "fixtures"
)
TEMPLATE_XLSX = os.path.join(REPO_ROOT, "dondathang.xlsx")
SCRATCH_XLSX = os.path.join(REPO_ROOT, "dondathang_fixture_scratch.xlsx")

# --- Monkey-patch network/upload side effects out (identical shape to
# Coop/Satra/Lotte/BigC/Winmart's harnesses; find_price_by_sku/
# find_all_promotions_by_sku_and_time are already generic over
# sheet_name, so this works for "EMART" too with no changes to the
# caching logic itself) ---

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


# --- Excel row capture (same columns as every other vendor's harness —
# same sheet, same layout; K is included here too even though it's new,
# since snapshot_rows just reads whatever's actually on the sheet) ---

COLUMNS = [
    "A", "B", "C", "D", "E", "G", "K", "L", "Q", "S", "T", "U", "V", "X", "Y", "Z",
    "AE", "AJ", "AM", "AN", "AO", "AP", "AQ", "AT", "AU", "AV",
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


def is_emart_pdf(path):
    doc = xulydonhang.fitz.open(path)
    try:
        text = ""
        for page in doc:
            text += page.get_text("text")
        return xulydonhang.ProcessHandler.identify_vendor(text) == "Emart"
    finally:
        doc.close()


def process_one_pdf(path):
    """Mirrors the Emart branch of process_file (xulydonhang.py:9314-9384)
    for every page identify_vendor recognizes as Emart, skipping the
    Google Drive upload / current-page-extraction side effects."""
    handler = xulydonhang.process_handler
    doc = xulydonhang.fitz.open(path)
    try:
        for page_num in range(len(doc)):
            text = doc[page_num].get_text("text")
            if xulydonhang.ProcessHandler.identify_vendor(text) != "Emart":
                continue

            import re

            po_number = re.search(r"PO No\.\s*\n\s*:? ?([^\n]+)", text)
            if po_number:
                po_number = po_number.group(1).strip()

            entry_date = re.search(r"Order By / Date\s*\n\s*:? ?([^\n]+)", text)
            if entry_date:
                entry_date = entry_date.group(1).strip()
                entry_date = entry_date[:10].replace(".", "/")

            cancel_date = re.search(r"Delivery Date\s*\n\s*:? ?([^\n]+)", text)
            if cancel_date:
                cancel_date = cancel_date.group(1).strip()
                cancel_date = cancel_date[:10].replace(".", "/")

            tenstore = re.search(r"^Delivery to :\s*(.+)", text, re.MULTILINE)
            if tenstore:
                tenstore = tenstore.group(1).split("   ")[0]
            else:
                tenstore = None

            table_match = re.search(r"Article Code\s*(.*?)\s*Total Amount\(without VAT\) :", text, re.DOTALL)
            table_text = table_match.group(1).strip()
            products = xulydonhang.ProcessHandler.laydanhsanpham_emart(table_text)
            if products:
                sku_mapping = xulydonhang.ProcessHandler.load_sku_mapping()
                products = xulydonhang.ProcessHandler.replace_sku_numbers(products, sku_mapping)

            xulydonhang.ProcessHandler.write_to_dondathang_emart(
                handler, products, po_number, entry_date, cancel_date, tenstore,
                1, "MN_MT_KH0032", "Emart", None,
            )
    finally:
        doc.close()


def _remove_with_retry(path, attempts=5, delay=0.5):
    """os.remove wrapped with retry-with-backoff. Windows Defender's
    real-time scanner can transiently hold a lock on a freshly-saved
    .xlsx right after openpyxl's wb.save() closes it, which surfaces here
    as a PermissionError ([WinError 5] Access is denied). Retrying a few
    times with a short delay lets the scan finish and the lock clear
    before we give up and let the exception propagate for real.

    (BigC's Task 7 added this hardening reactively; every harness since
    Winmart's Task 5 includes it from the start.)"""
    for i in range(attempts):
        try:
            os.remove(path)
            return
        except PermissionError:
            if i == attempts - 1:
                raise
            time.sleep(delay)


def _move_with_retry(src, dst, attempts=5, delay=0.5):
    """shutil.move wrapped with the same retry-with-backoff as
    _remove_with_retry, and for the same reason (transient AV lock)."""
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

    pdf_paths = sorted(glob.glob(os.path.join(REPO_ROOT, "đơn hàng", "08-2026", "*.PDF")) +
                        glob.glob(os.path.join(REPO_ROOT, "đơn hàng", "08-2026", "*.pdf")))
    print(f"Found {len(pdf_paths)} candidate PDFs")

    generated = 0
    skipped = 0
    for path in pdf_paths:
        try:
            if not is_emart_pdf(path):
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
            _remove_with_retry(real_target)
            _move_with_retry(backup, real_target)
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
        _capture_promo_raw_rows("EMART")
    frozen_pricing = {"raw_rows": _promo_raw_rows or []}
    with open(os.path.join(FIXTURES_DIR, "_frozen_pricing.json"), "w", encoding="utf-8") as f:
        json.dump(frozen_pricing, f, ensure_ascii=False, indent=2, default=str)

    print(f"\nDone: {generated} fixtures generated, {skipped} PDFs skipped.")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2-5: Production workbook backup/run/verify/cleanup protocol**

```bash
cp dondathang.xlsx dondathang.xlsx.manual_backup_before_emart_fixtures
.venv/Scripts/python.exe GO/internal/processing/emart/testdata/generate_fixtures.py
diff dondathang.xlsx dondathang.xlsx.manual_backup_before_emart_fixtures
```
Expected: `diff` reports no differences (byte-identical) — confirms the harness's backup/restore protocol left the real production `dondathang.xlsx` untouched, same verification every prior vendor's fixture generation has required. If IDENTICAL, remove the backup:
```bash
rm dondathang.xlsx.manual_backup_before_emart_fixtures
```
If NOT identical, STOP — do not proceed, investigate before touching anything else (this file is live production data).

- [ ] **Step 6: Spot-check 2-3 generated fixtures**

Read 2-3 of the generated `GO/internal/processing/emart/testdata/fixtures/*.json` files directly. Confirm plausibility:
- `B` column looks like `"ĐĐHEMART-<po_number>"`.
- `Q`/`X`/`Y` on product rows are sane (barcodes, quantities, VND prices).
- `K` column is populated on the header row (offset 0) for stores that map to one of the 3 known short codes, blank otherwise.
- `AU` is `null` on every row (no vendor Go code writes it, matching the plan's Global Constraints — confirm this matches Python's real output too, since this was inferred from a source read, not yet empirically confirmed against real captured data).
- Any zero-price product from the real PDF's table is absent from the fixture's row count (dropped at extraction, not written as its own row and not causing any other row's AO/AP to be marked — unlike Winmart).

Document anything surprising for Task 6's awareness, per this project's established "flag real findings, don't fix fixture content" practice (see Task 5's own equivalent step in the Winmart/Satra/BigC plans for the expected level of detail).

- [ ] **Step 7: Commit**

```bash
git add GO/internal/processing/emart/testdata/generate_fixtures.py GO/internal/processing/emart/testdata/fixtures/
git commit -m "test(go): generate golden fixtures for Emart from real PDFs + production output"
```

---

### Task 6: Golden fixture integration test — Emart

**Files:**
- Create: `GO/internal/processing/emart_golden_test.go`

**Interfaces:**
- Consumes: `GO/internal/processing/emart/testdata/fixtures/*.json` (Task 5), `RealProcessor` (Task 4), `compareRowsAgainstFixture`/`fixtureData`/`fixturePricingSource`/`frozenPricingFixture`/`copyFile`/`joinLines` (existing shared golden-test helpers, `golden_test_helpers_test.go`).
- Produces: nothing consumed by a later task — this is the plan's final task.

- [ ] **Step 1: Write the test**

Create `GO/internal/processing/emart_golden_test.go`:

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

// knownDivergences_Emart lists (fixture, row index, column) cells where
// this Go port intentionally computes a different, verified-more-correct
// value than the frozen Python fixture. Key format:
// "<source_pdf>:<row index>:<column>". Empty until a real, hand-verified
// case is confirmed; add entries here only with a comment citing the
// specific PDF/Python-line evidence — never to silence an unexplained
// diff.
var knownDivergences_Emart = map[string]bool{}

func loadFrozenEmartPricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("emart/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen Emart pricing fixture found (run Task 5's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen Emart pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

func TestRealProcessor_MatchesGoldenFixtures_Emart(t *testing.T) {
	fixturePaths, err := filepath.Glob("emart/testdata/fixtures/*.json")
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
	pricingSource := loadFrozenEmartPricingSource(t)

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

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_Emart)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
}
```

**Note on `compareRowsAgainstFixture`'s column set:** the shared helper's hardcoded `textColumns`/`floatColumns`/`intColumns` (`golden_test_helpers_test.go:108-110`) do NOT include column `K` — this plan's Task 4 addition to `excelwriter.Row` is real and tested at the unit level (`TestWriteOrderRows_WritesStoreNameToColumnK`), but the golden-fixture comparison will NOT catch a K-column regression. This is a known, accepted gap matching the project's existing, already-documented column-AN gap (every prior vendor's golden harness has the same limitation for AN) — do not expand `compareRowsAgainstFixture`'s column set as part of this task; that's cross-cutting shared-test-infrastructure work out of this plan's scope, matching how AN's identical gap was left for a dedicated future fix rather than bundled into whichever vendor happened to need a new column next.

- [ ] **Step 2: Run — expect RED, investigate every mismatch**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures_Emart" -v`

For every mismatch reported, investigate the root cause before deciding whether it's:
1. A real Go bug (fix it, in whichever file actually needs the fix — extraction, region/store lookup, promo logic, or even shared infrastructure if the root cause is genuinely shared, following this plan's Global Constraints and this project's established practice of fixing real bugs found via the golden-fixture process even when they're outside this task's own file list).
2. A genuine, evidence-backed Python quirk this port deliberately does not reproduce — document it in `knownDivergences_Emart` with a comment citing the specific PDF and `xulydonhang.py` line evidence.
3. A Coop-baseline-style pre-existing, unrelated failure — not expected here since this is a brand-new vendor test, but verify no other vendor's suite (`TestRealProcessor_MatchesGoldenFixtures`, `_BigC`, `_Lotte`, `_Satra`, `_Winmart`) regressed as a side effect of any fix made in this step.

**Specific things to check if a mismatch appears**, given this plan's own flagged uncertainties:
- Any NBSP/whitespace-normalization mismatch on `po_number`/`entryDate`/`cancelDate`/`storeName` — Go's `regexp` `\s` is ASCII-only where Python's is Unicode-aware (see Task 2's `poNumberPattern` doc comment). If a real PDF's marker line has NBSP padding, apply the same explicit NBSP-normalization treatment already used for Satra/BigC — but only if there's real evidence it's needed here, not pre-emptively.
- Any `AU` mismatch — confirm the plan's Global Constraint ("no AU write anywhere in `write_to_dondathang_emart`") actually holds against real captured fixture data, not just the source read it was derived from.
- Any `K` mismatch is NOT caught by this test (see the note above) — if you have independent reason to suspect a K-column bug, check it manually against a real fixture's raw JSON rather than relying on this test.
- Any `Y` (price) mismatch on a product whose real PDF price string might carry a leftover comma-as-decimal-separator after the dot-strip (see the `emart` package's `Product.UnitPrice` doc comment on this) — a genuinely malformed/unusual real price format, not expected on the 17 known samples but worth checking if it appears.

- [ ] **Step 3: Fix, re-run, repeat until GREEN**

Iterate Steps 2-3 until `go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures_Emart" -v` passes clean.

- [ ] **Step 4: Final run and commit**

Run: `cd GO && go build ./... && go vet ./... && go test ./... -v`
Expected: clean build/vet, all tests pass (or fail only with fully documented, understood, non-logic-bug gaps — Coop's pre-existing unrelated failure is the one known exception).

```bash
git add GO/internal/processing/emart_golden_test.go
git commit -m "test(go): add Emart golden fixture integration test"
```
