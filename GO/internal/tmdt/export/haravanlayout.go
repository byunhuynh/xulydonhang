package export

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/tmdt/haravan"
	"order-processor/internal/tmdt/lookup"
)

// Layout "haravan": mô phỏng đúng file Excel mà trang quản trị Haravan xuất ra —
// một dòng cho mỗi dòng hàng, 75 cột A..BW, tên sheet "Đơn hàng haravan".
//
// Vì sao phải giữ nguyên cả 75 cột thay vì gom 12 cột lại cho gọn: file
// "XUẤT HÀNG HN-LA MỚI.xlsx" có các cột công thức BX..CH tham chiếu **theo chữ
// cái cột** (`DATEVALUE(LEFT(Q2,10))`, `$T2&$V2`, `SEARCH(...,BB2)`) và theo tên
// cột của Table2 (`Table2[Mã sản phẩm]`, `Table2[Shop]`). Gom cột lại là gãy hết
// công thức. Nên tool điền đúng 12 cột được yêu cầu và để trống phần còn lại.
const sheetHaravan = "Đơn hàng haravan"

// Tiêu đề lấy nguyên văn từ file Haravan xuất ra.
var haravanHeaders = []string{
	"Mã đơn hàng", "Email", "Tình trạng thanh toán",
	"Thời gian thanh toán", "Người xác nhận thanh toán", "Tình trạng giao hàng",
	"Thời gian giao hàng", "Nhận Email quảng cáo", "Tiền tệ",
	"Tổng tiền", "Phí vận chuyển", "Taxes",
	"Tổng cộng", "Mã khuyến mãi", "Số tiền giảm",
	"Phương thức vận chuyển", "Ngày đặt hàng", "Người tạo",
	"Số lượng sản phẩm", "Tên sản phẩm", "Thuộc tính 1",
	"Giá trị thuộc tính 1", "Thuộc tính 2", "Giá trị thuộc tính 2",
	"Thuộc tính 3", "Giá trị thuộc tính 3", "Giá sản phẩm",
	"Giá so sánh sản phẩm", "Mã sản phẩm", "Yêu cầu giao hàng",
	"Lineitem taxable", "Tình trạng giao hàng của sản sản phẩm", "Tên người thanh toán",
	"Billing Street", "Địa chỉ thanh toán", "Billing Address2",
	"Billing Company", "Billing Zip", "Tỉnh/Thành phố thanh toán",
	"Quốc gia", "Số điện thoại thanh toán", "Tên người nhận",
	"Shipping Street", "Địa chỉ nhận hàng", "Shipping Address2",
	"Shipping Company", "Shipping Zip", "Phường/Xã nhận hàng",
	"Quận/Huyện nhận hàng", "Tỉnh/Thành phố nhận hàng", "Quốc gia2",
	"Số điện thoại", "Ghi chú", "Thuộc tính",
	"VAT", "Phương thức thanh toán", "Số tiền hoàn trả",
	"Hãng", "Id", "Tags",
	"Risk Level", "Người xác thực", "Ngày xác thực",
	"Ngày lưu trữ", "Trạng thái lưu trữ", "Thời gian hủy",
	"Trạng thái hủy", "Lý do hủy", "Ghi chú hủy",
	"Kho bán", "Kênh bán hàng", "Chương trình khách hàng thân thiết",
	"Loại chương trình chiết khấu khách hàng thân thiết", "Chiết khấu theo hạng thành viên (%)",
	"Số tiền chiết khấu cho khách hàng thân thiết",
}

// Chỉ số 0-based của 12 cột được yêu cầu.
const (
	colMaDonHang   = 0  // A
	colTongTien    = 9  // J
	colTongCong    = 12 // M
	colNgayDatHang = 16 // Q
	colSoLuong     = 18 // S
	colTenSanPham  = 19 // T
	colGiaTriTT1   = 21 // V
	colGiaSanPham  = 26 // AA
	colMaSanPham   = 28 // AC
	colThuocTinh   = 53 // BB
	colKhoBan      = 69 // BR
	colKenhBanHang = 70 // BS
)

// HaravanWriter ghi file theo đúng bố cục Haravan xuất ra.
type HaravanWriter struct {
	path  string
	f     *excelize.File
	sw    *excelize.StreamWriter
	row   int
	count int
}

