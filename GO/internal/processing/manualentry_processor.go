package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/manualentry"
)

// manualEntryOrderNumber dùng ĐÚNG quy ước "ĐĐH{VENDOR}-{po}" mọi vendor
// khác đã dùng (vd satraOrderNumber, lotteOrderNumber) - theo yêu cầu
// thực tế, đơn nhập tay ghi vào dondathang.xlsx PHẢI trông y hệt đơn tự
// động, không cần đánh dấu riêng gì cả.
func manualEntryOrderNumber(system, po string) string {
	return fmt.Sprintf("ĐĐH%s-%s", strings.ToUpper(system), po)
}

// processManualEntryDocument xử lý TOÀN BỘ file "đơn hàng tay.xlsx" -
// một file có thể chứa NHIỀU PO khác nhau (mỗi dòng sản phẩm lặp lại
// thông tin PO/ngày/khách hàng của đơn nó thuộc về), nên đọc hết rồi
// GỘP THEO PO trước khi xử lý từng đơn riêng - khác BigC/JIT (gộp theo
// cả file) nhưng giống bản chất "1 file, nhiều đơn" của chúng.
func (p *RealProcessor) processManualEntryDocument(filePath string, emit func(OrderRow)) ([]OrderRow, error) {
	lines, err := manualentry.Load(filePath)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("không có dòng đơn hàng nào trong sheet %q (đã bỏ qua dòng tiêu đề và dòng ví dụ)", manualentry.SheetName)
	}

	// Gộp theo PO, GIỮ NGUYÊN thứ tự PO xuất hiện lần đầu trong file - để
	// bảng kết quả hiện các đơn theo đúng thứ tự người dùng đã gõ, không
	// bị xáo trộn bởi thứ tự map (Go không đảm bảo).
	var order []string
	groups := make(map[string][]manualentry.Line)
	for _, l := range lines {
		if _, seen := groups[l.PO]; !seen {
			order = append(order, l.PO)
		}
		groups[l.PO] = append(groups[l.PO], l)
	}

	var rows []OrderRow
	for _, po := range order {
		position := fmt.Sprintf("po:%s", po)
		row, procErr := p.processManualEntryOrder(filePath, po, groups[po])
		if procErr != nil {
			rows = append(rows, emitIdentifiedOrderRow(emit, filePath, position, OrderRow{
				FileName: filepath.Base(filePath), Page: "nhập tay", PO: po,
				Status: fmt.Sprintf("%s - %v", StatusFailed, procErr), StatusKind: StatusKindFailed,
			}))
			continue
		}
		rows = append(rows, emitIdentifiedOrderRow(emit, filePath, position, row))
	}
	return rows, nil
}

