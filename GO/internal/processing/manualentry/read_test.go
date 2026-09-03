package manualentry

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

// writeSheet builds a minimal "Đơn hàng tay" workbook. Values are passed
// through SetCellValue as-is, so a time.Time lands in the file as a real
// Excel date cell (a serial number plus a date number-format) exactly as
// it does when a user types a date into Excel and it auto-formats.
func writeSheet(t *testing.T, rows [][]any) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	index, err := f.NewSheet(SheetName)
	if err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	f.SetActiveSheet(index)
	for r, row := range rows {
		for c, v := range row {
			cellName, err := excelize.CoordinatesToCellName(c+1, r+1)
			if err != nil {
				t.Fatalf("CoordinatesToCellName: %v", err)
			}
			if err := f.SetCellValue(SheetName, cellName, v); err != nil {
				t.Fatalf("SetCellValue: %v", err)
			}
		}
	}
	path := filepath.Join(t.TempDir(), "đơn hàng tay.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	return path
}

// TestLoad_RealExcelDateCellsBecomeDayMonthYearText covers a cell the user
// let Excel format as a DATE rather than typing as text. GetRows runs with
// RawCellValue (Qty/Đơn giá must not be rounded to their display format),
// and that hands back the underlying serial number — "46235", not
// "01/08/2026".
//
// That one value breaks two things at once downstream: it is written
// straight into the workbook's Ngày đặt / Hạn giao cells, and it is the
// timeToCheck that pricing.isWithinDateRange matches promo columns
// against — and a serial number matches no "D/M-D/M" column, so the order
// silently gets NO CTKM at all.
func TestLoad_RealExcelDateCellsBecomeDayMonthYearText(t *testing.T) {
	path := writeSheet(t, [][]any{
		{"PO", "Ngày đặt", "Hạn giao", "Hệ thống", "Mã khách hàng", "Nơi giao", "Mã hàng", "Số lượng", "Đơn giá"},
		{"PO-1", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC),
			"COOP", "KH-COOP-001", "Kho A", "SP0001", 10, 100000},
	})

	lines, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("Load returned %d lines, want 1", len(lines))
	}
	if lines[0].EntryDate != "01/08/2026" {
		t.Errorf("EntryDate = %q, want %q", lines[0].EntryDate, "01/08/2026")
	}
	if lines[0].CancelDate != "05/08/2026" {
		t.Errorf("CancelDate = %q, want %q", lines[0].CancelDate, "05/08/2026")
	}
}

// TestLoad_TextDatesAndEmptyCellsAreLeftAlone is the other half: a user who
// types the date as plain text already produces exactly the right string,
// and Hạn giao is optional, so neither may be touched by the serial-number
// conversion.
func TestLoad_TextDatesAndEmptyCellsAreLeftAlone(t *testing.T) {
	path := writeSheet(t, [][]any{
		{"PO", "Ngày đặt", "Hạn giao", "Hệ thống", "Mã khách hàng", "Nơi giao", "Mã hàng", "Số lượng", "Đơn giá"},
		{"PO-1", "01/08/2026", "", "COOP", "KH-COOP-001", "Kho A", "SP0001", 10, 100000},
	})

	lines, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("Load returned %d lines, want 1", len(lines))
	}
	if lines[0].EntryDate != "01/08/2026" {
		t.Errorf("EntryDate = %q, want %q unchanged", lines[0].EntryDate, "01/08/2026")
	}
	if lines[0].CancelDate != "" {
		t.Errorf("CancelDate = %q, want empty", lines[0].CancelDate)
	}
}

// TestLoad_QuantityAndPriceKeepTheirFullPrecision guards the reason
// RawCellValue is set in the first place: a price whose cell is formatted
// to fewer decimals must still load as the true stored number, not the
// rounded display string.
func TestLoad_QuantityAndPriceKeepTheirFullPrecision(t *testing.T) {
	path := writeSheet(t, [][]any{
		{"PO", "Ngày đặt", "Hạn giao", "Hệ thống", "Mã khách hàng", "Nơi giao", "Mã hàng", "Số lượng", "Đơn giá"},
		{"PO-1", "01/08/2026", "05/08/2026", "COOP", "KH-COOP-001", "Kho A", "SP0001", 3.475, 163540.25},
	})

	lines, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("Load returned %d lines, want 1", len(lines))
	}
	if lines[0].Qty != 3.475 {
		t.Errorf("Qty = %v, want 3.475", lines[0].Qty)
	}
	if lines[0].InvoicePrice != 163540.25 {
		t.Errorf("InvoicePrice = %v, want 163540.25", lines[0].InvoicePrice)
	}
}
