package export

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/tmdt/haravan"
	"order-processor/internal/tmdt/lookup"
)

// Bố cục "chuẩn": chỉ những cột thực sự dùng, xếp liền nhau — 12 cột dữ liệu lấy
// từ Haravan cộng 11 cột trước đây do công thức Excel sinh ra, nay Go tính sẵn.
//
// Bản trước còn giữ đủ 75 cột A..BW của file Haravan (phần lớn để trống) vì các
// công thức trong workbook cũ tham chiếu theo chữ cái cột (`$T2&$V2`, `BB2`...).
// Không còn công thức nữa thì ràng buộc đó cũng hết, nên cắt luôn cho gọn.
var standardHeaders = []string{
	"Mã đơn hàng",          // A
	"Tổng tiền",            // B
	"Tổng cộng",            // C
	"Ngày đặt hàng",        // D
	"Số lượng sản phẩm",    // E
	"Tên sản phẩm",         // F
	"Giá trị thuộc tính 1", // G
	"Giá sản phẩm",         // H
	"Mã sản phẩm",          // I
	"Thuộc tính",           // J
	"Kho bán",              // K
	"Kênh bán hàng",        // L
	"Thời gian Đặt",        // M
	"MÃ TP 1",              // N
	"SLTP1",                // O
	"MÃ TP 2",              // P
	"SLTP2",                // Q
	"MÃ TP 3",              // R
	"SLTP3",                // S
	"MÃ TP 4",              // T
	"SLTP4",                // U
	"Shop",                 // V
	"Mã misa",              // W
}

const (
	sColMaDonHang = iota
	sColTongTien
	sColTongCong
	sColNgayDatHang
	sColSoLuong
	sColTenSanPham
	sColGiaTriTT1
	sColGiaSanPham
	sColMaSanPham
	sColThuocTinh
	sColKhoBan
	sColKenhBanHang
	sColThoiGianDat
	sColMaTP1 // 8 cột MÃ TP / SLTP xen kẽ bắt đầu từ đây
	_
	_
	_
	_
	_
	_
	_
	sColShop
	sColMaMisa
	standardCols
)

var standardWidths = []float64{
	22, 12, 12, 26, 10, 50, 34, 12, 16, 40, 22, 14,
	16, 14, 8, 14, 8, 14, 8, 14, 8, 24, 18,
}

// Shop này không quy đổi ra mã thành phẩm — công thức cũ trả về rỗng cho toàn bộ
// cột MÃ TP / SLTP, giữ nguyên quy tắc đó.
const shopKhongQuyDoi = "CLEVY VIỆT NAM"

// StandardWriter ghi file hoàn chỉnh: dữ liệu Haravan + các cột đã tính sẵn.
type StandardWriter struct {
	path   string
	f      *excelize.File
	sw     *excelize.StreamWriter
	tables *lookup.Tables

	dateStyle  int
	moneyStyle int

	row   int
	count int

	// Thống kê để báo lại cho người dùng những dòng chưa khai báo trong bảng tra cứu.
	MissingCombo map[string]int
	MissingShop  map[string]int
}

func NewStandardWriter(path string, tables *lookup.Tables) (*StandardWriter, error) {
	f := excelize.NewFile()
	if _, err := f.NewSheet(sheetHaravan); err != nil {
		return nil, err
	}
	f.DeleteSheet("Sheet1")

	sw, err := f.NewStreamWriter(sheetHaravan)
	if err != nil {
		return nil, err
	}
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"1F4E79"}},
		Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "center", WrapText: true},
	})
	if err != nil {
		return nil, err
	}
	// Cột "Thời gian Đặt" giữ đúng định dạng hiển thị của file cũ (mm-dd-yy),
	// nhưng là giá trị ngày thật nên vẫn lọc/sắp xếp được.
	dateFmt := "mm-dd-yy"
	dateStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &dateFmt})
	if err != nil {
		return nil, err
	}
	moneyFmt := "#,##0"
	moneyStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &moneyFmt})
	if err != nil {
		return nil, err
	}

	for i, width := range standardWidths {
		if err := sw.SetColWidth(i+1, i+1, width); err != nil {
			return nil, err
		}
	}
	if err := sw.SetPanes(&excelize.Panes{
		Freeze: true, Split: false, XSplit: 0, YSplit: 1,
		TopLeftCell: "A2", ActivePane: "bottomLeft",
	}); err != nil {
		return nil, err
	}

	cells := make([]any, standardCols)
	for i, h := range standardHeaders {
		cells[i] = excelize.Cell{StyleID: headerStyle, Value: h}
	}
	if err := sw.SetRow("A1", cells, excelize.RowOpts{Height: 26}); err != nil {
		return nil, err
	}

	return &StandardWriter{
		path: path, f: f, sw: sw, tables: tables, row: 1,
		dateStyle: dateStyle, moneyStyle: moneyStyle,
		MissingCombo: map[string]int{}, MissingShop: map[string]int{},
	}, nil
}

