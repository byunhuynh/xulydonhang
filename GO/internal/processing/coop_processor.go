package processing

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"order-processor/internal/driveupload"
	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
	"order-processor/internal/processing/vendor"
)

// PricingSource abstracts fetching a vendor's price/promotion data for
// one order, so tests substitute a fixture-backed implementation instead
// of a live Google Sheets fetch. Production wiring uses *pricing.HTTPSource.
type PricingSource interface {
	FetchIndex(sheetKey string) (*pricing.Index, error)
}

// RealProcessor implements processing.Processor. Process dispatches each
// page to a vendor-specific handler based on vendor.Identify's result —
// Coop pages go through processSegment (coop_processor.go), Lotte pages
// through processLotteSegment (lotte_processor.go). Any page whose text
// doesn't match a recognized vendor's markers produces a single Failed
// OrderRow explaining why, rather than being silently skipped — support
// for additional vendors is added by extending this same dispatch.
type RealProcessor struct {
	Store       *productdata.Store
	Pricing     PricingSource
	ExcelPath   string
	DriveClient *http.Client // driveupload.NewHTTPClient() in production
	LogFunc     func(string) // optional (nil-safe) - routes background upload results to "process:log"
}

func (p *RealProcessor) Process(ctx context.Context, filePath string) ([]OrderRow, error) {
	return p.process(ctx, filePath, nil)
}

// ProcessStreaming keeps Process's result while reporting each row at the
// same return boundary. Vendor-loop timing is introduced separately.
func (p *RealProcessor) ProcessStreaming(ctx context.Context, filePath string, emit func(OrderRow)) ([]OrderRow, error) {
	return p.process(ctx, filePath, emit)
}

func emitOrderRow(emit func(OrderRow), row OrderRow) OrderRow {
	if row.ResultKey == "" {
		row.ResultKey = orderResultKey(row.SourceID, row.Page, row.PO)
	}
	if emit != nil {
		emit(row)
	}
	return row
}