// processManualEntryOrder xử lý 1 PO gộp từ NHIỀU dòng sản phẩm cùng PO
// đó - mirror processSatraSegment's phần "customerCode, priceIndex..."
// trở đi Y HỆT (cùng logic tra giá/khuyến mãi/ghi Excel dùng chung mọi
// vendor), CHỈ khác ở đầu vào: không có văn bản PDF nào để tách PO/ngày/
// sản phẩm - mọi thứ đã có sẵn từ Line người dùng gõ tay. Không upload
// Drive (không có trang PDF nguồn nào để cắt/tải lên).
func (p *RealProcessor) processManualEntryOrder(filePath, po string, lines []manualentry.Line) (OrderRow, error) {
	// Thông tin cấp-đơn lấy từ dòng ĐẦU TIÊN có giá trị khác rỗng cho
	// từng cột - cho phép người dùng chỉ gõ 1 lần (dòng đầu của PO) thay
	// vì bắt buộc lặp lại y hệt ở MỌI dòng sản phẩm cùng PO.
	var entryDate, cancelDate, system, customerCode, shipTo string
	for _, l := range lines {
		if entryDate == "" {
			entryDate = l.EntryDate
		}
		if cancelDate == "" {
			cancelDate = l.CancelDate
		}
		if system == "" {
			system = l.System
		}
		if customerCode == "" {
			customerCode = l.CustomerCode
		}
		if shipTo == "" {
			shipTo = l.ShipTo
		}
	}
	if entryDate == "" {
		return OrderRow{}, fmt.Errorf("thiếu Ngày đặt")
	}
	if system == "" {
		return OrderRow{}, fmt.Errorf("thiếu Hệ thống")
	}
	if customerCode == "" {
		return OrderRow{}, fmt.Errorf("thiếu Mã khách hàng")
	}

	// Coop bán qua HAI hệ thống (Coopmart/Coopfood) dùng chung một sheet
	// giá/khuyến mãi, và một CTKM có thể chỉ thuộc về một trong hai (xem
	// coop.PromoForSystem). Đơn nhập tay phải biết mình thuộc hệ thống
	// nào trước khi áp bất kỳ CTKM nào, nếu không đơn Coopmart gõ tay sẽ
	// ăn hết CTKM riêng của Coopfood và ngược lại.
	//
	// Cột "Hệ thống" chấp nhận cả tên vendor ("COOP" — suy ra hệ thống
	// con từ mã khách hàng, y như đường PDF) lẫn tên hệ thống con gõ
	// thẳng ("COOPMART"/"COOPFOOD" — người dùng nói rõ thì tin người
	// dùng). Cả hai đều đọc sheet "COOP", nên vendorKey mới là thứ đi
	// tra giá và đặt số ĐĐH; giá trị gõ tay chỉ đổi phần mô tả (L), đúng
	// như đường PDF ghi "COOPFOOD PO...".
	//
	// coopSystem để RỖNG với mọi vendor khác — họ không có khái niệm này
	// và coopPromosForSystem sẽ không lọc gì cả.
	vendorKey := strings.ToUpper(strings.TrimSpace(system))
	coopSystem := ""
	switch vendorKey {
	case "COOPMART", "COOPFOOD":
		coopSystem = vendorKey
		vendorKey = "COOP"
	case "COOP":
		if coopSystem = p.Store.GetSystemForCustomer(customerCode); coopSystem != "COOPFOOD" {
			coopSystem = "COOPMART"
		}
	}

	priceIndex, err := p.Pricing.FetchIndex(vendorKey)
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi cho hệ thống %q: %w", system, err)
	}

	region, statCode, warehouse := regionInfo(customerCode, p.Warehouses)
	orderNumber := manualEntryOrderNumber(vendorKey, po)
	// KHÔNG đánh dấu "(nhập tay)" ở đây - theo yêu cầu thực tế, dòng tiêu
	// đề (S) và Description (L) trong dondathang.xlsx phải trông y hệt
	// đơn tự động, đúng quy ước "{VENDOR} {po}" mọi vendor khác dùng (vd
	// satraOrderNumber's titleText/noteText). "nhập tay" vẫn còn đánh dấu
	// ở OrderRow.Page (chỉ hiện trên UI, không ghi vào Excel) để người
	// dùng vẫn phân biệt được trên bảng kết quả.
	titleText := fmt.Sprintf("%s %s", strings.ToUpper(system), po)
	noteText := titleText

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
		Description: noteText, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: titleText,
	})

	saigia := 0
	totalWeight := 0.0
	totalPackages := 0
	totalValue := 0.0
	validLines := 0
	promoTotals := map[string]*PromoItemSummary{}
	var skuLog []string
	var mismatchDetails []PriceMismatchDetail

	for _, l := range lines {
		rawSKU := strings.TrimSpace(l.RawSKU)
		if rawSKU == "" || l.Qty <= 0 {
			continue
		}
		validLines++

		barcode := p.Store.ResolveSku(rawSKU)
		productInfo, _ := p.Store.GetProductInfo(barcode)
		qty := l.Qty
		lineWeight := productInfo.WeightKg * qty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(qty / productInfo.PackSize))
		}
		totalWeight += lineWeight
		totalPackages += caseCount

		invoicePrice := l.InvoicePrice
		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice, _ := strconv.ParseFloat(strings.ReplaceAll(realPriceStr, ",", ""), 64)

		promos := coopPromosForSystem(priceIndex.FindPromotions(barcode, entryDate), coopSystem)
		lastExaminedPromo := ""
		lastExaminedPromoColumn := ""
		matched := false
		finalPrice := realPrice

		var leftmostPromo leftmostPromoFallback
		for _, promo := range promos {
			value := promo.Value
			lastExaminedPromo = value
			lastExaminedPromoColumn = promo.Column
			if value == "" {
				continue
			}
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
		leftmostPromo.apply(matched, &lastExaminedPromo, &lastExaminedPromoColumn, &finalPrice)
		if len(promos) == 0 && closeEnough(invoicePrice, realPrice) {
			matched = true
		}

		unitPrice := appliedUnitPrice(matched, invoicePrice, finalPrice)
		skuLog = append(skuLog, formatSkuLogLine(barcode, productInfo.Name, matched, invoicePrice, finalPrice, lastExaminedPromo, lastExaminedPromoColumn))

		productRow := excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
			Description: noteText, SKU: barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: qty, UnitPrice: unitPrice,
			ProductName: productInfo.Name, CaseCount: caseCount, LineWeightKg: lineWeight, UseZFormula: true,
			PromoContent: lastExaminedPromo,
		}
		productRowIndex := len(rows)
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
			mismatchDetails = append(mismatchDetails, PriceMismatchDetail{
				SKU: barcode, ProductName: productInfo.Name,
				InvoicePrice: invoicePrice, SystemPrice: finalPrice, Qty: qty,
				ExcelRow: productRowIndex, PromoText: truncatePromoText(lastExaminedPromo),
				PromoDateRange: lastExaminedPromoColumn,
			})
		}
		rows = append(rows, productRow)
		totalValue += unitPrice * qty

		currentRowIndex := productRowIndex
		for i, promoPart := range strings.Split(lastExaminedPromo, "|") {
			rows[currentRowIndex].PromoContent = lastExaminedPromo

			bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, promoPart,
				coop.Product{Barcode: barcode, Qty: qty}, i, entryDate, cancelDate, shipTo,
				customerCode, noteText, warehouse, region, statCode, orderNumber)
			if !added {
				continue
			}
			totalWeight += bonusRow.LineWeightKg
			totalPackages += bonusRow.CaseCount
			accumulatePromoItem(promoTotals, bonusRow.SKU, bonusRow.ProductName, bonusRow.Qty)
			if i == 0 {
				rows[productRowIndex].PromoNote = mainRowNote
				if mainRowBundleSku != "" {
					rows[productRowIndex].PromoBundleSku = mainRowBundleSku
				}
			}
			rows = append(rows, bonusRow)
			currentRowIndex = len(rows) - 1
		}
	}

	if validLines == 0 {
		return OrderRow{}, fmt.Errorf("không có dòng sản phẩm hợp lệ nào (thiếu Mã hàng hoặc Số lượng <= 0)")
	}

	if invoicePromo := coopInvoicePromotion(priceIndex, entryDate, coopSystem); invoicePromo != "" {
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, entryDate, cancelDate,
			shipTo, customerCode, noteText, warehouse, region, statCode, orderNumber); added {
			totalWeight += bonusRow.LineWeightKg
			totalPackages += bonusRow.CaseCount
			accumulatePromoItem(promoTotals, bonusRow.SKU, bonusRow.ProductName, bonusRow.Qty)
			rows = append(rows, bonusRow)
		}
	}

	totalWeightFormatted := coop.FormatWeightKg(totalWeight)
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", noteText, totalWeightFormatted)
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: "nhập tay", System: system, MaKhachHang: customerCode,
		ExcelRows: excelRowsFrom(startRow, len(rows)),
		PO:        po, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		ShipTo: shipTo, EntryDate: entryDate, CancelDate: cancelDate,
		TotalWeightKg: totalWeightFormatted, TotalPackages: totalPackages,
		PromoItems: finalizePromoItems(promoTotals),
		SkuLog:     skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
}
