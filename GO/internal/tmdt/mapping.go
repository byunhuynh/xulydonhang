package tmdt

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/warehouse"
	"order-processor/internal/tmdt/lookup"
)

// ShopKhongQuyDoi là shop CỐ Ý không quy đổi ra mã thành phẩm — công thức
// Excel cũ trả về rỗng cho toàn bộ cột MÃ TP/SLTP của shop này, và nhánh
// mới giữ nguyên quy tắc đó. Phân biệt rõ với "chưa khai báo": ô trống là
// đúng thiết kế, còn #N/A là thứ cần hỏi người dùng.
const ShopKhongQuyDoi = "CLEVY VIỆT NAM"

// vatDivisor: giá trên sàn đã gồm VAT 8%, còn cột Đơn giá của AMIS là giá
// chưa thuế.
const vatDivisor = 1.08

// ShopKhongTen là nhãn thay cho tên shop rỗng trong MissingShops. Haravan
// có đơn không mang note attribute BranchName, và Task 11 in thẳng khoá này
// ra cảnh báo — khoá "" sẽ thành câu 'Shop "" chưa có trong sheet...', vô nghĩa.
const ShopKhongTen = "(đơn không có tên shop)"

// Hai tiền tố của khoá trong Result.NoComponent, phân biệt HAI nguyên nhân
// khác hẳn nhau về mức nghiêm trọng — Task 11 đọc tiền tố để nói đúng chuyện:
//
//   - KhongKhaiThanhPham: dòng "data shop" TRA ĐƯỢC nhưng cố ý không khai mã
//     thành phẩm nào (bảng thật đang có 6 dòng quà tặng như vậy: "QUÀ TẶNG ĐƠN
//     TỪ 200K"...). Đúng thiết kế, chỉ báo cho biết, KHÔNG phải lỗi.
//   - SLTPKhongDocDuoc: dòng có khai MÃ TP nhưng không đọc được SLTP nào
//     (ô trống, chữ, hay dấu phẩy thập phân kiểu "1,5"). Đây là LỖI DỮ LIỆU
//     trong bảng tra cứu, người dùng cần sửa, vì đơn hàng thật bị bỏ khỏi file
//     hạch toán.
const (
	KhongKhaiThanhPham = "khong-khai-tp:"
	SLTPKhongDocDuoc   = "sltp-khong-doc-duoc:"
)

// OrderLine là MỘT dòng hàng đã tách khỏi kiểu dữ liệu của Haravan. Tầng
// quy đổi cố ý KHÔNG nhận *haravan.Order: nhờ vậy golden test nạp được
// 1.585 dòng thật từ CSV mà không cần mạng, và tầng này không phụ thuộc
// vào hình dạng JSON của một API bên ngoài.
type OrderLine struct {
	OrderCode    string    // Mã đơn hàng trên sàn
	Shop         string    // tên shop (note attribute BranchName)
	KhoBan       string    // location_name thô, ví dụ "Kho Hà Nội"
	KenhBanHang  string    // source_name thô, ví dụ "tiktokshop"
	CreatedAt    time.Time // đã ở giờ VN
	Quantity     float64
	Title        string
	VariantTitle string
	Price        float64 // giá 1 đơn vị dòng hàng, đã gồm VAT
	Subtotal     float64 // Tổng tiền của đơn — chỉ để ghi ra sheet
	Total        float64 // Tổng cộng của đơn — chỉ để ghi ra sheet
	SKU          string
	Attributes   string
}

// SheetRow là một dòng của sheet "Haravan" — một dòng hàng, một dòng sheet
// (KHÔNG tách theo thành phẩm như dondathang).
type SheetRow struct {
	OrderCode    string
	Subtotal     float64
	Total        float64
	OrderDate    string
	Quantity     float64
	Title        string
	VariantTitle string
	Price        float64
	SKU          string
	Attributes   string
	KhoBan       string
	KenhBanHang  string
	CreatedAt    time.Time
	TP           [4]string
	SL           [4]string
	Shop         string
	Misa         string
}

// MissingCombo là một mã CHƯA khai báo trong sheet "data shop", đã gom
// unique — 300 dòng cùng thiếu một mã chỉ thành một mục.
type MissingCombo struct {
	Key       string `json:"key"`
	Product   string `json:"product"`
	Variant   string `json:"variant"`
	Combo     string `json:"combo"`
	LineCount int    `json:"lineCount"`
}

