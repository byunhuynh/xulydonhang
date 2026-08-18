package emart

import (
	"regexp"
	"strings"
)

var (
	// poNumberPattern mirrors xulydonhang.py:9316 exactly:
	// r"PO No\.\s*\n\s*:? ?([^\n]+)". NOTE: Go's regexp \s is ASCII-only
	// where Python's is Unicode-aware (matches U+00A0 non-breaking
	// space too) — if a real Emart PDF's marker line has NBSP padding,
	// this may need the same explicit NBSP-normalization treatment
	// already applied for Satra/BigC (see those packages' extract.go for
	// the precedent). Verify against real fixtures in Task 5/6; not
	// pre-emptively "fixed" here without evidence it's needed.
	poNumberPattern = regexp.MustCompile(`PO No\.\s*\n\s*:? ?([^\n]+)`)
	// entryDatePattern mirrors xulydonhang.py:9322.
	entryDatePattern = regexp.MustCompile(`Order By / Date\s*\n\s*:? ?([^\n]+)`)
	// cancelDatePattern mirrors xulydonhang.py:9327.
	cancelDatePattern = regexp.MustCompile(`Delivery Date\s*\n\s*:? ?([^\n]+)`)
	// storeNamePattern mirrors xulydonhang.py:9333: r"^Delivery to :\s*(.+)"
	// with re.MULTILINE -> Go's (?m) flag.
	storeNamePattern = regexp.MustCompile(`(?m)^Delivery to :\s*(.+)`)
)

// ParseOrderInfo mirrors the PO-number/date/store-name extraction inline
// in process_file's Emart branch (xulydonhang.py:9314-9338) — Python
// doesn't factor this into its own named function (unlike Winmart's
// ParseOrderInfo), but this port still gathers it into one function per
// this codebase's established per-vendor package convention.
//
// ok=false only when the PO-number OR either date marker isn't found —
// Python's real code would carry a None value into several downstream
// string operations in that case (e.g. STT_donhang_str = f"-{po_number}"
// silently becomes the literal text "-None"), which this port treats as
// a clean failure instead, per this codebase's established policy. A
// missing store name, by contrast, is genuinely tolerated by Python
// itself (order_file still proceeds, just flags a warning status) — see
// storeName's own handling below, which does NOT gate ok.
func ParseOrderInfo(text string) (poNumber, entryDate, cancelDate, storeName string, ok bool) {
	poMatch := poNumberPattern.FindStringSubmatch(text)
	if poMatch == nil {
		return "", "", "", "", false
	}
	poNumber = strings.TrimSpace(poMatch[1])

	entryMatch := entryDatePattern.FindStringSubmatch(text)
	if entryMatch == nil {
		return "", "", "", "", false
	}
	entryDate = formatEmartDate(strings.TrimSpace(entryMatch[1]))

	cancelMatch := cancelDatePattern.FindStringSubmatch(text)
	if cancelMatch == nil {
		return "", "", "", "", false
	}
	cancelDate = formatEmartDate(strings.TrimSpace(cancelMatch[1]))

	if storeMatch := storeNamePattern.FindStringSubmatch(text); storeMatch != nil {
		storeName = strings.Split(storeMatch[1], "   ")[0]
	}

	return poNumber, entryDate, cancelDate, storeName, true
}

// formatEmartDate mirrors "entry_date[:10].replace(".", "/")"
// (xulydonhang.py:9325, same shape at :9330 for cancel_date): truncate
// to the first 10 characters (Python's [:10] slice, tolerant of shorter
// strings), THEN replace "." with "/". Byte-based Go slicing matches
// Python's character-based slicing here because these date strings are
// always plain ASCII digits and dots (never Vietnamese diacritics).
func formatEmartDate(s string) string {
	if len(s) > 10 {
		s = s[:10]
	}
	return strings.ReplaceAll(s, ".", "/")
}
