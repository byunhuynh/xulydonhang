# JMart RealProcessor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port JMart order processing from `xulydonhang.py` to Go, producing a `processJMartSegment` that plugs into `RealProcessor.Process`'s existing per-page dispatch — a new vendor outside the original Phase 2b 7-vendor roadmap, added because it is the only one of 8 candidate vendors with a real PDF sample available to verify against.

**Architecture:** New package `GO/internal/processing/jmart/` (pure text extraction: PO/date/address via direct regex — no layout divergence between Go and PyMuPDF in this region — and product-table extraction via a backward-scan algorithm re-derived for Go's own clean text shape) + new file `GO/internal/processing/jmart_processor.go` (dispatch, row builder reusing `buildPromoBonusRow`/`buildInvoiceBonusRow` from `processor_shared.go` AND reusing the existing, already-shipped `kingfoodRegionInfo` function directly — Python's real `write_to_dondathang_kingfood` is literally shared code between Kingfood and JMart, so this port mirrors that sharing at the region-lookup level without touching or duplicating Kingfood's own code). JMart is "1 PDF page = 1 order."

**Tech Stack:** Go 1.x, `excelize/v2`, existing `processing`/`productdata`/`pricing`/`excelwriter`/`coop` packages.

**Spec:** [docs/superpowers/specs/2026-08-20-jmart-real-processor-design.md](../specs/2026-08-20-jmart-real-processor-design.md)

## Global Constraints

- **⚠️ Only ONE real JMart PDF is available for this entire plan** (`đơn hàng/mẫu đơn hàng/13-07-2026/13-07-2026_[JMart][05-07-2026][MN_MT_JM0001][05-07-2026][DH01010844].pdf`, 3 products). Every test, every design decision in this plan is verified against exactly this one file — there is no second sample to cross-check against, no multi-product-count diversity beyond "3 products on one page," no evidence about how the product-table backward-scan algorithm behaves if a product name or other field ever wraps across more lines than this one sample shows. This is a real, accepted limitation (confirmed with the project owner before committing this plan) — do not manufacture synthetic "second sample" test data and present it as if it were real; keep single-sample tests honest about being single-sample.
- **Testing/divergence policy** (same as every vendor): golden-fixture tests compare against real Python output; intentional Go/Python divergences go in a `knownDivergences_JMart` allowlist with `sourcePDF:rowIndex:column` keys and evidence citations.
- **JMart's `vendor.Identify` case is APPENDED after FujiMart** (the current last case), not inserted mid-chain. Real Python order (`xulydonhang.py:90-179`): `...FujiMart(128) → Tiktok(131) → KOC(134,139) → JMart(145) → MR.DIY(149) → ...`. Tiktok and KOC (the two vendors between FujiMart and JMart in Python) are both unported, so JMart's correct relative position among PORTED vendors is simply last.
- **⚠️ CRITICAL: do NOT port `tachsanpham_JMart`'s literal `"1.00"` string match** (`xulydonhang.py:6962`). This is confirmed, via directly running Python's real function against the real PDF and cross-checking against Go's own `extractPageTexts` output on the SAME file, to be an artifact of PyMuPDF splitting the real value `"1.000"` (the QC/conversion-factor column, always exactly `1.000` in the one real sample) into two separate lines `"1.00"` and `"0"` due to the PDF's column width. **Go's `extractPageTexts` does NOT split this way — it produces a clean, single-line `"1.000"`.** A literal port of the `"1.00"` string match would never match anything against Go's own text, silently producing `nil`/empty `OU Qty` for every product. The correct Go-side anchor is the literal string `"1.000"` (not `"1.00"`), verified to correctly reproduce all 3 of Python's real captured `OU Qty` values when applied to Go's own clean-text output.
- **JMart's product-price field uses standard international number formatting** (comma = thousands separator, period = decimal — e.g. `"133,806.000"` → `133806.000`), the SAME convention every vendor before Kingfood used. The shared `parseNumericField` helper (`bigc_processor.go`) already handles this correctly — **no Kingfood-style dedicated parser is needed** for JMart's price field (unlike Kingfood, whose price field uses the reversed Vietnamese/European convention).
- **`cancelDate` is always exactly `entryDate`** (`xulydonhang.py:8148`, `cancel_date = entry_date` — a direct assignment, not a reformat or a fallback computation). No cross-validation, no ±N-day logic.
- **JMart's Excel-writing behavior is IDENTICAL to Kingfood's**, because real Python literally calls `write_to_dondathang_kingfood` for JMart (`xulydonhang.py:8192`) — there is no separate `write_to_dondathang_jmart` function. This means: per-item promo fallback text `"KM Giao Rời - Không Che Barcode"` that does NOT write column AP (matching Kingfood's already-shipped fix, not FujiMart's); invoice-level promo block with `Q=<first matched SKU only>`; AU (case count) written normally; no zero-price skip logic.
- **Region info: call the EXISTING, already-shipped `kingfoodRegionInfo(customerCode string) (region, statCode, warehouse string)` function directly** (`kingfood_processor.go:38`) — do NOT write a new `jmartRegionInfo` function, and do NOT modify `kingfoodRegionInfo` itself in any way. `xulydonhang.py:8144`'s hardcoded `makhachhang = 'MN_MT_JM0001'` is the exact literal value `kingfoodRegionInfo`'s own `case customerCode == "MN_MT_JM0001": return "MT_MN", "LA", "LA_TP"` branch already handles — this branch exists in Kingfood's shipped code specifically for JMart's sake (Kingfood's own real customer code never triggers it) but was never previously reachable by real input until this plan.
- **Delivery address comes from the PDF** (`xulydonhang.py:8151-8152`, `Địa chỉ giao hàng:...SĐT nhận hàng:`), unlike Kingfood's hardcoded constant. Python has no fallback if the marker doesn't match (`delivery_address = m.group(1).strip() if m else None` — if `None`, Python would write a literal `None` into the Excel cell, a latent bug). This port treats a missing delivery-address match as `ok=false` (clean failure), consistent with `poNumber`/`entryDate` already requiring a match with no fallback in the same function.
- **`excelwriter.Row` needs NO new fields** — every column JMart writes is already supported by the existing struct (verified identical column set to Kingfood's, since the write function is literally shared).
- **`settings.ini` already has a `JMART` gid entry** (`JMART = 1522007492`) — no changes needed there.
- **Source PDF is committed into `GO/internal/processing/jmart/testdata/realpdfs/`** (git-tracked, stable), matching the established pattern for Emart/FujiMart/Kingfood — confirm this decision explicitly with the project owner before Task 5's commit if it wasn't already confirmed during this plan's own brainstorming (it was, for this specific vendor, during spec approval).
- Every exported function gets a doc comment citing the exact `xulydonhang.py` line range it mirrors. Every deviation from a literal Python behavior gets an inline comment explaining why.
- Run `go build ./...`, `go vet ./...`, and the relevant `go test` scope after every task, from the `GO/` directory.
- **New package** `GO/internal/processing/jmart/` for JMart-only extraction. **New file** `GO/internal/processing/jmart_processor.go` — never append to `kingfood_processor.go` or any other vendor's file.

---

### Task 1: `vendor.Identify` — recognize JMart, appended after FujiMart

**Files:**
- Modify: `GO/internal/processing/vendor/identify.go`
- Modify: `GO/internal/processing/vendor/identify_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `Identify(text string) string` now also returns `"JMart"` — consumed by Task 4's dispatch.

- [ ] **Step 1: Write the failing tests**

Add to `GO/internal/processing/vendor/identify_test.go`:

```go
func TestIdentify_RecognizesJMartByUnitLine(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"real marker", "Header\nĐơn vị : HỆ THỐNG SIÊU THỊ JMART\nfooter", "JMart"},
		{"unrelated text", "Header\nĐơn vị : Some Other Store\nfooter", ""},
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

func TestIdentify_JMartCheckedAfterFujiMart(t *testing.T) {
	// Real xulydonhang.py order (xulydonhang.py:90-179): ...FujiMart(128)
	// -> Tiktok(131) -> KOC(134,139) -> JMart(145) -> MR.DIY(149) -> ...
	// Tiktok and KOC (the two vendors between FujiMart and JMart in
	// Python) are both unported, so JMart's correct position among
	// PORTED vendors is simply appended after FujiMart (the current last
	// case) — no insertion needed. There's no genuine ordering conflict
	// to construct here (JMart's marker doesn't overlap any other
	// vendor's pattern), so this test documents the intent for a future
	// reader, mirroring TestIdentify_KingfoodCheckedBetweenEmartAndWinmart's
	// own rationale.
	got := Identify("Đơn vị : HỆ THỐNG SIÊU THỊ JMART")
	if got != "JMart" {
		t.Fatalf("Identify with JMart marker = %q, want %q", got, "JMart")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/vendor/... -run TestIdentify_JMart -v`
Expected: FAIL (compile error — `Identify` doesn't recognize JMart's marker yet).

- [ ] **Step 3: Add `jmartPattern` and wire it into `Identify`, APPENDED after the FujiMart check**

In `GO/internal/processing/vendor/identify.go`, add to the `var (...)` block, after `fujimartPattern`:

```go
	// JMart's identify pattern (xulydonhang.py:145-146): a single
	// literal string substring, no alternation. Real Python order places
	// JMart after Tiktok/KOC (both unported) and after FujiMart — see
	// Identify's own doc comment for the full chain.
	jmartPattern = regexp.MustCompile(`Đơn vị : HỆ THỐNG SIÊU THỊ JMART`)
```

Update the doc comment on `Identify` to mention JMart is now implemented, appended after FujiMart (matching the file's existing style). Add the case inside `Identify`, after the `fujimartPattern` check and before `return ""`:

```go
	if fujimartPattern.MatchString(cleaned) {
		return "FujiMart"
	}
	if jmartPattern.MatchString(cleaned) {
		return "JMart"
	}
	return ""
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/vendor/... -v`
Expected: PASS, all tests including the new ones, and all pre-existing ordering tests still pass unchanged.

- [ ] **Step 5: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/vendor/identify.go GO/internal/processing/vendor/identify_test.go
git commit -m "feat(go): recognize JMart vendor in identify.Identify, appended after FujiMart"
```

---

### Task 2: `jmart` package — `ParseOrderInfo` (PO number, dates, delivery address)

**Files:**
- Create: `GO/internal/processing/jmart/extract.go`
- Test: `GO/internal/processing/jmart/extract_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `ParseOrderInfo(text string) (poNumber, entryDate, cancelDate, deliveryAddress string, ok bool)` — consumed by Task 4's `processJMartSegment`.

- [ ] **Step 1: Write the failing tests**

Create `GO/internal/processing/jmart/extract_test.go`:

```go
package jmart

import "testing"

func TestParseOrderInfo_ExtractsRealSampleFields(t *testing.T) {
	// Text shape mirrors this repo's OWN extractPageTexts output against
	// the real (and only available) sample JMart PDF
	// (đơn hàng/mẫu đơn hàng/13-07-2026/13-07-2026_[JMart][05-07-2026]
	// [MN_MT_JM0001][05-07-2026][DH01010844].pdf), confirmed during
	// planning by running the actual Go PDF pipeline directly. Unlike
	// most other vendors in this project, this specific region of the
	// PDF (header/PO/date/address) shows NO layout divergence between
	// Go's extraction and PyMuPDF's — both keep every marker and its
	// value on adjacent, unsplit lines here (the divergence in this PDF
	// template is confined to the product table, see Task 3).
	text := "\n" +
		"ĐC : L1 – 01, L1 – 02B Tầng 1, Tòa nhà Gold View, 346 Bến Vân Đồn,Phường Vĩnh Hội, TP.Hồ Chí Minh\n" +
		"Đơn vị : HỆ THỐNG SIÊU THỊ JMART\n" +
		"PHIẾU ĐẶT HÀNG NHÀ CUNG CẤP\n" +
		"Tên nhà cung cấp :\n" +
		"Số điện thoại :\n" +
		"Địa chỉ :\n" +
		"CÔNG TY TNHH TMDV XNK HÀTHÀNH\n" +
		"0903 19 11 15\n" +
		"666/46 ĐƯỜNG 3/2.P.14.QUẬN 10,TP.HCM\n" +
		"Ngày in : 05/07/2026\n" +
		"Người in: kimngoc\n" +
		"Số phiếu đặt: DH01010844\n" +
		"Điện thoại:  0707346346 -\n" +
		"Địa chỉ giao hàng:\n" +
		"L1 – 01, L1 – 02B Tầng 1, Tòa nhà Gold View, 346Bến Vân Đồn, Phường Vĩnh Hội, TP.Hồ Chí Minh\n" +
		"SĐT nhận hàng :\n" +
		"0707346346\n" +
		"\n" +
		"Ghi chú:\n"

	poNumber, entryDate, cancelDate, deliveryAddress, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if poNumber != "DH01010844" {
		t.Errorf("poNumber = %q, want %q", poNumber, "DH01010844")
	}
	if entryDate != "05/07/2026" {
		t.Errorf("entryDate = %q, want %q", entryDate, "05/07/2026")
	}
	if cancelDate != entryDate {
		t.Errorf("cancelDate = %q, want it to equal entryDate %q (xulydonhang.py:8148, a direct assignment)", cancelDate, entryDate)
	}
	wantAddress := "L1 – 01, L1 – 02B Tầng 1, Tòa nhà Gold View, 346Bến Vân Đồn, Phường Vĩnh Hội, TP.Hồ Chí Minh"
	if deliveryAddress != wantAddress {
		t.Errorf("deliveryAddress = %q, want %q", deliveryAddress, wantAddress)
	}
}

func TestParseOrderInfo_MissingEntryDateMarkerFailsCleanly(t *testing.T) {
	// No "Ngày in" marker at all -> ok=false. Mirrors Python's real
	// crash risk here (xulydonhang.py:8146's .group(1) has no try/except
	// and would raise AttributeError on a None match) with a clean
	// failure instead, per this codebase's established policy.
	text := "Đơn vị : HỆ THỐNG SIÊU THỊ JMART\n" +
		"Số phiếu đặt: DH01010844\n" +
		"Địa chỉ giao hàng:\nSome Address\nSĐT nhận hàng :\n"
	_, _, _, _, ok := ParseOrderInfo(text)
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for text with no entry-date marker, want false")
	}
}

func TestParseOrderInfo_MissingDeliveryAddressMarkerFailsCleanly(t *testing.T) {
	// No "Địa chỉ giao hàng:...SĐT nhận hàng:" pair -> ok=false. Python's
	// real code has a soft `if m else None` guard here (unlike
	// entry_date/po_number, which crash outright) — but this port gates
	// ok on ALL THREE markers resolving, not just two, since a missing
	// delivery address would otherwise write a literal empty/garbage
	// value into the Excel ShipTo column with no signal anything went
	// wrong (see the plan's own Global Constraints for the full
	// rationale).
	text := "Đơn vị : HỆ THỐNG SIÊU THỊ JMART\n" +
		"Ngày in : 05/07/2026\n" +
		"Số phiếu đặt: DH01010844\n"
	_, _, _, _, ok := ParseOrderInfo(text)
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for text with no delivery-address markers, want false")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/jmart/... -v`
Expected: FAIL with a build error (package `jmart` doesn't exist yet).

- [ ] **Step 3: Write the implementation**

Create `GO/internal/processing/jmart/extract.go`:

```go
package jmart

import (
	"regexp"
	"strings"
)

var entryDatePattern = regexp.MustCompile(`Ngày in\s*:\s*(\d{1,2}/\d{1,2}/\d{4})`)
var poNumberPattern = regexp.MustCompile(`Số phiếu đặt\s*:\s*([A-Z0-9]+)`)

// deliveryAddressPattern uses (?s) (Go's equivalent of Python's re.S/
// DOTALL) so "." matches the newline this repo's own extractPageTexts
// inserts between the "Địa chỉ giao hàng:" label and its value — real
// PyMuPDF keeps them on one line, Go splits them here, but the DOTALL
// flag makes the regex match correctly against EITHER shape, so no
// vendor-specific line-scan tolerance logic is needed (unlike most
// other label/value extractions in this project).
var deliveryAddressPattern = regexp.MustCompile(`(?s)Địa chỉ giao hàng\s*:\s*(.+?)\s*SĐT nhận hàng\s*:`)

// ParseOrderInfo mirrors the JMart branch of process_file
// (xulydonhang.py:8146-8153). Python has NO try/except around
// entry_date's or po_number's regex match — a missing marker crashes
// Python outright with AttributeError: 'NoneType' object has no
// attribute 'group'. This port returns ok=false cleanly instead, per
// this codebase's established policy. delivery_address has a SOFTER
// guard in Python (`if m else None`, defaulting to None rather than
// crashing) — but this port still gates ok on it resolving too, since a
// missing delivery address would otherwise silently write an empty
// ShipTo value with no signal anything went wrong.
//
// cancelDate is always exactly entryDate (xulydonhang.py:8148,
// `cancel_date = entry_date` — a direct assignment, no reformatting, no
// fallback logic, unlike FujiMart/Winmart/Emart's cross-validation).
//
// Confirmed during planning: this specific region of the PDF (header/
// PO/date/address) shows NO Go-vs-PyMuPDF layout divergence — both
// pipelines keep every marker and its value on directly matchable
// lines. The divergence in this PDF template is confined entirely to
// the product table (see ExtractProducts's own doc comment).
func ParseOrderInfo(text string) (poNumber, entryDate, cancelDate, deliveryAddress string, ok bool) {
	entryMatch := entryDatePattern.FindStringSubmatch(text)
	poMatch := poNumberPattern.FindStringSubmatch(text)
	addrMatch := deliveryAddressPattern.FindStringSubmatch(text)

	if entryMatch == nil || poMatch == nil || addrMatch == nil {
		return "", "", "", "", false
	}

	entryDate = entryMatch[1]
	poNumber = poMatch[1]
	cancelDate = entryDate
	deliveryAddress = strings.TrimSpace(addrMatch[1])

	return poNumber, entryDate, cancelDate, deliveryAddress, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/jmart/... -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/jmart/extract.go GO/internal/processing/jmart/extract_test.go
git commit -m "feat(go): add jmart package with PO/date/delivery-address extraction"
```

---

### Task 3: `jmart` package — product-table extraction

**Files:**
- Modify: `GO/internal/processing/jmart/extract.go`
- Modify: `GO/internal/processing/jmart/extract_test.go`

**Interfaces:**
- Consumes: nothing new (independent of Task 2's functions).
- Produces: `type Product struct { Barcode, OUQty, TotalPrice string }` and `ExtractProducts(text string) []Product` — consumed by Task 4's `processJMartSegment`.

- [ ] **Step 1: Write the failing tests**

Append to `GO/internal/processing/jmart/extract_test.go`:

```go
func TestExtractProducts_ParsesRealSampleThreeProducts(t *testing.T) {
	// Exact shape of this repo's OWN extractPageTexts output for the
	// product-table region of the real (and only available) sample
	// JMart PDF, confirmed during planning by running the actual Go PDF
	// pipeline. Cross-checked against real Python's own captured output
	// for the SAME file (ran xulydonhang.py's real cat_giua_theo_dong +
	// tachsanpham_JMart directly): Python produced exactly
	// [{Barcode:8936156730886 OUQty:8 TotalPrice:133806.000}
	//  {Barcode:8936156732668 OUQty:12 TotalPrice:26836.000}
	//  {Barcode:8936156732675 OUQty:12 TotalPrice:26836.260}]
	// — the expected values below match this real captured ground
	// truth exactly, re-derived against Go's own (differently-shaped,
	// unsplit) text using the corrected "1.000" anchor (see
	// ExtractProducts's own doc comment for the full explanation of why
	// Python's literal "1.00" anchor cannot be ported as-is).
	text := "Ghi chú:\n" +
		"Thành tiền(Chưa vat)\n" +
		"Chiết khấu\n" +
		"Đơn giá\n" +
		"Số lượng\n" +
		"QC\n" +
		"ĐVT\n" +
		"Tồn kho\n" +
		"Tên đầy đủ\n" +
		"Barcode\n" +
		"Mã vật tư\n" +
		"STT\n" +
		"1,070,448\n" +
		"0\n" +
		"133,806.000\n" +
		"8.000\n" +
		"1.000\n" +
		"Gói\n" +
		"0.000\n" +
		"NƯỚC GIẶT XẢ BLUE ĐẬMĐẶC H. NƯỚC HOA 3.6 L\n" +
		"8936156730886\n" +
		"03021269\n" +
		"1\n" +
		"322,032\n" +
		"0\n" +
		"26,836.000\n" +
		"12.000\n" +
		"1.000\n" +
		"Chai\n" +
		"3.000\n" +
		"NƯỚC LAU BẾP BLUECHANH 560ML\n" +
		"8936156732668\n" +
		"03021252\n" +
		"2\n" +
		"322,035\n" +
		"0\n" +
		"26,836.260\n" +
		"12.000\n" +
		"1.000\n" +
		"Chai\n" +
		"2.000\n" +
		"NƯỚC LAU BẾP BLUEBẠCH TRÀ Ô LIU  560ML\n" +
		"8936156732675\n" +
		"03021257\n" +
		"3\n" +
		"1,714,515\n" +
		"Tổng:\n" +
		"1,714,515\n"

	products := ExtractProducts(text)
	if len(products) != 3 {
		t.Fatalf("len(products) = %d, want 3", len(products))
	}
	want := []Product{
		{Barcode: "8936156730886", OUQty: "8", TotalPrice: "133806.000"},
		{Barcode: "8936156732668", OUQty: "12", TotalPrice: "26836.000"},
		{Barcode: "8936156732675", OUQty: "12", TotalPrice: "26836.260"},
	}
	for i, w := range want {
		if products[i] != w {
			t.Errorf("products[%d] = %+v, want %+v", i, products[i], w)
		}
	}
}

func TestExtractProducts_NoStartMarkerReturnsEmpty(t *testing.T) {
	products := ExtractProducts("no start marker anywhere\nTổng:\n")
	if products != nil {
		t.Errorf("ExtractProducts = %v, want nil", products)
	}
}

func TestExtractProducts_NoEndMarkerReturnsEmpty(t *testing.T) {
	products := ExtractProducts("Mã vật tư\nSTT\n8936156730886\nno end marker here\n")
	if products != nil {
		t.Errorf("ExtractProducts = %v, want nil", products)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/jmart/... -run TestExtractProducts -v`
Expected: FAIL with a build error (`Product`/`ExtractProducts` don't exist yet).

- [ ] **Step 3: Write the implementation**

Append to `GO/internal/processing/jmart/extract.go`:

```go
// tableStartPattern mirrors JMart's call to the shared
// cat_giua_theo_dong helper (xulydonhang.py:8155,
// dau_line="Mã vật tư"): a line STARTING WITH "Mã vật tư" (not
// necessarily an exact match — cat_giua_theo_dong uses .startswith,
// not ==) marks the beginning of the product table; everything AFTER
// that line, up to (not including) a line that exactly equals "Tổng:",
// is the product block.
const tableStartMarker = "Mã vật tư"
const tableEndMarker = "Tổng:"

// productLinePattern mirrors laydanhsachsanpham_kingfood-style barcode
// anchoring: a line that is EXACTLY 13 digits is a product's barcode
// (xulydonhang.py:6952, `re.fullmatch(r'\d{13}', line)`).
var barcodePattern = regexp.MustCompile(`^\d{13}$`)

// quantityValuePattern mirrors xulydonhang.py:6963,
// `re.fullmatch(r'[1-9]\d*\.000', lines[i - 1])` — a positive integer
// followed by ".000" (the "Số lượng" / quantity column's real format,
// e.g. "8.000", "12.000").
var quantityValuePattern = regexp.MustCompile(`^([1-9]\d*)\.000$`)

// pricePattern mirrors xulydonhang.py:6948,
// price_pattern = r'\d{1,3}(?:,\d{3})+\.\d{3}' — the standard
// international thousands-comma/decimal-period money format (e.g.
// "133,806.000"). This pattern is based on the NUMBER'S OWN FORMAT
// (requires at least one comma-grouped thousands segment), not on any
// PDF-extraction line-splitting artifact — confirmed during planning
// that it correctly identifies the "Đơn giá" (unit price) line when
// scanning backward from a barcode on Go's own (unsplit) text, the
// same way it does on Python's real (split) text, because neither the
// "Chiết khấu" ("0", no comma) nor "QC"/"Số lượng" (no comma) lines
// can ever satisfy this pattern — this part of Python's algorithm
// ports directly, unlike the OU-Qty anchor below.
var pricePattern = regexp.MustCompile(`^\d{1,3}(?:,\d{3})+\.\d{3}$`)

// qcAnchorValue is the corrected Go-side anchor for locating the "Số
// lượng" (quantity) column, replacing Python's literal "1.00" match
// (xulydonhang.py:6962). Python's "1.00" is an artifact of PyMuPDF
// splitting the real value "1.000" (the "QC"/conversion-factor column,
// confirmed always exactly 1.000 in the one real sample available)
// into two separate lines ("1.00" + "0") due to this PDF template's
// narrow table-column width. This repo's own extractPageTexts does NOT
// split this way — it produces the value as one clean line, "1.000".
// Confirmed by running Python's real function against the real sample
// PDF and comparing its captured OU_Qty values against what this
// corrected anchor produces on Go's own (unsplit) text: all 3 match
// exactly. A literal port of "1.00" would never match Go's text at
// all, silently producing an empty OU Qty for every product.
const qcAnchorValue = "1.000"

// Product is one extracted JMart product line. Only Barcode, OUQty, and
// TotalPrice are used downstream by processJMartSegment (via the
// shared write_to_dondathang_kingfood-equivalent row-building logic) —
// Python's tachsanpham_JMart never captures a product name at all (it
// only tracks Barcode/OU Qty/Total Price, xulydonhang.py:6973-6977),
// matching Kingfood's Product struct shape exactly.
//
// TotalPrice here is comma-stripped (matching Python's
// `.replace(",", "")`, xulydonhang.py:6970) but otherwise already in
// standard parseable float format (period decimal) — unlike Kingfood's
// TotalPrice, this does NOT need a dedicated Vietnamese-format parser;
// the shared parseNumericField (bigc_processor.go) handles it directly.
type Product struct {
	Barcode    string
	OUQty      string
	TotalPrice string
}

// extractProductTable mirrors cat_giua_theo_dong as called for JMart
// (xulydonhang.py:8155, dau_line="Mã vật tư", cuoi_line="Tổng:"):
// find the first line STARTING WITH the start marker, take everything
// after it, up to (not including) the first line that EXACTLY EQUALS
// the end marker. If either marker is missing, Python's shared helper
// returns "" (xulydonhang.py:6202-6203, `if start is None or end is
// None or end <= start: return ""`) — this port returns "" too,
// treated as "no products" by ExtractProducts.
func extractProductTable(text string) string {
	lines := strings.Split(text, "\n")

	start := -1
	end := -1
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if start == -1 && strings.HasPrefix(trimmed, tableStartMarker) {
			start = i + 1
			continue
		}
		if start != -1 && trimmed == tableEndMarker {
			end = i
			break
		}
	}

	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[start:end], "\n"))
}

// ExtractProducts mirrors tachsanpham_JMart (xulydonhang.py:6940-6979)
// applied to the already-sliced product table (see
// extractProductTable's own doc comment) — with the OU-Qty anchor
// corrected for Go's own (unsplit) text shape. See qcAnchorValue's doc
// comment for the full explanation of why "1.00" cannot be ported
// literally.
//
// Not ported: xulydonhang.py:6942-6943's two regex substitutions that
// re-join numbers PyMuPDF split mid-decimal across lines (e.g.
// "133,806.\n000" -> "133,806.000"). These exist solely to undo a
// PyMuPDF-specific line-splitting artifact that does not occur in this
// repo's own Go text extraction (confirmed during planning) — porting
// them would be dead code with no real input to exercise it.
func ExtractProducts(text string) []Product {
	table := extractProductTable(text)
	if table == "" {
		return nil
	}

	lines := strings.Split(table, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}

	var products []Product
	for idx, line := range lines {
		if !barcodePattern.MatchString(line) {
			continue
		}
		barcode := line

		// OU Qty: scan backward from the barcode (up to 20 lines, per
		// xulydonhang.py:6960's own range) for the QC anchor value;
		// once found, the line immediately before it (one line further
		// back) is the quantity value.
		ouQty := ""
		limit := idx - 20
		if limit < 0 {
			limit = 0
		}
		for i := idx - 1; i >= limit; i-- {
			if lines[i] == qcAnchorValue {
				if i-1 >= 0 {
					if m := quantityValuePattern.FindStringSubmatch(lines[i-1]); m != nil {
						ouQty = m[1]
					}
				}
				break
			}
		}

		// Total Price: scan backward from the barcode (unbounded, per
		// xulydonhang.py:6968's own range) for the first line matching
		// the international money-format pattern.
		totalPrice := ""
		for i := idx - 1; i >= 0; i-- {
			if pricePattern.MatchString(lines[i]) {
				totalPrice = strings.ReplaceAll(lines[i], ",", "")
				break
			}
		}

		products = append(products, Product{Barcode: barcode, OUQty: ouQty, TotalPrice: totalPrice})
	}
	return products
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/jmart/... -v`
Expected: PASS, all tests.

- [ ] **Step 5: Verify against the REAL sample PDF's actual Go-extracted text, not just the literal test strings above**

The test string above was transcribed from this repo's own `extractPageTexts` output against the real sample, captured during planning — but transcription errors are possible, and this is the ONLY real sample available for this entire vendor, so getting this exactly right matters more than usual. Run a throwaway scratch test (or `go run` snippet, deleted before committing) calling `extractPageTexts` (package `processing`, not `jmart`) directly against `đơn hàng/mẫu đơn hàng/13-07-2026/13-07-2026_[JMart][05-07-2026][MN_MT_JM0001][05-07-2026][DH01010844].pdf`, then feed that real output through `jmart.ExtractProducts`. Confirm the 3 products match exactly: `{8936156730886, 8, 133806.000}`, `{8936156732668, 12, 26836.000}`, `{8936156732675, 12, 26836.260}`. If the real extraction doesn't match, that's a real bug in the regex/anchor logic to fix — do not adjust the test to match incorrect real-PDF behavior instead. Remove the scratch code before committing — it should never be left in the repo.

- [ ] **Step 6: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/jmart/extract.go GO/internal/processing/jmart/extract_test.go
git commit -m "feat(go): add product-table extraction to jmart package"
```

---

### Task 4: `jmart_processor.go` (dispatch + row builder, reusing `kingfoodRegionInfo`)

**Files:**
- Create: `GO/internal/processing/jmart_processor.go`
- Create: `GO/internal/processing/jmart_processor_test.go`
- Modify: `GO/internal/processing/coop_processor.go` (add `case "JMart":` as the LAST case, appended after the existing `case "FujiMart":` block)
- Create: `GO/internal/processing/testdata/sample_jmart_order.pdf` (copy from `đơn hàng/mẫu đơn hàng/13-07-2026/13-07-2026_[JMart][05-07-2026][MN_MT_JM0001][05-07-2026][DH01010844].pdf`)

**Interfaces:**
- Consumes: `jmart.ParseOrderInfo`, `jmart.Product`, `jmart.ExtractProducts` (Tasks 2-3); `buildPromoBonusRow`, `buildInvoiceBonusRow`, `coopDebtDays`, `closeEnough`, `parseNumericField`, and — critically — the ALREADY-SHIPPED, UNMODIFIED `kingfoodRegionInfo(customerCode string) (region, statCode, warehouse string)` from `kingfood_processor.go`.
- Produces: `processJMartSegment`, `jmartOrderNumber` — consumed by Task 6's golden test only indirectly (via `RealProcessor.Process`).

- [ ] **Step 1: Copy the sample PDF**

```bash
cp "đơn hàng/mẫu đơn hàng/13-07-2026/13-07-2026_[JMart][05-07-2026][MN_MT_JM0001][05-07-2026][DH01010844].pdf" GO/internal/processing/testdata/sample_jmart_order.pdf
```

Verify byte-identical: `cmp "đơn hàng/mẫu đơn hàng/13-07-2026/13-07-2026_[JMart][05-07-2026][MN_MT_JM0001][05-07-2026][DH01010844].pdf" GO/internal/processing/testdata/sample_jmart_order.pdf`.

If this exact path is no longer available (a known, if less volatile, risk in this project's archive folder), search `đơn hàng/mẫu đơn hàng/*/` for any filename containing `DH01010844` and use that instead — note the actual path used in the task report. If genuinely unavailable anywhere, report BLOCKED — there is no other real sample to fall back to for this vendor.

- [ ] **Step 2: Write the failing processor tests**

Create `GO/internal/processing/jmart_processor_test.go`:

```go
package processing

import (
	"context"
	"testing"

	"github.com/xuri/excelize/v2"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleJMartFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	pricingSource := &fixturePricingSource{index: pricing.ParseIndex([][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
	})}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_jmart_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].System != "JMart" {
		t.Fatalf("System = %q, want %q", rows[0].System, "JMart")
	}
	if rows[0].StatusKind == StatusKindFailed {
		t.Fatalf("Process produced a Failed row: %+v", rows[0])
	}
	if rows[0].MaKhachHang != jmartCustomerCode {
		t.Fatalf("MaKhachHang = %q, want the hardcoded constant %q", rows[0].MaKhachHang, jmartCustomerCode)
	}
	if rows[0].PO != "DH01010844" {
		t.Fatalf("PO = %q, want %q", rows[0].PO, "DH01010844")
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
	// 1 header + 3 products = 4 new rows (the real sample has 3
	// products); no promo bonus row expected since the synthetic
	// pricing source above has no real promo data.
	if len(sheetRows) != 8+4 {
		t.Fatalf("total rows = %d, want %d (8 template + 1 header + 3 products)", len(sheetRows), 8+4)
	}
}

func TestJMartUsesKingfoodRegionInfoDirectly(t *testing.T) {
	// Confirms processJMartSegment calls the EXISTING, unmodified
	// kingfoodRegionInfo helper — not a JMart-specific copy — by
	// checking that kingfoodRegionInfo's own MN_MT_JM0001 branch (which
	// exists in kingfood_processor.go specifically for JMart's sake,
	// per that file's own doc comment) produces exactly what JMart's
	// real hardcoded customer code needs.
	region, statCode, warehouse := kingfoodRegionInfo(jmartCustomerCode)
	if region != "MT_MN" || statCode != "LA" || warehouse != "LA_TP" {
		t.Errorf("kingfoodRegionInfo(%q) = (%q, %q, %q), want (\"MT_MN\", \"LA\", \"LA_TP\")",
			jmartCustomerCode, region, statCode, warehouse)
	}
}

// TestRealProcessor_JMartNoBraceBonusRowDoesNotWriteAP regression-tests
// that JMart's promo-fallback logic matches Kingfood's exactly (since
// both are produced by the same real Python function,
// write_to_dondathang_kingfood) — the no-{...}-brace fallback text
// ("KM Giao Rời - Không Che Barcode") must NOT write column AP.
//
// Uses sample_jmart_order.pdf's real first product (barcode
// 8936156730886, OU Qty 8, price "133806.000" — confirmed by direct
// extraction during planning) with a "2+1 SP0002" promo and NO {...}
// braces.
func TestRealProcessor_JMartNoBraceBonusRowDoesNotWriteAP(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	const promoValue = "2+1 SP0002"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156730886", "Nước giặt xả", "133806", promoValue},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_jmart_order.pdf", 1); err != nil {
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
		case "8936156730886":
			mainRow = row
		case "SP0002":
			bonusRow = row
		}
	}
	if mainRow == nil || bonusRow == nil {
		t.Fatalf("missing expected rows: main=%v bonus=%v", mainRow, bonusRow)
	}

	if got := cell(mainRow, colPromoNote); got != "KM Giao Rời - Không Che Barcode" {
		t.Errorf("main row PromoNote (AO) = %q, want %q (JMart's own no-brace fallback, via shared write_to_dondathang_kingfood)", got, "KM Giao Rời - Không Che Barcode")
	}
	if got := cell(mainRow, colPromoBundleSku); got != "" {
		t.Errorf("main row PromoBundleSku (AP) = %q, want empty (no-brace branch does NOT write AP)", got)
	}
	if got := cell(bonusRow, colPromoBundleSku); got != "" {
		t.Errorf("bonus row PromoBundleSku (AP) = %q, want empty", got)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_ProcessesRealSampleJMartFile|TestJMartUsesKingfoodRegionInfoDirectly|TestRealProcessor_JMartNoBraceBonusRowDoesNotWriteAP" -v`
Expected: FAIL with a build error (`processJMartSegment`/`jmartCustomerCode` don't exist yet, and `vendor.Identify` isn't wired into the dispatch switch).

- [ ] **Step 4: Write `jmart_processor.go`**

Create `GO/internal/processing/jmart_processor.go`:

```go
package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/jmart"
)

// jmartCustomerCode mirrors write_to_dondathang_kingfood's makhachhang
// value as passed from JMart's own process_file branch — the literal
// "MN_MT_JM0001", hardcoded at the call site (xulydonhang.py:8144).
// This is the EXACT value kingfoodRegionInfo's own "MN_MT_JM0001"
// branch (kingfood_processor.go) was written for.
const jmartCustomerCode = "MN_MT_JM0001"

// jmartOrderNumber mirrors write_to_dondathang_kingfood's order-number
// field (xulydonhang.py:3899) as applied to JMart's call:
// f'ĐĐH{vendor}{STT_donhang_str}' where vendor is the uppercased
// literal "JMART" and STT_donhang_str is f"-{po_number}".
func jmartOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHJMART-%s", poNumber)
}

// processJMartSegment mirrors the JMart branch of process_file
// (xulydonhang.py:8143-8209), which itself calls the SAME
// write_to_dondathang_kingfood function Kingfood's own branch calls
// (xulydonhang.py:8192) — there is no separate write_to_dondathang_jmart.
// This function therefore mirrors processKingfoodSegment's row-building
// shape closely, but deliberately calls the EXISTING, already-shipped
// kingfoodRegionInfo helper directly (see this file's own const above)
// rather than duplicating or modifying it. JMart is "1 page = 1 order",
// same family as Kingfood. A trailing PDF page that lacks JMart's
// identify marker falls through to the shared per-page dispatch loop's
// default case (coop_processor.go), which emits a Failed/"Thất bại"
// OrderRow for that page.
func (p *RealProcessor) processJMartSegment(filePath, text, pageLabel string) (OrderRow, error) {
	poNumber, entryDate, cancelDate, deliveryAddress, ok := jmart.ParseOrderInfo(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được số PO/ngày đặt hàng/địa chỉ giao hàng")
	}

	products := jmart.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	priceIndex, err := p.Pricing.FetchIndex("JMART")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := kingfoodRegionInfo(jmartCustomerCode)
	orderNum := jmartOrderNumber(poNumber)
	description := fmt.Sprintf("JMART %s", poNumber)

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: jmartCustomerCode,
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
		invoicePrice := parseNumericField(rawProduct.TotalPrice)

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
			// Emart/FujiMart/Kingfood) — applied here from the FIRST
			// commit, not added later as a fix-round patch.
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
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: jmartCustomerCode,
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

		// Per-item promo bonus row — single attempt, buildPromoBonusRow
		// always called with index=0, matching Kingfood's exact shape
		// (both are produced by the same real Python function).
		bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, khuyenmai,
			coop.Product{Barcode: barcode, Qty: ouQty}, 0, entryDate, cancelDate, deliveryAddress,
			jmartCustomerCode, description, warehouse, region, statCode, orderNum)
		if added {
			totalWeight += bonusRow.LineWeightKg

			// No-{...}-brace fallback text ("KM Giao Rời - Không Che
			// Barcode") does NOT write AP — matching Kingfood's own
			// fix (write_to_dondathang_kingfood:4092-4096, only the
			// cachbokem branch writes AP; the else/fallback branch
			// never does), since this is the SAME real Python function.
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

	// Invoice-level ("Hóa Đơn") promo bonus row. Does NOT reuse the
	// shared buildInvoiceBonusRow — Q gets only the first matched SKU
	// (kiemtra[0]), not a joined list, matching Kingfood's exact shape.
	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		invoicePromo = strings.ReplaceAll(invoicePromo, "\r", "\n")
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
				invoiceNote = "KM Bó Kèm - Che Barcode"
			}

			rows = append(rows, excelwriter.Row{
				EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
				Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: jmartCustomerCode,
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
		FileName: filepath.Base(filePath), Page: pageLabel, System: "JMart", MaKhachHang: jmartCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
	}, nil
}
```

- [ ] **Step 5: Wire the `case "JMart":` into `RealProcessor.Process`'s dispatch switch, APPENDED after FujiMart**

In `GO/internal/processing/coop_processor.go`, find the existing `case "FujiMart":` block (it ends with `rows = append(rows, row)` followed by a blank line, immediately before `default:`). Insert the new `case "JMart":` block there, so `case "JMart":` becomes the new last case before `default:`:

```go
		case "JMart":
			row, err := p.processJMartSegment(filePath, text, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "JMart",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_ProcessesRealSampleJMartFile|TestJMartUsesKingfoodRegionInfoDirectly|TestRealProcessor_JMartNoBraceBonusRowDoesNotWriteAP" -v`
Expected: PASS, all tests.

Also run Kingfood's own full test suite to confirm zero regression from reusing (not modifying) `kingfoodRegionInfo`:
Run: `cd GO && go test ./internal/processing/... -run "Kingfood" -v`
Expected: unaffected, all pass exactly as before this task.

- [ ] **Step 7: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/jmart_processor.go GO/internal/processing/jmart_processor_test.go GO/internal/processing/coop_processor.go GO/internal/processing/testdata/sample_jmart_order.pdf
git commit -m "feat(go): dispatch RealProcessor to JMart via processJMartSegment"
```

---

### Task 5: Copy the real PDF into stable testdata + golden fixture generation script (throwaway)

**Files:**
- Create: `GO/internal/processing/jmart/testdata/realpdfs/DH01010844.pdf`
- Create: `GO/internal/processing/jmart/testdata/generate_fixtures.py`

**Interfaces:**
- Consumes: real, unmodified `xulydonhang.py` (repo root) — never modified by this task.
- Produces: `GO/internal/processing/jmart/testdata/fixtures/DH01010844.json` + `_frozen_pricing.json` — consumed by Task 6.

This decision was confirmed with the project owner during this plan's own brainstorming: commit the one currently-available real JMart PDF into a stable, git-tracked local directory, matching the established pattern for Emart/FujiMart/Kingfood.

- [ ] **Step 1: Copy the real PDF into stable testdata**

```bash
mkdir -p "GO/internal/processing/jmart/testdata/realpdfs"
cp "đơn hàng/mẫu đơn hàng/13-07-2026/13-07-2026_[JMart][05-07-2026][MN_MT_JM0001][05-07-2026][DH01010844].pdf" GO/internal/processing/jmart/testdata/realpdfs/DH01010844.pdf
```

Verify byte-identical to the source, and to Task 4's already-committed `GO/internal/processing/testdata/sample_jmart_order.pdf`:
```bash
cmp "đơn hàng/mẫu đơn hàng/13-07-2026/13-07-2026_[JMart][05-07-2026][MN_MT_JM0001][05-07-2026][DH01010844].pdf" GO/internal/processing/jmart/testdata/realpdfs/DH01010844.pdf
cmp GO/internal/processing/testdata/sample_jmart_order.pdf GO/internal/processing/jmart/testdata/realpdfs/DH01010844.pdf
```

This is a COPY (read-only source access) — do not move, rename, or modify anything under `đơn hàng/` itself. `đơn hàng/` is git-ignored (`.gitignore:19`, `**/đơn hàng/`) so nothing under it is trackable or committed by this repo anyway — only the new copy under `GO/internal/processing/jmart/testdata/realpdfs/` (not gitignored) will be committed.

- [ ] **Step 2: Write the fixture-generation script**

Create `GO/internal/processing/jmart/testdata/generate_fixtures.py`. Adapted from the same base as Kingfood's harness (`GO/internal/processing/kingfood/testdata/generate_fixtures.py` — read it in full first) with the same structural shape: reads PDFs from the stable local `realpdfs/` directory, same backup/restore protocol, retry-with-backoff, UTF-8 stdout fix, pricing/promotion monkeypatch caching. JMart needs no OCR, matching Kingfood's own no-OCR shape.

```python
"""
Throwaway dev tool — NOT part of the shipped Go or Python app.

Runs the CURRENT xulydonhang.py JMart pipeline against the ONE real
JMart PDF copied into GO/internal/processing/jmart/testdata/realpdfs/
(see this task's own Step 1 — committed directly into this repo's
testdata from the start, per explicit project-owner confirmation for
this vendor), capturing the resulting dondathang.xlsx rows (and the
live-fetched Google Sheets price/promotion data for the JMART sheet)
into a JSON fixture under GO/internal/processing/jmart/testdata/fixtures/.
The Go golden test (Task 6) diffs RealProcessor's output against this
fixture instead of against a live Google Sheets fetch, so it's
deterministic and offline.

⚠️ This harness, and the golden test it feeds, cover exactly ONE real
order. There is no second sample to cross-check against for this
vendor — see this plan's own Global Constraints for the full context.

Like Kingfood (one page == one order, write_to_dondathang_kingfood
appends immediately, no explicit start-row argument needed), this
harness computes start_row once up front and takes a single snapshot
after process_one_pdf's per-page loop has finished.

Run from the repo root:
    .venv/Scripts/python.exe GO/internal/processing/jmart/testdata/generate_fixtures.py
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
    REPO_ROOT, "GO", "internal", "processing", "jmart", "testdata", "realpdfs"
)
FIXTURES_DIR = os.path.join(
    REPO_ROOT, "GO", "internal", "processing", "jmart", "testdata", "fixtures"
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
    """Mirrors the JMart branch of process_file (xulydonhang.py:8143-
    8209) for every page identify_vendor recognizes as JMart, skipping
    the Google Drive upload side effect (monkeypatched to a no-op
    above). No OCR needed for JMart."""
    handler = xulydonhang.process_handler
    doc = xulydonhang.fitz.open(path)
    try:
        for page_num in range(len(doc)):
            text = doc[page_num].get_text("text")
            if xulydonhang.ProcessHandler.identify_vendor(text) != "JMart":
                continue

            import re

            makhachhang = "MN_MT_JM0001"
            entry_date = re.search(r"Ngày in\s*:\s*(\d{1,2}/\d{1,2}/\d{4})", text).group(1)
            cancel_date = entry_date
            po_number = re.search(r"Số phiếu đặt\s*:\s*([A-Z0-9]+)", text).group(1)
            m = re.search(r"Địa chỉ giao hàng\s*:\s*(.+?)\s*SĐT nhận hàng\s*:", text, re.S)
            delivery_address = m.group(1).strip() if m else None

            products = xulydonhang.ProcessHandler.cat_giua_theo_dong(text, "Mã vật tư", "Tổng:")
            products = xulydonhang.ProcessHandler.tachsanpham_JMart(products)
            if not products:
                continue
            sku_mapping = xulydonhang.ProcessHandler.load_sku_mapping()
            products = xulydonhang.ProcessHandler.replace_sku_numbers(products, sku_mapping)

            xulydonhang.ProcessHandler.write_to_dondathang_kingfood(
                handler, products, makhachhang, po_number, entry_date, cancel_date,
                1, "JMart", delivery_address, None,
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


def _restore_with_retry(backup, real_target, attempts=20, delay=1.0):
    # os.replace() is atomic (no separate remove-then-move step), and
    # this uses a much larger retry budget than earlier vendors' harnesses
    # did — a real, previously-discovered bug (see the Coop vendor's own
    # fixture-regeneration history) had a two-step remove-then-move
    # restore that could raise OUT of a finally block before the restore
    # ever ran, on a transient Windows file lock. This is the corrected,
    # standard pattern for any generate_fixtures.py harness in this
    # project going forward.
    for i in range(attempts):
        try:
            os.replace(backup, real_target)
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
            _restore_with_retry(backup, real_target)
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
        _capture_promo_raw_rows("JMART")
    frozen_pricing = {"raw_rows": _promo_raw_rows or []}
    with open(os.path.join(FIXTURES_DIR, "_frozen_pricing.json"), "w", encoding="utf-8") as f:
        json.dump(frozen_pricing, f, ensure_ascii=False, indent=2, default=str)

    print(f"\nDone: {generated} fixtures generated, {skipped} PDFs skipped.")


if __name__ == "__main__":
    main()
```

- [ ] **Step 3-6: Production workbook backup/run/verify/cleanup protocol, then spot-check**

```bash
cp dondathang.xlsx dondathang.xlsx.manual_backup_before_jmart_fixtures
.venv/Scripts/python.exe GO/internal/processing/jmart/testdata/generate_fixtures.py
diff dondathang.xlsx dondathang.xlsx.manual_backup_before_jmart_fixtures
```
Expected: `diff` reports no differences. If IDENTICAL, remove the backup. If NOT identical, STOP — investigate via `log.log` timestamps before touching anything else (this file is live production data, and a concurrent real process may be writing to it — same protocol every prior vendor's harness has used; also check whether the divergence is organic growth from that concurrent process rather than something this harness did, matching the diagnostic approach already established for Coop/Lotte/Satra/BigC/Winmart's own fixture regenerations).

Read the one generated `GO/internal/processing/jmart/testdata/fixtures/DH01010844.json` file directly. Confirm plausibility: `B` column looks like `"ĐĐHJMART-DH01010844"`, `E` column (ShipTo) shows the real delivery address text extracted from the PDF (not a hardcoded constant, unlike Kingfood — confirm it's a real address string), `AU` is populated (non-null) on all 3 product rows, `Q`/`X`/`Y` are sane for all 3 products (barcodes `8936156730886`/`8936156732668`/`8936156732675`, quantities `8`/`12`/`12`).

- [ ] **Step 7: Commit**

```bash
git add GO/internal/processing/jmart/testdata/realpdfs/ GO/internal/processing/jmart/testdata/generate_fixtures.py GO/internal/processing/jmart/testdata/fixtures/
git commit -m "test(go): copy the one real JMart PDF into stable testdata and generate its golden fixture"
```

---

### Task 6: Golden fixture integration test — JMart

**Files:**
- Create: `GO/internal/processing/jmart_golden_test.go`

**Interfaces:**
- Consumes: `GO/internal/processing/jmart/testdata/fixtures/DH01010844.json` and `GO/internal/processing/jmart/testdata/realpdfs/DH01010844.pdf` (Task 5), `RealProcessor` (Task 4), `compareRowsAgainstFixture`/`fixtureData`/`fixturePricingSource`/`frozenPricingFixture`/`copyFile`/`joinLines` (existing shared golden-test helpers).
- Produces: nothing consumed by a later task — this is the plan's final task.

- [ ] **Step 1: Write the test**

Create `GO/internal/processing/jmart_golden_test.go`:

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

// knownDivergences_JMart lists (fixture, row index, column) cells where
// this Go port intentionally computes a different, verified-more-
// correct value than the frozen Python fixture. Key format:
// "<source_pdf>:<row index>:<column>". Empty until a real, hand-verified
// case is confirmed; add entries here only with a comment citing the
// specific PDF and xulydonhang.py line evidence — never to silence an
// unexplained diff.
//
// ⚠️ Coverage note, more significant than usual: this test validates
// against exactly ONE real JMart PDF — the only one available anywhere
// in this project when this plan was executed. There is no second
// sample to cross-check the product-table backward-scan algorithm's
// robustness against (e.g. a product name that wraps across more lines,
// a QC/conversion-factor value other than "1.000", a different unit
// word). A clean pass here is real evidence for exactly this one order
// and its 3 products — not broad evidence this vendor's extraction
// logic generalizes correctly to JMart's full real order variety. If
// more real JMart PDFs surface later, copying them into realpdfs/ and
// re-running generate_fixtures.py requires no code change here (this
// test globs its inputs) — but the underlying extraction logic in the
// jmart package should be re-scrutinized against any new sample before
// trusting it, not just assumed correct because this test still passes.
var knownDivergences_JMart = map[string]bool{}

func loadFrozenJMartPricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("jmart/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen JMart pricing fixture found (run Task 5's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen JMart pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

func TestRealProcessor_MatchesGoldenFixtures_JMart(t *testing.T) {
	fixturePaths, err := filepath.Glob("jmart/testdata/fixtures/*.json")
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
	pricingSource := loadFrozenJMartPricingSource(t)

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

		pdfPath := filepath.Join("jmart", "testdata", "realpdfs", fixture.SourcePDF)
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

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_JMart)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixture(s) matched (single-sample coverage — see knownDivergences_JMart's own doc comment)", len(realFixtures))
}
```

- [ ] **Step 2: Run — expect RED, investigate every mismatch**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures_JMart" -v`

For every mismatch reported, investigate the root cause before deciding whether it's:
1. A real Go bug (fix it, in whichever file actually needs the fix — this may be outside this task's own file list; this project's established methodology explicitly authorizes fixing real bugs found via the golden-fixture process wherever they're found).
2. A genuine, evidence-backed Python quirk this port deliberately does not reproduce — document it in `knownDivergences_JMart` with a comment citing the exact evidence (there is only one PDF, so "the specific PDF" is always `DH01010844.pdf` here — still cite the exact `xulydonhang.py` line evidence).
3. A pre-existing, unrelated failure in a DIFFERENT vendor's suite — not this task's concern; confirm via `git stash` that this task's own commit didn't change any other vendor's pass/fail state.

**Specific things to check if a mismatch appears, given this plan's own flagged uncertainties:**
- Any `Q`/`X` (SKU/quantity) mismatch on any of the 3 products — check whether the `"1.000"` QC-anchor backward-scan is landing on the right line; this is the single highest-risk piece of logic in this entire vendor (see Task 3's own extensive doc comments).
- Any `Y` (unit price) mismatch — check whether `parseNumericField` is correctly parsing `rawProduct.TotalPrice` (should need no special handling, unlike Kingfood).
- Any `E` (ShipTo) mismatch — JMart extracts a REAL delivery address from the PDF (unlike Kingfood's hardcoded constant), so verify the `(?s)` DOTALL regex is capturing the exact same trimmed string Python's real `re.S` regex captures.
- Any `G` (customer code) or region-info-derived column (`V`/`AJ`/`AM`) mismatch — since these come from `kingfoodRegionInfo("MN_MT_JM0001")`, a mismatch here would be surprising (that function is already shipped and tested) — if one appears, double check `jmartCustomerCode`'s literal string value is EXACTLY `"MN_MT_JM0001"` (a typo here would silently fall through to `kingfoodRegionInfo`'s default branch instead of its `MN_MT_JM0001`-specific one).

- [ ] **Step 3: Fix, re-run, repeat until GREEN**

Iterate Steps 2-3 until `go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures_JMart" -v` passes clean.

- [ ] **Step 4: Final run and commit**

Run: `cd GO && go build ./... && go vet ./...`
Expected: clean build/vet.

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures_JMart" -v`
Expected: the one fixture matched.

Do NOT treat a bare `go test ./...` failure elsewhere in the module as a gate for this task — other vendors' golden tests may be affected by unrelated, pre-existing conditions (see this project's own memory for details). Confirm via `git stash` that this task's own commit specifically didn't change any other vendor's pass/fail state.

```bash
git add GO/internal/processing/jmart_golden_test.go
git commit -m "test(go): add JMart golden fixture integration test (1 real PDF)"
```
