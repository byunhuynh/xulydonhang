package winmart

import (
	"regexp"
	"strings"
)

// ParseOrderInfo mirrors the PO-number/date/note extraction inline in
// process_file's Winmart branch (xulydonhang.py:8989-9004ish — line-scan
// logic, not a regex function). Each of "Ngày đặt hàng (PO date)", "Số
// đơn hàng (PO No.)", and "Ngày giao (Delivery Date)" is a marker line;
// the value is the LINE IMMEDIATELY AFTER it. Dates are returned with
// "." replaced by "/" (Python's raw `.replace('.', '/')` — no reordering
// of month/day/year components, a literal character substitution only).
// ok=false only when the PO-number marker/line isn't found — mirrors
// Python's real crash-on-None behavior (a missing PO number makes
// several downstream string operations fail) with a clean failure
// instead, per this codebase's established error-handling policy.
//
// note mirrors `ghichu` (xulydonhang.py:8994-9000): the text between the
// literal "Ghi chú" marker and the literal supplier-ID string "Nhà cung
// cấp (Supplier): 0002011398", with the LAST line of that block dropped
// (Python's `.splitlines()[:-1]`) and the rest joined with a single
// space (Python's `.replace('\n', ' ')` after a `"\n".join(...)` —
// equivalent to joining with spaces directly). Returns "" (not a failure)
// if the "Ghi chú"/supplier-ID markers aren't found — Python's real code
// would raise an IndexError in that case (`text.split("Ghi chú")[1]` on
// a list of length 1); a missing note is not itself fatal in Go's port.
func ParseOrderInfo(text string) (poNumber, entryDate, cancelDate, note string, ok bool) {
	lines := strings.Split(text, "\n")

	entryDate, entryOk := lineAfterMarker(lines, "Ngày đặt hàng (PO date)")
	if entryOk {
		entryDate = strings.ReplaceAll(entryDate, ".", "/")
	}

	poNumber, poOk := lineAfterMarker(lines, "Số đơn hàng (PO No.)")
	if !poOk {
		return "", "", "", "", false
	}

	cancelDate, cancelOk := lineAfterMarker(lines, "Ngày giao (Delivery Date)")
	if cancelOk {
		cancelDate = strings.ReplaceAll(cancelDate, ".", "/")
	}

	note = parseNote(text)

	return poNumber, entryDate, cancelDate, note, true
}

// lineAfterMarker mirrors the repeated
// "idx = next(...); value = lines[idx+1].strip() if idx != -1 ... else None"
// pattern used for entry_date/po_number/cancel_date (xulydonhang.py:8989-9008).
func lineAfterMarker(lines []string, marker string) (string, bool) {
	for i, line := range lines {
		if strings.Contains(line, marker) {
			if i+1 < len(lines) {
				return strings.TrimSpace(lines[i+1]), true
			}
			return "", false
		}
	}
	return "", false
}

const supplierIDMarker = "Nhà cung cấp (Supplier): 0002011398"

// parseNote mirrors xulydonhang.py:8994-9000 exactly.
func parseNote(text string) string {
	parts := strings.SplitN(text, "Ghi chú", 2)
	if len(parts) != 2 {
		return ""
	}
	block := strings.SplitN(parts[1], supplierIDMarker, 2)[0]
	block = strings.TrimSpace(block)
	if block == "" {
		return ""
	}
	// Split on newlines, drop the LAST line (Python's `.splitlines()[:-1]`),
	// then join the rest with spaces. If only 0 or 1 line remains after
	// trimming, dropping the last line yields an empty slice and this
	// correctly returns "" — matching Python's real behavior, not a bug.
	lines := strings.Split(block, "\n")
	if len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, " ")
}

const (
	deliveryAddressMarker = "Địa chỉ giao hàng (Delivery Address)"
	deliveryAddressStop   = "Thông tin đơn hàng (Information)"
)

// ParseDeliveryAddress mirrors xulydonhang.py:9013-9041 (the
// diachigiaohang block, written to Excel column E — NOT the same as
// ParseFuzzyMatchAddress below, a genuinely separate scan over the same
// page text). The line immediately after the marker is a warehouse code
// ("ma_kho"); subsequent lines up to (not including) the stop marker are
// joined with " ", skipping any line containing the literal substring
// "WM+" (a duplicate-line artifact in the real PDF template,
// xulydonhang.py:9031-9033's comment gives the example
// "6863 - WM+ HCM 60 Liên khu 10-11"). Final result is
// "<ma_kho> - <joined address lines>". Returns ("", false) if the marker
// isn't found — mirrors Python's diachigiaohang staying None.
func ParseDeliveryAddress(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	idx := -1
	for i, line := range lines {
		if strings.Contains(line, deliveryAddressMarker) {
			idx = i
			break
		}
	}
	if idx == -1 || idx+1 >= len(lines) {
		return "", false
	}
	maKho := strings.TrimSpace(lines[idx+1])

	var addressLines []string
	for _, line := range lines[idx+2:] {
		if strings.Contains(line, deliveryAddressStop) {
			break
		}
		line = strings.TrimSpace(line)
		if strings.Contains(line, "WM+") {
			continue
		}
		if line != "" {
			addressLines = append(addressLines, line)
		}
	}
	return maKho + " - " + strings.Join(addressLines, " "), true
}

