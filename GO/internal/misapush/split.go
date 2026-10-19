package misapush

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

const (
	// SheetName là tên sheet trong mẫu nhập khẩu của MISA — không dấu,
	// đúng như file mẫu tải về từ AMIS Kế toán.
	SheetName = "Don dat hang"
	// FirstDataRow là dòng dữ liệu đầu tiên. Dòng 1..8 là khối hướng dẫn
	// và hàng tiêu đề của mẫu; cùng quy ước mà
	// excelwriter.ClearOrderRows đang dùng.
	FirstDataRow = 9
)

// SplitWorkbook copy src sang dst rồi xoá mọi dòng dữ liệu không nằm
// trong keep, để lại một workbook chỉ chứa đơn của một nhánh kế toán.
//
// Cách làm là copy-rồi-xoá chứ không dựng lại workbook từ đầu: khối tiêu
// đề của mẫu MISA mang ô gộp, style và độ rộng cột: chép tay từng thứ đó
// là thêm một nguồn sai lệch không cần thiết. Trước khi upload, cột Z
// ("Thành tiền") được materialize từ X*Y trong bản tạm. excelize giữ
// cached value cũ của công thức sau khi ConfirmPrice sửa Y, còn MISA đọc
// cached value đó thay vì tự tính lại, nên chỉ giữ công thức sẽ đẩy lên
// số tiền theo giá hệ thống cũ.
func SplitWorkbook(src, dst string, keep []int) error {
	if len(keep) == 0 {
		return fmt.Errorf("misapush: không có dòng nào để tách")
	}

	wanted := make(map[int]bool, len(keep))
	for _, r := range keep {
		wanted[r] = true
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("misapush: đọc %s: %w", src, err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return fmt.Errorf("misapush: ghi %s: %w", dst, err)
	}

	if err := trimRows(dst, wanted); err != nil {
		// Không để lại file dở dang: bước sau sẽ upload thẳng file này
		// lên MISA, một bản cắt nửa chừng là đẩy thiếu đơn mà không ai
		// nhìn thấy.
		os.Remove(dst)
		return err
	}
	return nil
}

// OrderNumbers đọc "Số đơn hàng" (cột B) của từng dòng trong rows. Đây
// chính là cột MISA ghép vào refno, nên là khoá DUY NHẤT nối kết quả
// kiểm tra của MISA (theo chứng từ) ngược về từng đơn trên bảng kết quả.
func OrderNumbers(path string, rows []int) (map[int]string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("misapush: mở %s: %w", path, err)
	}
	defer f.Close()

	out := make(map[int]string, len(rows))
	for _, r := range rows {
		v, err := f.GetCellValue(SheetName, fmt.Sprintf("B%d", r))
		if err != nil {
			return nil, fmt.Errorf("misapush: đọc số đơn hàng B%d: %w", r, err)
		}
		out[r] = strings.TrimSpace(v)
	}
	return out, nil
}

func trimRows(path string, wanted map[int]bool) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("misapush: mở %s: %w", path, err)
	}
	defer f.Close()

	rows, err := f.GetRows(SheetName)
	if err != nil {
		return fmt.Errorf("misapush: đọc sheet %q: %w", SheetName, err)
	}
	last := len(rows)

	for r := range wanted {
		if r < FirstDataRow || r > last {
			return fmt.Errorf("misapush: dòng %d nằm ngoài vùng dữ liệu %d..%d của %s",
				r, FirstDataRow, last, path)
		}
	}

	// Xoá từ dưới lên: xoá từ trên xuống thì mọi dòng phía sau tụt chỉ số
	// và những lần xoá tiếp theo sẽ nhắm nhầm hàng.
	for r := last; r >= FirstDataRow; r-- {
		if wanted[r] {
			continue
		}
		if err := f.RemoveRow(SheetName, r); err != nil {
			return fmt.Errorf("misapush: xoá dòng %d: %w", r, err)
		}
	}

	if err := materializeAmounts(f); err != nil {
		return err
	}

	if err := f.Save(); err != nil {
		return fmt.Errorf("misapush: lưu %s: %w", path, err)
	}
	return nil
}

func materializeAmounts(f *excelize.File) error {
	rows, err := f.GetRows(SheetName)
	if err != nil {
		return fmt.Errorf("misapush: đọc lại dữ liệu sau khi tách: %w", err)
	}
	for rowNumber := FirstDataRow; rowNumber <= len(rows); rowNumber++ {
		formula, err := f.GetCellFormula(SheetName, fmt.Sprintf("Z%d", rowNumber))
		if err != nil {
			return fmt.Errorf("misapush: đọc công thức Z%d: %w", rowNumber, err)
		}
		if formula == "" {
			continue
		}

		qty, err := cellNumber(f, fmt.Sprintf("X%d", rowNumber))
		if err != nil {
			return fmt.Errorf("misapush: đọc số lượng X%d: %w", rowNumber, err)
		}
		price, err := cellNumber(f, fmt.Sprintf("Y%d", rowNumber))
		if err != nil {
			return fmt.Errorf("misapush: đọc đơn giá Y%d: %w", rowNumber, err)
		}
		if err := f.SetCellValue(SheetName, fmt.Sprintf("Z%d", rowNumber), qty*price); err != nil {
			return fmt.Errorf("misapush: ghi thành tiền Z%d: %w", rowNumber, err)
		}
	}
	return nil
}

func cellNumber(f *excelize.File, cell string) (float64, error) {
	value, err := f.GetCellValue(SheetName, cell)
	if err != nil {
		return 0, err
	}
	value = strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	if value == "" {
		return 0, nil
	}
	return strconv.ParseFloat(value, 64)
}
