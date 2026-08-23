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
func (p *RealProcessor) processJMartSegment(filePath string, realPageNum int, text, pageLabel string) (OrderRow, error) {
	poNumber, entryDate, cancelDate, deliveryAddress, ok := jmart.ParseOrderInfo(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được số PO/ngày đặt hàng/địa chỉ giao hàng")
	}

	products := jmart.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}
	// An empty OUQty or TotalPrice means the backward-scan anchor (see
	// qcAnchorValue/pricePattern in jmart/extract.go) never found its
	// target for this product. parseNumericField("") silently returns 0
	// with no error, which would otherwise write a normal-looking
	// "Hoàn thành" row with a wrong (zero) quantity or price — the one
	// failure mode this single-sample vendor's algorithm cannot rule
	// out, given it's confirmed correct for exactly one real PDF. Fail
	// the page loudly instead, mirroring Python's own real behavior here
	// (xulydonhang.py:3924's `float(None)` raises TypeError on a missing
	// value rather than silently proceeding with a wrong number).
	for _, rawProduct := range products {
		if rawProduct.OUQty == "" || rawProduct.TotalPrice == "" {
			return OrderRow{}, fmt.Errorf("không trích xuất được số lượng/đơn giá cho barcode %s", rawProduct.Barcode)
		}
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
	totalPackages := 0
	totalValue := 0.0
	promoTotals := map[string]*PromoItemSummary{}
	var skuLog []string
	var mismatchDetails []PriceMismatchDetail

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
		totalPackages += caseCount

		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice := parseNumericField(realPriceStr)

		promos := priceIndex.FindPromotions(barcode, entryDate)
		khuyenmai := ""
		khuyenmaiColumn := ""
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
			khuyenmaiColumn = promo.Column
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
		skuLog = append(skuLog, formatSkuLogLine(barcode, productInfo.Name, matched, invoicePrice, finalPrice, khuyenmai, khuyenmaiColumn))

		productRow := excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: jmartCustomerCode,
			Description: description, SKU: barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: ouQty, UnitPrice: finalPrice,
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
				InvoicePrice: invoicePrice, SystemPrice: finalPrice, Qty: ouQty,
				ExcelRow: productRowIndex, PromoText: truncatePromoText(khuyenmai),
				PromoDateRange: khuyenmaiColumn,
			})
		}
		rows = append(rows, productRow)
		totalValue += finalPrice * ouQty

		// Per-item promo bonus row — single attempt, buildPromoBonusRow
		// always called with index=0, matching Kingfood's exact shape
		// (both are produced by the same real Python function).
		//
		// Gated on matched: a gift is part of the SAME CTKM that
		// explains the invoice price — only build it once that CTKM
		// is confirmed, never from khuyenmai's last-examined-but-
		// unconfirmed value on a genuine price mismatch. Deliberate
		// divergence from Python (this block runs unconditionally
		// there).
		if matched {
			bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, khuyenmai,
				coop.Product{Barcode: barcode, Qty: ouQty}, 0, entryDate, cancelDate, deliveryAddress,
				jmartCustomerCode, description, warehouse, region, statCode, orderNum)
			if added {
				totalWeight += bonusRow.LineWeightKg
				totalPackages += bonusRow.CaseCount
				accumulatePromoItem(promoTotals, bonusRow.SKU, bonusRow.ProductName, bonusRow.Qty)

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
	}

	// Invoice-level ("Hóa Đơn") promo bonus row. Does NOT reuse the
	// shared buildInvoiceBonusRow — Q gets only the first matched SKU
	// (kiemtra[0]), not a joined list, matching Kingfood's exact shape.
	//
	// KNOWN PYTHON DIVERGENCE (documented, not fixed) — same one already
	// noted in processKingfoodSegment (kingfood_processor.go), since
	// both go through the literal same Python line: real Python's
	// xulydonhang.py:4131 calls find_all_promotions_by_sku_and_time("Hóa
	// Đơn", entry_date) WITHOUT a vendor argument, defaulting to the
	// COOP sheet rather than JMART's own. priceIndex here was fetched
	// via p.Pricing.FetchIndex("JMART"), so this port deliberately reads
	// the correct JMART sheet instead. Currently latent: no real "Hóa
	// Đơn" promo rows exist in the one available real sample.
	//
	// This block and processKingfoodSegment's equivalent block are
	// near-duplicates (JMart and Kingfood share one real Python
	// function) — keep any future fix to one in sync with the other.
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
			totalPackages += invoiceCase
			accumulatePromoItem(promoTotals, invoiceSku, invoiceInfo.Name, soluongkm)

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
		Vendor:       "JMART",
		EntryDate:    entryDate,
		CustomerCode: jmartCustomerCode,
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
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "JMart", MaKhachHang: jmartCustomerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		DriveURL: driveURL,
		ShipTo: deliveryAddress, EntryDate: entryDate, CancelDate: cancelDate,
		TotalWeightKg: totalWeightFormatted, TotalPackages: totalPackages,
		PromoItems: finalizePromoItems(promoTotals),
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
}
