package excelwriter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func copyTestWorkbook(t *testing.T) string {
	t.Helper()
	src := "testdata/dondathang.xlsx"
	dst := filepath.Join(t.TempDir(), "dondathang.xlsx")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed reading test fixture: %v", err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("failed writing temp workbook: %v", err)
	}
	return dst
}

func TestWriteOrderRows_WritesColumnsAndFormula(t *testing.T) {
	path := copyTestWorkbook(t)

	rows := []Row{
		{EntryDate: "23/07/2026", OrderNumber: "ĐĐHCOOP-102945235-00", Status: "Chưa thực hiện", IsNoteRow: true, ProductName: "COOPMART PO102945235-00"},
		{SKU: "3564270-4", Qty: 24, UnitPrice: 33726, ProductName: "Chai tay toilet", UseZFormula: true},
	}

	if err := WriteOrderRows(path, rows, "COOPMART PO102945235-00 (Tổng trọng lượng: 4.32 kg)"); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()

	sku, _ := f.GetCellValue("Don dat hang", "Q10")
	if sku != "3564270-4" {
		t.Fatalf("Q10 = %q, want %q", sku, "3564270-4")
	}
	formula, _ := f.GetCellFormula("Don dat hang", "Z10")
	if formula != "Y10*X10" {
		t.Fatalf("Z10 formula = %q, want %q", formula, "Y10*X10")
	}
	desc, _ := f.GetCellValue("Don dat hang", "L9")
	if desc != "COOPMART PO102945235-00 (Tổng trọng lượng: 4.32 kg)" {
		t.Fatalf("L9 (header description) = %q, want the total-weight description", desc)
	}
}

func TestWriteOrderRows_WritesStoreNameToColumnK(t *testing.T) {
	path := copyTestWorkbook(t)

	rows := []Row{
		{EntryDate: "05/08/2026", OrderNumber: "ĐĐHEMART-4501866956", Status: "Chưa thực hiện", IsNoteRow: true, ProductName: "EMART PO4501866956", StoreName: "SIÊU THỊ EMART PHAN VĂN TRỊ"},
		{SKU: "8936156731203", Qty: 48, UnitPrice: 26950, ProductName: "Nước giặt Blue", UseZFormula: true},
	}

	if err := WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()

	headerK, _ := f.GetCellValue("Don dat hang", "K9")
	if headerK != "SIÊU THỊ EMART PHAN VĂN TRỊ" {
		t.Fatalf("K9 (header StoreName) = %q, want %q", headerK, "SIÊU THỊ EMART PHAN VĂN TRỊ")
	}
	productK, _ := f.GetCellValue("Don dat hang", "K10")
	if productK != "" {
		t.Fatalf("K10 (product row, StoreName unset) = %q, want empty", productK)
	}
}

func TestWriteOrderRows_PriceMismatchGetsRedFillAndComment(t *testing.T) {
	path := copyTestWorkbook(t)

	rows := []Row{
		{SKU: "3564270-4", Qty: 24, UnitPrice: 33000, InvoicePrice: 33726, PriceMismatch: true, UseZFormula: true},
	}
	if err := WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()

	styleID, err := f.GetCellStyle("Don dat hang", "Y9")
	if err != nil {
		t.Fatalf("GetCellStyle returned error: %v", err)
	}
	if styleID == 0 {
		t.Fatal("Y9 has default style, want the red-fill mismatch style applied")
	}

	comment, err := f.GetComments("Don dat hang")
	if err != nil {
		t.Fatalf("GetComments returned error: %v", err)
	}
	if len(comment) != 1 {
		t.Fatalf("comments = %d, want 1: %+v", len(comment), comment)
	}
}
