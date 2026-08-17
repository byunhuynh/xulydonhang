package bigc

import (
	"regexp"
	"strings"
	"time"
)

// poEntryDatePattern mirrors trichxuatinfo_donbigc's PO-number/entry-date
// match (xulydonhang.py:5948): r"(\d{13,})\s+(\d{2}/\d{2}/\d{2})" against
// whitespace-collapsed text — group 1 is the PO number, group 2 is the
// entry date in DD/MM/YY form (not yet century-expanded).
var poEntryDatePattern = regexp.MustCompile(`(\d{13,})\s+(\d{2}/\d{2}/\d{2})`)

var whitespaceCollapsePattern = regexp.MustCompile(`\s+`)
var totalNetPurchasePricePattern = regexp.MustCompile(`Total Net Purchase Price`)
var twoDigitDatePattern = regexp.MustCompile(`\d{2}/\d{2}/\d{2}`)
var twoDigitDMYPattern = regexp.MustCompile(`^(\d{2})/(\d{2})/(\d{2})$`)

// ParseOrderInfo mirrors trichxuatinfo_donbigc (xulydonhang.py:5941-5973)
// in full. Extracts the PO number and entry date from the first
// "<13+ digit number> <DD/MM/YY>" match in the whitespace-collapsed
// page-0 text (xulydonhang.py:5945,5948), then the cancel date from the
// first DD/MM/YY-shaped date found after the LAST occurrence of "Total
// Net Purchase Price" (:5952-5960) — falling back to entryDate + 5 days
// (:5964-5971) if no such date is found. Both dates are returned already
// century-expanded DD/MM/YY -> DD/MM/20YY (mirrors convert_entry_date,
// :5333-5340, applied to both entry_date and cancel_date at the end of
// the Python function).
//
// ok=false only when no PO/entry-date match is found at all, or the
// matched entry-date text isn't a real calendar date. Python's
// equivalent failure mode (entry_date stays None, or convert_entry_date
// receives an unparseable string) crashes with an unhandled TypeError/
// ValueError deep in the call chain — Go returns a clean failure instead
// (Phase 2b's "correct main flow" policy, same principle Satra's
// ParsePONumber/ParseEntryDate already established). If the cancel-date
// portion can't be resolved (neither the region-scan nor the +5-day
// fallback produces a parseable date — only possible if the matched
// entry-date digits aren't a real calendar date, e.g. "99/99/99"),
// cancelDate is returned as "" rather than failing the whole parse —
// best-effort, matching how ParseCancelDate works for other vendors in
// this codebase.
func ParseOrderInfo(text string) (poNumber, entryDate, cancelDate string, ok bool) {
	cleaned := strings.TrimSpace(whitespaceCollapsePattern.ReplaceAllString(text, " "))

	m := poEntryDatePattern.FindStringSubmatch(cleaned)
	if m == nil {
		return "", "", "", false
	}
	poNumber = m[1]
	rawEntryDate := m[2] // DD/MM/YY, not yet century-expanded

	entryDateConverted, entryOk := convertEntryDate(rawEntryDate)
	if !entryOk {
		return "", "", "", false
	}

	rawCancelDate := ""
	if matches := totalNetPurchasePricePattern.FindAllStringIndex(cleaned, -1); len(matches) > 0 {
		last := matches[len(matches)-1]
		region := cleaned[last[1]:]
		rawCancelDate = twoDigitDatePattern.FindString(region)
	}
	if rawCancelDate == "" {
		if t, err := time.Parse("02/01/06", rawEntryDate); err == nil {
			rawCancelDate = t.AddDate(0, 0, 5).Format("02/01/06")
		}
	}
	if rawCancelDate != "" {
		cancelDate, _ = convertEntryDate(rawCancelDate) // best-effort
	}

	return poNumber, entryDateConverted, cancelDate, true
}

// convertEntryDate mirrors convert_entry_date (xulydonhang.py:5333-5340):
// DD/MM/YY -> DD/MM/20YY. Always assumes the 2000s (Python's literal
// f"20{year}", not a general pivot-year rule) — deliberately not
// "future-proofed" beyond what the Python source actually does.
func convertEntryDate(raw string) (string, bool) {
	m := twoDigitDMYPattern.FindStringSubmatch(raw)
	if m == nil {
		return "", false
	}
	return m[1] + "/" + m[2] + "/20" + m[3], true
}
