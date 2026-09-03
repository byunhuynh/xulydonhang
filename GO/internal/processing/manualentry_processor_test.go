package processing

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"order-processor/internal/processing/manualentry"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"

	"github.com/xuri/excelize/v2"
)

// TestProcessManualEntryOrder_ScopesCoopPromotionsToTheCustomersSystem
// covers the manually-typed half of Coop's two-system CTKM rule. The
// typed sheet's "Hệ thống" column holds the VENDOR ("COOP"), not the
// Coopmart/Coopfood split, so — exactly like the PDF path does — the
// system has to be resolved from the customer code before any CTKM is
// applied. Without that, every Coopfood-only campaign was landing on
// manually-typed Coopmart orders and vice versa.
func TestProcessManualEntryOrder_ScopesCoopPromotionsToTheCustomersSystem(t *testing.T) {
	cases := []struct {
		name         string
		customerCode string
		promoCell    string
	}{
		// KH-CF-002 is the productdata fixture's Coopfood customer; any
		// other code falls to Coopmart, matching the PDF path's own
		// default branch.
		{"coopfood customer skips a Coopmart-only CTKM", "KH-CF-002", "CM Giảm 10% Tang SP0002 {Combo 2}"},
		{"coopmart customer skips a Coopfood-only CTKM", "KH-COOP-001", "CF Giảm 10% Tang SP0002 {Combo 2}"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store, err := productdata.Load("productdata/testdata/data.xlsx")
			if err != nil {
				t.Fatalf("Load productdata failed: %v", err)
			}
			excelPath := copyTestWorkbookForProcessor(t)

			// The typed invoice price equals the sheet's raw price, so
			// with the CTKM correctly skipped the line reconciles
			// cleanly; if it were applied its 10% would break that.
			priceCsv := [][]string{
				{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
				{"1", "SP0001", "Nước giặt Blue", "100000", c.promoCell},
			}
			pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

			lines := []manualentry.Line{{
				PO: "PO-TAY-1", EntryDate: "01/08/2026", CancelDate: "05/08/2026",
				System: "COOP", CustomerCode: c.customerCode, ShipTo: "Kho A",
				RawSKU: "SP0001", Qty: 10, InvoicePrice: 100000,
			}}

			rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
			row, err := rp.processManualEntryOrder("đơn hàng tay.xlsx", "PO-TAY-1", lines)
			if err != nil {
				t.Fatalf("processManualEntryOrder returned error: %v", err)
			}
			if row.PriceMismatchCount != 0 {
				t.Errorf("PriceMismatchCount = %d, want 0 (a CTKM of the other Coop system must not discount this price)", row.PriceMismatchCount)
			}

			f, err := excelize.OpenFile(excelPath)
			if err != nil {
				t.Fatalf("failed reopening written workbook: %v", err)
			}
			defer f.Close()
			sheetRows, err := f.GetRows("Don dat hang")
			if err != nil {
				t.Fatalf("failed reading Don dat hang rows: %v", err)
			}

			const colSKU, colPromoContent = 16, 42
			cell := func(row []string, idx int) string {
				if idx < len(row) {
					return row[idx]
				}
				return ""
			}
			for _, sheetRow := range sheetRows {
				switch cell(sheetRow, colSKU) {
				case "SP0002":
					t.Error("found an SP0002 gift row, want none (its CTKM belongs to the other Coop system)")
				case "SP0001":
					if got := cell(sheetRow, colPromoContent); got != "" {
						t.Errorf("product row PromoContent = %q, want empty", got)
					}
				}
			}
		})
	}
}

