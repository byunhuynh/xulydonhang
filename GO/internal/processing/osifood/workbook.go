package osifood

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// OsiFood's second order format is the campaign sheet itself ("CTKM
// 16-31.10...xlsx"): one workbook covering every store, where each store
// answers with the quantity it wants. One row per product, one block of
// four columns per store:
//
//	row 1   delivery address + contact   (above the quantity column)
//	row 2   store code                   (above all four columns)
//	row 3   "Barcode" … product headers … store name
//	row 4   L.BÁN 10 NGÀY | L.TỒN | CH phản hồi theo nhu cầu… | Thành tiền
//	row 5   supplier name | "MÃ NCC: 00758" … per-store totals
//	row 6…  one product per row
//
// "CH phản hồi theo nhu cầu kinh doanh" is the ordered quantity. A store
// whose quantities are all blank or zero placed no order.
//
// Nothing below depends on those row or column NUMBERS: the header row is
// found by its "Barcode" cell and every column by its own caption, because
// the sheet is hand-maintained (thirteen of its columns are hidden, and
// two trailing store blocks are unfilled templates with no store code).

var (
	workbookSupplierPattern = regexp.MustCompile(`MÃ NCC\s*:\s*00758\b`)
	barcodePattern          = regexp.MustCompile(`^\d{13}$`)
	storeCodePattern        = regexp.MustCompile(`^\d+$`)
	// promoRangePattern reads the first day of "16/10 - 31/10/2026".
	promoRangePattern = regexp.MustCompile(`(\d{1,2})/(\d{1,2})\s*-\s*\d{1,2}/\d{1,2}(?:/(\d{4}))?`)
)

// vietnamTime is the zone a workbook's saved-at timestamp is read in: it
// is stored in UTC, and a sheet saved before 07:00 local time would
// otherwise be dated the previous day.
var vietnamTime = time.FixedZone("ICT", 7*60*60)

// WorkbookLine is one product a store ordered. Price is "Giá vốn thường",
// PromoPrice is "Đơn giá KM (-VAT)" (0 when the line has none) and Total
// is the sheet's own "Thành tiền" for this store (0 when blank).
type WorkbookLine struct {
	Barcode    string
	Name       string
	Price      float64
	PromoPrice float64
	PromoText  string
	// PromoStart is the first day of the line's "Ngày KM NCC" range, as
	// DD/MM/YYYY, or "" when the cell holds no range.
	PromoStart string
	Qty        float64
	Total      float64
}

// StoreOrder is everything one store ordered in the workbook.
type StoreOrder struct {
	StoreCode       string
	StoreName       string
	DeliveryAddress string
	Lines           []WorkbookLine
	// PrintedQty/PrintedAmount are the store's totals from the supplier
	// row, valid when HasPrintedTotals — the workbook's counterpart of the
	// PDF order's TOTAL row.
	PrintedQty       float64
	PrintedAmount    float64
	HasPrintedTotals bool
}

// Workbook is one parsed campaign sheet.
type Workbook struct {
	// EntryDate is the day the workbook was last saved (DD/MM/YYYY). The
	// sheet itself carries no order date — only the campaign's own dates
	// — so the save time recorded inside the file is the closest thing to
	// "when the stores answered".
	EntryDate string
	Orders    []StoreOrder
}

func normalizeCaption(s string) string {
	return strings.ToLower(strings.TrimSpace(whitespacePattern.ReplaceAllString(s, " ")))
}

func cellAt(rows [][]string, r, c int) string {
	if r < 0 || r >= len(rows) || c < 0 || c >= len(rows[r]) {
		return ""
	}
	return strings.TrimSpace(rows[r][c])
}

func numberAt(rows [][]string, r, c int) float64 {
	v, _ := strconv.ParseFloat(strings.ReplaceAll(cellAt(rows, r, c), ",", ""), 64)
	return v
}

func firstSheetRows(f *excelize.File) ([][]string, error) {
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("workbook không có sheet nào")
	}
	// RawCellValue: the displayed string is rounded to the cell's number
	// format, and these prices carry half-đồng fractions (110,500.5).
	return f.GetRows(sheets[0], excelize.Options{RawCellValue: true})
}

// supplierRow returns the index of the row carrying "MÃ NCC: 00758", or
// -1.
func supplierRow(rows [][]string) int {
	for r, row := range rows {
		for _, cell := range row {
			if workbookSupplierPattern.MatchString(cell) {
				return r
			}
		}
	}
	return -1
}

// headerRow returns the index of the row whose first cell is "Barcode",
// or -1.
func headerRow(rows [][]string) int {
	for r := range rows {
		if normalizeCaption(cellAt(rows, r, 0)) == "barcode" {
			return r
		}
	}
	return -1
}

// IsWorkbook reports whether path is an OsiFood campaign order sheet: an
// .xlsx whose first sheet names our supplier code and has the product
// table's "Barcode" header. A file that cannot be opened is simply not
// one, so the caller falls through to its ordinary handling and reports
// the real problem there.
func IsWorkbook(path string) bool {
	if !strings.EqualFold(filepath.Ext(path), ".xlsx") {
		return false
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		return false
	}
	defer f.Close()
	rows, err := firstSheetRows(f)
	if err != nil {
		return false
	}
	return supplierRow(rows) >= 0 && headerRow(rows) >= 0
}

