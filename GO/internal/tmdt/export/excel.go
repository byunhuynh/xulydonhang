// Package export ghi đơn hàng ra file Excel.
//
// Dùng StreamWriter của excelize và nhận đơn theo kiểu streaming (AddOrder gọi
// dần khi từng trang API về) nên bộ nhớ gần như không đổi dù xuất vài chục nghìn
// đơn — store thật có thể có 20k+ đơn mỗi tháng.
package export

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/tmdt/haravan"
)

const (
	sheetOrders = "DonHang"
	sheetItems  = "ChiTietSanPham"
	sheetStats  = "TongHop"
)

type Row struct {
	Order   *haravan.Order
	Channel string
}

var orderHeaders = []string{
	"Sàn", "Tên shop", "Mã đơn", "Mã đơn trên sàn", "Order ID",
	"Ngày đặt hàng", "Ngày cập nhật", "source_name", "Tags",
	"Trạng thái đơn", "Thanh toán", "Giao hàng", "Lý do huỷ",
	"Khách hàng", "SĐT", "Email",
	"Người nhận", "SĐT nhận", "Địa chỉ giao",
	"Tiền hàng", "Giảm giá", "Phí vận chuyển", "Thuế", "Tổng tiền", "Tiền tệ",
	"Cổng thanh toán", "Số SP",
	"Dịch vụ vận chuyển", "Mã vận đơn", "Đơn vị vận chuyển", "Kho", "ID shop sàn", "Ghi chú",
}

var orderWidths = []float64{
	14, 24, 20, 20, 14, 17, 17, 16, 20, 14, 14, 14, 14,
	22, 14, 26, 22, 14, 46, 14, 12, 14, 10, 14, 8, 20, 8,
	18, 20, 20, 22, 22, 30,
}

var itemHeaders = []string{
	"Sàn", "Tên shop", "Mã đơn", "Mã đơn trên sàn", "Ngày đặt hàng",
	"Tên sản phẩm", "Mã sản phẩm (SKU)", "Barcode", "Thuộc tính",
	"Số lượng", "Giá bán", "Giá gốc", "Giảm giá", "Thành tiền",
	"Trạng thái giao", "Nhà cung cấp", "Product ID", "Variant ID",
}

var itemWidths = []float64{
	14, 24, 20, 20, 17, 42, 20, 18, 34, 10, 14, 14, 12, 16, 16, 18, 14, 14,
}

// channelShop là khoá gộp thống kê: một sàn có thể có nhiều shop.
type channelShop struct {
	channel string
	shop    string
}

type channelAgg struct {
	channel string
	shop    string
	orders  int
	items   int
	revenue float64
}

// Writer ghi dần đơn hàng ra file Excel.
type Writer struct {
	path string
	f    *excelize.File

	swOrders *excelize.StreamWriter
	swItems  *excelize.StreamWriter

	orderRow int // dòng cuối đã ghi ở sheet DonHang
	itemRow  int // dòng cuối đã ghi ở sheet ChiTietSanPham

	headerStyle int
	moneyStyle  int
	dateStyle   int

	byChannel map[channelShop]*channelAgg
	seq       []channelShop
}

// NewWriter tạo file Excel rỗng với 3 sheet và ghi sẵn dòng tiêu đề.
func NewWriter(path string) (*Writer, error) {
	f := excelize.NewFile()
	w := &Writer{path: path, f: f, byChannel: map[channelShop]*channelAgg{}}

	var err error
	if w.headerStyle, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"1F4E79"}},
		Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "center", WrapText: true},
	}); err != nil {
		return nil, err
	}
	moneyFmt := "#,##0"
	if w.moneyStyle, err = f.NewStyle(&excelize.Style{CustomNumFmt: &moneyFmt}); err != nil {
		return nil, err
	}
	dateFmt := "dd/mm/yyyy hh:mm"
	if w.dateStyle, err = f.NewStyle(&excelize.Style{CustomNumFmt: &dateFmt}); err != nil {
		return nil, err
	}

	for _, s := range []string{sheetOrders, sheetItems, sheetStats} {
		if _, err := f.NewSheet(s); err != nil {
			return nil, err
		}
	}
	f.DeleteSheet("Sheet1")

	if w.swOrders, err = f.NewStreamWriter(sheetOrders); err != nil {
		return nil, err
	}
	if w.swItems, err = f.NewStreamWriter(sheetItems); err != nil {
		return nil, err
	}

	if err := w.initSheet(w.swOrders, orderHeaders, orderWidths); err != nil {
		return nil, err
	}
	if err := w.initSheet(w.swItems, itemHeaders, itemWidths); err != nil {
		return nil, err
	}
	w.orderRow, w.itemRow = 1, 1
	return w, nil
}

