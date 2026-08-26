package productdata

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	fuzzywuzzy "github.com/paul-mannino/go-fuzzywuzzy"
	"github.com/xuri/excelize/v2"
)

// ProductInfo mirrors the columns in data.xlsx's SanPham sheet needed by
// the Coop pipeline: B=name, C=weight (kg), D=case pack size.
type ProductInfo struct {
	Name     string
	WeightKg float64
	PackSize float64
}

// Store is an in-memory index over data.xlsx's MaKH and SanPham sheets,
// loaded once at startup. xulydonhang.py re-opens data.xlsx from disk on
// almost every single lookup call; Store exists to avoid that.
type Store struct {
	customerRows   [][4]string
	products       map[string]ProductInfo
	skuMapping     map[string]string
	skuAlternation *regexp.Regexp
}

// Load builds a Store from data.xlsx's MaKH and SanPham sheets, read
// directly off disk. See LoadFromSheets (sheets_source.go) for the
// production Google-Sheets-backed equivalent — both share the same
// row-processing logic (loadCustomerRows/loadProducts below), which only
// cares about [][]string rows, not where they came from.
func Load(path string) (*Store, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("productdata: open %s: %w", path, err)
	}
	defer f.Close()

	customerSheetRows, err := f.GetRows("MaKH")
	if err != nil {
		return nil, fmt.Errorf("productdata: read MaKH sheet: %w", err)
	}
	// RawCellValue: true is required here — without it, GetRows returns
	// the cell's *displayed* string, rounded to whatever number format
	// is applied in the spreadsheet (e.g. weight 3.475 displays/rounds
	// to "3.48" under a "0.00" format). Python's openpyxl reads the
	// actual underlying float (3.475) with no such formatting applied,
	// so the un-raw read here silently double-rounds every weight/pack
	// size, compounding into wrong line-weight (AT) and total-weight (L)
	// values downstream — confirmed against data.xlsx directly (SKU
	// TP31630's true weight is 3.475 kg; GetRows without RawCellValue
	// returned "3.48").
	productSheetRows, err := f.GetRows("SanPham", excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, fmt.Errorf("productdata: read SanPham sheet: %w", err)
	}

	return newStore(customerSheetRows, productSheetRows), nil
}

// newStore builds a Store directly from already-read row data — shared
// by Load (local data.xlsx) and LoadFromSheets (live Google Sheets
// fetch), which differ only in how they obtain customerRows/productRows,
// not in how those rows get parsed.
func newStore(customerRows, productRows [][]string) *Store {
	customers := loadCustomerRows(customerRows)
	products, skuMapping := loadProducts(productRows)
	return &Store{
		customerRows:   customers,
		products:       products,
		skuMapping:     skuMapping,
		skuAlternation: buildSkuAlternation(products),
	}
}

func loadCustomerRows(rows [][]string) [][4]string {
	var out [][4]string
	for i, row := range rows {
		if i == 0 {
			continue
		}
		var r [4]string
		for c := 0; c < 4 && c < len(row); c++ {
			r[c] = row[c]
		}
		out = append(out, r)
	}
	return out
}

