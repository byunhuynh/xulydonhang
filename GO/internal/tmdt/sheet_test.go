package tmdt

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestWriteHaravanSheetReplacesContent(t *testing.T) {
	// Workbook giống file thật: 2 bảng tra cứu + 1 sheet Haravan có rác cũ.
	f := excelize.NewFile()
	for _, s := range []string{"Mã misa", "data shop", SheetHaravan} {
		if _, err := f.NewSheet(s); err != nil {
			t.Fatalf("NewSheet(%q): %v", s, err)
		}
	}
	f.DeleteSheet("Sheet1")
	if err := f.SetCellValue("data shop", "A1", "Tên sản phẩm"); err != nil {
		t.Fatalf("SetCellValue: %v", err)
	}
	// Rác của lần chạy trước, phải bị xoá sạch.
	for r := 1; r <= 40; r++ {
		cell, _ := excelize.CoordinatesToCellName(1, r)
		if err := f.SetCellValue(SheetHaravan, cell, "rác cũ"); err != nil {
			t.Fatalf("SetCellValue: %v", err)
		}
	}
	path := filepath.Join(t.TempDir(), "XUẤT HÀNG HN-LA MỚI.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	f.Close()

	rows := []SheetRow{{
		OrderCode: "585694438276170905", Subtotal: 29000, Total: 29000,
		OrderDate: "2026-08-23T23:56:56+07:00", Quantity: 1,
		Title: "Bột Tẩy Lồng", VariantTitle: "1 TÚI", Price: 29000,
		SKU: "", Attributes: "Tên : Giá trị", KhoBan: "Kho Hà Nội",
		KenhBanHang: "tiktokshop",
		CreatedAt:   time.Date(2026, 8, 23, 23, 56, 56, 0, time.FixedZone("ICT", 7*3600)),
		TP:          [4]string{"TP10127", "", "", ""},
		SL:          [4]string{"1", "", "", ""},
		Shop:        "Tẩy lồng máy giặt Blue", Misa: "MN_TMDT_00016",
	}}
	if err := WriteHaravanSheet(path, rows); err != nil {
		t.Fatalf("WriteHaravanSheet: %v", err)
	}

	got, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("mở lại: %v", err)
	}
	defer got.Close()

	// Hai bảng tra cứu KHÔNG được mất — người dùng khai sản phẩm ở đó.
	if v, _ := got.GetCellValue("data shop", "A1"); v != "Tên sản phẩm" {
		t.Errorf("data shop!A1 = %q — bảng tra cứu bị hỏng", v)
	}
	if _, err := got.GetSheetIndex("Mã misa"); err != nil {
		t.Errorf("sheet Mã misa biến mất: %v", err)
	}

	// Tiêu đề đúng 23 cột.
	if len(HaravanHeaders) != 23 {
		t.Fatalf("HaravanHeaders có %d cột, muốn 23", len(HaravanHeaders))
	}
	for i, h := range HaravanHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if v, _ := got.GetCellValue(SheetHaravan, cell); v != h {
			t.Errorf("%s = %q, muốn %q", cell, v, h)
		}
	}

	want := map[string]string{
		"A2": "585694438276170905", "E2": "1", "F2": "Bột Tẩy Lồng",
		"G2": "1 TÚI", "H2": "29000", "I2": "", "J2": "Tên : Giá trị",
		"K2": "Kho Hà Nội", "L2": "tiktokshop",
		"N2": "TP10127", "O2": "1", "P2": "",
		"V2": "Tẩy lồng máy giặt Blue", "W2": "MN_TMDT_00016",
	}
	for cell, expect := range want {
		v, err := got.GetCellValue(SheetHaravan, cell)
		if err != nil {
			t.Fatalf("GetCellValue(%s): %v", cell, err)
		}
		if v != expect {
			t.Errorf("%s = %q, muốn %q", cell, v, expect)
		}
	}

	// Rác cũ ở dòng 3..40 phải sạch.
	for r := 3; r <= 40; r++ {
		cell, _ := excelize.CoordinatesToCellName(1, r)
		if v, _ := got.GetCellValue(SheetHaravan, cell); v != "" {
			t.Errorf("%s = %q — rác của lần chạy trước chưa bị xoá", cell, v)
		}
	}
}

