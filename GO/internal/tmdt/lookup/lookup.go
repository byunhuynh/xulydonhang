// Package lookup nạp hai bảng tra cứu từ workbook "XUẤT HÀNG HN-LA MỚI.xlsx":
//
//   - sheet "data shop"  → quy đổi sản phẩm/combo trên sàn ra mã thành phẩm (MÃ TP 1..4, SLTP1..4)
//   - sheet "Mã misa"    → quy đổi tên shop ra mã MISA
//
// Đây là phần trước đây do công thức Excel (VLOOKUP) làm. Bảng vẫn để trong
// workbook vì đó là nơi người dùng thêm sản phẩm mới; code chỉ đọc ra.
package lookup

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

const (
	SheetDataShop = "data shop"
	SheetMisa     = "Mã misa"

	// Giá trị Excel trả về khi VLOOKUP không tìm thấy. Giữ nguyên chuỗi này để
	// người dùng nhận ra ngay dòng nào chưa khai báo trong bảng tra cứu.
	NotAvailable = "#N/A"
)

// ComboRow là một dòng của sheet "data shop".
type ComboRow struct {
	Product string    // A - Tên sản phẩm
	Variant string    // B - Phân loại
	Combo   string    // C - Mã combo
	TP      [4]string // D, F, H, J - MÃ TP 1..4
	SL      [4]string // E, G, I, K - SLTP1..4
}

type Tables struct {
	// byProductVariant khớp kiểu VLOOKUP: khoá là A&B&C, dùng khi đơn không có
	// Mã sản phẩm. Bảng có sẵn các dòng để trống Mã combo dành riêng cho nhánh này.
	byProductVariant map[string]*ComboRow
	byCombo          map[string]*ComboRow
	misa             map[string]string

	Combos int
	Misa   int
}

// Load đọc hai bảng tra cứu từ file Excel.
func Load(path string) (*Tables, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("mở workbook tra cứu %q: %w", path, err)
	}
	defer f.Close()

	t := &Tables{
		byProductVariant: map[string]*ComboRow{},
		byCombo:          map[string]*ComboRow{},
		misa:             map[string]string{},
	}

	rows, err := f.GetRows(SheetDataShop)
	if err != nil {
		return nil, fmt.Errorf("đọc sheet %q trong %q: %w", SheetDataShop, path, err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("sheet %q trong %q không có dữ liệu", SheetDataShop, path)
	}
	for _, r := range rows[1:] {
		cell := func(i int) string {
			if i < len(r) {
				return strings.TrimSpace(r[i])
			}
			return ""
		}
		row := &ComboRow{Product: cell(0), Variant: cell(1), Combo: cell(2)}
		for i := 0; i < 4; i++ {
			row.TP[i] = cell(3 + i*2)
			row.SL[i] = cell(4 + i*2)
		}
		if row.Product == "" && row.Combo == "" {
			continue
		}
		t.Combos++
		// VLOOKUP lấy dòng khớp ĐẦU TIÊN, nên không ghi đè khoá đã có.
		if k := key(row.Product + row.Variant + row.Combo); k != "" {
			if _, ok := t.byProductVariant[k]; !ok {
				t.byProductVariant[k] = row
			}
		}
		if row.Combo != "" {
			if k := key(row.Combo); k != "" {
				if _, ok := t.byCombo[k]; !ok {
					t.byCombo[k] = row
				}
			}
		}
	}

	misaRows, err := f.GetRows(SheetMisa)
	if err != nil {
		return nil, fmt.Errorf("đọc sheet %q trong %q: %w", SheetMisa, path, err)
	}
	// Vùng tra cứu trong công thức cũ là 'Mã misa'!$B$3:$D$12 — bỏ dòng tiêu đề
	// và dòng trống, cột B là tên kênh, cột D là mã MISA.
	for i := 2; i < len(misaRows); i++ {
		r := misaRows[i]
		cell := func(n int) string {
			if n < len(r) {
				return strings.TrimSpace(r[n])
			}
			return ""
		}
		name, code := cell(1), cell(3)
		if name == "" || code == "" {
			continue
		}
		t.Misa++
		if _, ok := t.misa[key(name)]; !ok {
			t.misa[key(name)] = code
		}
	}

	if t.Combos == 0 || t.Misa == 0 {
		return nil, fmt.Errorf("workbook %q thiếu dữ liệu tra cứu (data shop: %d dòng, Mã misa: %d dòng)",
			path, t.Combos, t.Misa)
	}
	return t, nil
}

// key chuẩn hoá khoá tra cứu. VLOOKUP của Excel không phân biệt hoa thường.
func key(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// ByProductVariant tra theo "Tên sản phẩm" + "Giá trị thuộc tính 1" — nhánh dùng
// khi đơn không có Mã sản phẩm.
func (t *Tables) ByProductVariant(product, variant string) (*ComboRow, bool) {
	r, ok := t.byProductVariant[key(product+variant)]
	return r, ok
}

// ByCombo tra theo Mã sản phẩm (= Mã combo trong bảng).
func (t *Tables) ByCombo(code string) (*ComboRow, bool) {
	r, ok := t.byCombo[key(code)]
	return r, ok
}

// MisaCode tra mã MISA theo tên shop.
func (t *Tables) MisaCode(shop string) (string, bool) {
	c, ok := t.misa[key(shop)]
	return c, ok
}
