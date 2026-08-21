package processing

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/productdata"
)

// coopDebtDays is songayno_MT in xulydonhang.py — one global constant,
// shared by every vendor's write function, not Coop-specific.
const coopDebtDays = 60

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

func closeEnough(a, b float64) bool {
	const relTol = 1e-4
	return math.Abs(a-b) <= relTol*math.Max(math.Abs(a), math.Abs(b))
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
