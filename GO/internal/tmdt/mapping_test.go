package tmdt

import (
	"math"
	"testing"
	"time"

	"order-processor/internal/tmdt/lookup"
)

func vnTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

// fakeTables dựng bảng tra cứu nhỏ trong bộ nhớ để test quy tắc, không
// phải mở file Excel.
func fakeTables(t *testing.T) *lookup.Tables {
	t.Helper()
	tb, err := lookup.FromRows(
		// data shop: Tên sản phẩm | Phân loại | Mã combo | TP1 | SL1 | ...
		[][]string{
			{"Tên sản phẩm", "Phân loại", "Mã combo", "MÃ TP 1", "SLTP1", "MÃ TP 2", "SLTP2", "MÃ TP 3", "SLTP3", "MÃ TP 4", "SLTP4"},
			{"Bột tẩy lồng", "Combo 5 Túi", "SP000443", "TP10127", "5", "", "", "", "", "", ""},
			{"Combo đôi", "Bộ 2 món", "SP999", "TP111", "1", "TP222", "2", "", "", "", ""},
			{"Không mã combo", "Loại A", "", "TP333", "1", "", "", "", "", "", ""},
			// Dòng quà tặng: tra được nhưng CỐ Ý không khai mã thành phẩm nào.
			{"QUÀ TẶNG ĐƠN TỪ 200K", "", "SP-QUA", "", "", "", "", "", "", "", ""},
			// Dòng lỗi dữ liệu: có mã thành phẩm nhưng SLTP dùng dấu phẩy thập phân.
			{"Sản phẩm SLTP dấu phẩy", "Loại B", "SP-PHAY", "TP555", "1,5", "", "", "", "", "", ""},
		},
		// Mã misa: cột B = tên kênh, cột D = mã MISA, dữ liệu từ dòng 3
		[][]string{
			{"", "Tên Kênh", "KÊNH BÁN", "Mã MISA"},
			{"", "", "", ""},
			{"", "Tẩy lồng máy giặt Blue", "TIKTOK", "MN_TMDT_00016"},
		},
	)
	if err != nil {
		t.Fatalf("lookup.FromRows: %v", err)
	}
	return tb
}

func baseLine() OrderLine {
	return OrderLine{
		OrderCode:    "585694423512745362",
		Shop:         "Tẩy lồng máy giặt Blue",
		KhoBan:       "Kho Hà Nội",
		KenhBanHang:  "tiktokshop",
		Quantity:     1,
		Title:        "Bột tẩy lồng",
		VariantTitle: "Combo 5 Túi",
		Price:        88999,
		SKU:          "SP000443",
	}
}

func TestChannelLabel(t *testing.T) {
	cases := map[string]string{
		"tiktokshop":  "TikTok",
		"TikTok Shop": "TikTok",
		"shopee":      "Shopee",
		"Shopee":      "Shopee",
		"web":         "web",
		"":            "",
	}
	for in, want := range cases {
		if got := ChannelLabel(in); got != want {
			t.Errorf("ChannelLabel(%q) = %q, muốn %q", in, got, want)
		}
	}
}

func TestBuildQtyAndUnitPrice(t *testing.T) {
	line := baseLine()
	line.CreatedAt = vnTime(t, "2026-08-23T23:54:05+07:00")

	res := Build([]OrderLine{line}, fakeTables(t), Options{ProductName: func(tp string) string { return "tên " + tp }})

	if len(res.OrderRows) != 1 {
		t.Fatalf("có %d dòng dondathang, muốn 1", len(res.OrderRows))
	}
	row := res.OrderRows[0]
	// SLTP = 5 → Số lượng = 1 × 5 = 5; Đơn giá = 88999 ÷ 5 ÷ 1.08.
	if row.Qty != 5 {
		t.Errorf("Qty = %v, muốn 5", row.Qty)
	}
	// Tính bằng biến float64, KHÔNG phải hằng số: hằng số Go rút gọn với độ
	// chính xác vô hạn rồi mới làm tròn một lần, nên có thể lệch chữ số cuối
	// so với hai phép chia float64 mà mapping.go thực sự làm.
	gia, sltp := 88999.0, 5.0
	wantPrice := gia / sltp / 1.08
	if row.UnitPrice != wantPrice {
		t.Errorf("UnitPrice = %v, muốn %v", row.UnitPrice, wantPrice)
	}
	if row.SKU != "TP10127" {
		t.Errorf("SKU = %q, muốn TP10127", row.SKU)
	}
	if row.ProductName != "tên TP10127" {
		t.Errorf("ProductName = %q — Build phải gọi Options.ProductName với mã thành phẩm", row.ProductName)
	}
	if row.EntryDate != "23/08/2026" {
		t.Errorf("EntryDate = %q, muốn 23/08/2026", row.EntryDate)
	}
	if row.OrderNumber != "ĐĐHTMĐT-TikTok-585694423512745362" {
		t.Errorf("OrderNumber = %q", row.OrderNumber)
	}
	wantDesc := "TMĐT-TikTok - Tẩy lồng máy giặt Blue - 585694423512745362 - Ngày đổ 23/08/2026 - HN"
	if row.Description != wantDesc {
		t.Errorf("Description = %q,\nmuốn                %q", row.Description, wantDesc)
	}
	if row.ShipTo != "HN" || row.Warehouse != "TP_HN_12" || row.RegionCode != "TMĐT_MB" || row.StatCode != "HN" {
		t.Errorf("cụm kho = %q/%q/%q/%q, muốn HN/TP_HN_12/TMĐT_MB/HN",
			row.ShipTo, row.Warehouse, row.RegionCode, row.StatCode)
	}
	if row.CustomerCode != "MN_TMDT_00016" {
		t.Errorf("CustomerCode = %q, muốn MN_TMDT_00016", row.CustomerCode)
	}
	if row.Note != "585694423512745362" {
		t.Errorf("Note = %q, muốn mã đơn gốc", row.Note)
	}
}