func (p *RealProcessor) process(ctx context.Context, filePath string, emit func(OrderRow)) ([]OrderRow, error) {
	pageTexts, pageNumbers, err := extractPageTexts(filePath)
	if err != nil {
		row := emitIdentifiedOrderRow(emit, filePath, "file", OrderRow{
			FileName:   filepath.Base(filePath),
			Status:     StatusFailed + " - không đọc được file: " + err.Error(),
			StatusKind: StatusKindFailed,
		})
		return []OrderRow{row}, nil
	}

	if warehouse, orderDate, ok := parseJITAirWaybillFilename(filePath); ok {
		rows, jitErr := p.processJITAirWaybillDocument(filePath, warehouse, orderDate, emit)
		if jitErr != nil {
			row := emitIdentifiedOrderRow(emit, filePath, "file", OrderRow{FileName: filepath.Base(filePath), System: "JIT-CHOICE", Status: fmt.Sprintf("%s - %v", StatusFailed, jitErr), StatusKind: StatusKindFailed})
			rows = []OrderRow{row}
		}
		return rows, nil
	}

	// BigC's identifying markers are present on every page of a real
	// BigC file (see vendor.Identify's bigcPattern doc comment), but
	// only page 0 carries the master price list, customer code, and
	// PO/dates every store page's row-building depends on — a per-page
	// dispatch can't supply that cross-page state. Pre-check page 0
	// specifically and, if it's BigC, hand the WHOLE file to
	// processBigcDocument instead of entering the per-page loop below.
	if len(pageTexts) > 0 && vendor.Identify(pageTexts[0]) == "BigC" {
		rows, err := p.processBigcDocument(filePath, pageTexts, emit)
		return rows, err
	}

	var rows []OrderRow
	for pageIdx, text := range pageTexts {
		pageLabel := fmt.Sprintf("%d/%d", pageIdx+1, len(pageTexts))
		physicalPage := pageNumbers[pageIdx]
		v := vendor.Identify(text)

		switch v {
		case "Coop":
			segments, ok := splitPageIntoPOs(text)
			if !ok {
				rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:0", physicalPage), OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Coop",
					Status: StatusFailed + " - không đếm khớp số đơn trên trang", StatusKind: StatusKindFailed,
				}))
				continue
			}
			for segIdx, segment := range segments {
				segLabel := fmt.Sprintf("%d/%d", segIdx+1, len(segments))
				row, err := p.processSegment(filePath, pageNumbers[pageIdx], segment, segLabel)
				if err != nil {
					rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:%d", physicalPage, segIdx+1), OrderRow{
						FileName: filepath.Base(filePath), Page: segLabel, System: "Coop",
						Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
					}))
					continue
				}
				rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:%d", physicalPage, segIdx+1), row))
			}

		case "Lotte":
			row, err := p.processLotteSegment(filePath, pageNumbers[pageIdx], text, pageLabel)
			if err != nil {
				rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Lotte",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				}))
				continue
			}
			rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), row))

		case "Satra":
			row, err := p.processSatraSegment(filePath, pageNumbers[pageIdx], text, pageLabel)
			if err != nil {
				rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Satra",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				}))
				continue
			}
			rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), row))

		case "Emart":
			row, err := p.processEmartSegment(filePath, pageNumbers[pageIdx], text, pageLabel)
			if err != nil {
				rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Emart",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				}))
				continue
			}
			rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), row))

		case "Kingfood":
			row, err := p.processKingfoodSegment(filePath, pageNumbers[pageIdx], text, pageLabel)
			if err != nil {
				rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Kingfood",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				}))
				continue
			}
			rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), row))

		case "Winmart":
			// Re-extract this page's text with extractWinmartPageText
			// instead of using the shared pass's `text` directly — see
			// that function's doc comment (winmart_pdftext.go) for why.
			// Use pageNumbers[pageIdx] (the real, uncompacted PDF page
			// number), NOT the loop's pageIdx itself: extractPageTexts
			// skips null pages without appending a placeholder, so
			// pageIdx only equals "real page number minus one" when no
			// earlier page in this document was null. Passing pageIdx
			// directly here would silently re-extract the WRONG page
			// whenever an earlier page is null, with no error returned
			// to trigger the fallback below.
			winmartText := text
			if improved, wErr := extractWinmartPageTextFromFile(filePath, pageNumbers[pageIdx]-1); wErr == nil && improved != "" {
				winmartText = improved
			}
			row, err := p.processWinmartSegment(filePath, pageNumbers[pageIdx], winmartText, pageLabel)
			if err != nil {
				rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Winmart",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				}))
				continue
			}
			rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), row))

		case "FujiMart":
			row, err := p.processFujimartSegment(filePath, pageNumbers[pageIdx], text, pageLabel)
			if err != nil {
				rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "FujiMart",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				}))
				continue
			}
			rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), row))

		case "JMart":
			row, err := p.processJMartSegment(filePath, pageNumbers[pageIdx], text, pageLabel)
			if err != nil {
				rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "JMart",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				}))
				continue
			}
			rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), row))

		case maxidiSystem:
			row, err := p.processMaxidiSegment(filePath, pageNumbers[pageIdx], text, pageLabel)
			if err != nil {
				rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: maxidiSystem,
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				}))
				continue
			}
			rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), row))

		default:
			reason := "không nhận diện được nhà cung cấp"
			if v != "" {
				reason = "nhà cung cấp " + v + " chưa được hỗ trợ"
			}
			rows = append(rows, emitIdentifiedOrderRow(emit, filePath, fmt.Sprintf("page:%d:segment:1", physicalPage), OrderRow{
				FileName: filepath.Base(filePath), Page: pageLabel, System: v,
				Status: StatusFailed + " - " + reason, StatusKind: StatusKindFailed,
			}))
		}
	}

	return rows, nil
}

func emitRows(emit func(OrderRow), rows []OrderRow) {
	if emit == nil {
		return
	}
	for _, row := range rows {
		emit(row)
	}
}

func splitPageIntoPOs(text string) ([]string, bool) {
	counts := coop.CountPOsOnPage(text)
	if counts.POM343 == 0 || counts.SubTotal == 0 || counts.POM343 != counts.SubTotal {
		return nil, false
	}
	if counts.POM343 == 1 {
		return []string{text}, true
	}
	segments := coop.SplitMultiPO(text)
	if len(segments) == 0 {
		return nil, false
	}
	return segments, true
}

