package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/kingfood"
)

// kingfoodCustomerCode mirrors write_to_dondathang_kingfood's default/
// only makhachhang value — the literal "MN_MT_KFMSL", hardcoded at the
// process_file call site (xulydonhang.py:9273). Kingfood never derives a
// customer code from the PDF.
const kingfoodCustomerCode = "MN_MT_KFMSL"

// kingfoodDeliveryAddress mirrors write_to_dondathang_kingfood's default/
// only delivery value — hardcoded at the process_file call site
// (xulydonhang.py:9274), never extracted from the PDF even though the
// same string DOES appear in the PDF's own "Địa chỉ giao hàng:" field
// (confirmed during planning) — Python simply doesn't read it from
// there.
const kingfoodDeliveryAddress = "Số 324, đường ĐT743A, Phường Đông Hoà, Thành phố Hồ Chí Minh"

// kingfoodRegionInfo mirrors write_to_dondathang_kingfood's warehouse/
// region branching (xulydonhang.py:3871-3883) — a genuine 3-way split,
// the first in this project (FujiMart/Winmart only ever had 2). The MB
// and "MN_MT_JM0001" branches are unreachable with real Kingfood input
// today — customerCode is always the hardcoded constant
// kingfoodCustomerCode, which is neither "MB"-prefixed nor exactly
// "MN_MT_JM0001" — but this is modeled as a full 3-branch function
// anyway, matching the fujimartRegionInfo/winmartRegionInfo precedent,
// for architectural consistency.
func kingfoodRegionInfo(customerCode string) (region, statCode, warehouse string) {
	switch {
	case strings.HasPrefix(customerCode, "MB"):
		return "MT_MB", "HN", "TP_HN_12"
	case customerCode == "MN_MT_JM0001":
		return "MT_MN", "LA", "LA_TP"
	default:
		return "MT_MN", "LA", "LA_KHO2026"
	}
}

// kingfoodOrderNumber mirrors write_to_dondathang_kingfood's order-
// number field (xulydonhang.py:3899): f'ĐĐH{vendor}{STT_donhang_str}'
// where vendor is the uppercased literal "KINGFOOD" and
// STT_donhang_str is f"-{po_number}".
func kingfoodOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHKINGFOOD-%s", poNumber)
}

