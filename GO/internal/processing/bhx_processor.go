package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"order-processor/internal/driveupload"
	"order-processor/internal/pdfpage"
	"order-processor/internal/processing/bhx"
	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/warehouse"
)

// bhxSystem is the system name Bách Hóa Xanh rows carry — the same string
// used as its key in the MaKH sheet's first column, in the MISA
// branch-routing table, and in vendor.Identify's return value.
const bhxSystem = "BHX"

// bhxPricingSheet is the key of this vendor's price/promotion tab in
// Cài đặt > Google Sheets.
const bhxPricingSheet = "BHX"

// bhxRegionInfo mirrors write_to_dondathang_bhx's two branches: a
// customer code starting with "MB" is a northern store, everything else
// ships from the southern warehouse.
func bhxRegionInfo(customerCode string, wh *warehouse.Resolver) (region, statCode, warehouseCode string) {
	if strings.HasPrefix(customerCode, "MB") {
		return "MT_MB", "HN", wh.Get("bhx/MB")
	}
	return "MT_MN", "LA", wh.Get("bhx/khac")
}

// bhxOrderNumber mirrors write_to_dondathang_bhx's column B:
// f'ĐĐH{vendor}-{po_number}' with vendor "BHX".
func bhxOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHBHX-%s", poNumber)
}