func TestBuildLongAnWarehouse(t *testing.T) {
	line := baseLine()
	line.KhoBan = "Miền Nam - Kho mặc định"
	res := Build([]OrderLine{line}, fakeTables(t), Options{})
	row := res.OrderRows[0]
	if row.ShipTo != "LA" || row.Warehouse != "LA_KHOTMDT" || row.RegionCode != "TMĐT_MN" || row.StatCode != "LA" {
		t.Errorf("cụm kho = %q/%q/%q/%q, muốn LA/LA_KHOTMDT/TMĐT_MN/LA",
			row.ShipTo, row.Warehouse, row.RegionCode, row.StatCode)
	}
}

func TestBuildPromoRowWhenPriceZero(t *testing.T) {
	line := baseLine()
	line.Price = 0
	res := Build([]OrderLine{line}, fakeTables(t), Options{})
	if !res.OrderRows[0].IsPromoItem {
		t.Errorf("giá 0 phải đánh dấu Hàng khuyến mại = Có")
	}
	if res.OrderRows[0].UnitPrice != 0 {
		t.Errorf("UnitPrice = %v, muốn 0", res.OrderRows[0].UnitPrice)
	}
}

func TestBuildComboWithTwoComponents(t *testing.T) {
	line := baseLine()
	line.Title, line.VariantTitle, line.SKU = "Combo đôi", "Bộ 2 món", "SP999"
	line.Quantity, line.Price = 2, 100000

	res := Build([]OrderLine{line}, fakeTables(t), Options{})
	if len(res.OrderRows) != 2 {
		t.Fatalf("có %d dòng, muốn 2 (một dòng cho mỗi thành phẩm)", len(res.OrderRows))
	}
	if res.OrderRows[0].SKU != "TP111" || res.OrderRows[0].Qty != 2 {
		t.Errorf("thành phẩm 1 = %q × %v, muốn TP111 × 2", res.OrderRows[0].SKU, res.OrderRows[0].Qty)
	}
	if res.OrderRows[1].SKU != "TP222" || res.OrderRows[1].Qty != 4 {
		t.Errorf("thành phẩm 2 = %q × %v, muốn TP222 × 4", res.OrderRows[1].SKU, res.OrderRows[1].Qty)
	}
	// Giá sản phẩm là giá CẢ combo nên chia cho TỔNG SLTP (1 + 2 = 3), không
	// phải cho SLTP của riêng từng thành phẩm — mẫu chuẩn buộc phải như vậy.
	// Tính bằng biến float64 chứ không phải hằng số: hằng số Go được rút gọn
	// với độ chính xác vô hạn rồi mới làm tròn một lần, lệch chữ số cuối so
	// với hai phép chia float64 mà mapping.go thực sự làm.
	gia, tongSLTP := 100000.0, 3.0
	wantUnit := gia / tongSLTP / 1.08
	if res.OrderRows[0].UnitPrice != wantUnit || res.OrderRows[1].UnitPrice != wantUnit {
		t.Errorf("Đơn giá = %v / %v, muốn cả hai bằng %v (÷ tổng SLTP = 3)",
			res.OrderRows[0].UnitPrice, res.OrderRows[1].UnitPrice, wantUnit)
	}
	// Hệ quả phải giữ: Σ(Số lượng × Đơn giá) = SL đặt × Giá sản phẩm ÷ 1.08.
	tong := res.OrderRows[0].Qty*res.OrderRows[0].UnitPrice + res.OrderRows[1].Qty*res.OrderRows[1].UnitPrice
	if muon := 2 * 100000.0 / 1.08; math.Abs(tong-muon) > 1e-6 {
		t.Errorf("tổng tiền dòng hàng = %v, muốn %v", tong, muon)
	}
	// Sheet Haravan vẫn CHỈ 1 dòng cho dòng hàng này.
	if len(res.SheetRows) != 1 {
		t.Errorf("có %d dòng sheet, muốn 1", len(res.SheetRows))
	}
}