type Options struct {
	// ProductName tra tên hàng theo mã thành phẩm (cột S). Bản thật truyền
	// productdata.Store.GetProductInfo; nil thì để trống tên hàng.
	ProductName func(tp string) string
	// Warehouses cấp mã kho TMĐT (cột V) lấy từ Cài đặt. Nil hợp lệ và
	// có nghĩa "dùng mã xuất xưởng" — xem warehouse.Resolver.Get.
	Warehouses *warehouse.Resolver
}

type Result struct {
	SheetRows    []SheetRow
	OrderRows    []excelwriter.TMDTRow
	Missing      []MissingCombo
	MissingShops map[string]int

	// NoComponent đếm những dòng hàng TRA ĐƯỢC bảng "data shop" nhưng không
	// sinh ra dòng hạch toán nào. Nếu không đếm thì đây là đường bỏ dòng ÂM
	// THẦM: không #N/A, không cảnh báo, đơn hàng chỉ đơn giản biến mất khỏi
	// file gửi AMIS. Khoá = tiền tố nguyên nhân (KhongKhaiThanhPham hoặc
	// SLTPKhongDocDuoc) + MissingKey, nên vẫn gom unique đúng một dòng bảng
	// tra cứu như Missing, mà Task 11 đọc tiền tố là biết nên báo hay chỉ ghi log.
	NoComponent map[string]int
}

// MissingKey là khoá gom nhóm mã thiếu: có Mã sản phẩm thì dùng nó, không
// có thì ghép Tên sản phẩm + Phân loại — ĐÚNG hai nhánh mà bảng tra cứu
// dùng để tra, nên khoá luôn tương ứng 1-1 với một dòng "data shop" cần bổ sung.
func MissingKey(sku, title, variant string) string {
	if s := strings.TrimSpace(sku); s != "" {
		return "sku:" + s
	}
	return "pv:" + strings.TrimSpace(title) + "|" + strings.TrimSpace(variant)
}

// ChannelLabel chuẩn hoá tên sàn về đúng dạng dùng trong cột "Số đơn hàng"
// và "Diễn giải": "tiktokshop" / "TikTok Shop" đều thành "TikTok".
func ChannelLabel(raw string) string {
	s := strings.TrimSpace(raw)
	low := strings.ToLower(s)
	switch {
	case strings.Contains(low, "tiktok") || strings.Contains(low, "tik tok") || low == "tts":
		return "TikTok"
	case strings.Contains(low, "shopee") || strings.Contains(low, "shoppe") || low == "spx":
		return "Shopee"
	}
	return s
}

// warehouseOf quy đổi tên kho của Haravan ra bộ mã mà AMIS cần: shipTo đi
// vào CẢ HAI cột E và AM, maKho vào V, maDonVi vào AJ.
//
// Mã kho đổi ngày 27/08/2026 theo yêu cầu người dùng: TP_HN_12 → TP_HN_13,
// LA_KHOTMDT → LA_TP. Chỉ nhánh TMĐT đổi; nhánh JIT (jit_airway_processor.go)
// và các vendor bán lẻ vẫn giữ mã cũ của họ. Chính lần đó là lý do hai mã
// này giờ nằm trong Cài đặt (warehouse.Branches "tmdt/HN" và "tmdt/khac")
// thay vì trong code — wh nil nghĩa là dùng mã xuất xưởng ghi ở đó.
func warehouseOf(khoBan string, wh *warehouse.Resolver) (shipTo, maKho, maDonVi string) {
	if strings.EqualFold(strings.TrimSpace(khoBan), "Kho Hà Nội") {
		return "HN", wh.Get("tmdt/HN"), "TMĐT_MB"
	}
	return "LA", wh.Get("tmdt/khac"), "TMĐT_MN"
}