func (w *StandardWriter) AddOrder(_ string, o *haravan.Order) error {
	items := o.LineItems
	if len(items) == 0 {
		items = []haravan.LineItem{{}}
	}
	shop := haravan.ShopName(o)
	attrs := noteAttributesText(o)
	orderDate := orderDateVN(o)
	orderCode := firstNonEmpty(o.Name, o.OrderNumber)
	misa, misaOK := w.tables.MisaCode(shop)
	if !misaOK {
		misa = lookup.NotAvailable
		w.MissingShop[shop]++
	}

	for i := range items {
		li := &items[i]
		row := make([]any, standardCols)

		row[sColMaDonHang] = orderCode
		row[sColTongTien] = w.money(o.SubtotalPrice.Float())
		row[sColTongCong] = w.money(o.TotalPrice.Float())
		row[sColNgayDatHang] = orderDate
		row[sColSoLuong] = li.Quantity
		row[sColTenSanPham] = li.Title
		row[sColGiaTriTT1] = li.VariantTitle
		row[sColGiaSanPham] = w.money(li.Price.Float())
		row[sColMaSanPham] = li.SKU
		row[sColThuocTinh] = attrs
		row[sColKhoBan] = o.LocationName
		row[sColKenhBanHang] = o.SourceName

		if !o.CreatedAt.IsZero() {
			row[sColThoiGianDat] = excelize.Cell{StyleID: w.dateStyle, Value: o.CreatedAt.InVN()}
		}
		w.fillComponents(row, shop, li)
		row[sColShop] = shop
		row[sColMaMisa] = misa

		w.row++
		cell, err := excelize.CoordinatesToCellName(1, w.row)
		if err != nil {
			return err
		}
		if err := w.sw.SetRow(cell, row); err != nil {
			return err
		}
	}
	w.count++
	return nil
}

func (w *StandardWriter) money(v float64) excelize.Cell {
	return excelize.Cell{StyleID: w.moneyStyle, Value: v}
}

// fillComponents điền MÃ TP 1..4 / SLTP1..4 — thay cho VLOOKUP trong file cũ.
// Có Mã sản phẩm thì tra theo mã; không có thì tra theo Tên sản phẩm + Phân loại.
func (w *StandardWriter) fillComponents(row []any, shop string, li *haravan.LineItem) {
	if strings.EqualFold(strings.TrimSpace(shop), shopKhongQuyDoi) {
		return
	}

	sku := strings.TrimSpace(li.SKU)
	var (
		combo *lookup.ComboRow
		ok    bool
	)
	if sku == "" {
		combo, ok = w.tables.ByProductVariant(li.Title, li.VariantTitle)
	} else {
		combo, ok = w.tables.ByCombo(sku)
	}
	if !ok {
		for i := 0; i < 8; i++ {
			row[sColMaTP1+i] = lookup.NotAvailable
		}
		w.MissingCombo[missingKey(sku, li)]++
		return
	}

	for i := 0; i < 4; i++ {
		row[sColMaTP1+i*2] = blankIfZero(combo.TP[i])
		if n, err := strconv.Atoi(combo.SL[i]); err == nil {
			if n != 0 {
				row[sColMaTP1+i*2+1] = n
			}
		} else {
			row[sColMaTP1+i*2+1] = blankIfZero(combo.SL[i])
		}
	}
}

func missingKey(sku string, li *haravan.LineItem) string {
	if sku != "" {
		return "Mã sản phẩm " + sku
	}
	return li.Title + " | " + li.VariantTitle
}

// Công thức cũ bọc kết quả trong IF(KQ=0,"",KQ): giá trị 0 hiển thị thành rỗng.
func blankIfZero(s string) any {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return ""
	}
	return s
}

func (w *StandardWriter) Count() int { return w.count }

func (w *StandardWriter) Close() error {
	defer w.f.Close()
	if w.row >= 2 {
		lastCol, err := excelize.ColumnNumberToName(standardCols)
		if err != nil {
			return err
		}
		showStripes := false
		if err := w.sw.AddTable(&excelize.Table{
			Range:          fmt.Sprintf("A1:%s%d", lastCol, w.row),
			Name:           "DonHang",
			StyleName:      "TableStyleLight1",
			ShowRowStripes: &showStripes,
		}); err != nil {
			return err
		}
	}
	if err := w.sw.Flush(); err != nil {
		return err
	}
	return w.f.SaveAs(w.path)
}

// Warnings liệt kê những gì chưa khai báo trong bảng tra cứu, để người dùng bổ sung.
func (w *StandardWriter) Warnings() []string {
	var out []string
	for shop, c := range w.MissingShop {
		out = append(out, fmt.Sprintf("shop %q chưa có trong sheet %q (%d dòng → Mã misa = %s)",
			shop, lookup.SheetMisa, c, lookup.NotAvailable))
	}
	for k, c := range w.MissingCombo {
		out = append(out, fmt.Sprintf("chưa có trong sheet %q: %s (%d dòng → MÃ TP = %s)",
			lookup.SheetDataShop, k, c, lookup.NotAvailable))
	}
	return out
}
