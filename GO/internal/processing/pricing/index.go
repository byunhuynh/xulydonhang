package pricing

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Promotion is one (date-range column, promo text) match, mirroring the
// (col, value) tuples find_all_promotions_by_sku_and_time returns.
type Promotion struct {
	Column string
	Value  string
}

// Index holds one fetched Coop pricing/promotion CSV in memory, so a
// whole order's worth of SKU/promotion lookups costs one network fetch
// instead of one fetch per SKU (xulydonhang.py's find_price_by_sku and
// find_all_promotions_by_sku_and_time each fetch the sheet fresh on
// every single call).
type Index struct {
	rows           [][]string // raw rows, row 0 = header
	priceBySku     map[string]string
	header         []string
	skuColumnIndex int // -1 if no "Mã hàng" column found
}

// ParseIndex mirrors both find_price_by_sku's positional 4-column read
// (SKU at index 1, price at index 3, "." stripped) and
// find_all_promotions_by_sku_and_time's named-header read (first column
// containing "Mã hàng"), from the same underlying CSV rows.
func ParseIndex(csvRows [][]string) *Index {
	idx := &Index{rows: csvRows, priceBySku: make(map[string]string), skuColumnIndex: -1}

	if len(csvRows) > 0 {
		idx.header = normalizeHeader(csvRows[0])
		for i, h := range idx.header {
			if strings.Contains(h, "Mã hàng") {
				idx.skuColumnIndex = i
				break
			}
		}
	}

	for _, row := range csvRows[minInt(1, len(csvRows)):] {
		if len(row) < 4 {
			continue
		}
		sku := row[1]
		price := strings.ReplaceAll(row[3], ".", "")
		if strings.TrimSpace(price) != "" {
			idx.priceBySku[sku] = price
		}
	}

	return idx
}

func normalizeHeader(row []string) []string {
	out := make([]string, len(row))
	for i, h := range row {
		h = strings.TrimSpace(h)
		h = strings.ReplaceAll(h, "\n", " ")
		h = strings.ReplaceAll(h, "\r", "")
		out[i] = h
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// FindPrice mirrors find_price_by_sku's lookup: the query SKU has all
// whitespace stripped before comparison, but stored CSV values are
// compared as-is (not stripped) — preserved exactly.
func (idx *Index) FindPrice(sku string) (string, bool) {
	sku = wsPattern.ReplaceAllString(sku, "")
	price, ok := idx.priceBySku[sku]
	return price, ok
}

var wsPattern = regexp.MustCompile(`\s+`)

// dateRangePattern requires a HYPHEN between the two dates, and that is
// load-bearing rather than incidental. The Coop sheet's owner parks a
// campaign by writing its dates with a space instead ("Riêng 17/07
// 30/08", "CTKM CF 17/07 30/09" — both live in the sheet today): the
// column stops matching here and goes dormant, cells and all, without
// being deleted. Do NOT "fix" this into accepting a space — it would
// reactivate every parked campaign in the sheet at once. Locked by
// TestFindPromotions_SpaceSeparatedDatesAreNotARange.
var dateRangePattern = regexp.MustCompile(`(\d{1,2}/\d{1,2})-(\d{1,2}/\d{1,2})`)
var yearSuffixPattern = regexp.MustCompile(`/\d{4}$`)

// FindPromotions mirrors find_all_promotions_by_sku_and_time: finds the
// SKU's row via the "Mã hàng"-named column, then returns every
// (column, value) pair whose column header is a "D/M-D/M" date range
// containing timeToCheck, skipping empty values.
func (idx *Index) FindPromotions(sku, timeToCheck string) []Promotion {
	if idx.skuColumnIndex < 0 || len(idx.rows) < 2 {
		return nil
	}

	var skuRow []string
	for _, row := range idx.rows[1:] {
		if idx.skuColumnIndex < len(row) && row[idx.skuColumnIndex] == sku {
			skuRow = row
			break
		}
	}
	if skuRow == nil {
		return nil
	}

	var promos []Promotion
	for i, h := range idx.header {
		if !isWithinDateRange(timeToCheck, h) {
			continue
		}
		if i >= len(skuRow) {
			continue
		}
		value := skuRow[i]
		if strings.TrimSpace(value) != "" {
			promos = append(promos, Promotion{Column: h, Value: value})
		}
	}
	return promos
}

// FindInvoicePromotion mirrors write_to_dondathang's invoice-level bonus
// lookup: find_all_promotions_by_sku_and_time("Hóa Đơn", entry_date,
// vendor) — same mechanism as FindPromotions but keyed on the literal
// SKU value "Hóa Đơn" instead of a real product SKU, then returns the
// single-column-string form used directly in write_to_dondathang
// (`kmhoadon = ProcessHandler.find_all_promotions_by_sku_and_time(...)`,
// then `if kmhoadon:` treats the whole returned value as one string,
// since exactly one date-range column is expected to be active at a
// time in practice). Returns "" if nothing matched.
func (idx *Index) FindInvoicePromotion(timeToCheck string) string {
	promos := idx.InvoicePromotions(timeToCheck)
	if len(promos) == 0 {
		return ""
	}
	return promos[0].Value
}

// InvoicePromotions is FindInvoicePromotion's underlying lookup with the
// column names kept. Coop needs them: which of the two systems
// (Coopmart/Coopfood) a CTKM belongs to can be written in the campaign
// name, so it has to scope the invoice-level CTKM itself rather than
// take FindInvoicePromotion's already-collapsed first value.
func (idx *Index) InvoicePromotions(timeToCheck string) []Promotion {
	return idx.FindPromotions("Hóa Đơn", timeToCheck)
}

func normalizeDDMM(s string) (string, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return "", errInvalidDate
	}
	day, err1 := strconv.Atoi(parts[0])
	month, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return "", errInvalidDate
	}
	return pad2(day) + "/" + pad2(month), nil
}

func pad2(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

var errInvalidDate = &dateError{}

type dateError struct{}

func (*dateError) Error() string { return "invalid date" }

func isWithinDateRange(timeToCheck, columnName string) bool {
	m := dateRangePattern.FindStringSubmatch(columnName)
	if m == nil {
		return false
	}
	startRaw, endRaw := m[1], m[2]

	// Mirrors is_within_date_range's swap heuristic: strip a trailing
	// /YYYY from timeToCheck, then if the first component is <=12 and
	// the second is >12, treat the input as M/D and swap to D/M.
	tc := yearSuffixPattern.ReplaceAllString(timeToCheck, "")
	parts := strings.Split(tc, "/")
	if len(parts) == 2 {
		p1, err1 := strconv.Atoi(parts[0])
		p2, err2 := strconv.Atoi(parts[1])
		if err1 == nil && err2 == nil && p1 <= 12 && p2 > 12 {
			tc = strconv.Itoa(p2) + "/" + strconv.Itoa(p1)
		}
	}

	start, err1 := normalizeDDMM(startRaw)
	end, err2 := normalizeDDMM(endRaw)
	tcNorm, err3 := normalizeDDMM(tc)
	if err1 != nil || err2 != nil || err3 != nil {
		return false
	}

	year := time.Now().Year()
	layout := "02/01/2006"
	tcDate, err4 := time.Parse(layout, tcNorm+"/"+strconv.Itoa(year))
	startDate, err5 := time.Parse(layout, start+"/"+strconv.Itoa(year))
	endDate, err6 := time.Parse(layout, end+"/"+strconv.Itoa(year))
	if err4 != nil || err5 != nil || err6 != nil {
		return false
	}

	return !tcDate.Before(startDate) && !tcDate.After(endDate)
}