// ParseFuzzyMatchAddress mirrors xulydonhang.py:9062-9087 — a
// SEPARATE scan from ParseDeliveryAddress over the same page text,
// producing the address string used ONLY as fuzzy-match input to
// productdata.Store.GetCustomerCodeByFuzzyAddress("WINMART", ...) — this
// value is never written to any Excel column directly. Anchors on
// either two consecutive lines where the first contains "tổng hợp"
// (case-insensitive) and the second contains "wincommerce", OR a single
// line containing "wincommerce" alone — whichever is found first
// scanning top to bottom. From the line after the anchor, collects
// lines until (not including) the first line containing "mst" or
// "địa chỉ giao hàng" (both checked case-insensitively, matching
// Python's `.lower()` comparisons), joined with " ". Returns ("", false)
// if no anchor is found — mirrors Python's diachi staying None.
func ParseFuzzyMatchAddress(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	idx := -1
	for i := 0; i < len(lines)-1; i++ {
		lineLower := strings.ToLower(lines[i])
		nextLower := strings.ToLower(lines[i+1])
		if strings.Contains(lineLower, "tổng hợp") && strings.Contains(nextLower, "wincommerce") {
			idx = i // Anchor on the "tổng hợp" line itself (Python: idx = i,
			// "Lấy dòng đầu tiên làm mốc" = "take the first line as anchor").
			// Collection starts at idx+1, i.e. the WINCOMMERCE line, which
			// IS included in the collected block.
			break
		}
		if strings.Contains(lineLower, "wincommerce") {
			idx = i
			break
		}
	}
	if idx == -1 {
		return "", false
	}

	var collected []string
	for _, line := range lines[idx+1:] {
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, "mst") || strings.Contains(lineLower, "địa chỉ giao hàng") {
			break
		}
		collected = append(collected, strings.TrimSpace(line))
	}
	return strings.Join(collected, " "), true
}

// Product is one row of Winmart's product table. Only the 3 fields
// trichxuatsanpham_winmart actually returns (xulydonhang.py:6794-6798) —
// "no" (STT), "article", and "unit_price" are matched by the regex but
// deliberately discarded, matching Python's returned dict shape exactly
// (no per-unit-price field survives extraction; Task 4 derives a unit
// price from TotalPrice/OUQty for Winmart specifically, per the
// giahoadon formula at xulydonhang.py:4347-4351).
type Product struct {
	Barcode    string
	OUQty      string
	TotalPrice string
}

var horizontalWhitespaceRunPattern = regexp.MustCompile(`[ \t]+`)

// productLinePattern mirrors trichxuatsanpham_winmart's re.VERBOSE
// pattern (xulydonhang.py:6779-6789) exactly: 7 fields, each on its own
// line — STT, article code, barcode, quantity, a 2-4 character unit code
// (uppercase letters/digits), unit price, and amount. Go's regexp
// package has no re.VERBOSE mode (which lets Python's pattern span
// multiple source lines with embedded whitespace/comments ignored) — the
// pattern below is the same shape with the VERBOSE-only whitespace
// removed, with no other change: Python's pattern has no "^" anchor and
// is not compiled with re.MULTILINE, so a match may begin anywhere
// (including mid-line) that the 7-field shape occurs — this pattern
// intentionally does not add a "^"/(?m) anchor either, to stay a true,
// unnarrowed mirror.
var productLinePattern = regexp.MustCompile(`(\d+)\s*\n(\d+)\s*\n(\d+)\s*\n([\d,]+)\s*\n[A-Z0-9]{2,4}\s*\n([\d,]+)\s*\n([\d,]+)`)

// ExtractProducts mirrors trichxuatsanpham_winmart (xulydonhang.py:6774-6805):
// collapses runs of spaces/tabs to a single space (re.sub(r"[ \t]+", " ", text),
// NOT touching newlines), then extracts every matching 7-field block.
// Group 1 (STT/"no"), group 2 ("article"), and group 5 ("unit_price")
// are matched but discarded — only Barcode (group 3), OUQty (group 4,
// commas stripped), and TotalPrice (group 6, commas stripped) survive
// into the returned Product, exactly matching Python's returned dict
// keys ("Barcode", "OU Qty", "Total Price").
func ExtractProducts(text string) []Product {
	collapsed := horizontalWhitespaceRunPattern.ReplaceAllString(text, " ")

	var products []Product
	for _, m := range productLinePattern.FindAllStringSubmatch(collapsed, -1) {
		products = append(products, Product{
			Barcode:    m[3],
			OUQty:      strings.ReplaceAll(m[4], ",", ""),
			TotalPrice: strings.ReplaceAll(m[6], ",", ""),
		})
	}
	return products
}
