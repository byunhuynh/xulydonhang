package processing

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/lotte"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
	"order-processor/internal/processing/vendor"
)

const coopDebtDays = 60 // songayno_MT in xulydonhang.py — one global constant, shared by every vendor's write function, not Coop-specific

// PricingSource abstracts fetching a vendor's price/promotion data for
// one order, so tests substitute a fixture-backed implementation instead
// of a live Google Sheets fetch. Production wiring uses *pricing.HTTPSource.
type PricingSource interface {
	FetchIndex(sheetKey string) (*pricing.Index, error)
}

// RealProcessor implements processing.Processor for the Coop vendor.
// Any page whose text doesn't match Coop's vendor markers produces a
// single Failed OrderRow explaining why, rather than being silently
// skipped — support for other vendors is added in later phases by
// extending this same dispatch.
type RealProcessor struct {
	Store     *productdata.Store
	Pricing   PricingSource
	ExcelPath string
}

func (p *RealProcessor) Process(ctx context.Context, filePath string, stt int) ([]OrderRow, error) {
	pageTexts, err := extractPageTexts(filePath)
	if err != nil {
		return []OrderRow{{
			FileName:   filepath.Base(filePath),
			Status:     StatusFailed + " - không đọc được PDF: " + err.Error(),
			StatusKind: StatusKindFailed,
		}}, nil
	}

	var rows []OrderRow
	for pageIdx, text := range pageTexts {
		pageLabel := fmt.Sprintf("%d/%d", pageIdx+1, len(pageTexts))
		v := vendor.Identify(text)

		switch v {
		case "Coop":
			segments, ok := splitPageIntoPOs(text)
			if !ok {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Coop",
					Status: StatusFailed + " - không đếm khớp số đơn trên trang", StatusKind: StatusKindFailed,
				})
				continue
			}
			for segIdx, segment := range segments {
				segLabel := fmt.Sprintf("%d/%d", segIdx+1, len(segments))
				row, err := p.processSegment(filePath, segment, segLabel)
				if err != nil {
					rows = append(rows, OrderRow{
						FileName: filepath.Base(filePath), Page: segLabel, System: "Coop",
						Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
					})
					continue
				}
				rows = append(rows, row)
			}

		case "Lotte":
			row, err := p.processLotteSegment(filePath, text, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Lotte",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)

		default:
			reason := "không nhận diện được nhà cung cấp"
			if v != "" {
				reason = "nhà cung cấp " + v + " chưa được hỗ trợ"
			}
			rows = append(rows, OrderRow{
				FileName: filepath.Base(filePath), Page: pageLabel, System: v,
				Status: StatusFailed + " - " + reason, StatusKind: StatusKindFailed,
			})
		}
	}

	return rows, nil
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

// xPlus1Pattern mirrors the "(\d+)\s*\+\s*1" match inside
// write_to_dondathang's promo-bonus-quantity logic.
var xPlus1Pattern = regexp.MustCompile(`(\d+)\s*\+\s*1`)

func (p *RealProcessor) processSegment(filePath, text, pageLabel string) (OrderRow, error) {
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
	totalValue := 0.0

	for _, product := range products {
		productInfo, _ := p.Store.GetProductInfo(product.Barcode)
		lineWeight := productInfo.WeightKg * product.Qty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(product.Qty / productInfo.PackSize))
		}
		totalWeight += lineWeight

		invoicePrice := product.Cost / product.Qty
		realPriceStr, _ := priceIndex.FindPrice(product.Barcode)
		realPrice, _ := strconv.ParseFloat(strings.ReplaceAll(realPriceStr, ",", ""), 64)

		promos := priceIndex.FindPromotions(product.Barcode, entryDate)
		lastExaminedPromo := ""
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

		productRow := excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber(info.PONumber),
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
			Description: description, SKU: product.Barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: product.Qty, UnitPrice: finalPrice,
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
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
		}

		productRowIndex := len(rows)
		rows = append(rows, productRow)
		totalValue += finalPrice * product.Qty

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
		currentRowIndex := productRowIndex
		for i, promoPart := range strings.Split(lastExaminedPromo, "|") {
			rows[currentRowIndex].PromoContent = lastExaminedPromo

			bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, promoPart, product, i, entryDate, cancelDate, shipTo,
				customerCode, description, warehouse, region, statCode, info.PONumber)
			if !added {
				continue
			}
			totalWeight += bonusRow.LineWeightKg
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

	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, entryDate, cancelDate,
			shipTo, customerCode, description, warehouse, region, statCode, info.PONumber); added {
			totalWeight += bonusRow.LineWeightKg
			rows = append(rows, bonusRow)
		}
	}

	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	if err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription); err != nil {
		return OrderRow{}, err
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: system, MaKhachHang: customerCode,
		PO: info.PONumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
	}, nil
}

