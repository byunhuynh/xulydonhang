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

	rows, err := f.GetRows(SheetDataShop)
	if err != nil {
		return nil, fmt.Errorf("đọc sheet %q trong %q: %w", SheetDataShop, path, err)
	}
	misaRows, err := f.GetRows(SheetMisa)
	if err != nil {
		return nil, fmt.Errorf("đọc sheet %q trong %q: %w", SheetMisa, path, err)
	}

	t, err := FromRows(rows, misaRows)
	if err != nil {
		// Bọc kèm tên file: bảng tra cứu nằm trong workbook người dùng tự chọn,
		// nên biết SAI Ở FILE NÀO mới sửa được.
		return nil, fmt.Errorf("workbook %q: %w", path, err)
	}
	return t, nil
}

// FromRows dựng bảng tra cứu từ dữ liệu thô của hai sheet — cùng logic
// Load dùng, tách ra để test dựng bảng mà không cần file Excel thật.
// dataShop và misa là kết quả GetRows của hai sheet tương ứng, KỂ CẢ
// dòng tiêu đề.
func FromRows(dataShop, misa [][]string) (*Tables, error) {
	t := &Tables{
		byProductVariant: map[string]*ComboRow{},
		byCombo:          map[string]*ComboRow{},
		misa:             map[string]string{},
	}
	if len(dataShop) < 2 {
		return nil, fmt.Errorf("sheet %q không có dữ liệu", SheetDataShop)
	}
	for _, r := range dataShop[1:] {
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

	// Vùng tra cứu trong công thức cũ là 'Mã misa'!$B$3:$D$12 — bỏ dòng tiêu đề
	// và dòng trống, cột B là tên kênh, cột D là mã MISA.
	for i := 2; i < len(misa); i++ {
		r := misa[i]
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
		return nil, fmt.Errorf("thiếu dữ liệu tra cứu (data shop: %d dòng, Mã misa: %d dòng)",
			t.Combos, t.Misa)
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

// AppendComboRows ghi tiếp các dòng khai báo mới vào sheet "data shop",
// đúng cột A..K, ngay dưới dòng CÓ DỮ LIỆU cuối cùng — trả về số dòng đầu
// tiên đã ghi.
//
// Vì sao phải tự dò dòng cuối thay vì dùng len(GetRows(...)): bảng này do
// người dùng gõ tay, thường có dòng trống lẫn ở cuối, và excelize đếm cả
// dòng chỉ mang kiểu dáng. Ghi xuống sau vùng trống sẽ tạo một khoảng hở
// giữa bảng, khiến chính người dùng khó rà soát về sau.
func AppendComboRows(path string, rows []ComboRow) (firstRow int, err error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return 0, fmt.Errorf("lookup: mở %s: %w", path, err)
	}
	defer f.Close()

	existing, err := f.GetRows(SheetDataShop)
	if err != nil {
		return 0, fmt.Errorf("lookup: đọc sheet %q: %w", SheetDataShop, err)
	}
	lastData := 1 // ít nhất là dòng tiêu đề
	for i, r := range existing {
		for _, cell := range r {
			if strings.TrimSpace(cell) != "" {
				lastData = i + 1
				break
			}
		}
	}
	firstRow = lastData + 1
	if len(rows) == 0 {
		return firstRow, nil
	}

	current := firstRow
	for _, row := range rows {
		cells := []interface{}{
			row.Product, row.Variant, row.Combo,
			row.TP[0], row.SL[0], row.TP[1], row.SL[1],
			row.TP[2], row.SL[2], row.TP[3], row.SL[3],
		}
		axis, cellErr := excelize.CoordinatesToCellName(1, current)
		if cellErr != nil {
			return 0, fmt.Errorf("lookup: tính ô dòng %d: %w", current, cellErr)
		}
		if err := f.SetSheetRow(SheetDataShop, axis, &cells); err != nil {
			return 0, fmt.Errorf("lookup: ghi dòng %d vào %q: %w", current, SheetDataShop, err)
		}
		current++
	}

	if err := f.Save(); err != nil {
		return 0, fmt.Errorf("lookup: lưu %s: %w", path, err)
	}
	return firstRow, nil
}
