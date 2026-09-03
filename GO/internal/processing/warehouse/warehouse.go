// Package warehouse giữ mã kho (cột V của dondathang.xlsx) của từng
// nhánh vendor, và cho phép người dùng đổi chúng trong Cài đặt thay vì
// phải sửa code.
//
// Trước đây mỗi hàm regionInfo của từng vendor trả thẳng hằng số. Điều
// đó đã một lần bắt phải sửa code: 27/08/2026 người dùng cần nhánh TMĐT
// đổi TP_HN_12 -> TP_HN_13 và LA_KHOTMDT -> LA_TP, trong khi mọi vendor
// bán lẻ giữ nguyên mã cũ — nên bảng cài đặt phải chia theo TỪNG nhánh
// của TỪNG vendor, không phải theo từng mã kho (đổi một mã sẽ đổi cả ở
// những vendor không liên quan).
package warehouse

import "strings"

// Branch là MỘT ô cấu hình: một nhánh của một vendor, kèm mã kho mặc
// định app xuất xưởng.
type Branch struct {
	// Key là địa chỉ của ô, do Go sinh chứ người dùng không gõ — gõ sai
	// một ký tự thì nhánh đó lặng lẽ không khớp gì cả (cùng lý do
	// MisaRoutingEditor không cho gõ khoá).
	Key string
	// Label là tên hiển thị trong popup Cài đặt.
	Label string
	// Default là mã kho app xuất xưởng, cũng là giá trị rơi về khi ô
	// cài đặt bị bỏ trống.
	Default string
}

// Branches là NGUỒN CHÂN LÝ DUY NHẤT cho danh sách nhánh: bảng gieo
// trong Cài đặt, danh sách dòng popup hiển thị, và mã mặc định các
// processor dùng đều đọc từ đây. Thêm nhánh mới cho một vendor thì phải
// thêm khoá vào đây, nếu không mã kho của nhánh đó vẫn nằm cứng trong
// code.
//
// Thứ tự ở đây là thứ tự hiển thị trong popup.
var Branches = []Branch{
	{"chung/MB", "Chung (Coop, Lotte, Satra, nhập tay) · Miền Bắc", "TP_HN_12"},
	{"chung/khac", "Chung (Coop, Lotte, Satra, nhập tay) · Còn lại", "LA_TP"},

	{"bigc/MB", "BigC · Miền Bắc", "TP_HN_12"},
	{"bigc/MN_MT", "BigC · MN_MT", "LA_KHO2026"},
	{"bigc/MN_GC", "BigC · MN_GC", "LA_TP"},

	{"winmart/MN_MT_WIN1326", "Winmart · MN_MT_WIN1326 (Đà Nẵng)", "TP_DN_1"},
	{"winmart/MB", "Winmart · Miền Bắc", "TP_HN_12"},
	{"winmart/khac", "Winmart · Còn lại", "LA_KHO2026"},

	{"emart/MB", "Emart · Miền Bắc", "TP_HN_12"},
	{"emart/khac", "Emart · Còn lại", "LA_KHO2026"},

	{"fujimart/MB", "FujiMart · Miền Bắc", "TP_HN_12"},
	{"fujimart/khac", "FujiMart · Còn lại", "LA_KHO2026"},

	// JMart dùng chung hàm kingfoodRegionInfo nên dùng chung luôn 3 ô
	// này — đổi ở đây là đổi cho cả hai.
	{"kingfood/MB", "Kingfood + JMart · Miền Bắc", "TP_HN_12"},
	{"kingfood/MN_MT_JM0001", "Kingfood + JMart · MN_MT_JM0001", "LA_TP"},
	{"kingfood/khac", "Kingfood + JMart · Còn lại", "LA_KHO2026"},

	{"jit/MB", "JIT-CHOICE · Miền Bắc", "TP_HN_12"},
	{"jit/khac", "JIT-CHOICE · Còn lại", "LA_KHOTMDT"},

	{"tmdt/HN", "TMĐT · Kho Hà Nội", "TP_HN_13"},
	{"tmdt/khac", "TMĐT · Còn lại", "LA_TP"},
}

// defaults tra Branches một lần lúc nạp package thay vì quét lại danh
// sách ở mỗi lần Get (Get chạy trên từng dòng sản phẩm của từng đơn).
var defaults = func() map[string]string {
	m := make(map[string]string, len(Branches))
	for _, b := range Branches {
		m[b.Key] = b.Default
	}
	return m
}()

// Resolver trả mã kho đang hiệu lực cho một nhánh: giá trị người dùng đã
// lưu nếu có, còn lại là mã mặc định.
type Resolver struct {
	saved map[string]string
}

// NewResolver dựng Resolver từ nhóm cài đặt "warehouse" đọc ở đĩa. Nhận
// nil (chưa có cài đặt nào) hoàn toàn hợp lệ.
func NewResolver(saved map[string]string) *Resolver {
	return &Resolver{saved: saved}
}

// Get trả mã kho của nhánh key.
//
// Ô cài đặt để TRỐNG (hoặc chỉ có khoảng trắng) nghĩa là "tôi xoá đi",
// KHÔNG phải "ghi kho rỗng vào cột V" — dòng như thế vào dondathang.xlsx
// là dòng hỏng mà không có gì báo — nên nó rơi về mã mặc định.
//
// Nil receiver cũng hợp lệ và trả về mã mặc định: các processor giữ
// Resolver như một field thường, và một processor dựng không kèm cài đặt
// (mọi test hiện có) vẫn phải chạy đúng với mã xuất xưởng.
//
// Khoá lạ trả về chuỗi rỗng: đó là lỗi lập trình chứ không phải dữ liệu
// người dùng, và không có mã mặc định nào để mà rơi về.
func (r *Resolver) Get(key string) string {
	if r != nil {
		if value := strings.TrimSpace(r.saved[key]); value != "" {
			return value
		}
	}
	return defaults[key]
}

// ApplySeed vật chất hoá mã kho mặc định xuống map cài đặt, CHỈ điền
// khoá còn thiếu, trả về true nếu có thêm khoá nào (khi đó caller ghi
// lại file cài đặt).
//
// Cùng lý do misapush.ApplySeed tồn tại: nếu bảng mặc định chỉ sống
// trong code như giá trị dự phòng, một lần sửa hằng số ở bản sau sẽ lặng
// lẽ đổi kho của mọi nhánh người dùng chưa từng chạm vào. Khoá đã có —
// kể cả khi giá trị rỗng vì người dùng CỐ Ý xoá ô đó — không bao giờ bị
// ghi đè.
func ApplySeed(saved map[string]string) bool {
	changed := false
	for _, b := range Branches {
		if _, ok := saved[b.Key]; !ok {
			saved[b.Key] = b.Default
			changed = true
		}
	}
	return changed
}
