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
	"order-processor/internal/processing/osifood"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/warehouse"
)

// osifoodSystem is the system name this vendor's rows carry — the same
// string used as its key in the MaKH sheet's first column, in the MISA
// branch-routing table, and in vendor.Identify's return value.
const osifoodSystem = "Osifood"

// osifoodPricingSheet is the key of this vendor's price/promotion tab in
// Cài đặt > Google Sheets.
const osifoodPricingSheet = "OSIFOOD"

// osifoodRegionInfo cấp mã vùng/thống kê/kho cho Osifood. Chuỗi này chỉ
// có cửa hàng ở miền Nam nên, giống JMart, không có nhánh miền Bắc; mã
// kho nằm trong Cài đặt để chỉnh được mà không phải build lại.
func osifoodRegionInfo(wh *warehouse.Resolver) (region, statCode, warehouseCode string) {
	return "MT_MN", "LA", wh.Get("osifood")
}

// osifoodOrderNumber builds column B, matching every other vendor's
// "ĐĐH<VENDOR>-<PO>" shape.
func osifoodOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHOSIFOOD-%s", poNumber)
}

// osifoodLine is one ordered product, already reduced to what the order
// actually charges, whichever of the two document formats it came from.
type osifoodLine struct {
	Barcode string
	Qty     float64
	// UnitPrice is the promotional cost when the line has one and the
	// list cost otherwise (project owner's rule), unrounded.
	UnitPrice float64
	// StatesPromo reports whether the document itself marks this line as
	// promotional — promotion text or a promotional price.
	StatesPromo bool
	// PromoDate is the date to look a stated promotion up by when the
	// order date has none active: the PDF order's receiving deadline, or
	// the first day of the workbook line's own campaign dates.
	PromoDate string
}

// osifoodOrder is one store's order.
type osifoodOrder struct {
	PONumber        string
	StoreCode       string
	EntryDate       string
	CancelDate      string
	DeliveryAddress string
	Lines           []osifoodLine
}

// osifoodPromotions returns the CTKM columns to reconcile one line
// against.
//
// Every other vendor looks promotions up by the order date alone. An
// Osifood order is routinely placed BEFORE the campaign it stocks up for
// — the real PDF sample was placed 12/10 for a 16–31/10 campaign, already
// at that campaign's prices, and the campaign workbook is by definition
// answered before the campaign starts — so a lookup by order date finds
// nothing and would flag every discounted line as a wrong price. When the
// order date has no active CTKM and the document itself states one for
// the line, line.PromoDate is tried instead. A line the document states
// no promotion for never takes this second look, so an ordinary order
// placed shortly before a campaign is not given that campaign's gift.
func osifoodPromotions(index *pricing.Index, sku, entryDate string, line osifoodLine) []pricing.Promotion {
	promos := index.FindPromotions(sku, entryDate)
	if len(promos) == 0 && line.StatesPromo && line.PromoDate != "" {
		promos = index.FindPromotions(sku, line.PromoDate)
	}
	return promos
}