func loadProducts(rows [][]string) (map[string]ProductInfo, map[string]string) {
	products := make(map[string]ProductInfo)
	skuMapping := make(map[string]string)
	ws := regexp.MustCompile(`\s+`)

	for i, row := range rows {
		if i == 0 || len(row) == 0 || strings.TrimSpace(row[0]) == "" {
			continue
		}
		skuCode := strings.TrimSpace(row[0])

		// Python's timten_sanpham/timtrongluong_sanpham/timquycach_sanpham
		// (xulydonhang.py:784-817) each linearly scan the sheet and
		// `return` on the FIRST row whose column A matches — so for the
		// handful of SKUs that appear more than once in data.xlsx (e.g.
		// TP32415_01, TP32422_01, each duplicated with stale/wrong
		// name+weight in a later row), Python always resolves to the
		// FIRST occurrence. An unconditional map write here would let
		// whichever duplicate is read LAST win instead, which for those
		// two SKUs silently substitutes the wrong product name
		// ("...can 3,2l" instead of "...can 3,2L") and wrong weight
		// (3.2kg instead of 3.45kg) — confirmed against real data.xlsx
		// and Satra's frozen fixtures during Task 7's golden-fixture
		// investigation. Note: this first-wins rule applies ONLY to this
		// products map — skuMapping below intentionally keeps Python's
		// load_sku_mapping dict-assignment (unconditional overwrite,
		// i.e. LAST occurrence wins), since that function builds a plain
		// dict rather than a scan-and-return, so it must still run for
		// every row including duplicates.
		if _, exists := products[skuCode]; !exists {
			info := ProductInfo{}
			if len(row) > 1 {
				info.Name = row[1]
			}
			if len(row) > 2 {
				info.WeightKg = parseFloat(row[2])
			}
			if len(row) > 3 {
				info.PackSize = parseFloat(row[3])
			}
			products[skuCode] = info
		}

		// Mirrors load_sku_mapping: EVERY non-empty cell from column C
		// (index 2) onward maps back to this row's internal SKU,
		// including the weight/pack-size columns, not just the
		// per-vendor SKU columns further right — preserved verbatim
		// from xulydonhang.py, see this plan's Global Constraints.
		for c := 2; c < len(row); c++ {
			cell := ws.ReplaceAllString(row[c], "")
			if cell != "" {
				skuMapping[cell] = skuCode
			}
		}
	}

	return products, skuMapping
}

