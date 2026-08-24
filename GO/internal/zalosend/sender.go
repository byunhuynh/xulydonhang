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

// coopSystemKey là NGOẠI LỆ DUY NHẤT khi ghép key: OrderRow.System hiện
// tại luôn là "Coop" (không phân biệt Co-opmart/Co-op Food ở tầng xử lý
// PDF — xem coop_processor.go, chỉ có đúng 1 nhánh "Coop"), nhưng key
// thật trong Cài đặt > Zalo (kế thừa từ settings.ini gốc) là "COOPMART"
// (vd "MNCOOPMART"), không phải "COOP". Mọi hệ thống khác khớp thẳng
// bằng cách viết hoa (BigC→BIGC, Satra→SATRA, FujiMart→FUJIMART, ...).
const coopSystemKey = "COOPMART"

// ResolveContact tra map settings.Zalo (đã có sẵn, sửa được qua popup
// Cài đặt) theo TỔ HỢP miền + hệ thống, khớp đúng key thật trong Cài đặt
// > Zalo (vd "MNBIGC", "MBCOOPMART", "MNSATRA") — 2 ký tự đầu của
// customerCode (OrderRow.MaKhachHang) là miền ("MN" = Miền Nam, "MB" =
// Miền Bắc, quy tắc do người dùng xác nhận trực tiếp), nối với tên hệ
// thống viết hoa. Giá trị rỗng trong map bị coi như CHƯA cấu hình (không
// phải "gửi tới tên rỗng") — khớp cách KeyValueEditor bỏ qua dòng
// key/value rỗng lúc lưu. Dùng rune (không phải byte) để lấy 2 ký tự đầu
// — customerCode có thể là chuỗi fallback tiếng Việt có dấu (vd "Không
// xác định" khi không tra được mã khách hàng thật) khi tra cứu thất bại
// phía trên, cắt theo byte có thể cắt giữa 1 ký tự UTF-8 nhiều byte.
func ResolveContact(system, customerCode string, zaloMap map[string]string) (string, error) {
	region := ""
	runes := []rune(strings.ToUpper(customerCode))
	if len(runes) >= 2 {
		region = string(runes[:2])
	}

	systemKey := strings.ToUpper(system)
	if systemKey == "COOP" {
		systemKey = coopSystemKey
	}

	key := region + systemKey
	contact, ok := zaloMap[key]
	if !ok || contact == "" {
		return "", fmt.Errorf("%w: %s", ErrNoContact, key)
	}
	return contact, nil
}
