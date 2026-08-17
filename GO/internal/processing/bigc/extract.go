package bigc

import (
	"regexp"
	"strconv"
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
// IMPORTANT: Go's RE2 engine treats \s as ASCII-only whitespace (space, tab,
// newline, etc.), while Python's re module treats \s as Unicode-aware and
// matches non-breaking space (U+00A0 / \xa0). This function normalizes
// U+00A0 to regular space before regex processing to match Python's implicit
// behavior. This is a confirmed real artifact in PDF-extracted text from
// this codebase — see xulydonhang.py's demsodonhang1trang_coop function,
// which explicitly does text.replace("\xa0", " ") before further processing.
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
	// Normalize non-breaking space (U+00A0 / \xa0) to regular space before regex processing
	// to match Python's re.sub behavior, which treats \s as Unicode-aware.
	text = strings.ReplaceAll(text, string(rune(0x00A0)), " ")
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

// Product is one row of BigC's page-0 master price/product list.
type Product struct {
	Barcode        string
	SKUOrUnit      string
	OrderedUnitQty string
	// UnitPrice is Python's "Total Price" dict key (laydanhsachsanpham_bigc,
	// xulydonhang.py:5869) — despite the name, it holds the PER-UNIT net
	// purchase price, not a line total; renamed here for clarity, not
	// literal fidelity to the (misleading) Python key name.
	UnitPrice float64
	// TotalNetPurchasePrice is captured for fidelity but never read
	// downstream anywhere in xulydonhang.py either — kept for
	// completeness/debugging only.
	TotalNetPurchasePrice float64
}

var articleHeaderPattern = regexp.MustCompile(`\bArticle\b`)
var priceListLinePattern = regexp.MustCompile(`(?s)(\d{13})\s+.+?\s+Pack\s+\d+\s+(\d+)\s+(\d+)\s+\d+\s+([\d,]+)\s+\w+\s+([\d,]+)`)

// ExtractPriceList mirrors laydanhsachsanpham_bigc (xulydonhang.py:5831-5873):
// slices page-0 text to everything from the first "Article" header word
// onward (xulydonhang.py:5837-5843), then extracts every matching
// product line from that slice. A line whose 4th/5th numeric field fails
// to parse (after stripping "," separators) is silently skipped —
// mirrors Python's `continue` on a malformed match; Go's regex only ever
// produces exactly 5 capture groups per match (unlike Python's
// len(match) != 5 check, which is checking tuple arity from findall,
// not field validity), so the equivalent Go failure mode is a
// strconv.ParseFloat error on group 4 or 5.
//
// IMPORTANT: Go's RE2 engine treats \s as ASCII-only whitespace, while
// Python's re module treats \s as Unicode-aware and matches non-breaking
// space (U+00A0 / \xa0). laydanhsachsanpham_bigc's own pattern
// (xulydonhang.py:5846-5849) never strips \xa0 from its input either —
// it just relies on Python's Unicode-aware \s to silently swallow it.
// This is a confirmed real artifact in this project's PDF-extracted text
// (see xulydonhang.py's demsodonhang1trang_coop, which explicitly does
// text.replace("\xa0", " ") before further processing) and this
// function's \s+-heavy pattern is exposed to the exact same risk
// ParseOrderInfo's pattern was (see its doc comment), so this function
// normalizes U+00A0 to a regular space before matching too, for the same
// reason. This normalization is local to this function — Go strings are
// immutable, so ParseOrderInfo's own normalization does not carry over.
func ExtractPriceList(pageZeroText string) []Product {
	pageZeroText = strings.ReplaceAll(pageZeroText, string(rune(0x00A0)), " ")

	loc := articleHeaderPattern.FindStringIndex(pageZeroText)
	if loc == nil {
		return nil
	}
	text := strings.TrimSpace(pageZeroText[loc[0]:])

	var products []Product
	for _, m := range priceListLinePattern.FindAllStringSubmatch(text, -1) {
		unitPrice, err1 := strconv.ParseFloat(strings.ReplaceAll(m[4], ",", ""), 64)
		totalPrice, err2 := strconv.ParseFloat(strings.ReplaceAll(m[5], ",", ""), 64)
		if err1 != nil || err2 != nil {
			continue
		}
		products = append(products, Product{
			Barcode: strings.TrimSpace(m[1]), SKUOrUnit: strings.TrimSpace(m[2]), OrderedUnitQty: strings.TrimSpace(m[3]),
			UnitPrice: unitPrice, TotalNetPurchasePrice: totalPrice,
		})
	}
	return products
}

// ResolveCustomerCode mirrors the 4-branch customer-code lookup inline
// in process_file's BigC branch (xulydonhang.py:9419-9433): a
// cross-product of 2 supplier codes x 2 warehouse names, checked via
// plain substring containment against page-0's raw text, in this exact
// order, with a default fallback matching Python's else branch. Returns
// both the resolved customer code AND the delivery-warehouse string
// (diachigiao, xulydonhang.py's second assigned variable in every
// branch — written to Excel column E downstream).
//
// No NBSP normalization here: this function only does plain
// strings.Contains literal substring checks, and Python's own `in`
// operator is likewise a literal substring check with no whitespace-class
// awareness. There is no Go/Python divergence to fix (unlike
// ExtractPriceList's \s-heavy regex above).
var storeNamePattern = regexp.MustCompile(`(?s)(FM LOGISTIC VSIP 2|LINFOX WAREHOUSE \(802\)).*?Vietnam\s*\n(.*?)\n`)

// ExtractStoreName mirrors lay_ten_store (xulydonhang.py:5878-5884): the
// line immediately following the first "Vietnam" occurrence after the
// warehouse name (FM LOGISTIC VSIP 2 or LINFOX WAREHOUSE (802)) on a
// single store page. Returns ("", false) if no match — mirrors Python
// returning None.
//
// IMPORTANT: Go's RE2 engine treats \s as ASCII-only whitespace, while
// Python's re module treats \s as Unicode-aware and matches non-breaking
// space (U+00A0 / \xa0). lay_ten_store's own pattern (xulydonhang.py:5880)
// never strips \xa0 from its input either — it just relies on Python's
// Unicode-aware \s to silently swallow it in "Vietnam\s*\n". This is a
// confirmed real artifact in this project's PDF-extracted text (see
// xulydonhang.py's demsodonhang1trang_coop, which explicitly does
// text.replace("\xa0", " ") before further processing), and this
// function's pattern is exposed to the same risk ParseOrderInfo's and
// ExtractPriceList's patterns were, so this function normalizes U+00A0
// to a regular space before matching too, for the same reason.
func ExtractStoreName(storePageText string) (string, bool) {
	storePageText = strings.ReplaceAll(storePageText, string(rune(0x00A0)), " ")
	m := storeNamePattern.FindStringSubmatch(storePageText)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[2]), true
}