func TestBuildLooksUpByProductVariantWhenNoSKU(t *testing.T) {
	line := baseLine()
	line.SKU = ""
	line.Title, line.VariantTitle = "Không mã combo", "Loại A"
	res := Build([]OrderLine{line}, fakeTables(t), Options{})
	if len(res.OrderRows) != 1 || res.OrderRows[0].SKU != "TP333" {
		t.Fatalf("không tra được theo Tên sản phẩm + Phân loại: %+v", res.OrderRows)
	}
}

func TestBuildCollectsMissingComboUnique(t *testing.T) {
	a := baseLine()
	a.SKU, a.Title, a.VariantTitle = "SP-CHUA-KHAI", "Sản phẩm lạ", "Loại lạ"
	b := a // đúng mã đó, dòng thứ hai
	res := Build([]OrderLine{a, b}, fakeTables(t), Options{})

	if len(res.Missing) != 1 {
		t.Fatalf("có %d mục thiếu, muốn 1 (gom unique theo khoá)", len(res.Missing))
	}
	m := res.Missing[0]
	if m.Key != MissingKey("SP-CHUA-KHAI", "Sản phẩm lạ", "Loại lạ") {
		t.Errorf("Key = %q", m.Key)
	}
	if m.LineCount != 2 {
		t.Errorf("LineCount = %d, muốn 2", m.LineCount)
	}
	if m.Combo != "SP-CHUA-KHAI" || m.Product != "Sản phẩm lạ" || m.Variant != "Loại lạ" {
		t.Errorf("mục thiếu thiếu thông tin điền sẵn: %+v", m)
	}
	// Vẫn phải sinh dòng dondathang mang #N/A, không được bỏ âm thầm.
	if len(res.OrderRows) != 2 {
		t.Fatalf("có %d dòng dondathang, muốn 2", len(res.OrderRows))
	}
	if res.OrderRows[0].SKU != lookup.NotAvailable {
		t.Errorf("SKU = %q, muốn %q", res.OrderRows[0].SKU, lookup.NotAvailable)
	}
	if res.SheetRows[0].TP[0] != lookup.NotAvailable {
		t.Errorf("sheet TP1 = %q, muốn %q", res.SheetRows[0].TP[0], lookup.NotAvailable)
	}
}

func TestBuildMissingShopKeepsNAAndCounts(t *testing.T) {
	line := baseLine()
	line.Shop = "Shop lạ chưa khai"
	res := Build([]OrderLine{line}, fakeTables(t), Options{})
	if res.OrderRows[0].CustomerCode != lookup.NotAvailable {
		t.Errorf("CustomerCode = %q, muốn %q", res.OrderRows[0].CustomerCode, lookup.NotAvailable)
	}
	if res.MissingShops["Shop lạ chưa khai"] != 1 {
		t.Errorf("MissingShops = %v, muốn đếm 1 dòng", res.MissingShops)
	}
}

func TestBuildClevyStaysInSheetButNotInDondathang(t *testing.T) {
	line := baseLine()
	line.Shop = ShopKhongQuyDoi
	res := Build([]OrderLine{line}, fakeTables(t), Options{})

	if len(res.SheetRows) != 1 {
		t.Fatalf("có %d dòng sheet, muốn 1 — đơn CLEVY vẫn nằm trong sheet Haravan", len(res.SheetRows))
	}
	if res.SheetRows[0].TP[0] != "" {
		t.Errorf("TP1 = %q, muốn TRỐNG (không quy đổi theo thiết kế, khác #N/A)", res.SheetRows[0].TP[0])
	}
	if len(res.OrderRows) != 0 {
		t.Errorf("có %d dòng dondathang, muốn 0", len(res.OrderRows))
	}
	if len(res.Missing) != 0 {
		t.Errorf("CLEVY không phải mã thiếu, không được hỏi người dùng: %+v", res.Missing)
	}
	// CLEVY chưa có trong sheet "Mã misa" và sẽ không bao giờ có — nó không
	// sinh dòng hạch toán nào. Đếm nó vào MissingShops là báo động giả ở MỌI
	// lần chạy, kèm câu "→ Mã khách hàng = #N/A" trong khi không có dòng nào
	// mang giá trị đó.
	if len(res.MissingShops) != 0 {
		t.Errorf("MissingShops = %v, muốn rỗng — CLEVY cố ý không quy đổi", res.MissingShops)
	}
	// Nhưng GIÁ TRỊ thì vẫn #N/A: workbook thật ghi #N/A vào cột Mã misa.
	if res.SheetRows[0].Misa != lookup.NotAvailable {
		t.Errorf("sheet Mã misa = %q, muốn %q", res.SheetRows[0].Misa, lookup.NotAvailable)
	}
	if res.NoComponent == nil || len(res.NoComponent) != 0 {
		t.Errorf("NoComponent = %v, muốn map rỗng khác nil — CLEVY không phải dòng bị bỏ âm thầm", res.NoComponent)
	}
}

