# FujiMart RealProcessor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port FujiMart order processing from `xulydonhang.py` to Go, producing a `processFujimartSegment` that plugs into `RealProcessor.Process`'s existing per-page dispatch, validated against all 15 real FujiMart PDFs currently available via the established golden-fixture methodology — with source PDFs committed into a stable, project-local testdata directory from the start (not the live `đơn hàng/` tree).

**Architecture:** New package `GO/internal/processing/fujimart/` (pure text extraction: PO/date/store-info parsing including a verified mojibake-decode table, product-table extraction) + new file `GO/internal/processing/fujimart_processor.go` (dispatch, region lookup, row builder reusing `buildPromoBonusRow` from `processor_shared.go`). FujiMart is "1 PDF page = 1 order" (Coop/Lotte/Satra/Winmart/Emart family), not BigC's whole-document shape. No OCR: confirmed via direct comparison against real `pytesseract` output that the store-info value Python gets via OCR is fully reconstructable from the PDF's plain text layer plus a verified character-decode table.

**Tech Stack:** Go 1.x, `excelize/v2`, existing `processing`/`productdata`/`pricing`/`excelwriter`/`coop` packages.

**Spec:** [docs/superpowers/specs/2026-08-18-fujimart-real-processor-design.md](../specs/2026-08-18-fujimart-real-processor-design.md)

## Global Constraints

- **Testing/divergence policy** (same as every vendor since Lotte, different from Coop): golden-fixture tests compare against real Python output; intentional Go/Python divergences go in a `knownDivergences_Fujimart` allowlist with `sourcePDF:rowIndex:column` keys and evidence citations — never force a fixture to pass by editing it.
- **No OCR** — confirmed by running Python's real `pytesseract` pipeline directly against 2 real FujiMart PDFs during planning: the OCR'd `"Nơi nhận:"` value (`"11031 FujiMart Tây Sơn"`, `"11051 FujiMart 10 Trần Phú-Hà Đông"`) is exactly `<5-digit store code, the text-layer line right after "N¬i nhËn:"> + " " + <the text-layer line starting with "FujiMart ", decoded through a verified mojibake table>`. Confirmed consistent across all 15 real FujiMart PDFs currently available.
- **Mojibake decode table** — this PDF's text layer uses a legacy 8-bit Vietnamese font encoding, misread as Latin-1/Windows-1252. Verified via running real `pytesseract` OCR (clean Vietnamese) side-by-side with the plain text layer (mojibake) across all 15 real FujiMart PDFs, character-aligning every mismatch, zero conflicts found:
  ```
  §→Đ  ©→â  ª→ê  «→ô  ¬→ơ  µ→à  ¸→á  ¹→ạ  Ç→ầ  È→ẩ  Ô→ễ  ä→ọ  ó→ú  ô→ụ  ú→ỳ
  ```
  This table covers every character seen across the 11 distinct real store branches in the current 15-file corpus. It is **not** the full legacy font's character set — a future PDF from an unseen branch could contain an undecoded character. The decode function must pass through any unmapped rune unchanged (never guess, never error) — see Task 2.
