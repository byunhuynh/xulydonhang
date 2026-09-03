// GO/internal/appsettings/store.go
package appsettings

import (
	"encoding/json"
	"fmt"
	"os"
)

// Settings là toàn bộ nội dung cấu hình app: 3 map, tương ứng 3 khối
// <gid>/<zalo>/<reminder> của settings.ini cũ. Cả 3 đều map tên khóa
// (string) -> giá trị (string) — Reminder hiện chỉ có giá trị "1"
// nhưng vẫn lưu dạng string để không giả định trước ý nghĩa giá trị
// tương lai (ví dụ sau này có thể là số ngày nhắc thay vì cờ bật/tắt).
type Settings struct {
	Gid      map[string]string `json:"gid"`
	Zalo     map[string]string `json:"zalo"`
	Reminder map[string]string `json:"reminder"`
	// Haravan giữ cấu hình nhánh TMĐT. Hai khoá quy ước:
	//   access_token  - private token Haravan, scope com.read_orders
	//   exclude_shops - danh sách shop bỏ qua, ngăn bởi dấu phẩy
	// Vẫn là map[string]string như 3 nhóm còn lại để popup Cài đặt dùng
	// lại nguyên KeyValueEditor, không phải viết form riêng.
	Haravan map[string]string `json:"haravan"`
	// Misa giữ cấu hình đẩy đơn lên AMIS Kế toán. Ba khoá quy ước:
	//   sid_url      - URL Apps Script cấp phiên mới khi phiên hết hạn
	//   db_ha_thanh  - tên (hoặc database_id) bộ dữ liệu nhánh Hà Thành
	//   db_htla      - tên (hoặc database_id) bộ dữ liệu nhánh HTLA
	// Vẫn là map[string]string như các nhóm khác để popup Cài đặt dùng
	// lại nguyên KeyValueEditor.
	Misa map[string]string `json:"misa"`
	// MisaRouting ánh xạ khoá định tuyến -> nhánh ("ha_thanh"/"htla").
	// Khoá do misapush.RouteKey sinh ra, ví dụ "Lotte", "BigC/GC",
	// "JIT-CHOICE/WH6_HN", "TMĐT-*". Đây là NGUỒN CHÂN LÝ: bảng gieo
	// trong code chỉ điền vào chỗ trống, không bao giờ ghi đè, để sửa
	// hằng số ở bản sau không xê dịch một cài đặt nào đang chạy.
	MisaRouting map[string]string `json:"misa_routing"`
	// Warehouse ánh xạ khoá nhánh vendor -> mã kho ghi vào cột V của
	// dondathang.xlsx, ví dụ "bigc/MN_MT" -> "LA_KHO2026". Khoá do
	// warehouse.Branches sinh ra. Cũng là NGUỒN CHÂN LÝ như MisaRouting:
	// bảng gieo trong code chỉ điền chỗ trống, không bao giờ ghi đè, để
	// sửa hằng số ở bản sau không xê dịch kho của một nhánh nào đang chạy.
	Warehouse map[string]string `json:"warehouse"`
}

// Store đọc/ghi Settings từ 1 file JSON đuôi .bhconfig (không phải
// .ini/.txt — đổi đuôi có chủ đích để không bị double-click mở nhầm
// bằng Notepad). Nội dung vẫn là text đọc được nếu người dùng CỐ TÌNH
// mở bằng tay (không mã hóa) — quyết định rõ ràng của user, đánh đổi
// lấy khả năng tự debug bằng tay nếu app lỗi.
type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load đọc Settings từ file .bhconfig. Nếu file .bhconfig CHƯA TỒN TẠI
// nhưng oldIniPath (settings.ini cũ) CÓ, tự động đọc file cũ (định
// dạng tag <gid>/<zalo>/<reminder>), ghi ra file .bhconfig mới, rồi
// trả về dữ liệu đã migrate — settings.ini cũ được GIỮ NGUYÊN trên đĩa,
// không xóa, không sửa. Nếu CẢ 2 file đều không tồn tại, trả về
// Settings với 3 map rỗng (không lỗi — app mới cài lần đầu, chưa có
// cấu hình gì).
func (s *Store) Load(oldIniPath string) (Settings, error) {
	data, err := os.ReadFile(s.path)
	if err == nil {
		var settings Settings
		if err := json.Unmarshal(data, &settings); err != nil {
			return Settings{}, fmt.Errorf("appsettings: parse %s: %w", s.path, err)
		}
		ensureMaps(&settings)
		return settings, nil
	}
	if !os.IsNotExist(err) {
		return Settings{}, fmt.Errorf("appsettings: read %s: %w", s.path, err)
	}

	settings, migrated, err := migrateFromOldIni(oldIniPath)
	if err != nil {
		return Settings{}, err
	}
	if !migrated {
		empty := Settings{}
		ensureMaps(&empty)
		return empty, nil
	}
	if err := s.Save(settings); err != nil {
		return Settings{}, fmt.Errorf("appsettings: write migrated %s: %w", s.path, err)
	}
	// Save nhận theo giá trị nên ensureMaps bên trong nó chỉ sửa bản sao cục bộ.
	// Phải vá lại biến settings ở đây trước khi trả về.
	ensureMaps(&settings)
	return settings, nil
}

// Save ghi Settings ra file .bhconfig, định dạng JSON có thụt lề (đọc
// được nếu người dùng cố mở bằng tay).
func (s *Store) Save(settings Settings) error {
	ensureMaps(&settings)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("appsettings: encode settings: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("appsettings: write %s: %w", s.path, err)
	}
	return nil
}

// ensureMaps thay mọi map nil bằng map rỗng — JSON marshal của map nil
// ra "null" thay vì "{}" (xác nhận thực nghiệm ở Step 1), và frontend
// (TypeScript) cần luôn nhận được object thật để render bảng, không
// phải null.
func ensureMaps(s *Settings) {
	if s.Gid == nil {
		s.Gid = map[string]string{}
	}
	if s.Zalo == nil {
		s.Zalo = map[string]string{}
	}
	if s.Reminder == nil {
		s.Reminder = map[string]string{}
	}
	if s.Haravan == nil {
		s.Haravan = map[string]string{}
	}
	if s.Misa == nil {
		s.Misa = map[string]string{}
	}
	if s.MisaRouting == nil {
		s.MisaRouting = map[string]string{}
	}
	if s.Warehouse == nil {
		s.Warehouse = map[string]string{}
	}
}