func (p *RealProcessor) processSegment(filePath string, realPageNum int, text, pageLabel string) (OrderRow, error) {
	info := coop.ParseInvoiceInfo(text)
	notes := coop.ExtractNotes(text)
	shipTo := coop.ExtractShipTo(text)

	entryDate := coop.ConvertDateFormat(info.EntryDate)
	cancelDate := coop.ConvertDateFormat(info.CancelDate)
	cancelDate, err := coop.ResolveCancelDate(entryDate, cancelDate)
	if err != nil {
		return OrderRow{}, err
	}

	customerCode := "Không tìm thấy"
	if info.POLocation != "" && info.POLocation != "Không tìm thấy" {
		customerCode = p.Store.GetCustomerCode(info.POLocation)
		if customerCode == "Không tìm thấy" && len(info.POLocation) > 1 {
			half := info.POLocation[:len(info.POLocation)/2]
			customerCode = p.Store.GetCustomerCode(half)
		}
	}

	products, err := coop.ExtractProducts(text)
	if err != nil {
		return OrderRow{}, err
	}
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}
	for i := range products {
		products[i].Barcode = p.Store.ResolveSku(products[i].Barcode)
	}

	system := p.Store.GetSystemForCustomer(customerCode)
	if system == "COOPFOOD" {
		if addr := p.Store.GetCoopfoodAddress(customerCode); addr != "" {
			shipTo = shipTo + " - " + addr
		}
	} else {
		system = "COOPMART"
	}

	priceIndex, err := p.Pricing.FetchIndex("COOP")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := regionInfo(customerCode)
	description := fmt.Sprintf("%s PO%s", system, info.PONumber)
	if notes != "" {
		description += " - " + notes
	}

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber(info.PONumber),
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
		Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: fmt.Sprintf("%s PO%s", system, info.PONumber),
	})

	saigia := 0
	totalWeight := 0.0
	totalPackages := 0
	totalValue := 0.0
	promoTotals := map[string]*PromoItemSummary{}
	var skuLog []string
	var mismatchDetails []PriceMismatchDetail

	for _, product := range products {
		productInfo, _ := p.Store.GetProductInfo(product.Barcode)
		lineWeight := productInfo.WeightKg * product.Qty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(product.Qty / productInfo.PackSize))
		}
		totalWeight += lineWeight
		totalPackages += caseCount

		invoicePrice := product.Cost / product.Qty
		realPriceStr, _ := priceIndex.FindPrice(product.Barcode)
		realPrice, _ := strconv.ParseFloat(strings.ReplaceAll(realPriceStr, ",", ""), 64)

		promos := priceIndex.FindPromotions(product.Barcode, entryDate)
		lastExaminedPromo := ""
		lastExaminedPromoColumn := ""
		matched := false
		finalPrice := realPrice

		// finalPrice mirrors Python's giathucte (xulydonhang.py:1080-1148):
		// it is overwritten on EVERY examined promo whose split value is
		// non-empty — discounted from realPrice/giathuctegoc fresh each
		// time (Python re-reads giathuctegoc as its base at line
		// 1097-1099 before subtracting, so multiple promo candidates for
		// the same SKU never compound), not only when that candidate
		// turns out to match the invoice price. A first Go port only
		// updated finalPrice inside the closeEnough-matched branch, so an
		// unmatched product's Y column silently fell back to the
		// undiscounted realPrice instead of the last examined discounted
		// price real fixtures show (e.g. a genuine 30%-off SKU priced at
		// 42158 writes Y=29510.6 with a price-mismatch flag in Python,
		// not Y=42158).
		for _, promo := range promos {
			value := coop.SplitPromoText(promo.Value, system)
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
			if closeEnough(invoicePrice, candidatePrice) {
				matched = true
				break
			}
		}
		// Python's raw-price fallback comparison ("if not results:",
		// xulydonhang.py:1131-1147) only runs when NO promotions were
		// found at all — not merely when none of the found promotions'
		// split values matched. Gating this on len(promos)==0 preserves
		// that: a SKU with promo candidates that all fail to match stays
		// flagged as a price mismatch (using the last examined
		// candidatePrice, per the loop above) even if its raw realPrice
		// happens to equal the invoice price, exactly like Python.
		if len(promos) == 0 && closeEnough(invoicePrice, realPrice) {
			matched = true
		}
		unitPrice := appliedUnitPrice(matched, invoicePrice, finalPrice)
		skuLog = append(skuLog, formatSkuLogLine(product.Barcode, productInfo.Name, matched, invoicePrice, finalPrice, lastExaminedPromo, lastExaminedPromoColumn))

		productRow := excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber(info.PONumber),
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
			Description: description, SKU: product.Barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: product.Qty, UnitPrice: unitPrice,
			ProductName: productInfo.Name, CaseCount: caseCount, LineWeightKg: lineWeight, UseZFormula: true,
			// AQ (PromoContent/khuyenmai) is written unconditionally in
			// Python (xulydonhang.py:1183, inside the `for i, hangkm in
			// enumerate(nhieuCtkm)` loop that always runs at least once
			// even when khuyenmai is "") — NOT only on a price mismatch
			// (that's a separate write at line 1168, which this
			// unconditional one always overwrites with the same value
			// anyway). Confirmed against real fixtures: matched rows with
			// no price mismatch still carry a real AQ value (e.g.
			// 102945235-00's product rows show AQ="20%" with
			// Y_has_comment=false).
			PromoContent: lastExaminedPromo,
		}
		productRowIndex := len(rows)
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
			mismatchDetails = append(mismatchDetails, PriceMismatchDetail{
				SKU: product.Barcode, ProductName: productInfo.Name,
				InvoicePrice: invoicePrice, SystemPrice: finalPrice, Qty: product.Qty,
				ExcelRow: productRowIndex, PromoText: truncatePromoText(lastExaminedPromo),
				PromoDateRange: lastExaminedPromoColumn,
			})
		}
		rows = append(rows, productRow)
		totalValue += unitPrice * product.Qty

		// currentRowIndex mirrors Python's current_row pointer through
		// this loop (xulydonhang.py:1174-1250): "AQ{current_row} =
		// khuyenmai" (line 1183) is written EVERY iteration, using
		// whatever row current_row happens to point at that moment —
		// and current_row only advances when a bonus row is actually
		// created (inside the "if kiemtra" block, line 1226), not once
		// per loop iteration. Net effect (verified against real
		// fixtures, e.g. 103108366-00.json rows 1-2): the FIRST promo
		// part's AQ lands on the main product row as expected, but every
		// SUBSEQUENT part's AQ lands on the PRECEDING bonus row (the one
		// the previous iteration just created), not on that part's own
		// bonus row — an off-by-one quirk in the original that this
		// mirrors exactly rather than "fixing", since golden-fixture
		// parity is the point.
		// Gated on matched: a gift is part of the SAME CTKM that explains
		// the invoice price, not a separate thing — when no promo's
		// discount actually matched the invoice (a genuine price
		// mismatch), lastExaminedPromo is just whichever promo was
		// examined LAST (see the price loop above), not a confirmed
		// applicable one, so granting a gift from it would be a guess.
		// Deliberate divergence from Python (xulydonhang.py:1181's
		// nhieuCtkm split runs unconditionally, even after a mismatch) —
		// confirmed as the intended business rule, not preserved.
		if matched {
			currentRowIndex := productRowIndex
			for i, promoPart := range strings.Split(lastExaminedPromo, "|") {
				rows[currentRowIndex].PromoContent = lastExaminedPromo

				bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, promoPart, product, i, entryDate, cancelDate, shipTo,
					customerCode, description, warehouse, region, statCode, orderNumber(info.PONumber))
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
	}

	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, entryDate, cancelDate,
			shipTo, customerCode, description, warehouse, region, statCode, orderNumber(info.PONumber)); added {
			totalWeight += bonusRow.LineWeightKg
			totalPackages += bonusRow.CaseCount
			accumulatePromoItem(promoTotals, bonusRow.SKU, bonusRow.ProductName, bonusRow.Qty)
			rows = append(rows, bonusRow)
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

	// Tách riêng tài liệu của ĐÚNG đơn này để link Drive của nó chỉ mở ra
	// chính nó: cắt trang với PDF, ghi khối văn bản của đơn với báo cáo
	// .txt. Thất bại thì lùi về upload nguyên file — thà link rộng hơn cần
	// còn hơn không có link nào.
	uploadPath := filePath
	if extractedPath, cleanup, extractErr := extractOrderDocument(filePath, realPageNum, text); extractErr == nil {
		uploadPath = extractedPath
		defer cleanup()
	} else if p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không tách được tài liệu của đơn để upload Drive (dùng nguyên file thay thế): %v", extractErr))
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, uploadPath, driveupload.Metadata{
		Vendor:       "COOP",
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
		FileName: filepath.Base(filePath), Page: pageLabel, System: system, MaKhachHang: customerCode,
		// So dong that cua don nay trong so dat hang - push MISA dua vao
		// day de tach file theo nhanh ke toan.
		ExcelRows: excelRowsFrom(startRow, len(rows)),
		PO:        info.PONumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		DriveURL: driveURL,
		ShipTo:   shipTo, EntryDate: entryDate, CancelDate: cancelDate,
		TotalWeightKg: totalWeightFormatted, TotalPackages: totalPackages,
		PromoItems: finalizePromoItems(promoTotals),
		SkuLog:     skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
}

// orderNumber mirrors write_to_dondathang's order-number field: it
// always uses the literal vendor code "COOP" (the string
// process_coop_invoice hardcodes when calling write_to_dondathang),
// NOT the resolved system (COOPMART/COOPFOOD) — preserve exactly.
func orderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHCOOP-%s", poNumber)
}
