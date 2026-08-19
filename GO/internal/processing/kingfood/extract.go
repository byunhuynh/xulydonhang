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
