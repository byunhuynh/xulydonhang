package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"order-processor/internal/processing/bigc"
	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/pricing"
)

// bigcRegionInfo mirrors the kho/khuvuc/mien branching inline in
// write_to_dondathang_bigc (xulydonhang.py:4569-4580) — a genuine 3-way
// split (MB / MN_MT / MN_GC) that the shared regionInfo (processor_shared.go)
// does NOT correctly cover: regionInfo only distinguishes "MB" vs
// everything else, giving every non-MB code the same warehouse
// ("LA_TP") — correct for BigC's MN_GC_BIGCAC code by coincidence, but
// WRONG for MN_MT_BIGCAC (needs "LA_KHO2026", not "LA_TP"). Extending
// the shared regionInfo instead of adding this BigC-only function would
// have silently changed Satra's already-shipped "MN_MT_*" customer
// codes' resolved warehouse (Satra's codes also start with "MN_MT") —
// confirmed during planning, not touched.
//
// Python has no else/default branch here (xulydonhang.py:4569-4580) —
// would raise UnboundLocalError if ever reached with an unmatched code.
// ResolveCustomerCode only ever returns the 4 codes covered below, so
// the default case is unreachable in practice; it returns the MN_GC
// branch's values defensively rather than panicking, since panicking
// mid-file would abort every remaining store page's processing for no
// evidence-backed reason.
func bigcRegionInfo(customerCode string) (region, statCode, warehouse string) {
	switch {
	case strings.HasPrefix(customerCode, "MB"):
		return "MT_MB", "HN", "TP_HN_12"
	case strings.HasPrefix(customerCode, "MN_MT"):
		return "MT_MN", "LA", "LA_KHO2026"
	case strings.HasPrefix(customerCode, "MN_GC"):
		return "MT_MN", "LA", "LA_TP"
	default:
		return "MT_MN", "LA", "LA_TP"
	}
}

// bigcOrderNumber mirrors write_to_dondathang_bigc's order-number field
// (xulydonhang.py:4613): f'ĐĐH{vendor}{STT_donhang_str}' where vendor is
// the uppercased literal "BIGC" and STT_donhang_str is f"-{po_number}"
// (same shape as every other vendor's order-number formatter in this
// codebase).
func bigcOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHBIGC-%s", poNumber)
}

// storePageResult is the outcome of processing ONE store page: either a
// set of rows to append to the file's combined Excel write, or a failure
// reason — never both. Mirrors the per-page isolation this plan's Global
// Constraints section commits to (a store page's failure never aborts
// other store pages, unlike Python's real, unguarded behavior).
type storePageResult struct {
	rows     []excelwriter.Row
	weightKg float64
	saigia   int
	tongtien float64
	err      error
}