func (w *Writer) initSheet(sw *excelize.StreamWriter, headers []string, widths []float64) error {
	for i, width := range widths {
		if i >= len(headers) {
			break
		}
		if err := sw.SetColWidth(i+1, i+1, width); err != nil {
			return err
		}
	}
	if err := sw.SetPanes(&excelize.Panes{
		Freeze: true, Split: false, XSplit: 0, YSplit: 1,
		TopLeftCell: "A2", ActivePane: "bottomLeft",
	}); err != nil {
		return err
	}
	cells := make([]any, len(headers))
	for i, h := range headers {
		cells[i] = excelize.Cell{StyleID: w.headerStyle, Value: h}
	}
	return sw.SetRow("A1", cells, excelize.RowOpts{Height: 24})
}

// AddOrder ghi một đơn vào sheet DonHang và các dòng sản phẩm vào ChiTietSanPham.
func (w *Writer) AddOrder(channel string, o *haravan.Order) error {
	tracking, carrier := trackingInfo(o)
	qty := 0
	for _, li := range o.LineItems {
		qty += li.Quantity
	}
	marketplaceCode := haravan.MarketplaceOrderCode(o)
	haravanCode := firstNonEmpty(o.Name, o.OrderNumber)
	shopName := haravan.ShopName(o)
	shippingService, shippingFee := haravan.ShippingService(o)

	w.orderRow++
	cell, err := excelize.CoordinatesToCellName(1, w.orderRow)
	if err != nil {
		return err
	}
	row := []any{
		channel,
		shopName,
		haravanCode,
		marketplaceCode,
		o.ID,
		w.dateCell(o.CreatedAt),
		w.dateCell(o.UpdatedAt),
		o.SourceName,
		o.Tags,
		orderState(o),
		o.FinancialStatus,
		o.FulfillmentStatus,
		o.CancelReason,
		o.Customer.FullName(),
		firstNonEmpty(o.Phone, custPhone(o), addrPhone(o.ShippingAddress)),
		firstNonEmpty(o.Email, o.ContactEmail, custEmail(o)),
		o.ShippingAddress.Receiver(),
		addrPhone(o.ShippingAddress),
		o.ShippingAddress.Full(),
		w.moneyCell(o.SubtotalPrice.Float()),
		w.moneyCell(o.TotalDiscounts.Float()),
		w.moneyCell(shippingFee),
		w.moneyCell(o.TotalTax.Float()),
		w.moneyCell(o.TotalPrice.Float()),
		o.Currency,
		firstNonEmpty(o.Gateway, o.GatewayCode),
		qty,
		shippingService,
		tracking,
		carrier,
		o.LocationName,
		haravan.ShopID(o),
		o.Note,
	}
	if err := w.swOrders.SetRow(cell, row); err != nil {
		return err
	}

	for _, li := range o.LineItems {
		w.itemRow++
		cell, err := excelize.CoordinatesToCellName(1, w.itemRow)
		if err != nil {
			return err
		}
		line := li.Price.Float()*float64(li.Quantity) - li.TotalDiscount.Float()
		if err := w.swItems.SetRow(cell, []any{
			channel,
			shopName,
			haravanCode,
			marketplaceCode,
			w.dateCell(o.CreatedAt),
			firstNonEmpty(li.Title, li.Name),
			li.SKU,
			li.Barcode,
			haravan.LineItemAttributes(&li),
			li.Quantity,
			w.moneyCell(li.Price.Float()),
			w.moneyCell(li.PriceOriginal.Float()),
			w.moneyCell(li.TotalDiscount.Float()),
			w.moneyCell(line),
			li.FulfillmentStatus,
			li.Vendor,
			li.ProductID,
			li.VariantID,
		}); err != nil {
			return err
		}
	}

	key := channelShop{channel: channel, shop: shopName}
	a, ok := w.byChannel[key]
	if !ok {
		a = &channelAgg{channel: channel, shop: shopName}
		w.byChannel[key] = a
		w.seq = append(w.seq, key)
	}
	a.orders++
	a.items += qty
	a.revenue += o.TotalPrice.Float()
	return nil
}

// Count trả về số đơn đã ghi.
func (w *Writer) Count() int { return w.orderRow - 1 }

// Close ghi sheet tổng hợp, bật bộ lọc rồi lưu file.
func (w *Writer) Close() error {
	defer w.f.Close()

	// AddTable phải gọi sau khi ghi hết dòng và trước Flush; nó cũng chính là
	// thứ tạo nút lọc trên dòng tiêu đề ở chế độ stream.
	if err := addFilterTable(w.swOrders, "Orders", len(orderHeaders), w.orderRow); err != nil {
		return err
	}
	if err := addFilterTable(w.swItems, "LineItems", len(itemHeaders), w.itemRow); err != nil {
		return err
	}
	if err := w.swOrders.Flush(); err != nil {
		return err
	}
	if err := w.swItems.Flush(); err != nil {
		return err
	}
	if err := w.writeStats(); err != nil {
		return err
	}
	if idx, err := w.f.GetSheetIndex(sheetOrders); err == nil {
		w.f.SetActiveSheet(idx)
	}
	return w.f.SaveAs(w.path)
}