// parseKingfoodPrice mirrors laydanhsachsanpham_kingfood's price
// parsing (xulydonhang.py:6744): price_str.replace('.', '').replace(',', '.')
// — Vietnamese/European number format (period = thousands separator,
// comma = decimal separator), the OPPOSITE convention from every other
// vendor in this project (which only ever strip commas, US-style
// thousands, with no decimal-comma). NOT a drop-in replacement for the
// shared parseNumericField helper — scoped to Kingfood's price field
// only; Kingfood's quantity field uses parseNumericField as usual (only
// strips periods, matching Python's quantity.replace('.', '')).
func parseKingfoodPrice(s string) float64 {
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// processKingfoodSegment mirrors the Kingfood branch of process_file
// (xulydonhang.py:9230-9310) plus write_to_dondathang_kingfood
// (:3848-4196). Kingfood is "1 page = 1 order", the same family as
// Coop/Lotte/Satra/Winmart/Emart/FujiMart. A trailing PDF page that
// lacks Kingfood's identify marker falls through to the shared per-page
// dispatch loop's default case (coop_processor.go), which emits a
// Failed/"Thất bại" OrderRow for that page.
func (p *RealProcessor) processKingfoodSegment(filePath, text, pageLabel string) (OrderRow, error) {
	poNumber, entryDate, cancelDate, ok := kingfood.ParseOrderInfo(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được số PO/ngày đặt hàng")
	}

	products := kingfood.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	priceIndex, err := p.Pricing.FetchIndex("KINGFOOD")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := kingfoodRegionInfo(kingfoodCustomerCode)
	orderNum := kingfoodOrderNumber(poNumber)
	description := fmt.Sprintf("KINGFOOD %s", poNumber)

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: kingfoodDeliveryAddress, CustomerCode: kingfoodCustomerCode,
		Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: description,
	})

	saigia := 0
	totalWeight := 0.0
	totalValue := 0.0
	var skuLog []string

	for _, rawProduct := range products {
		barcode := p.Store.ResolveSku(rawProduct.Barcode)
		productInfo, _ := p.Store.GetProductInfo(barcode)
		ouQty := parseNumericField(rawProduct.OUQty)

		// invoicePrice (giahoadon = dongia, xulydonhang.py:3925,3970): the
		// "Total Price" field from ExtractProducts is actually a PER-UNIT
		// final price (post-discount) for Kingfood, NOT a line total —
		// see kingfood.Product's own doc comment. Used DIRECTLY, no
		// division by ouQty (unlike FujiMart/Winmart, whose TotalPrice IS
		// a line total).
		invoicePrice := parseKingfoodPrice(rawProduct.TotalPrice)

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
			// Emart/FujiMart) — applied here from the FIRST commit, not
			// added later as a fix-round patch (FujiMart's final review
			// caught exactly this omission on its own invoice-level
			// block; Kingfood gets both call sites right from the start).
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
		skuLog = append(skuLog, formatSkuLogLine(barcode, productInfo.Name, matched, invoicePrice, finalPrice, khuyenmai))

		productRow := excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: kingfoodDeliveryAddress, CustomerCode: kingfoodCustomerCode,
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

		// Per-item promo bonus row (xulydonhang.py:4074-4128) — single
		// attempt, buildPromoBonusRow always called with index=0 (no
		// "|"-split multi-CTKM loop).
		bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, khuyenmai,
			coop.Product{Barcode: barcode, Qty: ouQty}, 0, entryDate, cancelDate, kingfoodDeliveryAddress,
			kingfoodCustomerCode, description, warehouse, region, statCode, orderNum)
		if added {
			totalWeight += bonusRow.LineWeightKg

			// Kingfood's own no-{...}-brace fallback text
			// (xulydonhang.py:4096, "KM Giao Rời - Không Che Barcode")
			// differs from buildPromoBonusRow's shared Coop-flavored
			// default ("KM Bó Kèm - Che Barcode"). Unlike FujiMart's
			// equivalent fallback (which still writes AP because its own
			// text contains "bó kèm"), Kingfood's fallback text does NOT
			// contain "bó kèm"/"quấn kèm", so this ALSO needs to
			// explicitly clear AP — matching Winmart's/Emart's identical
			// fix, confirmed against xulydonhang.py:4092-4096 (only the
			// cachbokem branch writes AP; the else/fallback branch never
			// does).
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

	// Invoice-level ("Hóa Đơn") promo bonus row (xulydonhang.py:4131-4177).
	// Does NOT reuse the shared buildInvoiceBonusRow — Q gets only the
	// first matched SKU (kiemtra[0]), not a joined list, the same
	// divergence already handled for Winmart/Emart/FujiMart.
	//
	// KNOWN PYTHON DIVERGENCE (documented, not fixed): real Python's
	// xulydonhang.py:4131 calls find_all_promotions_by_sku_and_time("Hóa
	// Đơn", entry_date) WITHOUT the vendor argument — the only one of the
	// 10 equivalent call sites in the whole file to omit it. Because the
	// function's signature defaults sheet_name to "Coop", real Python
	// therefore actually reads Kingfood's invoice-level promo from the
	// COOP sheet, not the KINGFOOD sheet — almost certainly an
	// unintentional bug (every other vendor's own call site correctly
	// passes its vendor). priceIndex here was fetched via
	// p.Pricing.FetchIndex("KINGFOOD"), so this port deliberately reads
	// from the correct KINGFOOD sheet instead, per this project's policy
	// of not preserving old Python bugs. Currently latent: no real
	// "Hóa Đơn" promo rows exist in either sheet's captured data.
	//
	// jmart_processor.go's processJMartSegment has a near-duplicate of
	// this whole block (same real Python function, same divergence) —
	// keep any future fix to one in sync with the other.
	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		invoicePromo = strings.ReplaceAll(invoicePromo, "\r", "\n")
		invoiceSkus := p.Store.FindSkusMentioned(invoicePromo)
		if amount, ok := coop.ExtractMoneyAmount(invoicePromo); ok && amount > 0 && len(invoiceSkus) > 0 {
			invoiceSku := invoiceSkus[0] // xulydonhang.py:4147 — kiemtra[0], not a joined list
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
				invoiceNote = "KM Bó Kèm - Che Barcode" // xulydonhang.py:4171
			}

			rows = append(rows, excelwriter.Row{
				EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
				Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: kingfoodDeliveryAddress, CustomerCode: kingfoodCustomerCode,
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
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Kingfood", MaKhachHang: kingfoodCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		SkuLog: skuLog,
	}, nil
}
