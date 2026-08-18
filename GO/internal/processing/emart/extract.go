package emart

import (
	"regexp"
	"strconv"
	"strings"
)

// valueAfterLabel finds the line containing label, then returns its
// value. It checks THREE layouts, in order: (1) same-line — label,
// optional colon, and value all on the label's own line (Python's real
// storeName regex, `^Delivery to :\s*(.+)`, assumes exactly this shape,
// and Task 2's original tests exercise it); (2)/(3) label alone on its
// own line, then the first of the following 2 lines that isn't just a
// colon on its own (with optional surrounding whitespace) — covering
// BOTH the 2-line layout Python's PO-No./date patterns assume (colon and
// value together, right after the label) AND a confirmed, real 3-line
// layout this repo's actual Go PDF text extraction produces for these
// specific PDFs (label, then a lone ":" line, then the value) — see
// ParseOrderInfo's own doc comment for the concrete evidence. In every
// case a leading ":" on the candidate value is stripped, then the result
// is trimmed. Returns ("", false) if the label isn't found, or if none
// of the same-line remainder or the next 2 lines after it is a real
// (non-colon-only) value.
func valueAfterLabel(lines []string, label string) (string, bool) {
	for i, line := range lines {
		idx := strings.Index(line, label)
		if idx == -1 {
			continue
		}

		if rest := strings.TrimSpace(line[idx+len(label):]); rest != "" {
			rest = strings.TrimPrefix(rest, ":")
			if rest = strings.TrimSpace(rest); rest != "" {
				return rest, true
			}
		}

		for j := i + 1; j < len(lines) && j <= i+2; j++ {
			candidate := strings.TrimSpace(lines[j])
			if candidate == ":" || candidate == "" {
				continue
			}
			candidate = strings.TrimPrefix(candidate, ":")
			return strings.TrimSpace(candidate), true
		}
		return "", false
	}
	return "", false
}

