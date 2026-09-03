package main

import (
	"errors"

	"order-processor/internal/processing"
	"order-processor/internal/processing/warehouse"
)

// errWarehouseBusy giữ nguyên cách diễn đạt reloadDataSources đã dùng cho
// đúng tình huống này, để người dùng thấy một thông điệp nhất quán.
var errWarehouseBusy = errors.New("đang xử lý đơn hàng, vui lòng thử lưu lại sau khi xử lý xong")

// WarehouseInfo là một dòng của bảng "Kho" trong popup Cài đặt.
type WarehouseInfo struct {
	// Key là địa chỉ của nhánh, do Go sinh — frontend chỉ hiển thị, không
	// cho gõ (xem warehouse.Branch.Key).
	Key string `json:"key"`
	// Label là tên nhánh hiển thị cho người dùng.
	Label string `json:"label"`
	// Code là mã kho đang hiệu lực: giá trị đã lưu nếu có, còn lại là mã
	// mặc định app xuất xưởng.
	Code string `json:"code"`
	// Default là mã xuất xưởng, để popup hiện nó làm gợi ý khi người
	// dùng xoá trắng ô — xoá trắng nghĩa là quay về mã này chứ không
	// phải ghi kho rỗng.
	Default string `json:"default"`
}

// WarehouseOptions trả danh sách nhánh kho cho popup Cài đặt dựng bảng.
//
// Giữ NGUYÊN thứ tự khai báo trong warehouse.Branches (nhóm theo vendor)
// chứ không sắp xếp lại theo bảng chữ cái như MisaRouteOptions: các nhánh
// của cùng một vendor phải nằm cạnh nhau thì người dùng mới đối chiếu
// được, còn khoá định tuyến MISA thì độc lập nhau nên xếp A-Z hợp lý hơn.
func (a *App) WarehouseOptions() ([]WarehouseInfo, error) {
	settings, err := a.GetAppSettings()
	if err != nil {
		return nil, err
	}
	resolver := warehouse.NewResolver(settings.Warehouse)

	out := make([]WarehouseInfo, 0, len(warehouse.Branches))
	for _, b := range warehouse.Branches {
		out = append(out, WarehouseInfo{Key: b.Key, Label: b.Label, Code: resolver.Get(b.Key), Default: b.Default})
	}
	return out, nil
}

// warehouseResolver dựng Resolver từ file cài đặt TẠI THỜI ĐIỂM GỌI.
// Dùng cho nhánh TMĐT, nơi mỗi lần dựng file là một hành động riêng của
// người dùng nên đọc lại đĩa vừa rẻ vừa luôn mới. Đọc lỗi thì trả nil,
// nghĩa là dùng mã xuất xưởng — không chặn cả luồng dựng file chỉ vì
// không đọc được cấu hình.
func (a *App) warehouseResolver() *warehouse.Resolver {
	settings, err := a.GetAppSettings()
	if err != nil {
		return nil
	}
	return warehouse.NewResolver(settings.Warehouse)
}

// applyWarehouseSettings áp mã kho mới vào RealProcessor đang chạy, thay
// cho việc bắt khởi động lại app — cùng lý do reloadDataSources tồn tại
// cho gid, nhưng rẻ hơn nhiều vì không phải tải lại gì qua mạng.
//
// Vẫn mượn CHÍNH cờ a.processing mà ProcessFiles/runBatch dùng: goroutine
// của batch đọc trường này trong lúc Process() chạy mà không qua khóa
// riêng nào, nên ghi đè giữa chừng là một cuộc đua thật sự.
func (a *App) applyWarehouseSettings(saved map[string]string) error {
	rp, ok := a.processor.(*processing.RealProcessor)
	if !ok {
		return nil
	}
	if !a.processing.CompareAndSwap(false, true) {
		return errWarehouseBusy
	}
	defer a.processing.Store(false)

	rp.Warehouses = warehouse.NewResolver(saved)
	return nil
}
