package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"order-processor/internal/driveupload"
	"order-processor/internal/pdfpage"
	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/maxidi"
)

// maxidiSystem is the system name this vendor's rows carry — the same
// string used as its key in the MaKH sheet's first column, in the MISA
// branch-routing table, and in vendor.Identify's return value. Kept as
// one constant so those three can never drift apart.
const maxidiSystem = "Maxidi"

// Warehouse (column V), business unit (AJ) and statistics code (AM) for
// every Maxidi order, as specified by the project owner. Fixed values
// rather than a per-region branch like Kingfood's or BigC's: both Maxidi
// branches deliver out of the same warehouse and book to the same unit,
// and the only thing that varies between them is who gets invoiced (see
// maxidi.Branch).
const (
	maxidiWarehouse = "GC_TP"
	maxidiRegion    = "OEM"
	maxidiStatCode  = "LA"
)

// maxidiOrderNumber builds column B, matching every other vendor's
// "ĐĐH<VENDOR>-<PO>" shape.
func maxidiOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHMAXIDI-%s", poNumber)
}

// maxidiDescription builds the "Diễn giải" (column L) text. The PO's own
// remarks — in practice a delivery-window instruction such as "THỜI GIAN
// GIAO HÀNG BUỔI SÁNG + CHIỀU" — are appended here at the project
// owner's request, so whoever picks the order reads them off the same
// column as the order reference.
func maxidiDescription(poNumber, remarks string) string {
	description := fmt.Sprintf("MAXIDI %s", poNumber)
	if remarks = strings.TrimSpace(remarks); remarks != "" {
		description += " - " + remarks
	}
	return description
}

// maxidiSkuLogLine reports one product's outcome. Deliberately NOT
// formatSkuLogLine: that helper's whole vocabulary ("Đúng giá" / "SAI
// GIÁ! ... Giá trên PO") describes comparing a PO's printed price
// against the system's, and a Maxidi delivery note prints no price at
// all, so every line it produced here would be answering a question the
// document never asked.
func maxidiSkuLogLine(sku, productName string, qty, unitPrice float64, promoText, promoDateRange string) string {
	label := sku
	if productName != "" {
		label = sku + " " + productName
	}
	line := fmt.Sprintf("%s — %.0f × %.0f (giá bảng, PO không in đơn giá)", label, qty, unitPrice)
	if promo := truncatePromoText(promoText); promo != "" {
		suffix := ""
		if promoDateRange != "" {
			suffix = fmt.Sprintf(" (%s)", promoDateRange)
		}
		line += fmt.Sprintf(" — ⚠️ có CTKM trên bảng giá, CHƯA áp dụng: %s%s", promo, suffix)
	}
	return line
}

