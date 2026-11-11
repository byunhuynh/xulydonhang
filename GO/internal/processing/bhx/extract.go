// Package bhx parses Bách Hóa Xanh purchase orders ("ĐƠN ĐẶT HÀNG").
//
// It ports xulydonhang.py's BHX branch (process_file's `vendor == "BHX"`
// and tach_san_pham_BHX), but finds every header value by its label
// rather than by a fixed line offset from it. The one real order
// available when this was written (store 7300, PO 07300PO2611606019,
// 10/11/2026) shows why: this repo's extractPageTexts emits a blank
// " " line between "Mã đơn hàng:" and the number, and the order date on
// its own line after "Ngày đặt hàng:", so Python's `lines[idx + 1]`
// offsets would read an empty PO and no order date here.
//
// Two properties of the extracted text drive the product parsing:
//
//  1. Each table cell is its own line and a cell that wraps on the page
//     (the product name, "4 - Nhập mua / hàng (trong / nước)") becomes
//     several lines, so rows are found by their own shape — a row number
//     followed by BHX's own 13-digit layout code — not by position.
//
//  2. The table ends with its own "Tổng" row (total quantity and total
//     amount), which is returned alongside the rows so the caller can
//     prove the shape-matching read every row.
package bhx

import (
	"regexp"
	"strings"
)

var (
	datePattern      = regexp.MustCompile(`\b(\d{1,2}/\d{1,2}/\d{4})\b`)
	dateRangePattern = regexp.MustCompile(`(\d{1,2}/\d{1,2}/\d{4})\s*-\s*(\d{1,2}/\d{1,2}/\d{4})`)
	// poPattern is BHX's order number: store code, "PO", then digits
	// ("07300PO2611606019"). Python additionally turns any "." into "/".
	poPattern = regexp.MustCompile(`^[0-9A-Za-z./-]*PO[0-9A-Za-z./-]+$`)
	// storeLinePattern is the "Kho NCC giao hàng" value: the store's
	// numeric code, then its name ("7300 - BHX_HCM_Q12 - 31 Vườn Lài").
	storeLinePattern = regexp.MustCompile(`^(\d+)\s*-\s*\S`)

	rowNumberPattern  = regexp.MustCompile(`^\d{1,3}$`)
	layoutCodePattern = regexp.MustCompile(`^\d{10,15}$`)
	barcodePattern    = regexp.MustCompile(`^\d{8,14}$`)
	// moneyPattern matches the table's decimal cells (quantity, unit
	// price, discounted unit price, line total) — always printed with two
	// decimals — and nothing else in a row: the VAT rate prints as a bare
	// "8" and the barcodes have no decimal point.
	moneyPattern  = regexp.MustCompile(`^[\d,]+\.\d{2}$`)
	numberPattern = regexp.MustCompile(`^[\d,]+(?:\.\d+)?$`)
)

// OrderInfo is everything the page header carries about the order.
type OrderInfo struct {
	PONumber string
	// BranchCode is the numeric code of "Tên chi nhánh NCC" — the
	// supplier branch BHX orders from ("36560 - BHX <> CÔNG TY CP HÀ
	// THÀNH LONG AN 1"). The customer code is looked up by it ("BHX36560"),
	// as decided by the project owner on 2026-11-11, rather than by the
	// receiving store as in Python.
	BranchCode string
	// StoreCode is the store's numeric code from "Kho NCC giao hàng",
	// e.g. "7300".
	StoreCode string
	// StoreName is the whole "Kho NCC giao hàng" value, which is what
	// Python writes as the delivery address (column E).
	StoreName string
	// EntryDate is the order date (DD/MM/YYYY), without its time.
	EntryDate string
	// CancelDate is the END of the "Ngày NCC giao hàng" window — the last
	// day the store accepts delivery, as in Python.
	CancelDate string
}

// Product is one table row, numeric cells kept as printed.
type Product struct {
	Barcode       string
	Qty           string
	Price         string
	DiscountPrice string
	Total         string
}

// Totals is the table's own "Tổng" row.
type Totals struct {
	Qty    string
	Amount string
}

