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

// addressLineWrapGapPattern repairs a real, confirmed PDF-text-extraction
// gap found via Task 6's golden-fixture run against the one real sample,
// DH01010844.pdf: real PyMuPDF's "text" mode reliably inserts a newline
// at every physical PDF line-wrap, so Python's real
// re.S-captured delivery_address (xulydonhang.py:8146-ish) keeps a
// literal "\n" where the address wraps mid-field — confirmed directly in
// the frozen golden fixture, whose ShipTo value contains an embedded
// "\n" between "...Gold View, 346" and "Bến Vân Đồn...".
//
// This repo's own PDF library (github.com/ledongthuc/pdf, via
// extractPageText/GetPlainText in pdfextract.go) only inserts "\n" on
// the content stream's BT/T* operators (confirmed by reading that
// library's own page.go Interpret callback). DH01010844.pdf renders the
// address's second physical line using a raw Td (line-position)
// operator instead of T*, so GetPlainText drops the separator ENTIRELY
// at that one spot — not even a space survives, fusing two words
// together. Confirmed directly: dumping this PDF's raw content-stream
// text runs (page.Content()) shows "...Gold View, 346" and "Bến Vân
// Đồn..." as two distinct physical text rows at different Y
// coordinates, while extractPageText's GetPlainText output has them as
// "...346Bến...", zero characters apart. (Whole-page position-based
// reconstruction, i.e. reconstructLinesFromContent, was tried and
// rejected as a general fix here: on this PDF it interleaves an
// unrelated field, "Người in: kimngoc", between the address's two
// physical lines, because that field happens to sit at an intervening Y
// coordinate elsewhere on the page — this repo's own existing caution
// about that function's reading-order assumptions, see
// reconstructLinesFromContent's doc comment in pdfextract.go.)
//
// This pattern targets exactly the confirmed failure signature: a digit
// immediately followed by an uppercase letter that itself starts a real
// (2+-letter) word. Deliberately narrower than "any digit followed by
// any uppercase letter" — this same address also contains "02B" (a
// legitimate room/floor code: digit run + ONE trailing capital letter +
// word boundary, not a fused word) which must NOT be touched.
// Requiring the uppercase letter to be followed by a further lowercase
// letter (\p{Lu}\p{Ll}, the start of a real word) distinguishes "346" +
// "Bến..." (real match: "B" followed by "ế", a letter) from "02" + "B"
// followed by a space (no match: "B" is followed by whitespace, not a
// letter).
//
// Scope: applied ONLY to the already-captured delivery-address
// substring below, never to the wider page text, so this cannot affect
// product-table parsing (ExtractProducts) or any other field.
// Single-sample coverage — see knownDivergences_JMart's own doc comment
// in jmart_golden_test.go: this heuristic is verified correct for
// exactly this one real address and not proven to generalize to a
// differently-shaped line-wrap in a future JMart PDF.
//
// KNOWN, CURRENTLY UNMITIGATED FALSE-POSITIVE RISK: this pattern cannot
// distinguish a genuine line-wrap fusion from a genuine Vietnamese
// house-number suffix like "Bis"/"Ter" (French-derived, still common in
// real Vietnamese addresses — e.g. "12Bis Nguyễn Thị Minh Khai", "5Ter
// Lê Duẩn", or even "L1 – 02Bis Tầng 1"), since those also have a digit
// directly touching an uppercase letter that starts a real word, with
// no space. This pattern WOULD spuriously inject a newline into any of
// those (e.g. "12Bis..." -> "12\nBis..."), corrupting an
// already-correct address the exact same way it repairs a genuinely
// fused one. ReplaceAllString also fires on every matching occurrence
// in the string, not just a single known wrap point, so a longer
// address containing more than one such transition would take more
// than one spurious injection.
//
// Deliberately NOT "fixed" with a smarter heuristic (e.g. word-length
// or diacritic checks to tell "Bis"/"Ter" apart from a real wrapped
// word): with only ONE real JMart PDF available for this entire
// vendor, there is no real evidence here to validate a more elaborate
// rule against — inventing one now would be an unverified guess, which
// this project's methodology treats as worse than an honestly
// documented gap. If a future real JMart sample surfaces this exact
// false positive (a "Bis"/"Ter"-style suffix getting corrupted), THAT
// is the evidence needed to design a real fix — not something to
// pre-empt without it.
//
// KNOWN FALSE-NEGATIVE GAP (symmetric to the false-positive risk above):
// requiring a LOWERCASE letter after the capital (\p{Lu}\p{Ll}) means an
// all-caps continuation word — e.g. a hypothetical wrap producing
// "346BẾN VÂN ĐỒN" — would NOT be repaired, silently diverging from
// Python's real inserted "\n". Not hypothetical for this template: this
// same PDF's supplier-address block is fully uppercase
// ("666/46 ĐƯỜNG 3/2.P.14.QUẬN 10,TP.HCM"), so an all-caps delivery
// address is a real shape this vendor's PDFs can produce, just not one
// this pattern currently covers.
var addressLineWrapGapPattern = regexp.MustCompile(`(\d)(\p{Lu}\p{Ll})`)

// ParseOrderInfo mirrors the JMart branch of process_file
// (xulydonhang.py:8146-8153). Python has NO try/except around
// entry_date's or po_number's regex match — a missing marker crashes
// Python outright with AttributeError: 'NoneType' object has no
// attribute 'group'. This port returns ok=false cleanly instead, per
// this codebase's established policy. delivery_address has a SOFTER
// guard in Python (`if m else None`, defaulting to None rather than
// crashing) at the regex site itself — but real Python does NOT ship a
// literal None into Excel: write_to_dondathang_kingfood (the shared
// function both Kingfood and JMart call) applies
// `delivery = delivery or "KHO SEEDLOG"` (xulydonhang.py:3865) before
// writing ShipTo, so a missing marker in real Python means the order
// still processes normally with ShipTo="KHO SEEDLOG". This port
// deliberately diverges from that: it gates ok on the address resolving
// too, failing the whole page instead of silently substituting a
// possibly-wrong warehouse address with no signal anything went wrong.
// This divergence has never been exercised — the one real sample has a
// valid address — so it is undocumented in any golden fixture.
//
// cancelDate is always exactly entryDate (xulydonhang.py:8148,
// `cancel_date = entry_date` — a direct assignment, no reformatting, no
// fallback logic, unlike FujiMart/Winmart/Emart's cross-validation).
//
// Confirmed during planning that the header/PO/date/address region
// keeps every MARKER and its value on directly matchable lines (no
// Go-vs-PyMuPDF divergence in whether "Địa chỉ giao hàng:" itself is
// found). What planning did NOT catch: the VALUE captured after that
// marker can itself lose an internal line-wrap separator — see
// addressLineWrapGapPattern's own doc comment, confirmed only during
// Task 6's real golden-fixture run, not during planning.
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
	deliveryAddress = addressLineWrapGapPattern.ReplaceAllString(deliveryAddress, "$1\n$2")

	return poNumber, entryDate, cancelDate, deliveryAddress, true
}

// tableStartMarker mirrors JMart's call to the shared
// cat_giua_theo_dong helper (xulydonhang.py:8155,
// dau_line="Mã vật tư"): a line STARTING WITH "Mã vật tư" (not
// necessarily an exact match — cat_giua_theo_dong uses .startswith,
// not ==) marks the beginning of the product table; everything AFTER
// that line, up to (not including) a line that exactly equals "Tổng:",
// is the product block.
const tableStartMarker = "Mã vật tư"
const tableEndMarker = "Tổng:"

// barcodePattern mirrors laydanhsachsanpham_kingfood-style barcode
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
