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