// orderNumber mirrors write_to_dondathang's order-number field: it
// always uses the literal vendor code "COOP" (the string
// process_coop_invoice hardcodes when calling write_to_dondathang),
// NOT the resolved system (COOPMART/COOPFOOD) — preserve exactly.
func orderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHCOOP-%s", poNumber)
}

// regionInfo mirrors write_to_dondathang's warehouse/region branching:
// customer codes starting with "MB" (Miền Bắc) map to the Hà Nội
// warehouse; everything else defaults to Miền Nam / Long An.
func regionInfo(customerCode string) (region, statCode, warehouse string) {
	if strings.HasPrefix(customerCode, "MB") {
		return "MT_MB", "HN", "TP_HN_12"
	}
	return "MT_MN", "LA", "LA_TP"
}

func closeEnough(a, b float64) bool {
	const relTol = 1e-4
	return math.Abs(a-b) <= relTol*math.Max(math.Abs(a), math.Abs(b))
}

func buildPromoBonusRow(store *productdata.Store, promoPart string, product coop.Product, index int,
	entryDate, cancelDate, shipTo, customerCode, description, warehouse, region, statCode, poNumber string,
) (row excelwriter.Row, mainRowNote string, mainRowBundleSku string, added bool) {
	skus := store.FindSkusMentioned(promoPart)
	bonusMatch := xPlus1Pattern.FindStringSubmatch(promoPart)
	bonusQty := product.Qty
	bonusSku := ""
	if len(skus) > 0 {
		bonusSku = strings.Join(skus, ", ")
	}
	if bonusMatch != nil {
		x, _ := strconv.Atoi(bonusMatch[1])
		if bonusSku == "" {
			bonusSku = product.Barcode
		}
		if x >= 2 {
			bonusQty = math.Floor(bonusQty / float64(x))
		}
	}
	if bonusSku == "" {
		return excelwriter.Row{}, "", "", false
	}

	bonusInfo, _ := store.GetProductInfo(bonusSku)
	bonusWeight := bonusInfo.WeightKg * bonusQty
	bonusCase := 0
	if bonusInfo.PackSize > 0 {
		bonusCase = int(math.Ceil(bonusQty / bonusInfo.PackSize))
	}

	bundleNote := coop.ExtractBraceContent(promoPart)
	if bundleNote == "" {
		bundleNote = "KM Bó Kèm - Che Barcode"
	}
	lower := strings.ToLower(bundleNote)
	isBundle := strings.Contains(lower, "bó kèm") || strings.Contains(lower, "quấn kèm")
	bundleSkuValue := ""
	if isBundle {
		bundleSkuValue = fmt.Sprintf("%s_%s_1", coop.LastFourDigits(product.Barcode), coop.LastFourDigits(bonusSku))
	}

	row = excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber(poNumber),
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
		Description: description, SKU: bonusSku, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsPromoItem: true, Qty: bonusQty, ProductName: bonusInfo.Name,
		CaseCount: bonusCase, LineWeightKg: bonusWeight, UseZFormula: false,
	}
	if isBundle {
		row.PromoBundleSku = bundleSkuValue
	}

	if index == 0 {
		// Python (xulydonhang.py:1201) writes the first promo item's AO
		// note onto the MAIN PRODUCT ROW, not this bonus row; AP goes
		// onto both the main row and this bonus row (already set above).
		mainRowNote = bundleNote
		mainRowBundleSku = bundleSkuValue
	} else {
		// Python (xulydonhang.py:1211) writes AO for i>0 onto that
		// item's own bonus row.
		row.PromoNote = bundleNote
	}

	return row, mainRowNote, mainRowBundleSku, true
}

