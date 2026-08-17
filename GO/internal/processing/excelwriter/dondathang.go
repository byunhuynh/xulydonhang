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
}

// WriteOrderRows appends rows to the "Don dat hang" sheet, mirroring
// write_to_dondathang's column layout and price-mismatch formatting.
// headerDescription, if non-empty, overwrites the Description (L) cell
// of the first row written — mirroring write_to_dondathang's final
// `sheet[f"L{start_row}"] = ...` step, which only happens once the
// order's total weight is known.
func WriteOrderRows(path string, rows []Row, headerDescription string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("excelwriter: open %s: %w", path, err)
	}
	defer f.Close()

	existingRows, err := f.GetRows(sheetName)
	if err != nil {
		return fmt.Errorf("excelwriter: read %s: %w", sheetName, err)
	}
	currentRow := len(existingRows) + 1
	firstRow := currentRow

	redFill, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"FF0000"}, Pattern: 1},
	})
	if err != nil {
		return fmt.Errorf("excelwriter: create red fill style: %w", err)
	}

	for _, row := range rows {
		if err := writeRow(f, currentRow, row, redFill); err != nil {
			return err
		}
		currentRow++
	}

	if headerDescription != "" {
		if err := f.SetCellValue(sheetName, fmt.Sprintf("L%d", firstRow), headerDescription); err != nil {
			return fmt.Errorf("excelwriter: set header description: %w", err)
		}
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