// TestProcessManualEntryOrder_NonCoopVendorKeepsEveryPromotion guards the
// blast radius of the scoping above: the manual-entry path is shared by
// every vendor, and only Coop has a Coopmart/Coopfood split. A Lotte or
// BigC promo cell that happens to contain the letters "cf"/"cm" must
// still be applied whole, never split or dropped.
func TestProcessManualEntryOrder_NonCoopVendorKeepsEveryPromotion(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	const promoValue = "CM Giảm 10% Tang SP0002 {Combo 2}"
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "SP0001", "Nước giặt Blue", "100000", promoValue},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	lines := []manualentry.Line{{
		PO: "PO-TAY-2", EntryDate: "01/08/2026", CancelDate: "05/08/2026",
		System: "LOTTE", CustomerCode: "KH-CF-002", ShipTo: "Kho A",
		RawSKU: "SP0001", Qty: 10, InvoicePrice: 90000,
	}}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	if _, err := rp.processManualEntryOrder("đơn hàng tay.xlsx", "PO-TAY-2", lines); err != nil {
		t.Fatalf("processManualEntryOrder returned error: %v", err)
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening written workbook: %v", err)
	}
	defer f.Close()
	sheetRows, err := f.GetRows("Don dat hang")
	if err != nil {
		t.Fatalf("failed reading Don dat hang rows: %v", err)
	}

	const colSKU, colPromoContent = 16, 42
	cell := func(row []string, idx int) string {
		if idx < len(row) {
			return row[idx]
		}
		return ""
	}
	sawGift := false
	for _, sheetRow := range sheetRows {
		switch cell(sheetRow, colSKU) {
		case "SP0002":
			sawGift = true
		case "SP0001":
			if got := cell(sheetRow, colPromoContent); got != promoValue {
				t.Errorf("product row PromoContent = %q, want the whole cell %q", got, promoValue)
			}
		}
	}
	if !sawGift {
		t.Error("missing the SP0002 gift row — a non-Coop vendor's CTKM must be applied whole")
	}
}

// recordingPricingSource is fixturePricingSource plus a note of which
// sheet key was asked for — the point of the test below.
type recordingPricingSource struct {
	index *pricing.Index
	keys  []string
}

func (r *recordingPricingSource) FetchIndex(sheetKey string) (*pricing.Index, error) {
	r.keys = append(r.keys, sheetKey)
	return r.index, nil
}

// TestProcessManualEntryOrder_AcceptsCoopmartAndCoopfoodAsTypedSystem
// lets the typed "Hệ thống" cell name the Coop sub-system directly
// instead of only the vendor. Typing COOPFOOD used to fail the whole
// order outright ("no COOPFOOD gid configured"), because the cell feeds
// the pricing sheet key — yet it is the natural thing to type once the
// two systems get different CTKM. Both systems share the COOP sheet, so
// the key stays "COOP" and only the CTKM scoping (and the Description
// text, as on the PDF path) follows what was typed.
//
// The typed value beats the customer-code lookup: KH-COOP-001 is not a
// Coopfood customer, and the Coopmart-only CTKM is skipped anyway.
func TestProcessManualEntryOrder_AcceptsCoopmartAndCoopfoodAsTypedSystem(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "1/1-31/12"},
		{"1", "SP0001", "Nước giặt Blue", "100000", "CM Giảm 10% Tang SP0002 {Combo 2}"},
	}
	pricingSource := &recordingPricingSource{index: pricing.ParseIndex(priceCsv)}

	lines := []manualentry.Line{{
		PO: "PO-TAY-3", EntryDate: "01/08/2026", CancelDate: "05/08/2026",
		System: "COOPFOOD", CustomerCode: "KH-COOP-001", ShipTo: "Kho A",
		RawSKU: "SP0001", Qty: 10, InvoicePrice: 100000,
	}}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	row, err := rp.processManualEntryOrder("đơn hàng tay.xlsx", "PO-TAY-3", lines)
	if err != nil {
		t.Fatalf("processManualEntryOrder returned error: %v", err)
	}
	if len(pricingSource.keys) != 1 || pricingSource.keys[0] != "COOP" {
		t.Errorf("FetchIndex called with %v, want exactly [COOP] (both systems share one sheet)", pricingSource.keys)
	}
	if row.PriceMismatchCount != 0 {
		t.Errorf("PriceMismatchCount = %d, want 0 (a Coopmart-only CTKM must not reach a Coopfood order)", row.PriceMismatchCount)
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening written workbook: %v", err)
	}
	defer f.Close()
	sheetRows, err := f.GetRows("Don dat hang")
	if err != nil {
		t.Fatalf("failed reading Don dat hang rows: %v", err)
	}

	const colOrderNumber, colDescription, colSKU = 1, 11, 16
	cell := func(row []string, idx int) string {
		if idx < len(row) {
			return row[idx]
		}
		return ""
	}
	sawOrder := false
	for _, sheetRow := range sheetRows {
		if cell(sheetRow, colSKU) == "SP0002" {
			t.Error("found an SP0002 gift row, want none (its CTKM is Coopmart-only)")
		}
		// The workbook's own header row lives in this column too.
		got := cell(sheetRow, colOrderNumber)
		if !strings.HasPrefix(got, "ĐĐH") {
			continue
		}
		sawOrder = true
		if got != "ĐĐHCOOP-PO-TAY-3" {
			t.Errorf("order number = %q, want %q (the vendor prefix, exactly as the PDF path writes it)", got, "ĐĐHCOOP-PO-TAY-3")
		}
		// The order's first row also carries the total-weight suffix
		// WriteOrderRows appends, hence the prefix check.
		if desc := cell(sheetRow, colDescription); !strings.HasPrefix(desc, "COOPFOOD PO-TAY-3") {
			t.Errorf("description = %q, want it to start with %q — the typed system names the order, as on the PDF path", desc, "COOPFOOD PO-TAY-3")
		}
	}
	if !sawOrder {
		t.Error("no rows written for the order")
	}
}

