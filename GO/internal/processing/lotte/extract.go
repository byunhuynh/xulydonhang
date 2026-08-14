package lotte

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// OrderInfo holds the PO number, entry date, and store code derived from
// a Lotte page's first two lines.
type OrderInfo struct {
	PONumber  string // formatted "yyMMdd-STORECODE-ORDER", e.g. "260727-01013-00057"
	EntryDate string // "dd/MM/yyyy"
	StoreCode string // the 5-digit middle segment of PONumber, e.g. "01013"
}

// ParseOrderInfo mirrors the PO-number/entry-date derivation at the top
// of process_file's Lotte branch (xulydonhang.py:9081-9092): the raw PO
// number is the page's SECOND line (index 1) — a 16-digit string with no
// separators (6-digit date + 5-digit store code + 5-digit order number).
// Hyphens are inserted at fixed byte offsets 6 and 12 to produce the
// formatted PO number, which is then split on "-" to recover each part.
//
// Deliberately more defensive than the Python original here: Python's
// po_number.split("-") is unpacked directly into 3 variables and raises
// an unhandled ValueError if the line doesn't produce exactly 3 hyphen-
// separated parts (e.g. too short to reach either insertion point) —
// this returns an error instead, per this plan's "correct main flow,
// don't need bug-for-bug parity" policy. Every real sample so far
// produces exactly 3 parts.
func ParseOrderInfo(text string) (OrderInfo, error) {
	lines := strings.Split(text, "\n")
	raw := ""
	if len(lines) > 1 {
		raw = lines[1]
	}

	poNumber := raw
	if len(poNumber) >= 7 {
		poNumber = poNumber[:6] + "-" + poNumber[6:]
	}
	if len(poNumber) >= 12 {
		poNumber = poNumber[:12] + "-" + poNumber[12:]
	}

	parts := strings.Split(poNumber, "-")
	if len(parts) != 3 {
		return OrderInfo{}, fmt.Errorf("không tách được số PO từ dòng thứ 2 của trang: %q", raw)
	}
	timePart, storeCode := parts[0], parts[1]

	entryDate, err := time.Parse("060102", timePart)
	if err != nil {
		return OrderInfo{}, fmt.Errorf("không đọc được ngày đặt hàng từ %q: %w", timePart, err)
	}

	return OrderInfo{
		PONumber:  poNumber,
		EntryDate: entryDate.Format("02/01/2006"),
		StoreCode: storeCode,
	}, nil
}

var dateLinePattern = regexp.MustCompile(`\d{1,2}/\d{1,2}/\d{4}`)

// ExtractCancelDate mirrors tachcancledate_lotte (xulydonhang.py:6051-6071):
// scans the lines between the line starting with poNumber and the line
// "00:00", keeping only lines that contain a d/m/yyyy-shaped date,
// joined back with a single newline. Returns "" if the (start, end)
// markers aren't both found (LinesBetween returns nil).
func ExtractCancelDate(text, poNumber string) string {
	between := LinesBetween(text, poNumber, "00:00")
	if between == nil {
		return ""
	}
	var filtered []string
	for _, line := range between {
		if dateLinePattern.MatchString(line) {
			filtered = append(filtered, strings.TrimSpace(line))
		}
	}
	return strings.Join(filtered, "\n")
}

// ExtractStoreName mirrors laytenstore_lotte (xulydonhang.py:6565-6584)
// exactly, including its edge-case behavior when the "DOAN TUAN ANH"
// anchor line and the poNumber line are adjacent — in that case Python's
// lines[end_index-1] resolves to the anchor line itself, not "".
//
// Not implemented via LinesBetween: that helper's returned slice can't
// distinguish "zero lines between the markers" from "the line
// immediately before the end marker IS the start marker itself", which
// this function needs to tell apart to match Python exactly.
func ExtractStoreName(text, poNumber string) string {
	lines := strings.Split(text, "\n")
	startIndex := -1
	endIndex := -1
	for i, line := range lines {
		if startIndex == -1 && strings.HasPrefix(line, "DOAN TUAN ANH") {
			startIndex = i
		}
		if strings.TrimSpace(line) == poNumber {
			endIndex = i
			break
		}
	}
	if startIndex == -1 || endIndex == -1 || startIndex >= endIndex {
		return ""
	}
	return strings.TrimSpace(lines[endIndex-1])
}

// Product is one product line extracted from a Lotte order's product
// table. Field names mirror tachsanpham_lotte's dict keys ("Qty-Box" ->
// QtyBox is the unit count per box; "Box Quantity" -> BoxQty is the
// number of boxes — write_to_dondathang_lotte computes total ordered
// quantity as QtyBox * BoxQty). "Product Code" and "Loose Quantity" are
// captured by Python's regex but never read anywhere in
// write_to_dondathang_lotte — omitted here (YAGNI).
type Product struct {
	Barcode    string
	QtyBox     int
	BoxQty     int
	TotalPrice float64
}

// productLinePattern mirrors tachsanpham_lotte's regex exactly
// (xulydonhang.py:6076): group 1 = product code (unused), group 2 =
// barcode, group 3 = unit count ("Qty-Box"), group 4 = box quantity
// ("Box Quantity"), group 5 = loose quantity (unused), group 6 = total
// price.
var productLinePattern = regexp.MustCompile(`(\d{1,2}-\d{6}-\d{3})\s+(\d{12,13})[\s\S]*?(\d+)\s+BOX\s+(\d+)\s+(\d+)\s+[\d,]+\s+([\d,]+)`)

// ExtractProducts mirrors tachsanpham_lotte (xulydonhang.py:6074-6091):
// cleans the order text down to the block between "Sply qty" and
// "Tot add tax" (lamsachdonhang_lotte, :6405-6423 — raw lines joined
// back with newlines, no per-line filtering), then extracts every
// product line matching productLinePattern.
func ExtractProducts(text string) []Product {
	between := LinesBetween(text, "Sply qty", "Tot add tax")
	if between == nil {
		return nil
	}
	cleaned := strings.Join(between, "\n")

	matches := productLinePattern.FindAllStringSubmatch(cleaned, -1)
	products := make([]Product, 0, len(matches))
	for _, m := range matches {
		qtyBox, _ := strconv.Atoi(m[3])
		boxQty, _ := strconv.Atoi(m[4])
		totalPrice, _ := strconv.ParseFloat(strings.ReplaceAll(m[6], ",", ""), 64)
		products = append(products, Product{
			Barcode:    m[2],
			QtyBox:     qtyBox,
			BoxQty:     boxQty,
			TotalPrice: totalPrice,
		})
	}
	return products
}