func buildInvoiceBonusRow(store *productdata.Store, invoicePromo string, totalValue float64,
	entryDate, cancelDate, shipTo, customerCode, description, warehouse, region, statCode, poNumber string,
) (excelwriter.Row, bool) {
	skus := store.FindSkusMentioned(invoicePromo)
	amount, ok := coop.ExtractMoneyAmount(invoicePromo)
	if !ok || amount <= 0 || len(skus) == 0 {
		return excelwriter.Row{}, false
	}
	bonusQty := math.Floor(totalValue / float64(amount))
	bonusInfo, _ := store.GetProductInfo(skus[0])
	bonusWeight := bonusInfo.WeightKg * bonusQty
	bonusCase := 0
	if bonusInfo.PackSize > 0 {
		bonusCase = int(math.Ceil(bonusQty / bonusInfo.PackSize))
	}
	bundleNote := coop.ExtractBraceContent(invoicePromo)
	if bundleNote == "" {
		bundleNote = "KM Bó Kèm - Che Barcode"
	}
	return excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber(poNumber),
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
		Description: description, SKU: strings.Join(skus, ", "), Warehouse: warehouse, VATPercent: 8,
		RegionCode: region, StatCode: statCode, IsPromoItem: true, Qty: bonusQty, ProductName: bonusInfo.Name,
		CaseCount: bonusCase, LineWeightKg: bonusWeight, PromoNote: bundleNote, PromoContent: invoicePromo,
		UseZFormula: false,
	}, true
}

// lotteOrderNumber mirrors write_to_dondathang_lotte's order-number
// field (xulydonhang.py:2018): f'ĐĐH{vendor}{STT_donhang_str}' where
// vendor is the uppercased literal "LOTTE" and STT_donhang_str is
// f"-{po_number}".
func lotteOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHLOTTE-%s", poNumber)
}