// Hai test dưới khoá đường BỎ DÒNG ÂM THẦM: dòng TRA ĐƯỢC bảng "data shop"
// nhưng không sinh ra dòng hạch toán nào. Không có bộ đếm thì đơn hàng biến
// mất khỏi file gửi AMIS mà không #N/A, không cảnh báo, không log.

func TestBuildGiftRowWithNoComponentIsCounted(t *testing.T) {
	line := baseLine()
	line.SKU, line.Title, line.VariantTitle = "SP-QUA", "QUÀ TẶNG ĐƠN TỪ 200K", ""
	res := Build([]OrderLine{line}, fakeTables(t), Options{})

	if len(res.OrderRows) != 0 {
		t.Fatalf("có %d dòng dondathang, muốn 0 — dòng này không khai mã thành phẩm", len(res.OrderRows))
	}
	if len(res.Missing) != 0 {
		t.Errorf("tra ĐƯỢC bảng nên không phải mã thiếu: %+v", res.Missing)
	}
	want := KhongKhaiThanhPham + MissingKey("SP-QUA", "QUÀ TẶNG ĐƠN TỪ 200K", "")
	if res.NoComponent[want] != 1 {
		t.Errorf("NoComponent = %v, muốn %q đếm 1", res.NoComponent, want)
	}
	// Sheet vẫn ghi ô TRỐNG (đúng thiết kế), khác #N/A.
	if res.SheetRows[0].TP[0] != "" {
		t.Errorf("sheet TP1 = %q, muốn trống", res.SheetRows[0].TP[0])
	}
}

func TestBuildUnreadableSLTPIsCountedAsDataError(t *testing.T) {
	line := baseLine()
	// SLTP1 trong bảng là "1,5" — dấu phẩy thập phân, ParseFloat không đọc được.
	line.SKU, line.Title, line.VariantTitle = "SP-PHAY", "Sản phẩm SLTP dấu phẩy", "Loại B"
	res := Build([]OrderLine{line}, fakeTables(t), Options{})

	if len(res.OrderRows) != 0 {
		t.Fatalf("có %d dòng dondathang, muốn 0", len(res.OrderRows))
	}
	if len(res.Missing) != 0 {
		t.Errorf("tra ĐƯỢC bảng nên không phải mã thiếu: %+v", res.Missing)
	}
	want := SLTPKhongDocDuoc + MissingKey("SP-PHAY", "Sản phẩm SLTP dấu phẩy", "Loại B")
	if res.NoComponent[want] != 1 {
		t.Errorf("NoComponent = %v, muốn %q đếm 1 (lỗi dữ liệu, khác dòng quà tặng)",
			res.NoComponent, want)
	}
	// Hai nguyên nhân KHÔNG được lẫn vào nhau: mức nghiêm trọng khác hẳn.
	if res.NoComponent[KhongKhaiThanhPham+MissingKey("SP-PHAY", "Sản phẩm SLTP dấu phẩy", "Loại B")] != 0 {
		t.Errorf("SLTP không đọc được bị xếp thành 'không khai thành phẩm': %v", res.NoComponent)
	}
}

func TestBuildBlankShopNameGetsReadableLabel(t *testing.T) {
	line := baseLine()
	line.Shop = "   " // Haravan có đơn thiếu note attribute BranchName
	res := Build([]OrderLine{line}, fakeTables(t), Options{})
	if res.MissingShops[ShopKhongTen] != 1 {
		t.Errorf("MissingShops = %v, muốn khoá %q đếm 1 — Task 11 in thẳng khoá này ra cảnh báo",
			res.MissingShops, ShopKhongTen)
	}
	if _, coKhoaRong := res.MissingShops[""]; coKhoaRong {
		t.Errorf("MissingShops còn khoá rỗng: %v", res.MissingShops)
	}
}
