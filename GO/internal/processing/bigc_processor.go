package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"order-processor/internal/driveupload"
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
	rows            []excelwriter.Row
	weightKg        float64
	saigia          int
	tongtien        float64
	skuLog          []string
	mismatchDetails []PriceMismatchDetail
	promoItems      []PromoItemSummary
	err             error
}

type pendingBigcStore struct {
	resultIndex int
	excelStart  int
	excelCount  int
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
func (p *RealProcessor) processBigcDocument(filePath string, pageTexts []string, emit func(OrderRow)) ([]OrderRow, error) {
	poNumber, entryDate, cancelDate, ok := bigc.ParseOrderInfo(pageTexts[0])
	if !ok {
		return []OrderRow{emitOrderRow(emit, OrderRow{
			FileName: filepath.Base(filePath), Page: fmt.Sprintf("1/%d", len(pageTexts)), System: "BigC",
			Status: StatusFailed + " - không tách được số PO/ngày đặt hàng từ trang 0", StatusKind: StatusKindFailed,
		})}, nil
	}
	priceList := bigc.ExtractPriceList(pageTexts[0])
	customerCode, deliveryWarehouse := bigc.ResolveCustomerCode(pageTexts[0])
	region, statCode, warehouse := bigcRegionInfo(customerCode)
	orderNum := bigcOrderNumber(poNumber)
	description := fmt.Sprintf("BIGC PO%s", poNumber)

	priceIndex, err := p.Pricing.FetchIndex("BIGC")
	if err != nil {
		return []OrderRow{emitOrderRow(emit, OrderRow{
			FileName: filepath.Base(filePath), Page: fmt.Sprintf("1/%d", len(pageTexts)), System: "BigC",
			Status: fmt.Sprintf("%s - không tải được giá/khuyến mãi: %v", StatusFailed, err), StatusKind: StatusKindFailed,
		})}, nil
	}

	var allRows []excelwriter.Row
	var totalWeight float64
	var orderRows []OrderRow
	var pending []pendingBigcStore
	headerWritten := false

	for pageIdx := 1; pageIdx < len(pageTexts); pageIdx++ {
		pageLabel := fmt.Sprintf("%d/%d", pageIdx+1, len(pageTexts))
		result := p.processBigcStorePage(pageTexts[pageIdx], priceList, priceIndex, orderNum, entryDate, cancelDate,
			customerCode, deliveryWarehouse, description, warehouse, region, statCode, !headerWritten)

		if result.err != nil {
			orderRows = append(orderRows, emitOrderRow(emit, OrderRow{
				FileName: filepath.Base(filePath), Page: pageLabel, System: "BigC",
				Status: fmt.Sprintf("%s - %v", StatusFailed, result.err), StatusKind: StatusKindFailed,
			}))
			continue
		}

		if !headerWritten {
			headerWritten = true
		}

		// Adjust each detail's ExcelRow from "index within THIS store's
		// own local rows" to "index within the combined allRows slice"
		// by adding how many rows every earlier successful store already
		// contributed — snapshotted as len(allRows) BEFORE this store's
		// own rows are appended below. The final absolute Excel row
		// (adding startRow, common to every store in this one combined
		// write) is applied once, after WriteOrderRows returns, below.
		for i := range result.mismatchDetails {
			result.mismatchDetails[i].ExcelRow += len(allRows)
		}

		excelStart := len(allRows)
		allRows = append(allRows, result.rows...)
		totalWeight += result.weightKg

		statusKind := StatusKindDone
		statusText := StatusDone
		if result.saigia > 0 {
			statusKind = StatusKindWarning
			statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, result.saigia)
		}
		finalRow := OrderRow{
			FileName: filepath.Base(filePath), Page: pageLabel, System: "BigC", MaKhachHang: customerCode,
			PO: poNumber, DonGia: fmt.Sprintf("%.0f", result.tongtien), Status: statusText, StatusKind: statusKind,
			// ShipTo is this store's own delivery warehouse, not a
			// document-wide value - BigC's one PDF holds multiple stores'
			// pages, each its own OrderRow. TotalPackages stays 0: BigC
			// never tracks case count at all (NoCaseCount: true on every
			// row this vendor writes), so 0 here is accurate, not a gap.
			ShipTo: deliveryWarehouse, EntryDate: entryDate, CancelDate: cancelDate,
			TotalWeightKg: coop.FormatWeightKg(result.weightKg),
			PromoItems:    result.promoItems,
			SkuLog:        result.skuLog, PriceMismatchCount: result.saigia, PriceMismatchDetails: result.mismatchDetails,
		}
		provisional := finalRow
		provisional.Status = StatusProcessing
		provisional.StatusKind = StatusKindProcessing
		provisional = emitOrderRow(emit, provisional)
		finalRow.ResultKey = provisional.ResultKey
		orderRows = append(orderRows, finalRow)
		pending = append(pending, pendingBigcStore{
			resultIndex: len(orderRows) - 1,
			excelStart:  excelStart,
			excelCount:  len(result.rows),
		})
	}

	if len(allRows) > 0 {
		headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
		startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, allRows, headerDescription)
		if err != nil {
			for _, store := range pending {
				failed := orderRows[store.resultIndex]
				failed.Status = fmt.Sprintf("%s - lỗi ghi dondathang.xlsx: %v", StatusFailed, err)
				failed.StatusKind = StatusKindFailed
				failed.ExcelRows = nil
				orderRows[store.resultIndex] = emitOrderRow(emit, failed)
			}
			return orderRows, nil
		}
		for i := range orderRows {
			for j := range orderRows[i].PriceMismatchDetails {
				orderRows[i].PriceMismatchDetails[j].ExcelRow += startRow
			}
		}

		driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
			Vendor:       "BIGC",
			EntryDate:    entryDate,
			CustomerCode: customerCode,
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
		for i := range orderRows {
			orderRows[i].DriveURL = driveURL
		}
		for _, store := range pending {
			finalRow := orderRows[store.resultIndex]
			finalRow.ExcelRows = make([]int, store.excelCount)
			for i := range finalRow.ExcelRows {
				finalRow.ExcelRows[i] = startRow + store.excelStart + i
			}
			orderRows[store.resultIndex] = emitOrderRow(emit, finalRow)
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
	// The extracted store name does NOT feed ShipTo/Description — those
	// come from a different, document-level source (customerCode/
	// description, resolved once in processBigcDocument from page 0 and
	// threaded through as parameters). It DOES feed column AN: Python's
	// write_to_dondathang_bigc receives this exact per-page name as its
	// `congtrinh` parameter (xulydonhang.py:9552/9560: `tenstore =
	// lay_ten_store(text)` then passed straight in) and looks it up via
	// tim_gia_tri_congtrinh (see productdata.Store.GetSiteValue) to get
	// the AN value written on every product/promo-bonus row below.
	storeName, ok := bigc.ExtractStoreName(storePageText)
	if !ok {
		return storePageResult{err: fmt.Errorf("không tách được tên store")}
	}
	siteCode := p.Store.GetSiteValue(storeName)
	// A page with ZERO extracted item lines is a legitimate, successful
	// "store" page in Python, not an error: write_to_dondathang_bigc
	// (xulydonhang.py:4605) just does `for item in items:` over
	// (possibly empty) items — an empty list means the loop body never
	// runs, no exception, no error return. Real PDFs have pages like
	// this: BigC's page-0 "PURCHASE NOTE" master list can overflow onto
	// a second physical page (PDF page index 1) that repeats the
	// header/store-info block but has no item table at all — Python's
	// caller (xulydonhang.py:9469) still calls write_to_dondathang_bigc
	// for every page from index 1 to len(doc)-2 unconditionally, and
	// since that call's `page_num == 1` (xulydonhang.py:4583), the
	// order's header/note row (this port's isFirstSuccessful row) is
	// STILL attached there. Confirmed via 2631057774693.pdf and
	// 2632057938612.pdf (real fixtures whose PDF page index 1 is exactly
	// this kind of blank continuation page) — treating this as a hard
	// error (an earlier version of this port did) produces a spurious
	// Failed OrderRow Python never reports for these files.
	//
	// Note this removes a safety net: with the hard-fail gone, a future
	// regression in bigc.ExtractStoreItems's regexes (one that starts
	// wrongly matching zero lines on a page that legitimately has items)
	// would now silently produce a "Hoàn Thành" success status with
	// missing rows, instead of a loud Failed row. The golden fixture
	// test (bigc_golden_test.go) is the only thing that would catch
	// such a regression, since it diffs actual row content/counts
	// against frozen Python output — not just status/error presence.
	rawItems := bigc.ExtractStoreItems(storePageText)
	items := bigc.JoinItemsWithPrices(rawItems, priceList)

	var rows []excelwriter.Row
	if isFirstSuccessful {
		rows = append(rows, excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
			Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
			StatCode: statCode, IsNoteRow: true, ProductName: description,
			// "BIGCANLAC" is a fixed literal on the header row regardless
			// of which store this page belongs to (xulydonhang.py:4669) -
			// NOT siteCode, which is per-store and only applies to the
			// product/promo-bonus rows below.
			SiteCode: "BIGCANLAC",
		})
	}

	var weightKg, tongtien float64
	saigia := 0
	promoTotals := map[string]*PromoItemSummary{}
	var skuLog []string
	var mismatchDetails []PriceMismatchDetail

	for _, item := range items {
		barcode := p.Store.ResolveSku(item.Barcode)
		// xulydonhang.py:4606-4607: `if timten_sanpham(item["Barcode"]) ==
		// "Không thấy tên sản phẩm": continue` — skip the item entirely (no
		// row, no weight/tongtien/saigia contribution) when its resolved
		// barcode doesn't match a known product. Checked against
		// item["Barcode"] AFTER Python's earlier replace_sku_numbers pass
		// (xulydonhang.py:4564), i.e. the already-resolved barcode —
		// mirrored here by checking `found` right after ResolveSku, not
		// against the raw item.Barcode. This exact "not found -> continue"
		// check exists at only one other place in the whole Python file —
		// genuinely BigC-specific, no Coop/Lotte/Satra counterpart.
		productInfo, found := p.Store.GetProductInfo(barcode)
		if !found {
			continue
		}

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
		khuyenmaiColumn := ""
		matched := false

		for _, promo := range promos {
			if promo.Value == "" {
				continue
			}
			khuyenmaiColumn = promo.Column
			// Normalize literal CR to LF here, matching Python's actual
			// EFFECTIVE output for this column rather than its raw fetch.
			// Root cause (confirmed by hand-tracing the xlsx XML): openpyxl
			// silently normalizes '\r' -> '\n' (and '\r\n' -> '\n\n') on
			// ANY write/read round-trip through an .xlsx file — Go's
			// excelize instead escapes '\r' as "&#xD;", which survives
			// untouched. This is a genuine behavioral difference between
			// the two libraries' CR-handling, not merely a quirk of how
			// Task 7's fixture-generation harness captures golden fixtures:
			// the SAME openpyxl round-trip happens in the real production
			// app too, so production Python's real dondathang.xlsx output
			// ALSO silently mangles bare '\r' to '\n'. Normalizing here
			// therefore makes Go match Python's actual effective behavior
			// in BOTH the test-fixture-capture path AND real production
			// output, not just the test. Scoped to BigC only: applying it
			// here (not in the shared excelwriter write site) avoids
			// touching Coop/Lotte/Satra, whose golden tests already pass
			// without this normalization — moving it to the shared layer is
			// a deliberately deferred, larger architectural change.
			khuyenmai = strings.ReplaceAll(promo.Value, "\r", "\n")
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
		skuLog = append(skuLog, formatSkuLogLine(barcode, productInfo.Name, matched, invoicePrice, finalPrice, khuyenmai, khuyenmaiColumn))

		// Promo bonus-row check (xulydonhang.py:4754-4808). BigC has NO
		// khuyenmai.split('|') loop (confirmed structurally different
		// from Coop/Satra during planning) — exactly one bonus row per
		// item, driven by the single khuyenmai string this item ended
		// up with. This is computed BEFORE productRow is built below
		// because Python writes AO/AP (xulydonhang.py:4771/4777) on the
		// PRODUCT row (current_row, before current_row is incremented
		// for the bonus row) — NOT on the bonus row itself, despite the
		// bonus-row fields being written physically later in the
		// function. Putting PromoNote/PromoBundleSku on the bonus row
		// (an earlier version of this port did) produces an off-by-one
		// row shift versus every real fixture.
		bonusQty := qtyOrdPcs
		bonusBarcode := ""
		var promoNote, promoBundleSku string
		// Gated on matched: a gift is part of the SAME CTKM that
		// explains the invoice price — only compute/attach a gift
		// (and its PromoNote/PromoBundleSku on the product row
		// below) once this item's own khuyenmai is confirmed as
		// the one matching the invoice price, never on a genuine
		// price mismatch. Deliberate divergence from Python (this
		// whole block runs unconditionally there).
		if matched {
			bonusSku := p.Store.FindSkusMentioned(khuyenmai)
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
				// xulydonhang.py:4769's laycachbo_khuyenmai(value) uses the
				// leftover "value" loop variable from the promo-matching
				// loop above, not "khuyenmai" — this plan's Global
				// Constraints section documents this as a confirmed Python
				// quirk NOT being ported; using khuyenmai (this item's own
				// resolved promo string) here instead, per Phase 2b's
				// "correct main flow" policy. Flag via knownDivergences_BigC
				// during Task 8 if a real fixture traces a mismatch to this.
				bundleNote := coop.ExtractBraceContent(khuyenmai)
				if bundleNote != "" {
					promoNote = bundleNote
					lower := strings.ToLower(bundleNote)
					if strings.Contains(lower, "bó kèm") || strings.Contains(lower, "quấn kèm") {
						// xulydonhang.py:4774-4775 — same AP value written on
						// BOTH the product row and the bonus row. Not
						// observed in any of the 29 real fixtures (cachbokem
						// is always empty for BigC's actual promo data), but
						// ported for fidelity in case it's ever hit.
						promoBundleSku = fmt.Sprintf("%s_%s_1", coop.LastFourDigits(barcode), coop.LastFourDigits(bonusBarcode))
					}
				} else {
					// BigC's per-item no-brace fallback text is genuinely
					// different from Coop/Satra's "KM Bó Kèm - Che Barcode"
					// (xulydonhang.py:4777: "KM Rời - Không Che Barcode") —
					// confirmed during planning, not a transcription error.
					promoNote = "KM Rời - Không Che Barcode"
				}
			}
		}

		productRow := excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
			Description: description, SKU: barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: qtyOrdPcs, UnitPrice: finalPrice,
			ProductName: productInfo.Name, LineWeightKg: lineWeight, UseZFormula: true, PromoContent: khuyenmai,
			PromoNote: promoNote, PromoBundleSku: promoBundleSku, NoCaseCount: true, SiteCode: siteCode,
		}
		productRowIndex := len(rows)
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
			mismatchDetails = append(mismatchDetails, PriceMismatchDetail{
				SKU: barcode, ProductName: productInfo.Name,
				InvoicePrice: invoicePrice, SystemPrice: finalPrice, Qty: qtyOrdPcs,
				ExcelRow: productRowIndex, PromoText: truncatePromoText(khuyenmai),
				PromoDateRange: khuyenmaiColumn,
			})
		}
		rows = append(rows, productRow)
		tongtien += finalPrice * qtyOrdPcs // xulydonhang.py:4749 — uses qtyOrdPcs BEFORE any promo-bonus division below

		if bonusBarcode != "" {
			bonusInfo, _ := p.Store.GetProductInfo(bonusBarcode)
			bonusWeight := bonusInfo.WeightKg * bonusQty
			weightKg += bonusWeight

			bonusRow := excelwriter.Row{
				EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
				Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
				Description: description, SKU: bonusBarcode, Warehouse: warehouse, VATPercent: 8,
				RegionCode: region, StatCode: statCode, IsPromoItem: true, Qty: bonusQty,
				ProductName: bonusInfo.Name, LineWeightKg: bonusWeight, UseZFormula: false,
				PromoBundleSku: promoBundleSku, NoCaseCount: true, SiteCode: siteCode,
			}
			accumulatePromoItem(promoTotals, bonusRow.SKU, bonusRow.ProductName, bonusRow.Qty)
			rows = append(rows, bonusRow)
		}
	}

	// Invoice-level ("Hóa Đơn") promo bonus row (xulydonhang.py:4810-4848).
	// Unlike the per-item bonus row above, this is checked ONCE PER STORE
	// PAGE using THIS page's own local tongtien/weightKg accumulators —
	// write_to_dondathang_bigc is called once per store page, each with
	// its own tongtien reset to 0 at the top of the call
	// (xulydonhang.py:4565-4566), so this block genuinely differs in
	// scope from Coop/Lotte/Satra's buildInvoiceBonusRow (processor_shared.go),
	// which checks this once for the whole order. Reuses
	// priceIndex.FindInvoicePromotion, matching
	// find_all_promotions_by_sku_and_time("Hóa Đơn", entry_date, vendor)
	// + take the first match's value (kmhoadon[0][1]) exactly.
	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		// Same CR normalization as the per-item khuyenmai above, same root
		// cause (openpyxl's write/read round-trip silently turns '\r' into
		// '\n') — applied before every downstream use, mirroring the
		// per-item block's own ordering.
		invoicePromo = strings.ReplaceAll(invoicePromo, "\r", "\n")

		// kiemtra[0] (xulydonhang.py:4826): Python writes ONLY the FIRST
		// mentioned SKU to column Q here — deliberately NOT the
		// comma-joined list buildInvoiceBonusRow uses for Coop/Lotte/Satra.
		// Confirmed real difference, do not "fix" this to match the shared
		// helper's shape.
		invoiceSkus := p.Store.FindSkusMentioned(invoicePromo)
		amount, amountOK := coop.ExtractMoneyAmount(invoicePromo)

		// Python has no guard before indexing kiemtra[0] or dividing by
		// tachtien_khuyenmai(kmhoadon) here — an empty SKU match or an
		// unparseable money amount would raise in real Python. Skipping
		// cleanly instead of replicating an undefined crash, per this
		// port's error-handling policy.
		if len(invoiceSkus) > 0 && amountOK && amount > 0 {
			bonusSKU := invoiceSkus[0]
			bonusQty := math.Floor(tongtien / float64(amount))
			bonusInfo, _ := p.Store.GetProductInfo(bonusSKU)
			bonusWeight := bonusInfo.WeightKg * bonusQty
			weightKg += bonusWeight

			// laycachbo_khuyenmai(kmhoadon), falling back to the SAME
			// default text Coop/Satra use ("KM Bó Kèm - Che Barcode",
			// xulydonhang.py:4845) — NOT this file's per-item fallback
			// ("KM Rời - Không Che Barcode"), which is a different branch
			// entirely and stays untouched.
			promoNote := coop.ExtractBraceContent(invoicePromo)
			if promoNote == "" {
				promoNote = "KM Bó Kèm - Che Barcode"
			}

			rows = append(rows, excelwriter.Row{
				EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
				Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
				Description: description, SKU: bonusSKU, Warehouse: warehouse, VATPercent: 8,
				RegionCode: region, StatCode: statCode, IsPromoItem: true, Qty: bonusQty,
				ProductName: bonusInfo.Name, LineWeightKg: bonusWeight, PromoNote: promoNote,
				PromoContent: invoicePromo, UseZFormula: false, NoCaseCount: true,
			})
			accumulatePromoItem(promoTotals, bonusSKU, bonusInfo.Name, bonusQty)
		}
	}

	return storePageResult{rows: rows, weightKg: weightKg, saigia: saigia, tongtien: tongtien, skuLog: skuLog, mismatchDetails: mismatchDetails, promoItems: finalizePromoItems(promoTotals)}
}

