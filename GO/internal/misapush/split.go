package misapush

import (
	"fmt"
	"os"

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
// là thêm một nguồn sai lệch không cần thiết. excelize.RemoveRow tự hạ
// chỉ số các công thức tương đối, nên "Thành tiền" (=Y{r}*X{r}) vẫn trỏ
// đúng hàng của nó sau khi dồn lên.
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

	if err := f.Save(); err != nil {
		return fmt.Errorf("misapush: lưu %s: %w", path, err)
	}
	return nil
}
