package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/lotte"
)

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
		// Applied here, before any row is built — earlier and more
		// defensively than Python, which never actually reaches a
		// placeholder substitution on this input; it crashes first
		// (xulydonhang.py:1992). Deliberate divergence, not a parity
		// gap — see GetCustomerCodeBySuffix's doc comment in
		// productdata/store.go for the full reasoning.
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
	var skuLog []string

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
		skuLog = append(skuLog, formatSkuLogLine(barcode, productInfo.Name, matched, invoicePrice, finalPrice, lastExaminedPromo))

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

		// Unlike Coop's write_to_dondathang (xulydonhang.py:1174,
		// "nhieuCtkm = khuyenmai.split('|')" followed by an
		// enumerate-loop), Lotte's write_to_dondathang_lotte
		// (xulydonhang.py:2196-2243, the single un-looped "if kiemtra:"
		// block) never splits the matched promo string on "|" — the
		// whole string is examined as one unit and at most one bonus
		// row is ever added per product. Some real promo values contain
		// a literal "|" as part of their own text (e.g.
		// "giảm 35% | 2+1 Nước lau sàn 1L TP30596 {KM Giao rời - Che
		// Barcode}" in đơn hàng/08-2026/260727-01013-00057.pdf's
		// matched CTKM cell). Splitting on "|" here (mirroring Coop's
		// pattern, as an earlier version of this function did)
		// misinterpreted that literal "|" as a multi-promo delimiter:
		// it wrote AO onto the bonus row (index i=1, "i>0" branch)
		// instead of the main product row, and evaluated the pre-"|"
		// fragment ("giảm 35% ") as a phantom promo segment on its own.
		// Passing the whole string through once, always as index 0 (so
		// buildPromoBonusRow's mainRowNote/mainRowBundleSku branch
		// applies), matches Python exactly: laycachbo_khuyenmai's `{...}`
		// extraction and the "X+1" regex both search the whole string
		// regardless of where "|" appears in it.
		bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, lastExaminedPromo,
			coop.Product{Barcode: barcode, Qty: qty}, 0, info.EntryDate, cancelDate, shipTo,
			customerCode, description, warehouse, region, statCode, lotteOrderNumber(info.PONumber))
		if added {
			totalWeight += bonusRow.LineWeightKg

			// buildPromoBonusRow's no-brace fallback ("KM Bó Kèm - Che
			// Barcode", which also flips isBundle true and writes AP)
			// is Coop's own default (xulydonhang.py:1198's "... or 'KM
			// Bó Kèm - Che Barcode'") and must stay unchanged there —
			// Coop's AP-writing behavior in that branch is verified,
			// already-shipped behavior from an earlier phase. Lotte's
			// write_to_dondathang_lotte has a different no-brace
			// branch (xulydonhang.py:2204-2217's "else:
			// sheet[f'AO{current_row}'] = 'KM Giao Rời - Không Che
			// Barcode'") that never writes AP at all in this case.
			// Override the shared helper's Coop-flavored result here,
			// scoped to Lotte only, rather than changing
			// buildPromoBonusRow itself.
			if coop.ExtractBraceContent(lastExaminedPromo) == "" {
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

	if invoicePromo := priceIndex.FindInvoicePromotion(info.EntryDate); invoicePromo != "" {
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, info.EntryDate, cancelDate,
			shipTo, customerCode, description, warehouse, region, statCode, lotteOrderNumber(info.PONumber)); added {
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
		SkuLog: skuLog,
	}, nil
}
