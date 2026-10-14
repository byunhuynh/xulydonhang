// Package osifood parses OsiFood purchase orders ("ĐƠN ĐẶT HÀNG NCC"),
// which are an Excel sheet printed to PDF.
//
// Like maxidi, this package has NO counterpart in xulydonhang.py —
// OsiFood was never implemented in the old Python app. The shapes parsed
// here were derived from the one real order available when it was written
// (store 68014, 12/10/2026), via this repo's own extractPageTexts.
//
// Three properties of that extracted text drive the decisions below:
//
//  1. Every spreadsheet cell comes out as its own line, padded with the
//     cell's own leading/trailing spaces, and a cell that wraps on the
//     page ("Mua 1 tặng 1 / (Quấn kèm)") comes out as several lines. Every
//     separator is therefore \s+ rather than a newline or a fixed offset.
//
//  2. The column headers are emitted AFTER the table, at the very end of
//     the page, so they cannot be used to anchor the rows. Rows are found
//     by their own shape instead (a 13-digit barcode followed by the
//     numeric columns), and the printed TOTAL row is returned alongside
//     them so the caller can prove that shape-matching read every row.
//
//  3. The order carries no PO number at all — only the store code and the
//     date it was placed. See OrderInfo.PONumber.
package osifood

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	whitespacePattern = regexp.MustCompile(`\s+`)

	storePattern     = regexp.MustCompile(`STORE\s+(\d+)`)
	entryDatePattern = regexp.MustCompile(`Ngày đặt hàng\s*:\s*(\d{1,2})/(\d{1,2})/(\d{4})`)
	deadlinePattern  = regexp.MustCompile(`Hạn chót CH nhận đơn ngày\s*:\s*(\d{1,2}/\d{1,2}/\d{4})`)
	// The address cell wraps across lines on the page; its label is the
	// two-line "ĐỊA CHỈ / GIAO HÀNG" cell and the next label after it is
	// the receiving deadline.
	addressPattern = regexp.MustCompile(`(?s)GIAO HÀNG\s+(.+?)\s*Hạn chót CH nhận đơn`)
	// supplierPattern reads the supplier code OsiFood assigned to us,
	// printed in the order's own title ("ĐƠN ĐẶT HÀNG NCC 00758 ...").
	supplierPattern = regexp.MustCompile(`ĐƠN ĐẶT HÀNG NCC\s+(\d+)`)

	// productPattern reads one product row: barcode, OsiFood's own item
	// code, name, carton pack size, list cost, promotional cost, promotion
	// text, loose-unit quantity, carton quantity and line total, in the
	// order the page emits them.
	//
	// The two quantity columns and the pack size always print two decimals
	// ("8.00") while the money columns never do ("110,501"), and that
	// difference is what lets the optional columns in between be told
	// apart: the promotional cost is "-" or absent when the line has no
	// discounted price, and the promotion text is free-form and may be
	// empty.
	productPattern = regexp.MustCompile(`(?s)(\d{13})\s+(\S+)\s+(.+?)\s+([\d,]+\.\d{2})\s+([\d,]+(?:\.\d+)?)\s+(?:(-|[\d,]+(?:\.\d+)?)\s+)?(.*?)\s*([\d,]+\.\d{2})\s+([\d,]+\.\d{2})\s+([\d,]+(?:\.\d+)?)\s`)

	// totalsPattern reads the table's own TOTAL row: loose units, cartons
	// and order value.
	totalsPattern = regexp.MustCompile(`TOTAL\s+([\d,.]+)\s+([\d,.]+)\s+([\d,.]+)`)
)

// OrderInfo is everything the page header carries about the order.
type OrderInfo struct {
	// PONumber is NOT printed on the order: OsiFood orders have no number
	// of their own, so one is built from the store code and the order
	// date ("68014-121026"), as decided by the project owner.
	PONumber        string
	StoreCode       string
	SupplierCode    string
	EntryDate       string
	Deadline        string
	DeliveryAddress string
}

