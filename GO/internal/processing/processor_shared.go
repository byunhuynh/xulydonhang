package processing

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/productdata"
)

// coopDebtDays is songayno_MT in xulydonhang.py — one global constant,
// shared by every vendor's write function, not Coop-specific.
const coopDebtDays = 60

// accumulatePromoItem folds one bonus/promo row's (SKU, ProductName, Qty)
// into totals, grouped by SKU — every vendor's processSegment calls this
// once at each point a bonus row is actually added (buildPromoBonusRow's
// and buildInvoiceBonusRow's own `added`/`ok` returns, or BigC's inline
// equivalent), the same call sites that already accumulate that bonus
// row's weight/case-count into totalWeight/totalPackages. A blank SKU or
// zero qty is skipped rather than polluting the summary with a
// meaningless zero-quantity entry.
func accumulatePromoItem(totals map[string]*PromoItemSummary, sku, productName string, qty float64) {
	if sku == "" || qty == 0 {
		return
	}
	if existing, ok := totals[sku]; ok {
		existing.Qty += qty
		return
	}
	totals[sku] = &PromoItemSummary{SKU: sku, ProductName: productName, Qty: qty}
}

// finalizePromoItems converts the accumulator map into the []PromoItemSummary
// OrderRow.PromoItems actually serializes, sorted by SKU for a stable,
// deterministic message/UI ordering. Uses make(...) (never a nil slice)
// so an order with no bonus items still serializes PromoItems as JSON
// "[]", not "null" — see OrderRow.PromoItems' own doc comment.
func finalizePromoItems(totals map[string]*PromoItemSummary) []PromoItemSummary {
	items := make([]PromoItemSummary, 0, len(totals))
	for _, v := range totals {
		items = append(items, *v)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].SKU < items[j].SKU })
	return items
}

// xPlus1Pattern mirrors the "(\d+)\s*\+\s*1" match inside
// write_to_dondathang's promo-bonus-quantity logic.
var xPlus1Pattern = regexp.MustCompile(`(\d+)\s*\+\s*1`)

// regionInfo mirrors write_to_dondathang's warehouse/region branching:
// customer codes starting with "MB" (Miền Bắc) map to the Hà Nội
// warehouse; everything else defaults to Miền Nam / Long An. Confirmed
// vendor-neutral for Coop, Lotte, and Satra's customer-code shapes — but
// NOT a fit for BigC's warehouse table, which needs a genuine 3-way
// split (MB / MN_MT / MN_GC) with a different Miền Nam warehouse per
// branch (see bigc_processor.go's own bigcRegionInfo, which does NOT
// call this function — extending this one instead would silently change
// Satra's already-shipped "MN_MT_*" codes' resolved warehouse).
func regionInfo(customerCode string) (region, statCode, warehouse string) {
	if strings.HasPrefix(customerCode, "MB") {
		return "MT_MB", "HN", "TP_HN_12"
	}
	return "MT_MN", "LA", "LA_TP"
}

// closeEnough quyết định một dòng sản phẩm có "đúng giá" hay không:
// giá hóa đơn và giá hệ thống — CẢ HAI đều là ĐƠN GIÁ của 1 sản phẩm,
// không phải thành tiền — chỉ được lệch tối đa 1 ĐỒNG.
//
// Trước đây hàm này port nguyên math.isclose(rel_tol=1e-4) của bản
// Python (xulydonhang.py:1122), tức khoảng dung sai nở theo giá: giá
// 5.000đ chỉ được lệch 0,5đ nhưng giá 133.806đ lại được lệch tới 13đ.
// Nguồn sai lệch thật duy nhất là phần lẻ đồng sinh ra khi áp % giảm
// lên một mức giá chẵn (giá gốc - giá gốc*%/100) — phần lẻ này luôn
// nhỏ hơn 1đ bất kể giá lớn hay nhỏ, nên ngưỡng tuyệt đối 1đ vừa chặt
// hơn ở giá cao vừa rộng hơn ở giá thấp, đúng với ý nghĩa nghiệp vụ
// (quyết định của người dùng, 2026-08-26).
func closeEnough(a, b float64) bool {
	const maxDiffDong = 1.0
	// Khe hở float64 cực nhỏ: đơn giá sau khi chia % giảm là số thực,
	// nên hai giá "lệch đúng 1đ" trên giấy có thể cho hiệu
	// 1.0000000000145 và bị báo sai giá oan nếu so <= 1.0 trần trụi.
	const floatSlack = 1e-6
	return math.Abs(a-b) <= maxDiffDong+floatSlack
}