func addFilterTable(sw *excelize.StreamWriter, name string, cols, lastRow int) error {
	if lastRow < 2 {
		return nil // bảng cần ít nhất tiêu đề + 1 dòng dữ liệu
	}
	lastCol, err := excelize.ColumnNumberToName(cols)
	if err != nil {
		return err
	}
	showStripes := false
	return sw.AddTable(&excelize.Table{
		Range:          fmt.Sprintf("A1:%s%d", lastCol, lastRow),
		Name:           name,
		StyleName:      "TableStyleLight1",
		ShowRowStripes: &showStripes,
	})
}

// Sheet TongHop nhỏ nên ghi bằng API thường.
func (w *Writer) writeStats() error {
	headers := []string{"Sàn", "Tên shop", "Số đơn", "Số sản phẩm", "Doanh thu"}
	for i, h := range headers {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return err
		}
		if err := w.f.SetCellValue(sheetStats, cell, h); err != nil {
			return err
		}
	}
	if err := w.f.SetCellStyle(sheetStats, "A1", "E1", w.headerStyle); err != nil {
		return err
	}
	if err := w.f.SetRowHeight(sheetStats, 1, 24); err != nil {
		return err
	}

	rn := 1
	total := channelAgg{}
	for _, key := range w.seq {
		a := w.byChannel[key]
		rn++
		if err := w.setStatsRow(rn, []any{a.channel, a.shop, a.orders, a.items, a.revenue}); err != nil {
			return err
		}
		total.orders += a.orders
		total.items += a.items
		total.revenue += a.revenue
	}
	rn++
	if err := w.setStatsRow(rn, []any{"TỔNG", "", total.orders, total.items, total.revenue}); err != nil {
		return err
	}
	if err := w.f.SetCellStyle(sheetStats, fmt.Sprintf("A%d", rn), fmt.Sprintf("E%d", rn), w.headerStyle); err != nil {
		return err
	}
	if err := w.f.SetCellStyle(sheetStats, "E2", fmt.Sprintf("E%d", rn), w.moneyStyle); err != nil {
		return err
	}
	for i, width := range []float64{16, 26, 12, 14, 18} {
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return err
		}
		if err := w.f.SetColWidth(sheetStats, col, col, width); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) setStatsRow(rowNum int, vals []any) error {
	cell, err := excelize.CoordinatesToCellName(1, rowNum)
	if err != nil {
		return err
	}
	return w.f.SetSheetRow(sheetStats, cell, &vals)
}

func (w *Writer) moneyCell(v float64) excelize.Cell {
	return excelize.Cell{StyleID: w.moneyStyle, Value: v}
}

func (w *Writer) dateCell(t haravan.Time) any {
	if t.IsZero() {
		return ""
	}
	return excelize.Cell{StyleID: w.dateStyle, Value: t.InVN()}
}

// Write là đường tắt cho trường hợp đã có sẵn toàn bộ đơn trong bộ nhớ.
func Write(path string, rows []Row) error {
	w, err := NewWriter(path)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.AddOrder(r.Channel, r.Order); err != nil {
			return err
		}
	}
	return w.Close()
}

func orderState(o *haravan.Order) string {
	switch {
	case !o.CancelledAt.IsZero() || o.CancelledStatus == "cancelled":
		return "Đã huỷ"
	case !o.ClosedAt.IsZero() || o.ClosedStatus == "closed":
		return "Đã đóng"
	default:
		return "Đang mở"
	}
}

func trackingInfo(o *haravan.Order) (tracking, carrier string) {
	codes, carriers := []string{}, []string{}
	for _, ff := range o.Fulfillments {
		if s := strings.TrimSpace(ff.TrackingNumber); s != "" {
			codes = append(codes, s)
		}
		if s := strings.TrimSpace(ff.TrackingCompany); s != "" {
			carriers = append(carriers, s)
		}
	}
	return strings.Join(codes, ", "), strings.Join(dedupe(carriers), ", ")
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func custPhone(o *haravan.Order) string {
	if o.Customer == nil {
		return ""
	}
	return o.Customer.Phone
}

func custEmail(o *haravan.Order) string {
	if o.Customer == nil {
		return ""
	}
	return o.Customer.Email
}

func addrPhone(a *haravan.Address) string {
	if a == nil {
		return ""
	}
	return a.Phone
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
