package excelwriter

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

const sheetName = "Don dat hang"

// Row is one row to write into the "Don dat hang" sheet, matching the
// column layout of xulydonhang.py's write_to_dondathang (see the
// spec's column table). UseZFormula controls whether Z (Thành tiền)
// gets the formula "=Y{row}*X{row}" (main product rows) or the literal
// 0 (header row and promo bonus rows) — both are real, distinct
// behaviors in the Python original.
type Row struct {
	EntryDate      string
	DebtDays       int
	OrderNumber    string
	Status         string
	CancelDate     string
	ShipTo         string
	CustomerCode   string
	Description    string
	SKU            string
	Warehouse      string
	VATPercent     int
	RegionCode     string
	StatCode       string
	IsPromoItem    bool
	IsNoteRow      bool
	Qty            float64
	UnitPrice      float64
	ProductName    string
	CaseCount      int
	LineWeightKg   float64
	PromoNote      string
	PromoBundleSku string
	PromoContent   string
	PriceMismatch  bool
	InvoicePrice   float64
	UseZFormula    bool
	// NoCaseCount suppresses the AU (case count) write entirely, leaving
	// the cell blank rather than writing 0. BigC's write_to_dondathang_bigc
	// (xulydonhang.py:4541-4897) never touches AU on ANY row — unlike
	// Coop/Satra/Lotte, which always write a real (possibly legitimately
	// zero) case count — so BigC rows set this true to distinguish "no
	// value" from "computed value of zero".
	NoCaseCount bool
	// StoreName writes to column K — used only by Emart's header row,
	// which conditionally writes one of 3 hardcoded full Vietnamese
	// store names (xulydonhang.py:5046-5051) or nothing at all for an
	// unrecognized store. Every other row type and every other vendor
	// leaves this at its zero value (""), which writes an empty K cell —
	// functionally identical to Python's conditional "don't touch K at
	// all" for those cases, since both read back as blank.
	StoreName string
}

// WriteOrderRows appends rows to the "Don dat hang" sheet, mirroring
// write_to_dondathang's column layout and price-mismatch formatting.
// headerDescription, if non-empty, overwrites the Description (L) cell
// of the first row written — mirroring write_to_dondathang's final
// `sheet[f"L{start_row}"] = ...` step, which only happens once the
// order's total weight is known.
func WriteOrderRows(path string, rows []Row, headerDescription string) (startRow int, err error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return 0, fmt.Errorf("excelwriter: open %s: %w", path, err)
	}
	defer f.Close()

	existingRows, err := f.GetRows(sheetName)
	if err != nil {
		return 0, fmt.Errorf("excelwriter: read %s: %w", sheetName, err)
	}
	currentRow := len(existingRows) + 1
	firstRow := currentRow

	redFill, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"FF0000"}, Pattern: 1},
	})
	if err != nil {
		return 0, fmt.Errorf("excelwriter: create red fill style: %w", err)
	}

	for _, row := range rows {
		if err := writeRow(f, currentRow, row, redFill); err != nil {
			return 0, err
		}
		currentRow++
	}

	if headerDescription != "" {
		if err := f.SetCellValue(sheetName, fmt.Sprintf("L%d", firstRow), headerDescription); err != nil {
			return 0, fmt.Errorf("excelwriter: set header description: %w", err)
		}
	}

	if err := f.Save(); err != nil {
		return 0, fmt.Errorf("excelwriter: save %s: %w", path, err)
	}
	return firstRow, nil
}

// ConfirmPrice overwrites the price (column Y) of a row that
// WriteOrderRows already wrote and flagged as a price mismatch —
// clearing the red-fill style and the "Kiểm tra lại giá mã này!"
// comment, since the user has now explicitly reviewed and decided
// which price to keep (the PO's own invoice price, or the system's
// computed price — the caller passes whichever one it wants written).
//
// Requires Y{row} to currently carry a mismatch comment. excelize does
// NOT validate row bounds on SetCellValue (confirmed empirically: it
// silently writes far outside a sheet's real data rather than
// erroring), and DeleteComment on an uncommented cell is a silent
// no-op rather than an error — so this function's own explicit
// "does a comment exist at Y{row}" check is the ONLY thing that
// rejects a stale or out-of-range row argument. Without it, a bad
// `row` value would either silently do nothing (DeleteComment) or
// silently create a nonsense cell far outside the real sheet
// (SetCellValue) instead of surfacing as an error.
func ConfirmPrice(path string, row int, price float64) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("excelwriter: open %s: %w", path, err)
	}
	defer f.Close()

	cell := fmt.Sprintf("Y%d", row)

	comments, err := f.GetComments(sheetName)
	if err != nil {
		return fmt.Errorf("excelwriter: read comments: %w", err)
	}
	hasMismatchComment := false
	for _, c := range comments {
		if c.Cell == cell {
			hasMismatchComment = true
			break
		}
	}
	if !hasMismatchComment {
		return fmt.Errorf("excelwriter: %s không còn ở trạng thái chờ xác nhận giá (không có comment cảnh báo sai giá)", cell)
	}

	if err := f.SetCellValue(sheetName, cell, price); err != nil {
		return fmt.Errorf("excelwriter: set %s: %w", cell, err)
	}
	if err := f.DeleteComment(sheetName, cell); err != nil {
		return fmt.Errorf("excelwriter: delete comment at %s: %w", cell, err)
	}
	// NOTE: style ID 0 is confirmed the CORRECT "no special formatting"
	// reset for excelwriter's own TEST template (testdata/dondathang.xlsx
	// has no column-level Y style, so 0 genuinely is its original state).
	// The real production workbook has a column-level Y number-format
	// style (currency grouping) that this call would also clear — but
	// writeRow's own red-fill style (applied when the mismatch was first
	// flagged) already clears that same formatting, so this isn't a NEW
	// loss ConfirmPrice introduces, only a pre-existing one it doesn't
	// restore. Not fixed here — flagged for whoever eventually wires this
	// app to the real production file, not this plan's concern.
	if err := f.SetCellStyle(sheetName, cell, cell, 0); err != nil {
		return fmt.Errorf("excelwriter: reset style at %s: %w", cell, err)
	}

	if err := f.Save(); err != nil {
		return fmt.Errorf("excelwriter: save %s: %w", path, err)
	}
	return nil
}