// appliedUnitPrice chọn đơn giá THỰC SỰ ghi xuống cột Y của
// "Don dat hang" cho một dòng sản phẩm.
//
// Khi giá khớp (chênh lệch trong khoảng closeEnough cho phép — xem hàm
// đó) thì ghi ĐƠN GIÁ CỦA PO, không ghi giá hệ thống. Lý do nghiệp vụ:
// PO của siêu thị luôn là số đồng chẵn, còn giá hệ thống sau khi áp %
// giảm hay sinh phần lẻ dưới 1đ (ví dụ 75136*0.6 = 45081.6 trong khi PO
// ghi 45082). Hai giá này coi như bằng nhau về mặt đối chiếu, nhưng nếu
// ghi con số lẻ xuống Excel thì hoá đơn xuất ra sẽ lệch vài đồng so với
// PO — nên lấy luôn số của PO (quyết định của người dùng, 2026-08-26).
//
// Khi giá KHÔNG khớp thì vẫn ghi giá hệ thống, y như trước: dòng đó
// được tô đỏ kèm comment để người dùng tự quyết (xem excelwriter's
// PriceMismatch và ConfirmPrice) — không được âm thầm lấy giá PO cho
// một chênh lệch chưa được xác nhận.
//
// Quy tắc này vốn đã là hành vi của RIÊNG Satra ngay từ bản Python
// (xulydonhang.py:2495/:2521, đối chiếu được với fixture thật của
// P-000022974.pdf), nay áp dụng cho mọi nhà cung cấp.
func appliedUnitPrice(matched bool, invoicePrice, systemPrice float64) float64 {
	if matched {
		return invoicePrice
	}
	return systemPrice
}

// skuLogPromoMaxLen caps how much of a promo's raw text (which can be a
// full multi-line sheet cell) appears in one log line, so a long promo
// description doesn't dominate the log panel.
const skuLogPromoMaxLen = 60

// formatSkuLogLine builds one human-readable diagnostic line for a
// single product — price-match status and any promotion detected —
// surfaced in real time via the "process:log" channel (see app.go's
// runBatch, which emits OrderRow.SkuLog before that row's own
// "process:row"). Pure formatting of values every vendor's per-product
// loop already computes for its own Excel write (matched, khuyenmai,
// invoicePrice, the system's own expected price) plus promoDateRange —
// the pricing sheet's own column header for whatever promo row matched
// (a "D/M-D/M" range, e.g. "1/1-31/12"; see pricing.Promotion.Column),
// already available in every vendor's promo loop but not previously
// threaded through. This function adds no new computation, so it
// carries zero risk to any vendor's existing price/promo logic.
func formatSkuLogLine(sku, productName string, matched bool, invoicePrice, systemPrice float64, promoText, promoDateRange string) string {
	label := sku
	if productName != "" {
		label = sku + " " + productName
	}
	promo := truncatePromoText(promoText)
	promoSuffix := ""
	if promo != "" && promoDateRange != "" {
		promoSuffix = fmt.Sprintf(" (áp dụng %s)", promoDateRange)
	}

	if matched {
		if promo == "" {
			return fmt.Sprintf("%s — Đúng giá", label)
		}
		return fmt.Sprintf("%s — Đúng giá, KM: %s%s", label, promo, promoSuffix)
	}
	if promo == "" {
		return fmt.Sprintf("%s — ⚠️ SAI GIÁ! Giá đúng: %.0f, Giá trên PO: %.0f", label, systemPrice, invoicePrice)
	}
	return fmt.Sprintf("%s — ⚠️ SAI GIÁ! Giá đúng: %.0f, Giá trên PO: %.0f, đã thử KM: %s%s", label, systemPrice, invoicePrice, promo, promoSuffix)
}