// processBigcDocument mirrors process_file's BigC branch
// (xulydonhang.py:9404-9536) plus write_to_dondathang_bigc
// (:4541-4897), for the WHOLE file at once rather than Python's
// per-page-call design — see this plan's top-level Architecture note for
// why: Go's processBigcDocument already holds every page's text in
// memory, so it can accumulate every SUCCESSFUL store's rows into ONE
// excelwriter.WriteOrderRows call with a correctly pre-computed
// aggregate weight, instead of replicating Python's "write once per
// page, then re-read the sheet and overwrite a cell on the last page"
// mechanism — same final outcome (one combined block, one header, one
// aggregate weight total), simpler mechanism enabled by the chosen
// architecture, not a behavior change.
func (p *RealProcessor) processBigcDocument(filePath string, pageTexts []string) ([]OrderRow, error) {
	poNumber, entryDate, cancelDate, ok := bigc.ParseOrderInfo(pageTexts[0])
	if !ok {
		return []OrderRow{{
			FileName: filepath.Base(filePath), Page: fmt.Sprintf("1/%d", len(pageTexts)), System: "BigC",
			Status: StatusFailed + " - không tách được số PO/ngày đặt hàng từ trang 0", StatusKind: StatusKindFailed,
		}}, nil
	}
	priceList := bigc.ExtractPriceList(pageTexts[0])
	customerCode, deliveryWarehouse := bigc.ResolveCustomerCode(pageTexts[0])
	region, statCode, warehouse := bigcRegionInfo(customerCode)
	orderNum := bigcOrderNumber(poNumber)
	description := fmt.Sprintf("BIGC PO%s", poNumber)

	priceIndex, err := p.Pricing.FetchIndex("BIGC")
	if err != nil {
		return []OrderRow{{
			FileName: filepath.Base(filePath), Page: fmt.Sprintf("1/%d", len(pageTexts)), System: "BigC",
			Status: fmt.Sprintf("%s - không tải được giá/khuyến mãi: %v", StatusFailed, err), StatusKind: StatusKindFailed,
		}}, nil
	}

	var allRows []excelwriter.Row
	var totalWeight float64
	var orderRows []OrderRow
	headerWritten := false

	for pageIdx := 1; pageIdx < len(pageTexts); pageIdx++ {
		pageLabel := fmt.Sprintf("%d/%d", pageIdx+1, len(pageTexts))
		result := p.processBigcStorePage(pageTexts[pageIdx], priceList, priceIndex, orderNum, entryDate, cancelDate,
			customerCode, deliveryWarehouse, description, warehouse, region, statCode, !headerWritten)

		if result.err != nil {
			orderRows = append(orderRows, OrderRow{
				FileName: filepath.Base(filePath), Page: pageLabel, System: "BigC",
				Status: fmt.Sprintf("%s - %v", StatusFailed, result.err), StatusKind: StatusKindFailed,
			})
			continue
		}

		if !headerWritten {
			headerWritten = true
		}
		allRows = append(allRows, result.rows...)
		totalWeight += result.weightKg

		statusKind := StatusKindDone
		statusText := StatusDone
		if result.saigia > 0 {
			statusKind = StatusKindWarning
			statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, result.saigia)
		}
		orderRows = append(orderRows, OrderRow{
			FileName: filepath.Base(filePath), Page: pageLabel, System: "BigC", MaKhachHang: customerCode,
			PO: poNumber, DonGia: fmt.Sprintf("%.0f", result.tongtien), Status: statusText, StatusKind: statusKind,
		})
	}

	if len(allRows) > 0 {
		headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
		if err := excelwriter.WriteOrderRows(p.ExcelPath, allRows, headerDescription); err != nil {
			return nil, err
		}
	}

	return orderRows, nil
}