// ParseOrderInfo mirrors the PO-number/date/store-name extraction inline
// in process_file's Emart branch (xulydonhang.py:9314-9338). Python's
// own regexes assume the label, an optional colon, and the value fit on
// at most 2 lines (colon-and-value together, right after the label) —
// confirmed, by direct extraction of 7 of the 17 real Emart PDFs through
// this repo's actual PDF pipeline (extractPageTexts), that this repo's
// Go PDF library instead produces a 3-line layout for these specific
// real files: label, then a lone ":" on its own line, then the value
// (e.g. "PO No.\n:\n4501866956\n", not Python's assumed
// "PO No.\n: 4501866956\n"). valueAfterLabel handles both shapes
// uniformly by scanning lines directly instead of relying on a
// newline-count-sensitive regex — this is a genuine cross-library
// text-layout difference (the PDF's real content, not a transcription
// bug), the same general class of issue already fixed once for Winmart
// elsewhere in this codebase.
//
// ok=false only when the PO-number OR either date marker fails to
// resolve to a real value — Python's real code would carry a None value
// into several downstream string operations in that case (e.g.
// STT_donhang_str = f"-{po_number}" silently becomes the literal text
// "-None"), which this port treats as a clean failure instead, per this
// codebase's established policy. A missing store name, by contrast, is
// genuinely tolerated by Python itself (order processing still
// proceeds, just flags a warning status) — see storeName's own handling
// below, which does NOT gate ok.
func ParseOrderInfo(text string) (poNumber, entryDate, cancelDate, storeName string, ok bool) {
	lines := strings.Split(text, "\n")

	poNumber, poOk := valueAfterLabel(lines, "PO No.")
	if !poOk {
		return "", "", "", "", false
	}

	entryValue, entryOk := valueAfterLabel(lines, "Order By / Date")
	if !entryOk {
		return "", "", "", "", false
	}
	entryDate = formatEmartDate(entryValue)

	cancelValue, cancelOk := valueAfterLabel(lines, "Delivery Date")
	if !cancelOk {
		return "", "", "", "", false
	}
	cancelDate = formatEmartDate(cancelValue)

	if storeValue, storeOk := valueAfterLabel(lines, "Delivery to"); storeOk {
		storeName = strings.Split(storeValue, "   ")[0]
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

// productTablePattern isolates the product-table region of the page text
// before the product-line regex runs, mirroring xulydonhang.py:9339-9340
// exactly:
//
//	text = re.search(r"Article Code\s*(.*?)\s*Total Amount\(without VAT\) :", text, re.DOTALL)
//	text = text.group(1).strip()
//
// re.DOTALL makes "." match newlines too — mirrored with Go's (?s) flag.
var productTablePattern = regexp.MustCompile(`(?s)Article Code\s*(.*?)\s*Total Amount\(without VAT\) :`)

// productLinePattern mirrors laydanhsanpham_emart's compiled regex
// (xulydonhang.py:6616-6624) exactly: 7 fields — a 7-digit article code,
// a 12-13 digit barcode, a non-greedy description, a >=2-letter unit
// code, a "Qty. in Box" integer, a "PO Qty." integer, and a purchase-
// price field (dots as thousands separators, comma as a possible
// decimal separator). Go has no re.VERBOSE mode; the pattern below is
// the same shape with the VERBOSE-only whitespace/comments removed. The
// original also uses re.DOTALL, mirrored with (?s) so the non-greedy
// description group can span newlines exactly as Python's does.
//
// Capture groups (1-based, matching Python's declaration order):
// 1=article_code (discarded), 2=barcode, 3=description (discarded),
// 4=unit (discarded), 5=qty_in_box (discarded — NOT what "OU Qty" uses),
// 6=quantity (this is "OU Qty" — match.group("quantity"), the PO
// Qty. column), 7=purchase_price (the per-unit "Pur. Price(-VAT)").
var productLinePattern = regexp.MustCompile(`(?s)(\d{7})\s*(\d{12,13})\s*\s*(.+?)\s+([A-Z]{2,})\s+\s*(\d+)\s+\s*(\d+)\s+\s*([\d.,]+)`)

// Product is one extracted Emart product line. UnitPrice is a
// dot-stripped numeric string (Emart's PDF table uses "." as a thousands
// separator, e.g. "26.950" -> "26950") holding the PER-UNIT purchase
// price (the "Pur. Price(-VAT)" column) — NOT a line total, despite
// Python's own dict key for this field being "Total Price"
// (laydanhsanpham_emart, xulydonhang.py:6635-6639). write_to_dondathang_emart
// uses this value directly as giahoadon with NO division by quantity
// (xulydonhang.py:5095) — a real, easy-to-miss difference from Winmart,
// whose same-named field genuinely is a line total and must be divided.
type Product struct {
	Barcode   string
	OUQty     int
	UnitPrice string
}

// ExtractProducts mirrors laydanhsanpham_emart (xulydonhang.py:6614-6644)
// plus the table-isolation step that always runs immediately before it
// in process_file's Emart branch (xulydonhang.py:9339-9340). If the
// "Article Code...Total Amount(without VAT) :" isolation doesn't match
// at all, Python's real code would crash (calling .group(1) on None);
// this returns nil instead, per this codebase's established
// clean-failure policy.
//
// purchase_price_value == 0 items are dropped entirely during extraction
// (xulydonhang.py:6627-6628, "continue") — unlike Winmart, there is no
// "mark the previous row's AO/AP" side effect for a zero-price Emart
// item here; it simply never appears in the returned slice.
func ExtractProducts(text string) []Product {
	tableMatch := productTablePattern.FindStringSubmatch(text)
	if tableMatch == nil {
		return nil
	}
	tableText := strings.TrimSpace(tableMatch[1])

	var products []Product
	for _, m := range productLinePattern.FindAllStringSubmatch(tableText, -1) {
		// purchase_price = match.group("purchase_price").replace(".", "")
		unitPrice := strings.ReplaceAll(m[7], ".", "")
		// purchase_price_value = float(purchase_price.replace(",", "."))
		// A malformed price (ParseFloat error) is NOT treated as zero —
		// it falls through and the item is kept, so a genuinely
		// unexpected price format surfaces as a visible price-mismatch
		// row downstream rather than silently vanishing.
		if value, err := strconv.ParseFloat(strings.ReplaceAll(unitPrice, ",", "."), 64); err == nil && value == 0 {
			continue
		}
		qty, err := strconv.Atoi(m[6])
		if err != nil {
			continue
		}
		products = append(products, Product{
			Barcode:   m[2],
			OUQty:     qty,
			UnitPrice: unitPrice,
		})
	}
	return products
}
