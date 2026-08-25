package zalosend

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ZaloSender trừu tượng hoá việc gửi 1 tin nhắn Zalo, để app.go test được
// logic vòng lặp gửi hàng loạt (SendZaloMessages/runZaloBatch) bằng fake
// sender, không cần trình duyệt thật — cùng lý do processing.Processor
// là interface trong app.go hiện có.
type ZaloSender interface {
	// EnsureLoggedIn mở trình duyệt (nếu chưa mở, chạy ẩn hoàn toàn — QR
	// hiện qua onQR chứ không hiện cửa sổ Chrome) và đảm bảo đã đăng nhập
	// chat.zalo.me, chờ quét QR nếu cần. Gọi 1 LẦN trước cả batch gửi,
	// không gọi lại cho từng tin — trình duyệt sống xuyên suốt vòng đời
	// App, khác với send_message.py (Python), vốn coi login là 1 phần
	// của mỗi lần chạy script riêng lẻ.
	//
	// onQR được gọi mỗi khi có mã QR MỚI cần hiện (chuỗi markup SVG lấy
	// trực tiếp từ trang, không phải ảnh chụp màn hình) — chuỗi rỗng nghĩa
	// là "không cần hiện QR nữa" (đã đăng nhập hoặc chưa tới lúc cần QR).
	// Có thể là nil nếu caller không quan tâm QR (vd test).
	EnsureLoggedIn(ctx context.Context, onQR func(svgMarkup string)) error

	// SendMessage tìm đúng hội thoại theo contactQuery (tên liên hệ/nhóm
	// hiển thị trên Zalo) rồi gửi message dạng text thuần.
	SendMessage(ctx context.Context, contactQuery, message string) error

	// RefreshQR bấm nút "Lấy mã mới" trên trang đăng nhập nếu đang hiện
	// (mã QR đã hết hạn hoặc người dùng chủ động muốn đổi mã) — không làm
	// gì (trả nil) nếu không có mã QR nào đang chờ (đã đăng nhập, hoặc
	// chưa từng gọi EnsureLoggedIn). An toàn gọi bất cứ lúc nào từ luồng
	// khác trong lúc EnsureLoggedIn vẫn đang chờ quét.
	RefreshQR(ctx context.Context) error

	// Close đóng trình duyệt — gọi lúc App shutdown.
	Close() error
}

// ErrNoContact báo tổ hợp miền+hệ thống (xem ResolveContact) chưa có
// mapping Zalo trong settings.Zalo — job tương ứng bị SKIP (không dừng
// cả batch), người dùng sửa qua Cài đặt > tab Zalo rồi gửi lại đúng PO
// đó.
var ErrNoContact = errors.New("zalosend: no zalo contact configured for this system")

// systemKeyOverrides là những hệ thống mà OrderRow.System KHÁC tên dùng
// làm key trong Cài đặt > Zalo (key kế thừa từ settings.ini gốc). Mọi hệ
// thống còn lại khớp thẳng bằng cách viết hoa: BigC→BIGC, Satra→SATRA,
// FujiMart→FUJIMART, Kingfood→KINGFOOD, JMart→JMART, Winmart→WINMART,
// Lotte→LOTTE, Emart→EMART.
//
//   - "Coop": tầng xử lý PDF chỉ có đúng 1 nhánh "Coop" (không tách
//     Co-opmart/Co-op Food — xem coop_processor.go), key thật là
//     "COOPMART" (vd "MNCOOPMART").
//
// "JIT-CHOICE" CỐ Ý không nằm đây: key ghép ra là "MNJIT-CHOICE" và
// người dùng đã chọn đổi tên key trong Cài đặt cho khớp thay vì thêm một
// ánh xạ ẩn nữa vào code (quyết định 25/08/2026). Một ánh xạ càng ít thì
// key trong Cài đặt càng nói đúng thứ nó thật sự là.
var systemKeyOverrides = map[string]string{
	"COOP": "COOPMART",
}