// LoadWorkbook reads every store's order out of the campaign sheet.
// Stores that ordered nothing are left out; a sheet in which no store
// ordered anything comes back with no Orders and no error.
func LoadWorkbook(path string) (Workbook, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return Workbook{}, fmt.Errorf("không mở được file: %w", err)
	}
	defer f.Close()
	rows, err := firstSheetRows(f)
	if err != nil {
		return Workbook{}, fmt.Errorf("không đọc được sheet: %w", err)
	}

	header := headerRow(rows)
	if header < 0 {
		return Workbook{}, fmt.Errorf("không tìm thấy dòng tiêu đề có cột \"Barcode\"")
	}

	// Product columns, by caption.
	priceCol, promoPriceCol, promoTextCol, promoDateCol, nameCol := -1, -1, -1, -1, -1
	for c := range rows[header] {
		switch caption := normalizeCaption(cellAt(rows, header, c)); {
		case caption == "giá vốn thường":
			priceCol = c
		case strings.HasPrefix(caption, "đơn giá km"):
			promoPriceCol = c
		case strings.HasPrefix(caption, "cơ cấu km"):
			promoTextCol = c
		case strings.HasPrefix(caption, "ngày km"):
			promoDateCol = c
		case caption == "tên hàng":
			nameCol = c
		}
	}
	if priceCol < 0 {
		return Workbook{}, fmt.Errorf("không tìm thấy cột \"Giá vốn thường\"")
	}

	// Store blocks: every "CH phản hồi…" column that has a store code
	// above it.
	type storeColumn struct {
		order    StoreOrder
		qtyCol   int
		totalCol int
	}
	var stores []storeColumn
	width := 0
	if header+1 < len(rows) {
		width = len(rows[header+1])
	}
	for c := 0; c < width; c++ {
		if !strings.HasPrefix(normalizeCaption(cellAt(rows, header+1, c)), "ch phản hồi") {
			continue
		}
		code := cellAt(rows, header-1, c)
		if !storeCodePattern.MatchString(code) {
			continue
		}
		totalCol := -1
		if normalizeCaption(cellAt(rows, header+1, c+1)) == "thành tiền" {
			totalCol = c + 1
		}
		stores = append(stores, storeColumn{
			order: StoreOrder{
				StoreCode:       code,
				StoreName:       cellAt(rows, header, c),
				DeliveryAddress: strings.TrimSpace(whitespacePattern.ReplaceAllString(cellAt(rows, header-2, c), " ")),
			},
			qtyCol: c, totalCol: totalCol,
		})
	}
	if len(stores) == 0 {
		return Workbook{}, fmt.Errorf("không tìm thấy cột \"CH phản hồi theo nhu cầu kinh doanh\" nào có mã cửa hàng")
	}

	if totalsRow := supplierRow(rows); totalsRow >= 0 {
		for i := range stores {
			s := &stores[i]
			if s.totalCol < 0 || cellAt(rows, totalsRow, s.qtyCol) == "" {
				continue
			}
			s.order.PrintedQty = numberAt(rows, totalsRow, s.qtyCol)
			s.order.PrintedAmount = numberAt(rows, totalsRow, s.totalCol)
			s.order.HasPrintedTotals = true
		}
	}

	for r := header + 2; r < len(rows); r++ {
		barcode := cellAt(rows, r, 0)
		if !barcodePattern.MatchString(barcode) {
			continue
		}
		line := WorkbookLine{
			Barcode:    barcode,
			Name:       cellAt(rows, r, nameCol),
			Price:      numberAt(rows, r, priceCol),
			PromoPrice: numberAt(rows, r, promoPriceCol),
			PromoText:  strings.TrimSpace(whitespacePattern.ReplaceAllString(cellAt(rows, r, promoTextCol), " ")),
			PromoStart: promoStart(cellAt(rows, r, promoDateCol)),
		}
		for i := range stores {
			s := &stores[i]
			qty := numberAt(rows, r, s.qtyCol)
			if qty <= 0 {
				continue
			}
			storeLine := line
			storeLine.Qty = qty
			storeLine.Total = numberAt(rows, r, s.totalCol)
			s.order.Lines = append(s.order.Lines, storeLine)
		}
	}

	wb := Workbook{EntryDate: savedDate(f)}
	for _, s := range stores {
		if len(s.order.Lines) > 0 {
			wb.Orders = append(wb.Orders, s.order)
		}
	}
	return wb, nil
}

func promoStart(cell string) string {
	m := promoRangePattern.FindStringSubmatch(cell)
	if m == nil {
		return ""
	}
	day, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	year := m[3]
	if year == "" {
		year = strconv.Itoa(time.Now().In(vietnamTime).Year())
	}
	return fmt.Sprintf("%02d/%02d/%s", day, month, year)
}

// savedDate returns the workbook's last-saved day, falling back to its
// creation day and then to today for a file that records neither.
func savedDate(f *excelize.File) string {
	if props, err := f.GetDocProps(); err == nil && props != nil {
		for _, stamp := range []string{props.Modified, props.Created} {
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(stamp)); err == nil {
				return t.In(vietnamTime).Format("02/01/2006")
			}
		}
	}
	return time.Now().In(vietnamTime).Format("02/01/2006")
}