func splitLines(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r", "\n"), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// indexOf returns the first line containing label, or -1.
func indexOf(lines []string, label string) int {
	for i, line := range lines {
		if strings.Contains(line, label) {
			return i
		}
	}
	return -1
}

// valueAfter returns the first line from the label's own line (text after
// the label) or the next few lines that matches match. The label's own
// line is tried first so a value printed on the same line still works.
func valueAfter(lines []string, label string, lookahead int, match func(string) string) string {
	idx := indexOf(lines, label)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(lines[idx][strings.Index(lines[idx], label)+len(label):])
	rest = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
	if rest != "" {
		if v := match(rest); v != "" {
			return v
		}
	}
	for i := idx + 1; i < len(lines) && i <= idx+lookahead; i++ {
		if v := match(lines[i]); v != "" {
			return v
		}
	}
	return ""
}

// ParseOrderInfo reads the order header. ok is false when any field the
// order cannot be written without is missing.
func ParseOrderInfo(text string) (OrderInfo, bool) {
	lines := splitLines(text)
	var info OrderInfo

	info.PONumber = valueAfter(lines, "Mã đơn hàng", 3, func(s string) string {
		if poPattern.MatchString(s) {
			return strings.ReplaceAll(s, ".", "/")
		}
		return ""
	})
	info.EntryDate = valueAfter(lines, "Ngày đặt hàng", 3, func(s string) string {
		if m := datePattern.FindStringSubmatch(s); m != nil {
			return normalizeDate(m[1])
		}
		return ""
	})
	info.CancelDate = valueAfter(lines, "Ngày NCC giao hàng", 3, func(s string) string {
		if m := dateRangePattern.FindStringSubmatch(s); m != nil {
			return normalizeDate(m[2])
		}
		return ""
	})
	// The label itself wraps ("Kho NCC giao" / "hàng:"), so only its
	// first half is searched for.
	info.StoreName = valueAfter(lines, "Kho NCC giao", 3, func(s string) string {
		s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(s, "hàng")), ":"))
		if storeLinePattern.MatchString(s) {
			return s
		}
		return ""
	})
	if m := storeLinePattern.FindStringSubmatch(info.StoreName); m != nil {
		info.StoreCode = m[1]
	}
	// The label wraps too ("Tên chi nhánh" / "NCC:").
	info.BranchCode = valueAfter(lines, "Tên chi nhánh", 3, func(s string) string {
		s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(s, "NCC")), ":"))
		if m := storeLinePattern.FindStringSubmatch(s); m != nil {
			return m[1]
		}
		return ""
	})

	ok := info.PONumber != "" && info.EntryDate != "" && info.CancelDate != "" && info.StoreCode != "" && info.BranchCode != ""
	return info, ok
}

// normalizeDate zero-pads D/M/YYYY to DD/MM/YYYY.
func normalizeDate(date string) string {
	parts := strings.Split(date, "/")
	if len(parts) != 3 {
		return date
	}
	for i := 0; i < 2; i++ {
		if len(parts[i]) == 1 {
			parts[i] = "0" + parts[i]
		}
	}
	return strings.Join(parts, "/")
}

// tableBounds returns the line range holding the product rows: after the
// LAST "Thành tiền" header line (Python's text.rfind) and before the
// "Tổng" row. end is -1 when there is no "Tổng" row.
func tableBounds(lines []string) (start, end int) {
	start = 0
	for i, line := range lines {
		if strings.Contains(line, "Thành tiền") {
			start = i + 1
		}
	}
	end = -1
	for i := start; i < len(lines); i++ {
		if lines[i] == "Tổng" {
			end = i
			break
		}
	}
	return start, end
}

// ExtractProducts reads every product row, mirroring tach_san_pham_BHX:
// a row starts at a 1–3 digit row number followed by a 10–15 digit
// layout code, the barcode is the next cell, the quantity is the row's
// first two-decimal cell and the line total its last.
func ExtractProducts(text string) []Product {
	lines := splitLines(text)
	start, end := tableBounds(lines)
	if end < 0 {
		end = len(lines)
	}
	lines = lines[start:end]

	var starts []int
	for i := 0; i+1 < len(lines); i++ {
		if rowNumberPattern.MatchString(lines[i]) && layoutCodePattern.MatchString(lines[i+1]) {
			starts = append(starts, i)
		}
	}

	var products []Product
	for n, s := range starts {
		e := len(lines)
		if n+1 < len(starts) {
			e = starts[n+1]
		}
		block := lines[s:e]
		if len(block) < 3 || !barcodePattern.MatchString(block[2]) {
			continue
		}
		var numbers []string
		for _, cell := range block[3:] {
			if moneyPattern.MatchString(cell) {
				numbers = append(numbers, cell)
			}
		}
		if len(numbers) < 3 {
			continue
		}
		p := Product{Barcode: block[2], Qty: numbers[0], Total: numbers[len(numbers)-1]}
		if len(numbers) >= 4 {
			p.Price, p.DiscountPrice = numbers[1], numbers[2]
		} else {
			p.Price = numbers[1]
		}
		products = append(products, p)
	}
	return products
}

// ParseTotals reads the "Tổng" row: total quantity, then total amount.
func ParseTotals(text string) (Totals, bool) {
	lines := splitLines(text)
	_, end := tableBounds(lines)
	if end < 0 || end+2 >= len(lines) {
		return Totals{}, false
	}
	qty, amount := lines[end+1], lines[end+2]
	if !numberPattern.MatchString(qty) || !numberPattern.MatchString(amount) {
		return Totals{}, false
	}
	return Totals{Qty: qty, Amount: amount}, true
}