// StoreItem is one line of a single store page's item list. UnitPrice is
// zero until JoinItemsWithPrices fills it in from the page-0 master
// list — ExtractStoreItems alone never sets it (mirrors
// trichxuatdanhsachforstore_bigc producing dicts with no price field at
// all, xulydonhang.py:5906).
type StoreItem struct {
	Barcode        string
	SKUOrUnit      string
	OrderedUnitQty string
	UnitPrice      float64
}

// storeItemPattern mirrors trichxuatdanhsachforstore_bigc's regex
// (xulydonhang.py:5902): r"(?<=\n)(\d{13})\s*\n(.*?)\s*\nPack\s*\n\d+\s*\n(\d+)\s*\n(\d+)"
// with re.DOTALL. Go's RE2 engine has no lookbehind support, so
// "(?<=\n)" is replaced with "(?:^|\n)" (non-capturing, doesn't shift
// group numbering) — equivalent for this use, since both only need to
// confirm the barcode starts at a line boundary. Group 2 (the
// description line between the barcode and "Pack") is matched but
// deliberately discarded, matching Python's list comprehension only
// keeping groups 1, 3, 4 (xulydonhang.py:5906: m[0], m[2], m[3] from a
// 0-indexed findall tuple).
var storeItemPattern = regexp.MustCompile(`(?s)(?:^|\n)(\d{13})\s*\n(.*?)\s*\nPack\s*\n\d+\s*\n(\d+)\s*\n(\d+)`)