// parseNumericField mirrors the repeated "strip commas, coerce to
// float/int" pattern applied to item["SKU/OU"] and item["OU Qty"]
// (xulydonhang.py:4632-4640) and to a fetched price string — returns 0
// on any parse failure rather than panicking, since a malformed numeric
// field should surface as a price mismatch / zero quantity downstream,
// not crash the whole store page. Also reused by Winmart's own numeric-
// field parsing (xulydonhang.py:4327-4337's equivalent "strip commas,
// coerce to float" pattern applied to OU Qty and Total Price).
//
// Also strips surrounding whitespace before parsing — confirmed a real
// Go bug, not a style choice: Python's own equivalent conversions
// (xulydonhang.py:4356 `giathuctegoc.replace(",", "").strip()`, and
// repeated identically at :4389-4390, :4400, :4422 for giathucte) always
// call .strip() immediately before float(...). This matters because
// pricing.Index.FindPrice's returned price string can carry real
// leading/trailing whitespace straight from the source sheet (e.g. the
// real frozen Winmart pricing fixture's raw row for SKU TP31203 has
// price "  162.272   ", confirmed in
// winmart/testdata/fixtures/_frozen_pricing.json) — Index.FindPrice
// intentionally preserves that whitespace (mirroring
// find_price_by_sku's own `return price` with only a .strip()-based
// non-empty CHECK, not a stripped return value, xulydonhang.py:5687-
// 5690), so the trim has to happen here, at the float-conversion site,
// exactly where Python's own .strip() calls live. Without this,
// strconv.ParseFloat rejects the untrimmed string outright and silently
// returns 0 — confirmed as the root cause of every real-fixture Y-column
// "got 0, want <price>" mismatch for Winmart's golden test (SKUs
// TP31203, TP31630, TP32767, TP30992_02, all of which have this same
// whitespace-padded price cell in the real frozen pricing data).
func parseNumericField(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(s, ",", "")), 64)
	if err != nil {
		return 0
	}
	return v
}
