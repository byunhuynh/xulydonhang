package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/productdata"
)

var (
	// Tên file JIT có 2 kiểu: kiểu đầy đủ "air_waybill_WH6_HN_29082026.pdf" và
	// kiểu rút gọn "WH6_HN_2908.pdf" (không tiền tố, ngày không có năm). Kiểu
	// rút gọn buộc phải bắt đầu bằng mã kho WHx_ để tên file của vendor khác
	// không lọt vào nhánh JIT.
	jitAirWaybillNamePattern      = regexp.MustCompile(`(?i)^air_waybill_(.+)_(\d{8}|\d{4})(?: \(\d+\))?\.pdf$`)
	jitAirWaybillShortNamePattern = regexp.MustCompile(`(?i)^(WH\d+_[A-Z]+)_(\d{8}|\d{4})(?: \(\d+\))?\.pdf$`)
	jitAirWaybillTrackingPattern  = regexp.MustCompile(`Mãvậnđơn:([A-Z0-9]+)Mãđơnhàng:`)
	jitAirWaybillPOPattern        = regexp.MustCompile(`Mãđơnhàng:(\d{6}[A-Z0-9]{8})`)
	jitAirWaybillItemStartPattern = regexp.MustCompile(`\[TopValue\]`)
	jitAirWaybillQtyPattern       = regexp.MustCompile(`SL:(\d+)`)
	jitAirWaybillSkuPattern       = regexp.MustCompile(`CH\d+`)
	jitWhitespacePattern          = regexp.MustCompile(`\s+`)
)

const jitDebtDays = 15

var writeJITOrderRows = excelwriter.WriteOrderRows

type jitAirWaybillPageResult struct {
	resultIndex int
	excelStart  int
	excelRows   []excelwriter.Row
	row         OrderRow
	productLogs []string
}

func jitRegionInfo(shipTo string) (region, statCode, warehouse string) {
	if shipTo == "WH6_HN" || shipTo == "WH6_HTLA" {
		return "TMĐT_MB", "HN", "TP_HN_12"
	}
	return "TMĐT_MN", "LA", "LA_KHOTMDT"
}

func parseJITAirWaybillFilename(path string) (warehouse, orderDate string, ok bool) {
	return parseJITAirWaybillFilenameAt(path, time.Now())
}

func parseJITAirWaybillFilenameAt(path string, now time.Time) (warehouse, orderDate string, ok bool) {
	base := filepath.Base(path)
	match := jitAirWaybillNamePattern.FindStringSubmatch(base)
	if match == nil {
		match = jitAirWaybillShortNamePattern.FindStringSubmatch(base)
	}
	if match == nil {
		return "", "", false
	}
	if len(match[2]) == 8 {
		parsed, err := time.Parse("02012006", match[2])
		if err != nil {
			return "", "", false
		}
		return match[1], parsed.Format("02/01/2006"), true
	}
	parsed, resolved := resolveJITShortDate(match[2], now)
	if !resolved {
		return "", "", false
	}
	return match[1], parsed.Format("02/01/2006"), true
}

