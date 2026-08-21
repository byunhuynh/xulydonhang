package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/jmart"
)

// jmartCustomerCode mirrors write_to_dondathang_kingfood's makhachhang
// value as passed from JMart's own process_file branch — the literal
// "MN_MT_JM0001", hardcoded at the call site (xulydonhang.py:8144).
// This is the EXACT value kingfoodRegionInfo's own "MN_MT_JM0001"
// branch (kingfood_processor.go) was written for.
const jmartCustomerCode = "MN_MT_JM0001"

// jmartOrderNumber mirrors write_to_dondathang_kingfood's order-number
// field (xulydonhang.py:3899) as applied to JMart's call:
// f'ĐĐH{vendor}{STT_donhang_str}' where vendor is the uppercased
// literal "JMART" and STT_donhang_str is f"-{po_number}".
func jmartOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHJMART-%s", poNumber)
}

// processJMartSegment mirrors the JMart branch of process_file
// (xulydonhang.py:8143-8209), which itself calls the SAME
// write_to_dondathang_kingfood function Kingfood's own branch calls
// (xulydonhang.py:8192) — there is no separate write_to_dondathang_jmart.
// This function therefore mirrors processKingfoodSegment's row-building
// shape closely, but deliberately calls the EXISTING, already-shipped
// kingfoodRegionInfo helper directly (see this file's own const above)
// rather than duplicating or modifying it. JMart is "1 page = 1 order",
// same family as Kingfood. A trailing PDF page that lacks JMart's
// identify marker falls through to the shared per-page dispatch loop's
// default case (coop_processor.go), which emits a Failed/"Thất bại"
// OrderRow for that page.
func (p *RealProcessor) processJMartSegment(filePath, text, pageLabel string) (OrderRow, error) {
	poNumber, entryDate, cancelDate, deliveryAddress, ok := jmart.ParseOrderInfo(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được số PO/ngày đặt hàng/địa chỉ giao hàng")
	}

	products := jmart.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	priceIndex, err := p.Pricing.FetchIndex("JMART")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := kingfoodRegionInfo(jmartCustomerCode)
	orderNum := jmartOrderNumber(poNumber)
	description := fmt.Sprintf("JMART %s", poNumber)

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: jmartCustomerCode,
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
		invoicePrice := parseNumericField(rawProduct.TotalPrice)

		lineWeight := productInfo.WeightKg * ouQty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(ouQty / productInfo.PackSize))
		}
		totalWeight += lineWeight

		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice := parseNumericField(realPriceStr)

		promos := priceIndex.FindPromotions(barcode, entryDate)
		khuyenmai := ""
		matched := false
		finalPrice := realPrice

		for _, promo := range promos {
			// CR normalization (openpyxl-vs-excelize \r round-trip
			// divergence, same class of fix already shipped for BigC/
			// Emart/FujiMart/Kingfood) — applied here from the FIRST
			// commit, not added later as a fix-round patch.
			value := strings.ReplaceAll(promo.Value, "\r", "\n")
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
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: jmartCustomerCode,
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

		// Per-item promo bonus row — single attempt, buildPromoBonusRow
		// always called with index=0, matching Kingfood's exact shape
		// (both are produced by the same real Python function).
		bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, khuyenmai,
			coop.Product{Barcode: barcode, Qty: ouQty}, 0, entryDate, cancelDate, deliveryAddress,
			jmartCustomerCode, description, warehouse, region, statCode, orderNum)
		if added {
			totalWeight += bonusRow.LineWeightKg

			// No-{...}-brace fallback text ("KM Giao Rời - Không Che
			// Barcode") does NOT write AP — matching Kingfood's own
			// fix (write_to_dondathang_kingfood:4092-4096, only the
			// cachbokem branch writes AP; the else/fallback branch
			// never does), since this is the SAME real Python function.
			if coop.ExtractBraceContent(khuyenmai) == "" {
				mainRowNote = "KM Giao Rời - Không Che Barcode"
				mainRowBundleSku = ""
				bonusRow.PromoBundleSku = ""
			}

			rows[productRowIndex].PromoNote = mainRowNote
			if mainRowBundleSku != "" {
				rows[productRowIndex].PromoBundleSku = mainRowBundleSku
			}
			rows = append(rows, bonusRow)
		}
	}

	// Invoice-level ("Hóa Đơn") promo bonus row. Does NOT reuse the
	// shared buildInvoiceBonusRow — Q gets only the first matched SKU
	// (kiemtra[0]), not a joined list, matching Kingfood's exact shape.
	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
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
				invoiceNote = "KM Bó Kèm - Che Barcode"
			}

			rows = append(rows, excelwriter.Row{
				EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
				Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: jmartCustomerCode,
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
		FileName: filepath.Base(filePath), Page: pageLabel, System: "JMart", MaKhachHang: jmartCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
	}, nil
}