// ExtractStoreItems mirrors trichxuatdanhsachforstore_bigc
// (xulydonhang.py:5900-5907).
//
// IMPORTANT: Go's RE2 engine treats \s as ASCII-only whitespace, while
// Python's re module treats \s as Unicode-aware and matches non-breaking
// space (U+00A0 / \xa0). trichxuatdanhsachforstore_bigc's own pattern
// (xulydonhang.py:5902) never strips \xa0 from its input either — it
// just relies on Python's Unicode-aware \s to silently swallow it across
// its several \s* runs. This is the same confirmed real artifact
// documented on ParseOrderInfo/ExtractPriceList/ExtractStoreName above,
// so this function normalizes U+00A0 to a regular space before matching
// too, for the same reason.
func ExtractStoreItems(storePageText string) []StoreItem {
	storePageText = strings.ReplaceAll(storePageText, string(rune(0x00A0)), " ")
	var items []StoreItem
	for _, m := range storeItemPattern.FindAllStringSubmatch(storePageText, -1) {
		items = append(items, StoreItem{Barcode: m[1], SKUOrUnit: m[3], OrderedUnitQty: m[4]})
	}
	return items
}

// JoinItemsWithPrices mirrors ghepgia_donhangbigc (xulydonhang.py:5888-5897):
// looks up each item's UnitPrice from the page-0 price list by barcode.
// An item whose barcode isn't in the price list silently gets UnitPrice
// 0 — Python's Vietnamese comment claims this "reports an error" but the
// actual code does not; faithfully reproduced, not "fixed" — a resulting
// 0 price surfaces downstream in bigc_processor.go as a price mismatch,
// same as any other genuinely-zero real price would.
//
// No NBSP normalization here: this function does a plain map lookup by
// barcode string, with no regex involved — no Go/Python divergence to
// fix (same reasoning as ResolveCustomerCode above).
func JoinItemsWithPrices(items []StoreItem, priceList []Product) []StoreItem {
	prices := make(map[string]float64, len(priceList))
	for _, p := range priceList {
		prices[p.Barcode] = p.UnitPrice
	}
	joined := make([]StoreItem, len(items))
	for i, item := range items {
		item.UnitPrice = prices[item.Barcode] // zero value if not found — matches Python's dict.get(article, 0)
		joined[i] = item
	}
	return joined
}

func ResolveCustomerCode(pageZeroText string) (customerCode, deliveryWarehouse string) {
	has3006900 := strings.Contains(pageZeroText, "3006900")
	has3005382 := strings.Contains(pageZeroText, "3005382")
	hasLinfox := strings.Contains(pageZeroText, "LINFOX WAREHOUSE (802)")
	hasFMLogistic := strings.Contains(pageZeroText, "FM LOGISTIC VSIP 2 (806)")

	switch {
	case has3006900 && hasLinfox:
		return "MB_GC_BIGC", "LINFOX WAREHOUSE (802)"
	case has3005382 && hasLinfox:
		return "MB_MT_BIGC", "LINFOX WAREHOUSE (802)"
	case has3005382 && hasFMLogistic:
		return "MN_MT_BIGCAC", "FM LOGISTIC VSIP 2 (806)"
	case has3006900 && hasFMLogistic:
		return "MN_GC_BIGCAC", "FM LOGISTIC VSIP 2 (806)"
	default:
		return "MN_MT_BIGCAC", "FM LOGISTIC VSIP 2 (806)"
	}
}
