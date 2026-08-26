package excelwriter

import (
	"fmt"
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

	startRow, err := WriteOrderRows(path, rows, "COOPMART PO102945235-00 (Tổng trọng lượng: 4.32 kg)")
	if err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}
	if startRow != 9 {
		t.Fatalf("startRow = %d, want 9 (the test template has 8 existing header rows)", startRow)
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

func TestClearOrderRows_DeletesDataRowsKeepsHeader(t *testing.T) {
	path := copyTestWorkbook(t)

	rows := []Row{
		{EntryDate: "23/07/2026", OrderNumber: "ĐĐHCOOP-1", Status: "Chưa thực hiện", IsNoteRow: true, ProductName: "COOPMART PO1"},
		{SKU: "1111111-1", Qty: 1, UnitPrice: 1000, ProductName: "Sản phẩm A"},
		{SKU: "2222222-2", Qty: 2, UnitPrice: 2000, ProductName: "Sản phẩm B"},
	}
	if _, err := WriteOrderRows(path, rows, "COOPMART PO1"); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	headerBefore, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	rowsBefore, _ := headerBefore.GetRows("Don dat hang")
	headerBefore.Close()
	if len(rowsBefore) != 11 {
		t.Fatalf("rows before clear = %d, want 11 (8 header + 3 written)", len(rowsBefore))
	}

	if err := ClearOrderRows(path); err != nil {
		t.Fatalf("ClearOrderRows returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook after clear: %v", err)
	}
	defer f.Close()

	rowsAfter, err := f.GetRows("Don dat hang")
	if err != nil {
		t.Fatalf("GetRows returned error: %v", err)
	}
	if len(rowsAfter) != 8 {
		t.Fatalf("rows after clear = %d, want 8 (header only, all data rows removed)", len(rowsAfter))
	}

	header, _ := f.GetCellValue("Don dat hang", "A8")
	if header != "STT" {
		t.Fatalf("A8 (header row) = %q, want the header text to survive the clear untouched", header)
	}
}

func TestClearOrderRowsDeletesCommentsSoSameRowsCanBeReused(t *testing.T) {
	path := copyTestWorkbook(t)
	mismatch := []Row{{
		SKU: "TP30671", Qty: 1, UnitPrice: 24537, InvoicePrice: 25000,
		PriceMismatch: true, UseZFormula: true,
	}}
	if _, err := WriteOrderRows(path, mismatch, ""); err != nil {
		t.Fatalf("first WriteOrderRows returned error: %v", err)
	}
	if err := ClearOrderRows(path); err != nil {
		t.Fatalf("ClearOrderRows returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	comments, err := f.GetComments("Don dat hang")
	f.Close()
	if err != nil {
		t.Fatalf("GetComments returned error: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments after clear = %+v, want none", comments)
	}

	if _, err := WriteOrderRows(path, mismatch, ""); err != nil {
		t.Fatalf("second WriteOrderRows must reuse Y9 without an existing-comment error: %v", err)
	}
}

func TestUpdateJITPeriodUpdatesOnlyRowsBelongingToSelectedPDF(t *testing.T) {
	path := copyTestWorkbook(t)
	rows := []Row{
		{OrderNumber: "old-jit-1", Description: "old-description-1"},
		{OrderNumber: "old-jit-2", Description: "old-description-2"},
		{OrderNumber: "other-file", Description: "other-description"},
	}
	if _, err := WriteOrderRows(path, rows, ""); err != nil {
		t.Fatal(err)
	}
	if err := UpdateJITPeriod(path, []int{9, 10}, "24/08/2026", "WH6_HN", "Tối"); err != nil {
		t.Fatal(err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, row := range []int{9, 10} {
		orderNumber, _ := f.GetCellValue("Don dat hang", fmt.Sprintf("B%d", row))
		description, _ := f.GetCellValue("Don dat hang", fmt.Sprintf("L%d", row))
		if orderNumber != "ĐĐHJIT-24/08/2026 (tối)-WH6_HN" {
			t.Errorf("B%d = %q", row, orderNumber)
		}
		if description != "JIT-CHOICE Ngày đổ 24/08/2026 (tối) WH6_HN" {
			t.Errorf("L%d = %q", row, description)
		}
	}
	otherOrder, _ := f.GetCellValue("Don dat hang", "B11")
	otherDescription, _ := f.GetCellValue("Don dat hang", "L11")
	if otherOrder != "other-file" || otherDescription != "other-description" {
		t.Fatalf("unselected row changed: B11=%q L11=%q", otherOrder, otherDescription)
	}
}

func TestUpdateJITPeriodRejectsUnknownPeriod(t *testing.T) {
	path := copyTestWorkbook(t)
	if err := UpdateJITPeriod(path, []int{9}, "24/08/2026", "WH6_HN", "Khuya"); err == nil {
		t.Fatal("UpdateJITPeriod returned nil for an unsupported period")
	}
}

func TestClearOrderRows_NoOpWhenNoDataRows(t *testing.T) {
	path := copyTestWorkbook(t)

	if err := ClearOrderRows(path); err != nil {
		t.Fatalf("ClearOrderRows on an already-empty template returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()

	rows, err := f.GetRows("Don dat hang")
	if err != nil {
		t.Fatalf("GetRows returned error: %v", err)
	}
	if len(rows) != 8 {
		t.Fatalf("rows = %d, want 8 (unchanged, nothing to clear)", len(rows))
	}
}

func TestWriteOrderRows_WritesStoreNameToColumnK(t *testing.T) {
	path := copyTestWorkbook(t)

	rows := []Row{
		{EntryDate: "05/08/2026", OrderNumber: "ĐĐHEMART-4501866956", Status: "Chưa thực hiện", IsNoteRow: true, ProductName: "EMART PO4501866956", StoreName: "SIÊU THỊ EMART PHAN VĂN TRỊ"},
		{SKU: "8936156731203", Qty: 48, UnitPrice: 26950, ProductName: "Nước giặt Blue", UseZFormula: true},
	}

	if _, err := WriteOrderRows(path, rows, ""); err != nil {
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
	if _, err := WriteOrderRows(path, rows, ""); err != nil {
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

func TestConfirmPrice_OverwritesValueAndClearsMismatchFlag(t *testing.T) {
	path := copyTestWorkbook(t)
	rows := []Row{
		{SKU: "3564270-4", Qty: 24, UnitPrice: 33000, InvoicePrice: 33726, PriceMismatch: true, UseZFormula: true},
	}
	if _, err := WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	if err := ConfirmPrice(path, 9, 33726); err != nil {
		t.Fatalf("ConfirmPrice returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()

	val, err := f.GetCellValue("Don dat hang", "Y9")
	if err != nil {
		t.Fatalf("GetCellValue: %v", err)
	}
	if val != "33726" {
		t.Fatalf("Y9 = %q, want %q", val, "33726")
	}

	styleID, err := f.GetCellStyle("Don dat hang", "Y9")
	if err != nil {
		t.Fatalf("GetCellStyle: %v", err)
	}
	if styleID != 0 {
		t.Fatalf("Y9 style = %d, want 0 (red-fill mismatch style cleared)", styleID)
	}

	comments, err := f.GetComments("Don dat hang")
	if err != nil {
		t.Fatalf("GetComments: %v", err)
	}
	for _, c := range comments {
		if c.Cell == "Y9" {
			t.Fatalf("comment still present at Y9 after ConfirmPrice: %+v", c)
		}
	}
}

func TestConfirmPrice_RejectsRowWithNoMismatchComment(t *testing.T) {
	path := copyTestWorkbook(t)
	rows := []Row{
		{SKU: "3564270-4", Qty: 24, UnitPrice: 33726, ProductName: "Chai tay toilet", UseZFormula: true},
	}
	if _, err := WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	err := ConfirmPrice(path, 9, 30000)
	if err == nil {
		t.Fatal("ConfirmPrice returned nil error, want a rejection — row 9 was never flagged as a price mismatch")
	}

	f, err2 := excelize.OpenFile(path)
	if err2 != nil {
		t.Fatalf("failed reopening workbook: %v", err2)
	}
	defer f.Close()
	val, _ := f.GetCellValue("Don dat hang", "Y9")
	if val != "33726" {
		t.Fatalf("Y9 = %q, want unchanged %q (rejected before any write)", val, "33726")
	}
}

func TestConfirmPrice_RejectsRowOutsideSheetBounds(t *testing.T) {
	path := copyTestWorkbook(t)
	rows := []Row{
		{SKU: "3564270-4", Qty: 24, UnitPrice: 33000, InvoicePrice: 33726, PriceMismatch: true, UseZFormula: true},
	}
	if _, err := WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	if err := ConfirmPrice(path, 99999, 33726); err == nil {
		t.Fatal("ConfirmPrice returned nil error for a row far outside the real sheet, want a rejection")
	}
}

func TestSetPrice_OverwritesValueWithNoCommentCheck(t *testing.T) {
	path := copyTestWorkbook(t)
	rows := []Row{
		{SKU: "3564270-4", Qty: 24, UnitPrice: 33726, ProductName: "Chai tay toilet", UseZFormula: true},
	}
	// This row has NO mismatch comment at all — SetPrice must still
	// succeed, unlike ConfirmPrice.
	if _, err := WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	if err := SetPrice(path, 9, 30000); err != nil {
		t.Fatalf("SetPrice returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()
	val, _ := f.GetCellValue("Don dat hang", "Y9")
	if val != "30000" {
		t.Fatalf("Y9 = %q, want %q", val, "30000")
	}
}

func TestWriteOrderRows_WritesCustomerBillingDetailsToColumnsHIJ(t *testing.T) {
	// H/I/J ("Tên khách hàng", "Địa chỉ", "Mã số thuế") are columns the
	// MISA template has always carried but nothing ever wrote. Maxidi
	// needs them because its two branches share one customer code while
	// invoicing under different names, addresses and tax codes, so the
	// customer code alone cannot tell the two apart in the accounting
	// system.
	path := copyTestWorkbook(t)

	rows := []Row{
		{
			EntryDate: "26/08/2026", OrderNumber: "ĐĐHMAXIDI-HO-PO00085936",
			Status: "Chưa thực hiện", IsNoteRow: true, ProductName: "MAXIDI HO-PO00085936",
			CustomerCode: "LA_GC_00002",
			CustomerName: "CHI NHÁNH BÌNH DƯƠNG - CÔNG TY TNHH MAXIDI VIỆT NAM",
			InvoiceAddress: "Khu A, Kho Liên Anh, số 189/8 Lê Hồng Phong, KP Tân Phước, " +
				"P.Tân Đông Hiệp, Hồ Chí Minh, Bình Dương",
			TaxCode: "0317899481-002",
		},
	}
	if _, err := WriteOrderRows(path, rows, "MAXIDI HO-PO00085936"); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()

	for _, c := range []struct{ cell, want string }{
		{"H9", "CHI NHÁNH BÌNH DƯƠNG - CÔNG TY TNHH MAXIDI VIỆT NAM"},
		{"I9", "Khu A, Kho Liên Anh, số 189/8 Lê Hồng Phong, KP Tân Phước, P.Tân Đông Hiệp, Hồ Chí Minh, Bình Dương"},
		{"J9", "0317899481-002"},
	} {
		got, _ := f.GetCellValue("Don dat hang", c.cell)
		if got != c.want {
			t.Errorf("%s = %q, want %q", c.cell, got, c.want)
		}
	}
}

func TestWriteOrderRows_LeavesColumnsHIJBlankWhenUnset(t *testing.T) {
	// Every vendor other than Maxidi leaves these three fields at their
	// zero value, and must keep writing blank cells there — MISA fills
	// the customer's own name/address/tax code from the customer code
	// when the columns are empty, so writing anything at all (even an
	// empty string is fine, but a placeholder would not be) must not
	// start happening by accident.
	path := copyTestWorkbook(t)

	rows := []Row{{SKU: "3564270-4", Qty: 24, UnitPrice: 33726, ProductName: "Chai tay toilet", UseZFormula: true}}
	if _, err := WriteOrderRows(path, rows, ""); err != nil {
		t.Fatalf("WriteOrderRows returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()

	for _, cell := range []string{"H9", "I9", "J9"} {
		got, _ := f.GetCellValue("Don dat hang", cell)
		if got != "" {
			t.Errorf("%s = %q, want an empty cell for a vendor that sets no billing details", cell, got)
		}
	}
}
