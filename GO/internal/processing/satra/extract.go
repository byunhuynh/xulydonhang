package satra

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var poNumberPattern = regexp.MustCompile(`\*(P-[^*]+)\*`)

// ParsePONumber mirrors the PO-number extraction at the top of
// process_file's Satra branch (xulydonhang.py:9309-9310): the PO number
// is whatever sits between two literal "*" characters, prefixed "P-".
// Python captures the WHOLE "*...*" match then strips the first/last
// character; this uses a capture group directly instead, equivalent.
func ParsePONumber(text string) (string, bool) {
	m := poNumberPattern.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	return m[1], true
}

var entryDateBlockPattern = regexp.MustCompile(`(?s)(.*?)\nNgày đặt hàng:`)
var printDateBlockPattern = regexp.MustCompile(`(?s)(.*?)\nNgày in:`)

const placeholderDate = "01/01/0001"

// ParseEntryDate mirrors xulydonhang.py:9326-9336: takes everything
// before the first "Ngày đặt hàng:" marker, uses its LAST line as the
// raw date, parses "MM/DD/YYYY" and reformats "DD/MM/YYYY". If the RAW
// extracted date string is the literal placeholder "01/01/0001" (the PDF
// template renders this when the entry-date field itself is unset in the
// source system — not a parse failure), retries the same shape against
// "Ngày in:" instead. Returns false if the "Ngày đặt hàng:" marker isn't
// found at all, or if a fallback is triggered but "Ngày in:" isn't found
// either, or if the date ultimately used doesn't parse as "MM/DD/YYYY".
//
// The fallback only triggers when the "Ngày đặt hàng:" marker WAS found
// and its raw value is exactly the placeholder — mirroring Python's
// control flow, where the "Ngày in:" retry sits inside the `if
// entry_date:` block and is never reached when the first marker is
// simply absent from the text.
//
// Deliberately checks the RAW pre-format string against the placeholder,
// not the formatted "DD/MM/YYYY" output: comparing formatted strings
// requires re-deriving the formatted placeholder via a second format
// call, which invites exactly the kind of stray literal comparison this
// implementation avoids.
func ParseEntryDate(text string) (string, bool) {
	raw, ok := rawDateBeforeMarker(text, entryDateBlockPattern)
	if !ok {
		return "", false
	}
	if raw == placeholderDate {
		raw, ok = rawDateBeforeMarker(text, printDateBlockPattern)
		if !ok {
			return "", false
		}
	}
	return formatMDYtoDMYChecked(raw)
}

// rawDateBeforeMarker returns the last line of the text preceding
// blockPattern's marker, trimmed, without parsing or formatting it.
func rawDateBeforeMarker(text string, blockPattern *regexp.Regexp) (string, bool) {
	m := blockPattern.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	lines := strings.Split(m[1], "\n")
	return strings.TrimSpace(lines[len(lines)-1]), true
}

// formatMDYtoDMYChecked parses raw as M/D/YYYY and reformats DD/MM/YYYY.
// The reference layout "1/2/2006" (not "01/02/2006") is deliberate:
// real Satra PDFs render single-digit months/days with no leading zero
// (e.g. "8/3/2026"), and Python's datetime.strptime(raw, "%m/%d/%Y") —
// the function this mirrors (xulydonhang.py:9329/9336/9346) — accepts
// both zero-padded and non-padded numeric fields. Go's time.Parse with
// a zero-padded reference field ("01"/"02") requires exactly two digits
// and rejects "8/3/2026" outright, which is not equivalent to Python's
// leniency here; "1/2/2006" accepts both widths in Go, matching
// strptime's actual behavior.
func formatMDYtoDMYChecked(raw string) (string, bool) {
	t, err := time.Parse("1/2/2006", raw)
	if err != nil {
		return "", false
	}
	return t.Format("02/01/2006"), true
}

var cancelDateBlockPattern = regexp.MustCompile(`(?s)Ngày giao hàng:\s*(.*?)\s*Địa chỉ giao hàng:`)
var cancelDateLinePattern = regexp.MustCompile(`\d{1,2}/\d{1,2}/\d{4}`)

