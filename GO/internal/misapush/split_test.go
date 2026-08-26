package misapush

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

// buildWorkbook dựng một workbook giống mẫu nhập khẩu của MISA: khối tiêu
// đề 8 dòng có ô gộp, rồi n dòng dữ liệu mang công thức tự trỏ vào chính
// dòng mình (Z{r} = Y{r}*X{r}) — đúng thứ excelwriter.WriteOrderRows ghi.
func buildWorkbook(t *testing.T, path string, n int) {
	t.Helper()
	f := excelize.NewFile()
	idx, err := f.NewSheet(SheetName)
	if err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	f.SetActiveSheet(idx)
	f.DeleteSheet("Sheet1")

	if err := f.SetCellValue(SheetName, "A1", "FILE MẪU ĐƠN ĐẶT HÀNG"); err != nil {
		t.Fatalf("SetCellValue A1: %v", err)
	}
	if err := f.SetCellValue(SheetName, "Q7", "Chi tiết hàng tiền"); err != nil {
		t.Fatalf("SetCellValue Q7: %v", err)
	}
	if err := f.MergeCell(SheetName, "Q7", "AP7"); err != nil {
		t.Fatalf("MergeCell: %v", err)
	}
	if err := f.SetCellValue(SheetName, "A8", "Ngày đơn hàng (*)"); err != nil {
		t.Fatalf("SetCellValue A8: %v", err)
	}
	if err := f.SetCellValue(SheetName, "B8", "Số đơn hàng (*)"); err != nil {
		t.Fatalf("SetCellValue B8: %v", err)
	}

	for i := 0; i < n; i++ {
		r := FirstDataRow + i
		if err := f.SetCellValue(SheetName, fmt.Sprintf("B%d", r), fmt.Sprintf("PO-%d", r)); err != nil {
			t.Fatalf("SetCellValue B%d: %v", r, err)
		}
		if err := f.SetCellValue(SheetName, fmt.Sprintf("X%d", r), r); err != nil {
			t.Fatalf("SetCellValue X%d: %v", r, err)
		}
		if err := f.SetCellValue(SheetName, fmt.Sprintf("Y%d", r), 100+r); err != nil {
			t.Fatalf("SetCellValue Y%d: %v", r, err)
		}
		if err := f.SetCellFormula(SheetName, fmt.Sprintf("Z%d", r), fmt.Sprintf("Y%d*X%d", r, r)); err != nil {
			t.Fatalf("SetCellFormula Z%d: %v", r, err)
		}
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSplitWorkbook_GiữĐúngCácDòngĐãChọn(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dondathang.xlsx")
	dst := filepath.Join(dir, "htla.xlsx")
	buildWorkbook(t, src, 5) // r9..r13

	if err := SplitWorkbook(src, dst, []int{9, 11, 13}); err != nil {
		t.Fatalf("SplitWorkbook: %v", err)
	}

	f, err := excelize.OpenFile(dst)
	if err != nil {
		t.Fatalf("OpenFile dst: %v", err)
	}
	defer f.Close()

	wantPO := []string{"PO-9", "PO-11", "PO-13"}
	for i, want := range wantPO {
		cell := fmt.Sprintf("B%d", FirstDataRow+i)
		got, err := f.GetCellValue(SheetName, cell)
		if err != nil {
			t.Fatalf("GetCellValue %s: %v", cell, err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q", cell, got, want)
		}
	}

	rows, err := f.GetRows(SheetName)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(rows) != FirstDataRow+len(wantPO)-1 {
		t.Errorf("số dòng = %d, want %d", len(rows), FirstDataRow+len(wantPO)-1)
	}
}

func TestSplitWorkbook_HạĐúngChỉSốCôngThức(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dondathang.xlsx")
	dst := filepath.Join(dir, "ha_thanh.xlsx")
	buildWorkbook(t, src, 5)

	if err := SplitWorkbook(src, dst, []int{9, 11, 13}); err != nil {
		t.Fatalf("SplitWorkbook: %v", err)
	}

	f, err := excelize.OpenFile(dst)
	if err != nil {
		t.Fatalf("OpenFile dst: %v", err)
	}
	defer f.Close()

	// Công thức tự trỏ vào chính dòng mình PHẢI theo dòng xuống chỗ mới,
	// nếu không "Thành tiền" sẽ nhân sai hàng và MISA đọc vào số bậy.
	for i := 0; i < 3; i++ {
		r := FirstDataRow + i
		got, err := f.GetCellFormula(SheetName, fmt.Sprintf("Z%d", r))
		if err != nil {
			t.Fatalf("GetCellFormula Z%d: %v", r, err)
		}
		want := fmt.Sprintf("Y%d*X%d", r, r)
		if got != want {
			t.Errorf("Z%d = %q, want %q", r, got, want)
		}
	}
}

func TestSplitWorkbook_GiữNguyênKhốiTiêuĐềVàÔGộp(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dondathang.xlsx")
	dst := filepath.Join(dir, "out.xlsx")
	buildWorkbook(t, src, 5)

	if err := SplitWorkbook(src, dst, []int{10}); err != nil {
		t.Fatalf("SplitWorkbook: %v", err)
	}

	f, err := excelize.OpenFile(dst)
	if err != nil {
		t.Fatalf("OpenFile dst: %v", err)
	}
	defer f.Close()

	for cell, want := range map[string]string{
		"A1": "FILE MẪU ĐƠN ĐẶT HÀNG",
		"Q7": "Chi tiết hàng tiền",
		"A8": "Ngày đơn hàng (*)",
		"B8": "Số đơn hàng (*)",
	} {
		got, err := f.GetCellValue(SheetName, cell)
		if err != nil {
			t.Fatalf("GetCellValue %s: %v", cell, err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q — khối tiêu đề mẫu MISA phải nguyên vẹn", cell, got, want)
		}
	}

	merged, err := f.GetMergeCells(SheetName)
	if err != nil {
		t.Fatalf("GetMergeCells: %v", err)
	}
	if len(merged) != 1 || merged[0].GetStartAxis() != "Q7" || merged[0].GetEndAxis() != "AP7" {
		t.Errorf("ô gộp = %#v, want đúng một ô Q7:AP7", merged)
	}
}

func TestSplitWorkbook_KhôngSửaFileNguồn(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dondathang.xlsx")
	dst := filepath.Join(dir, "out.xlsx")
	buildWorkbook(t, src, 5)

	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile trước: %v", err)
	}
	if err := SplitWorkbook(src, dst, []int{9}); err != nil {
		t.Fatalf("SplitWorkbook: %v", err)
	}
	after, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile sau: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("file nguồn bị sửa — SplitWorkbook chỉ được đọc nguồn, mọi thay đổi phải rơi vào bản sao")
	}
}

func TestSplitWorkbook_KeepRỗngLàLỗi(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dondathang.xlsx")
	dst := filepath.Join(dir, "out.xlsx")
	buildWorkbook(t, src, 3)

	if err := SplitWorkbook(src, dst, nil); err == nil {
		t.Error("SplitWorkbook với keep rỗng = nil, want lỗi")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("SplitWorkbook để lại file rác khi keep rỗng")
	}
}

func TestSplitWorkbook_ChỉSốNgoàiPhạmViLàLỗi(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dondathang.xlsx")
	buildWorkbook(t, src, 3) // r9..r11

	for _, bad := range [][]int{{8}, {12}, {9, 99}} {
		dst := filepath.Join(dir, fmt.Sprintf("out-%d.xlsx", bad[len(bad)-1]))
		if err := SplitWorkbook(src, dst, bad); err == nil {
			t.Errorf("SplitWorkbook keep=%v = nil, want lỗi", bad)
		}
	}
}
