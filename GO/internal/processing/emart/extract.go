package emart

import (
	"regexp"
	"strconv"
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