// ParseCancelDate mirrors xulydonhang.py:9339-9347: within the block
// between "Ngày giao hàng:" and "Địa chỉ giao hàng:", finds the FIRST
// line containing a d/d/dddd-shaped date, parses "MM/DD/YYYY", reformats
// "DD/MM/YYYY". Returns false if the block or no date-shaped line within
// it is found.
func ParseCancelDate(text string) (string, bool) {
	m := cancelDateBlockPattern.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	for _, line := range strings.Split(m[1], "\n") {
		if cancelDateLinePattern.MatchString(line) {
			return formatMDYtoDMYChecked(strings.TrimSpace(line))
		}
	}
	return "", false
}

var shipToAddressPattern = regexp.MustCompile(`Địa chỉ giao hàng:\s*((?:.*\n)+?)Địa chỉ thanh toán:`)

// ParseShipToAddress mirrors xulydonhang.py:9312-9314: the block of
// lines between "Địa chỉ giao hàng:" and "Địa chỉ thanh toán:", joined
// into one line (newlines replaced with a single space), with any
// double-space collapsed to one. The regex has NO DOTALL flag (no (?s)),
// matching Python's re.search with no flags argument at xulydonhang.py:9311;
// this ensures . does NOT match newline, so the lazy repetition (?:.*\n)+?
// stops at the FIRST occurrence of the terminator, not a later one.
func ParseShipToAddress(text string) (string, bool) {
	m := shipToAddressPattern.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	joined := strings.TrimSpace(strings.ReplaceAll(m[1], "\n", " "))
	return strings.ReplaceAll(joined, "  ", " "), true
}

// Product is one product line extracted from a Satra order's product
// table.
type Product struct {
	Barcode    string
	Qty        float64
	TotalPrice float64
}

var totalCutoffPattern = regexp.MustCompile(`\bTổng cộng\b`)
var productBlockStartPattern = regexp.MustCompile(`(?m)^\s*\d+\s+\d+\s*\n\s*(\d{13})`)
var quantityLinePattern = regexp.MustCompile(`\b(\d{1,3}),000\b`)
var trailingZeroCentsPattern = regexp.MustCompile(`,00$`)

// ExtractProducts mirrors trichxuatsanpham_satra (xulydonhang.py:6492-6529):
// cuts the text at the first "Tổng cộng", finds every position where a
// line matching "STT count" is immediately followed by a 13-digit
// barcode line, and treats each such position as the start of one
// product's block (ending where the next one starts, or at the cutoff).
// Within each block, spaces are stripped from every line; the first line
// matching "N,000" (1-3 digits before the literal ",000") is the
// quantity (with ",000" replaced by just the digits), and the line right
// after it is the total price (with a trailing ",00" stripped). A block
// with no quantity-shaped line, or whose price fails to parse as a
// non-zero number, is skipped entirely.
func ExtractProducts(text string) []Product {
	cut := totalCutoffPattern.Split(text, 2)[0]
	cut = strings.TrimSpace(cut)

	matches := productBlockStartPattern.FindAllStringSubmatchIndex(cut, -1)
	if matches == nil {
		return nil
	}

	type position struct {
		start   int
		barcode string
	}
	positions := make([]position, 0, len(matches)+1)
	for _, m := range matches {
		positions = append(positions, position{start: m[0], barcode: cut[m[2]:m[3]]})
	}
	positions = append(positions, position{start: len(cut), barcode: ""})

	var products []Product
	for i := 0; i < len(positions)-1; i++ {
		start, barcode := positions[i].start, positions[i].barcode
		end := positions[i+1].start
		block := strings.TrimSpace(cut[start:end])

		var lines []string
		for _, line := range strings.Split(block, "\n") {
			line = strings.ReplaceAll(line, " ", "")
			if line != "" {
				lines = append(lines, line)
			}
		}

		qtyIndex := -1
		for i, line := range lines {
			if quantityLinePattern.MatchString(line) {
				qtyIndex = i
				break
			}
		}
		if qtyIndex == -1 || qtyIndex+1 >= len(lines) {
			continue
		}

		qtyStr := quantityLinePattern.FindStringSubmatch(lines[qtyIndex])[1]
		qty, err := strconv.ParseFloat(qtyStr, 64)
		if err != nil {
			continue
		}

		priceStr := trailingZeroCentsPattern.ReplaceAllString(lines[qtyIndex+1], "")
		price, err := strconv.ParseFloat(strings.ReplaceAll(priceStr, ",", ""), 64)
		if err != nil || price == 0 {
			continue
		}

		products = append(products, Product{Barcode: barcode, Qty: qty, TotalPrice: price})
	}
	return products
}