// processLotteSegment mirrors the Lotte branch of process_file
// (xulydonhang.py:9079-9139) plus write_to_dondathang_lotte
// (:1968-2318). Structurally identical to processSegment's promo-
// matching/bonus-row logic — write_to_dondathang_lotte calls the exact
// same helper functions Coop's write_to_dondathang does
// (find_price_by_sku, find_all_promotions_by_sku_and_time,
// extract_discount, check_value_in_sanpham, laycachbo_khuyenmai,
// tachtien_khuyenmai, layduoi_mahang), just with vendor="LOTTE" instead
// of "COOPMART"/"COOPFOOD". Two confirmed differences from Coop's path:
// (1) Lotte's promo value is used as-is — no SplitPromoText (that's
// Coop's cm/cf-bundling convention only, write_to_dondathang_lotte never
// calls tachkhuyenmai_coop); (2) no ShipTo-address special-casing (no
// COOPFOOD-equivalent concept for Lotte).
func (p *RealProcessor) processLotteSegment(filePath, text, pageLabel string) (OrderRow, error) {
	// Normalize before handing text to the lotte package: this repo's Go
	// PDF library (github.com/ledongthuc/pdf, unlike the PyMuPDF the
	// original Python used) inserts spurious blank lines into
	// GetPlainText's output for real Lotte PDFs — confirmed on the sample
	// fixture (đơn hàng/08-2026/260727-01013-00057.pdf): raw extraction
	// yields "", "", "", "Ord sheet", "", "2607270101300057", ... instead
	// of PyMuPDF's clean "Ord sheet", "2607270101300057", ... with no
	// blank lines at all. lotte.ExtractCancelDate/ExtractStoreName/
	// ExtractProducts locate content by marker match so blank lines don't
	// affect them, but lotte.ParseOrderInfo indexes the raw second line
	// directly (mirroring Python's lines[1], which is only correct
	// against PyMuPDF-shaped text) — stripping blank lines here
	// reconstructs that exact shape without touching the already-shipped
	// lotte package or the shared, Coop-tested extractPageTexts.
	text = stripBlankLines(text)

	info, err := lotte.ParseOrderInfo(text)
	if err != nil {
		return OrderRow{}, err
	}

	cancelDate := lotte.ExtractCancelDate(text, info.PONumber)
	storeName := lotte.ExtractStoreName(text, info.PONumber)
	shipTo := "Lotte " + storeName

	products := lotte.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	// get_makhachhang_lotte(store_code[1:]) — the leading digit of the
	// 5-digit store code is dropped before matching (xulydonhang.py:9109).
	customerCode := ""
	if len(info.StoreCode) > 1 {
		customerCode = p.Store.GetCustomerCodeBySuffix("LOTTE", info.StoreCode[1:])
	}
	if customerCode == "" {
		customerCode = "Không xác định"
	}

	priceIndex, err := p.Pricing.FetchIndex("LOTTE")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := regionInfo(customerCode)
	description := fmt.Sprintf("LOTTE PO%s", info.PONumber)

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: info.EntryDate, DebtDays: coopDebtDays, OrderNumber: lotteOrderNumber(info.PONumber),
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
		Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: description,
	})

	saigia := 0
	totalWeight := 0.0
	totalValue := 0.0

	for _, rawProduct := range products {
		barcode := p.Store.ResolveSku(rawProduct.Barcode)
		productInfo, _ := p.Store.GetProductInfo(barcode)
		qty := float64(rawProduct.QtyBox * rawProduct.BoxQty)
		lineWeight := productInfo.WeightKg * qty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(qty / productInfo.PackSize))
		}
		totalWeight += lineWeight

		invoicePrice := rawProduct.TotalPrice / qty
		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice, _ := strconv.ParseFloat(strings.ReplaceAll(realPriceStr, ",", ""), 64)

		promos := priceIndex.FindPromotions(barcode, info.EntryDate)
		lastExaminedPromo := ""
		matched := false
		finalPrice := realPrice

		// No SplitPromoText here (see function doc): Lotte's promo cell
		// is used exactly as returned, not split into cm/cf variants.
		for _, promo := range promos {
			value := promo.Value
			lastExaminedPromo = value
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
		if len(promos) == 0 && closeEnough(invoicePrice, realPrice) {
			matched = true
		}

		productRow := excelwriter.Row{
			EntryDate: info.EntryDate, DebtDays: coopDebtDays, OrderNumber: lotteOrderNumber(info.PONumber),
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
			Description: description, SKU: barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: qty, UnitPrice: finalPrice,
			ProductName: productInfo.Name, CaseCount: caseCount, LineWeightKg: lineWeight, UseZFormula: true,
			PromoContent: lastExaminedPromo,
		}
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
		}

		productRowIndex := len(rows)
		rows = append(rows, productRow)
		totalValue += finalPrice * qty

		currentRowIndex := productRowIndex
		for i, promoPart := range strings.Split(lastExaminedPromo, "|") {
			rows[currentRowIndex].PromoContent = lastExaminedPromo

			bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, promoPart,
				coop.Product{Barcode: barcode, Qty: qty}, i, info.EntryDate, cancelDate, shipTo,
				customerCode, description, warehouse, region, statCode, info.PONumber)
			if !added {
				continue
			}
			bonusRow.OrderNumber = lotteOrderNumber(info.PONumber) // buildPromoBonusRow hardcodes Coop's order number
			totalWeight += bonusRow.LineWeightKg
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

	if invoicePromo := priceIndex.FindInvoicePromotion(info.EntryDate); invoicePromo != "" {
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, info.EntryDate, cancelDate,
			shipTo, customerCode, description, warehouse, region, statCode, info.PONumber); added {
			bonusRow.OrderNumber = lotteOrderNumber(info.PONumber) // buildInvoiceBonusRow hardcodes Coop's order number
			totalWeight += bonusRow.LineWeightKg
			rows = append(rows, bonusRow)
		}
	}

	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	if err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription); err != nil {
		return OrderRow{}, err
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Lotte", MaKhachHang: customerCode,
		PO: info.PONumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
	}, nil
}

// stripBlankLines drops every line that is empty or all-whitespace,
// rejoining the rest with "\n". See processLotteSegment's comment for
// why this is needed: it reconstructs the blank-line-free shape
// PyMuPDF's text extraction produces (and that the lotte package's
// position-dependent parsing, ParseOrderInfo in particular, assumes)
// from this repo's Go PDF library's GetPlainText output, which inserts
// extra blank lines on real Lotte PDFs that PyMuPDF does not.
func stripBlankLines(text string) string {
	lines := strings.Split(text, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\n")
}