// processBhxSegment turns one Bách Hóa Xanh order page into one order,
// porting the BHX branch of xulydonhang.py's process_file and
// write_to_dondathang_bhx: "1 page = 1 order", customer code looked up
// by "BHX" + store code, prices and promotions reconciled against the
// BHX tab by the order date.
//
// The unit price reconciled is line total ÷ quantity, as in Python. BHX
// prints its pre-VAT unit price rounded to two decimals (71,296.30)
// while the line total is computed from the unrounded one (252 ×
// 71,296.296… = 17,966,666.67), so total ÷ quantity is the price the PO
// actually charges.
func (p *RealProcessor) processBhxSegment(filePath string, realPageNum int, text, pageLabel string) (OrderRow, error) {
	info, ok := bhx.ParseOrderInfo(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được mã đơn hàng/ngày đặt hàng/ngày giao hàng/kho giao hàng")
	}
	products := bhx.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}
	totals, ok := bhx.ParseTotals(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không đọc được dòng Tổng của đơn (đơn nhiều trang chưa được hỗ trợ)")
	}

	customerCode := p.Store.GetCustomerCodeByStoreKey(bhxSystem, bhxSystem+info.StoreCode)
	if customerCode == "" {
		return OrderRow{}, fmt.Errorf("chưa có mã khách hàng cho cửa hàng BHX%s (%s) trong sheet MAKH", info.StoreCode, info.StoreName)
	}

	// Rows are found by shape, not by position (see the bhx package
	// comment), so a row the pattern failed to match would simply be
	// missing. The PO's own "Tổng" row is what proves none is.
	type bhxLine struct {
		barcode string
		qty     float64
		price   float64
	}
	var lines []bhxLine
	poQty, poAmount := 0.0, 0.0
	for _, raw := range products {
		qty := parseNumericField(raw.Qty)
		total := parseNumericField(raw.Total)
		if qty <= 0 || total <= 0 {
			return OrderRow{}, fmt.Errorf("không đọc được số lượng/thành tiền cho mã vạch %s", raw.Barcode)
		}
		printed := parseNumericField(raw.DiscountPrice)
		if printed <= 0 {
			printed = parseNumericField(raw.Price)
		}
		// The printed price is rounded to two decimals, so the total may
		// drift from qty × price by up to half a hundredth per unit.
		if printed > 0 && math.Abs(total-printed*qty) > qty*0.005+1 {
			return OrderRow{}, fmt.Errorf(
				"số liệu trên PO không khớp nhau cho mã vạch %s: %s × %.2f ≠ thành tiền %s",
				raw.Barcode, raw.Qty, printed, raw.Total)
		}
		poQty += qty
		poAmount += total
		lines = append(lines, bhxLine{barcode: raw.Barcode, qty: qty, price: total / qty})
	}
	if math.Abs(poQty-parseNumericField(totals.Qty)) > 0.01 || math.Abs(poAmount-parseNumericField(totals.Amount)) > 1 {
		return OrderRow{}, fmt.Errorf(
			"đọc được %d dòng hàng (%.0f, %.0f đ) nhưng dòng Tổng của PO ghi %s, %s đ — có dòng bị đọc thiếu hoặc lệch cột",
			len(products), poQty, poAmount, totals.Qty, totals.Amount)
	}

	priceIndex, err := p.Pricing.FetchIndex(bhxPricingSheet)
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouseCode := bhxRegionInfo(customerCode, p.Warehouses)
	orderNum := bhxOrderNumber(info.PONumber)
	description := fmt.Sprintf("BHX PO%s", info.PONumber)
	entryDate, cancelDate, deliveryAddress := info.EntryDate, info.CancelDate, info.StoreName

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: customerCode,
		Description: description, Warehouse: warehouseCode, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: description,
	})

	saigia := 0
	totalWeight := 0.0
	totalPackages := 0
	totalValue := 0.0
	promoTotals := map[string]*PromoItemSummary{}
	var skuLog []string
	var mismatchDetails []PriceMismatchDetail

	for _, line := range lines {
		qty := line.qty
		invoicePrice := line.price

		barcode := p.Store.ResolveSku(line.barcode)
		productInfo, _ := p.Store.GetProductInfo(barcode)

		lineWeight := productInfo.WeightKg * qty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(qty / productInfo.PackSize))
		}
		totalWeight += lineWeight
		totalPackages += caseCount

		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice := parseNumericField(realPriceStr)

		promos := priceIndex.FindPromotions(barcode, entryDate)
		khuyenmai := ""
		khuyenmaiColumn := ""
		matched := false
		finalPrice := realPrice

		var leftmostPromo leftmostPromoFallback
		for _, promo := range promos {
			value := strings.ReplaceAll(promo.Value, "\r", "\n")
			if value == "" {
				continue
			}
			khuyenmai = value
			khuyenmaiColumn = promo.Column
			candidatePrice := realPrice
			if discount := coop.ExtractDiscount(value); discount != 0 {
				candidatePrice = realPrice - (realPrice * discount / 100)
			}
			finalPrice = candidatePrice
			leftmostPromo.remember(value, promo.Column, candidatePrice)
			if closeEnough(invoicePrice, candidatePrice) {
				matched = true
				break
			}
		}
		leftmostPromo.apply(matched, &khuyenmai, &khuyenmaiColumn, &finalPrice)
		if len(promos) == 0 && closeEnough(invoicePrice, realPrice) {
			matched = true
		}
		unitPrice := appliedUnitPrice(matched, invoicePrice, finalPrice)
		if !matched && realPrice <= 0 {
			// Keep the order's own price when the price tab has none for
			// this item; the mismatch below still alerts the operator.
			unitPrice = invoicePrice
		}
		skuLog = append(skuLog, formatSkuLogLine(barcode, productInfo.Name, matched, invoicePrice, finalPrice, khuyenmai, khuyenmaiColumn))

		productRow := excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: customerCode,
			Description: description, SKU: barcode, Warehouse: warehouseCode, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: qty, UnitPrice: unitPrice,
			ProductName: productInfo.Name, CaseCount: caseCount, LineWeightKg: lineWeight, UseZFormula: true,
			PromoContent: khuyenmai,
		}
		productRowIndex := len(rows)
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
			mismatchDetails = append(mismatchDetails, PriceMismatchDetail{
				SKU: barcode, ProductName: productInfo.Name,
				InvoicePrice: invoicePrice, SystemPrice: finalPrice, Qty: qty,
				ExcelRow: productRowIndex, PromoText: truncatePromoText(khuyenmai),
				PromoDateRange: khuyenmaiColumn,
			})
		}
		rows = append(rows, productRow)
		totalValue += unitPrice * qty

		// Per-item promo bonus row. NOT gated on matched — see
		// promoGiftOnMismatchRule.
		bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, khuyenmai,
			coop.Product{Barcode: barcode, Qty: qty}, 0, entryDate, cancelDate, deliveryAddress,
			customerCode, description, warehouseCode, region, statCode, orderNum)
		if added {
			totalWeight += bonusRow.LineWeightKg
			totalPackages += bonusRow.CaseCount
			accumulatePromoItem(promoTotals, bonusRow.SKU, bonusRow.ProductName, bonusRow.Qty)

			rows[productRowIndex].PromoNote = mainRowNote
			if mainRowBundleSku != "" {
				rows[productRowIndex].PromoBundleSku = mainRowBundleSku
			}
			rows = append(rows, bonusRow)
		}
	}

	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		invoicePromo = strings.ReplaceAll(invoicePromo, "\r", "\n")
		if invoiceRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue,
			entryDate, cancelDate, deliveryAddress, customerCode, description, warehouseCode, region, statCode, orderNum); added {
			totalWeight += invoiceRow.LineWeightKg
			totalPackages += invoiceRow.CaseCount
			accumulatePromoItem(promoTotals, invoiceRow.SKU, invoiceRow.ProductName, invoiceRow.Qty)
			rows = append(rows, invoiceRow)
		}
	}

	totalWeightFormatted := coop.FormatWeightKg(totalWeight)
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, totalWeightFormatted)
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	uploadPath := filePath
	if extractedPath, cleanup, extractErr := pdfpage.ExtractPage(filePath, realPageNum); extractErr == nil {
		uploadPath = extractedPath
		defer cleanup()
	} else if p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không cắt được trang PDF để upload Drive (dùng nguyên file thay thế): %v", extractErr))
	}
	driveURL, uploadErr := driveupload.Upload(p.DriveClient, uploadPath, driveupload.Metadata{
		Vendor:       bhxSystem,
		EntryDate:    entryDate,
		CustomerCode: customerCode,
		CancelDate:   cancelDate,
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

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: bhxSystem, MaKhachHang: customerCode,
		ExcelRows: excelRowsFrom(startRow, len(rows)),
		PO:        info.PONumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		DriveURL: driveURL,
		ShipTo:   deliveryAddress, EntryDate: entryDate, CancelDate: cancelDate,
		TotalWeightKg: totalWeightFormatted, TotalPackages: totalPackages,
		PromoItems: finalizePromoItems(promoTotals),
		SkuLog:     skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
}
