package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"order-processor/internal/driveupload"
	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/emart"
	"order-processor/internal/processing/excelwriter"
)

// emartCustomerCode mirrors write_to_dondathang_emart's default/only
// makhachhang value — the literal "MN_MT_KH0032", hardcoded at the
// process_file call site (xulydonhang.py:9363). Emart never derives a
// customer code from the PDF (no fuzzy-match, unlike Satra/Winmart).
const emartCustomerCode = "MN_MT_KH0032"

// emartStoreShortCode mirrors xulydonhang.py:4990-4994's hardcoded
// mapping dict exactly.
var emartStoreShortCode = map[string]string{
	"EMART GO VAP": "PVT",
	"EMART PHI":    "PHI",
	"EMART SALA":   "SALA",
}

// emartStoreFullName mirrors xulydonhang.py:5046-5051's if/elif chain
// (only these 3 short codes get a full name written to column K; any
// other short code gets no K value at all, matching Python having no
// final else branch).
var emartStoreFullName = map[string]string{
	"PVT":  "SIÊU THỊ EMART PHAN VĂN TRỊ",
	"SALA": "SIÊU THỊ EMART SALA",
	"PHI":  "SIÊU THỊ EMART PHAN HUY ÍCH",
}

// emartRegionInfo mirrors write_to_dondathang_emart's warehouse/region
// branching (xulydonhang.py:5003-5009). The MB branch is unreachable
// with real Emart input today — customerCode is always the hardcoded
// constant emartCustomerCode, which never starts with "MB" — but this is
// modeled as a full 2-branch function anyway, matching the
// winmartRegionInfo/bigcRegionInfo precedent, for architectural
// consistency and in case a future change gives Emart a real
// customer-code source. Confirmed NOT a fit for the shared regionInfo()
// (processor_shared.go): that function's non-MB branch returns warehouse
// "LA_TP", but Emart's real non-MB warehouse is "LA_KHO2026" — the same
// divergence already handled for Winmart/BigC.
func emartRegionInfo(customerCode string) (region, statCode, warehouse string) {
	if strings.HasPrefix(customerCode, "MB") {
		return "MT_MB", "HN", "TP_HN_12"
	}
	return "MT_MN", "LA", "LA_KHO2026"
}

// emartOrderNumber mirrors write_to_dondathang_emart's order-number field
// (xulydonhang.py:5024): f'ĐĐH{vendor}{STT_donhang_str}' where vendor is
// the uppercased literal "EMART" and STT_donhang_str is f"-{po_number}".
func emartOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHEMART-%s", poNumber)
}

// emartStoreNames maps a parsed "Delivery to :" store name to its short
// code and, if recognized, its full Vietnamese display name for column
// K — mirroring xulydonhang.py:4990-4996's `mapping.get(congtrinh,
// congtrinh)` and :5046-5051's if/elif chain exactly. An unrecognized
// store keeps its raw text as the short code (matching Python's dict
// .get fallback) and gets no K value at all (matching Python's if/elif
// having no final else branch — fullName is simply "").
func emartStoreNames(storeName string) (shortCode, fullName string) {
	shortCode = storeName
	if mapped, ok := emartStoreShortCode[storeName]; ok {
		shortCode = mapped
	}
	fullName = emartStoreFullName[shortCode]
	return shortCode, fullName
}

