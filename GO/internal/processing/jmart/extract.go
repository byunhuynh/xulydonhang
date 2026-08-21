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