func Build(lines []OrderLine, tables *lookup.Tables, opt Options) Result {
	res := Result{MissingShops: map[string]int{}, NoComponent: map[string]int{}}
	missingIdx := map[string]int{} // khoá → vị trí trong res.Missing

	for _, line := range lines {
		shop := strings.TrimSpace(line.Shop)
		noConvert := strings.EqualFold(shop, ShopKhongQuyDoi)

		misa, ok := tables.MisaCode(shop)
		if !ok {
			misa = lookup.NotAvailable
		}

		sheet := SheetRow{
			OrderCode: line.OrderCode, Subtotal: line.Subtotal, Total: line.Total,
			OrderDate: line.CreatedAt.Format(time.RFC3339), Quantity: line.Quantity,
			Title: line.Title, VariantTitle: line.VariantTitle, Price: line.Price,
			SKU: line.SKU, Attributes: line.Attributes, KhoBan: line.KhoBan,
			KenhBanHang: line.KenhBanHang, CreatedAt: line.CreatedAt,
			Shop: shop, Misa: misa,
		}

		if !ok && !noConvert {
			// GIÁ TRỊ và BỘ ĐẾM chia tay nhau ở đây, cố ý: sheet "Haravan"
			// của workbook thật vẫn ghi #N/A vào cột Mã misa cho shop không
			// quy đổi, nên misa phải giữ #N/A. Nhưng shop đó KHÔNG sinh dòng
			// hạch toán nào, nên đếm nó vào MissingShops sẽ khiến Task 11
			// cảnh báo "chưa có trong sheet Mã misa (169 dòng → Mã khách hàng
			// = #N/A)" ở MỌI lần chạy — báo động giả vĩnh viễn, mà nửa sau
			// còn sai: không có dòng nào để mang #N/A cả.
			ten := shop
			if ten == "" {
				ten = ShopKhongTen
			}
			res.MissingShops[ten]++
		}

		var combo *lookup.ComboRow
		found := false
		if !noConvert {
			if strings.TrimSpace(line.SKU) == "" {
				combo, found = tables.ByProductVariant(line.Title, line.VariantTitle)
			} else {
				combo, found = tables.ByCombo(line.SKU)
			}
			if !found {
				key := MissingKey(line.SKU, line.Title, line.VariantTitle)
				if i, seen := missingIdx[key]; seen {
					res.Missing[i].LineCount++
				} else {
					missingIdx[key] = len(res.Missing)
					res.Missing = append(res.Missing, MissingCombo{
						Key: key, Product: strings.TrimSpace(line.Title),
						Variant: strings.TrimSpace(line.VariantTitle),
						Combo:   strings.TrimSpace(line.SKU), LineCount: 1,
					})
				}
				for i := 0; i < 4; i++ {
					sheet.TP[i], sheet.SL[i] = lookup.NotAvailable, lookup.NotAvailable
				}
			} else {
				for i := 0; i < 4; i++ {
					sheet.TP[i] = blankIfZero(combo.TP[i])
					sheet.SL[i] = blankIfZero(combo.SL[i])
				}
			}
		}
		res.SheetRows = append(res.SheetRows, sheet)

		if noConvert {
			// Cố ý không sinh dòng đặt hàng: không có mã thành phẩm để ghi.
			continue
		}
		rows, nguyenNhan := orderRowsFor(line, sheet, opt)
		if nguyenNhan != "" {
			res.NoComponent[nguyenNhan+MissingKey(line.SKU, line.Title, line.VariantTitle)]++
		}
		res.OrderRows = append(res.OrderRows, rows...)
	}
	return res
}