// processBigcStorePage handles ONE store page: extracts its name and
// item list, joins prices from the page-0 master list, price/promo
// matches every item, and builds this store's rows. isFirstSuccessful
// controls whether a header/note row is prepended (mirrors Python's
// header-only-on-page_num==1 behavior — but here it's "only on the
// first SUCCESSFULLY processed store", since a failed page 1 must not
// prevent page 2 from getting the header).
func (p *RealProcessor) processBigcStorePage(storePageText string, priceList []bigc.Product, priceIndex *pricing.Index,
	orderNum, entryDate, cancelDate, customerCode, shipTo, description, warehouse, region, statCode string, isFirstSuccessful bool,
) storePageResult {
	storeName, ok := bigc.ExtractStoreName(storePageText)
	if !ok {
		return storePageResult{err: fmt.Errorf("không tách được tên store")}
	}
	rawItems := bigc.ExtractStoreItems(storePageText)
	if len(rawItems) == 0 {
		return storePageResult{err: fmt.Errorf("không trích xuất được sản phẩm nào cho store %q", storeName)}
	}
	items := bigc.JoinItemsWithPrices(rawItems, priceList)

	var rows []excelwriter.Row
	if isFirstSuccessful {
		rows = append(rows, excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
			Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
			StatCode: statCode, IsNoteRow: true, ProductName: description,
		})
	}

	var weightKg, tongtien float64
	saigia := 0

	for _, item := range items {
		barcode := p.Store.ResolveSku(item.Barcode)
		productInfo, _ := p.Store.GetProductInfo(barcode)

		skuOU := parseNumericField(item.SKUOrUnit)
		ouQty := parseNumericField(item.OrderedUnitQty)
		qtyOrdPcs := ouQty * skuOU // xulydonhang.py:4642 — item["OU Qty"] * item["SKU/OU"], NOT "OU Qty" alone

		lineWeight := productInfo.WeightKg * qtyOrdPcs
		weightKg += lineWeight

		invoicePrice := item.UnitPrice // giahoadon: the joined-in per-unit price from page 0
		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice := parseNumericField(realPriceStr) // giathuctegoc
		finalPrice := realPrice                      // giathucte, mutates through the promo loop below

		promos := priceIndex.FindPromotions(barcode, entryDate)
		khuyenmai := ""
		matched := false

		for _, promo := range promos {
			if promo.Value == "" {
				continue
			}
			khuyenmai = promo.Value
			if discount := coop.ExtractDiscount(promo.Value); discount != 0 {
				// xulydonhang.py:4685 recomputes from the ORIGINAL
				// fetched price (giathuctegoc), not from whatever
				// finalPrice already holds from a prior iteration —
				// but when discount == 0 for an iteration, Python's
				// "else: giathucte = giathucte" is a literal no-op, so
				// finalPrice deliberately carries over UNCHANGED from
				// the previous iteration in that case (not reset to
				// realPrice) — port this exact quirk, do not "fix" it
				// by resetting finalPrice every iteration.
				finalPrice = realPrice - (realPrice * discount / 100)
			}
			if closeEnough(invoicePrice, finalPrice) {
				matched = true
				break
			}
		}
		if len(promos) == 0 && closeEnough(invoicePrice, finalPrice) {
			matched = true
		}

		productRow := excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
			Description: description, SKU: barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: qtyOrdPcs, UnitPrice: finalPrice,
			ProductName: productInfo.Name, LineWeightKg: lineWeight, UseZFormula: true, PromoContent: khuyenmai,
		}
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
		}
		rows = append(rows, productRow)
		tongtien += finalPrice * qtyOrdPcs // xulydonhang.py:4749 — uses qtyOrdPcs BEFORE any promo-bonus division below

		// Promo bonus-row check (xulydonhang.py:4754-4808). BigC has NO
		// khuyenmai.split('|') loop (confirmed structurally different
		// from Coop/Satra during planning) — exactly one bonus row per
		// item, driven by the single khuyenmai string this item ended
		// up with.
		bonusSku := p.Store.FindSkusMentioned(khuyenmai)
		bonusQty := qtyOrdPcs
		bonusBarcode := ""
		if len(bonusSku) > 0 {
			bonusBarcode = strings.Join(bonusSku, ", ")
		}
		if xm := xPlus1Pattern.FindStringSubmatch(khuyenmai); xm != nil {
			// xPlus1Pattern is already in scope here — it's declared in
			// processor_shared.go (Task 0), same package `processing`
			// as this file, so no new import or helper is needed.
			x, _ := strconv.Atoi(xm[1])
			if bonusBarcode == "" {
				bonusBarcode = barcode
			}
			if x >= 2 {
				bonusQty = math.Floor(qtyOrdPcs / float64(x))
			}
		}
		if bonusBarcode != "" {
			bonusInfo, _ := p.Store.GetProductInfo(bonusBarcode)
			bonusWeight := bonusInfo.WeightKg * bonusQty
			weightKg += bonusWeight

			// xulydonhang.py:4769's laycachbo_khuyenmai(value) uses the
			// leftover "value" loop variable from the promo-matching
			// loop above, not "khuyenmai" — this plan's Global
			// Constraints section documents this as a confirmed Python
			// quirk NOT being ported; using khuyenmai (this item's own
			// resolved promo string) here instead, per Phase 2b's
			// "correct main flow" policy. Flag via knownDivergences_BigC
			// during Task 8 if a real fixture traces a mismatch to this.
			bundleNote := coop.ExtractBraceContent(khuyenmai)
			bonusRow := excelwriter.Row{
				EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
				Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
				Description: description, SKU: bonusBarcode, Warehouse: warehouse, VATPercent: 8,
				RegionCode: region, StatCode: statCode, IsPromoItem: true, Qty: bonusQty,
				ProductName: bonusInfo.Name, LineWeightKg: bonusWeight, UseZFormula: false,
			}
			if bundleNote != "" {
				bonusRow.PromoNote = bundleNote
			} else {
				// BigC's per-item no-brace fallback text is genuinely
				// different from Coop/Satra's "KM Bó Kèm - Che Barcode"
				// (xulydonhang.py:4777: "KM Rời - Không Che Barcode") —
				// confirmed during planning, not a transcription error.
				bonusRow.PromoNote = "KM Rời - Không Che Barcode"
			}
			rows = append(rows, bonusRow)
		}
	}

	return storePageResult{rows: rows, weightKg: weightKg, saigia: saigia, tongtien: tongtien}
}

// parseNumericField mirrors the repeated "strip commas, coerce to
// float/int" pattern applied to item["SKU/OU"] and item["OU Qty"]
// (xulydonhang.py:4632-4640) and to a fetched price string — returns 0
// on any parse failure rather than panicking, since a malformed numeric
// field should surface as a price mismatch / zero quantity downstream,
// not crash the whole store page.
func parseNumericField(s string) float64 {
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil {
		return 0
	}
	return v
}