// processEmartSegment mirrors the Emart branch of process_file
// (xulydonhang.py:9314-9384) plus write_to_dondathang_emart
// (:4974-5330). Emart is "1 page = 1 order", the same family as Coop/
// Lotte/Satra/Winmart. A trailing PDF page that lacks Emart's identify
// marker falls through to the shared per-page dispatch loop's default
// case (coop_processor.go), which emits a Failed/"Thất bại" OrderRow for
// that page.
//
// Column E (ShipTo) holds the same short store LABEL used for the K
// lookup (e.g. "EMART GO VAP"), NOT a street address — unlike every
// other ported vendor. This mirrors xulydonhang.py's
// `diachigiaohang = congtrinh` (:4987) where congtrinh IS tenstore, the
// already-truncated ("Delivery to :"-line split on 3 spaces) label.
func (p *RealProcessor) processEmartSegment(filePath string, realPageNum int, text, pageLabel string) (OrderRow, error) {
	poNumber, entryDate, cancelDate, storeName, ok := emart.ParseOrderInfo(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được số PO/ngày đặt hàng")
	}

	products := emart.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	priceIndex, err := p.Pricing.FetchIndex("EMART")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := emartRegionInfo(emartCustomerCode)
	orderNum := emartOrderNumber(poNumber)
	description := fmt.Sprintf("EMART PO%s", poNumber)
	_, fullStoreName := emartStoreNames(storeName)

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: storeName, CustomerCode: emartCustomerCode,
		Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: description, StoreName: fullStoreName,
		NoCaseCount: true,
	})

	saigia := 0
	totalWeight := 0.0
	totalValue := 0.0
	var skuLog []string
	var mismatchDetails []PriceMismatchDetail

	for _, rawProduct := range products {
		barcode := p.Store.ResolveSku(rawProduct.Barcode)
		productInfo, _ := p.Store.GetProductInfo(barcode)
		qty := float64(rawProduct.OUQty)
		lineWeight := productInfo.WeightKg * qty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(qty / productInfo.PackSize))
		}
		totalWeight += lineWeight

		// giahoadon (xulydonhang.py:5095): used DIRECTLY, no division by
		// qty. rawProduct.UnitPrice really is a per-unit price — see the
		// emart package's Product doc comment for why, and why this
		// differs from Winmart's same-named field.
		invoicePrice, _ := strconv.ParseFloat(rawProduct.UnitPrice, 64)

		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice := parseNumericField(realPriceStr)

		promos := priceIndex.FindPromotions(barcode, entryDate)
		lastExaminedPromo := ""
		lastExaminedPromoColumn := ""
		matched := false
		finalPrice := realPrice

		for _, promo := range promos {
			// Same CR normalization BigC's own promo.Value read needed
			// (bigc_processor.go:270): openpyxl's write/read round-trip
			// of the frozen fixture silently normalizes a bare '\r' in
			// the source Google Sheets CSV to '\n' (independently of any
			// adjacent '\n'), while Go's excelize instead escapes '\r'
			// as "&#xD;" and leaves it untouched. Normalizing here
			// reproduces Python's effective (if incidentally mangled)
			// AQ output instead of diverging on a cosmetic CR/LF
			// difference. Confirmed via real fixture mismatches on
			// 4501866956.pdf row 5, 4501873471.pdf row 1, and
			// 4501873478.pdf row 3 (all col AQ, all "\r\n" vs "\n\n").
			value := strings.ReplaceAll(promo.Value, "\r", "\n")
			if value == "" {
				continue
			}
			lastExaminedPromo = value
			lastExaminedPromoColumn = promo.Column
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
		skuLog = append(skuLog, formatSkuLogLine(barcode, productInfo.Name, matched, invoicePrice, finalPrice, lastExaminedPromo, lastExaminedPromoColumn))

		productRow := excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: storeName, CustomerCode: emartCustomerCode,
			Description: description, SKU: barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: qty, UnitPrice: finalPrice,
			ProductName: productInfo.Name, CaseCount: caseCount, LineWeightKg: lineWeight, UseZFormula: true,
			PromoContent: lastExaminedPromo, NoCaseCount: true,
		}
		productRowIndex := len(rows)
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
			mismatchDetails = append(mismatchDetails, PriceMismatchDetail{
				SKU: barcode, ProductName: productInfo.Name,
				InvoicePrice: invoicePrice, SystemPrice: finalPrice,
				ExcelRow: productRowIndex,
			})
		}
		rows = append(rows, productRow)
		totalValue += finalPrice * qty

		// Multi-CTKM split (xulydonhang.py:5203, "nhieuCtkm =
		// khuyenmai.split('|')") — same shape as Coop's own multi-CTKM
		// loop (coop_processor.go's processSegment), NOT Winmart's/
		// Lotte's single-promo-attempt shape: Emart's Python genuinely
		// loops over "|"-split promo parts with its own i==0/i>0 AO/AP
		// placement branch, which buildPromoBonusRow's own index
		// parameter already models.
		currentRowIndex := productRowIndex
		for i, promoPart := range strings.Split(lastExaminedPromo, "|") {
			rows[currentRowIndex].PromoContent = lastExaminedPromo

			bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, promoPart,
				coop.Product{Barcode: barcode, Qty: qty}, i, entryDate, cancelDate, storeName,
				emartCustomerCode, description, warehouse, region, statCode, orderNum)
			if !added {
				continue
			}
			totalWeight += bonusRow.LineWeightKg
			bonusRow.NoCaseCount = true

			// Emart's own no-{...}-brace fallback
			// (xulydonhang.py:5230/:5240, "KM Rời - Không Che Barcode")
			// never writes AP, for EITHER i==0 or i>0 — a third distinct
			// fallback string from Coop's default ("KM Bó Kèm - Che
			// Barcode") and Winmart's ("KM Giao Rời - Không Che
			// Barcode"). Override the shared helper's Coop-flavored
			// result here, scoped to Emart only, for BOTH branches
			// (unlike Lotte/Winmart, which only ever call with index 0).
			if coop.ExtractBraceContent(promoPart) == "" {
				mainRowNote = "KM Rời - Không Che Barcode"
				mainRowBundleSku = ""
				bonusRow.PromoBundleSku = ""
				if i != 0 {
					bonusRow.PromoNote = "KM Rời - Không Che Barcode"
				}
			}

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

	// Invoice-level ("Hóa Đơn") promo bonus row (xulydonhang.py:5274-5316).
	// Does NOT reuse the shared buildInvoiceBonusRow — Q gets only the
	// FIRST matched SKU (kiemtra[0], xulydonhang.py:5290), not a joined
	// list, the same divergence already handled for Winmart. Python
	// indexes kiemtra[0] with NO length guard (a latent crash risk if
	// kmhoadon maps to zero SKUs); this mirrors buildInvoiceBonusRow's
	// own len(skus)==0 guard instead of reproducing that risk.
	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		// Same CR normalization as the per-item promo.Value read above,
		// same root cause (openpyxl round-trip quirk in the frozen
		// fixture, see comment above) — applied here too so an
		// invoice-level promo landing in PromoContent (line ~269) can't
		// hit the same cosmetic CR/LF divergence, even though no
		// currently-available fixture happens to exercise this path.
		invoicePromo = strings.ReplaceAll(invoicePromo, "\r", "\n")
		invoiceSkus := p.Store.FindSkusMentioned(invoicePromo)
		if amount, ok := coop.ExtractMoneyAmount(invoicePromo); ok && amount > 0 && len(invoiceSkus) > 0 {
			invoiceSku := invoiceSkus[0]
			soluongkm := math.Floor(totalValue / float64(amount))
			invoiceInfo, _ := p.Store.GetProductInfo(invoiceSku)
			invoiceWeight := invoiceInfo.WeightKg * soluongkm
			invoiceCase := 0
			if invoiceInfo.PackSize > 0 {
				invoiceCase = int(math.Ceil(soluongkm / invoiceInfo.PackSize))
			}
			totalWeight += invoiceWeight

			invoiceNote := coop.ExtractBraceContent(invoicePromo)
			if invoiceNote == "" {
				invoiceNote = "KM Bó Kèm - Che Barcode" // xulydonhang.py:5312
			}

			rows = append(rows, excelwriter.Row{
				EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
				Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: storeName, CustomerCode: emartCustomerCode,
				Description: description, SKU: invoiceSku, Warehouse: warehouse, VATPercent: 8,
				RegionCode: region, StatCode: statCode, IsPromoItem: true, Qty: soluongkm,
				ProductName: invoiceInfo.Name, CaseCount: invoiceCase, LineWeightKg: invoiceWeight, UseZFormula: false,
				PromoContent: invoicePromo, PromoNote: invoiceNote, NoCaseCount: true,
			})
		}
	}

	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
		Vendor:       "EMART",
		EntryDate:    entryDate,
		CustomerCode: emartCustomerCode,
		CancelDate:   cancelDate,
		OutputName:   poNumber,
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
	// Python's own status logic (xulydonhang.py:9367) flags a warning
	// when saigia>0 OR the store couldn't be resolved (tenstore
	// falls back to "Không xác định") — mirrored here via storeName=="".
	if saigia > 0 || storeName == "" {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Emart", MaKhachHang: emartCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		DriveURL: driveURL,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
}