- **Field-semantics note**: `dongia` (FujiMart's per-line "Total Price") IS a line total (unlike Emart's per-unit trap) — `giahoadon = dongia / qty_ord_pcs` (xulydonhang.py:2843) divides by quantity to get the per-unit invoice price, the SAME shape as Winmart's `TotalPrice`, not Emart's.
- **Customer code is a hardcoded constant**: `"MB_MT_FUJI"` (xulydonhang.py:8919) — no fuzzy-match, no OCR-derived lookup. Since it always starts with `"MB"`, the region-info function's non-`"MB"` branch is unreachable with real input today — still implement both branches fully, matching the established `winmartRegionInfo`/`emartRegionInfo` precedent, for architectural consistency.
- **FujiMart writes AU (case count) normally** — unlike Emart/BigC (never write AU), matching Winmart/Coop/Satra/Lotte. Do NOT set `NoCaseCount` on any row.
- **No zero-price skip logic exists in FujiMart's write function** — unlike Winmart, every extracted product gets its own row regardless of price.
- **Per-item promo block is single-attempt** (matches Winmart's/Lotte's shape, NOT Coop's/Emart's `"|"`-split multi-CTKM loop) — `buildPromoBonusRow` is always called with `index=0`.
- **FujiMart's own no-`{...}`-brace fallback text is `"KM Bó Kèm - Không Che Barcode"`** (xulydonhang.py:2973) — a fourth distinct fallback string project-wide. Unlike Winmart's/Emart's equivalent fix, this fallback text still contains the substring `"bó kèm"`, so `buildPromoBonusRow`'s own bundle-SKU (AP) computation is **unaffected** by the text override — only the AO note text itself needs overriding at the call site, and Python only ever writes it onto the MAIN product row (never onto the bonus row it creates one row later), matching `buildPromoBonusRow`'s own `index==0` behavior exactly (which never sets the bonus row's own `PromoNote` either).
- **Invoice-level ("Hóa Đơn") promo bonus row exists and must be ported** (xulydonhang.py:3010-3047) — same shape as Winmart's/Emart's hand-rolled block: `Q` gets only the FIRST matched SKU (`kiemtra[0]`, not a joined list), fallback text `"KM Bó Kèm - Che Barcode"` (matches the shared default, no override needed there). Python indexes `kiemtra[0]` with no length guard (a latent crash risk, xulydonhang.py:3025) — mirror `buildInvoiceBonusRow`'s own `len(skus)==0` guard instead of reproducing that risk.
- **`excelwriter.Row` needs NO new fields** — every column FujiMart writes (A,AV,B,C,D,G,L,V,AE,AJ,AM,U,Z,S,T,X,Y,E,Q,AT,AU,AO,AP,AQ) is already supported by the existing struct. No shared-file struct extension this time (unlike Emart's Task 4, which added `StoreName`/column K).
- **`vendor.Identify`'s FujiMart case must be APPENDED after Winmart** (the current last case) — Python's real order has Kingfood/CN-HCM/SHOPEE-CHOICE between Winmart and FujiMart, all three unported, so no insertion is needed.
- Every exported function gets a doc comment citing the exact `xulydonhang.py` line range it mirrors. Every deviation from a literal Python behavior gets an inline comment explaining why.
- Run `go build ./...`, `go vet ./...`, and the relevant `go test` scope after every task, from the `GO/` directory.
- **New package** `GO/internal/processing/fujimart/` for FujiMart-only extraction, mirroring the established per-vendor package shape. **New file** `GO/internal/processing/fujimart_processor.go` — never append to any other vendor's `_processor.go` file.
- **15 real FujiMart PDFs available now** (see Task 5's exact source paths) — this is a point-in-time count; 8 unique branches confirmed to date, more may exist. **Source PDFs are committed into `GO/internal/processing/fujimart/testdata/realpdfs/`** (git-tracked, stable, immune to the live `đơn hàng/` folder's ongoing reorganization by a concurrently-running production instance of this same application — see Emart's plan history for why) rather than read from the live folder at test-run time, per explicit project-owner confirmation for this vendor specifically (not assumed from Emart's own precedent).
- **`settings.ini` already has a `FUJIMART` gid entry** (confirmed at line 10) — no changes needed there.

---

### Task 1: `vendor.Identify` — recognize FujiMart, appended after Winmart

**Files:**
- Modify: `GO/internal/processing/vendor/identify.go`
- Modify: `GO/internal/processing/vendor/identify_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `Identify(text string) string` now also returns `"FujiMart"` — consumed by Task 4's dispatch.

- [ ] **Step 1: Write the failing tests**

Add to `GO/internal/processing/vendor/identify_test.go`:

```go
func TestIdentify_RecognizesFujiMartByTaxCode(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"real tax code", "Header\n251000000161\nfooter", "FujiMart"},
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

func TestIdentify_FujiMartCheckedAfterWinmart(t *testing.T) {
	// Python's real identify_vendor order (xulydonhang.py:90-179) has
	// Kingfood -> CN-HCM -> SHOPEE-CHOICE between Winmart and FujiMart,
	// all three unported to Go. Since none of them exist in Go today,
	// FujiMart's case only needs to be appended after Winmart's (the
	// current last case), not inserted mid-sequence, to preserve the
	// correct relative order among vendors that actually exist in Go.
	// This test doesn't have a genuine ordering conflict to construct (no
	// unported vendor's pattern is available), so it documents the
	// intent for a future reader, mirroring
	// TestIdentify_WinmartCheckedAfterSatra's own rationale.
	got := Identify("251000000161")
	if got != "FujiMart" {
		t.Fatalf("Identify with FujiMart marker = %q, want %q", got, "FujiMart")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/vendor/... -run TestIdentify_FujiMart -v`
Expected: FAIL (compile error — `Identify` doesn't recognize FujiMart's marker yet).

- [ ] **Step 3: Add `fujimartPattern` and wire it into `Identify`**

In `GO/internal/processing/vendor/identify.go`, add to the `var (...)` block, after `winmartPattern`:

```go
	// FujiMart's identify pattern (xulydonhang.py:128-129): a single
	// literal numeric substring (the vendor's own tax code), no
	// alternation.
	fujimartPattern = regexp.MustCompile(`251000000161`)
```

Update the doc comment on `Identify` to mention FujiMart is now implemented and appended after Winmart (matching the existing comment's established style). Add the case inside `Identify`, after the `winmartPattern` check and before `return ""`:

```go
	if winmartPattern.MatchString(cleaned) {
		return "Winmart"
	}
	if fujimartPattern.MatchString(cleaned) {
		return "FujiMart"
	}
	return ""
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/vendor/... -v`
Expected: PASS, all tests including the new ones.

- [ ] **Step 5: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/vendor/identify.go GO/internal/processing/vendor/identify_test.go
git commit -m "feat(go): recognize FujiMart vendor in identify.Identify, appended after Winmart"
```

---

### Task 2: `fujimart` package — `ParseOrderInfo` (PO number, dates, store info, mojibake decode)

**Files:**
- Create: `GO/internal/processing/fujimart/extract.go`
- Test: `GO/internal/processing/fujimart/extract_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `ParseOrderInfo(text string) (poNumber, entryDate, cancelDate, storeInfo string, ok bool)` — consumed by Task 4's `processFujimartSegment`.

- [ ] **Step 1: Write the failing tests**

Create `GO/internal/processing/fujimart/extract_test.go`:

```go
package fujimart

import "testing"

func TestDecodeMojibake_AppliesKnownMappings(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"Tay Son", "FujiMart T©y S¬n", "FujiMart Tây Sơn"},
		{"Hoang Cau", "FujiMart Hoµng CÇu", "FujiMart Hoàng Cầu"},
		{"10 Tran Phu - Ha Dong", "FujiMart 10 TrÇn Phó-Hµ §«ng", "FujiMart 10 Trần Phú-Hà Đông"},
		{"Thuy Khue", "FujiMart Thôy Khuª", "FujiMart Thụy Khuê"},
		{"Le Duan", "FujiMart Lª DuÈn", "FujiMart Lê Duẩn"},
		{"Linh Dam", "FujiMart Linh §µm", "FujiMart Linh Đàm"},
		{"89 Lac Long Quan", "FujiMart 89 L¹c Long Qu©n", "FujiMart 89 Lạc Long Quân"},
		{"Ngoc Khanh", "FujiMart Ngäc Kh¸nh", "FujiMart Ngọc Khánh"},
		{"Huynh Thuc Khang", "FujiMart Huúnh Thóc Kh¸ng", "FujiMart Huỳnh Thúc Kháng"},
		{"Tan Mai", "FujiMart T©n Mai", "FujiMart Tân Mai"},
		{"Nguyen Co Thach", "FujiMart NguyÔn C¬ Th¹ch", "FujiMart Nguyễn Cơ Thạch"},
		{"no mojibake at all", "FujiMart Times City", "FujiMart Times City"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := decodeFujimartMojibake(c.input)
			if got != c.want {
				t.Errorf("decodeFujimartMojibake(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

func TestDecodeMojibake_PassesThroughUnmappedRunes(t *testing.T) {
	// A future PDF from an unseen store branch could contain a
	// character not in the verified 15-entry table — must NOT error or
	// guess, just leave it unchanged so the gap is visible rather than
	// silently wrong.
	input := "FujiMart 東京" // arbitrary unmapped runes
	got := decodeFujimartMojibake(input)
	if got != input {
		t.Errorf("decodeFujimartMojibake(%q) = %q, want unchanged %q", input, got, input)
	}
}

func TestParseOrderInfo_ExtractsRealSampleFields(t *testing.T) {
	// Text shape mirrors this repo's OWN extractPageTexts output against
	// the real sample đơn hàng/08-2026/103001302608001342.pdf (confirmed
	// during planning by running the actual Go PDF pipeline directly —
	// NOT just PyMuPDF's shape), including the empty leading line Go's
	// extraction produces (PyMuPDF's doesn't) and the "Ngµy giao:"/value
	// split across two lines that PyMuPDF keeps on one line.
	text := "\n" +
		"Thµnh tiÒn\n" +
		"FujiMart T©y S¬n\n" +
		"Ghi chó:\n" +
		"STT\n" +
		"§iÖn tho¹i:\n" +
		"0862138966\n" +
		"C«ng ty CP Hµ Thµnh Long An 1\n" +
		"251000000161\n" +
		"NCC:\n" +
		"N¬i nhËn:\n" +
		"11031\n" +
		"18/08/2026\n" +
		"103001302608001342\n" +
		"14:43\n" +
		"Sè §¬n:\n" +
		"Ngµy ®Æt:\n" +
		"Page 1 of 1\n" +
		"Fax:\n" +
		"Ngµy giao:\n" +
		"20/08/2026\n" +
		"§Þa chØ:\n" +
		"1\n12.0\n1,695,264\nTUI\n141,272\nBLUE -N­íc giÆt\n8936156730879\n2006324377\n" +
		"VAT\n"

	poNumber, entryDate, cancelDate, storeInfo, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if poNumber != "103001302608001342" {
		t.Errorf("poNumber = %q, want %q", poNumber, "103001302608001342")
	}
	if entryDate != "18/08/2026" {
		t.Errorf("entryDate = %q, want %q", entryDate, "18/08/2026")
	}
	if cancelDate != "20/08/2026" {
		t.Errorf("cancelDate = %q, want %q", cancelDate, "20/08/2026")
	}
	if storeInfo != "11031 FujiMart Tây Sơn" {
		t.Errorf("storeInfo = %q, want %q", storeInfo, "11031 FujiMart Tây Sơn")
	}
}

func TestParseOrderInfo_MissingPONumberMarkerFailsCleanly(t *testing.T) {
	// No "Sè §¬n:" marker anywhere -> entry_date never resolves -> ok=false.
	// Mirrors Python's real crash risk here (entry_date would be an
	// undefined variable, NameError on first use) with a clean failure
	// instead, per this codebase's established policy.
	_, _, _, _, ok := ParseOrderInfo("nothing relevant here\nno markers at all\n")
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for text with no markers, want false")
	}
}

func TestParseOrderInfo_MissingStoreInfoStillSucceeds(t *testing.T) {
	// Store info is best-effort (matches Python's tenstore defaulting to
	// "" when its OCR regex doesn't match) — must NOT gate ok.
	text := "\n" +
		"Thµnh tiÒn\n" +
		"no FujiMart line at all here\n" +
		"11031\n" +
		"18/08/2026\n" +
		"103001302608001342\n" +
		"14:43\n" +
		"Sè §¬n:\n" +
		"Ngµy giao: 20/08/2026\n"

	poNumber, _, cancelDate, storeInfo, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false for missing store info, want true")
	}
	if poNumber != "103001302608001342" {
		t.Errorf("poNumber = %q, want %q", poNumber, "103001302608001342")
	}
	if cancelDate != "20/08/2026" {
		t.Errorf("cancelDate = %q, want %q (same-line layout must also work)", cancelDate, "20/08/2026")
	}
	if storeInfo != "" {
		t.Errorf("storeInfo = %q, want empty", storeInfo)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/fujimart/... -v`
Expected: FAIL with a build error (package `fujimart` doesn't exist yet).

- [ ] **Step 3: Write the implementation**

Create `GO/internal/processing/fujimart/extract.go`:

```go
package fujimart

import (
	"regexp"
	"strings"
	"time"
)

// fujimartMojibakeMap decodes this PDF template's legacy 8-bit Vietnamese
// font encoding artifacts (misread as Latin-1/Windows-1252 by this
// repo's PDF text extraction, same as PyMuPDF's) to correct Unicode.
// Verified by running Python's REAL pytesseract OCR output side-by-side
// with the plain text layer across all 15 real FujiMart PDFs available
// during planning, character-aligning every mismatch — zero conflicts
// found across 11 distinct real store branch names. This table covers
// every character actually observed; it is NOT the full legacy font's
// character set. decodeFujimartMojibake (below) passes through any
// unmapped rune unchanged rather than guessing.
var fujimartMojibakeMap = map[rune]rune{
	'§': 'Đ', '©': 'â', 'ª': 'ê', '«': 'ô', '¬': 'ơ',
	'µ': 'à', '¸': 'á', '¹': 'ạ', 'Ç': 'ầ', 'È': 'ẩ',
	'Ô': 'ễ', 'ä': 'ọ', 'ó': 'ú', 'ô': 'ụ', 'ú': 'ỳ',
}

// decodeFujimartMojibake applies fujimartMojibakeMap rune-by-rune. Any
// rune not in the map is passed through unchanged — never guessed. This
// is the ONLY place FujiMart's port needs a decode step; every other
// field either comes from a database lookup (product names via
// timten_sanpham) or is purely numeric/date (unaffected by the font
// encoding issue).
func decodeFujimartMojibake(s string) string {
	var b strings.Builder
	for _, r := range s {
		if mapped, ok := fujimartMojibakeMap[r]; ok {
			b.WriteRune(mapped)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var validDatePattern = regexp.MustCompile(`^\d{2}/\d{2}/\d{4}$`)

// ParseOrderInfo mirrors process_file's FujiMart branch
// (xulydonhang.py:8831-8930), minus the OCR step — see the design spec's
// "Không cần OCR" section for the full verification that storeInfo is
// reconstructable from the plain text layer alone.
//
// entryDate (xulydonhang.py:8853-8857): the line 3 positions BEFORE the
// line containing "Sè §¬n:" — a positional offset within this PDF
// template's fixed "values-then-labels" block, NOT a marker-adjacent
// value. Confirmed identical relative line ordering in both PyMuPDF's
// and this repo's own extractPageTexts output across multiple real
// FujiMart PDFs during planning.
//
// poNumber (xulydonhang.py:8885-8887): the line immediately AFTER the
// line whose content exactly equals the entryDate value.
//
// cancelDate (xulydonhang.py:8859): Python assumes the "Ngµy giao:"
// label and its value sit on the SAME line and splits on the literal
// marker. Confirmed this repo's own extractPageTexts instead puts them
// on two SEPARATE lines for real FujiMart PDFs (same class of layout
// mismatch already fixed for Emart's ParseOrderInfo) — valueAfterMarker
// tolerates BOTH shapes.
//
// Cross-validate/fallback ±2 days (xulydonhang.py:8862-8884): ported
// exactly, no simplification.
//
// storeInfo (xulydonhang.py:8895-8899, via OCR of "Nơi nhận:"): the
// 5-digit store code (the line right after "N¬i nhËn:") + " " + the
// line starting with "FujiMart " (decoded via decodeFujimartMojibake).
// Best-effort — matches Python's tenstore defaulting to "" when its OCR
// regex doesn't match (xulydonhang.py:8895) — does NOT gate ok.
//
// ok=false when poNumber, entryDate, or cancelDate fails to resolve to a
// real value (including the "Không tìm thấy" fallback-exhausted case) —
// Python's real code would carry an undefined/garbage value into
// several downstream operations in that case (entry_date could even be
// a genuine NameError, xulydonhang.py:8857's print(entry_date) if the
// marker line is never found at all), which this port treats as a clean
// failure instead, per this codebase's established policy.
func ParseOrderInfo(text string) (poNumber, entryDate, cancelDate, storeInfo string, ok bool) {
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			lines = append(lines, t)
		}
	}

	for i, l := range lines {
		if strings.Contains(l, "Sè §¬n:") && i >= 3 {
			entryDate = lines[i-3]
			break
		}
	}

	for i, l := range lines {
		if l == entryDate && i+1 < len(lines) {
			poNumber = lines[i+1]
			break
		}
	}

	cancelDate = valueAfterMarker(lines, "Ngµy giao:")

	if !validDatePattern.MatchString(cancelDate) {
		cancelDate = "Không tìm thấy"
		if validDatePattern.MatchString(entryDate) {
			if t, err := time.Parse("02/01/2006", entryDate); err == nil {
				cancelDate = t.AddDate(0, 0, 2).Format("02/01/2006")
			}
		}
	}
	if !validDatePattern.MatchString(entryDate) {
		entryDate = "Không tìm thấy"
		if validDatePattern.MatchString(cancelDate) {
			if t, err := time.Parse("02/01/2006", cancelDate); err == nil {
				entryDate = t.AddDate(0, 0, -2).Format("02/01/2006")
			}
		}
	}

	storeCode := ""
	for i, l := range lines {
		if strings.Contains(l, "N¬i nhËn:") && i+1 < len(lines) {
			storeCode = lines[i+1]
			break
		}
	}
	branchLine := ""
	for _, l := range lines {
		if strings.HasPrefix(l, "FujiMart ") {
			branchLine = l
			break
		}
	}
	if storeCode != "" && branchLine != "" {
		storeInfo = storeCode + " " + decodeFujimartMojibake(branchLine)
	}

	ok = poNumber != "" &&
		entryDate != "" && entryDate != "Không tìm thấy" &&
		cancelDate != "" && cancelDate != "Không tìm thấy"
	return poNumber, entryDate, cancelDate, storeInfo, ok
}

// valueAfterMarker finds the line containing marker, then returns either
// the remainder of that same line (after the marker text, trimmed) if
// non-empty, or the next line if the marker line has nothing left after
// it — tolerating both the same-line layout Python's own split-based
// extraction assumes and the two-line layout this repo's actual Go PDF
// extraction produces for real FujiMart PDFs.
func valueAfterMarker(lines []string, marker string) string {
	for i, l := range lines {
		idx := strings.Index(l, marker)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(l[idx+len(marker):])
		if rest != "" {
			return rest
		}
		if i+1 < len(lines) {
			return lines[i+1]
		}
		return ""
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/fujimart/... -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/fujimart/extract.go GO/internal/processing/fujimart/extract_test.go
git commit -m "feat(go): add fujimart package with PO/date/store-info extraction and mojibake decode"
```

---

### Task 3: `fujimart` package — product-table extraction

**Files:**
- Modify: `GO/internal/processing/fujimart/extract.go`
- Modify: `GO/internal/processing/fujimart/extract_test.go`

**Interfaces:**
- Consumes: nothing new (independent of Task 2's functions).
- Produces: `type Product struct { Barcode, OUQty, TotalPrice string }` and `ExtractProducts(text string) []Product` — consumed by Task 4's `processFujimartSegment`.

- [ ] **Step 1: Write the failing tests**

Append to `GO/internal/processing/fujimart/extract_test.go`:

```go
func TestExtractProducts_ParsesRealSampleFiveProducts(t *testing.T) {
	// Exact shape of this repo's OWN extractPageTexts output for the
	// product-table region of đơn hàng/08-2026/103001302608001342.pdf,
	// confirmed during planning by running the actual Go PDF pipeline —
	// including the internal SKU-like code line after each barcode
	// (e.g. "2006324377") that the regex must correctly skip over.
	text := "§Þa chØ:\n" +
		"1\n12.0\n1,695,264\nTUI\n141,272\nBLUE -N­íc giÆt x¶ ®Ëm ®Æc H. Th¶o méc 3.6 l\n8936156730879\n2006324377\n" +
		"2\n12.0\n1,695,264\nTUI\n141,272\nBLUE -N­íc giÆt x¶ ®Ëm ®Æc H. N­íc hoa 3.6 l\n8936156730886\n2006324378\n" +
		"3\n12.0\n490,836\nTUI\n40,903\nBLUE -N­íc röa chÐn chiÕt xuÊt g¹o tói 2.1L\n8936156730473\n2006324379\n" +
		"4\n12.0\n490,836\nTUI\n40,903\nBLUE -N­íc röa chÐn chiÕt xuÊt ®Ëu xanh tói 2.1L\n8936156730466\n2006324380\n" +
		"5\n12.0\n452,472\nCH\n37,706\nBLUE -Chai th¶ bån cÇu toilet h­¬ng Ngµn hoa 180g\n8809174900138\n2006324382\n" +
		"4,824,672\n" +
		"ng­êi ®Æt ®¬n\n" +
		"VAT\n"

	products := ExtractProducts(text)
	if len(products) != 5 {
		t.Fatalf("len(products) = %d, want 5", len(products))
	}
	want := []Product{
		{Barcode: "8936156730879", OUQty: "12.0", TotalPrice: "1695264"},
		{Barcode: "8936156730886", OUQty: "12.0", TotalPrice: "1695264"},
		{Barcode: "8936156730473", OUQty: "12.0", TotalPrice: "490836"},
		{Barcode: "8936156730466", OUQty: "12.0", TotalPrice: "490836"},
		{Barcode: "8809174900138", OUQty: "12.0", TotalPrice: "452472"},
	}
	for i, w := range want {
		if products[i] != w {
			t.Errorf("products[%d] = %+v, want %+v", i, products[i], w)
		}
	}
}

func TestExtractProducts_NoTableMarkerReturnsEmpty(t *testing.T) {
	products := ExtractProducts("no address marker or VAT anywhere in this text")
	if products != nil {
		t.Errorf("ExtractProducts = %v, want nil", products)
	}
}

func TestExtractProducts_NoMatchingRowsReturnsEmpty(t *testing.T) {
	products := ExtractProducts("§Þa chØ:\nnothing shaped like a product row\nVAT\n")
	if products != nil {
		t.Errorf("ExtractProducts = %v, want nil", products)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/fujimart/... -run TestExtractProducts -v`
Expected: FAIL with a build error (`Product`/`ExtractProducts` don't exist yet).

- [ ] **Step 3: Write the implementation**

Append to `GO/internal/processing/fujimart/extract.go` (add `"strconv"` is NOT needed — no numeric parsing happens in this package; values stay as strings for the processor to parse):

```go
// productTablePattern isolates the product-table region, mirroring
// xulydonhang.py:8903-8907 exactly: text.rfind("§Þa chØ:") to
// text.find("VAT"). Confirmed via real extraction that "§Þa chØ:" (this
// exact mojibake spelling) appears exactly once in real FujiMart page
// text, right before the product table — distinct from "§i¹ chØ:" (a
// different mojibake spelling, the vendor/NCC's own address label,
// appearing earlier in the text and never matched by this literal
// string).
var productTablePattern = regexp.MustCompile(`(?s)§Þa chØ:(.*?)VAT`)

// productLinePattern mirrors tach_san_pham_Fujimart's compiled regex
// (xulydonhang.py:7020-7028) exactly: 7 fields — STT, quantity, total
// amount, unit, unit price, product name, and a trailing barcode/product
// code. Go has no re.VERBOSE mode; the pattern below is the same shape
// with the VERBOSE-only whitespace/comments removed. Python's pattern
// uses re.MULTILINE but never references ^ or $, so that flag is
// vestigial there and is correctly omitted here too. re.DOTALL is also
// NOT used by Python for this pattern, so "." here does not match
// newlines either — matches Python exactly.
//
// Confirmed via direct testing against this repo's own extractPageTexts
// output for a real FujiMart PDF (see this task's own tests) that Go's
// non-backtracking FindAllStringSubmatch reproduces the same
// "skip the stray internal SKU-code line after each barcode" behavior
// Python's finditer exhibits: after the first full match's barcode
// group ends, the scan resumes right after it; the stray internal code
// line fails to satisfy the pattern's quantity/unit groups at that
// position (its digits don't line up with the expected shape), so the
// scanner naturally advances to the next real STT and matches there —
// this is a property of the pattern's own structure (no ambiguous
// internal backtracking needed), not something either engine does
// specially.
var productLinePattern = regexp.MustCompile(`(\d+)\n([\d.]+)\n([\d,]+)\n([A-Z]+)\n([\d,]+)\n(.+?)\n(\d+|[A-Z0-9]+)`)

// Product is one extracted FujiMart product line. Only Barcode, OUQty,
// and TotalPrice are used downstream by processFujimartSegment — Python
// captures "Unit"/"Unit Price"/"Product Name" too (xulydonhang.py:7043-
// 7050) but write_to_dondathang_fujimart never reads them (product name
// is always re-looked-up via timten_sanpham, xulydonhang.py:2821), so
// this struct omits them entirely.
type Product struct {
	Barcode    string
	OUQty      string
	TotalPrice string
}

// ExtractProducts mirrors tach_san_pham_Fujimart (xulydonhang.py:7017-
// 7053) plus the table-isolation step that always runs immediately
// before it (xulydonhang.py:8903-8909). If the "§Þa chØ:...VAT"
// isolation doesn't match at all, Python's real code would crash
// (start = -1 from rfind, producing a nonsensical negative-index slice
// that either errors or silently returns garbage depending on exact
// values) — this returns nil instead, per this codebase's established
// clean-failure policy.
func ExtractProducts(text string) []Product {
	tableMatch := productTablePattern.FindStringSubmatch(text)
	if tableMatch == nil {
		return nil
	}
	tableText := strings.TrimSpace(tableMatch[1])

	var products []Product
	for _, m := range productLinePattern.FindAllStringSubmatch(tableText, -1) {
		products = append(products, Product{
			Barcode:    m[7],
			OUQty:      strings.ReplaceAll(m[2], ",", ""),
			TotalPrice: strings.ReplaceAll(m[3], ",", ""),
		})
	}
	return products
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/fujimart/... -v`
Expected: PASS, all 8 tests (4 from Task 2 + 3 new from this task... actually 4 new — recount when running).

- [ ] **Step 5: Verify against the REAL sample PDF's actual Go-extracted text, not just the literal test strings above**

The test strings above were transcribed from this repo's own `extractPageTexts` output against the real sample, captured during planning — but transcription errors are possible. Copy `đơn hàng/08-2026/103001302608001342.pdf` to a temporary location if needed, and run a throwaway scratch test (or `go run` snippet) calling `extractPageTexts` (package `processing`, not `fujimart`) directly against it, then feed that real output through `fujimart.ExtractProducts`. It MUST extract exactly the 5 products listed in Step 1's test, in that exact order. Remove the scratch code before committing — do not leave it in the repo.

If the real extraction doesn't match, that's a real bug in the regex to fix — do not adjust the test to match incorrect real-PDF behavior instead.

- [ ] **Step 6: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/fujimart/extract.go GO/internal/processing/fujimart/extract_test.go
git commit -m "feat(go): add product-table extraction to fujimart package"
```

---

### Task 4: `fujimart_processor.go` (dispatch + row builder)

**Files:**
- Create: `GO/internal/processing/fujimart_processor.go`
- Create: `GO/internal/processing/fujimart_processor_test.go`
- Modify: `GO/internal/processing/coop_processor.go` (add `case "FujiMart":` to the dispatch switch)
- Create: `GO/internal/processing/testdata/sample_fujimart_order.pdf` (copy from `đơn hàng/08-2026/103001302608001342.pdf`)

**Interfaces:**
- Consumes: `fujimart.ParseOrderInfo`, `fujimart.Product`, `fujimart.ExtractProducts` (Tasks 2-3); `buildPromoBonusRow`, `buildInvoiceBonusRow`, `coopDebtDays`, `closeEnough`, `parseNumericField` (existing shared helpers from `processor_shared.go`/`bigc_processor.go`).
- Produces: `processFujimartSegment`, `fujimartRegionInfo`, `fujimartOrderNumber` — consumed by Task 6's golden test only indirectly (via `RealProcessor.Process`).

- [ ] **Step 1: Copy the sample PDF**

```bash
cp "đơn hàng/08-2026/103001302608001342.pdf" GO/internal/processing/testdata/sample_fujimart_order.pdf
```

Verify byte-identical: `cmp "đơn hàng/08-2026/103001302608001342.pdf" GO/internal/processing/testdata/sample_fujimart_order.pdf`.

- [ ] **Step 2: Write the failing processor tests**

Create `GO/internal/processing/fujimart_processor_test.go`:

```go
package processing

import (
	"context"
	"testing"

	"github.com/xuri/excelize/v2"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleFujiMartFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	pricingSource := &fixturePricingSource{index: pricing.ParseIndex([][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
	})}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_fujimart_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].System != "FujiMart" {
		t.Fatalf("System = %q, want %q", rows[0].System, "FujiMart")
	}
	if rows[0].StatusKind == StatusKindFailed {
		t.Fatalf("Process produced a Failed row: %+v", rows[0])
	}
	if rows[0].MaKhachHang != fujimartCustomerCode {
		t.Fatalf("MaKhachHang = %q, want the hardcoded constant %q", rows[0].MaKhachHang, fujimartCustomerCode)
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
	// 1 header + 5 products = 6 new rows, none should have a promo bonus
	// row since the synthetic pricing source above has no real promo data.
	if len(sheetRows) != 8+6 {
		t.Fatalf("total rows = %d, want %d (8 template + 1 header + 5 products)", len(sheetRows), 8+6)
	}
}

func TestFujimartRegionInfo(t *testing.T) {
	cases := []struct {
		name                                     string
		customerCode                             string
		wantRegion, wantStatCode, wantWarehouse string
	}{
		{
			name:          "the real, always-used hardcoded constant (MB branch)",
			customerCode:  fujimartCustomerCode,
			wantRegion:    "MT_MB",
			wantStatCode:  "HN",
			wantWarehouse: "TP_HN_12",
		},
		{
			name:          "non-MB code (unreachable with real input today, still tested)",
			customerCode:  "MN_SOMETHING",
			wantRegion:    "MT_MN",
			wantStatCode:  "LA",
			wantWarehouse: "LA_KHO2026",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRegion, gotStatCode, gotWarehouse := fujimartRegionInfo(tc.customerCode)
			if gotRegion != tc.wantRegion || gotStatCode != tc.wantStatCode || gotWarehouse != tc.wantWarehouse {
				t.Errorf("fujimartRegionInfo(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.customerCode, gotRegion, gotStatCode, gotWarehouse,
					tc.wantRegion, tc.wantStatCode, tc.wantWarehouse)
			}
		})
	}
}

// TestRealProcessor_FujimartNoBraceBonusRowUsesKMBoKemKhongCheBarcode
// regression-tests FujiMart's own no-{...}-brace fallback text
// ("KM Bó Kèm - Không Che Barcode", xulydonhang.py:2973) — the shared
// buildPromoBonusRow's default fallback ("KM Bó Kèm - Che Barcode")
// must be overridden at this call site. Unlike Winmart's/Emart's
// equivalent fix, FujiMart's own fallback STILL writes AP (both texts
// contain "bó kèm", so buildPromoBonusRow's own bundle detection is
// unaffected by the text override) — this test explicitly confirms AP
// stays populated, not cleared.
//
// Uses sample_fujimart_order.pdf's real first product (barcode
// 8936156730879, OU Qty 12.0, Total Price 1,695,264 -> per-unit
// giahoadon = 1695264/12 = 141272 — confirmed by direct extraction
// during planning) with a "2+1 SP0002" promo (an "X+1" match mentioning
// SP0002, a known internal SKU already present in the productdata test
// fixture — see TestFindSkusMentioned) and NO {...} braces.
func TestRealProcessor_FujimartNoBraceBonusRowUsesKMBoKemKhongCheBarcode(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	const promoValue = "2+1 SP0002"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156730879", "Nước giặt", "141272", promoValue},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_fujimart_order.pdf", 1); err != nil {
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
		case "8936156730879":
			mainRow = row
		case "SP0002":
			bonusRow = row
		}
	}
	if mainRow == nil || bonusRow == nil {
		t.Fatalf("missing expected rows: main=%v bonus=%v", mainRow, bonusRow)
	}

	if got := cell(mainRow, colPromoNote); got != "KM Bó Kèm - Không Che Barcode" {
		t.Errorf("main row PromoNote (AO) = %q, want %q (FujiMart's own no-brace fallback)", got, "KM Bó Kèm - Không Che Barcode")
	}
	if got := cell(mainRow, colPromoBundleSku); got == "" {
		t.Errorf("main row PromoBundleSku (AP) = %q, want NON-empty (FujiMart's no-brace branch DOES write AP, unlike Winmart/Emart)", got)
	}
	if got := cell(bonusRow, colPromoNote); got != "" {
		t.Errorf("bonus row PromoNote (AO) = %q, want empty (Python only ever writes AO onto the main row for FujiMart, never the bonus row)", got)
	}
}

// TestRealProcessor_FujimartInvoiceLevelPromoBonusRow covers the
// invoice-level ("Hóa Đơn") promo bonus row (xulydonhang.py:3010-3047)
// — Q gets only the FIRST mentioned SKU, not a joined list, same
// divergence already handled for Winmart/Emart.
//
// Uses all 5 of sample_fujimart_order.pdf's real products at their exact
// real per-unit price (confirmed against 103001302608001342.pdf:
// 12*141272 + 12*141272 + 12*40903 + 12*40903 + 12*37706 = 4,824,672,
// which matches the real PDF's own printed pre-VAT total exactly), so
// totalValue is a known, real, independently-confirmed constant.
func TestRealProcessor_FujimartInvoiceLevelPromoBonusRow(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "8936156730879", "SP1", "141272", ""},
		{"2", "8936156730886", "SP2", "141272", ""},
		{"3", "8936156730473", "SP3", "40903", ""},
		{"4", "8936156730466", "SP4", "40903", ""},
		{"5", "8809174900138", "SP5", "37706", ""},
		{"6", "Hóa Đơn", "", "0", "100000 SP0001 SP0002"},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.Process(context.Background(), "testdata/sample_fujimart_order.pdf", 1); err != nil {
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
	if got := cell(bonusRow, colQty); got != "48" {
		t.Errorf("invoice bonus row Qty (X) = %q, want %q (floor(totalValue=4824672 / amount=100000))", got, "48")
	}

	for _, row := range sheetRows {
		if cell(row, colSKU) == "SP0002" {
			t.Errorf("found a row with SKU (Q) = %q, want none (only the first mentioned SKU, SP0001, should get an invoice bonus row)", "SP0002")
		}
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_ProcessesRealSampleFujiMartFile|TestFujimartRegionInfo|TestRealProcessor_FujimartNoBraceBonusRowUsesKMBoKemKhongCheBarcode|TestRealProcessor_FujimartInvoiceLevelPromoBonusRow" -v`
Expected: FAIL with a build error (`processFujimartSegment`/`fujimartRegionInfo`/`fujimartCustomerCode` don't exist yet, and `vendor.Identify` isn't wired into the dispatch switch).

- [ ] **Step 4: Write `fujimart_processor.go`**

Create `GO/internal/processing/fujimart_processor.go`:

```go
package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/fujimart"
)

// fujimartCustomerCode mirrors write_to_dondathang_fujimart's default/
// only makhachhang value — the literal "MB_MT_FUJI", hardcoded at the
// process_file call site (xulydonhang.py:8919). FujiMart never derives a
// customer code from the PDF or OCR.
const fujimartCustomerCode = "MB_MT_FUJI"

// fujimartRegionInfo mirrors write_to_dondathang_fujimart's warehouse/
// region branching (xulydonhang.py:2753-2760). The non-MB branch is
// unreachable with real FujiMart input today — customerCode is always
// the hardcoded constant fujimartCustomerCode, which always starts with
// "MB" — but this is modeled as a full 2-branch function anyway,
// matching the winmartRegionInfo/emartRegionInfo precedent, for
// architectural consistency. Confirmed NOT a fit for the shared
// regionInfo() (processor_shared.go): that function's non-MB branch
// returns warehouse "LA_TP", but FujiMart's real non-MB warehouse is
// "LA_KHO2026" — the same divergence already handled for Winmart/Emart.
func fujimartRegionInfo(customerCode string) (region, statCode, warehouse string) {
	if strings.HasPrefix(customerCode, "MB") {
		return "MT_MB", "HN", "TP_HN_12"
	}
	return "MT_MN", "LA", "LA_KHO2026"
}

// fujimartOrderNumber mirrors write_to_dondathang_fujimart's order-
// number field (xulydonhang.py:2780): f'ĐĐH{vendor}{STT_donhang_str}'
// where vendor is the uppercased literal "FUJIMART" and STT_donhang_str
// is f"-{po_number}".
func fujimartOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHFUJIMART-%s", poNumber)
}

// processFujimartSegment mirrors the FujiMart branch of process_file
// (xulydonhang.py:8831-8964) plus write_to_dondathang_fujimart
// (:2732-3066). FujiMart is "1 page = 1 order", the same family as Coop/
// Lotte/Satra/Winmart/Emart. A trailing PDF page that lacks FujiMart's
// identify marker falls through to the shared per-page dispatch loop's
// default case (coop_processor.go), which emits a Failed/"Thất bại"
// OrderRow for that page.
func (p *RealProcessor) processFujimartSegment(filePath, text, pageLabel string) (OrderRow, error) {
	poNumber, entryDate, cancelDate, storeInfo, ok := fujimart.ParseOrderInfo(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được số PO/ngày đặt hàng")
	}

	products := fujimart.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	priceIndex, err := p.Pricing.FetchIndex("FUJIMART")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := fujimartRegionInfo(fujimartCustomerCode)
	orderNum := fujimartOrderNumber(poNumber)
	description := fmt.Sprintf("FUJIMART PO%s", poNumber)

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: storeInfo, CustomerCode: fujimartCustomerCode,
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
		totalPrice := parseNumericField(rawProduct.TotalPrice)

		lineWeight := productInfo.WeightKg * ouQty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(ouQty / productInfo.PackSize))
		}
		totalWeight += lineWeight

		// giahoadon (xulydonhang.py:2843): dongia / qty_ord_pcs — a LINE
		// TOTAL divided by quantity, the same shape as Winmart's
		// TotalPrice (NOT Emart's per-unit trap).
		invoicePrice := totalPrice / ouQty

		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice := parseNumericField(realPriceStr)

		promos := priceIndex.FindPromotions(barcode, entryDate)
		khuyenmai := ""
		matched := false
		finalPrice := realPrice

		for _, promo := range promos {
			value := promo.Value
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
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: storeInfo, CustomerCode: fujimartCustomerCode,
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

		// Per-item promo bonus row (xulydonhang.py:2949-3007) — single
		// attempt, buildPromoBonusRow always called with index=0 (no
		// "|"-split multi-CTKM loop, matching Winmart's/Lotte's shape,
		// not Coop's/Emart's).
		bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, khuyenmai,
			coop.Product{Barcode: barcode, Qty: ouQty}, 0, entryDate, cancelDate, storeInfo,
			fujimartCustomerCode, description, warehouse, region, statCode, orderNum)
		if added {
			totalWeight += bonusRow.LineWeightKg

			// FujiMart's own no-{...}-brace fallback text
			// (xulydonhang.py:2973, "KM Bó Kèm - Không Che Barcode")
			// differs from buildPromoBonusRow's shared Coop-flavored
			// default ("KM Bó Kèm - Che Barcode") — but BOTH strings
			// contain "bó kèm", so buildPromoBonusRow's own internal
			// bundle-SKU (AP) computation is unaffected by this text
			// override; only the AO note text itself needs overriding.
			// Python writes this fallback text ONLY onto the main
			// product row (xulydonhang.py:2973, at current_row, before
			// current_row is incremented for the bonus row) — never
			// onto the bonus row itself, matching buildPromoBonusRow's
			// own index==0 behavior (which likewise never sets the
			// bonus row's own PromoNote).
			if coop.ExtractBraceContent(khuyenmai) == "" {
				mainRowNote = "KM Bó Kèm - Không Che Barcode"
			}

			rows[productRowIndex].PromoNote = mainRowNote
			if mainRowBundleSku != "" {
				rows[productRowIndex].PromoBundleSku = mainRowBundleSku
			}
			rows = append(rows, bonusRow)
		}
	}

	// Invoice-level ("Hóa Đơn") promo bonus row (xulydonhang.py:3010-3047).
	// Does NOT reuse the shared buildInvoiceBonusRow — Q gets only the
	// first matched SKU (kiemtra[0]), not a joined list, the same
	// divergence already handled for Winmart/Emart.
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
				invoiceNote = "KM Bó Kèm - Che Barcode" // xulydonhang.py:3044
			}

			rows = append(rows, excelwriter.Row{
				EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
				Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: storeInfo, CustomerCode: fujimartCustomerCode,
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
		FileName: filepath.Base(filePath), Page: pageLabel, System: "FujiMart", MaKhachHang: fujimartCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
	}, nil
}
```

- [ ] **Step 5: Wire the `case "FujiMart":` into `RealProcessor.Process`'s dispatch switch**

In `GO/internal/processing/coop_processor.go`, add a `case "FujiMart":` block into the `switch v {` statement, as the LAST case before `default:` (after the existing `case "Winmart":` block):

```go
		case "FujiMart":
			row, err := p.processFujimartSegment(filePath, text, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "FujiMart",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_ProcessesRealSampleFujiMartFile|TestFujimartRegionInfo|TestRealProcessor_FujimartNoBraceBonusRowUsesKMBoKemKhongCheBarcode|TestRealProcessor_FujimartInvoiceLevelPromoBonusRow" -v`
Expected: PASS, all tests.

Also run the full existing suite to confirm no other vendor regressed:
Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures" -v`
Expected: every other vendor's own suite unaffected by this task's changes (their pass/fail state may currently reflect the live `đơn hàng/` folder's own unrelated availability — see this plan's Global Constraints and Emart's plan history; confirm via `git stash` if there's any doubt whether THIS task's own commit changed anything, the same technique used throughout Emart's plan).

- [ ] **Step 7: Commit**

```bash
cd GO && go build ./... && go vet ./...
git add GO/internal/processing/fujimart_processor.go GO/internal/processing/fujimart_processor_test.go GO/internal/processing/coop_processor.go GO/internal/processing/testdata/sample_fujimart_order.pdf
git commit -m "feat(go): dispatch RealProcessor to FujiMart via processFujimartSegment"
```

---

### Task 5: Copy real PDFs into stable testdata + golden fixture generation script (throwaway)

**Files:**
- Create: `GO/internal/processing/fujimart/testdata/realpdfs/*.pdf` (15 files, copied)
- Create: `GO/internal/processing/fujimart/testdata/generate_fixtures.py`

**Interfaces:**
- Consumes: real, unmodified `xulydonhang.py` (repo root) — never modified by this task.
- Produces: `GO/internal/processing/fujimart/testdata/fixtures/*.json` + `_frozen_pricing.json` — consumed by Task 6.

**This project-owner-confirmed decision (unlike Emart, confirmed explicitly for FujiMart, not assumed from Emart's precedent): commit the 15 currently-available real FujiMart PDFs into a stable, git-tracked local directory from the START**, rather than reading from the live `đơn hàng/` tree at test-run time (which Emart's own plan demonstrated mid-session to be an unstable dependency for this project's other vendors).

- [ ] **Step 1: Copy the 15 real PDFs into stable testdata**

Create `GO/internal/processing/fujimart/testdata/realpdfs/` and copy each of the 15 files below from its current location to a clean `<PONumber>.pdf` name in that directory. Use these EXACT source paths (already verified to exist during planning — re-verify each still exists before copying, since the live `đơn hàng/08-2026/` entry could have moved again by the time this task runs; if it has, search `đơn hàng/mẫu đơn hàng/*/` for a `[FujiMart]`-tagged file with the same PO number in brackets, the same technique already used once during this plan's own brainstorming):

```
"đơn hàng/08-2026/103001302608001342.pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/103001302608001342.pdf
"đơn hàng/mẫu đơn hàng/04-08-2026/04-08-2026_[FujiMart][01-08-2026][MB_MT_FUJI][03-08-2026][102001302608000155].pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/102001302608000155.pdf
"đơn hàng/mẫu đơn hàng/05-08-2026/05-08-2026_[FujiMart][05-08-2026][MB_MT_FUJI][07-08-2026][105001302608000288].pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/105001302608000288.pdf
"đơn hàng/mẫu đơn hàng/10-08-2026/10-08-2026_[FujiMart][09-08-2026][MB_MT_FUJI][11-08-2026][117003302608000667].pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/117003302608000667.pdf
"đơn hàng/mẫu đơn hàng/10-08-2026/10-08-2026_[FujiMart][10-08-2026][MB_MT_FUJI][12-08-2026][106003302608000751].pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/106003302608000751.pdf
"đơn hàng/mẫu đơn hàng/13-07-2026/13-07-2026_[FujiMart][12-07-2026][MB_MT_FUJI][14-07-2026][116001302607000453].pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/116001302607000453.pdf
"đơn hàng/mẫu đơn hàng/13-07-2026/13-07-2026_[FujiMart][12-07-2026][MB_MT_FUJI][14-07-2026][117003302607000942].pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/117003302607000942.pdf
"đơn hàng/mẫu đơn hàng/14-07-2026/14-07-2026_[FujiMart][13-07-2026][MB_MT_FUJI][15-07-2026][103001302607000991].pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/103001302607000991.pdf
"đơn hàng/mẫu đơn hàng/18-07-2026/18-07-2026_[FujiMart][17-07-2026][MB_MT_FUJI][19-07-2026][101003302607001286].pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/101003302607001286.pdf
"đơn hàng/mẫu đơn hàng/20-07-2026/20-07-2026_[FujiMart][20-07-2026][MB_MT_FUJI][22-07-2026][108003302607001012].pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/108003302607001012.pdf
"đơn hàng/mẫu đơn hàng/22-07-2026/22-07-2026_[FujiMart][21-07-2026][MB_MT_FUJI][23-07-2026][102001302607001667].pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/102001302607001667.pdf
"đơn hàng/mẫu đơn hàng/27-07-2026/27-07-2026_[FujiMart][15-07-2026][MB_MT_FUJI][17-07-2026][124003302607000742].pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/124003302607000742.pdf
"đơn hàng/mẫu đơn hàng/27-07-2026/27-07-2026_[FujiMart][27-07-2026][MB_MT_FUJI][29-07-2026][104001302607001834].pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/104001302607001834.pdf
"đơn hàng/mẫu đơn hàng/28-07-2026/28-07-2026_[FujiMart][28-07-2026][MB_MT_FUJI][30-07-2026][122003302607001901].pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/122003302607001901.pdf
"đơn hàng/mẫu đơn hàng/31-07-2026/31-07-2026_[FujiMart][01-08-2026][MB_MT_FUJI][03-08-2026][110003302608000013].pdf" -> GO/internal/processing/fujimart/testdata/realpdfs/110003302608000013.pdf
```

These are COPIES (read-only source access) — do not move, rename, or modify anything under `đơn hàng/` itself. `đơn hàng/` is git-ignored (`.gitignore:19`, `**/đơn hàng/`) so nothing under it is trackable or committed by this repo anyway — only the new copies under `GO/internal/processing/fujimart/testdata/realpdfs/` (not gitignored) will be committed.

Verify each copy is byte-identical to its source (`cmp` or equivalent) before proceeding. The `103001302608001342.pdf` copy should also be byte-identical to Task 4's already-committed `GO/internal/processing/testdata/sample_fujimart_order.pdf` — confirm this too.

- [ ] **Step 2: Write the fixture-generation script**

Create `GO/internal/processing/fujimart/testdata/generate_fixtures.py`. Adapted from the same base as Emart's harness (`GO/internal/processing/emart/testdata/generate_fixtures.py` — read it in full first) with the same structural shape: reads PDFs from the stable local `realpdfs/` directory (no live-folder dependency, no vendor pre-filter needed since every file there is already confirmed FujiMart-only), same backup/restore protocol, retry-with-backoff, UTF-8 stdout fix, pricing/promotion monkeypatch caching.

```python
"""
Throwaway dev tool — NOT part of the shipped Go or Python app.

Runs the CURRENT xulydonhang.py FujiMart pipeline against the 15 real
FujiMart PDFs copied into
GO/internal/processing/fujimart/testdata/realpdfs/ (see this task's own
Step 1 — committed directly into this repo's testdata from the start,
per explicit project-owner confirmation for this vendor, unlike the
first 5 ported vendors' harnesses which still read from the live
đơn hàng/ tree at test-run time), capturing the resulting dondathang.xlsx
rows (and the live-fetched Google Sheets price/promotion data for the
FUJIMART sheet) into JSON fixtures under
GO/internal/processing/fujimart/testdata/fixtures/. The Go golden test
(Task 6) diffs RealProcessor's output against these fixtures instead of
against a live Google Sheets fetch, so it's deterministic and offline.

Like Satra/Lotte/Winmart/Emart (one page == one order,
write_to_dondathang_fujimart appends immediately, no explicit start-row
argument needed), and UNLIKE BigC, this harness computes start_row once
up front and takes a single snapshot after process_one_pdf's per-page
loop has finished.

Run from the repo root:
    .venv/Scripts/python.exe GO/internal/processing/fujimart/testdata/generate_fixtures.py
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
    REPO_ROOT, "GO", "internal", "processing", "fujimart", "testdata", "realpdfs"
)
FIXTURES_DIR = os.path.join(
    REPO_ROOT, "GO", "internal", "processing", "fujimart", "testdata", "fixtures"
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
    """Mirrors the FujiMart branch of process_file (xulydonhang.py:8831-
    8964) for every page identify_vendor recognizes as FujiMart, skipping
    the Google Drive upload / current-page-extraction side effects and
    the OCR step (this harness runs the REAL Python function, which still
    does OCR internally — that's fine, this harness's job is only to
    capture ground-truth OUTPUT, not to validate the Go port's own
    no-OCR design, which Task 2/6 validate separately)."""
    handler = xulydonhang.process_handler
    doc = xulydonhang.fitz.open(path)
    try:
        for page_num in range(len(doc)):
            text = doc[page_num].get_text("text")
            if xulydonhang.ProcessHandler.identify_vendor(text) != "FujiMart":
                continue

            import io
            import re
            from datetime import datetime, timedelta
            from PIL import Image, ImageEnhance
            import pytesseract

            page = doc.load_page(page_num)
            pix = page.get_pixmap(dpi=500)
            img = Image.open(io.BytesIO(pix.tobytes("png")))
            img = img.convert("L")
            img = ImageEnhance.Contrast(img).enhance(2.5)
            info = pytesseract.image_to_string(img, lang="vie")

            lines = [line.strip() for line in text.strip().splitlines() if line.strip()]
            entry_date = None
            for i, line in enumerate(lines):
                if "Sè §¬n:" in line and i >= 3:
                    entry_date = lines[i - 3]

            cancel_date = next((line.split("Ngµy giao:")[1].strip() for line in text.splitlines() if "Ngµy giao:" in line), "")

            if not re.match(r"\d{2}/\d{2}/\d{4}$", cancel_date):
                cancel_date = "Không tìm thấy"
                if cancel_date == "Không tìm thấy" and entry_date and re.match(r"\d{2}/\d{2}/\d{4}$", entry_date):
                    entry_date_obj = datetime.strptime(entry_date, "%d/%m/%Y")
                    cancel_date = (entry_date_obj + timedelta(days=2)).strftime("%d/%m/%Y")

            if not entry_date or not re.match(r"\d{2}/\d{2}/\d{4}$", entry_date):
                entry_date = "Không tìm thấy"
                if entry_date == "Không tìm thấy" and re.match(r"\d{2}/\d{2}/\d{4}$", cancel_date):
                    cancel_date_obj = datetime.strptime(cancel_date, "%d/%m/%Y")
                    entry_date = (cancel_date_obj - timedelta(days=2)).strftime("%d/%m/%Y")

            m = re.search(rf"^{entry_date}\s*\n(.+)", text, re.MULTILINE)
            po_number = m.group(1) if m else None

            tenstore = ""
            match = re.search(r"N\s*ơ\s*i\s*[\s]*n\s*h\s*ậ\s*n\s*:\s*(.+?)(?=\n|$)", info, re.IGNORECASE)
            if match:
                tenstore = match.group(1)

            start = text.rfind("§Þa chØ:")
            end = text.find("VAT")
            product_block = text[start + len("§Þa chØ:"):end].strip()
            result = "\n".join(product_block.splitlines())

            products = xulydonhang.ProcessHandler.tach_san_pham_Fujimart(result)
            if not products:
                continue
            sku_mapping = xulydonhang.ProcessHandler.load_sku_mapping()
            products = xulydonhang.ProcessHandler.replace_sku_numbers(products, sku_mapping)

            xulydonhang.ProcessHandler.write_to_dondathang_fujimart(
                handler, products, "MB_MT_FUJI", po_number, entry_date, cancel_date,
                1, "FujiMart", tenstore, None,
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
        _capture_promo_raw_rows("FUJIMART")
    frozen_pricing = {"raw_rows": _promo_raw_rows or []}
    with open(os.path.join(FIXTURES_DIR, "_frozen_pricing.json"), "w", encoding="utf-8") as f:
        json.dump(frozen_pricing, f, ensure_ascii=False, indent=2, default=str)

    print(f"\nDone: {generated} fixtures generated, {skipped} PDFs skipped.")


if __name__ == "__main__":
    main()
```

- [ ] **Step 3-6: Production workbook backup/run/verify/cleanup protocol, then spot-check**

```bash
cp dondathang.xlsx dondathang.xlsx.manual_backup_before_fujimart_fixtures
.venv/Scripts/python.exe GO/internal/processing/fujimart/testdata/generate_fixtures.py
diff dondathang.xlsx dondathang.xlsx.manual_backup_before_fujimart_fixtures
```
Expected: `diff` reports no differences. If IDENTICAL, remove the backup. If NOT identical, STOP — investigate via `log.log` timestamps before touching anything else (this file is live production data, and a concurrent real process may be writing to it — same protocol every prior vendor's harness has used).

Read 2-3 of the generated `GO/internal/processing/fujimart/testdata/fixtures/*.json` files directly. Confirm plausibility: `B` column looks like `"ĐĐHFUJIMART-<po_number>"`, `E` column (ShipTo) shows a real `"<code> FujiMart <branch>"` string in CLEAN Vietnamese (this is Python's real OCR output, the ground truth Task 6 will compare the Go port's no-OCR reconstruction against), `AU` is populated (non-null) on product rows, `Q`/`X`/`Y` are sane. Document anything surprising for Task 6's awareness — especially whether any fixture's `E` value uses a Vietnamese character NOT in Task 2's 15-entry `fujimartMojibakeMap` (if so, that's real evidence to extend the table, not something to guess at).

- [ ] **Step 7: Commit**

```bash
git add GO/internal/processing/fujimart/testdata/realpdfs/ GO/internal/processing/fujimart/testdata/generate_fixtures.py GO/internal/processing/fujimart/testdata/fixtures/
git commit -m "test(go): copy 15 real FujiMart PDFs into stable testdata and generate golden fixtures"
```

---

### Task 6: Golden fixture integration test — FujiMart

**Files:**
- Create: `GO/internal/processing/fujimart_golden_test.go`

**Interfaces:**
- Consumes: `GO/internal/processing/fujimart/testdata/fixtures/*.json` and `GO/internal/processing/fujimart/testdata/realpdfs/*.pdf` (Task 5), `RealProcessor` (Task 4), `compareRowsAgainstFixture`/`fixtureData`/`fixturePricingSource`/`frozenPricingFixture`/`copyFile`/`joinLines` (existing shared golden-test helpers).
- Produces: nothing consumed by a later task — this is the plan's final task.

- [ ] **Step 1: Write the test**

Create `GO/internal/processing/fujimart_golden_test.go`:

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

// knownDivergences_Fujimart lists (fixture, row index, column) cells
// where this Go port intentionally computes a different, verified-more-
// correct value than the frozen Python fixture. Key format:
// "<source_pdf>:<row index>:<column>". Empty until a real, hand-verified
// case is confirmed; add entries here only with a comment citing the
// specific PDF/Python-line evidence — never to silence an unexplained
// diff.
//
// Coverage note: this test validates against all 15 real FujiMart PDFs
// available when this plan was executed (committed into
// fujimart/testdata/realpdfs/, not read from the live đơn hàng/ tree —
// see Task 5). More real FujiMart PDFs may exist beyond these 15;
// adding them later is a matter of copying into realpdfs/ and re-running
// generate_fixtures.py — this test globs its inputs, so no code change
// is needed here when that happens.
var knownDivergences_Fujimart = map[string]bool{}

func loadFrozenFujimartPricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("fujimart/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen FujiMart pricing fixture found (run Task 5's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen FujiMart pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

func TestRealProcessor_MatchesGoldenFixtures_Fujimart(t *testing.T) {
	fixturePaths, err := filepath.Glob("fujimart/testdata/fixtures/*.json")
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
	pricingSource := loadFrozenFujimartPricingSource(t)

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

		pdfPath := filepath.Join("fujimart", "testdata", "realpdfs", fixture.SourcePDF)
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

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_Fujimart)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
}
```

- [ ] **Step 2: Run — expect RED, investigate every mismatch**

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures_Fujimart" -v`

For every mismatch reported, investigate the root cause before deciding whether it's:
1. A real Go bug (fix it, in whichever file actually needs the fix, following this project's established practice of fixing real bugs found via the golden-fixture process even when they're outside this task's own file list).
2. A genuine, evidence-backed Python quirk this port deliberately does not reproduce — document it in `knownDivergences_Fujimart` with a comment citing the specific PDF and `xulydonhang.py` line evidence.
3. A pre-existing, unrelated failure in a DIFFERENT vendor's suite — not this task's concern; confirm via `git stash` that this task's own commit didn't change any other vendor's pass/fail state, the same technique used throughout Emart's plan.

**Specific things to check if a mismatch appears, given this plan's own flagged uncertainties:**
- Any `E` (ShipTo) mismatch — this is the highest-risk field in this whole plan. If the mismatch is a WRONG CHARACTER (not a wrong branch/code entirely), check whether that specific character is missing from `fujimartMojibakeMap` (Task 2) — if so, extend the table with real evidence (run `pytesseract` directly against that specific PDF to get the confirmed-correct clean Vietnamese, the same technique used during planning), don't guess. If the mismatch is a wrong branch/code entirely (not just a character), that's a real logic bug in the line-scan extraction to investigate fresh.
- Any `AU` mismatch on a product or bonus row — FujiMart writes AU normally (unlike Emart/BigC); confirm the plan's Global Constraint actually holds against real captured fixture data.
- Any mismatch on the barcode-boundary "skip the stray internal SKU-code line" behavior (see Task 3's own doc comment) — if `ExtractProducts` picks up a wrong barcode or wrong product count against a specific real fixture, that's the regex's non-greedy-vs-greedy scanning behaving differently than expected; investigate directly against that specific PDF's real Go-extracted text, don't assume Task 3's own unit tests already proved this for every real PDF (they only proved it for one).
- Any entry_date/cancel_date/po_number mismatch traceable to the `-3`-line-offset or same-line-vs-next-line-marker techniques (Task 2) not holding for a PDF whose layout differs slightly from the ones checked during planning.

- [ ] **Step 3: Fix, re-run, repeat until GREEN**

Iterate Steps 2-3 until `go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures_Fujimart" -v` passes clean.

- [ ] **Step 4: Final run and commit**

Run: `cd GO && go build ./... && go vet ./...`
Expected: clean build/vet.

Run: `cd GO && go test ./internal/processing/... -run "TestRealProcessor_MatchesGoldenFixtures_Fujimart" -v`
Expected: all 15 fixtures matched.

Do NOT treat a bare `go test ./...` failure elsewhere in the module as a gate for this task — every OTHER vendor's golden test may currently be affected by the live `đơn hàng/` folder's own unrelated availability (see this plan's Global Constraints). Confirm via `git stash` that this task's own commit specifically didn't change any other vendor's pass/fail state.

```bash
git add GO/internal/processing/fujimart_golden_test.go
git commit -m "test(go): add FujiMart golden fixture integration test (15 real PDFs)"
```