// TestProcessManualEntryDocument_AppliesCTKMWhenDatesAreRealExcelDateCells
// is the end-to-end half of the fix in manualentry.dateCell: it drives the
// whole manual-entry flow from a real .xlsx whose Ngày đặt / Hạn giao cells
// are genuine Excel date cells (the shape a user gets by simply typing a
// date and letting Excel format it), not text.
//
// Those cells load as raw serial numbers, and the entry date is what
// pricing matches promo columns against — so before the fix the order
// silently earned NO CTKM at all, and wrote "46235" into the workbook's
// Ngày đặt column. Both are asserted here.
func TestProcessManualEntryDocument_AppliesCTKMWhenDatesAreRealExcelDateCells(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	src := excelize.NewFile()
	defer src.Close()
	index, err := src.NewSheet(manualentry.SheetName)
	if err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	src.SetActiveSheet(index)
	rows := [][]any{
		{"PO", "Ngày đặt", "Hạn giao", "Hệ thống", "Mã khách hàng", "Nơi giao", "Mã hàng", "Số lượng", "Đơn giá"},
		{"PO-TAY-4", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC),
			"COOP", "KH-COOP-001", "Kho A", "SP0001", 10, 90000},
	}
	for r, row := range rows {
		for c, v := range row {
			cellName, err := excelize.CoordinatesToCellName(c+1, r+1)
			if err != nil {
				t.Fatalf("CoordinatesToCellName: %v", err)
			}
			if err := src.SetCellValue(manualentry.SheetName, cellName, v); err != nil {
				t.Fatalf("SetCellValue: %v", err)
			}
		}
	}
	srcPath := filepath.Join(t.TempDir(), "đơn hàng tay.xlsx")
	if err := src.SaveAs(srcPath); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}

	// The promo column is active across 01/08, and the typed price
	// matches its 10% discount, so a correctly-dated order both applies
	// the CTKM and reconciles cleanly.
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá", "01/07-31/08"},
		{"1", "SP0001", "Nước giặt Blue", "100000", "Giảm 10% Tang SP0002 {Combo 2}"},
	}
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(priceCsv)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	orderRows, err := rp.processManualEntryDocument(srcPath, func(OrderRow) {})
	if err != nil {
		t.Fatalf("processManualEntryDocument returned error: %v", err)
	}
	if len(orderRows) != 1 {
		t.Fatalf("got %d order rows, want 1: %+v", len(orderRows), orderRows)
	}
	if orderRows[0].StatusKind == StatusKindFailed {
		t.Fatalf("order failed: %s", orderRows[0].Status)
	}
	if orderRows[0].PriceMismatchCount != 0 {
		t.Errorf("PriceMismatchCount = %d, want 0 (the CTKM's discounted price matches the typed price)", orderRows[0].PriceMismatchCount)
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening written workbook: %v", err)
	}
	defer f.Close()
	sheetRows, err := f.GetRows("Don dat hang")
	if err != nil {
		t.Fatalf("failed reading Don dat hang rows: %v", err)
	}

	const colEntryDate, colCancelDate, colSKU = 0, 3, 16
	cell := func(row []string, idx int) string {
		if idx < len(row) {
			return row[idx]
		}
		return ""
	}
	sawGift := false
	sawProduct := false
	for _, sheetRow := range sheetRows {
		switch cell(sheetRow, colSKU) {
		case "SP0002":
			sawGift = true
		case "SP0001":
			sawProduct = true
			if got := cell(sheetRow, colEntryDate); got != "01/08/2026" {
				t.Errorf("Ngày đặt = %q, want %q", got, "01/08/2026")
			}
			if got := cell(sheetRow, colCancelDate); got != "05/08/2026" {
				t.Errorf("Hạn giao = %q, want %q", got, "05/08/2026")
			}
		}
	}
	if !sawProduct {
		t.Error("no product row written for the order")
	}
	if !sawGift {
		t.Error("missing the SP0002 gift row — the CTKM was not applied")
	}
}