// processOsifoodSegment turns one Osifood PDF order page into one order.
// Osifood is "1 page = 1 order", the same family as Kingfood, JMart and
// Maxidi, and reconciles prices and promotions against its own price tab
// the way Kingfood and JMart do.
//
// The price reconciled is the one the PO actually charges: "GIÁ VỐN KM"
// when the line has one, "GIÁ VỐN ĐVỊ (0VAT)" otherwise (project owner's
// rule). Both are printed rounded to whole đồng while the line total is
// computed from the unrounded price (110,501 printed, 8 × 110,500.5 =
// 884,004 charged), so the unit price carried forward is total ÷
// quantity — after checking that it agrees with the printed price, which
// is also what catches a misaligned parse.
func (p *RealProcessor) processOsifoodSegment(filePath string, realPageNum int, text, pageLabel string) (OrderRow, error) {
	info, ok := osifood.ParseOrderInfo(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được mã cửa hàng/ngày đặt hàng/hạn nhận đơn/địa chỉ giao hàng")
	}
	products := osifood.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}
	totals, ok := osifood.ParseTotals(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không đọc được dòng TOTAL của đơn")
	}

	order := osifoodOrder{
		PONumber: info.PONumber, StoreCode: info.StoreCode,
		EntryDate: info.EntryDate, CancelDate: info.Deadline, DeliveryAddress: info.DeliveryAddress,
	}
	poQty := 0.0
	poAmount := 0.0
	for _, rawProduct := range products {
		qty := parseNumericField(rawProduct.Qty)
		lineTotal := parseNumericField(rawProduct.Total)
		promoPrice := parseNumericField(rawProduct.PromoPrice)
		printedPrice := promoPrice
		if printedPrice <= 0 {
			printedPrice = parseNumericField(rawProduct.Price)
		}
		if qty <= 0 || printedPrice <= 0 {
			return OrderRow{}, fmt.Errorf("không đọc được số lượng/đơn giá cho mã vạch %s", rawProduct.Barcode)
		}
		// The printed price is rounded to a whole đồng, so the total may
		// drift from qty × price by up to half a đồng per unit.
		if math.Abs(lineTotal-printedPrice*qty) > qty*0.5+1 {
			return OrderRow{}, fmt.Errorf(
				"số liệu trên PO không khớp nhau cho mã vạch %s: %s × %.0f ≠ thành tiền %s",
				rawProduct.Barcode, rawProduct.Qty, printedPrice, rawProduct.Total)
		}
		poQty += qty
		poAmount += lineTotal
		order.Lines = append(order.Lines, osifoodLine{
			Barcode: rawProduct.Barcode, Qty: qty, UnitPrice: lineTotal / qty,
			StatesPromo: rawProduct.PromoText != "" || promoPrice > 0,
			PromoDate:   info.Deadline,
		})
	}

	// The rows above are found by shape, not by position (see the osifood
	// package comment), so a row the pattern failed to match would simply
	// be missing. The PO's own TOTAL row is what proves none is.
	if math.Abs(poQty-parseNumericField(totals.Qty)) > 0.01 || math.Abs(poAmount-parseNumericField(totals.Amount)) > 1 {
		return OrderRow{}, fmt.Errorf(
			"đọc được %d dòng hàng (%.0f lẻ, %.0f đ) nhưng dòng TOTAL của PO ghi %s lẻ, %s đ — có dòng bị đọc thiếu hoặc lệch cột",
			len(products), poQty, poAmount, strings.TrimSpace(totals.Qty), strings.TrimSpace(totals.Amount))
	}

	return p.writeOsifoodOrder(filePath, realPageNum, pageLabel, order)
}

// osifoodWorkbookOrder turns one store's block of the campaign workbook
// into an order, applying the same price rule and the same two
// self-consistency checks the PDF path applies.
//
// The workbook carries no receiving deadline, so the delivery date falls
// back to the order date, the way Kingfood's and JMart's does.
func osifoodWorkbookOrder(entryDate string, store osifood.StoreOrder) (osifoodOrder, error) {
	order := osifoodOrder{
		PONumber: osifood.PONumber(store.StoreCode, entryDate), StoreCode: store.StoreCode,
		EntryDate: entryDate, CancelDate: entryDate, DeliveryAddress: store.DeliveryAddress,
	}
	sumQty := 0.0
	sumAmount := 0.0
	for _, line := range store.Lines {
		price := line.PromoPrice
		if price <= 0 {
			price = line.Price
		}
		if price <= 0 {
			return osifoodOrder{}, fmt.Errorf("không đọc được đơn giá cho mã vạch %s", line.Barcode)
		}
		if line.Total > 0 && math.Abs(line.Total-price*line.Qty) > 1 {
			return osifoodOrder{}, fmt.Errorf(
				"số liệu trên file không khớp nhau cho mã vạch %s: %v × %v ≠ thành tiền %v",
				line.Barcode, line.Qty, price, line.Total)
		}
		sumQty += line.Qty
		sumAmount += price * line.Qty
		order.Lines = append(order.Lines, osifoodLine{
			Barcode: line.Barcode, Qty: line.Qty, UnitPrice: price,
			StatesPromo: line.PromoText != "" || line.PromoPrice > 0,
			PromoDate:   line.PromoStart,
		})
	}
	if store.HasPrintedTotals && (math.Abs(sumQty-store.PrintedQty) > 0.01 || math.Abs(sumAmount-store.PrintedAmount) > 1) {
		return osifoodOrder{}, fmt.Errorf(
			"đọc được %d dòng hàng (%.0f lẻ, %.0f đ) nhưng dòng tổng của cửa hàng ghi %.0f lẻ, %.0f đ — có dòng bị đọc thiếu hoặc lệch cột",
			len(store.Lines), sumQty, sumAmount, store.PrintedQty, store.PrintedAmount)
	}
	return order, nil
}