// processMaxidiSegment turns one Maxidi delivery-note page into one
// order. Maxidi is "1 page = 1 order", the same family as Kingfood and
// JMart.
//
// It differs from every vendor before it in one structural way: the
// delivery note prints NO unit price, anywhere. Confirmed against all
// four real archived samples, and confirmed with the project owner as
// how this vendor's documents work. Three consequences follow, and they
// are why this is not a copy of processJMartSegment:
//
//  1. There is nothing to reconcile, so there is no price-mismatch path
//     at all — no red fill, no comment, no PriceMismatchDetails, and the
//     row can never come back as a warning. The price sheet is the sole
//     authority.
//
//  2. A SKU missing from the price sheet is fatal rather than a
//     mismatch. Other vendors can still write the PO's own price and
//     flag it; here that would mean writing 0 đ into the accounting
//     workbook, so the page fails instead.
//
//  3. An active promotion is REPORTED but never applied. For every other
//     vendor, a promo's discount is confirmed by the PO's own price
//     agreeing with it; with no such price there is nothing to confirm
//     against, and silently discounting on a guess is worse than
//     surfacing the promo and letting a person decide. Maxidi's price
//     tab carries no promotion columns today, so this is a guard for the
//     day one is added, not currently-exercised behaviour.
func (p *RealProcessor) processMaxidiSegment(filePath string, realPageNum int, text, pageLabel string) (OrderRow, error) {
	info, ok := maxidi.ParseOrderInfo(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được số PO/ngày đặt hàng/ngày giao/địa chỉ giao hàng")
	}

	branch, ok := maxidi.BranchForTaxCode(info.TaxCode)
	if !ok {
		return OrderRow{}, fmt.Errorf("không nhận ra chi nhánh Maxidi có mã số thuế %q", info.TaxCode)
	}

	customerCode, ok := p.Store.GetCustomerCodeForSystem(maxidiSystem)
	if !ok {
		return OrderRow{}, fmt.Errorf("chưa có mã khách hàng cho hệ thống %q trong sheet MAKH", maxidiSystem)
	}

	products := maxidi.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	priceIndex, err := p.Pricing.FetchIndex("MAXIDI")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	orderNum := maxidiOrderNumber(info.PONumber)
	description := maxidiDescription(info.PONumber, info.Remarks)

	baseRow := excelwriter.Row{
		EntryDate: info.EntryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
		Status: "Chưa thực hiện", CancelDate: info.ShipDate, ShipTo: info.DeliveryAddress,
		CustomerCode: customerCode, CustomerName: branch.CustomerName,
		InvoiceAddress: branch.InvoiceAddress, TaxCode: branch.TaxCode,
		Description: description, Warehouse: maxidiWarehouse, VATPercent: 8,
		RegionCode: maxidiRegion, StatCode: maxidiStatCode,
	}

	noteRow := baseRow
	noteRow.IsNoteRow = true
	noteRow.ProductName = description
	rows := []excelwriter.Row{noteRow}

	totalWeight := 0.0
	totalPackages := 0
	totalValue := 0.0
	var skuLog []string

	for _, rawProduct := range products {
		// The note prints the same order three ways: cartons, unit count
		// and units-per-carton. Cross-checking them against each other
		// costs nothing and is the only defence against a silently
		// misaligned parse — nothing else in this document would reveal
		// that the "quantity" being written is the wrong column's number.
		cartons := parseNumericField(rawProduct.Cartons)
		packSize := parseNumericField(rawProduct.PackSize)
		qty := parseNumericField(rawProduct.Qty)
		if qty <= 0 {
			return OrderRow{}, fmt.Errorf("không đọc được số lượng lẻ cho mã vạch %s", rawProduct.Barcode)
		}
		if math.Abs(cartons*packSize-qty) > 0.5 {
			return OrderRow{}, fmt.Errorf(
				"số lượng trên PO không khớp nhau cho mã vạch %s: %s thùng × %s ≠ %s lẻ",
				rawProduct.Barcode, rawProduct.Cartons, rawProduct.PackSize, rawProduct.Qty)
		}

		sku := p.Store.ResolveSku(rawProduct.Barcode)
		productInfo, _ := p.Store.GetProductInfo(sku)

		realPriceStr, found := priceIndex.FindPrice(sku)
		unitPrice := parseNumericField(realPriceStr)
		if !found || unitPrice <= 0 {
			return OrderRow{}, fmt.Errorf(
				"không tìm thấy giá cho mã vạch %s (mã hàng %s) trên bảng giá MAXIDI — PO Maxidi không in đơn giá nên không có giá nào để thay thế",
				rawProduct.Barcode, sku)
		}

		lineWeight := productInfo.WeightKg * qty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(qty / productInfo.PackSize))
		}
		totalWeight += lineWeight
		totalPackages += caseCount
		totalValue += unitPrice * qty

		// Reported, never applied — see this function's own doc comment,
		// point 3. CR normalization matches every other vendor's handling
		// of a promo cell read back through excelize.
		promoText := ""
		promoColumn := ""
		for _, promo := range priceIndex.FindPromotions(sku, info.EntryDate) {
			value := strings.ReplaceAll(promo.Value, "\r", "\n")
			if strings.TrimSpace(value) == "" {
				continue
			}
			promoText = value
			promoColumn = promo.Column
			break
		}

		productRow := baseRow
		productRow.SKU = sku
		productRow.ProductName = productInfo.Name
		productRow.Qty = qty
		productRow.UnitPrice = unitPrice
		productRow.CaseCount = caseCount
		productRow.LineWeightKg = lineWeight
		productRow.UseZFormula = true
		productRow.PromoContent = promoText
		rows = append(rows, productRow)

		skuLog = append(skuLog, maxidiSkuLogLine(sku, productInfo.Name, qty, unitPrice, promoText, promoColumn))
	}

	totalWeightFormatted := coop.FormatWeightKg(totalWeight)
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, totalWeightFormatted)
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}

	uploadPath := filePath
	if extractedPath, cleanup, extractErr := pdfpage.ExtractPage(filePath, realPageNum); extractErr == nil {
		uploadPath = extractedPath
		defer cleanup()
	} else if p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không cắt được trang PDF để upload Drive (dùng nguyên file thay thế): %v", extractErr))
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, uploadPath, driveupload.Metadata{
		Vendor:       "MAXIDI",
		EntryDate:    info.EntryDate,
		CustomerCode: customerCode,
		CancelDate:   info.ShipDate,
		OutputName:   info.PONumber,
	}, func(ok bool, err error) {
		if p.LogFunc == nil {
			return
		}
		if ok {
			p.LogFunc(fmt.Sprintf("✅ Đã upload file lên Drive: %s", filepath.Base(filePath)))
		} else {
			p.LogFunc(fmt.Sprintf("❌ Upload Drive thất bại (%s): %v", filepath.Base(filePath), err))
		}
	})
	if uploadErr != nil && p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không đọc được file để upload Drive: %v", uploadErr))
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: maxidiSystem,
		MaKhachHang: customerCode,
		ExcelRows:   excelRowsFrom(startRow, len(rows)),
		PO:          info.PONumber, DonGia: fmt.Sprintf("%.0f", totalValue),
		Status: StatusDone, StatusKind: StatusKindDone,
		DriveURL: driveURL,
		ShipTo:   info.DeliveryAddress, EntryDate: info.EntryDate, CancelDate: info.ShipDate,
		TotalWeightKg: totalWeightFormatted, TotalPackages: totalPackages,
		PromoItems: finalizePromoItems(nil),
		SkuLog:     skuLog,
	}, nil
}
