package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/fujimart"
)

// fujimartCustomerCode mirrors write_to_dondathang_fujimart's default/
// only makhachhang value — the literal "MB_MT_FUJI", hardcoded at the
// process_file call site (xulydonhang.py:8919). FujiMart never derives a
// customer code from the PDF or OCR.
const fujimartCustomerCode = "MB_MT_FUJI"

// fujimartRegionInfo mirrors write_to_dondathang_fujimart's warehouse/
// region branching (xulydonhang.py:2753-2760). The non-MB branch is
// unreachable with real FujiMart input today — customerCode is always
// the hardcoded constant fujimartCustomerCode, which always starts with
// "MB" — but this is modeled as a full 2-branch function anyway,
// matching the winmartRegionInfo/emartRegionInfo precedent, for
// architectural consistency. Confirmed NOT a fit for the shared
// regionInfo() (processor_shared.go): that function's non-MB branch
// returns warehouse "LA_TP", but FujiMart's real non-MB warehouse is
// "LA_KHO2026" — the same divergence already handled for Winmart/Emart.
func fujimartRegionInfo(customerCode string) (region, statCode, warehouse string) {
	if strings.HasPrefix(customerCode, "MB") {
		return "MT_MB", "HN", "TP_HN_12"
	}
	return "MT_MN", "LA", "LA_KHO2026"
}

// fujimartOrderNumber mirrors write_to_dondathang_fujimart's order-
// number field (xulydonhang.py:2780): f'ĐĐH{vendor}{STT_donhang_str}'
// where vendor is the uppercased literal "FUJIMART" and STT_donhang_str
// is f"-{po_number}".
func fujimartOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHFUJIMART-%s", poNumber)
}

// processFujimartSegment mirrors the FujiMart branch of process_file
// (xulydonhang.py:8831-8964) plus write_to_dondathang_fujimart
// (:2732-3066). FujiMart is "1 page = 1 order", the same family as Coop/
// Lotte/Satra/Winmart/Emart. A trailing PDF page that lacks FujiMart's
// identify marker falls through to the shared per-page dispatch loop's
// default case (coop_processor.go), which emits a Failed/"Thất bại"
// OrderRow for that page.
func (p *RealProcessor) processFujimartSegment(filePath, text, pageLabel string) (OrderRow, error) {
	poNumber, entryDate, cancelDate, storeInfo, ok := fujimart.ParseOrderInfo(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được số PO/ngày đặt hàng")
	}

	products := fujimart.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	priceIndex, err := p.Pricing.FetchIndex("FUJIMART")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := fujimartRegionInfo(fujimartCustomerCode)
	orderNum := fujimartOrderNumber(poNumber)
	description := fmt.Sprintf("FUJIMART PO%s", poNumber)

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: storeInfo, CustomerCode: fujimartCustomerCode,
		Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: description,
	})

	saigia := 0
	totalWeight := 0.0
	totalValue := 0.0

	for _, rawProduct := range products {
		barcode := p.Store.ResolveSku(rawProduct.Barcode)
		productInfo, _ := p.Store.GetProductInfo(barcode)
		ouQty := parseNumericField(rawProduct.OUQty)
		totalPrice := parseNumericField(rawProduct.TotalPrice)

		lineWeight := productInfo.WeightKg * ouQty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(ouQty / productInfo.PackSize))
		}
		totalWeight += lineWeight

		// giahoadon (xulydonhang.py:2843): dongia / qty_ord_pcs — a LINE
		// TOTAL divided by quantity, the same shape as Winmart's
		// TotalPrice (NOT Emart's per-unit trap).
		invoicePrice := totalPrice / ouQty

		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice := parseNumericField(realPriceStr)

		promos := priceIndex.FindPromotions(barcode, entryDate)
		khuyenmai := ""
		matched := false
		finalPrice := realPrice

		for _, promo := range promos {
			value := promo.Value
			if value == "" {
				continue
			}
			khuyenmai = value
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
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: storeInfo, CustomerCode: fujimartCustomerCode,
			Description: description, SKU: barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: ouQty, UnitPrice: finalPrice,
			ProductName: productInfo.Name, CaseCount: caseCount, LineWeightKg: lineWeight, UseZFormula: true,
			PromoContent: khuyenmai,
		}
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
		}

		productRowIndex := len(rows)
		rows = append(rows, productRow)
		totalValue += finalPrice * ouQty

		// Per-item promo bonus row (xulydonhang.py:2949-3007) — single
		// attempt, buildPromoBonusRow always called with index=0 (no
		// "|"-split multi-CTKM loop, matching Winmart's/Lotte's shape,
		// not Coop's/Emart's).
		bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, khuyenmai,
			coop.Product{Barcode: barcode, Qty: ouQty}, 0, entryDate, cancelDate, storeInfo,
			fujimartCustomerCode, description, warehouse, region, statCode, orderNum)
		if added {
			totalWeight += bonusRow.LineWeightKg

			// FujiMart's own no-{...}-brace fallback text
			// (xulydonhang.py:2973, "KM Bó Kèm - Không Che Barcode")
			// differs from buildPromoBonusRow's shared Coop-flavored
			// default ("KM Bó Kèm - Che Barcode") — but BOTH strings
			// contain "bó kèm", so buildPromoBonusRow's own internal
			// bundle-SKU (AP) computation is unaffected by this text
			// override; only the AO note text itself needs overriding.
			// Python writes this fallback text ONLY onto the main
			// product row (xulydonhang.py:2973, at current_row, before
			// current_row is incremented for the bonus row) — never
			// onto the bonus row itself, matching buildPromoBonusRow's
			// own index==0 behavior (which likewise never sets the
			// bonus row's own PromoNote).
			if coop.ExtractBraceContent(khuyenmai) == "" {
				mainRowNote = "KM Bó Kèm - Không Che Barcode"
			}

			rows[productRowIndex].PromoNote = mainRowNote
			if mainRowBundleSku != "" {
				rows[productRowIndex].PromoBundleSku = mainRowBundleSku
			}
			rows = append(rows, bonusRow)
		}
	}

	// Invoice-level ("Hóa Đơn") promo bonus row (xulydonhang.py:3010-3047).
	// Does NOT reuse the shared buildInvoiceBonusRow — Q gets only the
	// first matched SKU (kiemtra[0]), not a joined list, the same
	// divergence already handled for Winmart/Emart.
	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
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
				invoiceNote = "KM Bó Kèm - Che Barcode" // xulydonhang.py:3044
			}

			rows = append(rows, excelwriter.Row{
				EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
				Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: storeInfo, CustomerCode: fujimartCustomerCode,
				Description: description, SKU: invoiceSku, Warehouse: warehouse, VATPercent: 8,
				RegionCode: region, StatCode: statCode, IsPromoItem: true, Qty: soluongkm,
				ProductName: invoiceInfo.Name, CaseCount: invoiceCase, LineWeightKg: invoiceWeight, UseZFormula: false,
				PromoContent: invoicePromo, PromoNote: invoiceNote,
			})
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
		FileName: filepath.Base(filePath), Page: pageLabel, System: "FujiMart", MaKhachHang: fujimartCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
	}, nil
}