// truncatePromoText collapses a (possibly multi-line, CR-normalized)
// promo cell into one short, log-line-friendly snippet.
func truncatePromoText(promo string) string {
	if promo == "" {
		return ""
	}
	oneLine := strings.Join(strings.Fields(promo), " ")
	// Truncate by rune, not byte, so a cut point never lands mid-way
	// through a multi-byte UTF-8 Vietnamese character.
	runes := []rune(oneLine)
	if len(runes) > skuLogPromoMaxLen {
		return string(runes[:skuLogPromoMaxLen]) + "..."
	}
	return oneLine
}

// stripBlankLines drops every line that is empty or all-whitespace,
// rejoining the rest with "\n". Originally written for Lotte (see
// processLotteSegment's comment for why this is needed: it reconstructs
// the blank-line-free shape PyMuPDF's text extraction produces from this
// repo's Go PDF library's GetPlainText output, which inserts extra blank
// lines PyMuPDF does not) — confirmed the same underlying library quirk
// also affects Satra's PDF template (see normalizeSatraText in
// satra_processor.go), so this now lives here as shared infrastructure.
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

// buildPromoBonusRow mirrors Coop's write_to_dondathang bonus-row
// construction (xulydonhang.py:1174-1211) and Satra's identical-shaped
// equivalent (:2555-2612) — confirmed NOT a fit for BigC's
// write_to_dondathang_bigc (see bigc_processor.go's own row-builder).
// orderNumber is the CALLER's fully-formed order-number string (e.g.
// "ĐĐHCOOP-12345", "ĐĐHLOTTE-12345", "ĐĐHSATRA-P-12345") — this function
// no longer hardcodes Coop's own orderNumber() formatter internally
// (Task 1 of this plan removed that), so every caller must pass its own
// vendor-correct order number directly; no post-patch needed afterward.
func buildPromoBonusRow(store *productdata.Store, promoPart string, product coop.Product, index int,
	entryDate, cancelDate, shipTo, customerCode, description, warehouse, region, statCode, orderNumber string,
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
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber,
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

// buildInvoiceBonusRow mirrors Coop's/Satra's invoice-level promo bonus
// row. orderNumber is the caller's fully-formed order-number string (see
// buildPromoBonusRow's doc comment — same Task 1 parameterization).
func buildInvoiceBonusRow(store *productdata.Store, invoicePromo string, totalValue float64,
	entryDate, cancelDate, shipTo, customerCode, description, warehouse, region, statCode, orderNumber string,
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
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNumber,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
		Description: description, SKU: strings.Join(skus, ", "), Warehouse: warehouse, VATPercent: 8,
		RegionCode: region, StatCode: statCode, IsPromoItem: true, Qty: bonusQty, ProductName: bonusInfo.Name,
		CaseCount: bonusCase, LineWeightKg: bonusWeight, PromoNote: bundleNote, PromoContent: invoicePromo,
		UseZFormula: false,
	}, true
}

// excelRowsFrom tra ve danh sach so dong TUYET DOI ma mot don vua chiem
// trong so dat hang: excelwriter.WriteOrderRows ghi count dong lien tiep
// bat dau tu startRow.
//
// OrderRow.ExcelRows la thu duy nhat noi mot don tren bang ket qua voi
// nhung dong that cua no trong dondathang.xlsx. Push MISA dua vao no de
// tach file theo nhanh ke toan; de trong thi don do bi bo qua im lang -
// da tung xay ra voi ca 8 vendor tru BigC va JIT.
func excelRowsFrom(startRow, count int) []int {
	if count <= 0 {
		return nil
	}
	out := make([]int, count)
	for i := range out {
		out[i] = startRow + i
	}
	return out
}