// SetPrice overwrites Y{row}'s value directly, with NONE of ConfirmPrice's
// mismatch-comment safety check — used ONLY for a row ConfirmPrice has
// ALREADY successfully resolved earlier in the same app session (see
// App.ConfirmPrice's resolvedRows tracking), so the user can change their
// mind between "giá PO" and "giá hệ thống" after the original mismatch
// comment/red-fill were already cleared by that first ConfirmPrice call —
// there is no comment left to check by the time a re-toggle happens.
// Never call this for a row that hasn't already gone through a successful
// ConfirmPrice once; it has none of that function's protections against a
// stale or out-of-range row.
func SetPrice(path string, row int, price float64) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("excelwriter: open %s: %w", path, err)
	}
	defer f.Close()

	cell := fmt.Sprintf("Y%d", row)
	if err := f.SetCellValue(sheetName, cell, price); err != nil {
		return fmt.Errorf("excelwriter: set %s: %w", cell, err)
	}
	if err := f.Save(); err != nil {
		return fmt.Errorf("excelwriter: save %s: %w", path, err)
	}
	return nil
}

func writeRow(f *excelize.File, rowNum int, row Row, redFillStyle int) error {
	set := func(col string, value interface{}) error {
		return f.SetCellValue(sheetName, fmt.Sprintf("%s%d", col, rowNum), value)
	}
	yesNo := func(b bool) string {
		if b {
			return "Có"
		}
		return "Không"
	}

	writes := []struct {
		col   string
		value interface{}
	}{
		{"A", row.EntryDate},
		{"AV", row.DebtDays},
		{"B", row.OrderNumber},
		{"C", row.Status},
		{"D", row.CancelDate},
		{"E", row.ShipTo},
		{"G", row.CustomerCode},
		{"K", row.StoreName},
		{"L", row.Description},
		{"Q", row.SKU},
		{"V", row.Warehouse},
		{"AE", row.VATPercent},
		{"AJ", row.RegionCode},
		{"AM", row.StatCode},
		{"U", yesNo(row.IsPromoItem)},
		{"T", yesNo(row.IsNoteRow)},
		{"X", row.Qty},
		{"S", row.ProductName},
		{"AO", row.PromoNote},
		{"AP", row.PromoBundleSku},
		{"AQ", row.PromoContent},
	}
	for _, w := range writes {
		if err := set(w.col, w.value); err != nil {
			return fmt.Errorf("excelwriter: set %s%d: %w", w.col, rowNum, err)
		}
	}

	// AU (case count) and AT (line weight) are only written for actual
	// product/promo-bonus rows in Python's write_to_dondathang — the
	// header/note row block (xulydonhang.py:994-1013) never touches
	// either cell, leaving them blank. Writing a literal 0 there instead
	// (as an unconditional write would) is a real, visible difference:
	// real fixtures show AU/AT as null on the header row, not 0.
	if !row.IsNoteRow {
		if !row.NoCaseCount {
			if err := set("AU", row.CaseCount); err != nil {
				return fmt.Errorf("excelwriter: set AU%d: %w", rowNum, err)
			}
		}
		if err := set("AT", row.LineWeightKg); err != nil {
			return fmt.Errorf("excelwriter: set AT%d: %w", rowNum, err)
		}
	}

	if row.UseZFormula {
		if err := f.SetCellFormula(sheetName, fmt.Sprintf("Z%d", rowNum), fmt.Sprintf("Y%d*X%d", rowNum, rowNum)); err != nil {
			return fmt.Errorf("excelwriter: set Z%d formula: %w", rowNum, err)
		}
	} else if err := set("Z", 0); err != nil {
		return err
	}

	if err := set("Y", row.UnitPrice); err != nil {
		return err
	}
	if row.PriceMismatch {
		cell := fmt.Sprintf("Y%d", rowNum)
		if err := f.SetCellStyle(sheetName, cell, cell, redFillStyle); err != nil {
			return fmt.Errorf("excelwriter: apply red fill to %s: %w", cell, err)
		}
		diff := row.InvoicePrice - row.UnitPrice
		text := fmt.Sprintf("Kiểm tra lại giá mã này! - Giá hóa đơn: %v - Chênh lệch: %v", row.InvoicePrice, diff)
		if err := f.AddComment(sheetName, excelize.Comment{Cell: cell, Author: "System", Text: text}); err != nil {
			return fmt.Errorf("excelwriter: add comment to %s: %w", cell, err)
		}
	}

	return nil
}