// PONumber builds the order number OsiFood itself never prints, from the
// store code and the order date (DD/MM/YYYY): "68014-121026".
func PONumber(storeCode, entryDate string) string {
	parts := strings.Split(entryDate, "/")
	if len(parts) != 3 || len(parts[2]) < 2 {
		return storeCode
	}
	return fmt.Sprintf("%s-%s%s%s", storeCode, parts[0], parts[1], parts[2][len(parts[2])-2:])
}

// ParseOrderInfo extracts the page header. Returns ok=false unless the
// store code, order date, receiving deadline and delivery address can all
// be read.
func ParseOrderInfo(text string) (OrderInfo, bool) {
	store := storePattern.FindStringSubmatch(text)
	entry := entryDatePattern.FindStringSubmatch(text)
	deadline := deadlinePattern.FindStringSubmatch(text)
	address := addressPattern.FindStringSubmatch(text)
	if store == nil || entry == nil || deadline == nil || address == nil {
		return OrderInfo{}, false
	}

	day, _ := strconv.Atoi(entry[1])
	month, _ := strconv.Atoi(entry[2])
	info := OrderInfo{
		StoreCode:       store[1],
		EntryDate:       fmt.Sprintf("%02d/%02d/%s", day, month, entry[3]),
		Deadline:        deadline[1],
		DeliveryAddress: strings.TrimSpace(whitespacePattern.ReplaceAllString(address[1], " ")),
	}
	info.PONumber = PONumber(info.StoreCode, info.EntryDate)
	if m := supplierPattern.FindStringSubmatch(text); m != nil {
		info.SupplierCode = m[1]
	}
	if info.DeliveryAddress == "" {
		return OrderInfo{}, false
	}
	return info, true
}

// Product is one line of the order's product table, every field exactly
// as printed.
//
// Qty is the LOOSE-UNIT count ("LƯỢNG LẺ"), the quantity both prices are
// quoted per. PromoPrice ("GIÁ VỐN KM") is "" or "-" when the line has no
// discounted price. Both printed prices are rounded to whole đồng, so
// Total ÷ Qty is the only exact unit price on the page.
//
// Name and Code are captured but not used downstream — names and item
// codes in the order workbook come from the SanPham sheet — and are kept
// because they make a misaligned parse obvious in a test failure.
type Product struct {
	Barcode    string
	Code       string
	Name       string
	PackSize   string
	Price      string
	PromoPrice string
	PromoText  string
	Qty        string
	Cartons    string
	Total      string
}

// Totals is the table's printed TOTAL row.
type Totals struct {
	Qty     string
	Cartons string
	Amount  string
}

// tableRegion returns the part of the page between the order title and
// the TOTAL row, which is where the product rows are.
func tableRegion(text string) (table string, totalIdx int) {
	start := 0
	if loc := supplierPattern.FindStringIndex(text); loc != nil {
		start = loc[1]
	}
	totalIdx = strings.LastIndex(text, "TOTAL")
	if totalIdx < start {
		return text[start:], -1
	}
	return text[start:totalIdx], totalIdx
}

// ExtractProducts reads every product row on the page.
func ExtractProducts(text string) []Product {
	table, _ := tableRegion(text)

	var products []Product
	for _, m := range productPattern.FindAllStringSubmatch(table, -1) {
		products = append(products, Product{
			Barcode:    m[1],
			Code:       m[2],
			Name:       strings.TrimSpace(whitespacePattern.ReplaceAllString(m[3], " ")),
			PackSize:   m[4],
			Price:      m[5],
			PromoPrice: m[6],
			PromoText:  strings.TrimSpace(whitespacePattern.ReplaceAllString(m[7], " ")),
			Qty:        m[8],
			Cartons:    m[9],
			Total:      m[10],
		})
	}
	return products
}

// ParseTotals reads the printed TOTAL row. Returns ok=false when the page
// has none.
func ParseTotals(text string) (Totals, bool) {
	_, totalIdx := tableRegion(text)
	if totalIdx < 0 {
		return Totals{}, false
	}
	m := totalsPattern.FindStringSubmatch(text[totalIdx:])
	if m == nil {
		return Totals{}, false
	}
	return Totals{Qty: m[1], Cartons: m[2], Amount: m[3]}, true
}
