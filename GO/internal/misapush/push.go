package misapush

import (
	"context"
	"fmt"

	"order-processor/internal/misa"
)

// Request là một lần đẩy cho MỘT nhánh kế toán.
type Request struct {
	// SessionPath là file misa-session.json. Đọc không được mà có SidURL
	// thì vẫn chạy — nhánh xin phiên mới sẽ lo.
	SessionPath string
	// SidURL là endpoint cấp phiên mới (Apps Script). Rỗng = không tự
	// gia hạn được, phiên chết là dừng.
	SidURL string
	// Database là tên (khớp một phần, không phân biệt hoa thường) hoặc
	// database_id đầy đủ của bộ dữ liệu kế toán.
	Database string
	// FilePath là file .xlsx đã tách riêng cho nhánh này.
	FilePath string
	// BaseURL để trống thì dùng host thật của AMIS Kế toán; test trỏ vào
	// server giả qua đây.
	BaseURL string

	// Force cho phep GHI SO du con dong khong hop le - MISA bo qua dung
	// nhung dong do va bao thanh cong. Mac dinh false vi day la duong de
	// tuong da day du ma thuc ra thieu don; chi bat khi nguoi dung nhin
	// thay danh sach dong hong va co y day phan con lai.
	Force bool
	// Log nhận từng dòng tiến độ; để nil thì im lặng.
	Log func(string)
}

// Pusher thực hiện một lần đẩy. Tách thành interface để App test được mà
// không cần mạng.
type Pusher interface {
	Push(ctx context.Context, req Request) (*misa.ImportResult, error)
}

// HTTPPusher là bản thật, gọi API AMIS Kế toán.
type HTTPPusher struct{}

// Push đăng nhập, chuyển sang đúng bộ dữ liệu, rồi nhập khẩu file Excel.
//
// Mỗi lần gọi dựng một Client MỚI. Client giữ Headers biến đổi
// (Authorization, X-MISA-Context) và SwitchDatabase thay X-MISA-Context
// tại chỗ; đổi bộ dữ liệu hai lần trong cùng một client là đường chưa ai
// đi, còn misapush dòng lệnh thì chỉ đổi đúng một lần mỗi lần chạy. Một
// client mỗi nhánh tái hiện chính xác lần chạy đã được kiểm chứng. Giá
// phải trả là một lời gọi cấp token thêm — cấp token mới không giết
// token đang có, nên vô hại.
//
// Luôn Commit=true. Force do bên gọi quyết: mặc định false nên MISA kiểm
// tra cả file trước, không dòng
// nào lỗi thì ghi sổ luôn; còn dòng lỗi thì CẢ NHÁNH không ghi gì. Kết
// quả vẫn được trả về kèm lỗi để bên gọi liệt kê đủ các dòng hỏng.
func (p *HTTPPusher) Push(ctx context.Context, req Request) (*misa.ImportResult, error) {
	c := misa.NewClient(req.BaseURL)
	if req.Log != nil {
		c.Log = func(format string, args ...any) { req.Log(fmt.Sprintf(format, args...)) }
	}

	// Gắn nguồn cấp phiên TRƯỚC khi đăng nhập: phiên trong file chết thì
	// client tự xin phiên mới rồi ghi đè file, thay vì bắt người dùng mở
	// trình duyệt chạy misasniff.
	if req.SidURL != "" && req.SessionPath != "" {
		dest := req.SessionPath
		c.SetRenewFromURL(req.SidURL, func(s *misa.Session) error { return s.Save(dest) })
	}

	if s, err := misa.LoadSession(req.SessionPath); err == nil {
		c.UseSession(s)
	} else if req.SidURL == "" {
		return nil, fmt.Errorf("không đọc được phiên %s (%w) và chưa khai URL cấp phiên trong Cài đặt > MISA",
			req.SessionPath, err)
	}

	// Cấp token ngay để phát hiện phiên hỏng TRƯỚC khi upload, thay vì để
	// lỗi nổ ra giữa lúc đang đẩy dữ liệu.
	if err := c.Login(ctx); err != nil {
		return nil, fmt.Errorf("đăng nhập MISA: %w", err)
	}

	db, err := c.SwitchDatabaseByName(ctx, req.Database)
	if err != nil {
		return nil, fmt.Errorf("chọn bộ dữ liệu %q: %w", req.Database, err)
	}
	if req.Log != nil {
		req.Log(fmt.Sprintf("bộ dữ liệu: %s", db.DatabaseName))
	}

	return c.ImportExcel(ctx, misa.ImportOptions{
		FilePath:   req.FilePath,
		RefType:    misa.RefTypeSAOrder,
		TableName:  misa.TableSAOrder,
		SheetIndex: -1,
		Commit:     true,
		Force:      req.Force,
	})
}
