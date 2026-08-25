package excelwriter

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xuri/excelize/v2"
)

// newTemplate dựng 1 dondathang.xlsx rỗng: sheet "Don dat hang" với 8
// dòng tiêu đề của khuôn AMIS, dữ liệu bắt đầu từ dòng 9 — đúng như
// ClearOrderRows giả định.
func newTemplate(t *testing.T) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	if _, err := f.NewSheet(sheetName); err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	f.DeleteSheet("Sheet1")
	for r := 1; r <= 8; r++ {
		cell, _ := excelize.CoordinatesToCellName(1, r)
		if err := f.SetCellValue(sheetName, cell, "tiêu đề"); err != nil {
			t.Fatalf("SetCellValue: %v", err)
		}
	}
	path := filepath.Join(t.TempDir(), "dondathang.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	return path
}

func TestWriteTMDTRows(t *testing.T) {
	path := newTemplate(t)

	rows := []TMDTRow{{
		EntryDate:    "23/08/2026",
		OrderNumber:  "ĐĐHTMĐT-TikTok-585694438276170905",
		ShipTo:       "HN",
		CustomerCode: "MN_TMDT_00016",
		Description:  "TMĐT-TikTok - Tẩy lồng máy giặt Blue - 585694438276170905 - Ngày đổ 23/08/2026 - HN",
		SKU:          "TP10127",
		ProductName:  "Bột tẩy lồng Blue Túi 150g - MỚI SẢN XUẤT",
		Warehouse:    "TP_HN_12",
		Qty:          1,
		UnitPrice:    26851.85185185185,
		RegionCode:   "TMĐT_MB",
		StatCode:     "HN",
		Note:         "585694438276170905",
	}, {
		EntryDate:    "23/08/2026",
		OrderNumber:  "ĐĐHTMĐT-Shopee-2608235QED370T",
		ShipTo:       "LA",
		CustomerCode: "MN_TMDT_00003",
		Description:  "TMĐT-Shopee - Blue Việt Nam - 2608235QED370T - Ngày đổ 23/08/2026 - LA",
		SKU:          "TP32743",
		ProductName:  "Bột tẩy lồng máy giặt Blue 150g",
		IsPromoItem:  true,
		Warehouse:    "LA_KHOTMDT",
		Qty:          1,
		UnitPrice:    0,
		RegionCode:   "TMĐT_MN",
		StatCode:     "LA",
		Note:         "2608235QED370T",
	}}

	startRow, err := WriteTMDTRows(path, rows)
	if err != nil {
		t.Fatalf("WriteTMDTRows: %v", err)
	}
	if startRow != 9 {
		t.Fatalf("startRow = %d, muốn 9", startRow)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("mở lại file: %v", err)
	}
	defer f.Close()

	want := map[string]string{
		"A9": "23/08/2026", "B9": "ĐĐHTMĐT-TikTok-585694438276170905",
		"C9": "Chưa thực hiện", "D9": "23/08/2026", "E9": "HN",
		"G9": "MN_TMDT_00016", "Q9": "TP10127",
		"S9": "Bột tẩy lồng Blue Túi 150g - MỚI SẢN XUẤT",
		"T9": "Không", "U9": "Không", "V9": "TP_HN_12", "X9": "1",
		"AE9": "8", "AJ9": "TMĐT_MB", "AM9": "HN",
		"AO9": "585694438276170905", "AV9": "15",
		// Dòng thứ hai: hàng tặng, kho Long An.
		"U10": "Có", "Y10": "0", "V10": "LA_KHOTMDT", "AJ10": "TMĐT_MN",
	}
	for cell, expect := range want {
		got, err := f.GetCellValue(sheetName, cell)
		if err != nil {
			t.Fatalf("GetCellValue(%s): %v", cell, err)
		}
		if got != expect {
			t.Errorf("%s = %q, muốn %q", cell, got, expect)
		}
	}

	// Y9 (đơn giá): excelize có thể render số thực với số chữ số có nghĩa
	// khác nhau, nên so sánh bằng giá trị số với sai số 1e-6 thay vì so
	// chuỗi tuyệt đối.
	const wantY9 = 26851.85185185185
	gotY9Str, err := f.GetCellValue(sheetName, "Y9")
	if err != nil {
		t.Fatalf("GetCellValue(Y9): %v", err)
	}
	gotY9, err := strconv.ParseFloat(gotY9Str, 64)
	if err != nil {
		t.Fatalf("Y9 = %q không parse được thành số: %v", gotY9Str, err)
	}
	if diff := gotY9 - wantY9; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("Y9 = %v, muốn %v (lệch %v)", gotY9, wantY9, diff)
	}

	// Z (Thành tiền), AT (Trọng lượng), AU (số thùng) PHẢI trống — mẫu
	// chuẩn TMĐT để trống cả ba, khác hẳn writeRow của nhánh vendor.
	for _, cell := range []string{"Z9", "AT9", "AU9", "Z10", "AT10", "AU10"} {
		got, err := f.GetCellValue(sheetName, cell)
		if err != nil {
			t.Fatalf("GetCellValue(%s): %v", cell, err)
		}
		if got != "" {
			t.Errorf("%s = %q, muốn trống", cell, got)
		}
	}
}

func TestWriteTMDTRowsAppendsAfterExistingRows(t *testing.T) {
	// Một batch có thể ghi file PDF vendor trước rồi mới tới file TMĐT:
	// dòng TMĐT phải nối tiếp, không đè.
	path := newTemplate(t)
	if _, err := WriteTMDTRows(path, []TMDTRow{{EntryDate: "22/08/2026", SKU: "TP1"}}); err != nil {
		t.Fatalf("lần ghi 1: %v", err)
	}
	startRow, err := WriteTMDTRows(path, []TMDTRow{{EntryDate: "23/08/2026", SKU: "TP2"}})
	if err != nil {
		t.Fatalf("lần ghi 2: %v", err)
	}
	if startRow != 10 {
		t.Fatalf("startRow lần 2 = %d, muốn 10", startRow)
	}
	f, _ := excelize.OpenFile(path)
	defer f.Close()
	if got, _ := f.GetCellValue(sheetName, "Q9"); got != "TP1" {
		t.Errorf("Q9 = %q, muốn TP1 (dòng cũ đã bị đè)", got)
	}
	if got, _ := f.GetCellValue(sheetName, "Q10"); got != "TP2" {
		t.Errorf("Q10 = %q, muốn TP2", got)
	}
}

func TestWriteTMDTRowsEmptyIsNoop(t *testing.T) {
	path := newTemplate(t)
	startRow, err := WriteTMDTRows(path, nil)
	if err != nil {
		t.Fatalf("WriteTMDTRows(nil): %v", err)
	}
	if startRow != 9 {
		t.Errorf("startRow = %d, muốn 9", startRow)
	}
}
