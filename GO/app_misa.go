package main

import (
	"sort"

	"order-processor/internal/misapush"
)

// MisaRouteInput là dòng ĐẦU của một nhóm đơn trên bảng kết quả — đủ để
// tính khoá định tuyến của cả nhóm.
type MisaRouteInput struct {
	System       string `json:"system"`
	CustomerCode string `json:"customerCode"`
	ShipTo       string `json:"shipTo"`
}

// MisaRouteInfo là khoá định tuyến đã phân giải: khoá máy đọc, nhãn cho
// người đọc, và nhánh mặc định ("" khi chưa map).
type MisaRouteInfo struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Branch string `json:"branch"`
}

// MisaResolveRoutes phân giải khoá + nhãn + nhánh mặc định cho từng nhóm
// đơn. Modal push gọi đúng một lần khi mở, cho cả lô.
//
// Quy tắc định tuyến CHỈ tồn tại ở đây. Viết lại nó bằng TypeScript sẽ
// tạo ra hai bản sao của một quy tắc kế toán, và bản nào lệch thì đơn vào
// nhầm sổ trong khi test của bên kia vẫn xanh.
func (a *App) MisaResolveRoutes(rows []MisaRouteInput) ([]MisaRouteInfo, error) {
	settings, err := a.GetAppSettings()
	if err != nil {
		return nil, err
	}
	out := make([]MisaRouteInfo, 0, len(rows))
	for _, r := range rows {
		key := misapush.RouteKey(r.System, r.CustomerCode, r.ShipTo)
		out = append(out, MisaRouteInfo{
			Key:    key,
			Label:  misapush.Label(key),
			Branch: misapush.Lookup(settings.MisaRouting, key),
		})
	}
	return out, nil
}

// MisaRouteOptions liệt kê mọi khoá định tuyến đã biết — hợp của bảng
// gieo và những khoá đã lưu trong cấu hình — sắp theo nhãn để danh sách
// trong Cài đặt không nhảy chỗ khi có khoá mới.
func (a *App) MisaRouteOptions() ([]MisaRouteInfo, error) {
	settings, err := a.GetAppSettings()
	if err != nil {
		return nil, err
	}

	branches := misapush.SeedRouting()
	// Giá trị đã lưu ĐÈ LÊN bảng gieo, kể cả khi người dùng đã đổi khác
	// mặc định — đây là điểm cả tính năng dựa vào.
	for k, v := range settings.MisaRouting {
		branches[k] = v
	}

	out := make([]MisaRouteInfo, 0, len(branches))
	for k, v := range branches {
		out = append(out, MisaRouteInfo{Key: k, Label: misapush.Label(k), Branch: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out, nil
}
