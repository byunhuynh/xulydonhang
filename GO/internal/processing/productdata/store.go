package productdata

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

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

func Load(path string) (*Store, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("productdata: open %s: %w", path, err)
	}
	defer f.Close()

	customerRows, err := loadCustomerRows(f)
	if err != nil {
		return nil, err
	}
	products, skuMapping, err := loadProducts(f)
	if err != nil {
		return nil, err
	}

	return &Store{
		customerRows:   customerRows,
		products:       products,
		skuMapping:     skuMapping,
		skuAlternation: buildSkuAlternation(products),
	}, nil
}

func loadCustomerRows(f *excelize.File) ([][4]string, error) {
	rows, err := f.GetRows("MaKH")
	if err != nil {
		return nil, fmt.Errorf("productdata: read MaKH sheet: %w", err)
	}
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
	return out, nil
}

func loadProducts(f *excelize.File) (map[string]ProductInfo, map[string]string, error) {
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
	rows, err := f.GetRows("SanPham", excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, nil, fmt.Errorf("productdata: read SanPham sheet: %w", err)
	}

	products := make(map[string]ProductInfo)
	skuMapping := make(map[string]string)
	ws := regexp.MustCompile(`\s+`)

	for i, row := range rows {
		if i == 0 || len(row) == 0 || strings.TrimSpace(row[0]) == "" {
			continue
		}
		skuCode := strings.TrimSpace(row[0])

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

	return products, skuMapping, nil
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
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

// GetProductInfo merges timten_sanpham/timtrongluong_sanpham/
// timquycach_sanpham (three separate linear scans + file re-opens in
// Python) into one lookup against the Store loaded once at startup.
func (s *Store) GetProductInfo(sku string) (ProductInfo, bool) {
	info, ok := s.products[sku]
	return info, ok
}

var skuCleanupPattern = regexp.MustCompile(`(\d{7})-\d`)

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

// FindSkusMentioned mirrors check_value_in_sanpham: scans free text for
// any known internal SKU as a whole word, returning every match in the
// order found (duplicates included, matching Python's re.findall).
func (s *Store) FindSkusMentioned(text string) []string {
	if text == "" || s.skuAlternation == nil {
		return nil
	}
	return s.skuAlternation.FindAllString(text, -1)
}
