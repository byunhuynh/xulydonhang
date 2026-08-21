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