func NewHaravanWriter(path string) (*HaravanWriter, error) {
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
		Font: &excelize.Font{Bold: true},
	})
	if err != nil {
		return nil, err
	}
	for _, c := range []struct {
		col   int
		width float64
	}{
		{colMaDonHang + 1, 22}, {colTongTien + 1, 12}, {colTongCong + 1, 12},
		{colNgayDatHang + 1, 26}, {colSoLuong + 1, 10}, {colTenSanPham + 1, 50},
		{colGiaTriTT1 + 1, 34}, {colGiaSanPham + 1, 12}, {colMaSanPham + 1, 16},
		{colThuocTinh + 1, 40}, {colKhoBan + 1, 22}, {colKenhBanHang + 1, 14},
	} {
		if err := sw.SetColWidth(c.col, c.col, c.width); err != nil {
			return nil, err
		}
	}
	if err := sw.SetPanes(&excelize.Panes{
		Freeze: true, Split: false, XSplit: 0, YSplit: 1,
		TopLeftCell: "A2", ActivePane: "bottomLeft",
	}); err != nil {
		return nil, err
	}
	cells := make([]any, len(haravanHeaders))
	for i, h := range haravanHeaders {
		cells[i] = excelize.Cell{StyleID: headerStyle, Value: h}
	}
	if err := sw.SetRow("A1", cells); err != nil {
		return nil, err
	}
	return &HaravanWriter{path: path, f: f, sw: sw, row: 1}, nil
}

// AddOrder ghi mỗi dòng hàng của đơn thành một dòng Excel; các trường cấp đơn
// được lặp lại trên từng dòng, đúng như file Haravan xuất ra.
func (w *HaravanWriter) AddOrder(_ string, o *haravan.Order) error {
	items := o.LineItems
	if len(items) == 0 {
		// Đơn không có dòng hàng vẫn giữ lại một dòng để không mất đơn.
		items = []haravan.LineItem{{}}
	}
	for i := range items {
		li := &items[i]
		row := make([]any, len(haravanHeaders))
		row[colMaDonHang] = firstNonEmpty(o.Name, o.OrderNumber)
		row[colTongTien] = o.SubtotalPrice.Float()
		row[colTongCong] = o.TotalPrice.Float()
		row[colNgayDatHang] = orderDateVN(o)
		row[colSoLuong] = li.Quantity
		row[colTenSanPham] = li.Title
		row[colGiaTriTT1] = li.VariantTitle
		row[colGiaSanPham] = li.Price.Float()
		row[colMaSanPham] = li.SKU
		row[colThuocTinh] = noteAttributesText(o)
		row[colKhoBan] = o.LocationName
		row[colKenhBanHang] = o.SourceName

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

func (w *HaravanWriter) Count() int { return w.count }

func (w *HaravanWriter) Close() error {
	defer w.f.Close()
	if err := w.sw.Flush(); err != nil {
		return err
	}
	return w.f.SaveAs(w.path)
}

// orderDateVN trả về chuỗi ISO 8601 kèm offset +07:00, y hệt cột Q của file
// Haravan xuất ra ("2026-08-23T23:56:56+07:00"). Để dạng chuỗi vì công thức
// BX dùng LEFT/MID cắt chuỗi này.
func orderDateVN(o *haravan.Order) string {
	if o.CreatedAt.IsZero() {
		return ""
	}
	return o.CreatedAt.InVN().Format("2006-01-02T15:04:05-07:00")
}

// noteAttributesText dựng lại cột BB "Thuộc tính": mỗi dòng là "Tên : Giá trị",
// nối bằng xuống dòng — đúng định dạng mà công thức CG (SEARCH + CHAR(10)) mong đợi.
func noteAttributesText(o *haravan.Order) string {
	parts := make([]string, 0, len(o.NoteAttributes))
	for _, a := range o.NoteAttributes {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s : %s", name, a.StringValue()))
	}
	// API trả note_attributes theo thứ tự không ổn định; file Haravan xuất ra lại
	// sắp theo tên. Sắp lại để trùng khít file mẫu và ổn định giữa các lần chạy.
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

// OrderWriter là bộ ghi đơn ra Excel; hai bố cục cùng thoả giao diện này.
type OrderWriter interface {
	AddOrder(channel string, o *haravan.Order) error
	Count() int
	Close() error
}

// Các bố cục hỗ trợ.
const (
	FormatStandard = "chuan"   // file hoàn chỉnh: 12 cột Haravan + 11 cột đã tính sẵn, không công thức
	FormatHaravan  = "haravan" // giống hệt file Haravan xuất ra, chỉ điền 12 cột
	FormatFull     = "full"    // 3 sheet tự thiết kế: DonHang / ChiTietSanPham / TongHop
)

// NewOrderWriter tạo bộ ghi theo bố cục yêu cầu. tables chỉ cần cho bố cục "chuan".
func NewOrderWriter(format, path string, tables *lookup.Tables) (OrderWriter, error) {
	switch format {
	case FormatStandard, "":
		if tables == nil {
			return nil, fmt.Errorf("bố cục %q cần bảng tra cứu (xem cờ -mapping)", FormatStandard)
		}
		return NewStandardWriter(path, tables)
	case FormatHaravan:
		return NewHaravanWriter(path)
	case FormatFull:
		return NewWriter(path)
	default:
		return nil, fmt.Errorf("-format %q không hợp lệ (chỉ nhận %q, %q hoặc %q)",
			format, FormatStandard, FormatHaravan, FormatFull)
	}
}