// Ngày kiểu rút gọn ("2908") không mang năm nên năm được suy ra từ hôm nay:
// lấy năm cho ra ngày GẦN hôm nay nhất trong {năm trước, năm nay, năm sau}, để
// file đặt tên cuối tháng 12 xử lý sang đầu tháng 1 (và ngược lại) vẫn đúng năm.
func resolveJITShortDate(dayMonth string, now time.Time) (time.Time, bool) {
	var best time.Time
	found := false
	for _, year := range []int{now.Year() - 1, now.Year(), now.Year() + 1} {
		parsed, err := time.Parse("02012006", fmt.Sprintf("%s%04d", dayMonth, year))
		if err != nil {
			continue
		}
		if !found || absDuration(parsed.Sub(now)) < absDuration(best.Sub(now)) {
			best, found = parsed, true
		}
	}
	return best, found
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func parseJITAirWaybillPage(text string) (string, string, []coop.Product, error) {
	compact := jitWhitespacePattern.ReplaceAllString(text, "")
	trackingMatch := jitAirWaybillTrackingPattern.FindStringSubmatch(compact)
	if trackingMatch == nil {
		return "", "", nil, fmt.Errorf("không tìm thấy mã vận đơn JIT")
	}
	poMatch := jitAirWaybillPOPattern.FindStringSubmatch(compact)
	if poMatch == nil {
		return "", "", nil, fmt.Errorf("không tìm thấy mã đơn hàng JIT")
	}

	productsText := compact
	if marker := strings.Index(compact, "Nộidunghàng"); marker >= 0 {
		productsText = compact[marker:]
	}
	starts := jitAirWaybillItemStartPattern.FindAllStringIndex(productsText, -1)
	products := make([]coop.Product, 0, len(starts))
	for i, start := range starts {
		end := len(productsText)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		block := productsText[start[1]:end]
		if i+1 < len(starts) {
			block = strings.TrimSuffix(block, fmt.Sprintf("%d.", i+2))
		}
		qtyMatch := jitAirWaybillQtyPattern.FindStringSubmatch(block)
		if qtyMatch == nil {
			continue
		}
		qty, err := strconv.Atoi(qtyMatch[1])
		if err != nil {
			continue
		}
		name := block[:strings.Index(block, qtyMatch[0])]
		name = "[TopValue]" + strings.Trim(name, " ,")
		products = append(products, coop.Product{Barcode: name, Qty: float64(qty)})
	}
	if len(products) == 0 {
		return "", "", nil, fmt.Errorf("không trích xuất được sản phẩm JIT nào")
	}
	return trackingMatch[1], poMatch[1], products, nil
}

func resolveJITProductSku(store *productdata.Store, productKey string) (string, bool) {
	resolved, ok := store.ResolveSkuAlias(productKey)
	if ok {
		return resolved, true
	}
	if chCode := jitAirWaybillSkuPattern.FindString(productKey); chCode != "" {
		resolved, ok = store.ResolveSkuAlias(chCode)
		if ok {
			return resolved, true
		}
	}
	return resolved, false
}

func extractJITAirWaybillPageTexts(path string) ([]string, error) {
	f, reader, err := pdfOpen(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	texts := make([]string, 0, reader.NumPage())
	for pageNum := 1; pageNum <= reader.NumPage(); pageNum++ {
		page := reader.Page(pageNum)
		text, ok := reconstructLinesFromContent(page)
		if !ok {
			text, err = extractPageText(page)
			if err != nil {
				return nil, fmt.Errorf("trang %d: %w", pageNum, err)
			}
		}
		texts = append(texts, text)
	}
	return texts, nil
}

func (p *RealProcessor) processJITAirWaybillDocument(filePath, warehouseCode, orderDate string, emit func(OrderRow)) ([]OrderRow, error) {
	texts, err := extractJITAirWaybillPageTexts(filePath)
	if err != nil {
		return nil, fmt.Errorf("không đọc được air waybill JIT: %w", err)
	}
	priceIndex, err := p.Pricing.FetchIndex("JIT")
	if err != nil {
		return nil, fmt.Errorf("không tải được giá JIT: %w", err)
	}

	location, locationErr := time.LoadLocation("Asia/Ho_Chi_Minh")
	if locationErr != nil {
		location = time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60)
	}
	period := "chiều"
	if time.Now().In(location).Hour() < 12 {
		period = "sáng"
	}
	dateDescription := fmt.Sprintf("%s (%s)", orderDate, period)
	orderNumber := fmt.Sprintf("ĐĐHJIT-%s-%s", dateDescription, warehouseCode)
	description := fmt.Sprintf("JIT-CHOICE Ngày đổ %s %s", dateDescription, warehouseCode)
	region, statCode, warehouse := jitRegionInfo(warehouseCode)

	result := make([]OrderRow, len(texts))
	pending := make([]jitAirWaybillPageResult, 0, len(texts))
	allExcelRows := make([]excelwriter.Row, 0)
	for pageIdx, text := range texts {
		tracking, po, products, parseErr := parseJITAirWaybillPage(text)
		pageLabel := fmt.Sprintf("%d/%d", pageIdx+1, len(texts))
		if parseErr != nil {
			p.emitJITLog(fmt.Sprintf("❌ JIT [%s] không đọc được đơn: %v", pageLabel, parseErr))
			result[pageIdx] = emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d", pageIdx+1), OrderRow{FileName: filepath.Base(filePath), Page: pageLabel, System: "JIT-CHOICE", Status: fmt.Sprintf("%s - %v", StatusFailed, parseErr), StatusKind: StatusKindFailed})
			continue
		}
		p.emitJITLog(fmt.Sprintf("🚀 JIT [%s] PO: %s | MVĐ: %s", pageLabel, po, tracking))

		excelRows := make([]excelwriter.Row, 0, len(products))
		productLogs := make([]string, 0, len(products))
		skus := make([]string, 0, len(products))
		totalValue := 0.0
		totalWeight := 0.0
		totalPackages := 0
		totalQty := 0
		for _, product := range products {
			sku, mapped := resolveJITProductSku(p.Store, product.Barcode)
			if !mapped {
				p.emitJITLog(fmt.Sprintf("❌ JIT [%s] PO: %s | không ánh xạ được sản phẩm %q", pageLabel, po, product.Barcode))
				result[pageIdx] = emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d", pageIdx+1), OrderRow{FileName: filepath.Base(filePath), Page: pageLabel, System: "JIT-CHOICE", PO: po, MaVanDon: tracking, Status: fmt.Sprintf("%s - không ánh xạ được sản phẩm %q", StatusFailed, product.Barcode), StatusKind: StatusKindFailed})
				excelRows = nil
				break
			}
			info, _ := p.Store.GetProductInfo(sku)
			priceText, foundPrice := priceIndex.FindPrice(sku)
			unitPrice, priceErr := strconv.ParseFloat(strings.ReplaceAll(priceText, ",", ""), 64)
			if !foundPrice || priceErr != nil || unitPrice <= 0 {
				p.emitJITLog(fmt.Sprintf("❌ JIT [%s] PO: %s | không tìm thấy đơn giá hợp lệ cho SKU %s", pageLabel, po, sku))
				result[pageIdx] = emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d", pageIdx+1), OrderRow{FileName: filepath.Base(filePath), Page: pageLabel, System: "JIT-CHOICE", PO: po, MaVanDon: tracking, Status: fmt.Sprintf("%s - không tìm thấy đơn giá JIT hợp lệ cho SKU %s", StatusFailed, sku), StatusKind: StatusKindFailed})
				excelRows = nil
				break
			}
			lineWeight := info.WeightKg * product.Qty
			caseCount := 0
			if info.PackSize > 0 {
				caseCount = int(math.Ceil(product.Qty / info.PackSize))
			}
			totalValue += unitPrice * product.Qty
			totalWeight += lineWeight
			totalPackages += caseCount
			totalQty += int(product.Qty)
			skus = append(skus, sku)
			productLogs = append(productLogs, fmt.Sprintf("✅ %s %s | SL: %s | Giá: %.0f", sku, info.Name, strconv.FormatFloat(product.Qty, 'f', -1, 64), unitPrice))
			excelRows = append(excelRows, excelwriter.Row{
				EntryDate: orderDate, DebtDays: jitDebtDays, OrderNumber: orderNumber,
				Status: "Chưa thực hiện", CancelDate: orderDate, ShipTo: warehouseCode,
				CustomerCode: "MN_JIT_01512", Description: description, SKU: sku,
				Warehouse: warehouse, VATPercent: 8, RegionCode: region, StatCode: statCode,
				Qty: product.Qty, UnitPrice: unitPrice, ProductName: info.Name,
				CaseCount: caseCount, LineWeightKg: lineWeight, PromoNote: po + " - " + tracking, UseZFormula: true,
			})
		}
		if excelRows == nil {
			continue
		}

		finalRow := OrderRow{
			FileName: filepath.Base(filePath), Page: pageLabel, System: "JIT-CHOICE",
			MaKhachHang: "MN_JIT_01512", PO: po, MaVanDon: tracking, DonGia: fmt.Sprintf("%.0f", totalValue),
			Status: StatusDone, StatusKind: StatusKindDone, ShipTo: warehouseCode,
			EntryDate: orderDate, CancelDate: orderDate, TotalWeightKg: coop.FormatWeightKg(totalWeight),
			TotalPackages: totalPackages, TotalQty: totalQty, SKUs: skus, PromoItems: make([]PromoItemSummary, 0),
			JITPeriod: period,
		}
		provisional := finalRow
		provisional.Status = StatusProcessing
		provisional.StatusKind = StatusKindProcessing
		provisional = emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d", pageIdx+1), provisional)
		finalRow.SourceID = provisional.SourceID
		finalRow.ResultKey = provisional.ResultKey

		pending = append(pending, jitAirWaybillPageResult{
			resultIndex: pageIdx,
			excelStart:  len(allExcelRows),
			excelRows:   excelRows,
			row:         finalRow,
			productLogs: productLogs,
		})
		allExcelRows = append(allExcelRows, excelRows...)
	}

	if len(pending) == 0 {
		return result, nil
	}

	startRow, writeErr := writeJITOrderRows(p.ExcelPath, allExcelRows, description)
	if writeErr != nil {
		for _, page := range pending {
			failed := page.row
			failed.Status = fmt.Sprintf("%s - %v", StatusFailed, writeErr)
			failed.StatusKind = StatusKindFailed
			failed.ExcelRows = nil
			p.emitJITLog(fmt.Sprintf("❌ JIT [%s] PO: %s | lỗi ghi dondathang.xlsx: %v", failed.Page, failed.PO, writeErr))
			result[page.resultIndex] = emitOrderRow(emit, failed)
		}
		return result, nil
	}

	for _, page := range pending {
		finalRow := page.row
		finalRow.ExcelRows = make([]int, len(page.excelRows))
		for i := range finalRow.ExcelRows {
			finalRow.ExcelRows[i] = startRow + page.excelStart + i
		}
		for _, line := range page.productLogs {
			p.emitJITLog(line)
		}
		p.emitJITLog(fmt.Sprintf("✅ JIT [%s] PO: %s | MVĐ: %s | đã ghi %d dòng sản phẩm", finalRow.Page, finalRow.PO, finalRow.MaVanDon, len(page.excelRows)))
		result[page.resultIndex] = emitOrderRow(emit, finalRow)
	}
	return result, nil
}

func (p *RealProcessor) emitJITLog(line string) {
	if p.LogFunc != nil {
		p.LogFunc(line)
	}
}