// parseFloat handles both number conventions this Store's two row
// sources actually produce: data.xlsx's raw excelize read (RawCellValue:
// true) always renders these weight/pack-size cells with a dot decimal
// separator (e.g. "3.475", confirmed — see loadProducts' own comment on
// TP31630), while a live Google Sheets CSV export renders the SAME kind
// of value in Vietnamese locale, comma decimal (e.g. "3,48" — confirmed
// empirically against the real production sheet: TP31630's own row
// there). Neither source has ever been observed using BOTH a comma AND
// a dot for these two columns (no thousands-grouping on values this
// small), so "comma present, dot absent" reliably identifies the
// Sheets-CSV convention and gets normalized to a dot before parsing;
// otherwise a comma is stripped as a thousands separator, unchanged from
// this function's original (xlsx-only) behavior.
func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if strings.Contains(s, ",") && !strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ",", ".")
	} else {
		s = strings.ReplaceAll(s, ",", "")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func buildSkuAlternation(products map[string]ProductInfo) *regexp.Regexp {
	if len(products) == 0 {
		return nil
	}
	skus := make([]string, 0, len(products))
	for sku := range products {
		skus = append(skus, regexp.QuoteMeta(sku))
	}
	sort.Strings(skus)
	return regexp.MustCompile(`\b(` + strings.Join(skus, "|") + `)\b`)
}

var trailingDigitsPattern = regexp.MustCompile(`(\d+)$`)

// GetCustomerCode mirrors get_makhachhang exactly, including its bug —
// see this plan's Global Constraints for the exact explanation.
func (s *Store) GetCustomerCode(poLocation string) string {
	poLocation = strings.TrimSpace(poLocation)
	for _, row := range s.customerRows {
		colA, colB, colC := row[0], row[2], row[2]
		system := strings.ToUpper(strings.TrimSpace(colA))
		if system != "COOP" && system != "COOPFOOD" {
			continue
		}
		if colB == "" {
			continue
		}
		m := trailingDigitsPattern.FindStringSubmatch(strings.TrimSpace(colB))
		if m == nil {
			continue
		}
		if m[1] == poLocation {
			return colC
		}
	}
	return "Không tìm thấy"
}

// GetCustomerCodeBySuffix mirrors get_makhachhang_lotte
// (xulydonhang.py:307-321): filters customer rows to the given system
// (column A, case-insensitive exact match), then returns the first
// row's column C value whose trimmed content ends with storeCode.
// Unlike GetCustomerCode (Coop's get_makhachhang, which has a genuine
// double-read-column-C bug preserved from Python), this reads columns A
// and C correctly — get_makhachhang_lotte also reads column B into a
// variable but never actually uses it in its comparison, so that read is
// simply omitted here (dead in the original, not a behavior to
// replicate). Returns "" (not Python's None) when nothing matches;
// callers that need a "Không xác định" placeholder apply that
// themselves — but this does NOT mirror where Python applies it.
// Python's write_to_dondathang_lotte (xulydonhang.py:9128-9139) is
// called with the raw, possibly-None store_code BEFORE any placeholder
// substitution; the substitution only happens afterward, and only
// feeds a UI status-table signal, never the row-building call. If
// get_makhachhang_lotte returns None, Python crashes with an unhandled
// TypeError inside write_to_dondathang_lotte on `makhachhang[:2] ==
// "MB"` (xulydonhang.py:1992) before it ever reaches a placeholder. Go's
// processLotteSegment (coop_processor.go) applies "Không xác định"
// earlier and more defensively than Python does — before building any
// row, so the placeholder lands in Excel column G and feeds regionInfo
// — a deliberate, documented divergence under this plan's "correct main
// flow, no bug-for-bug parity owed" policy, not a mirror of Python's
// (crashing) behavior on this input.
func (s *Store) GetCustomerCodeBySuffix(system, storeCode string) string {
	system = strings.ToUpper(strings.TrimSpace(system))
	storeCode = strings.TrimSpace(storeCode)
	for _, row := range s.customerRows {
		colA, colC := row[0], row[2]
		if strings.ToUpper(strings.TrimSpace(colA)) != system {
			continue
		}
		trimmedC := strings.TrimSpace(colC)
		if trimmedC != "" && strings.HasSuffix(trimmedC, storeCode) {
			return trimmedC
		}
	}
	return ""
}

// GetCustomerCodeForSystem returns the customer code (column C) of the
// FIRST row belonging to the named system (column A, compared
// case-insensitively), for a retailer that has exactly one customer code
// rather than one per store.
//
// Has no Python counterpart: every vendor in the old app either resolved
// a per-store code (Coop's get_makhachhang, Lotte's
// get_makhachhang_lotte) or hardcoded a single one at the call site
// (Kingfood, JMart). Maxidi has exactly one code but reads it from the
// sheet, so it can be corrected without rebuilding the app.
//
// Returns ok=false when no row matches or the matched row's code cell is
// blank, rather than a bare "" the caller could mistake for a real
// value: this code is written straight into the order workbook's
// customer column, where a blank means an order billed to nobody.
func (s *Store) GetCustomerCodeForSystem(system string) (string, bool) {
	system = strings.ToUpper(strings.TrimSpace(system))
	if system == "" {
		return "", false
	}
	for _, row := range s.customerRows {
		if strings.ToUpper(strings.TrimSpace(row[0])) != system {
			continue
		}
		if code := strings.TrimSpace(row[2]); code != "" {
			return code, true
		}
	}
	return "", false
}

// GetSystemForCustomer mirrors layhethong_COOP: column C -> column A.
func (s *Store) GetSystemForCustomer(customerCode string) string {
	customerCode = strings.TrimSpace(customerCode)
	for _, row := range s.customerRows {
		colC := row[2]
		if strings.TrimSpace(colC) == customerCode {
			return row[0]
		}
	}
	return ""
}

// GetCoopfoodAddress mirrors laydiachi_coopfood: column C -> column D.
func (s *Store) GetCoopfoodAddress(customerCode string) string {
	customerCode = strings.TrimSpace(customerCode)
	for _, row := range s.customerRows {
		if strings.TrimSpace(row[2]) == customerCode {
			return row[3]
		}
	}
	return ""
}

// GetSiteValue mirrors tim_gia_tri_congtrinh (xulydonhang.py:4586-4603):
// scans MaKH's column B (mã công trình, row[1]) for the row whose value
// equals code exactly (no trim on either side, matching Python's bare
// `==`), returning that row's column C (row[2], giá trị công trình) —
// this is BigC's per-product/promo-bonus AN column value
// (write_to_dondathang_bigc's `congtrinh` after the lookup).
//
// Exact match is the ONLY comparison Python performs, because PyMuPDF's
// text extraction always hands it a clean, single-line store name.
// Confirmed empirically (direct PyMuPDF extraction of a real fixture,
// 2632058001987.pdf page 2) that this port's own PDF text extraction
// does NOT always give the same guarantee: some real BigC pages have
// the store-name line and its immediately-following address line
// extracted with no line break between them (PyMuPDF: "GO! AN
// LAC\nSO 1231 KP 5, ..."; this port: "GO! AN LACSO 1231 KP 5, ..." as
// one run-together string) — a gap in the underlying PDF library's line
// detection (see bigc.ExtractStoreName's caller in bigc_processor.go),
// not something exact-match alone can route around. So when no row's
// column B equals code exactly, this ALSO tries the LONGEST column B
// value that code merely STARTS WITH — a Go-only addition (Python has
// no equivalent) that recovers the correct site value from that glued
// text without needing the PDF extraction itself fixed. "Longest", not
// "first": real MaKH data has both "GO! VINH" and "GO! VINH PHUC" as
// distinct stores, the former a genuine prefix of the latter — matching
// on the first (shortest) hit would silently misattribute every
// "GO! VINH PHUC..." page to "GO! VINH"'s site value instead.
//
// Only once BOTH checks fail does this fall back to code with every
// space character stripped (not trimmed - matches Python's
// `congtrinh.replace(" ", "")`, which removes spaces anywhere in the
// string, not just leading/trailing).
func (s *Store) GetSiteValue(code string) string {
	for _, row := range s.customerRows {
		if row[1] == code {
			return row[2]
		}
	}

	bestLen := -1
	bestValue := ""
	for _, row := range s.customerRows {
		name := row[1]
		if name == "" {
			continue
		}
		if len(name) > bestLen && strings.HasPrefix(code, name) {
			bestLen = len(name)
			bestValue = row[2]
		}
	}
	if bestLen >= 0 {
		return bestValue
	}

	return strings.ReplaceAll(code, " ", "")
}

// GetProductInfo merges timten_sanpham/timtrongluong_sanpham/
// timquycach_sanpham (three separate linear scans + file re-opens in
// Python) into one lookup against the Store loaded once at startup.
func (s *Store) GetProductInfo(sku string) (ProductInfo, bool) {
	info, ok := s.products[sku]
	return info, ok
}

var skuCleanupPattern = regexp.MustCompile(`(\d{7})-\d`)
var skuAliasNoisePattern = regexp.MustCompile(`[^\p{L}\p{N}]`)
var skuAliasSizePattern = regexp.MustCompile(`\d+(?:ml|kg|l|g)$`)

// CleanSkuNumber mirrors clean_sku_number: strips a Coop-style
// "1234567-1" barcode down to its 7-digit prefix.
func CleanSkuNumber(sku string) string {
	m := skuCleanupPattern.FindStringSubmatch(sku)
	if m == nil {
		return sku
	}
	return m[1]
}

// ResolveSku mirrors clean_sku_number + replace_sku_numbers' mapping
// lookup: returns the internal SKU if a mapping entry exists, else the
// cleaned (but unmapped) value.
func (s *Store) ResolveSku(barcode string) string {
	cleaned := CleanSkuNumber(barcode)
	if mapped, ok := s.skuMapping[cleaned]; ok {
		return mapped
	}
	return cleaned
}

// ResolveSkuAlias resolves exact aliases first, then tolerates the small
// presentation-only differences found in Top Value airway labels: spacing,
// punctuation, casing and the optional word "New". If exact normalized
// equality is required; this deliberately does not use substring/fuzzy
// matching because shorter and longer Top Value variants can be different SKUs.
func (s *Store) ResolveSkuAlias(alias string) (string, bool) {
	resolved := s.ResolveSku(alias)
	if _, ok := s.products[resolved]; ok {
		return resolved, true
	}
	normalized := normalizeSkuAlias(alias)
	matchedSku := ""
	for key, sku := range s.skuMapping {
		candidate := normalizeSkuAlias(key)
		if candidate == "" || candidate != normalized {
			continue
		}
		if matchedSku != "" && matchedSku != sku {
			return resolved, false
		}
		matchedSku = sku
	}
	if matchedSku != "" {
		if _, ok := s.products[matchedSku]; ok {
			return matchedSku, true
		}
	}
	return resolved, false
}

func normalizeSkuAlias(value string) string {
	value = strings.ToLower(value)
	// Top Value labels omit these catalogue-only presentation words on some
	// products. Ambiguous normalized matches are still rejected above.
	for _, noise := range []string{"topvalue", "new", "hương", "túi"} {
		value = strings.ReplaceAll(value, noise, "")
	}
	value = skuAliasNoisePattern.ReplaceAllString(value, "")
	if size := skuAliasSizePattern.FindString(value); size != "" {
		prefix := strings.TrimSuffix(value, size)
		if strings.HasSuffix(prefix, size) {
			value = prefix
		}
	}
	return value
}

// FindSkusMentioned mirrors check_value_in_sanpham: scans free text for
// any known internal SKU as a whole word, returning every match in the
// order found (duplicates included, matching Python's re.findall).
func (s *Store) FindSkusMentioned(text string) []string {
	if text == "" || s.skuAlternation == nil {
		return nil
	}
	return s.skuAlternation.FindAllString(text, -1)
}

var normalizeTextPattern = regexp.MustCompile(`[^a-z0-9\s]`)
var normalizeWhitespacePattern = regexp.MustCompile(`\s+`)

// NormalizeText mirrors normalize_text (xulydonhang.py:217-222) exactly:
// lowercase, then strip every character that is not an ASCII letter,
// digit, or whitespace (this deliberately removes Vietnamese diacritic
// letters entirely, not just their diacritic marks — e.g. "Huệ" becomes
// "hu", not "hue" — because Python's [^a-z0-9\s] character class only
// allows literal ASCII a-z/0-9, and re operates on Unicode code points,
// so any non-ASCII letter is stripped whole), then collapse runs of
// whitespace to one space and trim.
func NormalizeText(s string) string {
	lower := strings.ToLower(s)
	stripped := normalizeTextPattern.ReplaceAllString(lower, "")
	return strings.TrimSpace(normalizeWhitespacePattern.ReplaceAllString(stripped, " "))
}

// GetCustomerCodeByFuzzyAddress mirrors laymakhachhang_satra
// (xulydonhang.py:263-287): filters customer rows to those whose column
// A (system) is non-blank (Python: `if col_A and ...` — a row with a
// blank column A never participates, regardless of what system is being
// searched for; without this guard, Go's strings.Contains(x, "") is
// always true and a blank column A would wrongly pass the filter for
// every query) AND, uppercased and trimmed, is a SUBSTRING of the given
// system string (Python: `col_A.upper() in hethong` — NOT equality;
// preserved exactly, since Satra's real call site passes the literal
// system name itself as `hethong`, e.g. laymakhachhang_satra(diachi,
// "SATRA"), so a column A of "SATRA" is trivially "in" it, but this is a
// real substring check, not coincidentally equivalent to equality for
// every possible input), then finds the row whose column D (address),
// both sides run through NormalizeText, has the highest PartialRatio
// score against the given address — returns that row's column C if the
// best score is STRICTLY greater than 95, mirroring Python's
// `best_score > 95` (not >=). Returns ("", false) if no row exceeds the
// threshold — mirrors Python returning None; the caller applies any
// "Không xác định"-style placeholder itself.
func (s *Store) GetCustomerCodeByFuzzyAddress(system, address string) (string, bool) {
	systemUpper := strings.ToUpper(system)
	addressNorm := NormalizeText(address)

	bestScore := 0
	bestCode := ""
	for _, row := range s.customerRows {
		colA, colC, colD := row[0], row[2], row[3]
		trimmedA := strings.TrimSpace(colA)
		if trimmedA == "" {
			continue
		}
		if !strings.Contains(systemUpper, strings.ToUpper(trimmedA)) {
			continue
		}
		if colD == "" {
			continue
		}
		score := fuzzywuzzy.PartialRatio(addressNorm, NormalizeText(colD))
		if score > bestScore {
			bestScore = score
			bestCode = colC
		}
	}
	if bestScore > 95 {
		return bestCode, true
	}
	return "", false
}