func TestWriteHaravanSheetCreatesSheetWhenMissing(t *testing.T) {
	f := excelize.NewFile()
	for _, s := range []string{"Mã misa", "data shop"} {
		if _, err := f.NewSheet(s); err != nil {
			t.Fatalf("NewSheet: %v", err)
		}
	}
	f.DeleteSheet("Sheet1")
	path := filepath.Join(t.TempDir(), "khong-co-sheet-haravan.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	f.Close()

	if err := WriteHaravanSheet(path, nil); err != nil {
		t.Fatalf("WriteHaravanSheet: %v", err)
	}
	got, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("mở lại: %v", err)
	}
	defer got.Close()
	if v, _ := got.GetCellValue(SheetHaravan, "A1"); v != HaravanHeaders[0] {
		t.Errorf("sheet Haravan chưa được tạo kèm tiêu đề, A1 = %q", v)
	}
}

func TestWriteHaravanSheetKeepsMiddlePosition(t *testing.T) {
	// "Haravan" nằm GIỮA hai bảng tra cứu — người dùng có thể đã tự kéo tab
	// sang vị trí này trong Excel. WriteHaravanSheet xoá rồi tạo lại sheet
	// (excelize.NewSheet luôn thêm vào CUỐI), nên phải tự khôi phục đúng vị
	// trí cũ, không được lặng lẽ kéo tab ra cuối workbook mỗi lần chạy.
	f := excelize.NewFile()
	for _, s := range []string{"Mã misa", SheetHaravan, "data shop"} {
		if _, err := f.NewSheet(s); err != nil {
			t.Fatalf("NewSheet(%q): %v", s, err)
		}
	}
	f.DeleteSheet("Sheet1")
	path := filepath.Join(t.TempDir(), "haravan-o-giua.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	f.Close()

	rows := []SheetRow{{OrderCode: "ĐH001", Title: "Sản phẩm test"}}
	if err := WriteHaravanSheet(path, rows); err != nil {
		t.Fatalf("WriteHaravanSheet: %v", err)
	}

	got, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("mở lại: %v", err)
	}
	defer got.Close()

	wantOrder := []string{"Mã misa", SheetHaravan, "data shop"}
	if gotOrder := got.GetSheetList(); !slices.Equal(gotOrder, wantOrder) {
		t.Errorf("thứ tự sheet = %v, muốn %v — Haravan bị kéo khỏi vị trí cũ", gotOrder, wantOrder)
	}
	// Vị trí đúng nhưng dữ liệu sai thì cũng vô nghĩa — kiểm luôn cả hai.
	if v, _ := got.GetCellValue(SheetHaravan, "A2"); v != "ĐH001" {
		t.Errorf("A2 = %q, muốn ĐH001 — dữ liệu chưa ghi đúng dù đã khôi phục vị trí sheet", v)
	}
}

func TestWriteHaravanSheetKeepsLastPositionWhenAlreadyLast(t *testing.T) {
	// Trường hợp file thật hiện tại: "Haravan" đã là sheet cuối cùng — thứ
	// tự này phải giữ nguyên, không tự dưng đổi.
	f := excelize.NewFile()
	for _, s := range []string{"Mã misa", "data shop", SheetHaravan} {
		if _, err := f.NewSheet(s); err != nil {
			t.Fatalf("NewSheet(%q): %v", s, err)
		}
	}
	f.DeleteSheet("Sheet1")
	path := filepath.Join(t.TempDir(), "haravan-cuoi.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	f.Close()

	if err := WriteHaravanSheet(path, nil); err != nil {
		t.Fatalf("WriteHaravanSheet: %v", err)
	}

	got, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("mở lại: %v", err)
	}
	defer got.Close()

	wantOrder := []string{"Mã misa", "data shop", SheetHaravan}
	if gotOrder := got.GetSheetList(); !slices.Equal(gotOrder, wantOrder) {
		t.Errorf("thứ tự sheet = %v, muốn %v không đổi", gotOrder, wantOrder)
	}
}