// orderRowsFor sinh dòng dondathang cho MỘT dòng hàng: một dòng cho mỗi
// thành phẩm có mã. Mã chưa khai báo (#N/A) vẫn sinh ĐÚNG MỘT dòng mang
// #N/A — bỏ âm thầm vài trăm dòng khỏi file hạch toán nguy hiểm hơn nhiều
// so với một ô #N/A mà AMIS sẽ báo lỗi ngay khi import.
//
// Giá trị thứ hai là NGUYÊN NHÂN không sinh được dòng nào (một trong hai tiền
// tố KhongKhaiThanhPham / SLTPKhongDocDuoc), rỗng khi có sinh dòng. Build đếm
// nó vào Result.NoComponent để không còn đường bỏ dòng âm thầm nào.
func orderRowsFor(line OrderLine, sheet SheetRow, opt Options) ([]excelwriter.TMDTRow, string) {
	channel := ChannelLabel(line.KenhBanHang)
	shipTo, maKho, maDonVi := warehouseOf(line.KhoBan, opt.Warehouses)
	date := line.CreatedAt.Format("02/01/2006")
	desc := fmt.Sprintf("TMĐT-%s - %s - %s - Ngày đổ %s - %s",
		channel, sheet.Shop, line.OrderCode, date, shipTo)

	base := excelwriter.TMDTRow{
		EntryDate:    date,
		OrderNumber:  fmt.Sprintf("ĐĐHTMĐT-%s-%s", channel, line.OrderCode),
		ShipTo:       shipTo,
		CustomerCode: sheet.Misa,
		Description:  desc,
		IsPromoItem:  line.Price == 0,
		Warehouse:    maKho,
		RegionCode:   maDonVi,
		StatCode:     shipTo,
		Note:         line.OrderCode,
	}

	name := func(tp string) string {
		if opt.ProductName == nil {
			return ""
		}
		return opt.ProductName(tp)
	}

	if sheet.TP[0] == lookup.NotAvailable {
		row := base
		row.SKU = lookup.NotAvailable
		// Chưa biết SLTP nên giữ nguyên số lượng đặt và giá của dòng hàng.
		row.Qty = line.Quantity
		row.UnitPrice = line.Price / vatDivisor
		return []excelwriter.TMDTRow{row}, ""
	}

	// tongSL = TỔNG SLTP của các thành phẩm dòng hàng này sinh ra. Giá sản
	// phẩm là giá của CẢ combo, nên phải phân bổ đều cho toàn bộ số lượng
	// thành phẩm — nhờ vậy Σ(Số lượng × Đơn giá) đúng bằng Giá sản phẩm ÷ 1.08.
	// Combo một thành phẩm thì tổng = SLTP1, y hệt công thức "÷ SLTPᵢ" cũ; mẫu
	// chuẩn bắt lỗi ở 26 dòng combo HAI thành phẩm (SP000450, SLTP 1+1), nơi
	// "÷ SLTPᵢ" cho ra đơn giá gấp đôi vì mỗi thành phẩm nhận trọn giá combo.
	var tongSL float64
	for i := 0; i < 4; i++ {
		if sheet.TP[i] == "" {
			continue
		}
		tongSL += parseSL(sheet.SL[i])
	}
	if tongSL == 0 {
		// Không thành phẩm nào có SLTP đọc được: vòng lặp dưới không sinh dòng
		// nào, đặt 1 chỉ để chắc chắn không bao giờ chia cho 0.
		tongSL = 1
	}

	var out []excelwriter.TMDTRow
	for i := 0; i < 4; i++ {
		tp := sheet.TP[i]
		if tp == "" {
			continue
		}
		sl := parseSL(sheet.SL[i])
		if sl == 0 {
			continue
		}
		row := base
		row.SKU = tp
		row.ProductName = name(tp)
		row.Qty = line.Quantity * sl
		row.UnitPrice = line.Price / tongSL / vatDivisor
		out = append(out, row)
	}
	if len(out) == 0 {
		// Có khai MÃ TP mà vẫn không ra dòng nào ⇒ SLTP không đọc được; không
		// khai MÃ TP nào ⇒ dòng cố ý không có thành phẩm (quà tặng).
		for i := 0; i < 4; i++ {
			if sheet.TP[i] != "" {
				return nil, SLTPKhongDocDuoc
			}
		}
		return nil, KhongKhaiThanhPham
	}
	return out, ""
}

// parseSL đọc SLTP. Bảng tra cứu là do người dùng gõ tay nên giá trị có
// thể kèm khoảng trắng hoặc xuống dòng; giá trị không đọc được coi như 0
// (bỏ qua thành phẩm đó) thay vì làm hỏng cả lần chạy.
func parseSL(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

// blankIfZero: công thức cũ bọc kết quả trong IF(KQ=0,"",KQ) nên giá trị 0
// hiển thị thành rỗng. Cũng cắt luôn khoảng trắng/xuống dòng thừa mà bảng
// tra cứu gõ tay hay dính (ví dụ "TP10127\n").
func blankIfZero(s string) string {
	s = strings.TrimSpace(s)
	if s == "0" {
		return ""
	}
	return s
}
