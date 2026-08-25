package tmdt

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

// HaravanHeaders là 23 cột của bố cục "chuẩn" — giống hệt standardHeaders
// trong internal/tmdt/export/standard.go (bản CLI đã đối chiếu 100% với
// file người dùng làm tay). Giữ hai bản riêng thay vì dùng chung một biến:
// bản của export thuộc về StreamWriter ghi ra FILE MỚI, còn bản này ghi
// vào MỘT SHEET của workbook đang có — hai đường ghi khác nhau, và nếu mai
// này bố cục CLI đổi thì sheet trong workbook của người dùng không được đổi theo.
var HaravanHeaders = []string{
	"Mã đơn hàng", "Tổng tiền", "Tổng cộng", "Ngày đặt hàng",
	"Số lượng sản phẩm", "Tên sản phẩm", "Giá trị thuộc tính 1",
	"Giá sản phẩm", "Mã sản phẩm", "Thuộc tính", "Kho bán", "Kênh bán hàng",
	"Thời gian Đặt",
	"MÃ TP 1", "SLTP1", "MÃ TP 2", "SLTP2", "MÃ TP 3", "SLTP3", "MÃ TP 4", "SLTP4",
	"Shop", "Mã misa",
}

// WriteHaravanSheet ghi ĐÈ sheet "Haravan" của workbook tại path: xoá hẳn
// sheet cũ rồi tạo lại, nên rác của lần chạy trước không sót dòng nào (xoá
// từng ô sẽ để lại kiểu dáng và vùng dimension cũ). Hai sheet tra cứu
// "data shop" / "Mã misa" không bị đụng tới — đó là nơi người dùng khai
// sản phẩm, mất là mất dữ liệu thật.
func WriteHaravanSheet(path string, rows []SheetRow) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("tmdt: mở %s: %w", path, err)
	}
	defer f.Close()

	// excelize.NewSheet LUÔN thêm sheet mới vào CUỐI workbook (xem setWorkbook
	// trong excelize) — không có tham số nào để "tạo lại đúng chỗ cũ". Vì
	// hàm này xoá rồi tạo lại "Haravan" ở MỌI lần chạy, nếu không tự ghi nhớ
	// và khôi phục vị trí thì tab "Haravan" sẽ lặng lẽ bị kéo ra cuối workbook
	// mỗi lần chạy — kể cả khi người dùng đã tự kéo nó sang vị trí khác trong
	// Excel hôm trước. Không mất dữ liệu, nhưng đây là workbook người dùng mở
	// tay mỗi ngày, và "công cụ tự sắp lại tab của tôi" là bất ngờ không nên
	// có. ĐỪNG xoá đoạn ghi nhớ/khôi phục này coi là thừa — thiếu nó hàm vẫn
	// chạy đúng về dữ liệu, chỉ sai về vị trí tab, nên rất dễ bị tưởng là
	// ceremony vô nghĩa.
	list := f.GetSheetList()
	var followingSheet string
	for i, name := range list {
		if name == SheetHaravan {
			if i+1 < len(list) {
				followingSheet = list[i+1]
			}
			break
		}
	}

	if idx, err := f.GetSheetIndex(SheetHaravan); err == nil && idx >= 0 {
		if err := f.DeleteSheet(SheetHaravan); err != nil {
			return fmt.Errorf("tmdt: xoá sheet %s: %w", SheetHaravan, err)
		}
	}
	if _, err := f.NewSheet(SheetHaravan); err != nil {
		return fmt.Errorf("tmdt: tạo sheet %s: %w", SheetHaravan, err)
	}

	// MoveSheet(source, target) chuyển "source" ra ĐỨNG NGAY TRƯỚC "target"
	// (đã đọc doc comment của excelize để chắc chiều tham số, không đoán theo
	// tên biến). Nếu "Haravan" từng có sheet đứng ngay sau nó, chuyển nó về
	// lại trước sheet đó — khôi phục đúng vị trí cũ. Không có sheet nào theo
	// sau (Haravan từng là sheet cuối, hoặc chưa từng tồn tại) thì để nguyên,
	// NewSheet đã tự đặt nó ở cuối — không cần case riêng nào cả.
	if followingSheet != "" {
		if err := f.MoveSheet(SheetHaravan, followingSheet); err != nil {
			return fmt.Errorf("tmdt: khôi phục vị trí sheet %s: %w", SheetHaravan, err)
		}
	}

	header := make([]interface{}, len(HaravanHeaders))
	for i, h := range HaravanHeaders {
		header[i] = h
	}
	if err := f.SetSheetRow(SheetHaravan, "A1", &header); err != nil {
		return fmt.Errorf("tmdt: ghi tiêu đề: %w", err)
	}

	for i, r := range rows {
		cells := []interface{}{
			r.OrderCode, r.Subtotal, r.Total, r.OrderDate, r.Quantity,
			r.Title, r.VariantTitle, r.Price, r.SKU, r.Attributes,
			r.KhoBan, r.KenhBanHang, r.CreatedAt.Format("02/01/2006 15:04:05"),
			r.TP[0], r.SL[0], r.TP[1], r.SL[1], r.TP[2], r.SL[2], r.TP[3], r.SL[3],
			r.Shop, r.Misa,
		}
		axis := fmt.Sprintf("A%d", i+2)
		if err := f.SetSheetRow(SheetHaravan, axis, &cells); err != nil {
			return fmt.Errorf("tmdt: ghi dòng %d: %w", i+2, err)
		}
	}

	if err := f.Save(); err != nil {
		return fmt.Errorf("tmdt: lưu %s: %w", path, err)
	}
	return nil
}