// processOsifoodWorkbook handles the campaign workbook: one file, one
// order per store that answered with a quantity. A store that fails —
// typically one with no customer code in MaKH yet — fails on its own row
// and leaves the other stores' orders alone.
func (p *RealProcessor) processOsifoodWorkbook(filePath string, emit func(OrderRow)) ([]OrderRow, error) {
	wb, err := osifood.LoadWorkbook(filePath)
	if err != nil {
		return nil, err
	}
	if len(wb.Orders) == 0 {
		return nil, fmt.Errorf("không có cửa hàng nào điền số lượng ở cột \"CH phản hồi theo nhu cầu kinh doanh\"")
	}

	var rows []OrderRow
	for _, store := range wb.Orders {
		pageLabel := "CH " + store.StoreCode
		order, orderErr := osifoodWorkbookOrder(wb.EntryDate, store)
		var row OrderRow
		if orderErr == nil {
			row, orderErr = p.writeOsifoodOrder(filePath, 0, pageLabel, order)
		}
		if orderErr != nil {
			row = OrderRow{
				FileName: filepath.Base(filePath), Page: pageLabel, System: osifoodSystem,
				PO:     osifood.PONumber(store.StoreCode, wb.EntryDate),
				ShipTo: store.DeliveryAddress, EntryDate: wb.EntryDate,
				Status: fmt.Sprintf("%s - %v", StatusFailed, orderErr), StatusKind: StatusKindFailed,
			}
		}
		rows = append(rows, emitIdentifiedOrderRow(emit, filePath, "store:"+store.StoreCode, row))
	}
	return rows, nil
}

// writeOsifoodOrder reconciles one order against the OSIFOOD price tab and
// writes it to the order workbook. realPageNum is the PDF page the order
// came from, or 0 for an order read out of a campaign workbook.
func (p *RealProcessor) writeOsifoodOrder(filePath string, realPageNum int, pageLabel string, order osifoodOrder) (OrderRow, error) {
	customerCode := p.Store.GetCustomerCodeBySuffix(osifoodSystem, order.StoreCode)
	if customerCode == "" {
		return OrderRow{}, fmt.Errorf("chưa có mã khách hàng cho cửa hàng Osifood %s trong sheet MAKH", order.StoreCode)
	}

	priceIndex, err := p.Pricing.FetchIndex(osifoodPricingSheet)
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := osifoodRegionInfo(p.Warehouses)
	orderNum := osifoodOrderNumber(order.PONumber)
	description := fmt.Sprintf("OSIFOOD %s", order.PONumber)
	entryDate, cancelDate, deliveryAddress := order.EntryDate, order.CancelDate, order.DeliveryAddress

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: customerCode,
		Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: description,
	})

	saigia := 0
	totalWeight := 0.0
	totalPackages := 0
	totalValue := 0.0
	promoTotals := map[string]*PromoItemSummary{}
	var skuLog []string
	var mismatchDetails []PriceMismatchDetail

	for _, line := range order.Lines {
		qty := line.Qty
		invoicePrice := line.UnitPrice

		barcode := p.Store.ResolveSku(line.Barcode)
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

		promos := osifoodPromotions(priceIndex, barcode, entryDate, line)
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
			Description: description, SKU: barcode, Warehouse: warehouse, VATPercent: 8,
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
			customerCode, description, warehouse, region, statCode, orderNum)
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
			entryDate, cancelDate, deliveryAddress, customerCode, description, warehouse, region, statCode, orderNum); added {
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

	// A PDF order uploads its own page; a workbook has no page to cut, so
	// each store's order links to the whole campaign sheet.
	uploadPath := filePath
	if realPageNum > 0 {
		if extractedPath, cleanup, extractErr := pdfpage.ExtractPage(filePath, realPageNum); extractErr == nil {
			uploadPath = extractedPath
			defer cleanup()
		} else if p.LogFunc != nil {
			p.LogFunc(fmt.Sprintf("⚠️ Không cắt được trang PDF để upload Drive (dùng nguyên file thay thế): %v", extractErr))
		}
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, uploadPath, driveupload.Metadata{
		Vendor:       "OSIFOOD",
		EntryDate:    entryDate,
		CustomerCode: customerCode,
		CancelDate:   cancelDate,
		OutputName:   order.PONumber,
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
		FileName: filepath.Base(filePath), Page: pageLabel, System: osifoodSystem, MaKhachHang: customerCode,
		ExcelRows: excelRowsFrom(startRow, len(rows)),
		PO:        order.PONumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		DriveURL: driveURL,
		ShipTo:   deliveryAddress, EntryDate: entryDate, CancelDate: cancelDate,
		TotalWeightKg: totalWeightFormatted, TotalPackages: totalPackages,
		PromoItems: finalizePromoItems(promoTotals),
		SkuLog:     skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
}