// ResolveContact tra map settings.Zalo (đã có sẵn, sửa được qua popup
// Cài đặt) theo hai key, THEO THỨ TỰ:
//
//  1. miền + phân khúc + hệ thống (vd "MBGCBIGC") — cho phép tách đích
//     đến theo phân khúc: BigC Gia Công ("MB_GC_*") và BigC Modern Trade
//     ("MB_MT_*") là hai loại đơn khác nhau và đi về hai group Zalo khác
//     nhau. Chỉ cần thêm key này vào Cài đặt > Zalo là tách được, không
//     phải sửa code.
//  2. miền + hệ thống (vd "MNBIGC", "MBCOOPMART", "MNSATRA") — key chung
//     như trước, dùng khi không có key phân khúc nào khớp.
//
// Nhờ thứ tự đó, mọi cấu hình cũ chạy y hệt trước: key phân khúc là thứ
// người dùng chủ động thêm để ghi đè, không thêm thì không đổi gì. — 2 ký tự đầu của
// customerCode (OrderRow.MaKhachHang) là miền ("MN" = Miền Nam, "MB" =
// Miền Bắc, quy tắc do người dùng xác nhận trực tiếp), nối với tên hệ
// thống viết hoa. Giá trị rỗng trong map bị coi như CHƯA cấu hình (không
// phải "gửi tới tên rỗng") — khớp cách KeyValueEditor bỏ qua dòng
// key/value rỗng lúc lưu. Dùng rune (không phải byte) để lấy 2 ký tự đầu
// — customerCode có thể là chuỗi fallback tiếng Việt có dấu (vd "Không
// xác định" khi không tra được mã khách hàng thật) khi tra cứu thất bại
// phía trên, cắt theo byte có thể cắt giữa 1 ký tự UTF-8 nhiều byte.
func ResolveContact(system, customerCode string, zaloMap map[string]string) (string, error) {
	region, segment := splitCustomerCode(customerCode)

	systemKey := strings.ToUpper(system)
	if override, ok := systemKeyOverrides[systemKey]; ok {
		systemKey = override
	}

	keys := make([]string, 0, 2)
	if segment != "" {
		keys = append(keys, region+segment+systemKey)
	}
	keys = append(keys, region+systemKey)

	for _, key := range keys {
		if contact := lookupContact(zaloMap, key); contact != "" {
			return contact, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrNoContact, strings.Join(keys, " hoặc "))
}

// splitCustomerCode tách mã khách hàng thành miền và PHÂN KHÚC. Mã có
// dạng <miền>_<phân khúc>_<mã NCC> (vd "MB_GC_bgc06" = BigC Gia Công
// miền Bắc, "MB_MT_bgc06" = BigC Modern Trade miền Bắc, "MN_MT_cop120" =
// Co-op Modern Trade miền Nam). Miền vẫn là 2 ký tự đầu như trước, đọc
// theo rune chứ không theo byte vì customerCode có thể là chuỗi tiếng
// Việt có dấu khi tra cứu mã khách hàng thất bại (vd "Không xác định").
// Phân khúc rỗng khi mã không có đủ 3 phần - mọi mã như vậy chạy y hệt
// trước khi có hàm này.
func splitCustomerCode(customerCode string) (region, segment string) {
	upper := strings.ToUpper(customerCode)
	if runes := []rune(upper); len(runes) >= 2 {
		region = string(runes[:2])
	}
	if parts := strings.Split(upper, "_"); len(parts) >= 3 {
		segment = parts[1]
	}
	return region, segment
}

// lookupContact tra map theo key, khớp đúng trước rồi mới bỏ qua
// hoa/thường. Key trong Cài đặt > Zalo do người dùng gõ tay nên viết hoa
// không đồng nhất là chuyện thường (config thật có cả "MBBIGC" lẫn
// "MBGCBigC"); ưu tiên khớp đúng để hai key chỉ khác nhau hoa/thường
// không tráo chỗ cho nhau. Giá trị rỗng vẫn bị coi như CHƯA cấu hình.
func lookupContact(zaloMap map[string]string, key string) string {
	if contact := zaloMap[key]; contact != "" {
		return contact
	}
	for mapKey, contact := range zaloMap {
		if contact != "" && strings.EqualFold(mapKey, key) {
			return contact
		}
	}
	return ""
}
