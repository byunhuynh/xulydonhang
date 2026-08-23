package zalosend

import (
	"context"
	"errors"
	"fmt"
)

// ZaloSender trừu tượng hoá việc gửi 1 tin nhắn Zalo, để app.go test được
// logic vòng lặp gửi hàng loạt (SendZaloMessages/runZaloBatch) bằng fake
// sender, không cần trình duyệt thật — cùng lý do processing.Processor
// là interface trong app.go hiện có.
type ZaloSender interface {
	// EnsureLoggedIn mở trình duyệt (nếu chưa mở) và đảm bảo đã đăng nhập
	// chat.zalo.me, chờ quét QR nếu cần. Gọi 1 LẦN trước cả batch gửi,
	// không gọi lại cho từng tin — trình duyệt sống xuyên suốt vòng đời
	// App, khác với send_message.py (Python), vốn coi login là 1 phần
	// của mỗi lần chạy script riêng lẻ.
	EnsureLoggedIn(ctx context.Context) error

	// SendMessage tìm đúng hội thoại theo contactQuery (tên liên hệ/nhóm
	// hiển thị trên Zalo) rồi gửi message dạng text thuần.
	SendMessage(ctx context.Context, contactQuery, message string) error

	// Close đóng trình duyệt — gọi lúc App shutdown.
	Close() error
}

// ErrNoContact báo hệ thống (OrderRow.System) chưa có mapping Zalo trong
// settings.Zalo — job tương ứng bị SKIP (không dừng cả batch), người
// dùng sửa qua Cài đặt > tab Zalo rồi gửi lại đúng PO đó.
var ErrNoContact = errors.New("zalosend: no zalo contact configured for this system")

// ResolveContact tra map settings.Zalo (đã có sẵn, sửa được qua popup
// Cài đặt) theo tên hệ thống. Giá trị rỗng bị coi như CHƯA cấu hình
// (không phải "gửi tới tên rỗng") — khớp cách KeyValueEditor bỏ qua
// dòng key/value rỗng lúc lưu.
func ResolveContact(system string, zaloMap map[string]string) (string, error) {
	contact, ok := zaloMap[system]
	if !ok || contact == "" {
		return "", fmt.Errorf("%w: %s", ErrNoContact, system)
	}
	return contact, nil
}
