// Package misapush đẩy đơn hàng đã xử lý lên AMIS Kế toán: quyết định đơn
// nào vào sổ của pháp nhân nào (route.go), tách workbook theo nhánh
// (split.go), và thực hiện một lần nhập khẩu (push.go).
package misapush

import "strings"

// Hai nhánh kế toán. Đây là khoá LƯU TRỮ, không phải chuỗi hiển thị —
// đổi tên bộ dữ liệu bên MISA không được làm hỏng cấu hình đã lưu.
const (
	BranchHaThanh = "ha_thanh"
	BranchHTLA    = "htla"
)

const (
	systemJIT  = "JIT-CHOICE"
	systemBigC = "BigC"

	// TMDTPrefix là tiền tố mà app_tmdt.go gắn trước tên sàn.
	TMDTPrefix = "TMĐT-"
	// TMDTRouteKey đại diện cho MỌI sàn TMĐT. Tên sàn do
	// haravan.DetectChannel dò ra nên không liệt kê hết được; một khoá
	// tiền tố phủ luôn cả sàn mai sau mới có.
	TMDTRouteKey = "TMĐT-*"
)

// RouteKey trả về khoá tra bảng định tuyến cho một đơn.
//
// Tên hệ thống một mình không đủ ở hai chỗ, cả hai đều là yêu cầu nghiệp
// vụ thật:
//   - JIT tách theo kho giao (ShipTo, bóc từ tên file air waybill) — cùng
//     mang System "JIT-CHOICE" và cùng mã khách hàng gán cứng.
//   - BigC tách theo phân khúc mã khách hàng: gia công (GC) và modern
//     trade (MT) vào hai sổ khác nhau, cùng mang System "BigC".
func RouteKey(system, customerCode, shipTo string) string {
	switch system {
	case systemJIT:
		if w := strings.TrimSpace(shipTo); w != "" {
			return systemJIT + "/" + w
		}
		return systemJIT
	case systemBigC:
		if seg := customerSegment(customerCode); seg != "" {
			return systemBigC + "/" + seg
		}
		return systemBigC
	default:
		return system
	}
}

// customerSegment lấy phần giữa của mã khách hàng dạng
// <miền>_<phân khúc>_<mã NCC>, viết hoa. Trả rỗng nếu mã không đủ 3 phần
// (mã đời cũ như "BIGCGARDEN") — bên gọi tự quyết định làm gì với nó.
func customerSegment(code string) string {
	parts := strings.Split(strings.ToUpper(strings.TrimSpace(code)), "_")
	if len(parts) < 3 {
		return ""
	}
	return parts[1]
}

// Label dựng nhãn hiển thị từ khoá định tuyến. Khoá là chuỗi máy đọc,
// nhãn là thứ người dùng nhìn thấy trong Cài đặt và modal push — nhìn
// nhãn phải hiểu ngay vì sao hai đơn BigC lại vào hai sổ khác nhau.
func Label(key string) string {
	switch {
	case key == TMDTRouteKey:
		return "TMĐT (mọi sàn)"
	case key == systemJIT:
		return "JIT"
	case strings.HasPrefix(key, systemJIT+"/"):
		return "JIT · kho " + strings.TrimPrefix(key, systemJIT+"/")
	case key == systemBigC+"/GC":
		return "BigC · gia công"
	case key == systemBigC+"/MT":
		return "BigC · modern trade"
	case strings.HasPrefix(key, systemBigC+"/"):
		return "BigC · " + strings.TrimPrefix(key, systemBigC+"/")
	default:
		return key
	}
}

// SeedRouting là bảng định tuyến mặc định, phủ mọi hệ thống mà các
// processor hiện có sinh ra. Trả về map MỚI mỗi lần gọi để bên gọi sửa
// thoải mái mà không đụng vào bản gốc.
func SeedRouting() map[string]string {
	return map[string]string{
		TMDTRouteKey: BranchHTLA,
		// Dung HAI gia tri nay, khong co "Coop" tran: don Coop thanh cong
		// luon lay he thong tu cot A sheet MAKH va bi ep ve COOPFOOD hoac
		// COOPMART (xem coop_processor.go). Chuoi "Coop" chi xuat hien tren
		// DONG THAT BAI, ma dong that bai khong co ExcelRows nen khong bao
		// gio vao modal push - them no vao day chi lam bang Cai dat co mot
		// dong trong nhu co nghia ma khong bao gio khop don nao.
		"COOPMART":              BranchHTLA,
		"COOPFOOD":              BranchHTLA,
		"Lotte":                 BranchHTLA,
		"Satra":                 BranchHTLA,
		"MR.DIY":                BranchHTLA,
		"FujiMart":              BranchHTLA,
		systemBigC + "/GC":      BranchHTLA,
		systemJIT + "/WH6_HTLA": BranchHTLA,
		systemBigC + "/MT":      BranchHaThanh,
		"Emart":                 BranchHaThanh,
		"Winmart":               BranchHaThanh,
		"Kingfood":              BranchHaThanh,
		"JMart":                 BranchHaThanh,
		systemJIT + "/WH6_HN":   BranchHaThanh,
	}
}

// Lookup tra nhánh của một khoá. Khớp đúng trước (không phân biệt hoa
// thường), riêng khoá TMĐT thì thử thêm khoá tiền tố. Trả chuỗi rỗng khi
// chưa map — bên gọi PHẢI coi đó là "chưa biết", không được đoán bừa một
// nhánh.
func Lookup(routing map[string]string, key string) string {
	if b := lookupFold(routing, key); b != "" {
		return b
	}
	if strings.HasPrefix(key, TMDTPrefix) {
		return lookupFold(routing, TMDTRouteKey)
	}
	return ""
}

func lookupFold(routing map[string]string, key string) string {
	if b, ok := routing[key]; ok && b != "" {
		return b
	}
	for k, v := range routing {
		if v != "" && strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// ApplySeed điền các khoá còn thiếu của bảng gieo vào routing và cho biết
// có thêm gì không.
//
// KHÔNG BAO GIỜ ghi đè khoá đã có, kể cả khi giá trị hiện tại khác bảng
// gieo. Bảng gieo được vật chất hoá xuống settings.bhconfig ngay lần chạy
// đầu chính là để có tính chất này: sửa hằng số SeedRouting ở phiên bản
// sau không làm xê dịch một cấu hình nào đang chạy. Nếu bảng gieo chỉ
// sống trong code như một giá trị dự phòng, một lần sửa hằng số sẽ lặng
// lẽ đổi nhánh của mọi mục người dùng chưa từng chạm vào — tức là đẩy đơn
// vào sổ của pháp nhân khác mà không ai bấm gì.
func ApplySeed(routing map[string]string) bool {
	changed := false
	for k, v := range SeedRouting() {
		if _, ok := routing[k]; !ok {
			routing[k] = v
			changed = true
		}
	}
	return changed
}
